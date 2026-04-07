// Package btrfs provides helpers for btrfs filesystem operations including
// volume verification and snapshot management.
package btrfs

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

// CheckVolume verifies that a path is on a btrfs filesystem and is a valid subvolume.
func CheckVolume(log *logger.Logger, path string, dryRun bool) error {
	if dryRun {
		log.DryRun("check btrfs volume: %s", path)
		return nil
	}

	// Check filesystem type
	out, err := exec.Command("stat", "-f", "-c", "%T", path).Output()
	if err != nil {
		return fmt.Errorf("path '%s' does not exist or cannot be stat'd: %w", path, err)
	}

	if !strings.Contains(strings.TrimSpace(string(out)), "btrfs") {
		return fmt.Errorf("'%s' is not on a btrfs filesystem", path)
	}

	// Check it's a subvolume
	if err := exec.Command("btrfs", "subvolume", "show", path).Run(); err != nil {
		return fmt.Errorf("'%s' is not a btrfs volume or subvolume", path)
	}

	log.Info("Verified '%s' is on a btrfs filesystem and btrfs commands work", path)
	return nil
}

// CreateSnapshot creates a read-only btrfs snapshot.
func CreateSnapshot(log *logger.Logger, source, target string, dryRun bool) error {
	if dryRun {
		log.DryRun("btrfs subvolume snapshot -r %s %s", source, target)
		return nil
	}

	log.Info("Creating snapshot target: %s from: %s", target, source)
	cmd := exec.Command("btrfs", "subvolume", "snapshot", "-r", source, target)
	cmd.Stdout = log.Writer(logger.INFO, "[BTRFS] ")
	cmd.Stderr = log.Writer(logger.ERROR, "[BTRFS] ")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	return nil
}

// DeleteSnapshot deletes a btrfs subvolume.
func DeleteSnapshot(log *logger.Logger, target string, dryRun bool) error {
	if dryRun {
		log.DryRun("btrfs subvolume delete %s", target)
		return nil
	}

	cmd := exec.Command("btrfs", "subvolume", "delete", target)
	cmd.Stdout = log.Writer(logger.INFO, "[BTRFS] ")
	cmd.Stderr = log.Writer(logger.ERROR, "[BTRFS] ")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete subvolume %s: %w", target, err)
	}

	return nil
}
