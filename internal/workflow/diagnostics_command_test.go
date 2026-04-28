package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleDiagnosticsCommand_LocalTargetIncludesStateAndPermissions(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := t.TempDir()
	storagePath := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", sourcePath, storagePath, 4, ""))
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	if err := saveRunState(meta, "homes", "onsite-usb", &RunState{
		LastRunResult:                "success",
		LastRunCompletedAt:           "2026-04-22 08:00:00",
		LastSuccessfulRunAt:          "2026-04-22 08:00:00",
		LastSuccessfulBackupRevision: 42,
		LastSuccessfulBackupAt:       "2026-04-22 08:00:00",
		LastStatusAt:                 "2026-04-22 09:00:00",
	}); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	req := &Request{DiagnosticsCommand: "diagnostics", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := HandleDiagnosticsCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleDiagnosticsCommand() error = %v", err)
	}
	for _, want := range []string{
		"Diagnostics for homes/onsite-usb",
		"Storage Scheme",
		"local",
		"Secrets Status",
		"Not required",
		"State Status",
		"Available",
		"Last Backup Rev",
		"42",
		"Section: Permissions",
		"Storage Path",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestHandleDiagnosticsCommand_RemoteSecretsAreRedacted(t *testing.T) {
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://EU@gateway.example.invalid/bucket/homes", 4, ""))
	secretPath := filepath.Join(secretsDir, "homes-secrets.toml")
	body := "[targets.offsite-storj.keys]\ns3_id = \"visible-id\"\ns3_secret = \"visible-secret\"\n"
	if err := os.WriteFile(secretPath, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := &Request{DiagnosticsCommand: "diagnostics", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	out, err := HandleDiagnosticsCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleDiagnosticsCommand() error = %v", err)
	}
	if !strings.Contains(out, "Secrets Status") || !strings.Contains(out, "Available (**** (2 keys))") {
		t.Fatalf("output = %q", out)
	}
	if strings.Contains(out, "visible-id") || strings.Contains(out, "visible-secret") {
		t.Fatalf("diagnostics leaked secret values:\n%s", out)
	}
}

func TestHandleDiagnosticsCommand_JSONSummary(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://user:password@gateway.example.invalid/bucket/homes?token=secret-token&region=EU", 4, ""))

	req := &Request{DiagnosticsCommand: "diagnostics", Source: "homes", ConfigDir: configDir, SecretsDir: t.TempDir(), RequestedTarget: "offsite-storj", JSONSummary: true}
	out, err := HandleDiagnosticsCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleDiagnosticsCommand() error = %v", err)
	}
	var report DiagnosticsReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out)
	}
	if report.Label != "homes" ||
		report.Target != "offsite-storj" ||
		report.StorageScheme != "s3" ||
		report.SecretsStatus != "Required (file not found)" ||
		len(report.Paths) == 0 {
		t.Fatalf("report = %+v", report)
	}
	if strings.Contains(out, "password") || strings.Contains(out, "secret-token") || !strings.Contains(out, "token=%2A%2A%2A%2A") {
		t.Fatalf("diagnostics JSON did not redact storage secrets:\n%s", out)
	}
}

func TestHandleDiagnosticsCommand_JSONSummaryIncludesZeroRevisionWhenStateExists(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", t.TempDir(), t.TempDir(), 4, ""))
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	if err := saveRunState(meta, "homes", "onsite-usb", &RunState{
		LastRunResult:                "success",
		LastSuccessfulBackupRevision: 0,
	}); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	req := &Request{DiagnosticsCommand: "diagnostics", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", JSONSummary: true}
	out, err := HandleDiagnosticsCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleDiagnosticsCommand() error = %v", err)
	}
	if !strings.Contains(out, `"last_successful_backup_revision": 0`) {
		t.Fatalf("diagnostics JSON should include zero revision when state exists:\n%s", out)
	}
}
