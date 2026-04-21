package workflow

import (
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

func TestHandleRestoreCommand_Unsupported(t *testing.T) {
	_, err := HandleRestoreCommand(&Request{RestoreCommand: "run", Source: "homes"}, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "unsupported restore command") {
		t.Fatalf("err = %v", err)
	}
}
