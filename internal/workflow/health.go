package workflow

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type HealthIssue struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type HealthCheck struct {
	Name    string `json:"name"`
	Result  string `json:"result"`
	Message string `json:"message"`
}

type HealthRevisionResult struct {
	Revision  int    `json:"revision"`
	CreatedAt string `json:"created_at,omitempty"`
	Result    string `json:"result"`
	Message   string `json:"message"`
}

type HealthReport struct {
	Status               string                 `json:"status"`
	CheckType            string                 `json:"check_type"`
	Label                string                 `json:"label"`
	Target               string                 `json:"target"`
	Mode                 string                 `json:"mode"`
	StorageType          string                 `json:"storage_type,omitempty"`
	Location             string                 `json:"location,omitempty"`
	CheckedAt            string                 `json:"checked_at"`
	LastSuccessAt        string                 `json:"last_success_at,omitempty"`
	LastDoctorRunAt      string                 `json:"last_doctor_run_at,omitempty"`
	LastVerifyRunAt      string                 `json:"last_verify_run_at,omitempty"`
	RevisionCount        int                    `json:"revision_count,omitempty"`
	LatestRevision       int                    `json:"latest_revision,omitempty"`
	LatestRevisionAt     string                 `json:"latest_revision_at,omitempty"`
	CheckedRevisionCount int                    `json:"checked_revision_count,omitempty"`
	PassedRevisionCount  int                    `json:"passed_revision_count,omitempty"`
	FailedRevisionCount  int                    `json:"failed_revision_count,omitempty"`
	FailedRevisions      []int                  `json:"failed_revisions,omitempty"`
	FailureCode          string                 `json:"failure_code,omitempty"`
	FailureCodes         []string               `json:"failure_codes,omitempty"`
	RecommendedActions   []string               `json:"recommended_action_codes,omitempty"`
	RevisionResults      []HealthRevisionResult `json:"revision_results,omitempty"`
	Issues               []HealthIssue          `json:"issues,omitempty"`
	Checks               []HealthCheck          `json:"checks,omitempty"`
	NotificationSent     bool                   `json:"notification_sent"`
	startedAt            time.Time              `json:"-"`
	completedAt          time.Time              `json:"-"`
}

type HealthRunner struct {
	meta   Metadata
	rt     Runtime
	log    *logger.Logger
	runner execpkg.Runner
}

const (
	verifyFailureNoRevisionsFound  = "no_revisions_found"
	verifyFailureIntegrityFailed   = "integrity_check_failed"
	verifyFailureResultMissing     = "integrity_result_missing"
	verifyFailureAccessFailed      = "verify_access_failed"
	verifyFailureListingFailed     = "verify_listing_failed"
	verifyActionRunBackup          = "run_backup"
	verifyActionCheckStorageAccess = "check_storage_access"
	verifyActionRecheckRepository  = "recheck_repository_state"
	verifyActionRerunVerify        = "rerun_verify"
)

func NewHealthRunner(meta Metadata, rt Runtime, log *logger.Logger, runner execpkg.Runner) *HealthRunner {
	return &HealthRunner{meta: meta, rt: rt, log: log, runner: runner}
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
	report := &HealthReport{
		Status:    "unhealthy",
		CheckType: checkType,
		Label:     label,
		Target:    target,
		Mode:      mode,
		CheckedAt: formatReportTime(checkedAt),
		Issues: []HealthIssue{
			{Severity: "error", Message: normaliseOperatorSentence(message)},
		},
		Checks: []HealthCheck{
			{Name: "Health", Result: "fail", Message: normaliseOperatorSentence(message)},
		},
	}
	return report
}

