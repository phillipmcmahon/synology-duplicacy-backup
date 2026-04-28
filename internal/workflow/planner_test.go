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

func runtimeRequestForTest(req *Request) *RuntimeRequest {
	runtimeReq := NewRuntimeRequest(req)
	return &runtimeReq
}

func TestPlannerBuild_BackupPlan(t *testing.T) {
	dir := t.TempDir()
	writeTargetTestConfig(t, dir, "homes", "onsite-usb", localTargetConfig("homes", "/volume1/homes", "/backups", "", "", 4, ""))

	req := &Request{Source: "homes", DoBackup: true, RequestedTarget: "onsite-usb"}
	rt := testRuntime()
	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
	)
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt, testLogger(t), runner)
	req.ConfigDir = dir

	plan, err := planner.Build(runtimeRequestForTest(req))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan.Request.OperationMode != "Backup" {
		t.Fatalf("OperationMode = %q, want %q", plan.Request.OperationMode, "Backup")
	}
	if plan.Paths.BackupTarget != "/backups/homes" {
		t.Fatalf("BackupTarget = %q", plan.Paths.BackupTarget)
	}
	if !plan.Request.NeedsDuplicacySetup || !plan.Request.NeedsSnapshot {
		t.Fatalf("expected backup plan to need setup and snapshot: %+v", plan)
	}
	if plan.Config.PruneArgsDisplay != "" {
		t.Fatalf("PruneArgsDisplay = %q, want empty", plan.Config.PruneArgsDisplay)
	}
	if plan.Display.BackupCommand != "duplicacy backup -stats -threads 4" {
		t.Fatalf("BackupCommand = %q", plan.Display.BackupCommand)
	}
	if plan.Display.SnapshotCreateCommand == "" || plan.Display.WorkDirCreateCommand == "" {
		t.Fatalf("expected execution-ready commands, got %+v", plan)
	}
	if len(plan.Summary) == 0 {
		t.Fatal("expected summary lines to be precomputed")
	}
	if len(runner.Invocations) != 4 {
		t.Fatalf("invocations = %d, want 4", len(runner.Invocations))
	}
}

func TestPlannerBuild_RemotePlanLoadsSecrets(t *testing.T) {
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket", 4, ""))
	secretsFile := writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{Source: "homes", DoBackup: true, RequestedTarget: "offsite-storj", ConfigDir: configDir, SecretsDir: secretsDir}
	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
	)
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), runner)

	plan, err := planner.Build(runtimeRequestForTest(req))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan.Secrets == nil {
		t.Fatal("expected secrets to be loaded for remote plan")
	}
	if plan.Paths.SecretsFile != secretsFile {
		t.Fatalf("SecretsFile = %q, want %q", plan.Paths.SecretsFile, secretsFile)
	}
	if len(runner.Invocations) != 4 {
		t.Fatalf("invocations = %d, want 4", len(runner.Invocations))
	}
}

