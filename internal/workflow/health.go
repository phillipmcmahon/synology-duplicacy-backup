package workflow

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	healthpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/health"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type HealthIssue = healthpkg.Issue
type HealthCheck = healthpkg.Check
type HealthRevisionResult = healthpkg.RevisionResult
type HealthReport = healthpkg.Report

type HealthRunner struct {
	meta      Metadata
	rt        Runtime
	log       *logger.Logger
	runner    execpkg.Runner
	presenter *healthpkg.Presenter
}

const (
	verifyFailureNoRevisionsFound = healthpkg.VerifyFailureNoRevisionsFound
	verifyFailureIntegrityFailed  = healthpkg.VerifyFailureIntegrityFailed
	verifyFailureResultMissing    = healthpkg.VerifyFailureResultMissing
	verifyFailureAccessFailed     = healthpkg.VerifyFailureAccessFailed
	verifyFailureListingFailed    = healthpkg.VerifyFailureListingFailed
	verifyActionRunBackup         = "run_backup"
)

func NewHealthRunner(meta Metadata, rt Runtime, log *logger.Logger, runner execpkg.Runner) *HealthRunner {
	return &HealthRunner{
		meta:      meta,
		rt:        rt,
		log:       log,
		runner:    runner,
		presenter: healthpkg.NewPresenter(log, rt.Now),
	}
}

func NewFailureHealthReport(req *Request, checkType, message string, checkedAt time.Time) *HealthReport {
	mode := ""
	label := ""
	target := ""
	if req != nil {
		label = req.Source
		target = req.Target()
		mode = req.Target()
		if checkType == "" {
			checkType = req.HealthCommand
		}
	}
	return healthpkg.NewFailureReport(checkType, label, target, mode, message, checkedAt)
}

func WriteHealthReport(w io.Writer, report *HealthReport) error {
	return healthpkg.WriteReport(w, report)
}

func (h *HealthRunner) Run(req *Request) (*HealthReport, int) {
	checkedAt := h.rt.Now()
	report := &HealthReport{
		Status:    "healthy",
		CheckType: req.HealthCommand,
		Label:     req.Source,
		Target:    req.Target(),
		Mode:      req.Target(),
		CheckedAt: formatReportTime(checkedAt),
		StartedAt: checkedAt,
	}

	restoreDebugCommands := h.suppressCommandDebug()
	defer restoreDebugCommands()

	cfg, plan, sec, err := h.prepare(req)
	if err != nil {
		h.presenter.PrintHeader(report)
		report.AddCheck("Environment", "fail", OperatorMessage(err))
		report.Finalize()
		h.maybeSendEarlyNotification(req, report)
		h.presenter.PrintReport(report)
		return report, healthpkg.ExitCode(report.Status)
	}
	report.StorageType = plan.StorageType
	report.Location = plan.Location
	h.presenter.PrintHeader(report)

	state, stateErr := loadRunState(h.meta, req.Source, req.Target())
	if stateErr != nil {
		if os.IsNotExist(stateErr) {
			report.AddCheck("Backup state", "warn", fmt.Sprintf("No prior backup state found at %s", stateFilePath(h.meta, req.Source, req.Target())))
		} else {
			report.AddCheck("Backup state", "warn", fmt.Sprintf("Could not read backup state: %v", stateErr))
		}
	} else {
		report.AddCheck("Backup state", "pass", "Available")
		report.LastSuccessAt = chooseLocalSuccessTime(state)
	}

	lockStatus, lockErr := lock.InspectTarget(h.meta.LockParent, req.Source, req.Target())
	if lockErr != nil {
		report.AddCheck("Lock", "warn", OperatorMessage(lockErr))
	} else {
		switch {
		case lockStatus.Active:
			report.AddCheck("Lock", "warn", fmt.Sprintf("Active lock detected (PID %d)", lockStatus.PID))
		case lockStatus.Stale:
			report.AddCheck("Lock", "warn", "Stale lock detected")
		default:
			report.AddCheck("Lock", "pass", "No active lock")
		}
	}

	dup, dupErr := h.prepareDuplicacySetup(plan, sec)
	if dupErr != nil {
		report.AddCheck("Duplicacy setup", "fail", OperatorMessage(dupErr))
		report.Finalize()
		h.maybeSendEarlyNotification(req, report)
		h.presenter.PrintReport(report)
		return report, healthpkg.ExitCode(report.Status)
	}
	defer dup.Cleanup()

	visibleRevisions := h.runStatusChecks(report, req, cfg, plan, state, dup)
	if req.HealthCommand == "doctor" || req.HealthCommand == "verify" {
		h.runDoctorChecks(report, req, cfg, plan, dup)
	}
	if req.HealthCommand == "verify" {
		h.runVerifyChecks(report, cfg, dup, visibleRevisions)
	}

	if err := updateHealthCheckState(h.meta, req.Source, req.Target(), req.HealthCommand, checkedAt); err != nil {
		report.AddCheck("Health state", "warn", err.Error())
	}

	report.Finalize()
	if h.shouldSendNotification(req, cfg.Health, report.Status) {
		if payload := buildHealthNotificationPayload(h.rt, report); payload != nil {
			if err := notify.SendConfigured(cfg.Health.Notify, plan.SecretsFile, report.Target, payload); err != nil {
				report.AddCheck("Notification", "warn", OperatorMessage(err))
			} else {
				report.NotificationSent = true
				report.AddCheck("Notification", "pass", "Delivered")
			}
		}
	}
	report.Finalize()
	h.presenter.PrintReport(report)
	return report, healthpkg.ExitCode(report.Status)
}

