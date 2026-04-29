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

	if e.plan.Request.Verbose && e.cleanupHasDetails() {
		e.view.PrintPhase("Cleanup")
	}
	e.cleanupSnapshot()
	e.cleanupWorkRoot()
	e.releaseTargetLock()

	if e.report != nil {
		e.report.CompleteRun(e.exitCode, e.report.FailureMessage, e.rt.Now())
	}
	if e.exitCode != 0 {
		e.maybeSendFailureNotification()
	}
	if !e.plan.Request.DryRun {
		if err := updateRunState(e.meta, e.plan, e.report, e.lastBackupRevision); err != nil {
			e.log.Warn("%s", statusLinef("Failed to update local run state: %v", err))
		}
	}
	e.view.PrintCompletion(e.exitCode, e.startedAt)
	e.log.Close()
}

func (e *Executor) cleanupHasDetails() bool {
	if e.plan.Request.NeedsSnapshot {
		if _, err := os.Stat(e.plan.Paths.SnapshotTarget); err == nil {
			return true
		}
	}

	workRoot := e.plan.Paths.WorkRoot
	if e.dup != nil {
		workRoot = e.dup.WorkRoot
	}
	if _, err := os.Stat(workRoot); err == nil {
		return true
	}

	return false
}

func (e *Executor) cleanupSnapshot() {
	if !e.plan.Request.NeedsSnapshot {
		return
	}

	releaseSource := func() {}
	if e.sourceLock != nil {
		if err := e.sourceLock.Acquire(); err != nil {
			e.log.Warn("%s", statusLinef("Failed to acquire source lock for snapshot cleanup: %v", err))
		} else {
			releaseSource = func() { _ = e.sourceLock.Release() }
		}
	}
	defer releaseSource()

	if _, err := os.Stat(e.plan.Paths.SnapshotTarget); err == nil {
		if e.plan.Request.Verbose {
			e.log.PrintLine("Snapshot", e.plan.Paths.SnapshotTarget)
		}
		if e.plan.Request.DryRun {
			e.log.DryRun("%s", e.cmds.SnapshotDelete())
		} else if delErr := btrfs.DeleteSnapshot(e.runner, e.plan.Paths.SnapshotTarget, false); delErr != nil {
			e.log.Warn("%s", statusLinef("Failed to delete subvolume %s: %v", e.plan.Paths.SnapshotTarget, delErr))
		} else if e.plan.Request.Verbose {
			e.log.PrintLine("Snapshot", "Removed")
		}
	}

	if _, err := os.Stat(e.plan.Paths.SnapshotTarget); err == nil {
		if e.plan.Request.Verbose {
			e.log.PrintLine("Snapshot Dir", e.plan.Paths.SnapshotTarget)
		}
		if e.plan.Request.DryRun {
			e.log.DryRun("rm -rf %s", e.plan.Paths.SnapshotTarget)
		} else if rmErr := os.RemoveAll(e.plan.Paths.SnapshotTarget); rmErr != nil {
			e.log.Warn("%s", statusLinef("Failed to remove snapshot directory %s: %v", e.plan.Paths.SnapshotTarget, rmErr))
		}
	}
}

func (e *Executor) cleanupWorkRoot() {
	workRoot := e.plan.Paths.WorkRoot
	if e.dup != nil {
		workRoot = e.dup.WorkRoot
	}
	if _, err := os.Stat(workRoot); err != nil {
		return
	}

	if e.plan.Request.Verbose {
		e.log.PrintLine("Work Dir", workRoot)
	}
	if e.plan.Request.DryRun {
		e.log.DryRun("%s", e.cmds.WorkDirRemove(workRoot))
		return
	}

	if e.dup != nil {
		if err := e.dup.Cleanup(); err != nil {
			e.log.Warn("%s", statusLinef("Failed to remove work directory: %v", err))
		} else if e.plan.Request.Verbose {
			e.log.PrintLine("Work Dir", "Removed")
		}
		return
	}

	if err := os.RemoveAll(workRoot); err != nil {
		e.log.Warn("%s", statusLinef("Failed to remove work directory %s: %v", workRoot, err))
	} else if e.plan.Request.Verbose {
		e.log.PrintLine("Work Dir", "Removed")
	}
}

func (e *Executor) releaseTargetLock() {
	if e.targetLockAcquired && e.targetLock != nil {
		e.targetLock.Release()
	}
}
