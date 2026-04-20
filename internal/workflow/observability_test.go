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

	plan := &Plan{BackupLabel: "homes", Target: "onsite-usb", OperationMode: "Backup", ModeDisplay: "onsite-usb", StorageType: storageTypeDuplicacy, Location: locationLocal, DryRun: true}
	report := NewRunReport(plan, start)
	if report.Label != "homes" || report.Operation != "Backup" || report.Mode != "onsite-usb" || report.StorageType != storageTypeDuplicacy || report.Location != locationLocal || !report.DryRun {
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
	for _, token := range []string{`"label": "homes"`, `"storage_type": "duplicacy"`, `"location": "local"`, `"name": "Backup"`, `"duration_seconds": 4`} {
		if !strings.Contains(buf.String(), token) {
			t.Fatalf("json output missing %q:\n%s", token, buf.String())
		}
	}

	req := &Request{Source: "homes", FixPerms: true, RequestedTarget: "onsite-usb"}
	failurePlan := &Plan{BackupLabel: "homes", Target: "onsite-usb", OperationMode: "Fix permissions", StorageType: storageTypeDuplicacy, Location: locationLocal}
	failure := NewFailureRunReport(req, failurePlan, start, end, 1, "boom")
	if failure.Operation != "Fix permissions" || failure.Result != "failed" || failure.DurationSecond != 3 || failure.StorageType != storageTypeDuplicacy || failure.Location != locationLocal {
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
	plan := &Plan{StorageType: storageTypeDuplicacy, Location: locationRemote, ModeDisplay: "offsite-storj", WorkRoot: "/tmp/work"}
	if !plan.UsesDuplicacyStorage() {
		t.Fatal("UsesDuplicacyStorage() = false, want true")
	}
	if !plan.IsRemoteLocation() {
		t.Fatal("IsRemoteLocation() = false, want true")
	}
	if plan.ModeLabel() != "offsite-storj" {
		t.Fatalf("ModeLabel() = %q", plan.ModeLabel())
	}
	if plan.WorkDir() != "/tmp/work/duplicacy" {
		t.Fatalf("WorkDir() = %q", plan.WorkDir())
	}
	plan.DuplicacyRoot = "/tmp/custom"
	if plan.WorkDir() != "/tmp/custom" {
		t.Fatalf("WorkDir() = %q", plan.WorkDir())
	}

}
