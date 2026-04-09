package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

// ─── Integration tests ──────────────────────────────────────────────────────
// These tests exercise multiple coordinator methods together to verify that
// the refactored pipeline maintains the same end-to-end behaviour as the
// original monolithic run() function.

// itestLogger creates a logger that writes to a temp dir (for integration tests).
func itestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	dir := t.TempDir()
	log, err := logger.New(dir, "itest", false)
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return log
}

// writeConfig creates a config file in dir and returns its path.
func writeConfig(t *testing.T, dir, label, content string) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("%s-backup.conf", label))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return path
}

// ─── Full pipeline: loadConfig → loadSecrets → printHeader → printSummary ───

func TestIntegration_LocalPruneConfigPipeline(t *testing.T) {
	// Simulates: duplicacy-backup --prune homes (local mode)
	// Exercises: loadConfig, loadSecrets (no-op), printHeader, printSummary.

	confDir := t.TempDir()
	lockDir := t.TempDir()

	confContent := `[common]
PRUNE=-keep 1:728 -keep 91:364 -keep 28:182 -keep 7:28

[local]
DESTINATION=/volume2/backups
THREADS=4
`
	a := &app{
		log:   itestLogger(t),
		flags: &flags{mode: "prune", source: "homes", dryRun: true},

		doBackup:      false,
		doPrune:       true,
		deepPruneMode: false,
		fixPermsOnly:  false,

		backupLabel:    "homes",
		runTimestamp:   "20260409-120000",
		snapshotSource: "/volume1/homes",
		snapshotTarget: "/volume1/homes-20260409-120000",
		workRoot:       filepath.Join(t.TempDir(), "work"),
		repositoryPath: "/volume1/homes",

		configDir:  confDir,
		secretsDir: t.TempDir(),

		lk:     lock.New(lockDir, "homes"),
		runner: execpkg.NewMockRunner(),
	}
	a.configFile = writeConfig(t, confDir, "homes", confContent)

	// Step 1: loadConfig
	if err := a.loadConfig(); err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if a.cfg == nil {
		t.Fatal("cfg should not be nil")
	}
	if a.cfg.Destination != "/volume2/backups" {
		t.Errorf("Destination = %q, want /volume2/backups", a.cfg.Destination)
	}
	if a.backupTarget != "/volume2/backups/homes" {
		t.Errorf("backupTarget = %q, want /volume2/backups/homes", a.backupTarget)
	}
	if len(a.cfg.PruneArgs) == 0 {
		t.Error("PruneArgs should be populated after loadConfig")
	}

	// Step 2: loadSecrets (local mode – no-op)
	if err := a.loadSecrets(); err != nil {
		t.Fatalf("loadSecrets() should be no-op: %v", err)
	}

	// Step 3: printHeader (should not panic)
	a.printHeader()

	// Step 4: printSummary (should not panic)
	a.printSummary()
}

func TestIntegration_LocalBackupConfigPipeline(t *testing.T) {
	// Simulates config parsing for backup mode (local).
	// We set doBackup=false to skip the btrfs volume checks that require
	// /volume1 to exist, since this test focuses on config + secrets + summary.

	confDir := t.TempDir()
	lockDir := t.TempDir()

	confContent := `[common]
PRUNE=-keep 1:728

[local]
DESTINATION=/volume2/backups
THREADS=4
FILTER=e:^(.*/)?(@eaDir)/$
`
	a := &app{
		log:   itestLogger(t),
		flags: &flags{mode: "backup", source: "homes", dryRun: true},

		doBackup:      false, // skip btrfs checks (no /volume1 in test env)
		doPrune:       false,
		deepPruneMode: false,
		fixPermsOnly:  false,

		backupLabel:    "homes",
		runTimestamp:   "20260409-120000",
		snapshotSource: "/volume1/homes",
		snapshotTarget: "/volume1/homes-20260409-120000",
		workRoot:       filepath.Join(t.TempDir(), "work"),
		repositoryPath: "/volume1/homes-20260409-120000",

		configDir:  confDir,
		secretsDir: t.TempDir(),

		lk:     lock.New(lockDir, "homes"),
		runner: execpkg.NewMockRunner(),
	}
	a.configFile = writeConfig(t, confDir, "homes", confContent)

	if err := a.loadConfig(); err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if a.cfg.Filter == "" {
		t.Error("Filter should be set from config")
	}
	if a.cfg.Threads != 4 {
		t.Errorf("Threads = %d, want 4", a.cfg.Threads)
	}
	if err := a.loadSecrets(); err != nil {
		t.Fatalf("loadSecrets() should be no-op for local: %v", err)
	}

	// Re-set doBackup for summary display test
	a.doBackup = true
	a.printHeader()
	a.printSummary()
}

