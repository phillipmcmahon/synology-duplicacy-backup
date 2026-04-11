package workflow

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
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
	if u.Username != "root" && g.Name != "root" {
		return u.Username, g.Name
	}

	for _, name := range []string{"nobody", "daemon"} {
		if _, err := user.Lookup(name); err == nil {
			u.Username = name
			break
		}
	}
	for _, name := range []string{"nogroup", "nobody", "daemon", "staff", "users"} {
		if _, err := user.LookupGroup(name); err == nil && name != "root" {
			g.Name = name
			break
		}
	}
	if u.Username == "root" || g.Name == "root" {
		t.Skip("no non-root owner/group available on this system")
	}
	return u.Username, g.Name
}

func TestPlannerBuild_BackupPlan(t *testing.T) {
	dir := t.TempDir()
	writeTargetTestConfig(t, dir, "homes", "local", localTargetConfig("homes", "/volume1/homes", "/backups", "", "", 4, ""))

	req := &Request{Source: "homes", DoBackup: true}
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
	if plan.OperationMode != "Backup" {
		t.Fatalf("OperationMode = %q, want %q", plan.OperationMode, "Backup")
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
	if plan.BackupCommand != "duplicacy backup -stats -threads 4" {
		t.Fatalf("BackupCommand = %q", plan.BackupCommand)
	}
	if plan.SnapshotCreateCommand == "" || plan.WorkDirCreateCommand == "" {
		t.Fatalf("expected execution-ready commands, got %+v", plan)
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
	writeTargetTestConfig(t, dir, "homes", "local", localTargetConfig("homes", "/volume1/homes", "/backups", owner, group, 0, ""))

	req := &Request{FixPerms: true, FixPermsOnly: true, Source: "homes"}
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	req.ConfigDir = dir

	plan, err := planner.Build(req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan.OperationMode != "Fix permissions" {
		t.Fatalf("OperationMode = %q", plan.OperationMode)
	}
	if plan.ModeDisplay != "Local" {
		t.Fatalf("ModeDisplay = %q", plan.ModeDisplay)
	}
	if plan.OwnerGroup != owner+":"+group {
		t.Fatalf("OwnerGroup = %q", plan.OwnerGroup)
	}
	if plan.FixPermsChownCommand == "" {
		t.Fatal("expected fix-perms command to be precomputed")
	}
	if len(plan.Summary) != 5 {
		t.Fatalf("summary lines = %d, want 5", len(plan.Summary))
	}
}

func TestPlannerBuild_RemotePlanLoadsSecrets(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets access validation requires root-owned test file")
	}

	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "remote", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket", 4, ""))
	secretsFile := writeTargetTestSecrets(t, secretsDir, "homes", "remote")

	req := &Request{Source: "homes", DoBackup: true, RequestedTarget: "remote", RemoteMode: true, ConfigDir: configDir, SecretsDir: secretsDir}
	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
	)
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), runner)

	plan, err := planner.Build(req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan.Secrets == nil {
		t.Fatal("expected secrets to be loaded for remote plan")
	}
	if plan.SecretsFile != secretsFile {
		t.Fatalf("SecretsFile = %q, want %q", plan.SecretsFile, secretsFile)
	}
	if len(runner.Invocations) != 4 {
		t.Fatalf("invocations = %d, want 4", len(runner.Invocations))
	}
}

