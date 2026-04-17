package permissions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
)

// ─── Fix DryRun tests ───────────────────────────────────────────────────────

func TestFix_DryRun(t *testing.T) {
	mock := execpkg.NewMockRunner()
	target := t.TempDir()

	// Create a file with default perms
	testFile := filepath.Join(target, "file.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	// Dry run should not change anything
	if err := Fix(mock, target, "root", "root", true); err != nil {
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
	// Mock chown success
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	target := t.TempDir()
	subdir := filepath.Join(target, "subdir")
	os.MkdirAll(subdir, 0755)

	err := Fix(mock, target, "testuser", "testgroup", false)
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
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	target := t.TempDir()
	testFile := filepath.Join(target, "file.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := Fix(mock, target, "testuser", "testgroup", false)
	if err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	info, _ := os.Stat(testFile)
	if info.Mode().Perm() != 0660 {
		t.Errorf("file perm = %04o, want 0660", info.Mode().Perm())
	}
}

func TestFix_NestedStructure(t *testing.T) {
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	target := t.TempDir()

	// Create nested structure
	deepDir := filepath.Join(target, "a", "b", "c")
	os.MkdirAll(deepDir, 0755)
	os.WriteFile(filepath.Join(deepDir, "file.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(target, "root.txt"), []byte("test"), 0644)

	err := Fix(mock, target, "testuser", "testgroup", false)
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

func TestFix_SkipsSymlinkChmodTarget(t *testing.T) {
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	target := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	link := filepath.Join(target, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if err := Fix(mock, target, "testuser", "testgroup", false); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	info, err := os.Stat(outside)
	if err != nil {
		t.Fatalf("stat outside file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("symlink target perm = %04o, want unchanged 0600", info.Mode().Perm())
	}
}

// ─── Error handling tests ───────────────────────────────────────────────────

func TestFix_ChownFailure(t *testing.T) {
	mock := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("chown failed")})
	target := t.TempDir()

	err := Fix(mock, target, "nonexistent", "nonexistent", false)
	if err == nil {
		t.Fatal("expected error for chown failure")
	}
	var permErr *apperrors.PermissionsError
	if !errors.As(err, &permErr) {
		t.Errorf("expected *PermissionsError, got %T", err)
	}
}

func TestFix_InvalidTarget(t *testing.T) {
	// chown succeeds but walk fails on nonexistent path
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	err := Fix(mock, "/nonexistent/path/that/does/not/exist", "root", "root", false)
	if err == nil {
		t.Fatal("expected error for invalid target path")
	}
	var permErr *apperrors.PermissionsError
	if !errors.As(err, &permErr) {
		t.Errorf("expected *PermissionsError, got %T", err)
	}
}

// ─── Runner invocation validation ───────────────────────────────────────────

func TestFix_ChownArgs(t *testing.T) {
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	target := t.TempDir()

	Fix(mock, target, "myuser", "mygroup", false)

	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	inv := mock.Invocations[0]
	if inv.Cmd != "chown" {
		t.Errorf("cmd = %q, want chown", inv.Cmd)
	}
	wantArgs := []string{"-h", "-R", "myuser:mygroup", target}
	for i, a := range wantArgs {
		if i >= len(inv.Args) || inv.Args[i] != a {
			t.Errorf("args[%d] = %q, want %q", i, inv.Args[i], a)
		}
	}
}

func TestFix_ChownDoesNotFollowSymlinks(t *testing.T) {
	mock := execpkg.NewMockRunner(execpkg.MockResult{})
	target := t.TempDir()

	if err := Fix(mock, target, "myuser", "mygroup", false); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	inv := mock.Invocations[0]
	if len(inv.Args) < 2 || inv.Args[0] != "-h" || inv.Args[1] != "-R" {
		t.Fatalf("chown args = %v, want no-follow recursive flag first", inv.Args)
	}
}

// ─── Structured error context tests ─────────────────────────────────────────

func TestFix_ChownFailure_ErrorContext(t *testing.T) {
	mock := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("chown failed")})
	target := t.TempDir()

	err := Fix(mock, target, "admin", "users", false)
	var permErr *apperrors.PermissionsError
	if !errors.As(err, &permErr) {
		t.Fatalf("expected *PermissionsError, got %T", err)
	}
	if permErr.Phase != "chown" {
		t.Errorf("phase = %q, want chown", permErr.Phase)
	}
	if permErr.Context["target"] != target {
		t.Errorf("context target = %q, want %q", permErr.Context["target"], target)
	}
	if permErr.Context["owner"] != "admin:users" {
		t.Errorf("context owner = %q, want admin:users", permErr.Context["owner"])
	}
}