func TestIntegration_FixPermsOnlyPipeline(t *testing.T) {
	// Simulates: duplicacy-backup --fix-perms homes
	confDir := t.TempDir()
	lockDir := t.TempDir()

	confContent := `[common]
PRUNE=-keep 1:728

[local]
DESTINATION=/volume2/backups
LOCAL_OWNER=nobody
LOCAL_GROUP=nogroup
THREADS=4
`
	a := &app{
		log:   itestLogger(t),
		flags: &flags{fixPerms: true, source: "homes", dryRun: true},

		doBackup:      false,
		doPrune:       false,
		deepPruneMode: false,
		fixPermsOnly:  true,

		backupLabel:    "homes",
		runTimestamp:   "20260409-120000",
		snapshotSource: "/volume1/homes",
		snapshotTarget: "/volume1/homes-20260409-120000",
		workRoot:       filepath.Join(t.TempDir(), "work"),
		repositoryPath: "/volume1/homes",

		configDir:  confDir,
		secretsDir: t.TempDir(),

		lk:     lock.New(lockDir, "homes"),
		runner: execpkg.NewMockRunner(),
	}
	a.configFile = writeConfig(t, confDir, "homes", confContent)

	if err := a.loadConfig(); err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if a.cfg.LocalOwner != "nobody" {
		t.Errorf("LocalOwner = %q, want 'nobody'", a.cfg.LocalOwner)
	}

	a.printHeader()
	a.printSummary()

	// execute() in dry-run should succeed
	if err := a.execute(); err != nil {
		t.Fatalf("execute() failed: %v", err)
	}
}

// ─── Lock → Config → Cleanup lifecycle ──────────────────────────────────────

func TestIntegration_LockConfigCleanupLifecycle(t *testing.T) {
	confDir := t.TempDir()
	lockDir := t.TempDir()

	confContent := `[common]
PRUNE=-keep 1:728

[local]
DESTINATION=/volume2/backups
THREADS=4
`
	workRoot := filepath.Join(t.TempDir(), "work")
	os.MkdirAll(workRoot, 0755)

	a := &app{
		log:   itestLogger(t),
		flags: &flags{mode: "prune", source: "homes", dryRun: false},

		doBackup:      false,
		doPrune:       true,
		deepPruneMode: false,
		fixPermsOnly:  false,

		backupLabel:    "homes",
		runTimestamp:   "20260409-120000",
		snapshotSource: "/volume1/homes",
		snapshotTarget: "/volume1/homes-20260409-120000",
		workRoot:       workRoot,
		repositoryPath: "/volume1/homes",

		configDir:  confDir,
		secretsDir: t.TempDir(),

		lk:     lock.New(lockDir, "homes"),
		runner: execpkg.NewMockRunner(),
	}
	a.configFile = writeConfig(t, confDir, "homes", confContent)

	// Acquire lock
	if err := a.acquireLock(); err != nil {
		t.Fatalf("acquireLock() failed: %v", err)
	}
	// Verify lock exists
	if _, err := os.Stat(a.lk.Path); os.IsNotExist(err) {
		t.Fatal("lock directory should exist after acquireLock")
	}

	// Load config
	if err := a.loadConfig(); err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}

	// Cleanup
	a.cleanup()

	// Verify lock released
	if _, err := os.Stat(a.lk.Path); !os.IsNotExist(err) {
		t.Error("lock directory should be removed after cleanup")
	}
	// Verify workRoot removed
	if _, err := os.Stat(workRoot); !os.IsNotExist(err) {
		t.Error("workRoot should be removed after cleanup")
	}
	if !a.cleanedUp {
		t.Error("cleanedUp should be true")
	}
}

