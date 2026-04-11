package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func newPhaseExecutor(t *testing.T, plan *Plan, runner *execpkg.MockRunner) (*Executor, string) {
	t.Helper()
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	rt := testRuntime()
	rt.SignalNotify = func(chan<- os.Signal, ...os.Signal) {}
	rt.NewLock = func(_, label string) *lock.Lock { return lock.New(t.TempDir(), label) }
	rt.NewSourceLock = func(_, label string) *lock.Lock { return lock.NewSource(t.TempDir(), label) }
	executor := NewExecutor(DefaultMetadata("duplicacy-backup", "1.0.0", "now", logDir), rt, log, runner, plan)
	executor.dup = duplicacy.NewSetup(plan.WorkRoot, plan.RepositoryPath, plan.BackupTarget, plan.DryRun, runner)
	return executor, logDir
}

func TestPruneAndCleanupStoragePhases(t *testing.T) {
	t.Run("prune dry-run", func(t *testing.T) {
		plan := &Plan{
			DryRun:                      true,
			Verbose:                     true,
			WorkRoot:                    t.TempDir(),
			DuplicacyRoot:               filepath.Join(t.TempDir(), "duplicacy"),
			RepositoryPath:              "/volume1/homes",
			BackupTarget:                "/backups/homes",
			ValidateRepoCommand:         "duplicacy list -files",
			PrunePreviewCommand:         "duplicacy prune -dry-run",
			PolicyPruneCommand:          "duplicacy prune",
			SafePruneMaxDeleteCount:     25,
			SafePruneMaxDeletePercent:   10,
			SafePruneMinTotalForPercent: 20,
		}
		executor, logDir := newPhaseExecutor(t, plan, execpkg.NewMockRunner())
		if err := executor.runPrunePhase(); err != nil {
			t.Fatalf("runPrunePhase() error = %v", err)
		}
		executor.log.Close()
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Phase: Prune", "Dry run", "duplicacy list -files", "Prune phase completed (dry-run)"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})

	t.Run("prune success", func(t *testing.T) {
		plan := &Plan{
			Verbose:                     true,
			WorkRoot:                    t.TempDir(),
			DuplicacyRoot:               filepath.Join(t.TempDir(), "duplicacy"),
			RepositoryPath:              "/volume1/homes",
			BackupTarget:                "/backups/homes",
			PruneArgs:                   []string{"-keep", "0:365"},
			PruneArgsDisplay:            "-keep 0:365",
			ValidateRepoCommand:         "duplicacy list -files",
			PrunePreviewCommand:         "duplicacy prune -keep 0:365 -dry-run",
			PolicyPruneCommand:          "duplicacy prune -keep 0:365",
			SafePruneMaxDeleteCount:     25,
			SafePruneMaxDeletePercent:   10,
			SafePruneMinTotalForPercent: 20,
		}
		var revisionLines strings.Builder
		for i := 1; i <= 20; i++ {
			revisionLines.WriteString("Snapshot homes at revision ")
			revisionLines.WriteString(strconv.Itoa(i))
			revisionLines.WriteByte('\n')
		}
		runner := execpkg.NewMockRunner(
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "Deleting snapshot at revision 1\n"},
			execpkg.MockResult{Stdout: revisionLines.String()},
			execpkg.MockResult{Stdout: "prune complete\n"},
		)
		executor, logDir := newPhaseExecutor(t, plan, runner)
		if err := executor.runPrunePhase(); err != nil {
			t.Fatalf("runPrunePhase() error = %v", err)
		}
		executor.log.Close()
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Repository", "Validated", "Preview Deletes", "Preview Delete %", "5", "Duration", "Prune phase completed successfully"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})

	t.Run("storage cleanup dry-run", func(t *testing.T) {
		plan := &Plan{
			DryRun:                true,
			WorkRoot:              t.TempDir(),
			DuplicacyRoot:         filepath.Join(t.TempDir(), "duplicacy"),
			RepositoryPath:        "/volume1/homes",
			BackupTarget:          "/backups/homes",
			ValidateRepoCommand:   "duplicacy list -files",
			CleanupStorageCommand: "duplicacy prune -exhaustive -exclusive",
		}
		executor, logDir := newPhaseExecutor(t, plan, execpkg.NewMockRunner())
		if err := executor.runCleanupStoragePhase(); err != nil {
			t.Fatalf("runCleanupStoragePhase() error = %v", err)
		}
		executor.log.Close()
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Phase: Storage cleanup", "duplicacy prune -exhaustive -exclusive", "Storage cleanup phase completed (dry-run)"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})

	t.Run("storage cleanup success", func(t *testing.T) {
		plan := &Plan{
			Verbose:               true,
			WorkRoot:              t.TempDir(),
			DuplicacyRoot:         filepath.Join(t.TempDir(), "duplicacy"),
			RepositoryPath:        "/volume1/homes",
			BackupTarget:          "/backups/homes",
			ValidateRepoCommand:   "duplicacy list -files",
			CleanupStorageCommand: "duplicacy prune -exhaustive -exclusive",
		}
		runner := execpkg.NewMockRunner(
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "Repository set to /volume1/homes\n"},
		)
		executor, logDir := newPhaseExecutor(t, plan, runner)
		if err := executor.runCleanupStoragePhase(); err != nil {
			t.Fatalf("runCleanupStoragePhase() error = %v", err)
		}
		executor.log.Close()
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Phase: Storage cleanup", "Repository", "Validated", "Output", "Duration", "Storage cleanup phase completed successfully"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})
}

func TestCleanupHelpers(t *testing.T) {
	t.Run("no details when nothing exists", func(t *testing.T) {
		plan := &Plan{
			NeedsSnapshot:  true,
			SnapshotTarget: filepath.Join(t.TempDir(), "missing-snapshot"),
			WorkRoot:       filepath.Join(t.TempDir(), "missing-work"),
		}
		executor, _ := newPhaseExecutor(t, plan, execpkg.NewMockRunner())
		if executor.cleanupHasDetails() {
			t.Fatal("cleanupHasDetails() = true, want false")
		}
	})

	t.Run("snapshot dry-run keeps target", func(t *testing.T) {
		snapshotTarget := filepath.Join(t.TempDir(), "snap")
		if err := os.MkdirAll(snapshotTarget, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		plan := &Plan{
			NeedsSnapshot:         true,
			DryRun:                true,
			Verbose:               true,
			SnapshotTarget:        snapshotTarget,
			SnapshotDeleteCommand: "btrfs subvolume delete " + snapshotTarget,
		}
		executor, logDir := newPhaseExecutor(t, plan, execpkg.NewMockRunner())
		executor.cleanupSnapshot()
		executor.log.Close()
		if _, err := os.Stat(snapshotTarget); err != nil {
			t.Fatalf("snapshotTarget should still exist in dry-run: %v", err)
		}
		output := readSingleLogFile(t, logDir)
		if !strings.Contains(output, "Snapshot") || !strings.Contains(output, "Dry run") {
			t.Fatalf("output = %q", output)
		}
	})

	t.Run("snapshot and workdir cleanup remove paths", func(t *testing.T) {
		snapshotTarget := filepath.Join(t.TempDir(), "snap")
		workRoot := filepath.Join(t.TempDir(), "work")
		if err := os.MkdirAll(snapshotTarget, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.MkdirAll(workRoot, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		plan := &Plan{
			NeedsSnapshot:         true,
			Verbose:               true,
			SnapshotTarget:        snapshotTarget,
			SnapshotDeleteCommand: "btrfs subvolume delete " + snapshotTarget,
			WorkRoot:              workRoot,
			WorkDirRemoveCommand:  "rm -rf " + workRoot,
		}
		executor, logDir := newPhaseExecutor(t, plan, execpkg.NewMockRunner(execpkg.MockResult{}))
		if !executor.cleanupHasDetails() {
			t.Fatal("cleanupHasDetails() = false, want true")
		}
		executor.cleanupSnapshot()
		executor.cleanupWorkRoot()
		executor.log.Close()
		if _, err := os.Stat(snapshotTarget); !os.IsNotExist(err) {
			t.Fatalf("snapshotTarget should be removed, err = %v", err)
		}
		if _, err := os.Stat(workRoot); !os.IsNotExist(err) {
			t.Fatalf("workRoot should be removed, err = %v", err)
		}
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Snapshot", "Removed", "Work Dir"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})

	t.Run("dry-run cleanup uses configured command", func(t *testing.T) {
		workRoot := filepath.Join(t.TempDir(), "work")
		if err := os.MkdirAll(workRoot, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		plan := &Plan{
			DryRun:               true,
			Verbose:              true,
			WorkRoot:             workRoot,
			WorkDirRemoveCommand: "rm -rf " + workRoot,
		}
		executor, logDir := newPhaseExecutor(t, plan, execpkg.NewMockRunner())
		executor.cleanupWorkRoot()
		executor.log.Close()

		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Work Dir", "Dry run", "rm -rf " + workRoot} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})
}

func TestRunFixPermsPhase(t *testing.T) {
	t.Run("dry-run", func(t *testing.T) {
		plan := &Plan{
			DryRun:                   true,
			Verbose:                  true,
			BackupTarget:             "/backups/homes",
			LocalOwner:               "nobody",
			LocalGroup:               "nogroup",
			FixPermsChownCommand:     "chown -R nobody:nogroup /backups/homes",
			FixPermsDirPermsCommand:  "find /backups/homes -type d -exec chmod 770 {} +",
			FixPermsFilePermsCommand: "find /backups/homes -type f -exec chmod 660 {} +",
		}
		executor, logDir := newPhaseExecutor(t, plan, execpkg.NewMockRunner())
		if err := executor.runFixPermsPhase(); err != nil {
			t.Fatalf("runFixPermsPhase() error = %v", err)
		}
		executor.log.Close()
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Phase: Fix permissions", "Target", "Local Owner", "Local Group", "Dry run", "Fix permissions phase completed (dry-run)"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})

	t.Run("success", func(t *testing.T) {
		target := t.TempDir()
		plan := &Plan{
			BackupTarget: target,
			LocalOwner:   "nobody",
			LocalGroup:   "nogroup",
		}
		runner := execpkg.NewMockRunner(execpkg.MockResult{}, execpkg.MockResult{}, execpkg.MockResult{})
		executor, logDir := newPhaseExecutor(t, plan, runner)
		if err := executor.runFixPermsPhase(); err != nil {
			t.Fatalf("runFixPermsPhase() error = %v", err)
		}
		executor.log.Close()
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Phase: Fix permissions", "Target", "Duration", "Fix permissions phase completed successfully"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})
}

func TestPrepareDuplicacySetupAndRunBackup(t *testing.T) {
	t.Run("prepare setup dry-run with filters", func(t *testing.T) {
		workRoot := t.TempDir()
		plan := &Plan{
			DoBackup:                true,
			DryRun:                  true,
			Verbose:                 true,
			NeedsSnapshot:           true,
			WorkRoot:                workRoot,
			DuplicacyRoot:           filepath.Join(workRoot, "duplicacy"),
			RepositoryPath:          "/volume1/homes-snap",
			BackupTarget:            "/backups/homes",
			SnapshotSource:          "/volume1/homes",
			SnapshotTarget:          "/volume1/homes-snap",
			SnapshotCreateCommand:   "btrfs subvolume snapshot -r /volume1/homes /volume1/homes-snap",
			WorkDirCreateCommand:    "mkdir -p " + filepath.Join(workRoot, "duplicacy", ".duplicacy"),
			PreferencesWriteCommand: "write JSON preferences to " + filepath.Join(workRoot, "duplicacy", ".duplicacy", "preferences"),
			FiltersWriteCommand:     "write filters to " + filepath.Join(workRoot, "duplicacy", ".duplicacy", "filters"),
			WorkDirDirPermsCommand:  "find " + filepath.Join(workRoot, "duplicacy") + " -type d -exec chmod 770 {} +",
			WorkDirFilePermsCommand: "find " + filepath.Join(workRoot, "duplicacy") + " -type f -exec chmod 660 {} +",
			Filter:                  "-e *.tmp",
			FilterLines:             []string{"-e *.tmp"},
		}
		executor, logDir := newPhaseExecutor(t, plan, execpkg.NewMockRunner())
		if err := executor.prepareDuplicacySetup(); err != nil {
			t.Fatalf("prepareDuplicacySetup() error = %v", err)
		}
		executor.log.Close()
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Dry run", "btrfs subvolume snapshot", "write JSON preferences", "write filters", "Filter", "Directory"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})

	t.Run("prepare setup success", func(t *testing.T) {
		workRoot := t.TempDir()
		plan := &Plan{
			Verbose:               true,
			NeedsSnapshot:         true,
			WorkRoot:              workRoot,
			DuplicacyRoot:         filepath.Join(workRoot, "duplicacy"),
			RepositoryPath:        "/volume1/homes-snap",
			BackupTarget:          "/backups/homes",
			SnapshotSource:        "/volume1/homes",
			SnapshotTarget:        "/volume1/homes-snap",
			SnapshotCreateCommand: "btrfs subvolume snapshot -r /volume1/homes /volume1/homes-snap",
		}
		runner := execpkg.NewMockRunner(execpkg.MockResult{})
		executor, logDir := newPhaseExecutor(t, plan, runner)
		if err := executor.prepareDuplicacySetup(); err != nil {
			t.Fatalf("prepareDuplicacySetup() error = %v", err)
		}
		executor.log.Close()

		if _, err := os.Stat(filepath.Join(workRoot, "duplicacy", ".duplicacy", "preferences")); err != nil {
			t.Fatalf("preferences file missing: %v", err)
		}
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Snapshot", "/volume1/homes-snap", "Directory", filepath.Join(workRoot, "duplicacy")} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})

	t.Run("backup dry-run", func(t *testing.T) {
		workRoot := t.TempDir()
		plan := &Plan{
			DryRun:         true,
			Threads:        4,
			WorkRoot:       workRoot,
			DuplicacyRoot:  filepath.Join(workRoot, "duplicacy"),
			RepositoryPath: "/volume1/homes-snap",
			BackupTarget:   "/backups/homes",
			BackupCommand:  "duplicacy backup -stats -threads 4",
		}
		executor, logDir := newPhaseExecutor(t, plan, execpkg.NewMockRunner())
		executor.dup = duplicacy.NewSetup(workRoot, plan.RepositoryPath, plan.BackupTarget, true, execpkg.NewMockRunner())
		if err := os.MkdirAll(executor.dup.DuplicacyDir, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := executor.runBackupPhase(); err != nil {
			t.Fatalf("runBackupPhase() error = %v", err)
		}
		executor.log.Close()
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Dry run", "duplicacy backup -stats -threads 4", "Backup phase completed (dry-run)"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})

	t.Run("backup success", func(t *testing.T) {
		workRoot := t.TempDir()
		plan := &Plan{
			Verbose:        false,
			Threads:        4,
			WorkRoot:       workRoot,
			DuplicacyRoot:  filepath.Join(workRoot, "duplicacy"),
			RepositoryPath: "/volume1/homes-snap",
			BackupTarget:   "/backups/homes",
		}
		runner := execpkg.NewMockRunner(execpkg.MockResult{
			Stdout: "Backup for /volume1/homes-snap at revision 2361 completed\nFiles: 10 total, 42 bytes; 1 new, 10 bytes\nTotal running time: 00:00:03\n",
		})
		executor, logDir := newPhaseExecutor(t, plan, runner)
		executor.dup = duplicacy.NewSetup(workRoot, plan.RepositoryPath, plan.BackupTarget, false, runner)
		if err := os.MkdirAll(executor.dup.DuplicacyDir, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := executor.runBackupPhase(); err != nil {
			t.Fatalf("runBackupPhase() error = %v", err)
		}
		executor.log.Close()
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Status", "Backing up snapshot", "Revision", "2361", "Files", "Backup phase completed successfully"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})

	t.Run("execute cleanup storage and fix perms", func(t *testing.T) {
		workRoot := t.TempDir()
		plan := &Plan{
			DryRun:                   true,
			Verbose:                  true,
			NeedsDuplicacySetup:      true,
			DoCleanupStore:           true,
			FixPerms:                 true,
			BackupLabel:              "homes",
			OperationMode:            "Storage cleanup + Fix permissions",
			ModeDisplay:              "Local",
			WorkRoot:                 workRoot,
			DuplicacyRoot:            filepath.Join(workRoot, "duplicacy"),
			RepositoryPath:           "/volume1/homes",
			BackupTarget:             "/backups/homes",
			WorkDirCreateCommand:     "mkdir -p " + filepath.Join(workRoot, "duplicacy", ".duplicacy"),
			PreferencesWriteCommand:  "write JSON preferences to " + filepath.Join(workRoot, "duplicacy", ".duplicacy", "preferences"),
			WorkDirDirPermsCommand:   "find " + filepath.Join(workRoot, "duplicacy") + " -type d -exec chmod 770 {} +",
			WorkDirFilePermsCommand:  "find " + filepath.Join(workRoot, "duplicacy") + " -type f -exec chmod 660 {} +",
			ValidateRepoCommand:      "duplicacy list -files",
			CleanupStorageCommand:    "duplicacy prune -exhaustive -exclusive",
			LocalOwner:               "nobody",
			LocalGroup:               "nogroup",
			FixPermsChownCommand:     "chown -R nobody:nogroup /backups/homes",
			FixPermsDirPermsCommand:  "find /backups/homes -type d -exec chmod 770 {} +",
			FixPermsFilePermsCommand: "find /backups/homes -type f -exec chmod 660 {} +",
			WorkDirRemoveCommand:     "rm -rf " + workRoot,
			LogRetentionDays:         30,
		}
		executor, logDir := newPhaseExecutor(t, plan, execpkg.NewMockRunner())
		if err := executor.execute(); err != nil {
			t.Fatalf("execute() error = %v", err)
		}
		executor.log.Close()
		output := readSingleLogFile(t, logDir)
		for _, token := range []string{"Phase: Storage cleanup", "Storage cleanup phase completed (dry-run)", "Phase: Fix permissions", "Fix permissions phase completed (dry-run)"} {
			if !strings.Contains(output, token) {
				t.Fatalf("output missing %q:\n%s", token, output)
			}
		}
	})

	t.Run("execute stops on storage cleanup error", func(t *testing.T) {
		workRoot := t.TempDir()
		plan := &Plan{
			NeedsDuplicacySetup:   true,
			DoCleanupStore:        true,
			WorkRoot:              workRoot,
			DuplicacyRoot:         filepath.Join(workRoot, "duplicacy"),
			RepositoryPath:        "/volume1/homes",
			BackupTarget:          "/backups/homes",
			ValidateRepoCommand:   "duplicacy list -files",
			CleanupStorageCommand: "duplicacy prune -exhaustive -exclusive",
		}
		runner := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("repo not ready")})
		executor, _ := newPhaseExecutor(t, plan, runner)
		err := executor.execute()
		if err == nil || !strings.Contains(err.Error(), "validate-repo") {
			t.Fatalf("execute() error = %v", err)
		}
	})

	t.Run("execute returns fix perms error", func(t *testing.T) {
		plan := &Plan{
			FixPerms:     true,
			BackupTarget: "/backups/homes",
			LocalOwner:   "nobody",
			LocalGroup:   "nogroup",
		}
		runner := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("chown failed")})
		executor, _ := newPhaseExecutor(t, plan, runner)
		err := executor.execute()
		if err == nil || !strings.Contains(err.Error(), "permissions/chown") {
			t.Fatalf("execute() error = %v", err)
		}
	})
}
