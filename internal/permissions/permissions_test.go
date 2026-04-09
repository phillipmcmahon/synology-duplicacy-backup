package permissions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	dir := t.TempDir()
	log, err := logger.New(dir, "test", false)
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return log
}

// ─── Fix DryRun tests ───────────────────────────────────────────────────────

func TestFix_DryRun(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner()
	target := t.TempDir()

	// Create a file with default perms
	testFile := filepath.Join(target, "file.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	// Dry run should not change anything
	if err := Fix(mock, log, target, "root", "root", true); err != nil {
		t.Fatalf("Fix (dry-run) failed: %v", err)
	}

	// Verify permissions were NOT changed
	info, _ := os.Stat(testFile)
	if info.Mode().Perm() == 0660 {
		t.Error("dry-run should not change file permissions")
	}

	// Mock should not have been called
	if len(mock.Invocations) != 0 {
		t.Error("dry-run should not invoke any commands")
	}
}

// ─── Fix permission changes tests ───────────────────────────────────────────

func TestFix_SetsDirectoryPerms(t *testing.T) {
	log := newTestLogger(t)
	// Mock chown success
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	target := t.TempDir()
	subdir := filepath.Join(target, "subdir")
	os.MkdirAll(subdir, 0755)

	err := Fix(mock, log, target, "testuser", "testgroup", false)
	if err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify chown was called
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Cmd != "chown" {
		t.Errorf("cmd = %q, want chown", mock.Invocations[0].Cmd)
	}

	info, _ := os.Stat(subdir)
	if info.Mode().Perm() != 0770 {
		t.Errorf("subdir perm = %04o, want 0770", info.Mode().Perm())
	}
}

func TestFix_SetsFilePerms(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	target := t.TempDir()
	testFile := filepath.Join(target, "file.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := Fix(mock, log, target, "testuser", "testgroup", false)
	if err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	info, _ := os.Stat(testFile)
	if info.Mode().Perm() != 0660 {
		t.Errorf("file perm = %04o, want 0660", info.Mode().Perm())
	}
}

func TestFix_NestedStructure(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	target := t.TempDir()

	// Create nested structure
	deepDir := filepath.Join(target, "a", "b", "c")
	os.MkdirAll(deepDir, 0755)
	os.WriteFile(filepath.Join(deepDir, "file.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(target, "root.txt"), []byte("test"), 0644)

	err := Fix(mock, log, target, "testuser", "testgroup", false)
	if err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Check deep directory
	info, _ := os.Stat(deepDir)
	if info.Mode().Perm() != 0770 {
		t.Errorf("deep dir perm = %04o, want 0770", info.Mode().Perm())
	}

	// Check deep file
	info, _ = os.Stat(filepath.Join(deepDir, "file.txt"))
	if info.Mode().Perm() != 0660 {
		t.Errorf("deep file perm = %04o, want 0660", info.Mode().Perm())
	}
}

// ─── Error handling tests ───────────────────────────────────────────────────

func TestFix_ChownFailure(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("chown failed")})
	target := t.TempDir()

	err := Fix(mock, log, target, "nonexistent", "nonexistent", false)
	if err == nil {
		t.Fatal("expected error for chown failure")
	}
}

func TestFix_InvalidTarget(t *testing.T) {
	log := newTestLogger(t)
	// chown succeeds but walk fails on nonexistent path
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	err := Fix(mock, log, "/nonexistent/path/that/does/not/exist", "root", "root", false)
	if err == nil {
		t.Fatal("expected error for invalid target path")
	}
}

// ─── Runner invocation validation ───────────────────────────────────────────

func TestFix_ChownArgs(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	target := t.TempDir()

	Fix(mock, log, target, "myuser", "mygroup", false)

	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	inv := mock.Invocations[0]
	if inv.Cmd != "chown" {
		t.Errorf("cmd = %q, want chown", inv.Cmd)
	}
	wantArgs := []string{"-R", "myuser:mygroup", target}
	for i, a := range wantArgs {
		if i >= len(inv.Args) || inv.Args[i] != a {
			t.Errorf("args[%d] = %q, want %q", i, inv.Args[i], a)
		}
	}
}
