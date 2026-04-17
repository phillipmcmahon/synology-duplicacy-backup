package lock

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

func requireLinuxProc(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux /proc process semantics")
	}
}

func restoreLockHooks(t *testing.T) {
	t.Helper()
	origMkdirAll := lockMkdirAll
	origMkdir := lockMkdir
	origRemoveAll := lockRemoveAll
	origReadFile := lockReadFile
	origWriteFile := lockWriteFile
	origStat := lockStat
	origProcessExists := processExists
	t.Cleanup(func() {
		lockMkdirAll = origMkdirAll
		lockMkdir = origMkdir
		lockRemoveAll = origRemoveAll
		lockReadFile = origReadFile
		lockWriteFile = origWriteFile
		lockStat = origStat
		processExists = origProcessExists
	})
}

func requireLockPhase(t *testing.T, err error, phase string) *apperrors.LockError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected LockError phase %q, got nil", phase)
	}
	var lockErr *apperrors.LockError
	if !errors.As(err, &lockErr) {
		t.Fatalf("expected LockError, got %T", err)
	}
	if lockErr.Phase != phase {
		t.Fatalf("LockError phase = %q, want %q", lockErr.Phase, phase)
	}
	return lockErr
}

// ─── New tests ──────────────────────────────────────────────────────────────

func TestNew_PathConstruction(t *testing.T) {
	lk := New("/tmp/locks", "homes")
	expected := filepath.Join("/tmp/locks", "backup-homes.lock.d")
	if lk.Path != expected {
		t.Errorf("Path = %q, want %q", lk.Path, expected)
	}
	if lk.PIDFile != filepath.Join(expected, "pid") {
		t.Errorf("PIDFile = %q", lk.PIDFile)
	}
	if lk.pid != os.Getpid() {
		t.Errorf("pid = %d, want %d", lk.pid, os.Getpid())
	}
}

func TestNew_DifferentLabels(t *testing.T) {
	l1 := New("/tmp/locks", "homes")
	l2 := New("/tmp/locks", "photos")
	if l1.Path == l2.Path {
		t.Error("different labels should produce different paths")
	}
}

// ─── Acquire tests ──────────────────────────────────────────────────────────

func TestAcquire_Success(t *testing.T) {
	dir := t.TempDir()
	lk := New(dir, "test-acquire")

	if err := lk.Acquire(); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	defer lk.Release()

	// Lock directory should exist
	if _, err := os.Stat(lk.Path); err != nil {
		t.Fatalf("lock directory not created: %v", err)
	}

	// PID file should contain our PID
	data, err := os.ReadFile(lk.PIDFile)
	if err != nil {
		t.Fatalf("failed to read PID file: %v", err)
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		t.Fatalf("invalid PID in file: %q", pidStr)
	}
	if pid != os.Getpid() {
		t.Errorf("PID = %d, want %d", pid, os.Getpid())
	}
}

func TestAcquire_BlockedByRunningProcess(t *testing.T) {
	requireLinuxProc(t)

	dir := t.TempDir()

	// First lock: simulate an existing lock held by this process (which is running)
	lk1 := New(dir, "test-blocked")
	if err := lk1.Acquire(); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	defer lk1.Release()

	// Second lock with same label should fail because our process IS running
	lk2 := New(dir, "test-blocked")
	err := lk2.Acquire()
	if err == nil {
		t.Fatal("expected error when lock held by running process")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("error should mention 'already running', got: %v", err)
	}
}

func TestAcquire_StaleLockRemoval(t *testing.T) {
	dir := t.TempDir()
	lk := New(dir, "test-stale")

	// Create a stale lock with a PID that doesn't exist
	os.MkdirAll(lk.Path, 0755)
	// Use a very high PID unlikely to exist
	os.WriteFile(lk.PIDFile, []byte("99999999"), 0644)

	// Acquire should succeed by removing stale lock
	if err := lk.Acquire(); err != nil {
		t.Fatalf("Acquire (stale lock removal) failed: %v", err)
	}
	defer lk.Release()

	// Verify our PID is now in the file
	data, _ := os.ReadFile(lk.PIDFile)
	pidStr := strings.TrimSpace(string(data))
	if pidStr != strconv.Itoa(os.Getpid()) {
		t.Errorf("PID file contains %q, want %q", pidStr, strconv.Itoa(os.Getpid()))
	}
}

func TestAcquire_CreatesParentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "nested")
	lk := New(dir, "test-parent")

	if err := lk.Acquire(); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	defer lk.Release()

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("parent directory not created: %v", err)
	}
}

func TestAcquire_StaleLockNoPIDFile(t *testing.T) {
	dir := t.TempDir()
	lk := New(dir, "test-nopid")

	// Create a lock dir but no PID file (corrupt lock)
	os.MkdirAll(lk.Path, 0755)

	// Should still succeed by removing stale lock
	if err := lk.Acquire(); err != nil {
		t.Fatalf("Acquire (no PID file) failed: %v", err)
	}
	defer lk.Release()
}

