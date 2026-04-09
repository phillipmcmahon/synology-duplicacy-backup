// Package btrfs provides helpers for btrfs filesystem operations including
// volume verification and snapshot management.
//
// All external commands are executed via an [exec.Runner] so that callers
// can inject a [exec.MockRunner] in tests.
package btrfs

import (
	"context"
	"fmt"
	"strings"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

// CheckVolume verifies that a path is on a btrfs filesystem and is a valid subvolume.
// It executes `stat -f -c %T <path>` and `btrfs subvolume show <path>` via the
// provided [exec.Runner].
func CheckVolume(runner execpkg.Runner, log *logger.Logger, path string, dryRun bool) error {
	ctx := context.Background()

	// Check filesystem type
	stdout, _, err := runner.Run(ctx, "stat", "-f", "-c", "%T", path)
	if err != nil {
		return fmt.Errorf("path '%s' does not exist or cannot be stat'd: %w", path, err)
	}

	if !strings.Contains(strings.TrimSpace(stdout), "btrfs") {
		return fmt.Errorf("'%s' is not on a btrfs filesystem", path)
	}

	// Check it's a subvolume
	if _, _, err := runner.Run(ctx, "btrfs", "subvolume", "show", path); err != nil {
		return fmt.Errorf("'%s' is not a btrfs volume or subvolume", path)
	}

	log.Info("Verified '%s' is on a btrfs filesystem and btrfs commands work", path)
	return nil
}

// CreateSnapshot creates a read-only btrfs snapshot of source at target.
// In dry-run mode the command is logged but not executed.
func CreateSnapshot(runner execpkg.Runner, log *logger.Logger, source, target string, dryRun bool) error {
	if dryRun {
		log.DryRun("btrfs subvolume snapshot -r %s %s", source, target)
		return nil
	}

	log.Info("Creating snapshot target: %s from: %s", target, source)
	stdout, stderr, err := runner.Run(context.Background(), "btrfs", "subvolume", "snapshot", "-r", source, target)
	if stdout != "" {
		fmt.Print(stdout)
	}
	if stderr != "" {
		fmt.Print(stderr)
	}
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	return nil
}

// DeleteSnapshot deletes a btrfs subvolume at target.
// In dry-run mode the command is logged but not executed.
func DeleteSnapshot(runner execpkg.Runner, log *logger.Logger, target string, dryRun bool) error {
	if dryRun {
		log.DryRun("btrfs subvolume delete %s", target)
		return nil
	}

	stdout, stderr, err := runner.Run(context.Background(), "btrfs", "subvolume", "delete", target)
	if stdout != "" {
		fmt.Print(stdout)
	}
	if stderr != "" {
		fmt.Print(stderr)
	}
	if err != nil {
		return fmt.Errorf("failed to delete subvolume %s: %w", target, err)
	}

	return nil
}