// ─── Error propagation: fail() sets exitCode correctly ──────────────────────

func TestIntegration_FailSetsExitCodeForCleanup(t *testing.T) {
	lockDir := t.TempDir()

	a := &app{
		log:   itestLogger(t),
		flags: &flags{mode: "backup", source: "homes", dryRun: true},

		doBackup:       true,
		backupLabel:    "homes",
		runTimestamp:   "20260409-120000",
		snapshotSource: "/volume1/homes",
		snapshotTarget: "/volume1/homes-20260409-120000",
		workRoot:       filepath.Join(t.TempDir(), "work"),
		repositoryPath: "/volume1/homes-20260409-120000",

		lk:     lock.New(lockDir, "homes"),
		runner: execpkg.NewMockRunner(),
	}

	// Simulate a failure
	a.fail(fmt.Errorf("simulated error"))
	if a.exitCode != 1 {
		t.Errorf("exitCode = %d, want 1", a.exitCode)
	}

	// Cleanup should show FAILED status (we just verify no panic)
	a.cleanup()
	if !a.cleanedUp {
		t.Error("cleanedUp should be true")
	}
}

// ─── Config validation errors bubble up correctly ───────────────────────────

func TestIntegration_MissingDestinationErrors(t *testing.T) {
	confDir := t.TempDir()
	confContent := `[common]
PRUNE=-keep 1:728

[local]
THREADS=4
`
	a := &app{
		log:   itestLogger(t),
		flags: &flags{mode: "backup", source: "homes", dryRun: true},

		doBackup:       true,
		doPrune:        false,
		backupLabel:    "homes",
		runTimestamp:   "20260409-120000",
		snapshotSource: "/volume1/homes",
		snapshotTarget: "/volume1/homes-20260409-120000",
		workRoot:       filepath.Join(t.TempDir(), "work"),
		repositoryPath: "/volume1/homes-20260409-120000",

		configDir:  confDir,
		secretsDir: t.TempDir(),
	}
	a.configFile = writeConfig(t, confDir, "homes", confContent)

	err := a.loadConfig()
	if err == nil {
		t.Fatal("loadConfig() should fail for missing DESTINATION")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "destination") {
		t.Errorf("error = %q, expected mention of DESTINATION", err.Error())
	}
}

// ─── Remote mode: loadSecrets with missing file returns error ───────────────

func TestIntegration_RemoteSecretsMissing(t *testing.T) {
	confDir := t.TempDir()
	confContent := `[common]
PRUNE=-keep 1:728

[remote]
DESTINATION=s3://gateway.storjshare.io/bucket
THREADS=8
`
	a := &app{
		log:   itestLogger(t),
		flags: &flags{mode: "backup", source: "homes", remoteMode: true, dryRun: true},

		doBackup:       false, // skip btrfs checks (no /volume1 in test env)
		doPrune:        false,
		backupLabel:    "homes",
		runTimestamp:   "20260409-120000",
		snapshotSource: "/volume1/homes",
		snapshotTarget: "/volume1/homes-20260409-120000",
		workRoot:       filepath.Join(t.TempDir(), "work"),
		repositoryPath: "/volume1/homes-20260409-120000",

		configDir:  confDir,
		secretsDir: t.TempDir(), // empty – no secrets file
	}
	a.configFile = writeConfig(t, confDir, "homes", confContent)

	// loadConfig should succeed (remote config is valid)
	if err := a.loadConfig(); err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}

	// loadSecrets should fail (missing file)
	err := a.loadSecrets()
	if err == nil {
		t.Fatal("loadSecrets() should fail for missing secrets file in remote mode")
	}
}

