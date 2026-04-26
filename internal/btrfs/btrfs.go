// Package btrfs provides helpers for btrfs filesystem operations including
// volume verification and snapshot management.
//
// All external commands are executed via an [exec.Runner] so that callers
// can inject a [exec.MockRunner] in tests.
//
// Functions in this package return structured [errors.SnapshotError] values
// with rich context instead of logging directly.  The coordinator is
// responsible for all operator-facing output.
package btrfs

import (
	"context"
	"fmt"
	"strings"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
)

// CheckFilesystem verifies that a path is on a btrfs filesystem. It does not
// inspect subvolume metadata and is therefore suitable for non-root read-only
// checks such as storage integrity verification.
//
// Returns a [*errors.SnapshotError] on failure with context including the
// checked path.
func CheckFilesystem(runner execpkg.Runner, path string, dryRun bool) error {
	if dryRun {
		return nil
	}

	ctx := context.Background()

	stdout, _, err := runner.Run(ctx, "stat", "-f", "-c", "%T", path)
	if err != nil {
		return apperrors.NewSnapshotError("check-volume", fmt.Errorf("path does not exist or cannot be stat'd: %w", err), "path", path)
	}

	if !strings.Contains(strings.TrimSpace(stdout), "btrfs") {
		return apperrors.NewSnapshotError("check-volume", fmt.Errorf("path is not on a btrfs filesystem"), "path", path, "fstype", strings.TrimSpace(stdout))
	}

	return nil
}

// CheckVolume verifies that a path is on a btrfs filesystem and is a valid
// subvolume.  It executes `stat -f -c %T <path>` and `btrfs subvolume show
// <path>` via the provided [exec.Runner].
//
// Returns a [*errors.SnapshotError] on failure with context including the
// checked path.
func CheckVolume(runner execpkg.Runner, path string, dryRun bool) error {
	if err := CheckFilesystem(runner, path, dryRun); err != nil || dryRun {
		return err
	}

	ctx := context.Background()

	// Check snapshot-readiness. The stat probe above has already confirmed the
	// filesystem type; failures here mean the subvolume metadata probe failed.
	if _, stderr, err := runner.Run(ctx, "btrfs", "subvolume", "show", path); err != nil {
		return apperrors.NewSnapshotError("check-volume", fmt.Errorf("path is on a btrfs filesystem, but subvolume metadata could not be verified: %s", subvolumeProbeDetail(stderr, err)), "path", path)
	}

	return nil
}

func subvolumeProbeDetail(stderr string, err error) string {
	if detail := strings.TrimSpace(stderr); detail != "" {
		return detail
	}
	if err != nil {
		return err.Error()
	}
	return "btrfs subvolume show failed"
}

// CreateSnapshot creates a read-only btrfs snapshot of source at target.
// In dry-run mode no command is executed and nil is returned.
//
// Returns a [*errors.SnapshotError] on failure with context including
// source and target paths.
func CreateSnapshot(runner execpkg.Runner, source, target string, dryRun bool) error {
	if dryRun {
		return nil
	}

	if _, _, err := runner.Run(context.Background(), "btrfs", "subvolume", "snapshot", "-r", source, target); err != nil {
		return apperrors.NewSnapshotError("create", fmt.Errorf("failed to create snapshot: %w", err), "source", source, "target", target)
	}

	return nil
}

// DeleteSnapshot deletes a btrfs subvolume at target.
// In dry-run mode no command is executed and nil is returned.
//
// Returns a [*errors.SnapshotError] on failure with context including the
// target path.
func DeleteSnapshot(runner execpkg.Runner, target string, dryRun bool) error {
	if dryRun {
		return nil
	}

	if _, _, err := runner.Run(context.Background(), "btrfs", "subvolume", "delete", target); err != nil {
		return apperrors.NewSnapshotError("delete", fmt.Errorf("failed to delete subvolume: %w", err), "target", target)
	}

	return nil
}
