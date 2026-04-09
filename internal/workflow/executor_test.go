package workflow

import (
	"os"
	"os/user"
	"path/filepath"
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

func TestOperationMode_PruneDeepWithFixPerms(t *testing.T) {
	req := &Request{DoPrune: true, DeepPruneMode: true, FixPerms: true}
	if got := OperationMode(req); got != "Prune deep + fix permissions" {
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
		DefaultNotice:            "No primary mode specified: using fix-perms only.",
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
	if got := OperatorMessage(err); got != "Refusing to continue because safe prune thresholds were exceeded." {
		t.Fatalf("OperatorMessage() = %q", got)
	}
}
