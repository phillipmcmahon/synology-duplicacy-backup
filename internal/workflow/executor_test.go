package workflow

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
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
		Request: &Request{
			FixPerms:      true,
			FixPermsOnly:  true,
			DryRun:        true,
			DefaultNotice: "No primary mode specified: using fix-perms only.",
		},
		Config: &config.Config{
			Destination:      "/backups",
			LocalOwner:       u.Username,
			LocalGroup:       g.Name,
			LogRetentionDays: 30,
		},
		BackupLabel:   "homes",
		BackupTarget:  "/backups/homes",
		WorkRoot:      filepath.Join(t.TempDir(), "work"),
		OperationMode: "Fix permissions only",
	}

	executor := NewExecutor(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt, testExecutorLogger(t), execpkg.NewMockRunner(), plan)
	if code := executor.Run(); code != 0 {
		t.Fatalf("Run() = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(lockParent, "backup-homes.lock.d")); !os.IsNotExist(err) {
		t.Fatalf("expected lock directory cleanup, stat err = %v", err)
	}
}