func TestPlannerValidateEnvironmentErrors(t *testing.T) {
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())

	t.Run("non-root", func(t *testing.T) {
		rt := testRuntime()
		rt.Geteuid = func() int { return 1000 }
		planner.rt = rt
		if err := planner.validateEnvironment(&Request{DoBackup: true}); err == nil || err.Error() != "Must be run as root" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("missing duplicacy", func(t *testing.T) {
		rt := testRuntime()
		rt.LookPath = func(name string) (string, error) {
			if name == "duplicacy" {
				return "", os.ErrNotExist
			}
			return "/usr/bin/true", nil
		}
		planner.rt = rt
		if err := planner.validateEnvironment(&Request{DoPrune: true}); err == nil || !strings.Contains(err.Error(), "Required command 'duplicacy' not found") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("missing btrfs", func(t *testing.T) {
		rt := testRuntime()
		rt.LookPath = func(name string) (string, error) {
			if name == "btrfs" {
				return "", os.ErrNotExist
			}
			return "/usr/bin/true", nil
		}
		planner.rt = rt
		if err := planner.validateEnvironment(&Request{DoBackup: true}); err == nil || !strings.Contains(err.Error(), "Required command 'btrfs' not found") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPlannerLoadSecrets(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets access validation requires root-owned test file")
	}

	secretsDir := t.TempDir()
	secretsFile := filepath.Join(secretsDir, "duplicacy-homes-remote.toml")
	body := "storj_s3_id = \"ABCDEFGHIJKLMNOPQRSTUVWXYZ01\"\nstorj_s3_secret = \"abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR\"\n"
	if err := os.WriteFile(secretsFile, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chown(secretsFile, 0, 0); err != nil {
		t.Fatalf("Chown() error = %v", err)
	}

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	sec, err := planner.loadSecrets(&Plan{SecretsFile: secretsFile})
	if err != nil {
		t.Fatalf("loadSecrets() error = %v", err)
	}
	if sec.MaskedID() == "" || sec.MaskedSecret() == "" {
		t.Fatalf("unexpected masked secrets: %#v", sec)
	}
}

func TestPlannerLoadConfig_FixPermsRequiresOwnerGroup(t *testing.T) {
	configDir := t.TempDir()
	configFile := writeTargetTestConfig(t, configDir, "homes", "local", "label = \"homes\"\nsource_path = \"/volume1/homes\"\n\n[target]\nname = \"local\"\ntype = \"local\"\nallow_local_accounts = true\n\n[storage]\ndestination = \"/backups\"\nrepository = \"homes\"\n\n[capture]\nthreads = 4\n")

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadConfig(&Plan{
		ConfigFile:  configFile,
		DoBackup:    true,
		FixPerms:    true,
		RemoteMode:  false,
		BackupLabel: "homes",
	})
	if err == nil || !strings.Contains(err.Error(), "local_owner") {
		t.Fatalf("loadConfig() error = %v", err)
	}
}

func TestPlannerLoadConfig_MissingFile(t *testing.T) {
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadConfig(&Plan{ConfigFile: filepath.Join(t.TempDir(), "missing.toml")})
	if err == nil || !strings.Contains(err.Error(), "Configuration file not found") {
		t.Fatalf("loadConfig() error = %v", err)
	}
}

func TestPlannerLoadConfig_DoesNotFallbackToLegacyLabelFile(t *testing.T) {
	configDir := t.TempDir()
	legacyFile := filepath.Join(configDir, "homes-backup.toml")
	if err := os.WriteFile(legacyFile, []byte(localTargetConfig("homes", "/volume1/homes", "/backups", "", "", 4, "-keep 0:365")), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadConfig(&Plan{
		ConfigDir:   configDir,
		ConfigFile:  filepath.Join(configDir, "homes-remote-backup.toml"),
		BackupLabel: "homes",
		Target:      "remote",
	})
	if err == nil || !strings.Contains(err.Error(), "homes-remote-backup.toml") {
		t.Fatalf("loadConfig() error = %v", err)
	}
}

func TestPlannerLoadConfig_RejectsLabelMismatch(t *testing.T) {
	configDir := t.TempDir()
	configFile := writeTargetTestConfig(t, configDir, "homes", "remote", remoteTargetConfig("plexaudio", "/volume1/homes", "s3://bucket", 4, "-keep 0:365"))

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadConfig(&Plan{
		ConfigFile:  configFile,
		BackupLabel: "homes",
		Target:      "remote",
	})
	if err == nil || !strings.Contains(err.Error(), "expected \"homes\"") {
		t.Fatalf("loadConfig() error = %v", err)
	}
}

func TestPlannerLoadSecrets_Invalid(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets access validation requires root-owned test file")
	}

	secretsDir := t.TempDir()
	secretsFile := filepath.Join(secretsDir, "duplicacy-homes-remote.toml")
	body := "storj_s3_id = \"short\"\nstorj_s3_secret = \"also-short\"\n"
	if err := os.WriteFile(secretsFile, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chown(secretsFile, 0, 0); err != nil {
		t.Fatalf("Chown() error = %v", err)
	}

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadSecrets(&Plan{SecretsFile: secretsFile})
	if err == nil || !strings.Contains(err.Error(), "storj_s3_id must be at least 28 characters") {
		t.Fatalf("loadSecrets() error = %v", err)
	}
}

func TestPlannerLoadSecrets_DoesNotFallbackToLegacyLabelFile(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets access validation requires root-owned test file")
	}

	secretsDir := t.TempDir()
	legacyFile := filepath.Join(secretsDir, "duplicacy-homes.toml")
	body := "storj_s3_id = \"ABCDEFGHIJKLMNOPQRSTUVWXYZ01\"\nstorj_s3_secret = \"abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR\"\n"
	if err := os.WriteFile(legacyFile, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chown(legacyFile, 0, 0); err != nil {
		t.Fatalf("Chown() error = %v", err)
	}

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadSecrets(&Plan{SecretsDir: secretsDir, SecretsFile: filepath.Join(secretsDir, "duplicacy-homes-remote.toml"), BackupLabel: "homes", Target: "remote"})
	if err == nil || !strings.Contains(err.Error(), "duplicacy-homes-remote.toml") {
		t.Fatalf("loadSecrets() error = %v", err)
	}
}

func TestPlannerValidateBackupFilesystem(t *testing.T) {
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "ext4\n"},
	))

	if err := planner.validateBackupFilesystem(&Plan{}); err != nil {
		t.Fatalf("validateBackupFilesystem(no backup) error = %v", err)
	}

	err := planner.validateBackupFilesystem(&Plan{DoBackup: true, SnapshotSource: "/volume1/homes", RepositoryPath: "/volume1/homes-snap"})
	if err == nil || !strings.Contains(err.Error(), "not on a btrfs filesystem") {
		t.Fatalf("validateBackupFilesystem() error = %v", err)
	}
}

func TestPlannerLoadConfigAndFilesystemHelpers(t *testing.T) {
	owner, group := currentUserGroup(t)
	dir := t.TempDir()
	writeTargetTestConfig(t, dir, "homes", "local", localTargetConfig("homes", "/volume1/homes", "/backups", owner, group, 4, "-keep 0:365"))

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	plan := planner.derivePlan(&Request{Source: "homes", DoBackup: true, DoPrune: true, FixPerms: true, ConfigDir: dir})

	cfg, err := planner.loadConfig(plan)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.Destination != "/backups" || cfg.LocalOwner != owner || cfg.LocalGroup != group || len(cfg.PruneArgs) == 0 {
		t.Fatalf("cfg = %+v", cfg)
	}

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
	)
	planner.runner = runner
	if err := planner.validateBackupFilesystem(plan); err != nil {
		t.Fatalf("validateBackupFilesystem() error = %v", err)
	}
	if err := planner.validateBackupFilesystem(planner.derivePlan(&Request{Source: "homes"})); err != nil {
		t.Fatalf("validateBackupFilesystem(no backup) error = %v", err)
	}
	if got := splitNonEmptyLines("a\n\nb\n"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitNonEmptyLines() = %#v", got)
	}
}