func WriteHealthReport(w io.Writer, report *HealthReport) error {
	if report == nil {
		return nil
	}
	payload := healthJSONReport(report)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(payload)
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
		startedAt: checkedAt,
	}

	restoreDebugCommands := h.suppressCommandDebug()
	defer restoreDebugCommands()

	cfg, plan, sec, err := h.prepare(req)
	if err != nil {
		h.printHeader(report)
		h.addCheck(report, "Environment", "fail", OperatorMessage(err))
		h.finalizeReport(report)
		h.maybeSendEarlyNotification(req, report)
		h.printReport(report)
		return report, healthExitCode(report.Status)
	}
	report.StorageType = plan.StorageType
	report.Location = plan.Location
	h.printHeader(report)

	state, stateErr := loadRunState(h.meta, req.Source, req.Target())
	if stateErr != nil {
		if os.IsNotExist(stateErr) {
			h.addCheck(report, "Backup state", "warn", fmt.Sprintf("No prior backup state found at %s", stateFilePath(h.meta, req.Source, req.Target())))
		} else {
			h.addCheck(report, "Backup state", "warn", fmt.Sprintf("Could not read backup state: %v", stateErr))
		}
	} else {
		h.addCheck(report, "Backup state", "pass", "Available")
		report.LastSuccessAt = chooseLocalSuccessTime(state)
	}

	lockStatus, lockErr := lock.InspectTarget(h.meta.LockParent, req.Source, req.Target())
	if lockErr != nil {
		h.addCheck(report, "Lock", "warn", OperatorMessage(lockErr))
	} else {
		switch {
		case lockStatus.Active:
			h.addCheck(report, "Lock", "warn", fmt.Sprintf("Active lock detected (PID %d)", lockStatus.PID))
		case lockStatus.Stale:
			h.addCheck(report, "Lock", "warn", "Stale lock detected")
		default:
			h.addCheck(report, "Lock", "pass", "No active lock")
		}
	}

	dup, dupErr := h.prepareDuplicacySetup(plan, sec)
	if dupErr != nil {
		h.addCheck(report, "Duplicacy setup", "fail", OperatorMessage(dupErr))
		h.finalizeReport(report)
		h.maybeSendEarlyNotification(req, report)
		h.printReport(report)
		return report, healthExitCode(report.Status)
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
		h.addCheck(report, "Health state", "warn", err.Error())
	}

	h.finalizeReport(report)
	if h.shouldSendNotification(req, cfg.Health, report.Status) {
		if payload := buildHealthNotificationPayload(h.rt, report); payload != nil {
			if err := notify.SendConfigured(cfg.Health.Notify, plan.SecretsFile, report.Target, payload); err != nil {
				h.addCheck(report, "Notification", "warn", OperatorMessage(err))
			} else {
				report.NotificationSent = true
				h.addCheck(report, "Notification", "pass", "Delivered")
			}
		}
	}
	h.finalizeReport(report)
	h.printReport(report)
	return report, healthExitCode(report.Status)
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
	h.addCheck(report, "Config file", "pass", plan.ConfigFile)
	defer func() {
		if plan.UsesObjectStorage() {
			h.addCheck(report, "Secrets", "pass", plan.SecretsFile)
		}
	}()

	stopInspecting := h.startStatusActivity("Checking stored revisions")
	revisions, _, err := dup.ListVisibleRevisions()
	stopInspecting()
	if err != nil {
		if req.HealthCommand == "verify" {
			h.addVerifyFailureCode(report, verifyFailureListingFailed)
		}
		h.addCheck(report, "Latest revision", "fail", OperatorMessage(err))
		return nil
	}
	report.RevisionCount = len(revisions)
	if len(revisions) == 0 {
		h.addCheck(report, "Revision count", "warn", "0")
		h.addCheck(report, "Latest revision", "warn", "No revisions were visible in storage")
		return nil
	}
	h.addCheck(report, "Revision count", "pass", fmt.Sprintf("%d", len(revisions)))
	latest := revisions[0]
	report.LatestRevision = latest.Revision
	if !latest.CreatedAt.IsZero() {
		report.LatestRevisionAt = formatReportTime(latest.CreatedAt)
		h.addCheck(report, "Latest revision", "pass", fmt.Sprintf("%d (%s)", latest.Revision, latest.CreatedAt.Format("2006-01-02 15:04:05")))
	} else {
		h.addCheck(report, "Latest revision", "pass", fmt.Sprintf("%d", latest.Revision))
	}

	if !latest.CreatedAt.IsZero() {
		h.evaluateFreshness(report, cfg.Health, latest.CreatedAt, "Backup freshness")
		return revisions
	}

	if state == nil {
		h.addCheck(report, "Backup freshness", "warn", "Latest revision time is unavailable and no backup state exists")
		return revisions
	}
	if state.LastSuccessfulBackupAt == "" {
		h.addCheck(report, "Backup freshness", "warn", "Latest revision time is unavailable and no backup timestamp is recorded")
		return revisions
	}

	if state.LastSuccessfulBackupRevision == latest.Revision {
		report.LatestRevisionAt = state.LastSuccessfulBackupAt
		if parsed, parseErr := time.Parse(time.RFC3339, state.LastSuccessfulBackupAt); parseErr == nil {
			h.evaluateFreshness(report, cfg.Health, parsed, "Backup freshness")
		} else {
			h.addCheck(report, "Backup freshness", "warn", "Recorded backup timestamp is invalid")
		}
		return revisions
	}

	h.addCheck(report, "Backup freshness", "warn", fmt.Sprintf("Latest revision %d does not match recorded backup revision %d", latest.Revision, state.LastSuccessfulBackupRevision))
	return revisions
}

