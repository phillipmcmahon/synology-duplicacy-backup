package workflow

import (
	"fmt"
	"os"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/permissions"
)

type Executor struct {
	meta   Metadata
	rt     Runtime
	log    *logger.Logger
	runner execpkg.Runner
	plan   *Plan

	lock         *lock.Lock
	dup          *duplicacy.Setup
	exitCode     int
	cleanedUp    bool
	lockAcquired bool
}

func NewExecutor(meta Metadata, rt Runtime, log *logger.Logger, runner execpkg.Runner, plan *Plan) *Executor {
	return &Executor{
		meta:   meta,
		rt:     rt,
		log:    log,
		runner: runner,
		plan:   plan,
		lock:   rt.NewLock(meta.LockParent, plan.BackupLabel),
	}
}

func (e *Executor) Run() int {
	e.installSignalHandler()
	defer e.cleanup()

	if e.plan.Request.DefaultNotice != "" {
		e.log.Info("%s", e.plan.Request.DefaultNotice)
	}
	e.log.CleanupOldLogs(e.plan.Config.LogRetentionDays, e.plan.Request.DryRun)

	if err := e.acquireLock(); err != nil {
		e.fail(err)
		return e.exitCode
	}

	e.printHeader()
	e.printSummary()

	if err := e.execute(); err != nil {
		e.fail(err)
		return e.exitCode
	}

	e.log.Info("All operations completed.")
	return 0
}

func (e *Executor) installSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	e.rt.SignalNotify(sigChan, SignalSet()...)
	go func() {
		sig := <-sigChan
		e.log.Warn("Received signal: %v — initiating cleanup.", sig)
		e.exitCode = 1
		e.cleanup()
		os.Exit(1)
	}()
}

func (e *Executor) acquireLock() error {
	e.log.Info("Acquiring lock for label %q.", e.plan.BackupLabel)
	if err := e.lock.Acquire(); err != nil {
		return fmt.Errorf("Lock acquisition failed: %w", err)
	}
	e.lockAcquired = true
	e.log.Info("Lock acquired: %s.", e.lock.Path)
	return nil
}

func (e *Executor) printHeader() {
	e.log.PrintSeparator()
	e.log.Info("Backup script started - %s", e.rt.Now().Format("2006-01-02 15:04:05"))
	e.log.PrintLine("Script", e.meta.ScriptName)
	e.log.PrintLine("PID", fmt.Sprintf("%d", e.rt.Getpid()))
	e.log.PrintLine("Lock Path", e.lock.Path)
	e.log.PrintSeparator()
}

func (e *Executor) printSummary() {
	e.log.Info("Configuration Summary:")
	for _, line := range SummaryLines(e.plan) {
		e.log.PrintLine(line.Label, line.Value)
	}
}

