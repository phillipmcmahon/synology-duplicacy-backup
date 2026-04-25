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

	plan := &Plan{BackupLabel: "homes", Target: "onsite-usb", OperationMode: "Backup", ModeDisplay: "onsite-usb", Location: locationLocal, DryRun: true}
	report := NewRunReport(plan, start)
	if report.Label != "homes" || report.Operation != "Backup" || report.Mode != "onsite-usb" || report.Location != locationLocal || !report.DryRun {
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
	for _, token := range []string{`"label": "homes"`, `"location": "local"`, `"name": "Backup"`, `"duration_seconds": 4`} {
		if !strings.Contains(buf.String(), token) {
			t.Fatalf("json output missing %q:\n%s", token, buf.String())
		}
	}
	if strings.Contains(buf.String(), `"storage_type"`) {
		t.Fatalf("json output should not include storage_type:\n%s", buf.String())
	}

	req := &RuntimeRequest{Label: "homes", Mode: RuntimeModeFixPerms, TargetName: "onsite-usb"}
	failurePlan := &Plan{BackupLabel: "homes", Target: "onsite-usb", OperationMode: "Fix permissions", Location: locationLocal}
	failure := NewFailureRunReport(req, failurePlan, start, end, 1, "boom")
	if failure.Operation != "Fix permissions" || failure.Result != "failed" || failure.DurationSecond != 3 || failure.Location != locationLocal {
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
	plan := &Plan{Location: locationRemote, ModeDisplay: "offsite-storj", WorkRoot: "/tmp/work"}
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

func TestPlanSectionsExposeFocusedViews(t *testing.T) {
	plan := &Plan{
		DoBackup:      true,
		OperationMode: "Backup",
		BackupLabel:   "homes",
		Target:        "onsite-usb",
		Location:      locationLocal,
		BackupTarget:  "/backups/homes",
		WorkRoot:      "/tmp/work",
		ModeDisplay:   "onsite-usb",
	}
	sections := plan.Sections()

	if !sections.Request.DoBackup || sections.Request.OperationMode != "Backup" {
		t.Fatalf("request section = %+v", sections.Request)
	}
	if sections.Config.Target != "onsite-usb" || sections.Config.Location != locationLocal {
		t.Fatalf("config section = %+v", sections.Config)
	}
	if sections.Paths.BackupTarget != "/backups/homes" || sections.Paths.WorkRoot != "/tmp/work" {
		t.Fatalf("paths section = %+v", sections.Paths)
	}
	if sections.Display.ModeDisplay != "onsite-usb" {
		t.Fatalf("display section = %+v", sections.Display)
	}
}
