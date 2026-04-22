package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRollbackCheckOnlySelectsNewestPreviousRetainedVersion(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "6.0.0")
	installRoot := filepath.Dir(mustEvalSymlinks(t, executablePath))
	writeFile(t, filepath.Join(installRoot, "duplicacy-backup_5.1.1_linux_amd64"), "#!/bin/sh\n", 0755)
	writeFile(t, filepath.Join(installRoot, "duplicacy-backup_4.9.0_linux_amd64"), "#!/bin/sh\n", 0755)

	updater := New("duplicacy-backup", "6.0.0", testRuntime(executablePath))
	result, err := updater.RollbackResult(RollbackOptions{CheckOnly: true})
	if err != nil {
		t.Fatalf("RollbackResult() error = %v", err)
	}
	if !strings.Contains(result.Output, "Current Version") ||
		!strings.Contains(result.Output, "v6.0.0") ||
		!strings.Contains(result.Output, "Target Version") ||
		!strings.Contains(result.Output, "v5.1.1") ||
		!strings.Contains(result.Output, "Ready to rollback") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestRollbackActivatesExplicitRetainedVersion(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "6.0.0")
	installRoot := filepath.Dir(mustEvalSymlinks(t, executablePath))
	targetName := "duplicacy-backup_5.1.1_linux_amd64"
	writeFile(t, filepath.Join(installRoot, targetName), "#!/bin/sh\n", 0755)

	updater := New("duplicacy-backup", "6.0.0", testRuntime(executablePath))
	result, err := updater.RollbackResult(RollbackOptions{RequestedVersion: "v5.1.1", Yes: true})
	if err != nil {
		t.Fatalf("RollbackResult() error = %v", err)
	}
	currentTarget, err := os.Readlink(filepath.Join(installRoot, "current"))
	if err != nil {
		t.Fatalf("Readlink(current) error = %v", err)
	}
	if currentTarget != targetName {
		t.Fatalf("current target = %q, want %q", currentTarget, targetName)
	}
	if !strings.Contains(result.Output, "Rolled back") ||
		!strings.Contains(result.Output, "Activated:") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestRollbackActivationRequiresYesWhenNonInteractive(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "6.0.0")
	installRoot := filepath.Dir(mustEvalSymlinks(t, executablePath))
	writeFile(t, filepath.Join(installRoot, "duplicacy-backup_5.1.1_linux_amd64"), "#!/bin/sh\n", 0755)

	updater := New("duplicacy-backup", "6.0.0", testRuntime(executablePath))
	_, err := updater.RollbackResult(RollbackOptions{})
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("RollbackResult() err = %v", err)
	}
}

func TestRollbackFailsWhenNoPreviousVersionRetained(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "6.0.0")
	updater := New("duplicacy-backup", "6.0.0", testRuntime(executablePath))
	_, err := updater.RollbackResult(RollbackOptions{CheckOnly: true})
	if err == nil || !strings.Contains(err.Error(), "no previous retained version") {
		t.Fatalf("RollbackResult() err = %v", err)
	}
}

func TestRollbackRejectsActiveExplicitVersion(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "6.0.0")
	updater := New("duplicacy-backup", "6.0.0", testRuntime(executablePath))
	_, err := updater.RollbackResult(RollbackOptions{CheckOnly: true, RequestedVersion: "v6.0.0"})
	if err == nil || !strings.Contains(err.Error(), "already active") {
		t.Fatalf("RollbackResult() err = %v", err)
	}
}

func mustEvalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", path, err)
	}
	return resolved
}
