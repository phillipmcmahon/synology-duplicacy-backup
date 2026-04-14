package workflow

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

func TestHandleNotifyCommand_DryRun(t *testing.T) {
	configDir := t.TempDir()
	writeNotifyConfig(t, configDir, "homes", strings.Join([]string{
		`label = "homes"`,
		`source_path = "/volume1/homes"`,
		`[health.notify]`,
		`webhook_url = "https://example.invalid/duplicacy"`,
		`[targets.onsite-usb]`,
		`type = "filesystem"`,
		`location = "local"`,
		`destination = "/backups"`,
		`repository = "homes"`,
	}, "\n"))

	req := &Request{
		NotifyCommand:   "test",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
		NotifyProvider:  "all",
		NotifySeverity:  "warning",
		DryRun:          true,
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())

	out, err := HandleNotifyCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleNotifyCommand() error = %v", err)
	}
	if !strings.Contains(out, "Notification test for homes/onsite-usb") ||
		!strings.Contains(out, "Webhook") ||
		!strings.Contains(out, "preview") {
		t.Fatalf("output = %q", out)
	}
}

func TestHandleNotifyCommand_SendAllProviders(t *testing.T) {
	var webhookBody string
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		webhookBody = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	var ntfyTitle, ntfyBody string
	ntfyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ntfyTitle = r.Header.Get("Title")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		ntfyBody = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer ntfyServer.Close()

	configDir := t.TempDir()
	writeNotifyConfig(t, configDir, "homes", strings.Join([]string{
		`label = "homes"`,
		`source_path = "/volume1/homes"`,
		`[health.notify]`,
		`webhook_url = "` + webhookServer.URL + `"`,
		`[health.notify.ntfy]`,
		`url = "` + ntfyServer.URL + `"`,
		`topic = "duplicacy-alerts"`,
		`[targets.onsite-usb]`,
		`type = "filesystem"`,
		`location = "local"`,
		`destination = "/backups"`,
		`repository = "homes"`,
	}, "\n"))

	req := &Request{
		NotifyCommand:   "test",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
		SecretsDir:      t.TempDir(),
		NotifyProvider:  "all",
		NotifySeverity:  "critical",
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())

	out, err := HandleNotifyCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleNotifyCommand() error = %v", err)
	}
	if !strings.Contains(out, "Webhook") || !strings.Contains(out, "Ntfy") || !strings.Contains(out, "Success") {
		t.Fatalf("output = %q", out)
	}
	if !strings.Contains(webhookBody, `"category":"test"`) || !strings.Contains(webhookBody, `"event":"notification_test"`) {
		t.Fatalf("webhookBody = %q", webhookBody)
	}
	if ntfyTitle != "CRITICAL: Notification test for homes/onsite-usb" {
		t.Fatalf("Title = %q", ntfyTitle)
	}
	if !strings.Contains(ntfyBody, "Category: test") || !strings.Contains(ntfyBody, "simulated operator-initiated test notification") {
		t.Fatalf("ntfyBody = %q", ntfyBody)
	}
}

func TestHandleNotifyCommand_JSONSummary(t *testing.T) {
	configDir := t.TempDir()
	writeNotifyConfig(t, configDir, "homes", strings.Join([]string{
		`label = "homes"`,
		`source_path = "/volume1/homes"`,
		`[health.notify.ntfy]`,
		`url = "https://ntfy.sh"`,
		`topic = "duplicacy-alerts"`,
		`[targets.onsite-usb]`,
		`type = "filesystem"`,
		`location = "local"`,
		`destination = "/backups"`,
		`repository = "homes"`,
	}, "\n"))

	req := &Request{
		NotifyCommand:   "test",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
		NotifyProvider:  "ntfy",
		NotifySeverity:  "warning",
		DryRun:          true,
		JSONSummary:     true,
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())

	out, err := HandleNotifyCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleNotifyCommand() error = %v", err)
	}
	var report NotifyTestReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out)
	}
	if report.Result != "preview" || report.Provider != "ntfy" || len(report.Providers) != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestHandleNotifyCommand_NoDestinationConfigured(t *testing.T) {
	configDir := t.TempDir()
	writeNotifyConfig(t, configDir, "homes", strings.Join([]string{
		`label = "homes"`,
		`source_path = "/volume1/homes"`,
		`[targets.onsite-usb]`,
		`type = "filesystem"`,
		`location = "local"`,
		`destination = "/backups"`,
		`repository = "homes"`,
	}, "\n"))

	req := &Request{
		NotifyCommand:   "test",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
		NotifyProvider:  "all",
		NotifySeverity:  "warning",
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())

	out, err := HandleNotifyCommand(req, meta, testRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out, "Result") || !strings.Contains(out, "Failed") {
		t.Fatalf("output = %q", out)
	}
	if !strings.Contains(OperatorMessage(err), "Notification test failed for homes/onsite-usb") {
		t.Fatalf("err = %v", err)
	}
}