func (h *HealthRunner) runDoctorChecks(report *HealthReport, req *Request, cfg *config.Config, plan *Plan, dup *duplicacy.Setup) {
	if _, err := os.Stat(plan.SnapshotSource); err != nil {
		h.addCheck(report, "Source path", "fail", fmt.Sprintf("Source path is not accessible: %v", err))
	} else {
		h.addCheck(report, "Source path", "pass", plan.SnapshotSource)
	}
	rootErr := btrfs.CheckVolume(h.runner, h.meta.RootVolume, false)
	sourceErr := btrfs.CheckVolume(h.runner, plan.SnapshotSource, false)
	switch {
	case rootErr == nil && sourceErr == nil:
		h.addCheck(report, "Btrfs", "pass", "Yes")
	default:
		if rootErr != nil {
			h.addCheck(report, "Btrfs root", "fail", OperatorMessage(rootErr))
		}
		if sourceErr != nil {
			h.addCheck(report, "Btrfs source", "fail", OperatorMessage(sourceErr))
		}
	}

	stopValidating := h.startStatusActivity("Validating repository access")
	if err := dup.ValidateRepo(); err != nil {
		stopValidating()
		if req.HealthCommand == "verify" {
			h.addVerifyFailureCode(report, verifyFailureAccessFailed)
		}
		h.addCheck(report, "Repository access", "fail", OperatorMessage(err))
	} else {
		stopValidating()
		h.addCheck(report, "Repository access", "pass", "Validated")
	}

	h.evaluateHealthRecency(report, cfg.Health, "doctor", "Last doctor run")
}

