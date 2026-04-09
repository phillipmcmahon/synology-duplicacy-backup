package btrfs

import (
	"errors"
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

// ---------------------------------------------------------------------------
// CheckVolume tests
// ---------------------------------------------------------------------------

func TestCheckVolume_Success(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"}, // stat -f -c %T
		execpkg.MockResult{},                  // btrfs subvolume show
	)

	err := CheckVolume(mock, log, "/volume1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Invocations) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Cmd != "stat" {
		t.Errorf("first command = %q, want stat", mock.Invocations[0].Cmd)
	}
	if mock.Invocations[1].Cmd != "btrfs" {
		t.Errorf("second command = %q, want btrfs", mock.Invocations[1].Cmd)
	}
}

func TestCheckVolume_StatFails(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Err: errors.New("stat failed")},
	)

	err := CheckVolume(mock, log, "/nonexistent", false)
	if err == nil {
		t.Fatal("expected error when stat fails")
	}
	if len(mock.Invocations) != 1 {
		t.Errorf("should stop after stat failure, got %d invocations", len(mock.Invocations))
	}
}

func TestCheckVolume_NotBtrfs(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "ext4\n"}, // not btrfs
	)

	err := CheckVolume(mock, log, "/volume1", false)
	if err == nil {
		t.Fatal("expected error for non-btrfs filesystem")
	}
	if len(mock.Invocations) != 1 {
		t.Errorf("should stop after non-btrfs detection, got %d invocations", len(mock.Invocations))
	}
}

func TestCheckVolume_NotSubvolume(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Err: errors.New("not a subvolume")},
	)

	err := CheckVolume(mock, log, "/volume1", false)
	if err == nil {
		t.Fatal("expected error for non-subvolume")
	}
}

// ---------------------------------------------------------------------------
// CreateSnapshot tests
// ---------------------------------------------------------------------------

func TestCreateSnapshot_Success(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "snapshot created\n"})

	err := CreateSnapshot(mock, log, "/src", "/dst", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	inv := mock.Invocations[0]
	if inv.Cmd != "btrfs" {
		t.Errorf("cmd = %q, want btrfs", inv.Cmd)
	}
	wantArgs := []string{"subvolume", "snapshot", "-r", "/src", "/dst"}
	for i, a := range wantArgs {
		if i >= len(inv.Args) || inv.Args[i] != a {
			t.Errorf("args[%d] = %q, want %q", i, inv.Args[i], a)
		}
	}
}

func TestCreateSnapshot_Failure(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("snapshot failed")})

	err := CreateSnapshot(mock, log, "/src", "/dst", false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateSnapshot_DryRun(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner()

	err := CreateSnapshot(mock, log, "/src", "/dst", true)
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	if len(mock.Invocations) != 0 {
		t.Error("dry-run should not invoke any commands")
	}
}

// ---------------------------------------------------------------------------
// DeleteSnapshot tests
// ---------------------------------------------------------------------------

func TestDeleteSnapshot_Success(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(execpkg.MockResult{})

	err := DeleteSnapshot(mock, log, "/snap", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.Invocations[0].Cmd != "btrfs" {
		t.Errorf("cmd = %q, want btrfs", mock.Invocations[0].Cmd)
	}
}

func TestDeleteSnapshot_Failure(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("delete failed")})

	err := DeleteSnapshot(mock, log, "/snap", false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteSnapshot_DryRun(t *testing.T) {
	log := newTestLogger(t)
	mock := execpkg.NewMockRunner()

	err := DeleteSnapshot(mock, log, "/snap", true)
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	if len(mock.Invocations) != 0 {
		t.Error("dry-run should not invoke any commands")
	}
}
