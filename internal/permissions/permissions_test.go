package permissions

import (
	"os"
	"path/filepath"
	"testing"

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
	target := t.TempDir()

	// Create a file with default perms
	testFile := filepath.Join(target, "file.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	// Dry run should not change anything
	if err := Fix(log, target, "root", "root", true); err != nil {
		t.Fatalf("Fix (dry-run) failed: %v", err)
	}

	// Verify permissions were NOT changed
	info, _ := os.Stat(testFile)
	if info.Mode().Perm() == 0660 {
		t.Error("dry-run should not change file permissions")
	}
}

// ─── Fix permission changes tests ───────────────────────────────────────────

func TestFix_SetsDirectoryPerms(t *testing.T) {
	log := newTestLogger(t)
	target := t.TempDir()
	subdir := filepath.Join(target, "subdir")
	os.MkdirAll(subdir, 0755)

	// We can't test chown without root, so we'll just test the chmod part.
	// Fix will fail on chown if not root - that's expected.
	// Let's test with dry-run=false but accept chown may fail.
	err := Fix(log, target, os.Getenv("USER"), os.Getenv("USER"), false)
	// chown may fail if not root - that's acceptable for the test
	if err != nil {
		t.Skipf("Skipping: chown failed (probably not root): %v", err)
	}

	info, _ := os.Stat(subdir)
	if info.Mode().Perm() != 0770 {
		t.Errorf("subdir perm = %04o, want 0770", info.Mode().Perm())
	}
}

func TestFix_SetsFilePerms(t *testing.T) {
	log := newTestLogger(t)
	target := t.TempDir()
	testFile := filepath.Join(target, "file.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := Fix(log, target, os.Getenv("USER"), os.Getenv("USER"), false)
	if err != nil {
		t.Skipf("Skipping: chown failed (probably not root): %v", err)
	}

	info, _ := os.Stat(testFile)
	if info.Mode().Perm() != 0660 {
		t.Errorf("file perm = %04o, want 0660", info.Mode().Perm())
	}
}

func TestFix_NestedStructure(t *testing.T) {
	log := newTestLogger(t)
	target := t.TempDir()

	// Create nested structure
	deepDir := filepath.Join(target, "a", "b", "c")
	os.MkdirAll(deepDir, 0755)
	os.WriteFile(filepath.Join(deepDir, "file.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(target, "root.txt"), []byte("test"), 0644)

	err := Fix(log, target, os.Getenv("USER"), os.Getenv("USER"), false)
	if err != nil {
		t.Skipf("Skipping: chown failed (probably not root): %v", err)
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

func TestFix_InvalidTarget(t *testing.T) {
	log := newTestLogger(t)
	err := Fix(log, "/nonexistent/path/that/does/not/exist", "root", "root", false)
	if err == nil {
		t.Fatal("expected error for invalid target path")
	}
}

func TestFix_InvalidOwner(t *testing.T) {
	log := newTestLogger(t)
	target := t.TempDir()
	err := Fix(log, target, "nonexistent_user_xyz_12345", "nonexistent_group_xyz_12345", false)
	if err == nil {
		t.Fatal("expected error for invalid owner/group")
	}
}