func TestHandleNotifyCommand_ObjectTargetCanSendWithoutReadableSecretsWhenTokenIsOptional(t *testing.T) {
	configDir := t.TempDir()
	var ntfyTitle string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ntfyTitle = r.Header.Get("Title")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	writeNotifyConfig(t, configDir, "homes", strings.Join([]string{
		`label = "homes"`,
		`source_path = "/volume1/homes"`,
		`[health.notify.ntfy]`,
		`url = "` + server.URL + `"`,
		`topic = "duplicacy-alerts"`,
		`[targets.offsite-storj]`,
		`type = "object"`,
		`location = "remote"`,
		`destination = "s3://EU@gateway.storjshare.io/bucket"`,
		`repository = "homes"`,
	}, "\n"))

	req := &Request{
		NotifyCommand:   "test",
		RequestedTarget: "offsite-storj",
		Source:          "homes",
		ConfigDir:       configDir,
		SecretsDir:      t.TempDir(),
		NotifyProvider:  "ntfy",
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	oldLoad := loadOptionalHealthNtfyToken
	loadOptionalHealthNtfyToken = func(path, target string) (string, error) {
		return "", apperrors.NewSecretsError("open", errors.New("secrets file is not readable: /root/.secrets/homes-secrets.toml"), "path", "/root/.secrets/homes-secrets.toml")
	}
	defer func() {
		loadOptionalHealthNtfyToken = oldLoad
	}()

	out, err := HandleNotifyCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleNotifyCommand() error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Ntfy") || !strings.Contains(out, "Success") {
		t.Fatalf("output = %q", out)
	}
	if ntfyTitle != "WARNING: Notification test for homes/offsite-storj" {
		t.Fatalf("Title = %q", ntfyTitle)
	}
}

func TestHandleNotifyCommand_ObjectTargetTokenParseErrorStillFails(t *testing.T) {
	configDir := t.TempDir()
	writeNotifyConfig(t, configDir, "homes", strings.Join([]string{
		`label = "homes"`,
		`source_path = "/volume1/homes"`,
		`[health.notify.ntfy]`,
		`url = "https://ntfy.sh"`,
		`topic = "duplicacy-alerts"`,
		`[targets.offsite-storj]`,
		`type = "object"`,
		`location = "remote"`,
		`destination = "s3://EU@gateway.storjshare.io/bucket"`,
		`repository = "homes"`,
	}, "\n"))

	req := &Request{
		NotifyCommand:   "test",
		RequestedTarget: "offsite-storj",
		Source:          "homes",
		ConfigDir:       configDir,
		SecretsDir:      t.TempDir(),
		NotifyProvider:  "ntfy",
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	oldLoad := loadOptionalHealthNtfyToken
	loadOptionalHealthNtfyToken = func(path, target string) (string, error) {
		return "", apperrors.NewSecretsError("parse", errors.New("unexpected key \"bad\" in secrets file /root/.secrets/homes-secrets.toml"), "source", "/root/.secrets/homes-secrets.toml")
	}
	defer func() {
		loadOptionalHealthNtfyToken = oldLoad
	}()

	out, err := HandleNotifyCommand(req, meta, testRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out, "Ntfy") || !strings.Contains(out, "failed") {
		t.Fatalf("output = %q", out)
	}
	if !strings.Contains(OperatorMessage(err), "unexpected key") {
		t.Fatalf("OperatorMessage(err) = %q", OperatorMessage(err))
	}
}

func writeNotifyConfig(t *testing.T, configDir, label, content string) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	path := filepath.Join(configDir, label+"-backup.toml")
	if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
