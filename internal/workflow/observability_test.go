package workflow

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestObservabilityHelpers(t *testing.T) {
	start := time.Date(2026, 4, 10, 16, 47, 50, 900_000_000, time.UTC)
	end := time.Date(2026, 4, 10, 16, 47, 54, 100_000_000, time.UTC)

	plan := &Plan{BackupLabel: "homes", OperationMode: "Backup", ModeDisplay: "Local", DryRun: true}
	report := NewRunReport(plan, start)
	if report.Label != "homes" || report.Operation != "Backup" || report.Mode != "Local" || !report.DryRun {
		t.Fatalf("report = %+v", report)
	}
	report.ResetStart(start)
	if report.StartedAt != formatReportTime(start) {
		t.Fatalf("StartedAt = %q", report.StartedAt)
	}

	idx := report.StartPhase("Backup", start)
	report.CompletePhase(idx, "success", end)
	report.CompleteRun(0, "", end)

	var buf bytes.Buffer
	if err := WriteRunReport(&buf, report); err != nil {
		t.Fatalf("WriteRunReport() error = %v", err)
	}
	for _, token := range []string{`"label": "homes"`, `"name": "Backup"`, `"duration_seconds": 4`} {
		if !strings.Contains(buf.String(), token) {
			t.Fatalf("json output missing %q:\n%s", token, buf.String())
		}
	}

	req := &Request{Source: "homes", FixPerms: true}
	failure := NewFailureRunReport(req, start, end, 1, "boom")
	if failure.Operation != "Fix permissions" || failure.Result != "failed" || failure.DurationSecond != 3 {
		t.Fatalf("failure = %+v", failure)
	}

	if got := durationSeconds(-1 * time.Second); got != 0 {
		t.Fatalf("durationSeconds(-1s) = %v", got)
	}
	if got := durationSeconds(3900 * time.Millisecond); got != 3 {
		t.Fatalf("durationSeconds(3.9s) = %v", got)
	}
}

func TestPlanHelpersAndVersionText(t *testing.T) {
	plan := &Plan{RemoteMode: true, ModeDisplay: "Remote", WorkRoot: "/tmp/work"}
	if !plan.IsRemote() {
		t.Fatal("IsRemote() = false, want true")
	}
	if plan.ModeLabel() != "Remote" {
		t.Fatalf("ModeLabel() = %q", plan.ModeLabel())
	}
	if plan.WorkDir() != "/tmp/work/duplicacy" {
		t.Fatalf("WorkDir() = %q", plan.WorkDir())
	}
	plan.DuplicacyRoot = "/tmp/custom"
	if plan.WorkDir() != "/tmp/custom" {
		t.Fatalf("WorkDir() = %q", plan.WorkDir())
	}

	text := VersionText(DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir()))
	if !strings.Contains(text, "duplicacy-backup 2.1.3 (built now)") {
		t.Fatalf("VersionText() = %q", text)
	}
}
