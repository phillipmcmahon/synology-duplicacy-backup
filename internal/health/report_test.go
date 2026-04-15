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
	report.Finalize()
	if report.Status != "degraded" {
		t.Fatalf("report.Status = %q, want degraded", report.Status)
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

func TestPresenterPrintsStructuredHealthReport(t *testing.T) {
	log, logDir := newHealthTestLogger(t)
	presenter := NewPresenter(log, func() time.Time {
		return time.Date(2026, 4, 15, 10, 0, 4, 0, time.UTC)
	})
	report := &Report{
		Status:      "degraded",
		CheckType:   "verify",
		Label:       "homes",
		Target:      "offsite-storj",
		StorageType: "object",
		Location:    "remote",
		StartedAt:   time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
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