func (h *HealthRunner) runVerifyChecks(report *HealthReport, cfg *config.Config, dup *duplicacy.Setup, revisions []duplicacy.RevisionInfo) {
	if h.hasVerifyFailureCode(report, verifyFailureListingFailed) {
		report.RevisionCount = 0
		report.CheckedRevisionCount = 0
		report.PassedRevisionCount = 0
		report.FailedRevisionCount = 0
		h.addDisplayCheck(report, "Revisions checked", "fail", "0")
		h.addDisplayCheck(report, "Revisions passed", "pass", "0")
		h.addDisplayCheck(report, "Revisions failed", "pass", "0")
		h.addCheck(report, "Integrity check", "fail", "Revision inspection failed")
		h.populateHealthRecencyTimestamp(report, "verify")
		return
	}

	if len(revisions) == 0 {
		h.addVerifyFailureCode(report, verifyFailureNoRevisionsFound)
		h.addDisplayCheck(report, "Revisions checked", "fail", "0")
		h.addDisplayCheck(report, "Revisions passed", "pass", "0")
		h.addDisplayCheck(report, "Revisions failed", "pass", "0")
		h.addCheck(report, "Integrity check", "fail", "No revisions were found for this backup")
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}

	status := fmt.Sprintf("Checking revision integrity for this backup (%d total)", len(revisions))
	stopVerifying := h.startStatusActivity(status)
	results, _, err := dup.CheckVisibleRevisions()
	stopVerifying()
	if err != nil {
		h.addVerifyFailureCode(report, verifyFailureAccessFailed)
		report.CheckedRevisionCount = 0
		report.PassedRevisionCount = 0
		report.FailedRevisionCount = 0
		h.addDisplayCheck(report, "Revisions checked", "fail", "0")
		h.addDisplayCheck(report, "Revisions passed", "pass", "0")
		h.addDisplayCheck(report, "Revisions failed", "pass", "0")
		h.addCheck(report, "Integrity check", "fail", "Integrity check did not complete")
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
			h.addVerifyFailureCode(report, verifyFailureIntegrityFailed)
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
		h.addVerifyFailureCode(report, verifyFailureResultMissing)
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
	h.addDisplayCheck(report, "Revisions checked", verifiedResult, fmt.Sprintf("%d", report.CheckedRevisionCount))
	h.addDisplayCheck(report, "Revisions passed", "pass", fmt.Sprintf("%d", report.PassedRevisionCount))
	failedResult := "pass"
	if report.FailedRevisionCount > 0 {
		failedResult = "fail"
	}
	failedSummary := fmt.Sprintf("%d", report.FailedRevisionCount)
	if report.FailedRevisionCount > 0 {
		failedSummary = fmt.Sprintf("%d (%s)", report.FailedRevisionCount, summariseRevisionIDs(report.FailedRevisions, 4))
	}
	h.addDisplayCheck(report, "Revisions failed", failedResult, failedSummary)

	if len(missing) > 0 {
		h.addCheck(report, "Integrity check", "fail", integrityCheckFailureMessage(report.FailedRevisions, missing))
		for _, check := range detailChecks {
			h.addDisplayCheck(report, check.Name, check.Result, check.Message)
		}
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}
	if report.FailedRevisionCount > 0 {
		h.addCheck(report, "Integrity check", "fail", integrityCheckFailureMessage(report.FailedRevisions, nil))
		for _, check := range detailChecks {
			h.addDisplayCheck(report, check.Name, check.Result, check.Message)
		}
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}
	h.addCheck(report, "Integrity check", "pass", "All revisions validated")
	h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
}

func (h *HealthRunner) evaluateFreshness(report *HealthReport, cfg config.HealthConfig, last time.Time, checkName string) {
	age := h.rt.Now().Sub(last)
	warnAfter := time.Duration(cfg.FreshnessWarnHours) * time.Hour
	failAfter := time.Duration(cfg.FreshnessFailHours) * time.Hour
	message := fmt.Sprintf("%s old", humanAge(age))
	switch {
	case failAfter > 0 && age > failAfter:
		h.addCheck(report, checkName, "fail", message)
	case warnAfter > 0 && age > warnAfter:
		h.addCheck(report, checkName, "warn", message)
	default:
		h.addCheck(report, checkName, "pass", message)
	}
}

func (h *HealthRunner) evaluateHealthRecency(report *HealthReport, cfg config.HealthConfig, kind, name string) {
	_, at, ok := h.loadHealthRecencyTime(report, kind)
	if !ok {
		h.addCheck(report, name, "warn", "No prior health state is available")
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
		h.addCheck(report, name, "warn", humanAgo(age))
		return
	}
	h.addCheck(report, name, "pass", humanAgo(age))
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

func healthJSONReport(report *HealthReport) map[string]any {
	payload := map[string]any{
		"status":            report.Status,
		"check_type":        report.CheckType,
		"label":             report.Label,
		"target":            report.Target,
		"mode":              report.Mode,
		"checked_at":        report.CheckedAt,
		"notification_sent": report.NotificationSent,
	}
	if report.LastSuccessAt != "" {
		payload["last_success_at"] = report.LastSuccessAt
	}
	if report.LastDoctorRunAt != "" {
		payload["last_doctor_run_at"] = report.LastDoctorRunAt
	}
	if report.LastVerifyRunAt != "" {
		payload["last_verify_run_at"] = report.LastVerifyRunAt
	}
	if report.CheckType == "verify" || report.RevisionCount > 0 {
		payload["revision_count"] = report.RevisionCount
	}
	if report.LatestRevision > 0 {
		payload["latest_revision"] = report.LatestRevision
	}
	if report.LatestRevisionAt != "" {
		payload["latest_revision_at"] = report.LatestRevisionAt
	}
	if report.CheckType == "verify" || report.CheckedRevisionCount > 0 {
		payload["checked_revision_count"] = report.CheckedRevisionCount
	}
	if report.CheckType == "verify" || report.PassedRevisionCount > 0 {
		payload["passed_revision_count"] = report.PassedRevisionCount
	}
	if report.CheckType == "verify" {
		payload["failed_revision_count"] = report.FailedRevisionCount
		failed := report.FailedRevisions
		if failed == nil {
			failed = []int{}
		}
		payload["failed_revisions"] = failed
		if len(report.FailureCodes) > 0 {
			payload["failure_code"] = report.FailureCode
			payload["failure_codes"] = report.FailureCodes
			payload["recommended_action_codes"] = report.RecommendedActions
		}
	}
	if len(report.RevisionResults) > 0 {
		payload["revision_results"] = report.RevisionResults
	}
	if len(report.Issues) > 0 {
		payload["issues"] = report.Issues
	}
	return payload
}

func (h *HealthRunner) addVerifyFailureCode(report *HealthReport, code string) {
	if report == nil || report.CheckType != "verify" || code == "" {
		return
	}
	if report.FailureCode == "" {
		report.FailureCode = code
	}
	if !containsString(report.FailureCodes, code) {
		report.FailureCodes = append(report.FailureCodes, code)
	}
	for _, action := range verifyRecommendedActions(code) {
		if !containsString(report.RecommendedActions, action) {
			report.RecommendedActions = append(report.RecommendedActions, action)
		}
	}
}

func (h *HealthRunner) hasVerifyFailureCode(report *HealthReport, code string) bool {
	if report == nil || code == "" {
		return false
	}
	return containsString(report.FailureCodes, code)
}

func verifyRecommendedActions(code string) []string {
	switch code {
	case verifyFailureNoRevisionsFound:
		return []string{verifyActionRunBackup}
	case verifyFailureIntegrityFailed:
		return []string{verifyActionCheckStorageAccess, verifyActionRecheckRepository, verifyActionRerunVerify}
	case verifyFailureResultMissing:
		return []string{verifyActionCheckStorageAccess, verifyActionRerunVerify}
	case verifyFailureAccessFailed:
		return []string{verifyActionCheckStorageAccess, verifyActionRecheckRepository}
	case verifyFailureListingFailed:
		return []string{verifyActionCheckStorageAccess, verifyActionRecheckRepository}
	default:
		return nil
	}
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
		h.addCheck(report, "Notification", "warn", OperatorMessage(err))
		return
	}
	report.NotificationSent = true
	h.addCheck(report, "Notification", "pass", "Delivered")
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

func (h *HealthRunner) addCheck(report *HealthReport, name, result, message string) {
	report.Checks = append(report.Checks, HealthCheck{Name: name, Result: result, Message: normaliseOperatorSentence(message)})
	switch result {
	case "warn":
		report.Issues = append(report.Issues, HealthIssue{Severity: "warning", Message: normaliseOperatorSentence(message)})
	case "fail":
		report.Issues = append(report.Issues, HealthIssue{Severity: "error", Message: normaliseOperatorSentence(message)})
	}
}

func (h *HealthRunner) addDisplayCheck(report *HealthReport, name, result, message string) {
	report.Checks = append(report.Checks, HealthCheck{Name: name, Result: result, Message: normaliseOperatorSentence(message)})
}

func (h *HealthRunner) finalizeReport(report *HealthReport) {
	hasWarnings := false
	report.Status = "healthy"
	for _, issue := range report.Issues {
		if issue.Severity == "error" {
			report.Status = "unhealthy"
			break
		}
		if issue.Severity == "warning" {
			hasWarnings = true
		}
	}
	if report.Status != "unhealthy" && hasWarnings {
		report.Status = "degraded"
	}
}

func (h *HealthRunner) printHeader(report *HealthReport) {
	h.log.PrintSeparator()
	h.log.Info("%s", statusLinef("Health check started - %s", report.startedAt.Format("2006-01-02 15:04:05")))
	h.log.PrintLine("Check", strings.Title(report.CheckType))
	h.log.PrintLine("Label", report.Label)
	h.log.PrintLine("Target", report.Target)
	if report.StorageType != "" {
		h.log.PrintLine("Type", report.StorageType)
	}
	if report.Location != "" {
		h.log.PrintLine("Location", report.Location)
	}
}

func (h *HealthRunner) printReport(report *HealthReport) {
	report.completedAt = h.rt.Now()
	currentSection := ""
	for _, check := range report.Checks {
		section := healthCheckSection(check.Name)
		if section != currentSection {
			h.log.PrintSeparator()
			h.log.Info("%s", statusLinef("Section: %s", section))
			currentSection = section
		}
		h.printCheck(check)
	}
	h.log.PrintSeparator()
	h.log.Info("  %s : %s", h.log.FormatLabel("Result"), h.log.FormatResult(strings.Title(report.Status)))
	h.log.PrintLine("Code", fmt.Sprintf("%d", healthExitCode(report.Status)))
	h.log.PrintLine("Duration", formatClockDuration(report.completedAt.Sub(report.startedAt)))
	h.log.Info("%s", statusLinef("Health check completed - %s", report.completedAt.Format("2006-01-02 15:04:05")))
	h.log.PrintSeparator()
}

func (h *HealthRunner) printCheck(check HealthCheck) {
	label := healthCheckLabel(check.Name)
	switch check.Result {
	case "warn":
		h.log.Warn("  %s : %s", h.log.FormatLabel(label), check.Message)
	case "fail":
		h.log.Error("  %s : %s", h.log.FormatLabel(label), check.Message)
	default:
		h.log.PrintLine(label, check.Message)
	}
}

func (h *HealthRunner) startStatusActivity(status string) func() {
	if h.log.Interactive() {
		return h.log.StartActivity(status)
	}
	h.log.PrintLine("Status", status)
	return func() {}
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

func healthCheckSection(name string) string {
	switch name {
	case "Notification":
		return "Alerts"
	case "Source path", "Btrfs", "Btrfs root", "Btrfs source", "Repository access", "Last doctor run":
		return "Doctor"
	case "Revision count", "Latest revision", "Backup freshness":
		return "Status"
	case "Revisions checked", "Revisions passed", "Revisions failed", "Integrity check", "Last verify run":
		return "Verify"
	}
	if strings.HasPrefix(name, "Revision ") {
		return "Verify"
	}
	return "Status"
}

func healthExitCode(status string) int {
	switch status {
	case "healthy":
		return 0
	case "degraded":
		return 1
	default:
		return 2
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

func humanAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return "less than 1m"
	}
	d = d.Truncate(time.Minute)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 && len(parts) < 2 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 {
		return "less than 1m"
	}
	return strings.Join(parts, "")
}

func humanAgo(d time.Duration) string {
	age := humanAge(d)
	if age == "less than 1m" {
		return "<1m ago"
	}
	return age + " ago"
}

func healthCheckLabel(name string) string {
	switch name {
	default:
		return name
	}
}

func summariseRevisionIDs(revisions []int, limit int) string {
	if len(revisions) == 0 {
		return ""
	}
	if limit <= 0 {
		limit = len(revisions)
	}
	parts := make([]string, 0, minInt(len(revisions), limit)+1)
	for i, revision := range revisions {
		if i >= limit {
			break
		}
		parts = append(parts, fmt.Sprintf("%d", revision))
	}
	if extra := len(revisions) - limit; extra > 0 {
		parts = append(parts, fmt.Sprintf("+%d more", extra))
	}
	return strings.Join(parts, ", ")
}

func integrityCheckFailureMessage(failedRevisions, missingRevisions []int) string {
	switch {
	case len(failedRevisions) > 0 && len(missingRevisions) > 0:
		return fmt.Sprintf(
			"%d failed; %d returned no result",
			len(failedRevisions),
			len(missingRevisions),
		)
	case len(missingRevisions) > 0:
		return fmt.Sprintf(
			"%d revision(s) returned no integrity result: %s",
			len(missingRevisions),
			summariseRevisionIDs(missingRevisions, 4),
		)
	case len(failedRevisions) > 0:
		return fmt.Sprintf(
			"%d revision(s) failed integrity checks: %s",
			len(failedRevisions),
			summariseRevisionIDs(failedRevisions, 4),
		)
	default:
		return "Integrity validation did not succeed"
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func formatClockDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	seconds := int(duration.Truncate(time.Second) / time.Second)
	if duration > 0 && seconds == 0 {
		seconds = 1
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
