package workflow

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaybeSendUpdateFailureNotification(t *testing.T) {
	configDir := t.TempDir()
	var gotTitle, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		gotBody = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	writeUpdateNotifyConfig(t, configDir, strings.Join([]string{
		`[update.notify]`,
		`notify_on = ["failed"]`,
		`[update.notify.ntfy]`,
		`url = "` + server.URL + `"`,
		`topic = "duplicacy-updates"`,
	}, "\n"))

	rt := testRuntime()
	rt.StdinIsTTY = func() bool { return false }
	req := &Request{UpdateCommand: "update", ConfigDir: configDir, UpdateYes: true}
	meta := DefaultMetadata("duplicacy-backup", "4.2.2", "now", t.TempDir())

	if err := MaybeSendUpdateFailureNotification(req, meta, rt, UpdateStatusFailed, errors.New("update install failed: exit status 1")); err != nil {
		t.Fatalf("MaybeSendUpdateFailureNotification() error = %v", err)
	}
	if gotTitle != "WARNING: Duplicacy Backup update install failed" {
		t.Fatalf("Title = %q", gotTitle)
	}
	if !strings.Contains(gotBody, "Event: update_install_failed") ||
		!strings.Contains(gotBody, "Operation: update") ||
		strings.Contains(gotBody, "Label:") ||
		strings.Contains(gotBody, "Target:") {
		t.Fatalf("Body = %q", gotBody)
	}
}

func TestMaybeSendUpdateFailureNotificationMissingConfigNoops(t *testing.T) {
	rt := testRuntime()
	rt.StdinIsTTY = func() bool { return false }
	req := &Request{UpdateCommand: "update", ConfigDir: t.TempDir(), UpdateYes: true}
	meta := DefaultMetadata("duplicacy-backup", "4.2.2", "now", t.TempDir())

	if err := MaybeSendUpdateFailureNotification(req, meta, rt, UpdateStatusFailed, errors.New("release lookup failed")); err != nil {
		t.Fatalf("MaybeSendUpdateFailureNotification() error = %v", err)
	}
}

func TestMaybeSendUpdateFailureNotificationHonoursInteractiveGate(t *testing.T) {
	configDir := t.TempDir()
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	writeUpdateNotifyConfig(t, configDir, strings.Join([]string{
		`[update.notify.ntfy]`,
		`url = "` + server.URL + `"`,
		`topic = "duplicacy-updates"`,
	}, "\n"))

	rt := testRuntime()
	rt.StdinIsTTY = func() bool { return true }
	req := &Request{UpdateCommand: "update", ConfigDir: configDir}
	meta := DefaultMetadata("duplicacy-backup", "4.2.2", "now", t.TempDir())

	if err := MaybeSendUpdateFailureNotification(req, meta, rt, UpdateStatusFailed, errors.New("release lookup failed")); err != nil {
		t.Fatalf("MaybeSendUpdateFailureNotification() error = %v", err)
	}
	if called {
		t.Fatal("expected interactive update notification to be suppressed by default")
	}
}

func TestMaybeSendUpdateSuccessNotificationOptIn(t *testing.T) {
	configDir := t.TempDir()
	var gotTitle string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	writeUpdateNotifyConfig(t, configDir, strings.Join([]string{
		`[update.notify]`,
		`notify_on = ["succeeded"]`,
		`[update.notify.ntfy]`,
		`url = "` + server.URL + `"`,
		`topic = "duplicacy-updates"`,
	}, "\n"))

	rt := testRuntime()
	rt.StdinIsTTY = func() bool { return false }
	req := &Request{UpdateCommand: "update", ConfigDir: configDir, UpdateYes: true}
	meta := DefaultMetadata("duplicacy-backup", "4.2.2", "now", t.TempDir())

	if err := MaybeSendUpdateSuccessNotification(req, meta, rt, UpdateStatusInstalled); err != nil {
		t.Fatalf("MaybeSendUpdateSuccessNotification() error = %v", err)
	}
	if gotTitle != "INFO: Duplicacy Backup update installed" {
		t.Fatalf("Title = %q", gotTitle)
	}
}

