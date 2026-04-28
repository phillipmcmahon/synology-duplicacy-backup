package health

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
)

type Issue struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type Check struct {
	Name    string `json:"name"`
	Result  string `json:"result"`
	Message string `json:"message"`
}

type RevisionResult struct {
	Revision  int    `json:"revision"`
	CreatedAt string `json:"created_at,omitempty"`
	Result    string `json:"result"`
	Message   string `json:"message"`
}

type Report struct {
	Status               string           `json:"status"`
	CheckType            string           `json:"check_type"`
	Label                string           `json:"label"`
	Target               string           `json:"target"`
	Mode                 string           `json:"mode"`
	Location             string           `json:"location,omitempty"`
	CheckedAt            string           `json:"checked_at"`
	LastSuccessAt        string           `json:"last_success_at,omitempty"`
	LastDoctorRunAt      string           `json:"last_doctor_run_at,omitempty"`
	LastVerifyRunAt      string           `json:"last_verify_run_at,omitempty"`
	RevisionCount        int              `json:"revision_count,omitempty"`
	LatestRevision       int              `json:"latest_revision,omitempty"`
	LatestRevisionAt     string           `json:"latest_revision_at,omitempty"`
	CheckedRevisionCount int              `json:"checked_revision_count,omitempty"`
	PassedRevisionCount  int              `json:"passed_revision_count,omitempty"`
	FailedRevisionCount  int              `json:"failed_revision_count,omitempty"`
	FailedRevisions      []int            `json:"failed_revisions,omitempty"`
	FailureCode          string           `json:"failure_code,omitempty"`
	FailureCodes         []string         `json:"failure_codes,omitempty"`
	RecommendedActions   []string         `json:"recommended_action_codes,omitempty"`
	RevisionResults      []RevisionResult `json:"revision_results,omitempty"`
	Issues               []Issue          `json:"issues,omitempty"`
	Checks               []Check          `json:"checks,omitempty"`
	NotificationSent     bool             `json:"notification_sent"`
	StartedAt            time.Time        `json:"-"`
	CompletedAt          time.Time        `json:"-"`
}

const (
	VerifyFailureNoRevisionsFound  = "no_revisions_found"
	VerifyFailureIntegrityFailed   = "integrity_check_failed"
	VerifyFailureResultMissing     = "integrity_result_missing"
	VerifyFailureAccessFailed      = "verify_access_failed"
	VerifyFailureListingFailed     = "verify_listing_failed"
	verifyActionRunBackup          = "run_backup"
	verifyActionCheckStorageAccess = "check_storage_access"
	verifyActionRecheckRepository  = "recheck_repository_state"
	verifyActionRerunVerify        = "rerun_verify"
)

func NewFailureReport(checkType, label, target, mode, message string, checkedAt time.Time) *Report {
	return &Report{
		Status:    "unhealthy",
		CheckType: checkType,
		Label:     label,
		Target:    target,
		Mode:      mode,
		CheckedAt: formatReportTime(checkedAt),
		Issues: []Issue{
			{Severity: "error", Message: normaliseSentence(message)},
		},
		Checks: []Check{
			{Name: "Health", Result: "fail", Message: normaliseSentence(message)},
		},
	}
}

