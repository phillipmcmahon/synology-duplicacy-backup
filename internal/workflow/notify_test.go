package workflow

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

func TestBuildRuntimeWebhookPayload_BackupCouldNotStart(t *testing.T) {
	rt := testRuntime()
	plan := &Plan{DoBackup: true}
	report := &RunReport{
		Label:       "homes",
		Target:      "offsite-storj",
		StorageType: storageTypeObject,
		Location:    locationRemote,
		Operation:   "Backup",
		ExitCode:    1,
	}

	payload := buildRuntimeWebhookPayload(rt, plan, report, errors.New("boom"), false, nil)
	if payload == nil {
		t.Fatal("buildRuntimeWebhookPayload() = nil")
	}
	if payload.Event != "backup_could_not_start" || payload.Severity != "critical" || payload.Operation != "backup" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Label != "homes" || payload.Target != "offsite-storj" || payload.StorageType != storageTypeObject || payload.Location != locationRemote {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Timestamp == "" || payload.EventID == "" || payload.Host == "" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestBuildRuntimeWebhookPayload_BackupFailed(t *testing.T) {
	rt := testRuntime()
	plan := &Plan{DoBackup: true}
	report := &RunReport{
		Label:          "homes",
		Target:         "onsite-usb",
		StorageType:    storageTypeFilesystem,
		Location:       locationLocal,
		ExitCode:       1,
		DurationSecond: 38,
		Phases: []PhaseReport{
			{Name: "Backup", Result: "failed"},
		},
	}

	payload := buildRuntimeWebhookPayload(rt, plan, report, errors.New("boom"), true, nil)
	if payload == nil {
		t.Fatal("buildRuntimeWebhookPayload() = nil")
	}
	if payload.Event != "backup_failed" || payload.Category != "backup" || payload.Status != "failed" {
		t.Fatalf("payload = %+v", payload)
	}
	if got := payload.Details["duration_seconds"]; got != 38 {
		t.Fatalf("duration_seconds = %#v", got)
	}
}

func TestBuildRuntimeWebhookPayload_SafePruneBlocked(t *testing.T) {
	rt := testRuntime()
	plan := &Plan{
		DoPrune:                   true,
		SafePruneMaxDeletePercent: 10,
		SafePruneMaxDeleteCount:   25,
	}
	report := &RunReport{
		Label:       "homes",
		Target:      "onsite-usb",
		StorageType: storageTypeFilesystem,
		Location:    locationLocal,
		ExitCode:    1,
		Phases: []PhaseReport{
			{Name: "Prune", Result: "failed"},
		},
	}
	preview := &duplicacy.PrunePreview{
		DeleteCount:     42,
		TotalRevisions:  100,
		DeletePercent:   42,
		PercentEnforced: true,
	}

	payload := buildRuntimeWebhookPayload(rt, plan, report, NewMessageError("Refusing to continue because safe prune thresholds were exceeded"), true, preview)
	if payload == nil {
		t.Fatal("buildRuntimeWebhookPayload() = nil")
	}
	if payload.Event != "safe_prune_blocked" || payload.Severity != "warning" || payload.Status != "blocked" {
		t.Fatalf("payload = %+v", payload)
	}
	if got := payload.Details["preview_deletes"]; got != 42 {
		t.Fatalf("preview_deletes = %#v", got)
	}
	if got := payload.Details["delete_percent"]; got != 42 {
		t.Fatalf("delete_percent = %#v", got)
	}
}

func TestBuildRuntimeWebhookPayload_PruneFailed(t *testing.T) {
	rt := testRuntime()
	plan := &Plan{DoPrune: true}
	report := &RunReport{
		Label:       "homes",
		Target:      "offsite-storj",
		StorageType: storageTypeObject,
		Location:    locationRemote,
		ExitCode:    1,
		Phases: []PhaseReport{
			{Name: "Prune", Result: "failed"},
		},
	}

	payload := buildRuntimeWebhookPayload(rt, plan, report, errors.New("boom"), true, nil)
	if payload == nil {
		t.Fatal("buildRuntimeWebhookPayload() = nil")
	}
	if payload.Event != "prune_failed" || payload.Severity != "warning" || payload.Operation != "prune" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestBuildHealthWebhookPayload_FreshnessFailed(t *testing.T) {
	rt := testRuntime()
	report := &HealthReport{
		Status:      "unhealthy",
		CheckType:   "status",
		Label:       "homes",
		Target:      "offsite-storj",
		StorageType: storageTypeObject,
		Location:    locationRemote,
		Issues: []HealthIssue{
			{Severity: "error", Message: "72h old"},
		},
		Checks: []HealthCheck{
			{Name: "Backup freshness", Result: "fail", Message: "72h old"},
		},
	}

	payload := buildHealthWebhookPayload(rt, report)
	if payload == nil {
		t.Fatal("buildHealthWebhookPayload() = nil")
	}
	if payload.Event != "freshness_failed" || payload.Check != "status" || payload.Severity != "critical" {
		t.Fatalf("payload = %+v", payload)
	}
	if got := payload.Details["freshness"]; got != "72h old" {
		t.Fatalf("freshness = %#v", got)
	}
}

func TestBuildHealthWebhookPayload_VerifyFailedRevisions(t *testing.T) {
	rt := testRuntime()
	report := &HealthReport{
		Status:              "unhealthy",
		CheckType:           "verify",
		Label:               "homes",
		Target:              "onsite-usb",
		StorageType:         storageTypeFilesystem,
		Location:            locationLocal,
		FailedRevisionCount: 2,
		FailedRevisions:     []int{41, 39},
		Checks: []HealthCheck{
			{Name: "Integrity check", Result: "fail", Message: "Integrity verification found failed revisions"},
		},
	}

	payload := buildHealthWebhookPayload(rt, report)
	if payload == nil {
		t.Fatal("buildHealthWebhookPayload() = nil")
	}
	if payload.Event != "verify_failed_revisions" || payload.Check != "verify" {
		t.Fatalf("payload = %+v", payload)
	}
	if got := payload.Details["failed_revision_count"]; got != 2 {
		t.Fatalf("failed_revision_count = %#v", got)
	}
}

func TestBuildHealthWebhookPayload_Degraded(t *testing.T) {
	rt := testRuntime()
	report := &HealthReport{
		Status:      "degraded",
		CheckType:   "doctor",
		Label:       "homes",
		Target:      "onsite-usb",
		StorageType: storageTypeFilesystem,
		Location:    locationLocal,
		Issues: []HealthIssue{
			{Severity: "warning", Message: "Last doctor run is overdue"},
		},
	}

	payload := buildHealthWebhookPayload(rt, report)
	if payload == nil {
		t.Fatal("buildHealthWebhookPayload() = nil")
	}
	if payload.Event != "health_degraded" || payload.Severity != "warning" || payload.Check != "doctor" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestExecutorMaybeSendFailureWebhook_BackupFailed(t *testing.T) {
	rt := testRuntime()
	log := testExecutorLogger(t)
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		body = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := &Executor{
		rt:                rt,
		log:               log,
		plan:              &Plan{Notify: configHealthNotifyForTest(server.URL)},
		report:            &RunReport{Label: "homes", Target: "onsite-usb", StorageType: storageTypeFilesystem, Location: locationLocal, ExitCode: 1, Phases: []PhaseReport{{Name: "Backup", Result: "failed"}}},
		lastErr:           errors.New("boom"),
		visibleRunStarted: true,
	}
	executor.maybeSendFailureWebhook()

	if !strings.Contains(body, `"event":"backup_failed"`) || !strings.Contains(body, `"operation":"backup"`) {
		t.Fatalf("body = %q", body)
	}
}

func TestExecutorMaybeSendFailureWebhook_SafePruneBlocked(t *testing.T) {
	rt := testRuntime()
	log := testExecutorLogger(t)
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		body = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := &Executor{
		rt:   rt,
		log:  log,
		plan: &Plan{Notify: configHealthNotifyForTest(server.URL), SafePruneMaxDeletePercent: 10, SafePruneMaxDeleteCount: 25},
		report: &RunReport{
			Label:       "homes",
			Target:      "offsite-storj",
			StorageType: storageTypeObject,
			Location:    locationRemote,
			ExitCode:    1,
			Phases:      []PhaseReport{{Name: "Prune", Result: "failed"}},
		},
		lastErr:           NewMessageError("Refusing to continue because safe prune thresholds were exceeded"),
		lastPrunePreview:  &duplicacy.PrunePreview{DeleteCount: 42, TotalRevisions: 100, DeletePercent: 42, PercentEnforced: true},
		visibleRunStarted: true,
	}
	executor.maybeSendFailureWebhook()

	if !strings.Contains(body, `"event":"safe_prune_blocked"`) || !strings.Contains(body, `"status":"blocked"`) {
		t.Fatalf("body = %q", body)
	}
}

func configHealthNotifyForTest(url string) config.HealthNotifyConfig {
	return config.HealthNotifyConfig{
		WebhookURL:  url,
		SendFor:     []string{"backup", "prune", "cleanup-storage", "status", "doctor", "verify"},
		NotifyOn:    []string{"degraded", "unhealthy"},
		Interactive: false,
	}
}