func TestMaybeSendUpdateSuccessNotificationUsesStructuredStatus(t *testing.T) {
	configDir := t.TempDir()
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	writeUpdateNotifyConfig(t, configDir, strings.Join([]string{
		`[update.notify]`,
		`notify_on = ["succeeded"]`,
		`[update.notify.ntfy]`,
		`url = "` + server.URL + `"`,
		`topic = "duplicacy-updates"`,
	}, "\n"))

	rt := testRuntime()
	rt.StdinIsTTY = func() bool { return false }
	req := &Request{UpdateCommand: "update", ConfigDir: configDir, UpdateYes: true}
	meta := DefaultMetadata("duplicacy-backup", "4.2.2", "now", t.TempDir())

	if err := MaybeSendUpdateSuccessNotification(req, meta, rt, UpdateStatusAvailable); err != nil {
		t.Fatalf("MaybeSendUpdateSuccessNotification(available) error = %v", err)
	}
	if called {
		t.Fatal("available status should preserve existing no-notification behaviour")
	}

	if err := MaybeSendUpdateSuccessNotification(req, meta, rt, UpdateStatusInstalled); err != nil {
		t.Fatalf("MaybeSendUpdateSuccessNotification(installed) error = %v", err)
	}
	if !called {
		t.Fatal("installed status should send notification without reading rendered report text")
	}
}

func TestUpdateSuccessStatusEventMappingContract(t *testing.T) {
	tests := []struct {
		name        string
		status      UpdateStatus
		wantStatus  string
		wantEvent   string
		wantSummary string
	}{
		{
			name:        "installed",
			status:      UpdateStatusInstalled,
			wantStatus:  "succeeded",
			wantEvent:   "update_install_succeeded",
			wantSummary: "Duplicacy Backup update installed",
		},
		{
			name:        "current",
			status:      UpdateStatusCurrent,
			wantStatus:  "current",
			wantEvent:   "update_already_current",
			wantSummary: "Duplicacy Backup is already up to date",
		},
		{
			name:        "reinstall requested",
			status:      UpdateStatusReinstallRequested,
			wantStatus:  "reinstall-requested",
			wantEvent:   "update_reinstall_requested",
			wantSummary: "Duplicacy Backup update reinstall requested",
		},
		{name: "available", status: UpdateStatusAvailable},
		{name: "failed", status: UpdateStatusFailed},
		{name: "cancelled", status: UpdateStatusCancelled},
		{name: "unknown", status: UpdateStatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotEvent, gotSummary := classifyUpdateSuccessStatus(tt.status)
			if gotStatus != tt.wantStatus || gotEvent != tt.wantEvent || gotSummary != tt.wantSummary {
				t.Fatalf("classifyUpdateSuccessStatus(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.status, gotStatus, gotEvent, gotSummary, tt.wantStatus, tt.wantEvent, tt.wantSummary)
			}
		})
	}
}

func TestUpdateFailureEventMappingContract(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantEvent string
	}{
		{name: "checksum", err: errors.New("downloaded package checksum did not match duplicacy-backup.tar.gz.sha256"), wantEvent: "update_checksum_failed"},
		{name: "download", err: errors.New("failed to download duplicacy-backup.tar.gz: 404 Not Found"), wantEvent: "update_download_failed"},
		{name: "install", err: errors.New("update install failed: exit status 1"), wantEvent: "update_install_failed"},
		{name: "extract", err: errors.New("failed to extract package: unsupported file"), wantEvent: "update_install_failed"},
		{name: "staging", err: errors.New("failed to create update staging directory: permission denied"), wantEvent: "update_install_failed"},
		{name: "check", err: errors.New("GitHub release metadata did not include a tag name"), wantEvent: "update_check_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyUpdateFailureEvent(tt.err); got != tt.wantEvent {
				t.Fatalf("classifyUpdateFailureEvent(%v) = %q, want %q", tt.err, got, tt.wantEvent)
			}
		})
	}
}

func TestUpdateNotifyTestEventStatusMappingContract(t *testing.T) {
	tests := []struct {
		event      string
		wantStatus string
	}{
		{event: "update_install_succeeded", wantStatus: "succeeded"},
		{event: "update_already_current", wantStatus: "current"},
		{event: "update_reinstall_requested", wantStatus: "reinstall-requested"},
		{event: "update_check_failed", wantStatus: "failed"},
		{event: "update_download_failed", wantStatus: "failed"},
		{event: "update_checksum_failed", wantStatus: "failed"},
		{event: "update_install_failed", wantStatus: "failed"},
		{event: "", wantStatus: "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			if got := updateStatusForEvent(tt.event); got != tt.wantStatus {
				t.Fatalf("updateStatusForEvent(%q) = %q, want %q", tt.event, got, tt.wantStatus)
			}
		})
	}
}

func writeUpdateNotifyConfig(t *testing.T, configDir, content string) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	path := filepath.Join(configDir, "duplicacy-backup.toml")
	if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
