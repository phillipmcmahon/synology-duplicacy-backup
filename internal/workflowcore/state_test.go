package workflowcore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateFilePathAndLoadRunState(t *testing.T) {
	stateDir := t.TempDir()
	meta := Metadata{StateDir: stateDir}
	path := StateFilePath(meta, "homes", "onsite-usb")
	if path != filepath.Join(stateDir, "homes.onsite-usb.json") {
		t.Fatalf("StateFilePath = %q", path)
	}

	if err := os.WriteFile(path, []byte(`{"last_run_result":"success","last_successful_backup_revision":42}`), 0600); err != nil {
		t.Fatal(err)
	}

	state, err := LoadRunState(meta, "homes", "onsite-usb")
	if err != nil {
		t.Fatalf("LoadRunState returned error: %v", err)
	}
	if state.Label != "homes" || state.Storage != "onsite-usb" {
		t.Fatalf("state identity = %q/%q", state.Label, state.Storage)
	}
	if state.LastRunResult != "success" || state.LastSuccessfulBackupRevision != 42 {
		t.Fatalf("state body = %+v", state)
	}
}

func TestLoadRunStatePreservesStoredIdentity(t *testing.T) {
	stateDir := t.TempDir()
	path := StateFilePath(Metadata{StateDir: stateDir}, "homes", "onsite-usb")
	if err := os.WriteFile(path, []byte(`{"label":"stored","storage":"remote"}`), 0600); err != nil {
		t.Fatal(err)
	}

	state, err := LoadRunState(Metadata{StateDir: stateDir}, "homes", "onsite-usb")
	if err != nil {
		t.Fatalf("LoadRunState returned error: %v", err)
	}
	if state.Label != "stored" || state.Storage != "remote" {
		t.Fatalf("state identity overwritten: %+v", state)
	}
}

func TestLoadRunStateErrors(t *testing.T) {
	meta := Metadata{StateDir: t.TempDir()}
	if _, err := LoadRunState(meta, "homes", "missing"); err == nil {
		t.Fatalf("LoadRunState missing file expected error")
	}

	path := StateFilePath(meta, "homes", "bad")
	if err := os.WriteFile(path, []byte(`not-json`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRunState(meta, "homes", "bad")
	if err == nil || !strings.Contains(err.Error(), "invalid state file") {
		t.Fatalf("LoadRunState invalid json error = %v", err)
	}
}
