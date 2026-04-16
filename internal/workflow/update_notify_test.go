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

	if err := MaybeSendUpdateFailureNotification(req, meta, rt, errors.New("update install failed: exit status 1")); err != nil {
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

	if err := MaybeSendUpdateFailureNotification(req, meta, rt, errors.New("release lookup failed")); err != nil {
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

	if err := MaybeSendUpdateFailureNotification(req, meta, rt, errors.New("release lookup failed")); err != nil {
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
	output := "Update\n  Result               : Installed\n"

	if err := MaybeSendUpdateSuccessNotification(req, meta, rt, output); err != nil {
		t.Fatalf("MaybeSendUpdateSuccessNotification() error = %v", err)
	}
	if gotTitle != "INFO: Duplicacy Backup update installed" {
		t.Fatalf("Title = %q", gotTitle)
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