func TestPlannerBuild_LocalDuplicacyPlanLoadsSecrets(t *testing.T) {
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-rustfs", localDuplicacyTargetConfig("homes", "/volume1/homes", "s3://rustfs.local/bucket", 4, ""))
	secretsFile := writeTargetTestSecrets(t, secretsDir, "homes", "onsite-rustfs")

	req := &Request{Source: "homes", DoBackup: true, RequestedTarget: "onsite-rustfs", ConfigDir: configDir, SecretsDir: secretsDir}
	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
	)
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), runner)

	plan, err := planner.Build(runtimeRequestForTest(req))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan.IsRemoteLocation() {
		t.Fatal("expected local duplicacy target not to be reported as remote")
	}
	if plan.Config.Location != locationLocal {
		t.Fatalf("Location = %q, want %q", plan.Config.Location, locationLocal)
	}
	if plan.Secrets == nil {
		t.Fatal("expected secrets to be loaded for local duplicacy plan")
	}
	if plan.Paths.SecretsFile != secretsFile {
		t.Fatalf("SecretsFile = %q, want %q", plan.Paths.SecretsFile, secretsFile)
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
		if err := planner.validateEnvironment(runtimeRequestForTest(&Request{DoBackup: true})); err == nil || !strings.Contains(err.Error(), "backup must be run as root") {
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
		if err := planner.validateEnvironment(runtimeRequestForTest(&Request{DoPrune: true})); err == nil || !strings.Contains(err.Error(), "required command 'duplicacy' not found") {
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
		if err := planner.validateEnvironment(runtimeRequestForTest(&Request{DoBackup: true})); err == nil || !strings.Contains(err.Error(), "required command 'btrfs' not found") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPlannerBuild_NonRootLocalRepositoryMutationRequiresRoot(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", "/volume1/homes", "/backups", "", "", 4, "-keep 0:365"))

	rt := testRuntime()
	rt.Geteuid = func() int { return 1000 }
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt, testLogger(t), execpkg.NewMockRunner())

	tests := []struct {
		name string
		req  *Request
		want string
	}{
		{
			name: "prune",
			req:  &Request{Source: "homes", DoPrune: true, RequestedTarget: "onsite-usb", ConfigDir: configDir},
			want: "prune requires sudo; path-based local repository storage",
		},
		{
			name: "prune dry-run",
			req:  &Request{Source: "homes", DoPrune: true, DryRun: true, RequestedTarget: "onsite-usb", ConfigDir: configDir},
			want: "prune --dry-run requires sudo; path-based local repository storage",
		},
		{
			name: "cleanup storage",
			req:  &Request{Source: "homes", DoCleanupStore: true, RequestedTarget: "onsite-usb", ConfigDir: configDir},
			want: "cleanup-storage requires sudo; path-based local repository storage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := planner.Build(runtimeRequestForTest(tt.req))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Build() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPlannerBuild_NonRootLocalRepositoryCleanupDryRunIsSimulationOnly(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", "/volume1/homes", "/backups", "", "", 4, "-keep 0:365"))

	rt := testRuntime()
	rt.Geteuid = func() int { return 1000 }
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt, testLogger(t), execpkg.NewMockRunner())

	req := &Request{Source: "homes", DoCleanupStore: true, DryRun: true, RequestedTarget: "onsite-usb", ConfigDir: configDir}
	plan, err := planner.Build(runtimeRequestForTest(req))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !plan.Request.DoCleanupStore || !plan.Request.DryRun {
		t.Fatalf("plan cleanup/dry-run = %t/%t, want true/true", plan.Request.DoCleanupStore, plan.Request.DryRun)
	}
}

func TestPlannerBuild_NonRootObjectRepositoryMutationUsesCredentials(t *testing.T) {
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-rustfs", localDuplicacyTargetConfig("homes", "/volume1/homes", "s3://rustfs.local/bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "onsite-rustfs")

	rt := testRuntime()
	rt.Geteuid = func() int { return 1000 }
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt, testLogger(t), execpkg.NewMockRunner())

	req := &Request{Source: "homes", DoPrune: true, DryRun: true, RequestedTarget: "onsite-rustfs", ConfigDir: configDir, SecretsDir: secretsDir}
	plan, err := planner.Build(runtimeRequestForTest(req))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan.Paths.BackupTarget != "s3://rustfs.local/bucket" {
		t.Fatalf("BackupTarget = %q", plan.Paths.BackupTarget)
	}
}

