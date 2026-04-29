package health

import (
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func newHealthStateTestRunner(t *testing.T, now time.Time) (*HealthRunner, Metadata) {
	t.Helper()
	meta := workflow.MetadataForLogDir("duplicacy-backup", "test", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	return &HealthRunner{meta: meta, rt: rt}, meta
}

func TestHealthRunnerLoadHealthRecencyTimeIgnoresMissingAndInvalidState(t *testing.T) {
	now := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	runner, meta := newHealthStateTestRunner(t, now)

	missingReport := &Report{Label: "homes", Target: "onsite-usb"}
	if _, _, ok := runner.loadHealthRecencyTime(missingReport, "verify"); ok {
		t.Fatal("loadHealthRecencyTime() ok = true for missing state")
	}
	if missingReport.LastVerifyRunAt != "" {
		t.Fatalf("LastVerifyRunAt = %q, want empty for missing state", missingReport.LastVerifyRunAt)
	}

	if err := saveRunState(meta, "homes", "onsite-usb", &RunState{LastVerifyAt: "not-a-time"}); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}
	invalidReport := &Report{Label: "homes", Target: "onsite-usb"}
	if _, _, ok := runner.loadHealthRecencyTime(invalidReport, "verify"); ok {
		t.Fatal("loadHealthRecencyTime() ok = true for invalid timestamp")
	}
	if invalidReport.LastVerifyRunAt != "" {
		t.Fatalf("LastVerifyRunAt = %q, want empty for invalid timestamp", invalidReport.LastVerifyRunAt)
	}
}

func TestHealthRunnerEvaluateHealthRecencyWarnsForMissingOrStaleState(t *testing.T) {
	now := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	runner, meta := newHealthStateTestRunner(t, now)
	cfg := config.HealthConfig{VerifyWarnAfter: 24}

	missingReport := &Report{Label: "homes", Target: "onsite-usb"}
	runner.evaluateHealthRecency(missingReport, cfg, "verify", "Last verify run")
	if len(missingReport.Checks) != 1 ||
		missingReport.Checks[0].Result != "warn" ||
		missingReport.Checks[0].Message != "No prior health state is available" ||
		missingReport.LastVerifyRunAt != "" {
		t.Fatalf("missing state report = %+v", missingReport)
	}

	staleAt := now.Add(-25 * time.Hour)
	if err := saveRunState(meta, "homes", "onsite-usb", &RunState{LastVerifyAt: formatReportTime(staleAt)}); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}
	staleReport := &Report{Label: "homes", Target: "onsite-usb"}
	runner.evaluateHealthRecency(staleReport, cfg, "verify", "Last verify run")
	if len(staleReport.Checks) != 1 ||
		staleReport.Checks[0].Result != "warn" ||
		staleReport.Checks[0].Message != "1d1h ago" ||
		staleReport.LastVerifyRunAt != "2026-04-16T08:00:00Z" {
		t.Fatalf("stale state report = %+v", staleReport)
	}
}

func TestHealthRunnerEvaluateHealthRecencyPassesForFreshState(t *testing.T) {
	now := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	runner, meta := newHealthStateTestRunner(t, now)
	if err := saveRunState(meta, "homes", "onsite-usb", &RunState{LastDoctorAt: formatReportTime(now.Add(-90 * time.Minute))}); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	report := &Report{Label: "homes", Target: "onsite-usb"}
	runner.evaluateHealthRecency(report, config.HealthConfig{DoctorWarnAfter: 24}, "doctor", "Last doctor run")

	if len(report.Checks) != 1 ||
		report.Checks[0].Result != "pass" ||
		report.Checks[0].Message != "1h30m ago" ||
		report.LastDoctorRunAt != "2026-04-17T07:30:00Z" {
		t.Fatalf("fresh state report = %+v", report)
	}
}