func (h *HealthRunner) prepare(req *Request) (*config.Config, *Plan, *secrets.Secrets, error) {
	if h.rt.Geteuid() != 0 {
		return nil, nil, nil, NewMessageError("Health commands must be run as root")
	}
	if _, err := h.rt.LookPath("duplicacy"); err != nil {
		return nil, nil, nil, NewMessageError("Required command 'duplicacy' not found")
	}

	planner := NewPlanner(h.meta, h.rt, h.log, h.runner)
	plan := planner.derivePlan(req)

	cfgReq := configValidationRequest(req, req.Target())
	cfgPlan := planner.derivePlan(cfgReq)
	cfg, err := planner.loadConfig(cfgPlan)
	if err != nil {
		return nil, nil, nil, err
	}

	plan.Target = cfg.Target
	plan.StorageType = cfg.StorageType
	plan.Location = cfg.Location
	plan.ConfigFile = cfgPlan.ConfigFile
	plan.SecretsFile = cfgPlan.SecretsFile
	plan.BackupTarget = JoinDestination(cfg.StorageType, cfg.Destination, cfg.Repository)
	plan.SnapshotSource = cfg.SourcePath
	plan.RepositoryPath = cfg.SourcePath
	plan.ModeDisplay = modeDisplay(plan.TargetName(), plan.StorageType)
	plan.OperationMode = "Health " + strings.Title(req.HealthCommand)
	plan.LocalOwner = cfg.LocalOwner
	plan.LocalGroup = cfg.LocalGroup
	plan.LogRetentionDays = cfg.LogRetentionDays
	plan.Filter = cfg.Filter
	plan.FilterLines = splitNonEmptyLines(cfg.Filter)
	plan.PruneOptions = cfg.Prune
	plan.Threads = cfg.Threads

	var sec *secrets.Secrets
	if cfg.UsesObjectStorage() {
		sec, err = planner.loadSecrets(cfgPlan)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	plan.Secrets = sec
	return cfg, plan, sec, nil
}

func (h *HealthRunner) prepareDuplicacySetup(plan *Plan, sec *secrets.Secrets) (*duplicacy.Setup, error) {
	dup := duplicacy.NewSetup(plan.WorkRoot, plan.RepositoryPath, plan.BackupTarget, false, h.runner)
	if err := dup.CreateDirs(); err != nil {
		return nil, err
	}
	if err := dup.WritePreferences(sec); err != nil {
		return nil, err
	}
	if plan.Filter != "" {
		if err := dup.WriteFilters(plan.Filter); err != nil {
			return nil, err
		}
	}
	if err := dup.SetPermissions(); err != nil {
		return nil, err
	}
	return dup, nil
}

func (h *HealthRunner) runStatusChecks(report *HealthReport, req *Request, cfg *config.Config, plan *Plan, state *RunState, dup *duplicacy.Setup) []duplicacy.RevisionInfo {
	report.AddCheck("Config file", "pass", plan.ConfigFile)
	defer func() {
		if plan.UsesObjectStorage() {
			report.AddCheck("Secrets", "pass", plan.SecretsFile)
		}
	}()

	stopInspecting := h.presenter.StartStatusActivity("Checking stored revisions")
	revisions, _, err := dup.ListVisibleRevisions()
	stopInspecting()
	if err != nil {
		if req.HealthCommand == "verify" {
			report.AddVerifyFailureCode(verifyFailureListingFailed)
		}
		report.AddCheck("Latest revision", "fail", OperatorMessage(err))
		return nil
	}
	report.RevisionCount = len(revisions)
	if len(revisions) == 0 {
		report.AddCheck("Revision count", "warn", "0")
		report.AddCheck("Latest revision", "warn", "No revisions were visible in storage")
		return nil
	}
	report.AddCheck("Revision count", "pass", fmt.Sprintf("%d", len(revisions)))
	latest := revisions[0]
	report.LatestRevision = latest.Revision
	if !latest.CreatedAt.IsZero() {
		report.LatestRevisionAt = formatReportTime(latest.CreatedAt)
		report.AddCheck("Latest revision", "pass", fmt.Sprintf("%d (%s)", latest.Revision, latest.CreatedAt.Format("2006-01-02 15:04:05")))
	} else {
		report.AddCheck("Latest revision", "pass", fmt.Sprintf("%d", latest.Revision))
	}

	if !latest.CreatedAt.IsZero() {
		h.evaluateFreshness(report, cfg.Health, latest.CreatedAt, "Backup freshness")
		return revisions
	}

	if state == nil {
		report.AddCheck("Backup freshness", "warn", "Latest revision time is unavailable and no backup state exists")
		return revisions
	}
	if state.LastSuccessfulBackupAt == "" {
		report.AddCheck("Backup freshness", "warn", "Latest revision time is unavailable and no backup timestamp is recorded")
		return revisions
	}

	if state.LastSuccessfulBackupRevision == latest.Revision {
		report.LatestRevisionAt = state.LastSuccessfulBackupAt
		if parsed, parseErr := time.Parse(time.RFC3339, state.LastSuccessfulBackupAt); parseErr == nil {
			h.evaluateFreshness(report, cfg.Health, parsed, "Backup freshness")
		} else {
			report.AddCheck("Backup freshness", "warn", "Recorded backup timestamp is invalid")
		}
		return revisions
	}

	report.AddCheck("Backup freshness", "warn", fmt.Sprintf("Latest revision %d does not match recorded backup revision %d", latest.Revision, state.LastSuccessfulBackupRevision))
	return revisions
}

func (h *HealthRunner) runDoctorChecks(report *HealthReport, req *Request, cfg *config.Config, plan *Plan, dup *duplicacy.Setup) {
	if _, err := os.Stat(plan.SnapshotSource); err != nil {
		report.AddCheck("Source path", "fail", fmt.Sprintf("Source path is not accessible: %v", err))
	} else {
		report.AddCheck("Source path", "pass", plan.SnapshotSource)
	}
	rootErr := btrfs.CheckVolume(h.runner, h.meta.RootVolume, false)
	sourceErr := btrfs.CheckVolume(h.runner, plan.SnapshotSource, false)
	switch {
	case rootErr == nil && sourceErr == nil:
		report.AddCheck("Btrfs", "pass", "Yes")
	default:
		if rootErr != nil {
			report.AddCheck("Btrfs root", "fail", OperatorMessage(rootErr))
		}
		if sourceErr != nil {
			report.AddCheck("Btrfs source", "fail", OperatorMessage(sourceErr))
		}
	}

	stopValidating := h.presenter.StartStatusActivity("Validating repository access")
	if err := dup.ValidateRepo(); err != nil {
		stopValidating()
		if req.HealthCommand == "verify" {
			report.AddVerifyFailureCode(verifyFailureAccessFailed)
		}
		report.AddCheck("Repository access", "fail", OperatorMessage(err))
	} else {
		stopValidating()
		report.AddCheck("Repository access", "pass", "Validated")
	}

	h.evaluateHealthRecency(report, cfg.Health, "doctor", "Last doctor run")
}

func (h *HealthRunner) runVerifyChecks(report *HealthReport, cfg *config.Config, dup *duplicacy.Setup, revisions []duplicacy.RevisionInfo) {
	if report.HasVerifyFailureCode(verifyFailureListingFailed) {
		report.RevisionCount = 0
		report.CheckedRevisionCount = 0
		report.PassedRevisionCount = 0
		report.FailedRevisionCount = 0
		report.AddDisplayCheck("Revisions checked", "fail", "0")
		report.AddDisplayCheck("Revisions passed", "pass", "0")
		report.AddDisplayCheck("Revisions failed", "pass", "0")
		report.AddCheck("Integrity check", "fail", "Revision inspection failed")
		h.populateHealthRecencyTimestamp(report, "verify")
		return
	}

	if len(revisions) == 0 {
		report.AddVerifyFailureCode(verifyFailureNoRevisionsFound)
		report.AddDisplayCheck("Revisions checked", "fail", "0")
		report.AddDisplayCheck("Revisions passed", "pass", "0")
		report.AddDisplayCheck("Revisions failed", "pass", "0")
		report.AddCheck("Integrity check", "fail", "No revisions were found for this backup")
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}

	status := fmt.Sprintf("Checking revision integrity for this backup (%d total)", len(revisions))
	stopVerifying := h.presenter.StartStatusActivity(status)
	results, _, err := dup.CheckVisibleRevisions()
	stopVerifying()
	if err != nil {
		report.AddVerifyFailureCode(verifyFailureAccessFailed)
		report.CheckedRevisionCount = 0
		report.PassedRevisionCount = 0
		report.FailedRevisionCount = 0
		report.AddDisplayCheck("Revisions checked", "fail", "0")
		report.AddDisplayCheck("Revisions passed", "pass", "0")
		report.AddDisplayCheck("Revisions failed", "pass", "0")
		report.AddCheck("Integrity check", "fail", "Integrity check did not complete")
		h.populateHealthRecencyTimestamp(report, "verify")
		return
	}

	visibleByRevision := make(map[int]duplicacy.RevisionInfo, len(revisions))
	for _, revision := range revisions {
		visibleByRevision[revision.Revision] = revision
	}
	accounted := make(map[int]bool, len(results))
	report.CheckedRevisionCount = len(results)
	var detailChecks []HealthCheck

	for _, result := range results {
		revisionInfo, ok := visibleByRevision[result.Revision]
		if !ok {
			continue
		}
		accounted[result.Revision] = true
		if result.Result == "fail" {
			report.AddVerifyFailureCode(verifyFailureIntegrityFailed)
			entry := HealthRevisionResult{
				Revision: result.Revision,
				Result:   result.Result,
				Message:  normaliseOperatorSentence(result.Message),
			}
			if !revisionInfo.CreatedAt.IsZero() {
				entry.CreatedAt = formatReportTime(revisionInfo.CreatedAt)
			}
			report.RevisionResults = append(report.RevisionResults, entry)
			report.FailedRevisionCount++
			report.FailedRevisions = append(report.FailedRevisions, result.Revision)
			detailChecks = append(detailChecks, HealthCheck{
				Name:    fmt.Sprintf("Revision %d", result.Revision),
				Result:  "fail",
				Message: result.Message,
			})
			continue
		}
		report.PassedRevisionCount++
	}

	missing := make([]int, 0)
	for _, revision := range revisions {
		if accounted[revision.Revision] {
			continue
		}
		report.AddVerifyFailureCode(verifyFailureResultMissing)
		missing = append(missing, revision.Revision)
		entry := HealthRevisionResult{
			Revision: revision.Revision,
			Result:   "fail",
			Message:  "No integrity result returned",
		}
		if !revision.CreatedAt.IsZero() {
			entry.CreatedAt = formatReportTime(revision.CreatedAt)
		}
		report.RevisionResults = append(report.RevisionResults, entry)
		detailChecks = append(detailChecks, HealthCheck{
			Name:    fmt.Sprintf("Revision %d", revision.Revision),
			Result:  "fail",
			Message: "No integrity result returned",
		})
	}

	verifiedResult := "pass"
	if len(missing) > 0 {
		verifiedResult = "fail"
	}
	report.AddDisplayCheck("Revisions checked", verifiedResult, fmt.Sprintf("%d", report.CheckedRevisionCount))
	report.AddDisplayCheck("Revisions passed", "pass", fmt.Sprintf("%d", report.PassedRevisionCount))
	failedResult := "pass"
	if report.FailedRevisionCount > 0 {
		failedResult = "fail"
	}
	failedSummary := fmt.Sprintf("%d", report.FailedRevisionCount)
	if report.FailedRevisionCount > 0 {
		failedSummary = fmt.Sprintf("%d (%s)", report.FailedRevisionCount, healthpkg.SummariseRevisionIDs(report.FailedRevisions, 4))
	}
	report.AddDisplayCheck("Revisions failed", failedResult, failedSummary)

	if len(missing) > 0 {
		report.AddCheck("Integrity check", "fail", healthpkg.IntegrityCheckFailureMessage(report.FailedRevisions, missing))
		for _, check := range detailChecks {
			report.AddDisplayCheck(check.Name, check.Result, check.Message)
		}
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}
	if report.FailedRevisionCount > 0 {
		report.AddCheck("Integrity check", "fail", healthpkg.IntegrityCheckFailureMessage(report.FailedRevisions, nil))
		for _, check := range detailChecks {
			report.AddDisplayCheck(check.Name, check.Result, check.Message)
		}
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}
	report.AddCheck("Integrity check", "pass", "All revisions validated")
	h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
}

func (h *HealthRunner) evaluateFreshness(report *HealthReport, cfg config.HealthConfig, last time.Time, checkName string) {
	age := h.rt.Now().Sub(last)
	warnAfter := time.Duration(cfg.FreshnessWarnHours) * time.Hour
	failAfter := time.Duration(cfg.FreshnessFailHours) * time.Hour
	message := fmt.Sprintf("%s old", healthpkg.HumanAge(age))
	switch {
	case failAfter > 0 && age > failAfter:
		report.AddCheck(checkName, "fail", message)
	case warnAfter > 0 && age > warnAfter:
		report.AddCheck(checkName, "warn", message)
	default:
		report.AddCheck(checkName, "pass", message)
	}
}

func (h *HealthRunner) evaluateHealthRecency(report *HealthReport, cfg config.HealthConfig, kind, name string) {
	_, at, ok := h.loadHealthRecencyTime(report, kind)
	if !ok {
		report.AddCheck(name, "warn", "No prior health state is available")
		return
	}
	var thresholdHours int
	switch kind {
	case "doctor":
		thresholdHours = cfg.DoctorWarnAfter
	case "verify":
		thresholdHours = cfg.VerifyWarnAfter
	default:
		return
	}
	age := h.rt.Now().Sub(at)
	if thresholdHours > 0 && age > time.Duration(thresholdHours)*time.Hour {
		report.AddCheck(name, "warn", healthpkg.HumanAgo(age))
		return
	}
	report.AddCheck(name, "pass", healthpkg.HumanAgo(age))
}

func (h *HealthRunner) populateHealthRecencyTimestamp(report *HealthReport, kind string) bool {
	_, _, ok := h.loadHealthRecencyTime(report, kind)
	return ok
}

func (h *HealthRunner) loadHealthRecencyTime(report *HealthReport, kind string) (*RunState, time.Time, bool) {
	state, err := loadRunState(h.meta, report.Label, report.Target)
	if err != nil || state == nil {
		return nil, time.Time{}, false
	}

	var last string
	switch kind {
	case "doctor":
		last = state.LastDoctorAt
	case "verify":
		last = state.LastVerifyAt
	default:
		return state, time.Time{}, false
	}
	if last == "" {
		return state, time.Time{}, false
	}
	at, parseErr := time.Parse(time.RFC3339, last)
	if parseErr != nil {
		return state, time.Time{}, false
	}
	switch kind {
	case "doctor":
		report.LastDoctorRunAt = formatReportTime(at)
	case "verify":
		report.LastVerifyRunAt = formatReportTime(at)
	}
	return state, at, true
}

func (h *HealthRunner) shouldSendNotification(req *Request, cfg config.HealthConfig, status string) bool {
	if req == nil {
		return false
	}
	if !shouldSendConfiguredNotification(h.rt, h.log.Interactive(), cfg.Notify, req.HealthCommand) {
		return false
	}
	if !containsString(cfg.Notify.NotifyOn, status) {
		return false
	}
	return true
}

func (h *HealthRunner) maybeSendEarlyNotification(req *Request, report *HealthReport) {
	if req == nil || report == nil {
		return
	}
	cfg, secretsFile, ok := h.loadHealthNotifyConfig(req)
	if !ok || !h.shouldSendNotification(req, cfg, report.Status) {
		return
	}
	payload := buildHealthNotificationPayload(h.rt, report)
	if payload == nil {
		return
	}
	if err := notify.SendConfigured(cfg.Notify, secretsFile, report.Target, payload); err != nil {
		report.AddCheck("Notification", "warn", OperatorMessage(err))
		return
	}
	report.NotificationSent = true
	report.AddCheck("Notification", "pass", "Delivered")
}

func (h *HealthRunner) loadHealthNotifyConfig(req *Request) (config.HealthConfig, string, bool) {
	if req == nil || req.Source == "" {
		return config.HealthConfig{}, "", false
	}

	planner := NewPlanner(h.meta, h.rt, h.log, h.runner)
	plan := planner.derivePlan(configValidationRequest(req, req.Target()))
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return config.HealthConfig{}, "", false
	}
	if err := cfg.Health.Validate(); err != nil {
		return config.HealthConfig{}, "", false
	}
	return cfg.Health, plan.SecretsFile, true
}

