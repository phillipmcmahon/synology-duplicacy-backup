package workflow

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	return log
}

func testRuntime() Runtime {
	rt := DefaultRuntime()
	rt.Geteuid = func() int { return 0 }
	rt.LookPath = func(string) (string, error) { return "/usr/bin/true", nil }
	rt.Now = func() time.Time { return time.Date(2026, 4, 9, 18, 0, 0, 0, time.UTC) }
	rt.TempDir = func() string { return os.TempDir() }
	rt.Getpid = func() int { return 4242 }
	return rt
}

func currentUserGroup(t *testing.T) (string, string) {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() error = %v", err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Fatalf("user.LookupGroupId() error = %v", err)
	}
	return u.Username, g.Name
}

func TestPlannerBuild_BackupPlan(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "homes-backup.conf")
	if err := os.WriteFile(configFile, []byte("[common]\nDESTINATION=/backups\nTHREADS=4\n[local]\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := &Request{Mode: "backup", Source: "homes", DoBackup: true}
	rt := testRuntime()
	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
	)
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt, testLogger(t), runner)
	req.ConfigDir = dir

	plan, err := planner.Build(req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan.OperationMode != "Backup only" {
		t.Fatalf("OperationMode = %q, want %q", plan.OperationMode, "Backup only")
	}
	if plan.BackupTarget != "/backups/homes" {
		t.Fatalf("BackupTarget = %q", plan.BackupTarget)
	}
	if !plan.NeedsDuplicacySetup || !plan.NeedsSnapshot {
		t.Fatalf("expected backup plan to need setup and snapshot: %+v", plan)
	}
	if plan.PruneArgsDisplay != "" {
		t.Fatalf("PruneArgsDisplay = %q, want empty", plan.PruneArgsDisplay)
	}
	if len(plan.Summary) == 0 {
		t.Fatal("expected summary lines to be precomputed")
	}
	if len(runner.Invocations) != 4 {
		t.Fatalf("invocations = %d, want 4", len(runner.Invocations))
	}
}

func TestPlannerBuild_FixPermsOnlyPlan(t *testing.T) {
	dir := t.TempDir()
	owner, group := currentUserGroup(t)
	configFile := filepath.Join(dir, "homes-backup.conf")
	content := "[common]\nDESTINATION=/backups\n[local]\nLOCAL_OWNER=" + owner + "\nLOCAL_GROUP=" + group + "\n"
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := &Request{FixPerms: true, FixPermsOnly: true, Source: "homes"}
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	req.ConfigDir = dir

	plan, err := planner.Build(req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan.OperationMode != "Fix permissions only" {
		t.Fatalf("OperationMode = %q", plan.OperationMode)
	}
	if plan.ModeDisplay != "LOCAL" {
		t.Fatalf("ModeDisplay = %q", plan.ModeDisplay)
	}
	if len(plan.Summary) != 5 {
		t.Fatalf("summary lines = %d, want 5", len(plan.Summary))
	}
}
