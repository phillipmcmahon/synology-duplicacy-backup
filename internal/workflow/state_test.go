package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunStateRoundTrip(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()

	state := &RunState{
		Label:                        "homes",
		LastRunResult:                "success",
		LastSuccessfulRunAt:          "2026-04-10T17:00:00Z",
		LastSuccessfulOperation:      "Backup",
		LastSuccessfulBackupRevision: 42,
		LastSuccessfulBackupAt:       "2026-04-10T17:00:00Z",
	}
	if err := saveRunState(meta, "homes", state, "local"); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	loaded, err := loadRunState(meta, "homes", "local")
	if err != nil {
		t.Fatalf("loadRunState() error = %v", err)
	}
	if loaded.LastSuccessfulBackupRevision != 42 || loaded.LastSuccessfulOperation != "Backup" {
		t.Fatalf("loaded = %+v", loaded)
	}

	dirInfo, err := os.Stat(meta.StateDir)
	if err != nil {
		t.Fatalf("Stat(state dir) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("state dir perms = %04o, want 0700", got)
	}

	fileInfo, err := os.Stat(stateFilePath(meta, "homes", "local"))
	if err != nil {
		t.Fatalf("Stat(state file) error = %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("state file perms = %04o, want 0600", got)
	}
}

func TestLoadRunState_DoesNotFallbackToLegacyLabelState(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()

	legacyPath := filepath.Join(meta.StateDir, "homes.json")
	if err := os.WriteFile(legacyPath, []byte(`{"label":"homes","last_successful_backup_revision":9}`), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := loadRunState(meta, "homes", "remote")
	if err == nil {
		t.Fatal("expected missing target state error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("error = %v, want IsNotExist", err)
	}
}
