package health

import (
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func testNotifyRuntime() Env {
	rt := workflow.DefaultEnv()
	rt.Now = func() time.Time { return time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC) }
	rt.Getpid = func() int { return 4242 }
	rt.StdinIsTTY = func() bool { return false }
	return rt
}

func TestBuildHealthNotificationPayload_FreshnessFailed(t *testing.T) {
	rt := testNotifyRuntime()
	report := &Report{
		Status:    "unhealthy",
		CheckType: "status",
		Label:     "homes",
		Target:    "offsite-storj",
		Location:  "remote",
		Issues: []Issue{
			{Severity: "error", Message: "72h old"},
		},
		Checks: []Check{
			{Name: "Backup freshness", Result: "fail", Message: "72h old"},
		},
	}

	payload := buildHealthNotificationPayload(rt, report)
	if payload == nil {
		t.Fatal("buildHealthNotificationPayload() = nil")
	}
	if payload.Event != "freshness_failed" || payload.Check != "status" || payload.Severity != "critical" {
		t.Fatalf("payload = %+v", payload)
	}
	if got := payload.Details["freshness"]; got != "72h old" {
		t.Fatalf("freshness = %#v", got)
	}
}

func TestBuildHealthNotificationPayload_VerifyFailedRevisions(t *testing.T) {
	rt := testNotifyRuntime()
	report := &Report{
		Status:              "unhealthy",
		CheckType:           "verify",
		Label:               "homes",
		Target:              "onsite-usb",
		Location:            "local",
		FailedRevisionCount: 2,
		FailedRevisions:     []int{41, 39},
		FailureCode:         verifyFailureIntegrityFailed,
		FailureCodes:        []string{verifyFailureIntegrityFailed, verifyFailureResultMissing},
		RecommendedActions:  []string{"check_storage_access", "rerun_verify"},
		Checks: []Check{
			{Name: "Integrity check", Result: "fail", Message: "Integrity verification found failed revisions"},
		},
	}

	payload := buildHealthNotificationPayload(rt, report)
	if payload == nil {
		t.Fatal("buildHealthNotificationPayload() = nil")
	}
	if payload.Event != "verify_failed_revisions" || payload.Check != "verify" {
		t.Fatalf("payload = %+v", payload)
	}
	if got := payload.Details["failed_revision_count"]; got != 2 {
		t.Fatalf("failed_revision_count = %#v", got)
	}
	if got := payload.Details["failure_code"]; got != verifyFailureIntegrityFailed {
		t.Fatalf("failure_code = %#v", got)
	}
	if got, ok := payload.Details["failure_codes"].([]string); !ok || len(got) != 2 || got[0] != verifyFailureIntegrityFailed || got[1] != verifyFailureResultMissing {
		t.Fatalf("failure_codes = %#v", payload.Details["failure_codes"])
	}
	if got, ok := payload.Details["recommended_action_codes"].([]string); !ok || len(got) != 2 || got[0] != "check_storage_access" || got[1] != "rerun_verify" {
		t.Fatalf("recommended_action_codes = %#v", payload.Details["recommended_action_codes"])
	}
}

func TestBuildHealthNotificationPayload_VerifyMetadataWithoutFailedRevisions(t *testing.T) {
	rt := testNotifyRuntime()
	report := &Report{
		Status:             "unhealthy",
		CheckType:          "verify",
		Label:              "homes",
		Target:             "offsite-storj",
		Location:           "remote",
		FailureCode:        verifyFailureNoRevisionsFound,
		FailureCodes:       []string{verifyFailureNoRevisionsFound},
		RecommendedActions: []string{verifyActionRunBackup},
		Issues: []Issue{
			{Severity: "error", Message: "No revisions were found"},
		},
	}

	payload := buildHealthNotificationPayload(rt, report)
	if payload == nil {
		t.Fatal("buildHealthNotificationPayload() = nil")
	}
	if payload.Event != "health_unhealthy" || payload.Check != "verify" || payload.Severity != "critical" {
		t.Fatalf("payload = %+v", payload)
	}
	if got := payload.Details["failure_code"]; got != verifyFailureNoRevisionsFound {
		t.Fatalf("failure_code = %#v", got)
	}
	if got, ok := payload.Details["failure_codes"].([]string); !ok || len(got) != 1 || got[0] != verifyFailureNoRevisionsFound {
		t.Fatalf("failure_codes = %#v", payload.Details["failure_codes"])
	}
	if got, ok := payload.Details["recommended_action_codes"].([]string); !ok || len(got) != 1 || got[0] != verifyActionRunBackup {
		t.Fatalf("recommended_action_codes = %#v", payload.Details["recommended_action_codes"])
	}
}

func TestBuildHealthNotificationPayload_Degraded(t *testing.T) {
	rt := testNotifyRuntime()
	report := &Report{
		Status:    "degraded",
		CheckType: "doctor",
		Label:     "homes",
		Target:    "onsite-usb",
		Location:  "local",
		Issues: []Issue{
			{Severity: "warning", Message: "Last doctor run is overdue"},
		},
	}

	payload := buildHealthNotificationPayload(rt, report)
	if payload == nil {
		t.Fatal("buildHealthNotificationPayload() = nil")
	}
	if payload.Event != "health_degraded" || payload.Severity != "warning" || payload.Check != "doctor" {
		t.Fatalf("payload = %+v", payload)
	}
}
