package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
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

type HealthReport struct {
	Status                  string        `json:"status"`
	CheckType               string        `json:"check_type"`
	Label                   string        `json:"label"`
	Mode                    string        `json:"mode"`
	CheckedAt               string        `json:"checked_at"`
	LocalLastSuccessAt      string        `json:"local_last_success_at,omitempty"`
	StorageLatestRevision   int           `json:"storage_latest_revision,omitempty"`
	StorageLatestRevisionAt string        `json:"storage_latest_revision_at,omitempty"`
	Issues                  []HealthIssue `json:"issues,omitempty"`
	Checks                  []HealthCheck `json:"checks,omitempty"`
	WebhookSent             bool          `json:"webhook_sent"`
	startedAt               time.Time     `json:"-"`
	completedAt             time.Time     `json:"-"`
}

type HealthRunner struct {
	meta   Metadata
	rt     Runtime
	log    *logger.Logger
	runner execpkg.Runner
}

func NewHealthRunner(meta Metadata, rt Runtime, log *logger.Logger, runner execpkg.Runner) *HealthRunner {
	return &HealthRunner{meta: meta, rt: rt, log: log, runner: runner}
}

func NewFailureHealthReport(req *Request, checkType, message string, checkedAt time.Time) *HealthReport {
	mode := "Local"
	label := ""
	if req != nil {
		label = req.Source
		if req.RemoteMode {
			mode = "Remote"
		}
		if checkType == "" {
			checkType = req.HealthCommand
		}
	}
	report := &HealthReport{
		Status:    "unhealthy",
		CheckType: checkType,
		Label:     label,
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
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func (h *HealthRunner) Run(req *Request) (*HealthReport, int) {
	checkedAt := h.rt.Now()
	report := &HealthReport{
		Status:    "healthy",
		CheckType: req.HealthCommand,
		Label:     req.Source,
		Mode:      modeDisplay(req.RemoteMode),
		CheckedAt: formatReportTime(checkedAt),
		startedAt: checkedAt,
	}

	h.printHeader(report)

	cfg, plan, sec, err := h.prepare(req)
	if err != nil {
		h.addCheck(report, "Environment", "fail", OperatorMessage(err))
		h.finalizeReport(report)
		h.maybeSendEarlyWebhook(req, report)
		h.printReport(report)
		return report, healthExitCode(report.Status)
	}

	state, stateErr := loadRunState(h.meta, req.Source)
	if stateErr != nil {
		if os.IsNotExist(stateErr) {
			h.addCheck(report, "Local state", "warn", fmt.Sprintf("No prior local state found at %s", stateFilePath(h.meta, req.Source)))
		} else {
			h.addCheck(report, "Local state", "warn", fmt.Sprintf("Could not read local state: %v", stateErr))
		}
	} else {
		h.addCheck(report, "Local state", "pass", "Read successfully")
		report.LocalLastSuccessAt = chooseLocalSuccessTime(state)
	}

	lockStatus, lockErr := lock.Inspect(h.meta.LockParent, req.Source)
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
		h.maybeSendEarlyWebhook(req, report)
		h.printReport(report)
		return report, healthExitCode(report.Status)
	}
	defer dup.Cleanup()

	h.runStatusChecks(report, req, cfg, plan, state, dup)
	if req.HealthCommand == "doctor" || req.HealthCommand == "verify" {
		h.runDoctorChecks(report, req, cfg, plan, dup)
	}
	if req.HealthCommand == "verify" {
		h.runVerifyChecks(report, cfg, state, dup)
	}

	if err := updateHealthCheckState(h.meta, req.Source, req.HealthCommand, checkedAt); err != nil {
		h.addCheck(report, "Health state", "warn", err.Error())
	}

	h.finalizeReport(report)
	if h.shouldSendWebhook(req, cfg.Health, report.Status) {
		if err := h.sendWebhook(cfg.Health.Notify, plan.SecretsFile, report); err != nil {
			h.addCheck(report, "Webhook", "warn", err.Error())
		} else {
			report.WebhookSent = true
			h.addCheck(report, "Webhook", "pass", "Delivered")
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

	cfgReq := configValidationRequest(req, req.RemoteMode)
	cfgPlan := planner.derivePlan(cfgReq)
	cfg, err := planner.loadConfig(cfgPlan)
	if err != nil {
		return nil, nil, nil, err
	}

	plan.BackupTarget = JoinDestination(cfg.Destination, plan.BackupLabel)
	plan.ModeDisplay = modeDisplay(req.RemoteMode)
	plan.OperationMode = "Health " + strings.Title(req.HealthCommand)
	plan.LocalOwner = cfg.LocalOwner
	plan.LocalGroup = cfg.LocalGroup
	plan.LogRetentionDays = cfg.LogRetentionDays
	plan.Filter = cfg.Filter
	plan.FilterLines = splitNonEmptyLines(cfg.Filter)
	plan.PruneOptions = cfg.Prune
	plan.Threads = cfg.Threads

	var sec *secrets.Secrets
	if req.RemoteMode {
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

func (h *HealthRunner) runStatusChecks(report *HealthReport, req *Request, cfg *config.Config, plan *Plan, state *RunState, dup *duplicacy.Setup) {
	h.addCheck(report, "Config", "pass", fmt.Sprintf("Loaded %s", plan.ConfigFile))
	if req.RemoteMode {
		h.addCheck(report, "Remote secrets", "pass", fmt.Sprintf("Validated %s", plan.SecretsFile))
	}

	stopInspecting := h.startStatusActivity("Inspecting visible storage revisions")
	latest, _, err := dup.GetLatestRevisionInfo()
	stopInspecting()
	if err != nil {
		h.addCheck(report, "Storage revisions", "fail", OperatorMessage(err))
		return
	}
	if latest == nil || latest.Revision == 0 {
		h.addCheck(report, "Storage revisions", "warn", "No revisions were visible in storage")
		return
	}
	report.StorageLatestRevision = latest.Revision
	if !latest.CreatedAt.IsZero() {
		report.StorageLatestRevisionAt = formatReportTime(latest.CreatedAt)
		h.addCheck(report, "Storage revisions", "pass", fmt.Sprintf("Latest revision visible: %d (%s)", latest.Revision, latest.CreatedAt.Format("2006-01-02 15:04:05")))
	} else {
		h.addCheck(report, "Storage revisions", "pass", fmt.Sprintf("Latest revision visible: %d", latest.Revision))
	}

	if !latest.CreatedAt.IsZero() {
		h.evaluateFreshness(report, cfg.Health, latest.CreatedAt, "Storage freshness")
		return
	}

	if state == nil {
		h.addCheck(report, "Storage freshness", "warn", "Latest storage revision does not include a parsable creation time and no local backup state is available")
		return
	}
	if state.LastSuccessfulBackupAt == "" {
		h.addCheck(report, "Storage freshness", "warn", "Latest storage revision does not include a parsable creation time and no successful backup timestamp is recorded locally")
		return
	}

	if state.LastSuccessfulBackupRevision == latest.Revision {
		report.StorageLatestRevisionAt = state.LastSuccessfulBackupAt
		if parsed, parseErr := time.Parse(time.RFC3339, state.LastSuccessfulBackupAt); parseErr == nil {
			h.evaluateFreshness(report, cfg.Health, parsed, "Storage freshness")
		} else {
			h.addCheck(report, "Storage freshness", "warn", "Stored local backup timestamp is invalid")
		}
		return
	}

	h.addCheck(report, "Storage freshness", "warn", fmt.Sprintf("Latest storage revision is %d but local state last recorded successful backup revision %d", latest.Revision, state.LastSuccessfulBackupRevision))
}

func (h *HealthRunner) runDoctorChecks(report *HealthReport, req *Request, cfg *config.Config, plan *Plan, dup *duplicacy.Setup) {
	if !req.RemoteMode {
		if _, err := os.Stat(plan.SnapshotSource); err != nil {
			h.addCheck(report, "Source path", "fail", fmt.Sprintf("Source path is not accessible: %v", err))
		} else {
			h.addCheck(report, "Source path", "pass", plan.SnapshotSource)
		}
		if err := btrfs.CheckVolume(h.runner, h.meta.RootVolume, false); err != nil {
			h.addCheck(report, "Btrfs root", "fail", OperatorMessage(err))
		} else {
			h.addCheck(report, "Btrfs root", "pass", h.meta.RootVolume)
		}
		if err := btrfs.CheckVolume(h.runner, plan.SnapshotSource, false); err != nil {
			h.addCheck(report, "Btrfs source", "fail", OperatorMessage(err))
		} else {
			h.addCheck(report, "Btrfs source", "pass", plan.SnapshotSource)
		}
	}

	stopValidating := h.startStatusActivity("Validating repository access")
	if err := dup.ValidateRepo(); err != nil {
		stopValidating()
		h.addCheck(report, "Repository access", "fail", OperatorMessage(err))
	} else {
		stopValidating()
		h.addCheck(report, "Repository access", "pass", "Repository validated")
	}

	h.evaluateHealthRecency(report, cfg.Health, "doctor", "Last doctor run")
}

func (h *HealthRunner) runVerifyChecks(report *HealthReport, cfg *config.Config, state *RunState, dup *duplicacy.Setup) {
	stopVerifying := h.startStatusActivity("Verifying visible storage revisions")
	latest, _, err := dup.GetLatestRevisionInfo()
	stopVerifying()
	if err != nil {
		h.addCheck(report, "Verify revisions", "fail", OperatorMessage(err))
		return
	}
	if latest == nil || latest.Revision == 0 {
		h.addCheck(report, "Verify revisions", "fail", "No revisions are available to verify")
		return
	}
	if !latest.CreatedAt.IsZero() {
		h.addCheck(report, "Verify revisions", "pass", fmt.Sprintf("Confirmed latest revision %d is visible and dated %s", latest.Revision, latest.CreatedAt.Format("2006-01-02 15:04:05")))
	} else {
		h.addCheck(report, "Verify revisions", "pass", fmt.Sprintf("Confirmed latest revision %d is visible", latest.Revision))
	}

	if latest.CreatedAt.IsZero() {
		switch {
		case state == nil || state.LastSuccessfulBackupRevision == 0:
			h.addCheck(report, "Verify metadata", "warn", "No recorded successful backup revision is available for timestamp fallback")
		case state.LastSuccessfulBackupRevision != latest.Revision:
			h.addCheck(report, "Verify metadata", "warn", fmt.Sprintf("Latest storage revision %d does not match local recorded revision %d", latest.Revision, state.LastSuccessfulBackupRevision))
		case state.LastSuccessfulBackupAt == "":
			h.addCheck(report, "Verify metadata", "warn", "No local successful backup timestamp is available for revision fallback")
		default:
			if _, parseErr := time.Parse(time.RFC3339, state.LastSuccessfulBackupAt); parseErr == nil {
				h.addCheck(report, "Verify metadata", "pass", "Used local state as the timestamp fallback for the latest revision")
			} else {
				h.addCheck(report, "Verify metadata", "warn", "Stored local backup timestamp is invalid")
			}
		}
	} else {
		h.addCheck(report, "Verify metadata", "pass", "Revision timestamp is available for freshness checks")
	}
	h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
}

func (h *HealthRunner) evaluateFreshness(report *HealthReport, cfg config.HealthConfig, last time.Time, checkName string) {
	age := h.rt.Now().Sub(last)
	warnAfter := time.Duration(cfg.FreshnessWarnHours) * time.Hour
	failAfter := time.Duration(cfg.FreshnessFailHours) * time.Hour
	message := fmt.Sprintf("Latest known backup is %s old", humanDuration(age))
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
	state, err := loadRunState(h.meta, report.Label)
	if err != nil || state == nil {
		h.addCheck(report, name, "warn", "No prior health state is available")
		return
	}

	var last string
	var thresholdHours int
	switch kind {
	case "doctor":
		last = state.LastDoctorAt
		thresholdHours = cfg.DoctorWarnAfter
	case "verify":
		last = state.LastVerifyAt
		thresholdHours = cfg.VerifyWarnAfter
	default:
		return
	}
	if last == "" {
		h.addCheck(report, name, "warn", "This health check has not been recorded before")
		return
	}
	at, parseErr := time.Parse(time.RFC3339, last)
	if parseErr != nil {
		h.addCheck(report, name, "warn", "Prior health check timestamp is invalid")
		return
	}
	age := h.rt.Now().Sub(at)
	if thresholdHours > 0 && age > time.Duration(thresholdHours)*time.Hour {
		h.addCheck(report, name, "warn", fmt.Sprintf("Last %s run was %s ago", kind, humanDuration(age)))
		return
	}
	h.addCheck(report, name, "pass", fmt.Sprintf("Last %s run was %s ago", kind, humanDuration(age)))
}

func (h *HealthRunner) shouldSendWebhook(req *Request, cfg config.HealthConfig, status string) bool {
	if req == nil {
		return false
	}
	if cfg.Notify.WebhookURL == "" {
		return false
	}
	if h.log.Interactive() && h.rt.StdinIsTTY() && !cfg.Notify.Interactive {
		return false
	}
	if !containsString(cfg.Notify.NotifyOn, status) {
		return false
	}
	return containsString(cfg.Notify.SendFor, req.HealthCommand)
}

func (h *HealthRunner) maybeSendEarlyWebhook(req *Request, report *HealthReport) {
	if req == nil || report == nil {
		return
	}
	cfg, secretsFile, ok := h.loadHealthNotifyConfig(req)
	if !ok || !h.shouldSendWebhook(req, cfg, report.Status) {
		return
	}
	if err := h.sendWebhook(cfg.Notify, secretsFile, report); err != nil {
		h.addCheck(report, "Webhook", "warn", err.Error())
		return
	}
	report.WebhookSent = true
	h.addCheck(report, "Webhook", "pass", "Delivered")
}

func (h *HealthRunner) loadHealthNotifyConfig(req *Request) (config.HealthConfig, string, bool) {
	if req == nil || req.Source == "" {
		return config.HealthConfig{}, "", false
	}

	configDir := ResolveDir(h.rt, req.ConfigDir, "DUPLICACY_BACKUP_CONFIG_DIR", ExecutableConfigDir(h.rt))
	configFile := filepath.Join(configDir, fmt.Sprintf("%s-backup.toml", req.Source))
	raw, err := config.ParseFile(configFile)
	if err != nil {
		return config.HealthConfig{}, "", false
	}

	cfg := raw.ResolveHealth()
	if err := cfg.Validate(); err != nil {
		return config.HealthConfig{}, "", false
	}

	secretsDir := ResolveDir(h.rt, req.SecretsDir, "DUPLICACY_BACKUP_SECRETS_DIR", config.DefaultSecretsDir)
	secretsFile := secrets.GetSecretsFilePath(secretsDir, config.DefaultSecretsPrefix, req.Source)
	return cfg, secretsFile, true
}

func (h *HealthRunner) sendWebhook(cfg config.HealthNotifyConfig, secretsFile string, report *HealthReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to encode webhook payload: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token, err := secrets.LoadOptionalHealthWebhookToken(secretsFile); err != nil {
		return err
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook delivery failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook delivery returned %s", resp.Status)
	}
	return nil
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
	h.log.PrintLine("Mode", report.Mode)
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

func healthCheckSection(name string) string {
	switch name {
	case "Webhook":
		return "Alerts"
	case "Source path", "Btrfs root", "Btrfs source", "Repository access", "Last doctor run":
		return "Doctor"
	case "Verify revisions", "Verify metadata", "Last verify run":
		return "Verify"
	default:
		return "Status"
	}
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

func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Truncate(time.Minute)
	if d < time.Minute {
		return "less than a minute"
	}
	return d.String()
}

func healthCheckLabel(name string) string {
	switch name {
	default:
		return name
	}
}

func formatClockDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	seconds := int(duration.Truncate(time.Second) / time.Second)
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
