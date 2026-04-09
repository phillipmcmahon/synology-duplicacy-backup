package workflow

import (
	"os"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
)

func (e *Executor) cleanup() {
	if e.cleanedUp {
		return
	}
	e.cleanedUp = true

	e.log.Info("%s", statusLinef("Starting cleanup."))
	e.cleanupSnapshot()
	e.cleanupWorkRoot()
	e.releaseLock()

	e.view.PrintCompletion(e.exitCode)
	e.log.Close()
}

func (e *Executor) cleanupSnapshot() {
	if !e.plan.NeedsSnapshot {
		return
	}

	if _, err := os.Stat(e.plan.SnapshotTarget); err == nil {
		e.log.Info("%s", statusLinef("Deleting snapshot subvolume: %s.", e.plan.SnapshotTarget))
		if e.plan.DryRun {
			e.log.DryRun("%s", e.plan.SnapshotDeleteCommand)
		} else if delErr := btrfs.DeleteSnapshot(e.runner, e.plan.SnapshotTarget, false); delErr != nil {
			e.log.Warn("%s", statusLinef("Failed to delete subvolume %s: %v.", e.plan.SnapshotTarget, delErr))
		}
	}

	if _, err := os.Stat(e.plan.SnapshotTarget); err == nil {
		e.log.Info("%s", statusLinef("Removing snapshot directory: %s.", e.plan.SnapshotTarget))
		if e.plan.DryRun {
			e.log.DryRun("rm -rf %s", e.plan.SnapshotTarget)
		} else if rmErr := os.RemoveAll(e.plan.SnapshotTarget); rmErr != nil {
			e.log.Warn("%s", statusLinef("Failed to remove snapshot directory %s: %v.", e.plan.SnapshotTarget, rmErr))
		}
	}
}

func (e *Executor) cleanupWorkRoot() {
	workRoot := e.plan.WorkRoot
	if e.dup != nil {
		workRoot = e.dup.WorkRoot
	}
	if _, err := os.Stat(workRoot); err != nil {
		return
	}

	e.log.Info("%s", statusLinef("Removing duplicacy work directory: %s.", workRoot))
	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.WorkDirRemoveCommand)
		return
	}

	if e.dup != nil {
		if err := e.dup.Cleanup(); err != nil {
			e.log.Warn("%s", statusLinef("Failed to remove work directory: %v.", err))
		}
		return
	}

	if err := os.RemoveAll(workRoot); err != nil {
		e.log.Warn("%s", statusLinef("Failed to remove work directory %s: %v.", workRoot, err))
	}
}

func (e *Executor) releaseLock() {
	if e.lockAcquired {
		e.lock.Release()
	}
}