// ─── Verify backupTarget is correctly derived ───────────────────────────────

func TestIntegration_BackupTargetDerivation(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		label       string
		expected    string
	}{
		{"local", "/volume2/backups", "homes", "/volume2/backups/homes"},
		{"s3", "s3://gateway.storjshare.io/bucket", "homes", "s3://gateway.storjshare.io/bucket/homes"},
		{"https", "https://example.com/backup", "docs", "https://example.com/backup/docs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confDir := t.TempDir()
			confContent := fmt.Sprintf(`[common]
PRUNE=-keep 1:728

[local]
DESTINATION=%s
THREADS=4
`, tt.destination)

			a := &app{
				log:   itestLogger(t),
				flags: &flags{mode: "prune", source: tt.label, dryRun: true},

				doBackup:       false,
				doPrune:        true,
				backupLabel:    tt.label,
				runTimestamp:   "20260409-120000",
				snapshotSource: filepath.Join("/volume1", tt.label),
				snapshotTarget: filepath.Join("/volume1", tt.label+"-20260409-120000"),
				workRoot:       filepath.Join(t.TempDir(), "work"),
				repositoryPath: filepath.Join("/volume1", tt.label),

				configDir:  confDir,
				secretsDir: t.TempDir(),
			}
			a.configFile = writeConfig(t, confDir, tt.label, confContent)

			if err := a.loadConfig(); err != nil {
				t.Fatalf("loadConfig() failed: %v", err)
			}
			if a.backupTarget != tt.expected {
				t.Errorf("backupTarget = %q, want %q", a.backupTarget, tt.expected)
			}
		})
	}
}

// ─── Verify config parsing applies safe prune thresholds ────────────────────

func TestIntegration_CustomSafePruneThresholds(t *testing.T) {
	confDir := t.TempDir()
	confContent := `[common]
PRUNE=-keep 1:728
SAFE_PRUNE_MAX_DELETE_PERCENT=20
SAFE_PRUNE_MAX_DELETE_COUNT=50
SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT=10

[local]
DESTINATION=/volume2/backups
THREADS=4
`
	a := &app{
		log:   itestLogger(t),
		flags: &flags{mode: "prune", source: "homes", dryRun: true},

		doBackup:       false,
		doPrune:        true,
		backupLabel:    "homes",
		runTimestamp:   "20260409-120000",
		snapshotSource: "/volume1/homes",
		snapshotTarget: "/volume1/homes-20260409-120000",
		workRoot:       filepath.Join(t.TempDir(), "work"),
		repositoryPath: "/volume1/homes",

		configDir:  confDir,
		secretsDir: t.TempDir(),
	}
	a.configFile = writeConfig(t, confDir, "homes", confContent)

	if err := a.loadConfig(); err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if a.cfg.SafePruneMaxDeletePercent != 20 {
		t.Errorf("SafePruneMaxDeletePercent = %d, want 20", a.cfg.SafePruneMaxDeletePercent)
	}
	if a.cfg.SafePruneMaxDeleteCount != 50 {
		t.Errorf("SafePruneMaxDeleteCount = %d, want 50", a.cfg.SafePruneMaxDeleteCount)
	}
	if a.cfg.SafePruneMinTotalForPercent != 10 {
		t.Errorf("SafePruneMinTotalForPercent = %d, want 10", a.cfg.SafePruneMinTotalForPercent)
	}
}

// ─── Verify that app defaults match config.NewDefaults ──────────────────────

