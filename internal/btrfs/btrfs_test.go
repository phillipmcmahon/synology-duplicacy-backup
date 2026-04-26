package btrfs

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
)

// ---------------------------------------------------------------------------
// CheckVolume tests
// ---------------------------------------------------------------------------

func TestCheckFilesystem_SuccessDoesNotRequireSubvolumeMetadata(t *testing.T) {
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
	)

	err := CheckFilesystem(mock, "/volume1/homes", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Cmd != "stat" {
		t.Fatalf("command = %q, want stat", mock.Invocations[0].Cmd)
	}
}

func TestCheckFilesystem_NotBtrfs(t *testing.T) {
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "ext4\n"},
	)

	err := CheckFilesystem(mock, "/volume1/homes", false)
	if err == nil {
		t.Fatal("expected error for non-btrfs filesystem")
	}
	if !strings.Contains(err.Error(), "path is not on a btrfs filesystem") {
		t.Fatalf("error = %q, want filesystem wording", err.Error())
	}
}

func TestCheckVolume_Success(t *testing.T) {
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"}, // stat -f -c %T
		execpkg.MockResult{Stdout: "256\n"},   // stat -c %i
	)

	err := CheckVolume(mock, "/volume1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Invocations) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Cmd != "stat" {
		t.Errorf("first command = %q, want stat", mock.Invocations[0].Cmd)
	}
	if mock.Invocations[1].Cmd != "stat" {
		t.Errorf("second command = %q, want stat", mock.Invocations[1].Cmd)
	}
	if len(mock.Invocations[1].Args) != 3 || mock.Invocations[1].Args[0] != "-c" || mock.Invocations[1].Args[1] != "%i" {
		t.Errorf("second command args = %#v, want stat inode command", mock.Invocations[1].Args)
	}
}

func TestCheckVolume_DryRunSkipsChecks(t *testing.T) {
	mock := execpkg.NewMockRunner()

	err := CheckVolume(mock, "/volume1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Invocations) != 0 {
		t.Fatalf("expected 0 invocations, got %d", len(mock.Invocations))
	}
}

func TestCheckVolume_StatFails(t *testing.T) {
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Err: errors.New("stat failed")},
	)

	err := CheckVolume(mock, "/nonexistent", false)
	if err == nil {
		t.Fatal("expected error when stat fails")
	}
	if len(mock.Invocations) != 1 {
		t.Errorf("should stop after stat failure, got %d invocations", len(mock.Invocations))
	}
	// Verify structured error type
	var snapErr *apperrors.SnapshotError
	if !errors.As(err, &snapErr) {
		t.Errorf("expected *SnapshotError, got %T", err)
	}
}

func TestCheckVolume_NotBtrfs(t *testing.T) {
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "ext4\n"}, // not btrfs
	)

	err := CheckVolume(mock, "/volume1", false)
	if err == nil {
		t.Fatal("expected error for non-btrfs filesystem")
	}
	if len(mock.Invocations) != 1 {
		t.Errorf("should stop after non-btrfs detection, got %d invocations", len(mock.Invocations))
	}
	var snapErr *apperrors.SnapshotError
	if !errors.As(err, &snapErr) {
		t.Errorf("expected *SnapshotError, got %T", err)
	}
}

func TestCheckVolume_NotSubvolume(t *testing.T) {
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "257\n"},
	)

	err := CheckVolume(mock, "/volume1", false)
	if err == nil {
		t.Fatal("expected error for non-subvolume")
	}
	if !strings.Contains(err.Error(), "not a subvolume root") {
		t.Fatalf("error = %q, want subvolume root wording", err.Error())
	}
	if !strings.Contains(err.Error(), "inode 257, expected 256") {
		t.Fatalf("error = %q, want inode detail", err.Error())
	}
}

func TestCheckVolume_InodeStatFails(t *testing.T) {
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Err: errors.New("permission denied")},
	)

	err := CheckVolume(mock, "/volume1", false)
	if err == nil {
		t.Fatal("expected error when inode stat fails")
	}
	if !strings.Contains(err.Error(), "path inode could not be inspected") {
		t.Fatalf("error = %q, want inode inspection wording", err.Error())
	}
}

// ---------------------------------------------------------------------------
// CreateSnapshot tests
// ---------------------------------------------------------------------------

func TestCreateSnapshot_Success(t *testing.T) {
	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "snapshot created\n"})

	err := CreateSnapshot(mock, "/src", "/dst", false)
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
	mock := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("snapshot failed")})

	err := CreateSnapshot(mock, "/src", "/dst", false)
	if err == nil {
		t.Fatal("expected error")
	}
	var snapErr *apperrors.SnapshotError
	if !errors.As(err, &snapErr) {
		t.Errorf("expected *SnapshotError, got %T", err)
	}
}

func TestCreateSnapshot_DryRun(t *testing.T) {
	mock := execpkg.NewMockRunner()

	err := CreateSnapshot(mock, "/src", "/dst", true)
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
	mock := execpkg.NewMockRunner(execpkg.MockResult{})

	err := DeleteSnapshot(mock, "/snap", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.Invocations[0].Cmd != "btrfs" {
		t.Errorf("cmd = %q, want btrfs", mock.Invocations[0].Cmd)
	}
}

func TestDeleteSnapshot_Failure(t *testing.T) {
	mock := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("delete failed")})

	err := DeleteSnapshot(mock, "/snap", false)
	if err == nil {
		t.Fatal("expected error")
	}
	var snapErr *apperrors.SnapshotError
	if !errors.As(err, &snapErr) {
		t.Errorf("expected *SnapshotError, got %T", err)
	}
}

func TestDeleteSnapshot_DryRun(t *testing.T) {
	mock := execpkg.NewMockRunner()

	err := DeleteSnapshot(mock, "/snap", true)
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	if len(mock.Invocations) != 0 {
		t.Error("dry-run should not invoke any commands")
	}
}

// ---------------------------------------------------------------------------
// Structured error context tests
// ---------------------------------------------------------------------------

func TestCheckVolume_ErrorContext(t *testing.T) {
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "ext4\n"},
	)
	err := CheckVolume(mock, "/volume1/test", false)
	var snapErr *apperrors.SnapshotError
	if !errors.As(err, &snapErr) {
		t.Fatalf("expected *SnapshotError, got %T", err)
	}
	if snapErr.Phase != "check-volume" {
		t.Errorf("phase = %q, want check-volume", snapErr.Phase)
	}
	if snapErr.Context["path"] != "/volume1/test" {
		t.Errorf("context path = %q, want /volume1/test", snapErr.Context["path"])
	}
}

func TestCreateSnapshot_ErrorContext(t *testing.T) {
	mock := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("fail")})
	err := CreateSnapshot(mock, "/src", "/dst", false)
	var snapErr *apperrors.SnapshotError
	if !errors.As(err, &snapErr) {
		t.Fatalf("expected *SnapshotError, got %T", err)
	}
	if snapErr.Context["source"] != "/src" {
		t.Errorf("context source = %q", snapErr.Context["source"])
	}
	if snapErr.Context["target"] != "/dst" {
		t.Errorf("context target = %q", snapErr.Context["target"])
	}
}
