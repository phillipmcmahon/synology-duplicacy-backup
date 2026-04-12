package workflow

import (
	"fmt"
	"os"
	"strconv"
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

	sourceLock         *lock.Lock
	targetLock         *lock.Lock
	dup                *duplicacy.Setup
	report             *RunReport
	startedAt          time.Time
	exitCode           int
	cleanedUp          bool
	targetLockAcquired bool
	lastBackupRevision int
}

func NewExecutor(meta Metadata, rt Runtime, log *logger.Logger, runner execpkg.Runner, plan *Plan) *Executor {
	startedAt := rt.Now()
	targetLockKey := targetRepositoryLockKey(plan.BackupLabel, plan.TargetName())
	sourceLock := rt.NewLock(meta.LockParent, "source-"+plan.BackupLabel)
	if rt.NewSourceLock != nil {
		sourceLock = rt.NewSourceLock(meta.LockParent, plan.BackupLabel)
	}
	return &Executor{
		meta:       meta,
		rt:         rt,
		log:        log,
		runner:     runner,
		plan:       plan,
		view:       NewPresenter(meta, rt, log, plan.Verbose),
		sourceLock: sourceLock,
		targetLock: rt.NewLock(meta.LockParent, targetLockKey),
		startedAt:  startedAt,
		report:     NewRunReport(plan, startedAt),
	}
}

func (e *Executor) Run() int {
	e.installSignalHandler()
	defer e.cleanup()

	e.log.CleanupOldLogs(e.plan.LogRetentionDays, e.plan.DryRun)
	if err := e.confirmSafetyRails(); err != nil {
		e.view.PrintPreRunFailurePlan(e.plan)
		e.fail(err)
		return e.exitCode
	}

	if err := e.acquireTargetLock(); err != nil {
		e.view.PrintPreRunFailurePlan(e.plan)
		e.fail(err)
		return e.exitCode
	}

	e.startVisibleRun()
	e.view.PrintHeader(e.plan, e.startedAt, e.targetLock.Path)
	if e.plan.Verbose {
		e.view.PrintSummary(e.plan)
	}

	if err := e.execute(); err != nil {
		e.fail(err)
		return e.exitCode
	}

	return 0
}

func (e *Executor) startVisibleRun() {
	e.startedAt = e.rt.Now()
	if e.report != nil {
		e.report.ResetStart(e.startedAt)
	}
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

func (e *Executor) acquireTargetLock() error {
	if err := e.targetLock.Acquire(); err != nil {
		return err
	}
	e.targetLockAcquired = true
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
			if e.plan.DryRun {
				if err := e.runTrackedPhase("Setup", e.runSetupPhase); err != nil {
					return err
				}
			} else {
				if err := e.prepareDuplicacySetup(); err != nil {
					return err
				}
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

func (e *Executor) runSetupPhase() error {
	e.view.PrintPhase("Setup")
	if err := e.prepareDuplicacySetup(); err != nil {
		return err
	}
	e.log.Info("%s", statusLinef("Setup phase completed (dry-run)"))
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
		if err := e.withSourceLock(func() error {
			if e.plan.DryRun {
				e.log.DryRun("%s", e.plan.SnapshotCreateCommand)
				return nil
			}
			if err := btrfs.CreateSnapshot(e.runner, e.plan.SnapshotSource, e.plan.SnapshotTarget, false); err != nil {
				return err
			}
			e.log.PrintLine("Snapshot", e.plan.SnapshotTarget)
			return nil
		}); err != nil {
			return err
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
	if match := backupRevisionPattern.FindStringSubmatch(stdout); len(match) > 1 {
		if revision, convErr := strconv.Atoi(match[1]); convErr == nil {
			e.lastBackupRevision = revision
		}
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

func (e *Executor) withSourceLock(fn func() error) error {
	if e.sourceLock == nil {
		return fn()
	}
	if err := e.sourceLock.Acquire(); err != nil {
		return err
	}
	defer e.sourceLock.Release()
	return fn()
}

func targetRepositoryLockKey(label, target string) string {
	return fmt.Sprintf("%s-%s", label, target)
}