func TestAcquire_StaleLockInvalidPID(t *testing.T) {
	dir := t.TempDir()
	lk := New(dir, "test-badpid")

	// Create a lock dir with invalid PID content
	os.MkdirAll(lk.Path, 0755)
	os.WriteFile(lk.PIDFile, []byte("not-a-pid"), 0644)

	// Should succeed by removing stale lock (readPID returns error)
	if err := lk.Acquire(); err != nil {
		t.Fatalf("Acquire (invalid PID) failed: %v", err)
	}
	defer lk.Release()
}

func TestAcquire_CreateParentFailureReturnsLockError(t *testing.T) {
	dir := t.TempDir()
	fileParent := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(fileParent, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	lk := New(filepath.Join(fileParent, "child"), "test-parent-failure")
	err := lk.Acquire()
	if err == nil {
		t.Fatal("expected error when lock parent cannot be created")
	}

	var lockErr *apperrors.LockError
	if !errors.As(err, &lockErr) {
		t.Fatalf("expected LockError, got %T", err)
	}
	if lockErr.Phase != "create-parent" {
		t.Fatalf("LockError phase = %q, want create-parent", lockErr.Phase)
	}
}

func TestAcquire_CreateDirectoryFailureWrapsCause(t *testing.T) {
	restoreLockHooks(t)
	lockMkdir = func(string, os.FileMode) error {
		return os.ErrPermission
	}

	lk := New(t.TempDir(), "test-create-failure")
	err := lk.Acquire()
	requireLockPhase(t, err, "create")
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Acquire() err = %v, want wrapped permission error", err)
	}
}

func TestAcquire_StaleRemoveFailureWrapsCause(t *testing.T) {
	restoreLockHooks(t)
	processExists = func(int) bool { return false }
	lockRemoveAll = func(string) error {
		return os.ErrPermission
	}

	dir := t.TempDir()
	lk := New(dir, "test-stale-remove-failure")
	if err := os.MkdirAll(lk.Path, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(lk.Path) })
	if err := os.WriteFile(lk.PIDFile, []byte("99999999"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := lk.Acquire()
	requireLockPhase(t, err, "stale-remove")
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Acquire() err = %v, want wrapped permission error", err)
	}
}

