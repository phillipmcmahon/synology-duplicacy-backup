package workflow

import (
	"os"
	"time"

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
	report       *RunReport
	startedAt    time.Time
	exitCode     int
	cleanedUp    bool
	lockAcquired bool
}

func NewExecutor(meta Metadata, rt Runtime, log *logger.Logger, runner execpkg.Runner, plan *Plan) *Executor {
	startedAt := rt.Now()
	return &Executor{
		meta:      meta,
		rt:        rt,
		log:       log,
		runner:    runner,
		plan:      plan,
		view:      NewPresenter(meta, rt, log, plan.Verbose),
		lock:      rt.NewLock(meta.LockParent, plan.BackupLabel),
		startedAt: startedAt,
		report:    NewRunReport(plan, startedAt),
	}
}

func (e *Executor) Run() int {
	e.installSignalHandler()
	defer e.cleanup()

	e.log.CleanupOldLogs(e.plan.LogRetentionDays, e.plan.DryRun)
	if err := e.confirmSafetyRails(); err != nil {
		e.fail(err)
		return e.exitCode
	}

	if err := e.acquireLock(); err != nil {
		e.fail(err)
		return e.exitCode
	}

	e.view.PrintHeader(e.plan, e.lock.Path)
	if e.plan.Verbose {
		e.view.PrintSummary(e.plan)
	}

	if err := e.execute(); err != nil {
		e.fail(err)
		return e.exitCode
	}

	return 0
}

func (e *Executor) installSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	e.rt.SignalNotify(sigChan, SignalSet()...)
	go func() {
		sig := <-sigChan
		e.log.Warn("%s", statusLinef("Run interrupted by signal %v; starting cleanup", sig))
		e.exitCode = 1
		e.cleanup()
		os.Exit(1)
	}()
}

func (e *Executor) acquireLock() error {
	if err := e.lock.Acquire(); err != nil {
		return err
	}
	e.lockAcquired = true
	return nil
}

func (e *Executor) confirmSafetyRails() error {
	stdin := e.rt.Stdin()
	if !shouldPromptForSafety(e.plan, e.log.Interactive(), e.rt.StdinIsTTY()) {
		return nil
	}

	for _, warning := range safetyWarnings(e.plan) {
		e.log.Warn("%s", statusLinef("%s", warning))
	}

	confirmed, err := e.log.Confirm("Continue with requested operations? [y/N]:", stdin)
	if err != nil {
		return NewMessageError("Could not read confirmation from the terminal")
	}
	if !confirmed {
		return NewMessageError("Operation cancelled at the interactive safety prompt")
	}
	return nil
}

func (e *Executor) execute() error {
	if e.plan.NeedsDuplicacySetup {
		if e.plan.DoBackup {
			e.view.PrintPhase("Backup")
			if err := e.runTrackedPhase("Backup", func() error {
				if err := e.prepareDuplicacySetup(); err != nil {
					return err
				}
				return e.runBackupPhase()
			}); err != nil {
				return err
			}
		} else {
			if err := e.prepareDuplicacySetup(); err != nil {
				return err
			}
		}
	}
	if e.plan.DoPrune {
		if err := e.runTrackedPhase("Prune", e.runPrunePhase); err != nil {
			return err
		}
	}
	if e.plan.DoCleanupStore {
		if err := e.runTrackedPhase("Storage cleanup", e.runCleanupStoragePhase); err != nil {
			return err
		}
	}
	if e.plan.FixPerms {
		if err := e.runTrackedPhase("Fix permissions", e.runFixPermsPhase); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) runTrackedPhase(name string, fn func() error) error {
	index := -1
	if e.report != nil {
		index = e.report.StartPhase(name, e.rt.Now())
	}
	err := fn()
	if e.report != nil {
		result := "success"
		if err != nil {
			result = "failed"
		}
		e.report.CompletePhase(index, result, e.rt.Now())
	}
	return err
}

func (e *Executor) prepareDuplicacySetup() error {
	if e.plan.NeedsSnapshot {
		if e.plan.DryRun {
			e.log.DryRun("%s", e.plan.SnapshotCreateCommand)
		} else if err := btrfs.CreateSnapshot(e.runner, e.plan.SnapshotSource, e.plan.SnapshotTarget, false); err != nil {
			return err
		} else {
			e.log.PrintLine("Snapshot", e.plan.SnapshotTarget)
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
		if err := dup.WriteFilters(e.plan.Filter); err != nil {
			return err
		}
		if e.plan.DryRun {
			e.log.DryRun("%s", e.plan.FiltersWriteCommand)
		}
		if e.plan.Verbose {
			for _, line := range e.plan.FilterLines {
				e.log.PrintLine("Filter", line)
			}
		}
	}

	if err := dup.SetPermissions(); err != nil {
		return err
	}
	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.WorkDirDirPermsCommand)
		e.log.DryRun("%s", e.plan.WorkDirFilePermsCommand)
	}

	if e.plan.Verbose {
		e.log.PrintLine("Directory", dup.DuplicacyRoot)
	}
	e.dup = dup
	return nil
}

func (e *Executor) runBackupPhase() error {
	start := e.rt.Now()
	stopBackup := e.view.StartStatusActivity("Backing up snapshot")

	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.BackupCommand)
		stopBackup()
		e.log.Info("%s", statusLinef("Backup phase completed (dry-run)"))
		return nil
	}

	stdout, stderr, err := e.dup.RunBackup(e.plan.Threads)
	stopBackup()
	e.view.PrintBackupResult(stdout, stderr, err != nil)
	if err != nil {
		return err
	}
	if e.plan.Verbose {
		e.view.PrintDuration(start)
	}
	e.log.Info("%s", statusLinef("Backup phase completed successfully"))
	return nil
}

func (e *Executor) fail(err error) {
	e.log.Error("%s", OperatorMessage(err))
	e.exitCode = 1
	if e.report != nil {
		e.report.FailureMessage = OperatorMessage(err)
	}
}

func (e *Executor) Report() *RunReport {
	return e.report
}
