package health

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func newHealthTestLogger(t *testing.T) (*logger.Logger, string) {
	t.Helper()
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	return log, logDir
}

func readSingleLogFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 log file, found %d", len(entries))
	}
	path := filepath.Join(dir, entries[0].Name())
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	return string(data)
}

func TestNewFailureReportAndWriteReport(t *testing.T) {
	checkedAt := time.Date(2026, 4, 15, 9, 30, 0, 0, time.UTC)
	report := NewFailureReport("doctor", "homes", "offsite-storj", "offsite-storj", "could not read state", checkedAt)

	if report.Status != "unhealthy" || report.CheckType != "doctor" {
		t.Fatalf("report = %+v", report)
	}
	if len(report.Issues) != 1 || report.Issues[0].Message != "Could not read state" {
		t.Fatalf("report issues = %+v", report.Issues)
	}

	var buf bytes.Buffer
	if err := WriteReport(&buf, report); err != nil {
		t.Fatalf("WriteReport() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["status"] != "unhealthy" || payload["check_type"] != "doctor" {
		t.Fatalf("payload = %#v", payload)
	}
	if _, ok := payload["summary"]; ok {
		t.Fatalf("payload should not include summary: %#v", payload)
	}
}

func TestReportFinalizeAndVerifyFailureCodes(t *testing.T) {
	report := &Report{CheckType: "verify"}
	report.AddCheck("Backup freshness", "warn", "older than expected")
	report.AddDisplayCheck("Repository access", "pass", "reachable")
	report.Finalize()
	if report.Status != "degraded" {
		t.Fatalf("report.Status = %q, want degraded", report.Status)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("AddDisplayCheck should not add an issue: %+v", report.Issues)
	}
	if result, message, ok := CheckResult(report, "Repository access"); !ok || result != "pass" || message != "Reachable" {
		t.Fatalf("CheckResult() = %q, %q, %t", result, message, ok)
	}
	if got := CheckMessage(report, "Backup freshness"); got != "Older than expected" {
		t.Fatalf("CheckMessage() = %q", got)
	}
	if got := FirstIssueMessage(report); got != "Older than expected" {
		t.Fatalf("FirstIssueMessage() = %q", got)
	}

	report.AddVerifyFailureCode(VerifyFailureNoRevisionsFound)
	report.AddVerifyFailureCode(VerifyFailureNoRevisionsFound)
	report.AddVerifyFailureCode(VerifyFailureAccessFailed)

	if report.FailureCode != VerifyFailureNoRevisionsFound {
		t.Fatalf("FailureCode = %q", report.FailureCode)
	}
	if len(report.FailureCodes) != 2 {
		t.Fatalf("FailureCodes = %#v", report.FailureCodes)
	}
	if !report.HasVerifyFailureCode(VerifyFailureAccessFailed) {
		t.Fatalf("report should contain %q", VerifyFailureAccessFailed)
	}
	if got := strings.Join(report.RecommendedActions, ","); !strings.Contains(got, "run_backup") || !strings.Contains(got, "check_storage_access") {
		t.Fatalf("RecommendedActions = %#v", report.RecommendedActions)
	}

	report.AddCheck("Integrity check", "fail", "integrity failed")
	report.Finalize()
	if report.Status != "unhealthy" {
		t.Fatalf("report.Status = %q, want unhealthy", report.Status)
	}
}

func TestHealthFormattingHelpers(t *testing.T) {
	ageCases := []struct {
		duration time.Duration
		age      string
		ago      string
	}{
		{duration: -time.Second, age: "less than 1m", ago: "<1m ago"},
		{duration: 30 * time.Second, age: "less than 1m", ago: "<1m ago"},
		{duration: 90 * time.Minute, age: "1h30m", ago: "1h30m ago"},
		{duration: 49*time.Hour + 45*time.Minute, age: "2d1h", ago: "2d1h ago"},
	}
	for _, tt := range ageCases {
		if got := HumanAge(tt.duration); got != tt.age {
			t.Fatalf("HumanAge(%s) = %q, want %q", tt.duration, got, tt.age)
		}
		if got := HumanAgo(tt.duration); got != tt.ago {
			t.Fatalf("HumanAgo(%s) = %q, want %q", tt.duration, got, tt.ago)
		}
	}

	if got := SummariseRevisionIDs([]int{10, 11, 12, 13, 14}, 3); got != "10, 11, 12, +2 more" {
		t.Fatalf("SummariseRevisionIDs() = %q", got)
	}
	if got := SummariseRevisionIDs(nil, 3); got != "" {
		t.Fatalf("SummariseRevisionIDs(nil) = %q", got)
	}

	messageCases := []struct {
		failed  []int
		missing []int
		want    string
	}{
		{failed: []int{1, 2}, missing: []int{3}, want: "2 failed; 1 returned no result"},
		{missing: []int{3, 4, 5, 6, 7}, want: "5 revision(s) returned no integrity result: 3, 4, 5, 6, +1 more"},
		{failed: []int{8}, want: "1 revision(s) failed integrity checks: 8"},
		{want: "Integrity validation did not succeed"},
	}
	for _, tt := range messageCases {
		if got := IntegrityCheckFailureMessage(tt.failed, tt.missing); got != tt.want {
			t.Fatalf("IntegrityCheckFailureMessage(%v, %v) = %q, want %q", tt.failed, tt.missing, got, tt.want)
		}
	}

	if got := SectionForCheck("Revision 42"); got != "Verify" {
		t.Fatalf("SectionForCheck(Revision 42) = %q", got)
	}
	for _, tc := range []struct {
		name string
		want string
	}{
		{"Notification", "Alerts"},
		{"Source path", "Doctor"},
		{"Btrfs root", "Doctor"},
		{"Repository access", "Doctor"},
		{"Revision count", "Status"},
		{"Latest revision", "Status"},
		{"Backup freshness", "Status"},
		{"Revisions checked", "Verify"},
		{"Integrity check", "Verify"},
		{"Last verify run", "Verify"},
		{"Unknown check", "Status"},
	} {
		if got := SectionForCheck(tc.name); got != tc.want {
			t.Fatalf("SectionForCheck(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := LabelForCheck("Repository access"); got != "Repository Access" {
		t.Fatalf("LabelForCheck(Repository access) = %q, want Repository Access", got)
	}
	if got := LabelForCheck("Any check"); got != "Any check" {
		t.Fatalf("LabelForCheck should preserve unknown check names, got %q", got)
	}
	if got := ExitCode("healthy"); got != 0 {
		t.Fatalf("ExitCode(healthy) = %d", got)
	}
	if got := ExitCode("degraded"); got != 1 {
		t.Fatalf("ExitCode(degraded) = %d", got)
	}
	if got := ExitCode("mystery"); got != 2 {
		t.Fatalf("ExitCode(mystery) = %d", got)
	}
	for _, tc := range []struct {
		duration time.Duration
		want     string
	}{
		{-time.Second, "00:00:00"},
		{500 * time.Millisecond, "00:00:01"},
		{time.Hour + 2*time.Minute + 3*time.Second, "01:02:03"},
	} {
		if got := FormatClockDuration(tc.duration); got != tc.want {
			t.Fatalf("FormatClockDuration(%s) = %q, want %q", tc.duration, got, tc.want)
		}
	}
}

func TestReportNilAndMissingHelpers(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, nil); err != nil {
		t.Fatalf("WriteReport(nil) error = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("WriteReport(nil) wrote %q", buf.String())
	}
	(*Report)(nil).AddCheck("Check", "fail", "message")
	(*Report)(nil).AddDisplayCheck("Check", "pass", "message")
	(*Report)(nil).Finalize()
	(*Report)(nil).AddVerifyFailureCode(VerifyFailureAccessFailed)
	if (*Report)(nil).HasVerifyFailureCode(VerifyFailureAccessFailed) {
		t.Fatal("nil reports should not have failure codes")
	}
	if result, message, ok := CheckResult(nil, "missing"); result != "" || message != "" || ok {
		t.Fatalf("CheckResult(nil) = %q, %q, %t", result, message, ok)
	}
	if got := CheckMessage(nil, "missing"); got != "" {
		t.Fatalf("CheckMessage(nil) = %q", got)
	}
	if got := FirstIssueMessage(nil); got != "" {
		t.Fatalf("FirstIssueMessage(nil) = %q", got)
	}

	report := &Report{CheckType: "status", Issues: []Issue{{Severity: "warning"}, {Severity: "warning", Message: " useful message "}}}
	report.AddVerifyFailureCode(VerifyFailureAccessFailed)
	if report.HasVerifyFailureCode("") {
		t.Fatal("empty failure code should not match")
	}
	if got := FirstIssueMessage(report); got != " useful message " {
		t.Fatalf("FirstIssueMessage() = %q", got)
	}
	if result, message, ok := CheckResult(report, "missing"); result != "" || message != "" || ok {
		t.Fatalf("missing CheckResult() = %q, %q, %t", result, message, ok)
	}
	report.Finalize()
	if report.Status != "degraded" {
		t.Fatalf("Finalize() status = %q", report.Status)
	}
}

func TestVerifyRecommendedActions(t *testing.T) {
	cases := map[string][]string{
		VerifyFailureNoRevisionsFound: {verifyActionRunBackup},
		VerifyFailureIntegrityFailed:  {verifyActionCheckStorageAccess, verifyActionRecheckRepository, verifyActionRerunVerify},
		VerifyFailureResultMissing:    {verifyActionCheckStorageAccess, verifyActionRerunVerify},
		VerifyFailureAccessFailed:     {verifyActionCheckStorageAccess, verifyActionRecheckRepository},
		VerifyFailureListingFailed:    {verifyActionCheckStorageAccess, verifyActionRecheckRepository},
		"unknown":                     nil,
	}
	for code, want := range cases {
		got := verifyRecommendedActions(code)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("verifyRecommendedActions(%q) = %#v, want %#v", code, got, want)
		}
	}
}

func TestWriteReportVerifyKeepsStableFailureFields(t *testing.T) {
	report := &Report{
		Status:              "unhealthy",
		CheckType:           "verify",
		Label:               "homes",
		Target:              "onsite-usb",
		Mode:                "onsite-usb",
		CheckedAt:           "2026-04-15T09:30:00Z",
		FailedRevisionCount: 0,
		FailedRevisions:     nil,
	}

	var buf bytes.Buffer
	if err := WriteReport(&buf, report); err != nil {
		t.Fatalf("WriteReport() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["failed_revision_count"] != float64(0) {
		t.Fatalf("payload failed_revision_count = %#v", payload["failed_revision_count"])
	}
	failed, ok := payload["failed_revisions"].([]any)
	if !ok || len(failed) != 0 {
		t.Fatalf("payload failed_revisions = %#v", payload["failed_revisions"])
	}
}

func TestWriteReportIncludesOptionalFields(t *testing.T) {
	report := &Report{
		Status:               "unhealthy",
		CheckType:            "verify",
		Label:                "homes",
		Target:               "onsite-usb",
		Mode:                 "onsite-usb",
		Location:             "local",
		CheckedAt:            "2026-04-15T09:30:00Z",
		LastSuccessAt:        "2026-04-15T08:30:00Z",
		LastDoctorRunAt:      "2026-04-15T07:30:00Z",
		LastVerifyRunAt:      "2026-04-15T06:30:00Z",
		RevisionCount:        4,
		LatestRevision:       4,
		LatestRevisionAt:     "2026-04-15T05:30:00Z",
		CheckedRevisionCount: 2,
		PassedRevisionCount:  1,
		FailedRevisionCount:  1,
		FailedRevisions:      []int{3},
		RevisionResults:      []RevisionResult{{Revision: 3, CreatedAt: "2026-04-15T05:30:00Z", Result: "fail", Message: "Missing chunks"}},
		Issues:               []Issue{{Severity: "error", Message: "Missing chunks"}},
		Checks:               []Check{{Name: "Integrity check", Result: "fail", Message: "Missing chunks"}},
		NotificationSent:     true,
	}
	report.AddVerifyFailureCode(VerifyFailureIntegrityFailed)

	var buf bytes.Buffer
	if err := WriteReport(&buf, report); err != nil {
		t.Fatalf("WriteReport() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, key := range []string{
		"last_success_at",
		"last_doctor_run_at",
		"last_verify_run_at",
		"revision_count",
		"latest_revision",
		"latest_revision_at",
		"checked_revision_count",
		"passed_revision_count",
		"failed_revision_count",
		"failed_revisions",
		"failure_code",
		"failure_codes",
		"recommended_action_codes",
		"revision_results",
		"issues",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload missing %q: %#v", key, payload)
		}
	}
	if payload["notification_sent"] != true {
		t.Fatalf("notification_sent = %#v", payload["notification_sent"])
	}
}

func TestPresenterPrintsStructuredHealthReport(t *testing.T) {
	log, logDir := newHealthTestLogger(t)
	presenter := NewPresenter(log, func() time.Time {
		return time.Date(2026, 4, 15, 10, 0, 4, 0, time.UTC)
	})
	report := &Report{
		Status:    "degraded",
		CheckType: "verify",
		Label:     "homes",
		Target:    "offsite-storj",
		Location:  "remote",
		StartedAt: time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
		Checks: []Check{
			{Name: "Revision count", Result: "pass", Message: "5"},
			{Name: "Notification", Result: "warn", Message: "Delivery delayed"},
		},
	}

	presenter.PrintHeader(report)
	presenter.PrintReport(report)
	log.Close()

	output := readSingleLogFile(t, logDir)
	for _, token := range []string{
		"Health check started - 2026-04-15 10:00:00",
		"Check", "Verify",
		"Target", "offsite-storj",
		"Section: Status",
		"Section: Alerts",
		"Result", "Degraded",
		"Duration", "00:00:04",
	} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}
}