func TestAcquire_StaleRetryFailureWrapsCause(t *testing.T) {
	restoreLockHooks(t)
	processExists = func(int) bool { return false }
	origMkdir := lockMkdir
	attempts := 0
	lockMkdir = func(path string, perm os.FileMode) error {
		attempts++
		if attempts == 2 {
			return os.ErrPermission
		}
		return origMkdir(path, perm)
	}

	dir := t.TempDir()
	lk := New(dir, "test-stale-retry-failure")
	if err := os.MkdirAll(lk.Path, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(lk.PIDFile, []byte("99999999"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := lk.Acquire()
	requireLockPhase(t, err, "stale-retry")
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Acquire() err = %v, want wrapped permission error", err)
	}
}

func TestAcquire_StaleRetryRaceReportsHeldLock(t *testing.T) {
	restoreLockHooks(t)
	processExists = func(pid int) bool { return pid == os.Getpid() }
	lockRemoveAll = func(path string) error {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(path, "pid"), []byte(strconv.Itoa(os.Getpid())), 0644)
	}

	dir := t.TempDir()
	lk := New(dir, "test-stale-race")
	if err := os.MkdirAll(lk.Path, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(lk.Path) })
	if err := os.WriteFile(lk.PIDFile, []byte("99999999"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := lk.Acquire()
	requireLockPhase(t, err, "held")
	if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("Acquire() err = %v, want already running diagnostic", err)
	}
}

// ─── Release tests ──────────────────────────────────────────────────────────

func TestRelease_RemovesOwnedLock(t *testing.T) {
	dir := t.TempDir()
	lk := New(dir, "test-release")

	if err := lk.Acquire(); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	if !lk.Release() {
		t.Fatal("Release() = false, want true for owned lock")
	}

	if _, err := os.Stat(lk.Path); !os.IsNotExist(err) {
		t.Error("lock directory should be removed after Release")
	}
}

func TestRelease_IgnoresForeignLock(t *testing.T) {
	dir := t.TempDir()
	lk := New(dir, "test-foreign")

	// Create a lock with different PID
	os.MkdirAll(lk.Path, 0755)
	os.WriteFile(lk.PIDFile, []byte("1"), 0644) // PID 1 (init, always running)

	if lk.Release() {
		t.Fatal("Release() = true, want false for foreign lock")
	}

	// Lock should still exist (not ours)
	if _, err := os.Stat(lk.Path); err != nil {
		t.Error("foreign lock should NOT be removed by Release")
	}

	// Cleanup
	os.RemoveAll(lk.Path)
}

func TestRelease_EmptyPath(t *testing.T) {
	lk := &Lock{Path: ""}
	// Should be a no-op, not panic
	if lk.Release() {
		t.Fatal("Release() = true, want false for empty path")
	}
}

func TestRelease_Idempotent(t *testing.T) {
	dir := t.TempDir()
	lk := New(dir, "test-idempotent")

	if err := lk.Acquire(); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	if !lk.Release() {
		t.Fatal("first Release() = false, want true")
	}
	// Second release should be safe
	if !lk.Release() {
		t.Fatal("second Release() = false, want true for idempotent cleanup")
	}
}

func TestInspect_NoLockPresent(t *testing.T) {
	status, err := Inspect(t.TempDir(), "homes")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Present || status.Active || status.Stale {
		t.Fatalf("status = %+v", status)
	}
}

func TestInspect_ActiveAndStale(t *testing.T) {
	requireLinuxProc(t)

	dir := t.TempDir()
	active := New(dir, "active")
	if err := active.Acquire(); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer active.Release()

	status, err := Inspect(dir, "active")
	if err != nil {
		t.Fatalf("Inspect(active) error = %v", err)
	}
	if !status.Present || !status.Active || status.Stale {
		t.Fatalf("active status = %+v", status)
	}

	stale := New(dir, "stale")
	if err := os.MkdirAll(stale.Path, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(stale.PIDFile, []byte("99999999"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	status, err = Inspect(dir, "stale")
	if err != nil {
		t.Fatalf("Inspect(stale) error = %v", err)
	}
	if !status.Present || status.Active || !status.Stale {
		t.Fatalf("stale status = %+v", status)
	}
}

// ─── writePID / readPID tests ───────────────────────────────────────────────

func TestWriteReadPID(t *testing.T) {
	dir := t.TempDir()
	lk := New(dir, "test-pid-rw")
	os.MkdirAll(lk.Path, 0755)

	if err := lk.writePID(); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	pid, err := lk.readPID()
	if err != nil {
		t.Fatalf("readPID failed: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("readPID = %d, want %d", pid, os.Getpid())
	}
}

func TestReadPID_MissingFile(t *testing.T) {
	lk := &Lock{PIDFile: "/nonexistent/pid"}
	_, err := lk.readPID()
	if err == nil {
		t.Fatal("expected error for missing PID file")
	}
}

func TestReadPID_WhitespaceHandling(t *testing.T) {
	dir := t.TempDir()
	lk := New(dir, "test-ws")
	os.MkdirAll(lk.Path, 0755)
	os.WriteFile(lk.PIDFile, []byte("  12345  \n"), 0644)

	pid, err := lk.readPID()
	if err != nil {
		t.Fatalf("readPID failed: %v", err)
	}
	if pid != 12345 {
		t.Errorf("pid = %d, want 12345", pid)
	}
}

// ─── processExists tests ───────────────────────────────────────────────────

func TestProcessExists_CurrentProcess(t *testing.T) {
	requireLinuxProc(t)

	if !processExists(os.Getpid()) {
		t.Error("current process should exist")
	}
}

func TestProcessExists_NonexistentProcess(t *testing.T) {
	// PID 99999999 is unlikely to exist
	if processExists(99999999) {
		t.Error("PID 99999999 should not exist")
	}
}

func TestProcessExists_PID1(t *testing.T) {
	requireLinuxProc(t)

	// PID 1 (init/systemd) should always exist on Linux
	if !processExists(1) {
		t.Error("PID 1 should exist")
	}
}

// ─── Concurrent lock tests ─────────────────────────────────────────────────

func TestConcurrentAcquire_SameLabel(t *testing.T) {
	requireLinuxProc(t)

	dir := t.TempDir()

	// Acquire first lock
	lk1 := New(dir, "concurrent")
	if err := lk1.Acquire(); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	defer lk1.Release()

	// Try to acquire with same label (same process, simulating concurrent run)
	lk2 := New(dir, "concurrent")
	err := lk2.Acquire()
	if err == nil {
		t.Fatal("concurrent acquire with same label should fail")
	}
}

func TestAcquire_DifferentLabelsSucceed(t *testing.T) {
	dir := t.TempDir()

	lk1 := New(dir, "label-a")
	if err := lk1.Acquire(); err != nil {
		t.Fatalf("Acquire label-a failed: %v", err)
	}
	defer lk1.Release()

	lk2 := New(dir, "label-b")
	if err := lk2.Acquire(); err != nil {
		t.Fatalf("Acquire label-b should succeed (different label): %v", err)
	}
	defer lk2.Release()
}
