package workflow

import (
	"os"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

type Executor struct {
	meta   Metadata
	rt     Runtime
	log    *logger.Logger
	runner execpkg.Runner
	plan   *Plan
	view   *Presenter

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
		view:   NewPresenter(meta, rt, log),
		lock:   rt.NewLock(meta.LockParent, plan.BackupLabel),
	}
}

func (e *Executor) Run() int {
	e.installSignalHandler()
	defer e.cleanup()

	if e.plan.DefaultNotice != "" {
		e.log.Info("%s", statusLinef("%s", e.plan.DefaultNotice))
	}
	e.log.CleanupOldLogs(e.plan.LogRetentionDays, e.plan.DryRun)

	if err := e.acquireLock(); err != nil {
		e.fail(err)
		return e.exitCode
	}

	e.view.PrintHeader(e.lock.Path)
	e.view.PrintSummary(e.plan)

	if err := e.execute(); err != nil {
		e.fail(err)
		return e.exitCode
	}

	e.log.Info("%s", statusLinef("All operations completed."))
	return 0
}

func (e *Executor) installSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	e.rt.SignalNotify(sigChan, SignalSet()...)
	go func() {
		sig := <-sigChan
		e.log.Warn("%s", statusLinef("Received signal: %v — initiating cleanup.", sig))
		e.exitCode = 1
		e.cleanup()
		os.Exit(1)
	}()
}

func (e *Executor) acquireLock() error {
	e.log.Info("%s", statusLinef("Acquiring lock for label %q.", e.plan.BackupLabel))
	if err := e.lock.Acquire(); err != nil {
		return err
	}
	e.lockAcquired = true
	e.log.Info("%s", statusLinef("Lock acquired: %s.", e.lock.Path))
	return nil
}

func (e *Executor) execute() error {
	if e.plan.NeedsDuplicacySetup {
		if err := e.prepareDuplicacySetup(); err != nil {
			return err
		}
	}
	if e.plan.DoBackup {
		if err := e.runBackupPhase(); err != nil {
			return err
		}
	}
	if e.plan.DoPrune {
		if err := e.runPrunePhase(); err != nil {
			return err
		}
	}
	if e.plan.FixPerms {
		if err := e.runFixPermsPhase(); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) prepareDuplicacySetup() error {
	if e.plan.NeedsSnapshot {
		e.log.Info("%s", statusLinef("Creating btrfs snapshot: %s → %s.", e.plan.SnapshotSource, e.plan.SnapshotTarget))
		if e.plan.DryRun {
			e.log.DryRun("%s", e.plan.SnapshotCreateCommand)
		} else if err := btrfs.CreateSnapshot(e.runner, e.plan.SnapshotSource, e.plan.SnapshotTarget, false); err != nil {
			return err
		} else {
			e.log.Info("%s", statusLinef("Snapshot created successfully."))
		}
	}

	dup := duplicacy.NewSetup(e.plan.WorkRoot, e.plan.RepositoryPath, e.plan.BackupTarget, e.plan.DryRun, e.runner)
	if err := dup.CreateDirs(); err != nil {
		return err
	}
	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.WorkDirCreateCommand)
	}

	if err := dup.WritePreferences(e.plan.Secrets); err != nil {
		return err
	}
	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.PreferencesWriteCommand)
	}

	if e.plan.DoBackup && e.plan.Filter != "" {
		e.log.Info("%s", statusLinef("Creating filter definitions."))
		if err := dup.WriteFilters(e.plan.Filter); err != nil {
			return err
		}
		if e.plan.DryRun {
			e.log.DryRun("%s", e.plan.FiltersWriteCommand)
		} else {
			e.log.Info("%s", statusLinef("Active filters:"))
		}
		for _, line := range e.plan.FilterLines {
			e.log.Info("  %s", line)
		}
	}

	if err := dup.SetPermissions(); err != nil {
		return err
	}
	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.WorkDirDirPermsCommand)
		e.log.DryRun("%s", e.plan.WorkDirFilePermsCommand)
	}

	e.log.Info("%s", statusLinef("Changing to directory: %s.", dup.DuplicacyRoot))
	e.dup = dup
	return nil
}

func (e *Executor) runBackupPhase() error {
	e.log.Info("%s", statusLinef("Starting backup phase."))
	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.BackupCommand)
		e.log.Info("%s", statusLinef("Backup phase completed (dry-run)."))
		return nil
	}

	stdout, stderr, err := e.dup.RunBackup(e.plan.Threads)
	e.view.PrintCommandOutput(stdout, stderr)
	if err != nil {
		return err
	}
	e.log.Info("%s", statusLinef("Backup phase completed successfully."))
	return nil
}

func (e *Executor) fail(err error) {
	e.log.Error("%s", OperatorMessage(err))
	e.exitCode = 1
}