func (e *Executor) execute() error {
	if e.plan.Request.DoBackup || e.plan.Request.DoPrune {
		if err := e.prepareDuplicacySetup(); err != nil {
			return err
		}
	}
	if e.plan.Request.DoBackup {
		if err := e.runBackupPhase(); err != nil {
			return err
		}
	}
	if e.plan.Request.DoPrune {
		if err := e.runPrunePhase(); err != nil {
			return err
		}
	}
	if e.plan.Request.FixPerms {
		if err := e.runFixPermsPhase(); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) prepareDuplicacySetup() error {
	if e.plan.Request.DoBackup {
		e.log.Info("Creating btrfs snapshot: %s → %s.", e.plan.SnapshotSource, e.plan.SnapshotTarget)
		if e.plan.Request.DryRun {
			e.log.DryRun("btrfs subvolume snapshot -r %s %s", e.plan.SnapshotSource, e.plan.SnapshotTarget)
		} else if err := btrfs.CreateSnapshot(e.runner, e.plan.SnapshotSource, e.plan.SnapshotTarget, false); err != nil {
			return fmt.Errorf("Failed to create snapshot: %w.", err)
		} else {
			e.log.Info("Snapshot created successfully.")
		}
	}

	dup := duplicacy.NewSetup(e.plan.WorkRoot, e.plan.RepositoryPath, e.plan.BackupTarget, e.plan.Request.DryRun, e.runner)
	if err := dup.CreateDirs(); err != nil {
		return err
	}
	if e.plan.Request.DryRun {
		e.log.DryRun("mkdir -p %s", dup.DuplicacyDir)
	}

	if err := dup.WritePreferences(e.plan.Secrets); err != nil {
		return fmt.Errorf("Failed to write preferences: %w", err)
	}
	if e.plan.Request.DryRun {
		e.log.DryRun("write JSON preferences to %s", dup.PrefsFile)
	}

	if e.plan.Request.DoBackup && e.plan.Config.Filter != "" {
		e.log.Info("Creating filter definitions.")
		if err := dup.WriteFilters(e.plan.Config.Filter); err != nil {
			return err
		}
		if e.plan.Request.DryRun {
			e.log.DryRun("Write filters to %s", dup.FilterFile)
		} else {
			e.log.Info("Active filters:")
		}
		for _, line := range strings.Split(e.plan.Config.Filter, "\n") {
			if line != "" {
				e.log.Info("  %s", line)
			}
		}
	}

	if err := dup.SetPermissions(); err != nil {
		return err
	}
	if e.plan.Request.DryRun {
		e.log.DryRun("find %s -type d -exec chmod 770 {} +", dup.DuplicacyRoot)
		e.log.DryRun("find %s -type f -exec chmod 660 {} +", dup.DuplicacyRoot)
	}

	e.log.Info("Changing to directory: %s.", dup.DuplicacyRoot)
	e.dup = dup
	return nil
}

func (e *Executor) runBackupPhase() error {
	e.log.Info("Starting backup phase.")
	if e.plan.Request.DryRun {
		e.log.DryRun("duplicacy backup -stats -threads %d", e.plan.Config.Threads)
		e.log.Info("Backup phase completed (dry-run).")
		return nil
	}

	stdout, stderr, err := e.dup.RunBackup(e.plan.Config.Threads)
	e.printCommandOutput(stdout, stderr)
	if err != nil {
		return fmt.Errorf("Backup failed: %w.", err)
	}
	e.log.Info("Backup phase completed successfully.")
	return nil
}

func (e *Executor) runPrunePhase() error {
	e.log.Info("Starting prune phase.")

	if e.plan.Request.DryRun {
		e.log.DryRun("duplicacy list -files")
	} else {
		if err := e.dup.ValidateRepo(); err != nil {
			return fmt.Errorf("Cannot perform prune operation — repository not ready: %w.", err)
		}
		e.log.Info("Duplicacy repository validated.")
	}

	if e.plan.Request.DryRun {
		e.log.DryRun("duplicacy prune %s -dry-run", strings.Join(e.plan.Config.PruneArgs, " "))
	}
	preview, err := e.dup.SafePrunePreview(e.plan.Config.PruneArgs, e.plan.Config.SafePruneMinTotalForPercent)
	if err != nil {
		return fmt.Errorf("Safe prune preview failed: %w.", err)
	}

	if preview.Output != "" {
		for _, line := range strings.Split(preview.Output, "\n") {
			if line != "" {
				e.log.Info("[SAFE-PRUNE-PREVIEW] %s", line)
			}
		}
	}
	if preview.RevisionOutput != "" {
		for _, line := range strings.Split(preview.RevisionOutput, "\n") {
			if line != "" {
				e.log.Info("[REVISION-LIST] %s", line)
			}
		}
	}

	if preview.RevisionCountFailed {
		if e.plan.Request.ForcePrune {
			e.log.Warn("Revision count failed; proceeding because --force-prune was supplied (percentage threshold not enforced).")
		} else {
			return fmt.Errorf("Revision count is required for safe prune but failed; use --force-prune to override.")
		}
	}

	e.log.PrintLine("Preview Deletes", fmt.Sprintf("%d", preview.DeleteCount))
	e.log.PrintLine("Preview Total Revs", fmt.Sprintf("%d", preview.TotalRevisions))
	if preview.PercentEnforced {
		e.log.PrintLine("Preview Delete %", fmt.Sprintf("%d", preview.DeletePercent))
	} else {
		e.log.PrintLine("Preview Delete %", fmt.Sprintf("<not enforced; total revisions unavailable or below %d>", e.plan.Config.SafePruneMinTotalForPercent))
	}

	blocked := false
	if preview.DeleteCount > e.plan.Config.SafePruneMaxDeleteCount {
		e.log.Error("Safe prune preview exceeds delete count threshold: %d > %d.", preview.DeleteCount, e.plan.Config.SafePruneMaxDeleteCount)
		blocked = true
	}
	if preview.ExceedsPercent(e.plan.Config.SafePruneMaxDeletePercent) {
		e.log.Error("Safe prune preview exceeds delete percentage threshold (%d of %d revisions > %d%%).", preview.DeleteCount, preview.TotalRevisions, e.plan.Config.SafePruneMaxDeletePercent)
		blocked = true
	}
	if blocked {
		if e.plan.Request.ForcePrune {
			e.log.Warn("Proceeding despite safe prune threshold breach because --force-prune was supplied.")
		} else {
			return fmt.Errorf("Refusing to continue because safe prune thresholds were exceeded.")
		}
	}

	e.log.Info("Starting policy prune.")
	if e.plan.Request.DryRun {
		e.log.DryRun("duplicacy prune %s", strings.Join(e.plan.Config.PruneArgs, " "))
	} else {
		stdout, stderr, err := e.dup.RunPrune(e.plan.Config.PruneArgs)
		e.printCommandOutput(stdout, stderr)
		if err != nil {
			return fmt.Errorf("Policy prune failed: %w.", err)
		}
	}
	e.log.Info("Policy prune completed.")

	if e.plan.Request.DeepPruneMode {
		e.log.Warn("Starting deep prune maintenance step: duplicacy prune -exhaustive -exclusive.")
		if e.plan.Request.DryRun {
			e.log.DryRun("duplicacy prune -exhaustive -exclusive")
		} else {
			stdout, stderr, err := e.dup.RunDeepPrune()
			e.printCommandOutput(stdout, stderr)
			if err != nil {
				return fmt.Errorf("Deep prune failed: %w.", err)
			}
		}
		e.log.Info("Deep prune completed.")
	}

	e.log.Info("Prune phase completed successfully.")
	return nil
}

func (e *Executor) runFixPermsPhase() error {
	e.log.Info("Starting permission normalisation on %s.", e.plan.BackupTarget)
	e.log.PrintLine("Fix Perms Path", e.plan.BackupTarget)
	e.log.PrintLine("Fix Perms Owner", e.plan.Config.LocalOwner)
	e.log.PrintLine("Fix Perms Group", e.plan.Config.LocalGroup)

	if e.plan.Request.DryRun {
		ownerGroup := fmt.Sprintf("%s:%s", e.plan.Config.LocalOwner, e.plan.Config.LocalGroup)
		e.log.DryRun("chown -R %s %s", ownerGroup, e.plan.BackupTarget)
		e.log.DryRun("find %s -type d -exec chmod 770 {} +", e.plan.BackupTarget)
		e.log.DryRun("find %s -type f -exec chmod 660 {} +", e.plan.BackupTarget)
		e.log.Info("Permission normalisation completed (dry-run).")
		return nil
	}

	if err := permissions.Fix(e.runner, e.plan.BackupTarget, e.plan.Config.LocalOwner, e.plan.Config.LocalGroup, false); err != nil {
		return fmt.Errorf("Permission normalisation failed: %w.", err)
	}
	e.log.Info("Permission normalisation completed successfully.")
	return nil
}

func (e *Executor) cleanup() {
	if e.cleanedUp {
		return
	}
	e.cleanedUp = true

	e.log.Info("Starting cleanup.")

	if e.plan.Request.DoBackup {
		if _, err := os.Stat(e.plan.SnapshotTarget); err == nil {
			e.log.Info("Deleting snapshot subvolume: %s.", e.plan.SnapshotTarget)
			if e.plan.Request.DryRun {
				e.log.DryRun("btrfs subvolume delete %s", e.plan.SnapshotTarget)
			} else if delErr := btrfs.DeleteSnapshot(e.runner, e.plan.SnapshotTarget, false); delErr != nil {
				e.log.Warn("Failed to delete subvolume %s: %v.", e.plan.SnapshotTarget, delErr)
			}
		}

		if _, err := os.Stat(e.plan.SnapshotTarget); err == nil {
			e.log.Info("Removing snapshot directory: %s.", e.plan.SnapshotTarget)
			if e.plan.Request.DryRun {
				e.log.DryRun("rm -rf %s", e.plan.SnapshotTarget)
			} else if rmErr := os.RemoveAll(e.plan.SnapshotTarget); rmErr != nil {
				e.log.Warn("Failed to remove snapshot directory %s.", e.plan.SnapshotTarget)
			}
		}
	}

	if e.dup != nil {
		e.log.Info("Removing duplicacy work directory: %s.", e.dup.WorkRoot)
		if e.plan.Request.DryRun {
			e.log.DryRun("rm -rf %s", e.dup.WorkRoot)
		} else if err := e.dup.Cleanup(); err != nil {
			e.log.Warn("Failed to remove work directory: %v.", err)
		}
	} else if _, err := os.Stat(e.plan.WorkRoot); err == nil {
		e.log.Info("Removing duplicacy work directory: %s.", e.plan.WorkRoot)
		if e.plan.Request.DryRun {
			e.log.DryRun("rm -rf %s", e.plan.WorkRoot)
		} else if rmErr := os.RemoveAll(e.plan.WorkRoot); rmErr != nil {
			e.log.Warn("Failed to remove work directory %s: %v.", e.plan.WorkRoot, rmErr)
		}
	}

	if e.lockAcquired {
		e.lock.Release()
	}

	status := "SUCCESS"
	if e.exitCode != 0 {
		status = "FAILED"
	}
	e.log.PrintSeparator()
	e.log.Info("Backup script completed:")
	e.log.PrintLine("Result", e.log.FormatResult(status))
	e.log.PrintLine("Code", fmt.Sprintf("%d", e.exitCode))
	e.log.PrintLine("Timestamp", e.rt.Now().Format("2006-01-02 15:04:05"))
	e.log.PrintSeparator()
	e.log.Close()
}

func (e *Executor) fail(err error) {
	e.log.Error("%v", err)
	e.exitCode = 1
}

func (e *Executor) printCommandOutput(stdout, stderr string) {
	if stdout != "" {
		for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
			if line != "" {
				e.log.Info("%s", line)
			}
		}
	}
	if stderr != "" {
		for _, line := range strings.Split(strings.TrimRight(stderr, "\n"), "\n") {
			if line != "" {
				e.log.Warn("%s", line)
			}
		}
	}
}