func WriteReport(w io.Writer, report *Report) error {
	if report == nil {
		return nil
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(jsonPayload(report))
}

func ExitCode(status string) int {
	switch status {
	case "healthy":
		return 0
	case "degraded":
		return 1
	default:
		return 2
	}
}

func (r *Report) AddCheck(name, result, message string) {
	if r == nil {
		return
	}
	message = normaliseSentence(message)
	r.Checks = append(r.Checks, Check{Name: name, Result: result, Message: message})
	switch result {
	case "warn":
		r.Issues = append(r.Issues, Issue{Severity: "warning", Message: message})
	case "fail":
		r.Issues = append(r.Issues, Issue{Severity: "error", Message: message})
	}
}

func (r *Report) AddDisplayCheck(name, result, message string) {
	if r == nil {
		return
	}
	r.Checks = append(r.Checks, Check{Name: name, Result: result, Message: normaliseSentence(message)})
}

func (r *Report) Finalize() {
	if r == nil {
		return
	}
	hasWarnings := false
	r.Status = "healthy"
	for _, issue := range r.Issues {
		if issue.Severity == "error" {
			r.Status = "unhealthy"
			return
		}
		if issue.Severity == "warning" {
			hasWarnings = true
		}
	}
	if hasWarnings {
		r.Status = "degraded"
	}
}

func (r *Report) AddVerifyFailureCode(code string) {
	if r == nil || r.CheckType != "verify" || code == "" {
		return
	}
	if r.FailureCode == "" {
		r.FailureCode = code
	}
	if !containsString(r.FailureCodes, code) {
		r.FailureCodes = append(r.FailureCodes, code)
	}
	for _, action := range verifyRecommendedActions(code) {
		if !containsString(r.RecommendedActions, action) {
			r.RecommendedActions = append(r.RecommendedActions, action)
		}
	}
}

func (r *Report) HasVerifyFailureCode(code string) bool {
	if r == nil || code == "" {
		return false
	}
	return containsString(r.FailureCodes, code)
}

func CheckResult(report *Report, name string) (result string, message string, ok bool) {
	if report == nil {
		return "", "", false
	}
	for _, check := range report.Checks {
		if check.Name == name {
			return check.Result, check.Message, true
		}
	}
	return "", "", false
}

func CheckMessage(report *Report, name string) string {
	_, message, _ := CheckResult(report, name)
	return message
}

func FirstIssueMessage(report *Report) string {
	if report == nil {
		return ""
	}
	for _, issue := range report.Issues {
		if strings.TrimSpace(issue.Message) != "" {
			return issue.Message
		}
	}
	return ""
}

func SectionForCheck(name string) string {
	switch name {
	case "Notification":
		return "Alerts"
	case "Source path", "Btrfs", "Btrfs root", "Btrfs source", "Last doctor run", "Root config profile":
		return "Doctor"
	case "Repository access":
		return "Repository"
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

func LabelForCheck(name string) string {
	return presentation.DisplayLabel(name)
}

func HumanAge(d time.Duration) string {
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

func HumanAgo(d time.Duration) string {
	age := HumanAge(d)
	if age == "less than 1m" {
		return "<1m ago"
	}
	return age + " ago"
}

func SummariseRevisionIDs(revisions []int, limit int) string {
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

func IntegrityCheckFailureMessage(failedRevisions, missingRevisions []int) string {
	switch {
	case len(failedRevisions) > 0 && len(missingRevisions) > 0:
		return fmt.Sprintf("%d failed; %d returned no result", len(failedRevisions), len(missingRevisions))
	case len(missingRevisions) > 0:
		return fmt.Sprintf("%d revision(s) returned no integrity result: %s", len(missingRevisions), SummariseRevisionIDs(missingRevisions, 4))
	case len(failedRevisions) > 0:
		return fmt.Sprintf("%d revision(s) failed integrity checks: %s", len(failedRevisions), SummariseRevisionIDs(failedRevisions, 4))
	default:
		return "Integrity validation did not succeed"
	}
}

func FormatClockDuration(duration time.Duration) string {
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

func jsonPayload(report *Report) map[string]any {
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

func verifyRecommendedActions(code string) []string {
	switch code {
	case VerifyFailureNoRevisionsFound:
		return []string{verifyActionRunBackup}
	case VerifyFailureIntegrityFailed:
		return []string{verifyActionCheckStorageAccess, verifyActionRecheckRepository, verifyActionRerunVerify}
	case VerifyFailureResultMissing:
		return []string{verifyActionCheckStorageAccess, verifyActionRerunVerify}
	case VerifyFailureAccessFailed:
		return []string{verifyActionCheckStorageAccess, verifyActionRecheckRepository}
	case VerifyFailureListingFailed:
		return []string{verifyActionCheckStorageAccess, verifyActionRecheckRepository}
	default:
		return nil
	}
}

func formatReportTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func normaliseSentence(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	return strings.ToUpper(message[:1]) + message[1:]
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