func TestIntegration_DefaultThresholdsMatch(t *testing.T) {
	confDir := t.TempDir()
	confContent := `[common]
PRUNE=-keep 1:728

[local]
DESTINATION=/volume2/backups
THREADS=4
`
	a := &app{
		log:   itestLogger(t),
		flags: &flags{mode: "prune", source: "homes", dryRun: true},

		doBackup:       false,
		doPrune:        true,
		backupLabel:    "homes",
		runTimestamp:   "20260409-120000",
		snapshotSource: "/volume1/homes",
		snapshotTarget: "/volume1/homes-20260409-120000",
		workRoot:       filepath.Join(t.TempDir(), "work"),
		repositoryPath: "/volume1/homes",

		configDir:  confDir,
		secretsDir: t.TempDir(),
	}
	a.configFile = writeConfig(t, confDir, "homes", confContent)

	if err := a.loadConfig(); err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}

	defaults := config.NewDefaults()
	if a.cfg.SafePruneMaxDeletePercent != defaults.SafePruneMaxDeletePercent {
		t.Errorf("SafePruneMaxDeletePercent = %d, want default %d",
			a.cfg.SafePruneMaxDeletePercent, defaults.SafePruneMaxDeletePercent)
	}
	if a.cfg.SafePruneMaxDeleteCount != defaults.SafePruneMaxDeleteCount {
		t.Errorf("SafePruneMaxDeleteCount = %d, want default %d",
			a.cfg.SafePruneMaxDeleteCount, defaults.SafePruneMaxDeleteCount)
	}
	if a.cfg.LogRetentionDays != defaults.LogRetentionDays {
		t.Errorf("LogRetentionDays = %d, want default %d",
			a.cfg.LogRetentionDays, defaults.LogRetentionDays)
	}
}

// ─── Sub-initializer integration tests (Phase 2) ────────────────────────────

// TestIntegration_FullInitFlow exercises the complete sub-initializer
// sequence: initLogger → parseAppFlags → derivePaths.  We skip
// validateEnvironment because it requires root and real binaries.
func TestIntegration_FullInitFlow(t *testing.T) {
	a := &app{}

	// Step 1: initLogger
	code := a.initLogger()
	if code != 0 {
		t.Skipf("initLogger() returned %d (logDir not writable)", code)
	}
	defer a.log.Close()

	// Step 2: parseAppFlags
	code = a.parseAppFlags([]string{"--prune", "--force-prune", "homes"})
	if code != 0 {
		t.Fatalf("parseAppFlags returned %d, want 0", code)
	}

	// Verify mode derivation happened correctly
	if a.doBackup {
		t.Error("doBackup should be false for --prune")
	}
	if !a.doPrune {
		t.Error("doPrune should be true")
	}
	if a.backupLabel != "homes" {
		t.Errorf("backupLabel = %q, want 'homes'", a.backupLabel)
	}

	// Step 3: derivePaths
	if err := a.derivePaths(); err != nil {
		t.Fatalf("derivePaths() error: %v", err)
	}

	// Verify paths are populated
	if a.snapshotSource == "" {
		t.Error("snapshotSource should not be empty")
	}
	if a.configFile == "" {
		t.Error("configFile should not be empty")
	}
	if a.runner == nil {
		t.Error("runner should not be nil")
	}
	if a.lk == nil {
		t.Error("lock should not be nil")
	}
}

