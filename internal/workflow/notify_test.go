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

func TestBuildRuntimeNotificationPayload_BackupCouldNotStart(t *testing.T) {
	rt := testRuntime()
	plan := &Plan{Request: PlanRequest{DoBackup: true}}
	report := &RunReport{
		Label:     "homes",
		Target:    "offsite-storj",
		Location:  locationRemote,
		Operation: "Backup",
		ExitCode:  1,
	}

	payload := buildRuntimeNotificationPayload(rt, plan, report, errors.New("boom"), false, nil)
	if payload == nil {
		t.Fatal("buildRuntimeNotificationPayload() = nil")
	}
	if payload.Event != "backup_could_not_start" || payload.Severity != "critical" || payload.Operation != "backup" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Label != "homes" || payload.Target != "offsite-storj" || payload.Location != locationRemote {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Timestamp == "" || payload.EventID == "" || payload.Host == "" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestBuildRuntimeNotificationPayload_LocalDuplicacyBackupCouldNotStart(t *testing.T) {
	rt := testRuntime()
	plan := &Plan{Request: PlanRequest{DoBackup: true}}
	report := &RunReport{
		Label:     "homes",
		Target:    "onsite-rustfs",
		Location:  locationLocal,
		Operation: "Backup",
		ExitCode:  1,
	}

	payload := buildRuntimeNotificationPayload(rt, plan, report, errors.New("boom"), false, nil)
	if payload == nil {
		t.Fatal("buildRuntimeNotificationPayload() = nil")
	}
	if payload.Event != "backup_could_not_start" || payload.Severity != "critical" || payload.Operation != "backup" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Label != "homes" || payload.Target != "onsite-rustfs" || payload.Location != locationLocal {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestBuildRuntimeNotificationPayload_BackupFailed(t *testing.T) {
	rt := testRuntime()
	plan := &Plan{Request: PlanRequest{DoBackup: true}}
	report := &RunReport{
		Label:          "homes",
		Target:         "onsite-usb",
		Location:       locationLocal,
		ExitCode:       1,
		DurationSecond: 38,
		Phases: []PhaseReport{
			{Name: "Backup", Result: "failed"},
		},
	}

	payload := buildRuntimeNotificationPayload(rt, plan, report, errors.New("boom"), true, nil)
	if payload == nil {
		t.Fatal("buildRuntimeNotificationPayload() = nil")
	}
	if payload.Event != "backup_failed" || payload.Category != "backup" || payload.Status != "failed" {
		t.Fatalf("payload = %+v", payload)
	}
	if got := payload.Details["duration_seconds"]; got != 38 {
		t.Fatalf("duration_seconds = %#v", got)
	}
}

func TestBuildRuntimeNotificationPayload_SafePruneBlocked(t *testing.T) {
	rt := testRuntime()
	plan := &Plan{
		Request: PlanRequest{DoPrune: true},
		Config: PlanConfig{
			SafePruneMaxDeletePercent: 10,
			SafePruneMaxDeleteCount:   25,
		},
	}
	report := &RunReport{
		Label:    "homes",
		Target:   "onsite-usb",
		Location: locationLocal,
		ExitCode: 1,
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

	payload := buildRuntimeNotificationPayload(rt, plan, report, NewMessageError("Refusing to continue because safe prune thresholds were exceeded"), true, preview)
	if payload == nil {
		t.Fatal("buildRuntimeNotificationPayload() = nil")
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

func TestBuildRuntimeNotificationPayload_PruneFailed(t *testing.T) {
	rt := testRuntime()
	plan := &Plan{Request: PlanRequest{DoPrune: true}}
	report := &RunReport{
		Label:    "homes",
		Target:   "offsite-storj",
		Location: locationRemote,
		ExitCode: 1,
		Phases: []PhaseReport{
			{Name: "Prune", Result: "failed"},
		},
	}

	payload := buildRuntimeNotificationPayload(rt, plan, report, errors.New("boom"), true, nil)
	if payload == nil {
		t.Fatal("buildRuntimeNotificationPayload() = nil")
	}
	if payload.Event != "prune_failed" || payload.Severity != "warning" || payload.Operation != "prune" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestExecutorMaybeSendFailureNotification_BackupFailed(t *testing.T) {
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
		plan:              &Plan{Config: PlanConfig{Notify: configHealthNotifyForTest(server.URL)}},
		report:            &RunReport{Label: "homes", Target: "onsite-usb", Location: locationLocal, ExitCode: 1, Phases: []PhaseReport{{Name: "Backup", Result: "failed"}}},
		lastErr:           errors.New("boom"),
		visibleRunStarted: true,
	}
	executor.maybeSendFailureNotification()

	if !strings.Contains(body, `"event":"backup_failed"`) || !strings.Contains(body, `"operation":"backup"`) {
		t.Fatalf("body = %q", body)
	}
}

func TestExecutorMaybeSendFailureNotification_SafePruneBlocked(t *testing.T) {
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
		rt:  rt,
		log: log,
		plan: &Plan{Config: PlanConfig{
			Notify:                    configHealthNotifyForTest(server.URL),
			SafePruneMaxDeletePercent: 10,
			SafePruneMaxDeleteCount:   25,
		}},
		report: &RunReport{
			Label:    "homes",
			Target:   "offsite-storj",
			Location: locationRemote,
			ExitCode: 1,
			Phases:   []PhaseReport{{Name: "Prune", Result: "failed"}},
		},
		lastErr:           NewMessageError("Refusing to continue because safe prune thresholds were exceeded"),
		lastPrunePreview:  &duplicacy.PrunePreview{DeleteCount: 42, TotalRevisions: 100, DeletePercent: 42, PercentEnforced: true},
		visibleRunStarted: true,
	}
	executor.maybeSendFailureNotification()

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
