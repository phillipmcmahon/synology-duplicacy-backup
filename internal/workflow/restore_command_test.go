package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleRestoreCommand_PlanLocalReadOnlyWithState(t *testing.T) {
	configDir := t.TempDir()
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", "/volume1/homes", "/backups", "", "", 4, "-keep 0:365"))
	if err := saveRunState(meta, "homes", "onsite-usb", &RunState{
		LastSuccessfulBackupRevision: 2403,
		LastSuccessfulBackupAt:       "2026-04-20T02:30:00Z",
	}); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	req := &Request{RestoreCommand: "plan", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}

	for _, token := range []string{
		"Restore plan for homes/onsite-usb",
		"Read Only",
		"true",
		"Executes Restore",
		"false",
		"Section: Resolved",
		"Source Path",
		"/volume1/homes",
		"Storage",
		"/backups/homes",
		"Section: Safe Workspace",
		"/volume1/restore-drills/homes-onsite-usb",
		"Snapshot ID",
		"data",
		"Section: Revision Signal",
		"Latest Revision",
		"2403 (2026-04-20T02:30:00Z)",
		"Section: Suggested Commands",
		"duplicacy init 'data' '/backups/homes'",
		"duplicacy list -files -r <revision>",
		"duplicacy restore -r <revision> -stats",
		`duplicacy restore -r <revision> -stats -- "relative/path/from/snapshot"`,
		"rsync -a --dry-run",
		"Section: Safety",
		"not performed by this command",
		"docs/restore-drills.md",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}

	if strings.Contains(out, "mkdir -p /volume1/restore-drills") {
		t.Fatalf("workspace command should shell-quote paths:\n%s", out)
	}
	if _, err := os.Stat("/volume1/restore-drills/homes-onsite-usb"); err == nil {
		t.Fatalf("restore plan command must not create the suggested workspace")
	}
}

func TestHandleRestoreCommand_PlanRemoteDoesNotLoadSecrets(t *testing.T) {
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://gateway.example.invalid/bucket/homes", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "plan", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}

	secretsFile := filepath.Join(secretsDir, "homes-secrets.toml")
	for _, token := range []string{
		"Restore plan for homes/offsite-storj",
		"Location",
		"remote",
		"Secrets File",
		secretsFile,
		"State",
		"Not found",
		"duplicacy init 'data' 's3://gateway.example.invalid/bucket/homes'",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "s3_id") || strings.Contains(out, "s3_secret") {
		t.Fatalf("restore plan should not expose or require secret key values:\n%s", out)
	}
}

func TestHandleRestoreCommand_PrepareLocalWritesWorkspacePreferences(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "prepare", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}

	preferences := filepath.Join(workspace, ".duplicacy", "preferences")
	for _, token := range []string{
		"Restore workspace prepared for homes/onsite-usb",
		"Executes Restore",
		"false",
		"Copies Back",
		"false",
		"Section: Workspace",
		workspace,
		preferences,
		"duplicacy list",
		"duplicacy list -files -r <revision>",
		"not performed by this command",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "duplicacy restore") {
		t.Fatalf("prepare output should not suggest restore execution:\n%s", out)
	}

	body, err := os.ReadFile(preferences)
	if err != nil {
		t.Fatalf("ReadFile(preferences) error = %v", err)
	}
	var prefs []map[string]any
	if err := json.Unmarshal(body, &prefs); err != nil {
		t.Fatalf("preferences JSON error = %v\n%s", err, body)
	}
	if len(prefs) != 1 || prefs[0]["id"] != "data" || prefs[0]["repository"] != workspace || prefs[0]["storage"] != storage {
		t.Fatalf("preferences = %#v", prefs)
	}
	if _, ok := prefs[0]["keys"].(map[string]any); ok {
		t.Fatalf("local prepare should not write storage keys: %#v", prefs[0])
	}
}

func TestHandleRestoreCommand_PrepareRemoteLoadsSecretsWithoutPrintingValues(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote restore prepare requires root-owned secrets file")
	}

	configDir := t.TempDir()
	secretsDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket/homes", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{RestoreCommand: "prepare", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj", RestoreWorkspace: workspace}
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}
	if !strings.Contains(out, "Secrets File") || !strings.Contains(out, filepath.Join(secretsDir, "homes-secrets.toml")) {
		t.Fatalf("output missing secrets file:\n%s", out)
	}
	if strings.Contains(out, "ABCDEFGHIJKLMNOPQRSTUVWXYZ01") || strings.Contains(out, "abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR") {
		t.Fatalf("output leaked secret values:\n%s", out)
	}
	body, err := os.ReadFile(filepath.Join(workspace, ".duplicacy", "preferences"))
	if err != nil {
		t.Fatalf("ReadFile(preferences) error = %v", err)
	}
	if !strings.Contains(string(body), `"s3_id"`) || !strings.Contains(string(body), `"s3_secret"`) {
		t.Fatalf("preferences missing required storage keys:\n%s", body)
	}
}

func TestHandleRestoreCommand_PrepareRejectsUnsafeWorkspaces(t *testing.T) {
	configDir := t.TempDir()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source")
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, filepath.Join(t.TempDir(), "backups", "homes"), "", "", 4, "-keep 0:365"))

	tests := []struct {
		name      string
		workspace string
		want      string
	}{
		{name: "relative", workspace: "relative/path", want: "absolute path"},
		{name: "source", workspace: sourcePath, want: "live source path"},
		{name: "source child", workspace: filepath.Join(sourcePath, "restore"), want: "inside the live source path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{RestoreCommand: "prepare", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: tt.workspace}
			_, err := HandleRestoreCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestHandleRestoreCommand_PrepareRejectsNonEmptyWorkspace(t *testing.T) {
	configDir := t.TempDir()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "existing.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, "/volume1/homes", filepath.Join(t.TempDir(), "backups", "homes"), "", "", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "prepare", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	_, err := HandleRestoreCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "must be empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRestoreCommand_Unsupported(t *testing.T) {
	_, err := HandleRestoreCommand(&Request{RestoreCommand: "run", Source: "homes"}, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "unsupported restore command") {
		t.Fatalf("err = %v", err)
	}
}