func TestPlannerLoadSecrets(t *testing.T) {
	secretsDir := t.TempDir()
	secretsFile := filepath.Join(secretsDir, "homes-secrets.toml")
	body := "[targets.offsite-storj.keys]\ns3_id = \"ABCDEFGHIJKLMNOPQRSTUVWXYZ01\"\ns3_secret = \"abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR\"\n"
	if err := os.WriteFile(secretsFile, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	sec, err := planner.loadSecrets(&Plan{
		Config: PlanConfig{Target: "offsite-storj"},
		Paths:  PlanPaths{SecretsFile: secretsFile},
	})
	if err != nil {
		t.Fatalf("loadSecrets() error = %v", err)
	}
	if sec.MaskedKeys() == "" {
		t.Fatalf("unexpected masked secrets: %#v", sec)
	}
}

func TestPlannerLoadConfig_MissingFile(t *testing.T) {
	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadConfig(&Plan{Paths: PlanPaths{ConfigFile: filepath.Join(t.TempDir(), "missing.toml")}})
	if err == nil || !strings.Contains(err.Error(), "configuration file not found") {
		t.Fatalf("loadConfig() error = %v", err)
	}
}

func TestPlannerLoadConfig_MissingCanonicalFileReportsCanonicalPath(t *testing.T) {
	configDir := t.TempDir()
	unrelatedFile := filepath.Join(configDir, "plexaudio-backup.toml")
	if err := os.WriteFile(unrelatedFile, []byte(localTargetConfig("plexaudio", "/volume1/plexaudio", "/backups", "", "", 4, "-keep 0:365")), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadConfig(&Plan{
		Config: PlanConfig{
			BackupLabel: "homes",
			Target:      "offsite-storj",
		},
		Paths: PlanPaths{
			ConfigDir:  configDir,
			ConfigFile: filepath.Join(configDir, "homes-backup.toml"),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "homes-backup.toml") {
		t.Fatalf("loadConfig() error = %v", err)
	}
}

func TestPlannerLoadConfig_RejectsLabelMismatch(t *testing.T) {
	configDir := t.TempDir()
	configFile := writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("plexaudio", "/volume1/homes", "s3://bucket", 4, "-keep 0:365"))

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadConfig(&Plan{
		Config: PlanConfig{
			BackupLabel: "homes",
			Target:      "offsite-storj",
		},
		Paths: PlanPaths{ConfigFile: configFile},
	})
	if err == nil || !strings.Contains(err.Error(), "expected \"homes\"") {
		t.Fatalf("loadConfig() error = %v", err)
	}
}

func TestPlannerLoadSecrets_Invalid(t *testing.T) {
	secretsDir := t.TempDir()
	secretsFile := filepath.Join(secretsDir, "homes-secrets.toml")
	body := "[targets.offsite-storj.keys]\ns3_id = \"\"\n"
	if err := os.WriteFile(secretsFile, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadSecrets(&Plan{
		Config: PlanConfig{Target: "offsite-storj"},
		Paths:  PlanPaths{SecretsFile: secretsFile},
	})
	if err == nil || !strings.Contains(err.Error(), "s3_id") {
		t.Fatalf("loadSecrets() error = %v", err)
	}
}

func TestPlannerLoadSecrets_DoesNotFallbackToLegacyLabelFile(t *testing.T) {
	secretsDir := t.TempDir()
	legacyFile := filepath.Join(secretsDir, "duplicacy-homes.toml")
	body := "[targets.offsite-storj.keys]\ns3_id = \"ABCDEFGHIJKLMNOPQRSTUVWXYZ01\"\ns3_secret = \"abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR\"\n"
	if err := os.WriteFile(legacyFile, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	_, err := planner.loadSecrets(&Plan{
		Config: PlanConfig{
			BackupLabel: "homes",
			Target:      "offsite-storj",
		},
		Paths: PlanPaths{
			SecretsDir:  secretsDir,
			SecretsFile: filepath.Join(secretsDir, "homes-secrets.toml"),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "homes-secrets.toml") {
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

	err := planner.validateBackupFilesystem(&Plan{
		Request: PlanRequest{DoBackup: true},
		Paths: PlanPaths{
			SnapshotSource: "/volume1/homes",
			RepositoryPath: "/volume1/homes-snap",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not on a btrfs filesystem") {
		t.Fatalf("validateBackupFilesystem() error = %v", err)
	}
}

func TestPlannerLoadConfigAndLocalDiskStorageHelpers(t *testing.T) {
	owner, group := currentUserGroup(t)
	dir := t.TempDir()
	writeTargetTestConfig(t, dir, "homes", "onsite-usb", localTargetConfig("homes", "/volume1/homes", "/backups", owner, group, 4, "-keep 0:365"))

	planner := NewPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testLogger(t), execpkg.NewMockRunner())
	plan := planner.deriveRuntimePlan(runtimeRequestForTest(&Request{Source: "homes", DoBackup: true, ConfigDir: dir, RequestedTarget: "onsite-usb"}))

	cfg, err := planner.loadConfig(plan)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.Storage != "/backups/homes" || cfg.LocalOwner != owner || cfg.LocalGroup != group || len(cfg.PruneArgs) == 0 {
		t.Fatalf("cfg = %+v", cfg)
	}

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
	)
	planner.runner = runner
	if err := planner.validateBackupFilesystem(plan); err != nil {
		t.Fatalf("validateBackupFilesystem() error = %v", err)
	}
	if err := planner.validateBackupFilesystem(planner.deriveRuntimePlan(runtimeRequestForTest(&Request{Source: "homes", RequestedTarget: "onsite-usb"}))); err != nil {
		t.Fatalf("validateBackupFilesystem(no backup) error = %v", err)
	}
	if got := splitNonEmptyLines("a\n\nb\n"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitNonEmptyLines() = %#v", got)
	}
}