func (h *HealthRunner) suppressCommandDebug() func() {
	type commandDebugController interface {
		SetDebugCommands(bool)
		DebugCommands() bool
	}

	controller, ok := h.runner.(commandDebugController)
	if !ok {
		return func() {}
	}

	previous := controller.DebugCommands()
	controller.SetDebugCommands(false)
	return func() {
		controller.SetDebugCommands(previous)
	}
}

func chooseLocalSuccessTime(state *RunState) string {
	if state == nil {
		return ""
	}
	if state.LastSuccessfulBackupAt != "" {
		return state.LastSuccessfulBackupAt
	}
	return state.LastSuccessfulRunAt
}

func healthCheckSection(name string) string { return healthpkg.SectionForCheck(name) }
func healthCheckLabel(name string) string   { return healthpkg.LabelForCheck(name) }
func humanAge(d time.Duration) string       { return healthpkg.HumanAge(d) }
func humanAgo(d time.Duration) string       { return healthpkg.HumanAgo(d) }

func (h *HealthRunner) addVerifyFailureCode(report *HealthReport, code string) {
	report.AddVerifyFailureCode(code)
}

func (h *HealthRunner) hasVerifyFailureCode(report *HealthReport, code string) bool {
	return report.HasVerifyFailureCode(code)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
