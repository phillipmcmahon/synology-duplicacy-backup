package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunStateRoundTrip(t *testing.T) {
	meta := MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()

	state := &RunState{
		Label:                        "homes",
		LastRunResult:                "success",
		LastSuccessfulRunAt:          "2026-04-10T17:00:00Z",
		LastSuccessfulOperation:      "Backup",
		LastSuccessfulBackupRevision: 42,
		LastSuccessfulBackupAt:       "2026-04-10T17:00:00Z",
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	loaded, err := loadRunState(meta, "homes", "onsite-usb")
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

	fileInfo, err := os.Stat(stateFilePath(meta, "homes", "onsite-usb"))
	if err != nil {
		t.Fatalf("Stat(state file) error = %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("state file perms = %04o, want 0600", got)
	}
}

func TestSaveRunStateRestoresSudoOperatorOwnership(t *testing.T) {
	meta := MetadataForLogDir("duplicacy-backup", "9.1.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	meta.HasProfileOwner = true
	meta.ProfileOwnerUID = 1026
	meta.ProfileOwnerGID = 100

	var calls []string
	previous := profileChown
	profileChown = func(path string, uid, gid int) error {
		calls = append(calls, filepath.Base(path))
		if uid != 1026 || gid != 100 {
			t.Fatalf("profileChown(%q, %d, %d), want uid 1026 gid 100", path, uid, gid)
		}
		return nil
	}
	t.Cleanup(func() { profileChown = previous })

	if err := saveRunState(meta, "homes", "onsite-usb", &RunState{LastRunResult: "success"}); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("profileChown calls = %v, want state dir and state file", calls)
	}
	if calls[0] != filepath.Base(meta.StateDir) || calls[1] != "homes.onsite-usb.json" {
		t.Fatalf("profileChown calls = %v", calls)
	}
}

func TestLoadRunState_DoesNotFallbackToLegacyLabelState(t *testing.T) {
	meta := MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()

	legacyPath := filepath.Join(meta.StateDir, "homes.json")
	if err := os.WriteFile(legacyPath, []byte(`{"label":"homes","last_successful_backup_revision":9}`), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := loadRunState(meta, "homes", "offsite-storj")
	if err == nil {
		t.Fatal("expected missing target state error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("error = %v, want IsNotExist", err)
	}
}

func TestMutateRunStateLoadsMutatesAndSaves(t *testing.T) {
	meta := MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()

	if err := mutateRunState(meta, "homes", "onsite-usb", func(state *RunState) error {
		state.LastRunResult = "success"
		state.LastSuccessfulOperation = "Backup"
		return nil
	}); err != nil {
		t.Fatalf("mutateRunState() error = %v", err)
	}
	if err := mutateRunState(meta, "homes", "onsite-usb", func(state *RunState) error {
		state.LastDoctorAt = "2026-04-20T12:00:00Z"
		return nil
	}); err != nil {
		t.Fatalf("mutateRunState() second error = %v", err)
	}

	loaded, err := loadRunState(meta, "homes", "onsite-usb")
	if err != nil {
		t.Fatalf("loadRunState() error = %v", err)
	}
	if loaded.LastSuccessfulOperation != "Backup" || loaded.LastDoctorAt != "2026-04-20T12:00:00Z" {
		t.Fatalf("loaded = %+v", loaded)
	}
}