// TestIntegration_ParseAndDerive verifies that parseAppFlags output feeds
// correctly into derivePaths for different modes.
func TestIntegration_ParseAndDerive(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantBackup     bool
		wantRepoIsSnap bool // repositoryPath == snapshotTarget
	}{
		{"backup", []string{"homes"}, true, true},
		{"prune", []string{"--prune", "homes"}, false, false},
		{"fix-perms", []string{"--fix-perms", "homes"}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &app{log: itestLogger(t)}

			code := a.parseAppFlags(tt.args)
			if code != 0 {
				t.Fatalf("parseAppFlags returned %d", code)
			}

			if err := a.derivePaths(); err != nil {
				t.Fatalf("derivePaths() error: %v", err)
			}

			if a.doBackup != tt.wantBackup {
				t.Errorf("doBackup = %v, want %v", a.doBackup, tt.wantBackup)
			}

			if tt.wantRepoIsSnap {
				if a.repositoryPath != a.snapshotTarget {
					t.Errorf("repositoryPath = %q, want snapshotTarget %q", a.repositoryPath, a.snapshotTarget)
				}
			} else {
				if a.repositoryPath != a.snapshotSource {
					t.Errorf("repositoryPath = %q, want snapshotSource %q", a.repositoryPath, a.snapshotSource)
				}
			}
		})
	}
}

// TestIntegration_EarlyExitHelp verifies --help exits with exitHandled from
// the parseAppFlags sub-initializer (converted to 0 at the run() boundary).
func TestIntegration_EarlyExitHelp(t *testing.T) {
	a := &app{log: itestLogger(t)}

	// Redirect stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
		w.Close()
		r.Close()
	}()

	code := a.parseAppFlags([]string{"--help"})
	if code != exitHandled {
		t.Errorf("--help should return exitHandled (%d), got %d", exitHandled, code)
	}
	// flags should be nil (early exit before parse)
	if a.flags != nil {
		t.Error("flags should be nil for --help")
	}
}

// TestIntegration_EarlyExitVersion verifies --version exits with exitHandled.
func TestIntegration_EarlyExitVersion(t *testing.T) {
	a := &app{log: itestLogger(t)}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
		w.Close()
		r.Close()
	}()

	code := a.parseAppFlags([]string{"--version"})
	if code != exitHandled {
		t.Errorf("--version should return exitHandled (%d), got %d", exitHandled, code)
	}
}

// TestIntegration_ParseFlagErrorPropagation verifies that invalid flags
// are caught by parseAppFlags and return code 1.
func TestIntegration_ParseFlagErrorPropagation(t *testing.T) {
	a := &app{log: itestLogger(t)}
	code := a.parseAppFlags([]string{"--nonexistent"})
	if code != 1 {
		t.Errorf("invalid flag should return code 1, got %d", code)
	}
}

// TestIntegration_ValidateEnvironmentErrorPropagation verifies that
// validateEnvironment errors (non-root) are caught.
func TestIntegration_ValidateEnvironmentErrorPropagation(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test must run as non-root")
	}

	a := &app{log: itestLogger(t)}
	code := a.parseAppFlags([]string{"homes"})
	if code != 0 {
		t.Fatalf("parseAppFlags failed: code=%d", code)
	}

	err := a.validateEnvironment()
	if err == nil {
		t.Error("validateEnvironment() should fail for non-root")
	}
	if !strings.Contains(err.Error(), "root") {
		t.Errorf("error = %q, expected mention of root", err.Error())
	}
}

// TestIntegration_DerivePathsThenConfig verifies that after derivePaths,
// the config pipeline can still load and parse correctly.
func TestIntegration_DerivePathsThenConfig(t *testing.T) {
	confDir := t.TempDir()
	confContent := `[common]
PRUNE=-keep 1:728

[local]
DESTINATION=/volume2/backups
THREADS=4
`
	a := &app{log: itestLogger(t)}
	code := a.parseAppFlags([]string{"--prune", "homes"})
	if code != 0 {
		t.Fatalf("parseAppFlags failed: code=%d", code)
	}

	if err := a.derivePaths(); err != nil {
		t.Fatalf("derivePaths error: %v", err)
	}

	// Override configFile to point to our test config
	a.configFile = writeConfig(t, confDir, "homes", confContent)

	if err := a.loadConfig(); err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if a.cfg.Destination != "/volume2/backups" {
		t.Errorf("Destination = %q, want /volume2/backups", a.cfg.Destination)
	}
}
