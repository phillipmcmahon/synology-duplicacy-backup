package workflow

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func testExecutorLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	return log
}

func readSingleLogFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("log file count = %d, want 1", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return string(data)
}

func TestOperationMode_CleanupStorageWithFixPerms(t *testing.T) {
	req := &Request{DoCleanupStore: true, FixPerms: true}
	if got := OperationMode(req); got != "Storage cleanup + Fix permissions" {
		t.Fatalf("OperationMode() = %q", got)
	}
}

func TestExecutorRun_FixPermsOnlyDryRun(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() error = %v", err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Fatalf("user.LookupGroupId() error = %v", err)
	}

	lockParent := t.TempDir()
	rt := testRuntime()
	rt.NewLock = func(_, label string) *lock.Lock {
		return lock.New(lockParent, label)
	}
	rt.SignalNotify = func(chan<- os.Signal, ...os.Signal) {}

	plan := &Plan{
		FixPerms:                 true,
		FixPermsOnly:             true,
		DryRun:                   true,
		DefaultNotice:            "Primary operation specified: fix-perms only",
		LogRetentionDays:         30,
		LocalOwner:               u.Username,
		LocalGroup:               g.Name,
		OwnerGroup:               u.Username + ":" + g.Name,
		BackupLabel:              "homes",
		BackupTarget:             "/backups/homes",
		WorkRoot:                 filepath.Join(t.TempDir(), "work"),
		OperationMode:            "Fix permissions only",
		FixPermsChownCommand:     "chown -R " + u.Username + ":" + g.Name + " /backups/homes",
		FixPermsDirPermsCommand:  "find /backups/homes -type d -exec chmod 770 {} +",
		FixPermsFilePermsCommand: "find /backups/homes -type f -exec chmod 660 {} +",
	}

	executor := NewExecutor(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt, testExecutorLogger(t), execpkg.NewMockRunner(), plan)
	if code := executor.Run(); code != 0 {
		t.Fatalf("Run() = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(lockParent, "backup-homes.lock.d")); !os.IsNotExist(err) {
		t.Fatalf("expected lock directory cleanup, stat err = %v", err)
	}
}

func TestExecutor_EnforcePrunePreview_ThresholdExceededWithoutForce(t *testing.T) {
	plan := &Plan{
		SafePruneMaxDeleteCount:   1,
		SafePruneMaxDeletePercent: 10,
	}
	executor := NewExecutor(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testExecutorLogger(t), execpkg.NewMockRunner(), plan)

	preview := &duplicacy.PrunePreview{
		DeleteCount:     2,
		TotalRevisions:  10,
		PercentEnforced: true,
	}

	err := executor.enforcePrunePreview(preview)
	if err == nil {
		t.Fatal("expected prune threshold error")
	}
	if got := OperatorMessage(err); got != "Refusing to continue because safe prune thresholds were exceeded" {
		t.Fatalf("OperatorMessage() = %q", got)
	}
}

func TestExecutor_LogPrunePreviewOutput_SuppressesRevisionListing(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}

	executor := NewExecutor(
		DefaultMetadata("duplicacy-backup", "1.0.0", "now", logDir),
		testRuntime(),
		log,
		execpkg.NewMockRunner(),
		&Plan{Verbose: true},
	)

	preview := &duplicacy.PrunePreview{
		Output:         "Repository set to /volume1/homes\nNo snapshot to delete\n",
		RevisionOutput: "revision 1\nrevision 2\n",
	}

	executor.logPrunePreviewOutput(preview)
	log.Close()

	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("log file count = %d, want 1", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "Preview") || !strings.Contains(output, "Repository set to /volume1/homes") {
		t.Fatalf("expected safe prune preview output, got %q", output)
	}
	if strings.Contains(output, "[REVISION-LIST]") {
		t.Fatalf("expected revision listing to be suppressed, got %q", output)
	}
}

func TestExecutorRun_BackupCommandFailureStillPrintsFailureFooter(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	defer log.Close()

	lockParent := t.TempDir()
	rt := testRuntime()
	rt.NewLock = func(_, label string) *lock.Lock {
		return lock.New(lockParent, label)
	}
	rt.SignalNotify = func(chan<- os.Signal, ...os.Signal) {}

	workRoot := filepath.Join(t.TempDir(), "work")
	plan := &Plan{
		DoBackup:            true,
		NeedsDuplicacySetup: true,
		LogRetentionDays:    30,
		BackupLabel:         "homes",
		OperationMode:       "Backup only",
		ModeDisplay:         "Local",
		WorkRoot:            workRoot,
		DuplicacyRoot:       filepath.Join(workRoot, "duplicacy"),
		RepositoryPath:      "/volume1/homes-snap",
		BackupTarget:        "/backups/homes",
		Threads:             4,
	}
	runner := execpkg.NewMockRunner(execpkg.MockResult{
		Stdout: "Repository set to /volume1/homes-snap\n",
		Stderr: "storage write failed\n",
		Err:    errors.New("exit status 1"),
	})

	executor := NewExecutor(DefaultMetadata("duplicacy-backup", "1.0.0", "now", logDir), rt, log, runner, plan)
	if code := executor.Run(); code != 1 {
		t.Fatalf("Run() = %d, want 1", code)
	}

	output := readSingleLogFile(t, logDir)
	required := []string{
		"Phase: Backup",
		"Backup failed while running duplicacy backup",
		"Result",
		"Failed",
		"Code",
		"Run completed -",
	}
	for _, token := range required {
		if !strings.Contains(output, token) {
			t.Fatalf("expected %q in log output, got %q", token, output)
		}
	}
}
