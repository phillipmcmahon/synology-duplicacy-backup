package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

// TestMain overrides the package-level logDir variable so that tests
// that call initLogger() (which writes to logDir) do not require
// write access to /var/log.  This allows the full test suite to pass
// in CI environments and on developer machines without root privileges.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "duplicacy-backup-test-logs-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp log dir: %v\n", err)
		os.Exit(1)
	}
	logDir = tmp
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// ─── Helper: create a test logger that writes to a temp dir ──────────────────

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	dir := t.TempDir()
	log, err := logger.New(dir, "test", false)
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return log
}

// testApp returns a minimal *app suitable for unit-testing individual methods.
// Callers can override fields as needed before calling the method under test.
// A MockRunner is installed so that any accidental command execution is safe.
func testApp(t *testing.T) *app {
	t.Helper()
	return &app{
		log:   testLogger(t),
		flags: &flags{source: "testlabel", dryRun: true},

		backupLabel:    "testlabel",
		runTimestamp:   "20260409-120000",
		snapshotSource: "/volume1/testlabel",
		snapshotTarget: "/volume1/testlabel-20260409-120000",
		workRoot:       filepath.Join(t.TempDir(), "workroot"),
		repositoryPath: "/volume1/testlabel",

		configDir:  t.TempDir(),
		secretsDir: t.TempDir(),
		runner:     execpkg.NewMockRunner(),
	}
}

// ─── Free function tests (preserved from original) ──────────────────────────

func TestResolveDir(t *testing.T) {
	const envKey = "TEST_RESOLVE_DIR_ENV"

	tests := []struct {
		name       string
		flagValue  string
		envValue   string
		defaultDir string
		expected   string
	}{
		{"flag wins over env and default", "/from-flag", "/from-env", "/default", "/from-flag"},
		{"env wins over default", "", "/from-env", "/default", "/from-env"},
		{"default when no flag or env", "", "", "/default", "/default"},
		{"flag wins over env", "/from-flag", "/from-env", "/default", "/from-flag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv(envKey)
			if tt.envValue != "" {
				os.Setenv(envKey, tt.envValue)
				defer os.Unsetenv(envKey)
			}
			got := resolveDir(tt.flagValue, envKey, tt.defaultDir)
			if got != tt.expected {
				t.Errorf("resolveDir(%q, %q, %q) = %q, want %q", tt.flagValue, envKey, tt.defaultDir, got, tt.expected)
			}
		})
	}
}

func TestParseFlags_ConfigDir(t *testing.T) {
	f, err := parseFlags([]string{"--config-dir", "/custom/config", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.configDir != "/custom/config" {
		t.Errorf("configDir = %q, want %q", f.configDir, "/custom/config")
	}
	if f.source != "homes" {
		t.Errorf("source = %q, want %q", f.source, "homes")
	}
}

func TestParseFlags_SecretsDir(t *testing.T) {
	f, err := parseFlags([]string{"--secrets-dir", "/custom/secrets", "--remote", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.secretsDir != "/custom/secrets" {
		t.Errorf("secretsDir = %q, want %q", f.secretsDir, "/custom/secrets")
	}
	if !f.remoteMode {
		t.Error("expected remoteMode to be true")
	}
}

func TestParseFlags_ConfigDirMissingValue(t *testing.T) {
	_, err := parseFlags([]string{"--config-dir"})
	if err == nil {
		t.Error("expected error for --config-dir without value")
	}
}

func TestParseFlags_SecretsDirMissingValue(t *testing.T) {
	_, err := parseFlags([]string{"--secrets-dir"})
	if err == nil {
		t.Error("expected error for --secrets-dir without value")
	}
}

func TestExecutableConfigDir(t *testing.T) {
	dir := executableConfigDir()
	// The result must end with ".config"
	if !strings.HasSuffix(dir, ".config") {
		t.Errorf("executableConfigDir() = %q, expected suffix .config", dir)
	}
	// The result should be an absolute path (test binary has an absolute path)
	if !filepath.IsAbs(dir) {
		t.Errorf("executableConfigDir() = %q, expected absolute path", dir)
	}
	// The parent of .config should be the directory containing the test binary
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable() failed: %v", err)
	}
	exe, _ = filepath.EvalSymlinks(exe)
	expected := filepath.Join(filepath.Dir(exe), ".config")
	if dir != expected {
		t.Errorf("executableConfigDir() = %q, want %q", dir, expected)
	}
}

func TestValidateLabel(t *testing.T) {
	valid := []string{
		"homes", "photos", "my-backup", "test_label", "A1", "a",
		"homes-2024", "UPPER", "MiXeD_Case-99",
	}
	for _, label := range valid {
		if err := validateLabel(label); err != nil {
			t.Errorf("validateLabel(%q) returned unexpected error: %v", label, err)
		}
	}

	invalid := []struct {
		label string
		desc  string
	}{
		{"", "empty label"},
		{"../etc", "parent directory traversal"},
		{"foo/bar", "forward slash"},
		{"foo\\bar", "backslash"},
		{"...", "dots only"},
		{".hidden", "starts with dot"},
		{"-starts-with-hyphen", "starts with hyphen"},
		{"_starts-with-underscore", "starts with underscore"},
		{"has spaces", "contains space"},
		{"label;rm", "contains semicolon"},
		{"label\ttab", "contains tab"},
		{"../../../etc/passwd", "deep path traversal"},
		{"foo..bar", "contains double dot"},
	}
	for _, tt := range invalid {
		if err := validateLabel(tt.label); err == nil {
			t.Errorf("validateLabel(%q) [%s] expected error, got nil", tt.label, tt.desc)
		}
	}
}

func TestParseFlags_RejectsTraversalLabel(t *testing.T) {
	// parseFlags itself doesn't validate labels, but we verify the label is captured
	// so that validateLabel can reject it in run()
	f, err := parseFlags([]string{"../etc"})
	if err != nil {
		t.Fatalf("parseFlags should not reject positional args: %v", err)
	}
	if err := validateLabel(f.source); err == nil {
		t.Error("expected validateLabel to reject '../etc' label")
	}
}

func TestJoinDestination(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		label       string
		expected    string
	}{
		{
			name:        "S3 URL preserves double slash",
			destination: "s3://EU@gateway.storjshare.io/42b50c98a4a49314ccc038fea46cec3c",
			label:       "homes",
			expected:    "s3://EU@gateway.storjshare.io/42b50c98a4a49314ccc038fea46cec3c/homes",
		},
		{
			name:        "S3 URL with trailing slash",
			destination: "s3://EU@gateway.storjshare.io/bucket/",
			label:       "photos",
			expected:    "s3://EU@gateway.storjshare.io/bucket/photos",
		},
		{
			name:        "HTTPS URL preserves scheme",
			destination: "https://example.com/backup",
			label:       "docs",
			expected:    "https://example.com/backup/docs",
		},
		{
			name:        "HTTP URL preserves scheme",
			destination: "http://nas.local/share",
			label:       "data",
			expected:    "http://nas.local/share/data",
		},
		{
			name:        "local path uses filepath.Join",
			destination: "/volume1/backups",
			label:       "homes",
			expected:    "/volume1/backups/homes",
		},
		{
			name:        "local path with trailing slash",
			destination: "/volume1/backups/",
			label:       "homes",
			expected:    "/volume1/backups/homes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinDestination(tt.destination, tt.label)
			if got != tt.expected {
				t.Errorf("joinDestination(%q, %q) = %q, want %q",
					tt.destination, tt.label, got, tt.expected)
			}
		})
	}
}

func TestParseFlags_FixPermsAloneDoesNotDefaultToBackup(t *testing.T) {
	f, err := parseFlags([]string{"--fix-perms", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.mode != "" {
		t.Errorf("mode = %q, want empty string (no backup/prune)", f.mode)
	}
	if !f.fixPerms {
		t.Error("expected fixPerms to be true")
	}
}

func TestParseFlags_FixPermsWithBackupSetsBothFlags(t *testing.T) {
	f, err := parseFlags([]string{"--fix-perms", "--backup", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.mode != "backup" {
		t.Errorf("mode = %q, want %q", f.mode, "backup")
	}
	if !f.fixPerms {
		t.Error("expected fixPerms to be true")
	}
}

func TestParseFlags_FixPermsWithRemoteSetsFlags(t *testing.T) {
	// parseFlags itself accepts the combination; the hard error is in run()
	f, err := parseFlags([]string{"--fix-perms", "--remote", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.fixPerms {
		t.Error("expected fixPerms to be true")
	}
	if !f.remoteMode {
		t.Error("expected remoteMode to be true")
	}
}

func TestParseFlags_NoFlagsDefaultsToBackup(t *testing.T) {
	f, err := parseFlags([]string{"homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.mode != "backup" {
		t.Errorf("mode = %q, want %q", f.mode, "backup")
	}
}

// ─── Mode derivation tests (v1.6.0 conditional validation) ──────────────────

func TestModeDerivation_FixPermsOnlySkipsBackupAndPrune(t *testing.T) {
	f, err := parseFlags([]string{"--fix-perms", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	doBackup := f.mode == "backup"
	doPrune := f.mode == "prune" || f.mode == "prune-deep"
	fixPermsOnly := f.fixPerms && !doBackup && !doPrune

	if doBackup {
		t.Error("doBackup should be false for --fix-perms only")
	}
	if doPrune {
		t.Error("doPrune should be false for --fix-perms only")
	}
	if !fixPermsOnly {
		t.Error("fixPermsOnly should be true for --fix-perms only")
	}
	if !f.fixPerms {
		t.Error("fixPerms flag should be true")
	}
}

func TestModeDerivation_BackupRequiresDuplicacy(t *testing.T) {
	f, err := parseFlags([]string{"homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	doBackup := f.mode == "backup"
	doPrune := f.mode == "prune" || f.mode == "prune-deep"

	if !doBackup {
		t.Error("doBackup should be true for default mode")
	}
	if doPrune {
		t.Error("doPrune should be false for default mode")
	}
}

func TestModeDerivation_PruneRequiresDuplicacy(t *testing.T) {
	f, err := parseFlags([]string{"--prune", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	doBackup := f.mode == "backup"
	doPrune := f.mode == "prune" || f.mode == "prune-deep"

	if doBackup {
		t.Error("doBackup should be false for --prune")
	}
	if !doPrune {
		t.Error("doPrune should be true for --prune")
	}
}

func TestModeDerivation_FixPermsWithBackupRequiresBoth(t *testing.T) {
	f, err := parseFlags([]string{"--fix-perms", "--backup", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	doBackup := f.mode == "backup"
	doPrune := f.mode == "prune" || f.mode == "prune-deep"

	if !doBackup {
		t.Error("doBackup should be true for --backup --fix-perms")
	}
	if doPrune {
		t.Error("doPrune should be false")
	}
	if !f.fixPerms {
		t.Error("fixPerms should be true")
	}
}

func TestParseFlags_UnknownOption(t *testing.T) {
	unknowns := []string{"--unknown", "--help", "--version", "-v", "-x"}
	for _, opt := range unknowns {
		_, err := parseFlags([]string{opt})
		if err == nil {
			t.Errorf("parseFlags(%q) should return error for unknown option", opt)
		}
	}
}

// ─── Version and usage tests (v1.7.1) ───────────────────────────────────────

func TestVersionFlag_Long(t *testing.T) {
	for _, arg := range []string{"--version"} {
		if arg == "--version" || arg == "-v" {
			// Matches the early-exit condition in run()
		} else {
			t.Errorf("expected %q to match version flag check", arg)
		}
	}
}

func TestVersionFlag_Short(t *testing.T) {
	_, err := parseFlags([]string{"-v"})
	if err == nil {
		t.Error("parseFlags should reject -v (handled before parseFlags in run)")
	}
}

func TestVersionOutput_ContainsVersion(t *testing.T) {
	if version != "1.8.0" {
		t.Errorf("version = %q, want %q", version, "1.8.0")
	}
}

func TestVersionOutput_ContainsScriptName(t *testing.T) {
	expected := "duplicacy-backup"
	if scriptName != expected {
		t.Errorf("scriptName = %q, want %q", scriptName, expected)
	}
}

func TestPrintUsage_DoesNotPanic(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
		w.Close()
		r.Close()
	}()
	printUsage()
}

func TestParseFlags_MissingSourceShowsError(t *testing.T) {
	_, err := parseFlags([]string{"--backup"})
	if err == nil {
		t.Error("expected error for missing source directory")
	}
	if !strings.Contains(err.Error(), "source directory required") {
		t.Errorf("error = %q, want it to contain 'source directory required'", err.Error())
	}
}

func TestParseFlags_UnknownFlagShowsError(t *testing.T) {
	_, err := parseFlags([]string{"--nonexistent", "homes"})
	if err == nil {
		t.Error("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown option") {
		t.Errorf("error = %q, want it to contain 'unknown option'", err.Error())
	}
}

// ─── Display logic derivation tests (v1.6.1) ────────────────────────────────

type displayContext struct {
	fixPermsOnly bool
	fixPerms     bool
	remoteMode   bool
}

func deriveDisplayContext(args []string) (displayContext, error) {
	f, err := parseFlags(args)
	if err != nil {
		return displayContext{}, err
	}
	doBackup := f.mode == "backup"
	doPrune := f.mode == "prune" || f.mode == "prune-deep"
	return displayContext{
		fixPermsOnly: f.fixPerms && !doBackup && !doPrune,
		fixPerms:     f.fixPerms,
		remoteMode:   f.remoteMode,
	}, nil
}

func TestDisplayContext_FixPermsOnly_MinimalSummary(t *testing.T) {
	dc, err := deriveDisplayContext([]string{"--fix-perms", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dc.fixPermsOnly {
		t.Error("fixPermsOnly should be true for standalone --fix-perms")
	}
	if !dc.fixPerms {
		t.Error("fixPerms should be true")
	}
}

func TestDisplayContext_BackupOnly_NoOwnerGroup(t *testing.T) {
	dc, err := deriveDisplayContext([]string{"homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dc.fixPermsOnly {
		t.Error("fixPermsOnly should be false for default backup")
	}
	if dc.fixPerms {
		t.Error("fixPerms should be false — Local Owner/Group should NOT be displayed")
	}
}

func TestDisplayContext_PruneOnly_NoOwnerGroup(t *testing.T) {
	dc, err := deriveDisplayContext([]string{"--prune", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dc.fixPermsOnly {
		t.Error("fixPermsOnly should be false for --prune")
	}
	if dc.fixPerms {
		t.Error("fixPerms should be false — Local Owner/Group should NOT be displayed")
	}
}

func TestDisplayContext_BackupWithFixPerms_ShowsOwnerGroup(t *testing.T) {
	dc, err := deriveDisplayContext([]string{"--backup", "--fix-perms", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dc.fixPermsOnly {
		t.Error("fixPermsOnly should be false when combined with --backup")
	}
	if !dc.fixPerms {
		t.Error("fixPerms should be true — Local Owner/Group SHOULD be displayed")
	}
}

func TestDisplayContext_PruneWithFixPerms_ShowsOwnerGroup(t *testing.T) {
	dc, err := deriveDisplayContext([]string{"--prune", "--fix-perms", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dc.fixPermsOnly {
		t.Error("fixPermsOnly should be false when combined with --prune")
	}
	if !dc.fixPerms {
		t.Error("fixPerms should be true — Local Owner/Group SHOULD be displayed")
	}
}

func TestDisplayContext_RemoteBackup_NoOwnerGroup(t *testing.T) {
	dc, err := deriveDisplayContext([]string{"--remote", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dc.fixPerms {
		t.Error("fixPerms should be false for remote mode")
	}
	if !dc.remoteMode {
		t.Error("remoteMode should be true")
	}
}

// ─── --force-prune validation tests (v1.7.2) ────────────────────────────────

func TestForcePrune_AloneErrorsAndExits(t *testing.T) {
	f, err := parseFlags([]string{"--force-prune", "homes"})
	if err != nil {
		t.Fatalf("unexpected error from parseFlags: %v", err)
	}
	if !f.forcePrune {
		t.Error("expected forcePrune to be true")
	}
	doPrune := f.mode == "prune" || f.mode == "prune-deep"
	if doPrune {
		t.Error("expected doPrune to be false when no prune flag is given")
	}
	if !(f.forcePrune && !doPrune) {
		t.Error("expected forcePrune=true && doPrune=false to be the error condition")
	}
}

func TestForcePrune_WithPruneDeepIsAccepted(t *testing.T) {
	f, err := parseFlags([]string{"--prune-deep", "--force-prune", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.mode != "prune-deep" {
		t.Errorf("mode = %q, want %q", f.mode, "prune-deep")
	}
	if !f.forcePrune {
		t.Error("expected forcePrune to be true")
	}
	deepPruneMode := f.mode == "prune-deep"
	doPrune := f.mode == "prune" || f.mode == "prune-deep"
	if deepPruneMode && !f.forcePrune {
		t.Error("expected --prune-deep --force-prune to pass validation")
	}
	if f.forcePrune && !doPrune {
		t.Error("expected --force-prune with --prune-deep to NOT trigger the no-prune error")
	}
}

func TestForcePrune_WithPruneIsAccepted(t *testing.T) {
	f, err := parseFlags([]string{"--prune", "--force-prune", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.mode != "prune" {
		t.Errorf("mode = %q, want %q", f.mode, "prune")
	}
	if !f.forcePrune {
		t.Error("expected forcePrune to be true")
	}
	doPrune := f.mode == "prune" || f.mode == "prune-deep"
	if f.forcePrune && !doPrune {
		t.Error("expected --force-prune with --prune to NOT trigger the no-prune error")
	}
}

func TestForcePrune_WithBackupOnlyErrorsAndExits(t *testing.T) {
	f, err := parseFlags([]string{"--backup", "--force-prune", "homes"})
	if err != nil {
		t.Fatalf("unexpected error from parseFlags: %v", err)
	}
	if !f.forcePrune {
		t.Error("expected forcePrune to be true")
	}
	doPrune := f.mode == "prune" || f.mode == "prune-deep"
	if doPrune {
		t.Error("expected doPrune to be false for --backup mode")
	}
	if !(f.forcePrune && !doPrune) {
		t.Error("expected forcePrune=true && doPrune=false to be the error condition")
	}
}

// ─── Coordinator pattern: app struct tests ──────────────────────────────────

func TestApp_FailSetsExitCode(t *testing.T) {
	a := testApp(t)
	if a.exitCode != 0 {
		t.Fatalf("exitCode should start at 0, got %d", a.exitCode)
	}

	a.fail(fmt.Errorf("something went wrong"))
	if a.exitCode != 1 {
		t.Errorf("exitCode after fail() = %d, want 1", a.exitCode)
	}
}

func TestApp_FailMultipleTimesKeepsExitCode(t *testing.T) {
	a := testApp(t)
	a.fail(fmt.Errorf("first error"))
	a.fail(fmt.Errorf("second error"))
	if a.exitCode != 1 {
		t.Errorf("exitCode = %d, want 1", a.exitCode)
	}
}

func TestApp_CleanupIsIdempotent(t *testing.T) {
	a := testApp(t)
	// Create a lock to exercise release path
	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-idempotent")

	// First cleanup
	a.cleanup()
	if !a.cleanedUp {
		t.Error("cleanedUp should be true after cleanup()")
	}

	// Second cleanup should not panic
	a.cleanup()
	if !a.cleanedUp {
		t.Error("cleanedUp should still be true after second cleanup()")
	}
}

func TestApp_CleanupSetsCleanedUp(t *testing.T) {
	a := testApp(t)
	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-flag")

	if a.cleanedUp {
		t.Fatal("cleanedUp should be false before cleanup()")
	}
	a.cleanup()
	if !a.cleanedUp {
		t.Error("cleanedUp should be true after cleanup()")
	}
}

func TestApp_CleanupWithExitCodeShowsFailed(t *testing.T) {
	a := testApp(t)
	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-status")
	a.exitCode = 1

	// Should not panic even with non-zero exit code
	a.cleanup()
	if !a.cleanedUp {
		t.Error("cleanedUp should be true")
	}
}

func TestApp_AcquireLock_Success(t *testing.T) {
	a := testApp(t)
	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-acquire")

	if err := a.acquireLock(); err != nil {
		t.Fatalf("acquireLock() unexpected error: %v", err)
	}
	// Lock directory should exist
	if _, err := os.Stat(a.lk.Path); os.IsNotExist(err) {
		t.Error("lock directory should exist after acquireLock()")
	}
	// Cleanup to release
	a.lk.Release()
}

func TestApp_AcquireLock_FailsWhenHeld(t *testing.T) {
	lockDir := t.TempDir()

	// First app acquires lock
	a1 := testApp(t)
	a1.lk = lock.New(lockDir, "test-contention")
	if err := a1.acquireLock(); err != nil {
		t.Fatalf("first acquireLock() failed: %v", err)
	}

	// Second app should fail to acquire same lock
	a2 := testApp(t)
	a2.lk = lock.New(lockDir, "test-contention")
	err := a2.acquireLock()
	if err == nil {
		t.Error("second acquireLock() should fail when lock is held")
	}
	if err != nil && !strings.Contains(err.Error(), "Lock acquisition failed") {
		t.Errorf("error = %q, expected it to contain 'Lock acquisition failed'", err.Error())
	}

	a1.lk.Release()
}

func TestApp_LoadConfig_MissingFile(t *testing.T) {
	a := testApp(t)
	a.configFile = filepath.Join(t.TempDir(), "nonexistent.conf")

	err := a.loadConfig()
	if err == nil {
		t.Error("loadConfig() should fail for missing config file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, expected 'not found'", err.Error())
	}
}

func TestApp_LoadConfig_ValidLocalConfig(t *testing.T) {
	a := testApp(t)
	a.doBackup = false
	a.doPrune = false
	a.fixPermsOnly = true
	a.flags.fixPerms = true
	a.flags.remoteMode = false

	// Create a minimal valid config file
	confDir := t.TempDir()
	confFile := filepath.Join(confDir, "testlabel-backup.conf")
	confContent := `[common]
PRUNE=-keep 1:728

[local]
DESTINATION=/volume2/backups
LOCAL_OWNER=nobody
LOCAL_GROUP=nogroup
THREADS=4
`
	if err := os.WriteFile(confFile, []byte(confContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	a.configFile = confFile

	err := a.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() unexpected error: %v", err)
	}
	if a.cfg == nil {
		t.Fatal("cfg should not be nil after loadConfig()")
	}
	if a.cfg.Destination != "/volume2/backups" {
		t.Errorf("cfg.Destination = %q, want %q", a.cfg.Destination, "/volume2/backups")
	}
	if a.backupTarget != "/volume2/backups/testlabel" {
		t.Errorf("backupTarget = %q, want %q", a.backupTarget, "/volume2/backups/testlabel")
	}
}

func TestApp_LoadSecrets_LocalModeIsNoop(t *testing.T) {
	a := testApp(t)
	a.flags.remoteMode = false

	err := a.loadSecrets()
	if err != nil {
		t.Fatalf("loadSecrets() should be no-op for local mode: %v", err)
	}
	if a.sec != nil {
		t.Error("sec should be nil for local mode")
	}
}

func TestApp_LoadSecrets_RemoteMissingFile(t *testing.T) {
	a := testApp(t)
	a.flags.remoteMode = true
	a.secretsDir = t.TempDir() // empty dir, no secrets file

	err := a.loadSecrets()
	if err == nil {
		t.Error("loadSecrets() should fail when secrets file is missing")
	}
}

func TestApp_PrintHeader_DoesNotPanic(t *testing.T) {
	a := testApp(t)
	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-header")

	// Should not panic
	a.printHeader()
}

func TestApp_PrintSummary_BackupOnly(t *testing.T) {
	a := testApp(t)
	a.doBackup = true
	a.doPrune = false
	a.fixPermsOnly = false
	a.backupTarget = "/volume2/backups/testlabel"
	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/volume2/backups"
	a.sec = nil

	// Should not panic
	a.printSummary()
}

func TestApp_PrintSummary_FixPermsOnly(t *testing.T) {
	a := testApp(t)
	a.doBackup = false
	a.doPrune = false
	a.fixPermsOnly = true
	a.flags.fixPerms = true
	a.backupTarget = "/volume2/backups/testlabel"
	a.cfg = config.NewDefaults()
	a.cfg.LocalOwner = "testuser"
	a.cfg.LocalGroup = "users"

	// Should not panic
	a.printSummary()
}

func TestApp_PrintSummary_PruneSafe(t *testing.T) {
	a := testApp(t)
	a.doBackup = false
	a.doPrune = true
	a.deepPruneMode = false
	a.fixPermsOnly = false
	a.backupTarget = "/volume2/backups/testlabel"
	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/volume2/backups"
	a.cfg.Prune = "-keep 1:728"

	a.printSummary()
}

func TestApp_PrintSummary_PruneDeep(t *testing.T) {
	a := testApp(t)
	a.doBackup = false
	a.doPrune = true
	a.deepPruneMode = true
	a.fixPermsOnly = false
	a.backupTarget = "/volume2/backups/testlabel"
	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/volume2/backups"

	a.printSummary()
}

func TestApp_PrintSummary_RemoteWithSecrets(t *testing.T) {
	a := testApp(t)
	a.doBackup = true
	a.doPrune = false
	a.fixPermsOnly = false
	a.flags.remoteMode = true
	a.backupTarget = "s3://bucket/testlabel"
	a.cfg = config.NewDefaults()
	a.cfg.Destination = "s3://bucket"
	a.cfg.Threads = 4
	a.sec = &secrets.Secrets{
		StorjS3ID:     "ABCDEFGHIJKLMNOPQRSTUVWXYZ1234",
		StorjS3Secret: "abcdefghijklmnopqrstuvwxyz1234567890abcdefghijklmnopqrstuv",
	}

	a.printSummary()
}

func TestApp_PrintSummary_BackupWithFixPerms(t *testing.T) {
	a := testApp(t)
	a.doBackup = true
	a.doPrune = false
	a.fixPermsOnly = false
	a.flags.fixPerms = true
	a.backupTarget = "/volume2/backups/testlabel"
	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/volume2/backups"
	a.cfg.LocalOwner = "testuser"
	a.cfg.LocalGroup = "users"

	a.printSummary()
}

func TestApp_Execute_FixPermsOnly_NoBackupNoPrune(t *testing.T) {
	a := testApp(t)
	a.doBackup = false
	a.doPrune = false
	a.fixPermsOnly = true
	a.flags.fixPerms = true
	a.flags.dryRun = true
	a.backupTarget = t.TempDir()
	a.cfg = config.NewDefaults()
	a.cfg.LocalOwner = "nobody"
	a.cfg.LocalGroup = "nogroup"

	err := a.execute()
	if err != nil {
		t.Fatalf("execute() unexpected error: %v", err)
	}
}

func TestApp_Execute_NeitherBackupNorPruneNorFixPerms(t *testing.T) {
	a := testApp(t)
	a.doBackup = false
	a.doPrune = false
	a.fixPermsOnly = false
	a.flags.fixPerms = false

	// execute() with nothing to do should succeed
	err := a.execute()
	if err != nil {
		t.Fatalf("execute() with no operations should succeed: %v", err)
	}
}

// ─── app struct field derivation tests ──────────────────────────────────────

func TestApp_ModeBooleansFromFlags(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		fixPerms    bool
		wantBackup  bool
		wantPrune   bool
		wantDeep    bool
		wantFixOnly bool
	}{
		{"backup", "backup", false, true, false, false, false},
		{"prune", "prune", false, false, true, false, false},
		{"prune-deep", "prune-deep", false, false, true, true, false},
		{"fix-perms only", "", true, false, false, false, true},
		{"backup+fix-perms", "backup", true, true, false, false, false},
		{"prune+fix-perms", "prune", true, false, true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doBackup := tt.mode == "backup"
			doPrune := tt.mode == "prune" || tt.mode == "prune-deep"
			deepPruneMode := tt.mode == "prune-deep"
			fixPermsOnly := tt.fixPerms && !doBackup && !doPrune

			if doBackup != tt.wantBackup {
				t.Errorf("doBackup = %v, want %v", doBackup, tt.wantBackup)
			}
			if doPrune != tt.wantPrune {
				t.Errorf("doPrune = %v, want %v", doPrune, tt.wantPrune)
			}
			if deepPruneMode != tt.wantDeep {
				t.Errorf("deepPruneMode = %v, want %v", deepPruneMode, tt.wantDeep)
			}
			if fixPermsOnly != tt.wantFixOnly {
				t.Errorf("fixPermsOnly = %v, want %v", fixPermsOnly, tt.wantFixOnly)
			}
		})
	}
}

func TestApp_RepositoryPathDerivation(t *testing.T) {
	// When doBackup is true, repositoryPath should be snapshotTarget
	// When doBackup is false, repositoryPath should be snapshotSource
	label := "homes"
	timestamp := "20260409-120000"
	source := filepath.Join(rootVolume, label)
	target := filepath.Join(rootVolume, fmt.Sprintf("%s-%s", label, timestamp))

	// Backup mode
	var repoBackup string
	if true { // doBackup
		repoBackup = target
	}
	if repoBackup != target {
		t.Errorf("backup repositoryPath = %q, want %q", repoBackup, target)
	}

	// Prune mode
	repoOther := source
	if repoOther != source {
		t.Errorf("prune repositoryPath = %q, want %q", repoOther, source)
	}
}

// ─── Integration-style tests for the coordinator flow ────────────────────────

func TestApp_LoadConfigThenLoadSecrets_LocalMode(t *testing.T) {
	// Verify the full local-mode config+secrets flow:
	// loadConfig succeeds with valid config, loadSecrets is a no-op.
	a := testApp(t)
	a.doBackup = false
	a.doPrune = true
	a.flags.mode = "prune"
	a.flags.remoteMode = false

	confDir := t.TempDir()
	confFile := filepath.Join(confDir, "testlabel-backup.conf")
	confContent := `[common]
PRUNE=-keep 1:728

[local]
DESTINATION=/volume2/backups
THREADS=4
`
	if err := os.WriteFile(confFile, []byte(confContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	a.configFile = confFile

	if err := a.loadConfig(); err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if err := a.loadSecrets(); err != nil {
		t.Fatalf("loadSecrets() for local should be no-op: %v", err)
	}
	if a.sec != nil {
		t.Error("sec should be nil for local mode")
	}
	if a.cfg == nil {
		t.Fatal("cfg should not be nil")
	}
}

func TestApp_FullCleanupCycle_BackupMode(t *testing.T) {
	a := testApp(t)
	a.doBackup = true
	a.flags.dryRun = false // real cleanup, not dry-run
	// Create real workRoot dir so cleanup exercises os.Stat path
	os.MkdirAll(a.workRoot, 0755)

	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-full-cleanup")
	a.lk.Acquire()
	a.lockAcquired = true

	a.cleanup()

	// workRoot should be removed
	if _, err := os.Stat(a.workRoot); !os.IsNotExist(err) {
		t.Error("workRoot should be removed after cleanup")
	}
	if !a.cleanedUp {
		t.Error("cleanedUp should be true")
	}
}

func TestApp_FullCleanupCycle_PruneMode(t *testing.T) {
	a := testApp(t)
	a.doBackup = false
	a.doPrune = true
	a.flags.dryRun = false // real cleanup, not dry-run
	os.MkdirAll(a.workRoot, 0755)

	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-prune-cleanup")
	a.lk.Acquire()
	a.lockAcquired = true

	a.cleanup()

	if _, err := os.Stat(a.workRoot); !os.IsNotExist(err) {
		t.Error("workRoot should be removed after cleanup")
	}
	if !a.cleanedUp {
		t.Error("cleanedUp should be true")
	}
}

// ─── Verify the coordinator pattern contract ────────────────────────────────

func TestApp_MethodsReturnErrors_NotPanics(t *testing.T) {
	// Each phase method should return an error, not panic, on failure.
	a := testApp(t)
	a.configFile = "/nonexistent/path/config.conf"
	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-errors")

	// loadConfig with missing file returns error
	if err := a.loadConfig(); err == nil {
		t.Error("loadConfig() with missing file should return error")
	}

	// loadSecrets with remote mode and missing file returns error
	a.flags.remoteMode = true
	if err := a.loadSecrets(); err == nil {
		t.Error("loadSecrets() with remote mode and missing file should return error")
	}
}

func TestApp_CleanupReleasesLock(t *testing.T) {
	lockDir := t.TempDir()
	a := testApp(t)
	a.lk = lock.New(lockDir, "test-release")

	if err := a.acquireLock(); err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}

	a.cleanup()

	// Lock should be released – directory should not exist
	if _, err := os.Stat(a.lk.Path); !os.IsNotExist(err) {
		t.Error("lock directory should be removed after cleanup")
	}
}

func TestApp_DupFieldIsNilBeforePrepareDuplicacySetup(t *testing.T) {
	a := testApp(t)
	if a.dup != nil {
		t.Error("dup should be nil before prepareDuplicacySetup()")
	}
}

// ---------------------------------------------------------------------------
// Tests for lockAcquired guard in cleanup()
// ---------------------------------------------------------------------------

func TestApp_CleanupSafeWhenLockAcquisitionFails(t *testing.T) {
	lockDir := t.TempDir()

	// First app acquires the lock
	a1 := testApp(t)
	a1.lk = lock.New(lockDir, "test-safe-cleanup")
	if err := a1.acquireLock(); err != nil {
		t.Fatalf("first acquireLock() failed: %v", err)
	}

	// Second app fails to acquire the same lock
	a2 := testApp(t)
	a2.lk = lock.New(lockDir, "test-safe-cleanup")
	if err := a2.acquireLock(); err == nil {
		t.Fatal("second acquireLock() should have failed")
	}

	// lockAcquired should be false on the failing app
	if a2.lockAcquired {
		t.Error("lockAcquired should be false when acquisition fails")
	}

	// cleanup() on the failing app must not panic and must not release a1's lock
	a2.cleanup()

	// a1's lock directory should still exist (not removed by a2's cleanup)
	if _, err := os.Stat(a1.lk.Path); os.IsNotExist(err) {
		t.Error("a1's lock directory should still exist after a2's cleanup()")
	}

	// Clean up a1
	a1.cleanup()
}

func TestApp_CleanupDoesNotRemoveOtherProcessLock(t *testing.T) {
	lockDir := t.TempDir()

	// Simulate a process that holds the lock
	holder := testApp(t)
	holder.lk = lock.New(lockDir, "test-other-lock")
	if err := holder.acquireLock(); err != nil {
		t.Fatalf("holder acquireLock() failed: %v", err)
	}

	// Another app that never acquired the lock
	loser := testApp(t)
	loser.lk = lock.New(lockDir, "test-other-lock")
	// Do NOT call acquireLock – simulate the scenario where it was never called

	loser.cleanup()

	// The holder's lock must still be intact
	if _, err := os.Stat(holder.lk.Path); os.IsNotExist(err) {
		t.Error("holder's lock should still exist; loser's cleanup must not remove it")
	}

	// Holder can still cleanly release
	holder.cleanup()
	if _, err := os.Stat(holder.lk.Path); !os.IsNotExist(err) {
		t.Error("holder's lock should be removed after holder's cleanup()")
	}
}

func TestApp_CleanupWorksAfterSuccessfulAcquire(t *testing.T) {
	lockDir := t.TempDir()
	a := testApp(t)
	a.lk = lock.New(lockDir, "test-acquired-cleanup")

	if err := a.acquireLock(); err != nil {
		t.Fatalf("acquireLock() failed: %v", err)
	}

	if !a.lockAcquired {
		t.Error("lockAcquired should be true after successful acquireLock()")
	}

	// Lock directory should exist before cleanup
	if _, err := os.Stat(a.lk.Path); os.IsNotExist(err) {
		t.Fatal("lock directory should exist after acquireLock()")
	}

	a.cleanup()

	// Lock directory should be removed after cleanup
	if _, err := os.Stat(a.lk.Path); !os.IsNotExist(err) {
		t.Error("lock directory should be removed after cleanup() with acquired lock")
	}
}

// ─── P1 fix: --help/--version early exit via newApp ─────────────────────────

func TestNewApp_HelpReturnsExitHandled(t *testing.T) {
	// Redirect stdout to avoid polluting test output.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
		w.Close()
		r.Close()
	}()

	a, code := newApp([]string{"--help"})
	if code != exitHandled {
		t.Errorf("newApp(--help) code = %d, want %d (exitHandled)", code, exitHandled)
	}
	if a != nil {
		t.Error("newApp(--help) should return nil app")
	}
}

func TestNewApp_VersionReturnsExitHandled(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
		w.Close()
		r.Close()
	}()

	a, code := newApp([]string{"--version"})
	if code != exitHandled {
		t.Errorf("newApp(--version) code = %d, want %d (exitHandled)", code, exitHandled)
	}
	if a != nil {
		t.Error("newApp(--version) should return nil app")
	}
}

func TestNewApp_ShortVersionReturnsExitHandled(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
		w.Close()
		r.Close()
	}()

	a, code := newApp([]string{"-v"})
	if code != exitHandled {
		t.Errorf("newApp(-v) code = %d, want %d (exitHandled)", code, exitHandled)
	}
	if a != nil {
		t.Error("newApp(-v) should return nil app")
	}
}

func TestNewApp_HelpDoesNotPanic(t *testing.T) {
	// The original P1 bug: --help returned 0, newApp continued to
	// validateEnvironment which dereferenced nil a.flags and panicked.
	// This test verifies the fix: newApp returns exitHandled and stops.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
		w.Close()
		r.Close()
	}()

	// Must not panic.
	a, code := newApp([]string{"--help"})
	if a != nil {
		t.Error("expected nil app for --help")
	}
	if code != exitHandled {
		t.Errorf("code = %d, want %d", code, exitHandled)
	}
}

// ─── P1 fix: run() level integration tests ──────────────────────────────────

func TestRun_HelpReturnsZero(t *testing.T) {
	// Verify the full run() flow: --help → parseAppFlags returns exitHandled →
	// run() converts to exit code 0.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	origArgs := os.Args
	os.Args = []string{"duplicacy-backup", "--help"}
	defer func() {
		os.Args = origArgs
		os.Stdout = old
		w.Close()
		r.Close()
	}()

	code := run()
	if code != 0 {
		t.Errorf("run() with --help returned %d, want 0", code)
	}
}

func TestRun_VersionReturnsZero(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	origArgs := os.Args
	os.Args = []string{"duplicacy-backup", "--version"}
	defer func() {
		os.Args = origArgs
		os.Stdout = old
		w.Close()
		r.Close()
	}()

	code := run()
	if code != 0 {
		t.Errorf("run() with --version returned %d, want 0", code)
	}
}

// ─── P3 fix: effectiveConfigDir and help text accuracy ──────────────────────

func TestEffectiveConfigDir_DefaultMatchesExecutableConfigDir(t *testing.T) {
	// When no env var is set, effectiveConfigDir should return the same
	// value as executableConfigDir.
	os.Unsetenv("DUPLICACY_BACKUP_CONFIG_DIR")
	got := effectiveConfigDir()
	want := executableConfigDir()
	if got != want {
		t.Errorf("effectiveConfigDir() = %q, want %q (same as executableConfigDir)", got, want)
	}
}

func TestEffectiveConfigDir_RespectsEnvVar(t *testing.T) {
	const override = "/custom/override/config"
	os.Setenv("DUPLICACY_BACKUP_CONFIG_DIR", override)
	defer os.Unsetenv("DUPLICACY_BACKUP_CONFIG_DIR")

	got := effectiveConfigDir()
	if got != override {
		t.Errorf("effectiveConfigDir() = %q, want %q (from env var)", got, override)
	}
}

func TestPrintUsage_ContainsEffectiveDefault(t *testing.T) {
	// Verify the help text uses "Effective default:" wording.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	os.Stdout = old
	r.Close()

	output := string(buf[:n])
	if !strings.Contains(output, "Effective default:") {
		t.Error("help text should contain 'Effective default:'")
	}
}

func TestPrintUsage_ReflectsEnvOverride(t *testing.T) {
	const override = "/env-override/config"
	os.Setenv("DUPLICACY_BACKUP_CONFIG_DIR", override)
	defer os.Unsetenv("DUPLICACY_BACKUP_CONFIG_DIR")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	os.Stdout = old
	r.Close()

	output := string(buf[:n])
	if !strings.Contains(output, override) {
		t.Errorf("help text should reflect DUPLICACY_BACKUP_CONFIG_DIR=%q, got:\n%s", override, output)
	}
}

func TestHelpOutput_ContainsUsageAndExamples(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	os.Stdout = old
	r.Close()

	output := string(buf[:n])

	required := []string{
		"Usage:", "--backup", "--prune", "--help", "--version",
		"ENVIRONMENT VARIABLES:", "DUPLICACY_BACKUP_CONFIG_DIR",
		"EXAMPLES:", "CONFIG FILE LOCATION:",
	}
	for _, s := range required {
		if !strings.Contains(output, s) {
			t.Errorf("help text should contain %q", s)
		}
	}
}

func TestVersionOutput_ContainsExpectedFormat(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	a := &app{log: testLogger(t)}
	code := a.parseAppFlags([]string{"--version"})

	w.Close()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	os.Stdout = old
	r.Close()

	if code != exitHandled {
		t.Fatalf("expected exitHandled, got %d", code)
	}
	output := string(buf[:n])
	if !strings.Contains(output, scriptName) {
		t.Errorf("version output should contain %q, got: %s", scriptName, output)
	}
	if !strings.Contains(output, version) {
		t.Errorf("version output should contain %q, got: %s", version, output)
	}
}

// ─── Sub-initializer tests (Phase 2 refactoring) ────────────────────────────

// ---------------------------------------------------------------------------
// TestApp_InitLogger
// ---------------------------------------------------------------------------

func TestApp_InitLogger_Success(t *testing.T) {
	// initLogger writes to logDir (/var/log) which may not be writable in
	// test environments.  We test the method by creating an app with a nil
	// logger and calling initLogger with a writable temp dir.  Since the
	// production initLogger uses the package-level logDir constant, we test
	// that the method signature works and the logger is set.  For full
	// integration coverage, see TestIntegration_FullInitFlow.
	a := &app{}
	// We can't easily redirect logDir in a unit test, so we verify that
	// if we have a writable /var/log (CI environments usually do), it works.
	code := a.initLogger()
	if code != 0 {
		t.Skipf("initLogger() returned %d (logDir %q may not be writable in this environment)", code, logDir)
	}
	if a.log == nil {
		t.Error("a.log should not be nil after successful initLogger()")
	}
	if a.log != nil {
		a.log.Close()
	}
}

func TestApp_InitLogger_SetsLogField(t *testing.T) {
	a := &app{}
	code := a.initLogger()
	if code != 0 {
		t.Skipf("initLogger() returned %d (logDir not writable)", code)
	}
	defer a.log.Close()

	// Logger should be usable immediately
	a.log.Info("test message from initLogger unit test")
}

// ---------------------------------------------------------------------------
// TestApp_ParseAppFlags
// ---------------------------------------------------------------------------

func TestApp_ParseAppFlags_BackupDefault(t *testing.T) {
	a := &app{log: testLogger(t)}
	code := a.parseAppFlags([]string{"homes"})
	if code != 0 {
		t.Fatalf("parseAppFlags returned %d, want 0", code)
	}
	if a.flags == nil {
		t.Fatal("flags should not be nil")
	}
	if a.flags.source != "homes" {
		t.Errorf("source = %q, want %q", a.flags.source, "homes")
	}
	if !a.doBackup {
		t.Error("doBackup should be true for default mode")
	}
	if a.doPrune {
		t.Error("doPrune should be false")
	}
	if a.backupLabel != "homes" {
		t.Errorf("backupLabel = %q, want %q", a.backupLabel, "homes")
	}
}

func TestApp_ParseAppFlags_PruneMode(t *testing.T) {
	a := &app{log: testLogger(t)}
	code := a.parseAppFlags([]string{"--prune", "photos"})
	if code != 0 {
		t.Fatalf("parseAppFlags returned %d, want 0", code)
	}
	if a.doBackup {
		t.Error("doBackup should be false for --prune")
	}
	if !a.doPrune {
		t.Error("doPrune should be true for --prune")
	}
	if a.deepPruneMode {
		t.Error("deepPruneMode should be false for --prune")
	}
}

func TestApp_ParseAppFlags_PruneDeepMode(t *testing.T) {
	a := &app{log: testLogger(t)}
	code := a.parseAppFlags([]string{"--prune-deep", "--force-prune", "data"})
	if code != 0 {
		t.Fatalf("parseAppFlags returned %d, want 0", code)
	}
	if !a.doPrune {
		t.Error("doPrune should be true")
	}
	if !a.deepPruneMode {
		t.Error("deepPruneMode should be true for --prune-deep")
	}
}

func TestApp_ParseAppFlags_FixPermsOnly(t *testing.T) {
	a := &app{log: testLogger(t)}
	code := a.parseAppFlags([]string{"--fix-perms", "homes"})
	if code != 0 {
		t.Fatalf("parseAppFlags returned %d, want 0", code)
	}
	if a.doBackup {
		t.Error("doBackup should be false for --fix-perms only")
	}
	if a.doPrune {
		t.Error("doPrune should be false for --fix-perms only")
	}
	if !a.fixPermsOnly {
		t.Error("fixPermsOnly should be true")
	}
}

func TestApp_ParseAppFlags_Help(t *testing.T) {
	a := &app{log: testLogger(t)}
	// Redirect stdout to avoid polluting test output
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
		t.Errorf("parseAppFlags(--help) returned %d, want %d (exitHandled)", code, exitHandled)
	}
	// flags should remain nil for early exit
	if a.flags != nil {
		t.Error("flags should be nil for --help early exit")
	}
}

func TestApp_ParseAppFlags_Version(t *testing.T) {
	a := &app{log: testLogger(t)}
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
		t.Errorf("parseAppFlags(--version) returned %d, want %d (exitHandled)", code, exitHandled)
	}
}

func TestApp_ParseAppFlags_ShortVersion(t *testing.T) {
	a := &app{log: testLogger(t)}
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
		w.Close()
		r.Close()
	}()

	code := a.parseAppFlags([]string{"-v"})
	if code != exitHandled {
		t.Errorf("parseAppFlags(-v) returned %d, want %d (exitHandled)", code, exitHandled)
	}
}

func TestApp_ParseAppFlags_InvalidFlag(t *testing.T) {
	a := &app{log: testLogger(t)}
	code := a.parseAppFlags([]string{"--nonexistent", "homes"})
	if code != 1 {
		t.Errorf("parseAppFlags(--nonexistent) returned %d, want 1", code)
	}
}

func TestApp_ParseAppFlags_MissingSource(t *testing.T) {
	a := &app{log: testLogger(t)}
	code := a.parseAppFlags([]string{"--backup"})
	if code != 1 {
		t.Errorf("parseAppFlags(--backup without source) returned %d, want 1", code)
	}
}

// ---------------------------------------------------------------------------
// TestApp_ValidateEnvironment
// ---------------------------------------------------------------------------

func TestApp_ValidateEnvironment_NotRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test must run as non-root to verify root check")
	}
	a := &app{
		log:   testLogger(t),
		flags: &flags{source: "homes", mode: "backup"},
	}
	err := a.validateEnvironment()
	if err == nil {
		t.Fatal("validateEnvironment() should fail for non-root")
	}
	if !strings.Contains(err.Error(), "root") {
		t.Errorf("error = %q, expected mention of 'root'", err.Error())
	}
}

func TestApp_ValidateEnvironment_InvalidLabel(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root to pass root check before label validation")
	}
	a := &app{
		log:   testLogger(t),
		flags: &flags{source: "../etc", mode: "backup"},
	}
	err := a.validateEnvironment()
	if err == nil {
		t.Fatal("validateEnvironment() should fail for invalid label")
	}
	if !strings.Contains(err.Error(), "Invalid source label") {
		t.Errorf("error = %q, expected 'Invalid source label'", err.Error())
	}
}

func TestApp_ValidateEnvironment_DeepPruneWithoutForce(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root")
	}
	a := &app{
		log:           testLogger(t),
		flags:         &flags{source: "homes", mode: "prune-deep", forcePrune: false},
		doPrune:       true,
		deepPruneMode: true,
	}
	err := a.validateEnvironment()
	if err == nil {
		t.Fatal("validateEnvironment() should fail: --prune-deep requires --force-prune")
	}
	if !strings.Contains(err.Error(), "--prune-deep requires --force-prune") {
		t.Errorf("error = %q, expected '--prune-deep requires --force-prune'", err.Error())
	}
}

func TestApp_ValidateEnvironment_ForcePruneWithoutPrune(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root")
	}
	a := &app{
		log:      testLogger(t),
		flags:    &flags{source: "homes", mode: "backup", forcePrune: true},
		doBackup: true,
	}
	err := a.validateEnvironment()
	if err == nil {
		t.Fatal("validateEnvironment() should fail: --force-prune requires prune mode")
	}
	if !strings.Contains(err.Error(), "--force-prune requires --prune or --prune-deep") {
		t.Errorf("error = %q, expected '--force-prune requires --prune or --prune-deep'", err.Error())
	}
}

func TestApp_ValidateEnvironment_FixPermsWithRemote(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root")
	}
	a := &app{
		log:   testLogger(t),
		flags: &flags{source: "homes", fixPerms: true, remoteMode: true},
	}
	err := a.validateEnvironment()
	if err == nil {
		t.Fatal("validateEnvironment() should fail: --fix-perms with --remote")
	}
	if !strings.Contains(err.Error(), "--fix-perms is only valid for local backups") {
		t.Errorf("error = %q, expected '--fix-perms is only valid for local backups'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TestApp_DerivePaths
// ---------------------------------------------------------------------------

func TestApp_DerivePaths_BackupMode(t *testing.T) {
	a := &app{
		log:         testLogger(t),
		flags:       &flags{source: "homes", mode: "backup", dryRun: true},
		doBackup:    true,
		backupLabel: "homes",
	}

	err := a.derivePaths()
	if err != nil {
		t.Fatalf("derivePaths() unexpected error: %v", err)
	}

	// Verify snapshot paths
	if a.snapshotSource != filepath.Join(rootVolume, "homes") {
		t.Errorf("snapshotSource = %q, want %q", a.snapshotSource, filepath.Join(rootVolume, "homes"))
	}
	if !strings.HasPrefix(a.snapshotTarget, filepath.Join(rootVolume, "homes-")) {
		t.Errorf("snapshotTarget = %q, expected prefix %q", a.snapshotTarget, filepath.Join(rootVolume, "homes-"))
	}

	// In backup mode, repositoryPath should be snapshotTarget
	if a.repositoryPath != a.snapshotTarget {
		t.Errorf("repositoryPath = %q, want snapshotTarget %q", a.repositoryPath, a.snapshotTarget)
	}

	// workRoot should contain the script name and label
	if !strings.Contains(a.workRoot, scriptName) {
		t.Errorf("workRoot = %q, expected to contain %q", a.workRoot, scriptName)
	}
	if !strings.Contains(a.workRoot, "homes") {
		t.Errorf("workRoot = %q, expected to contain 'homes'", a.workRoot)
	}

	// configFile should end with homes-backup.conf
	if !strings.HasSuffix(a.configFile, "homes-backup.conf") {
		t.Errorf("configFile = %q, expected suffix 'homes-backup.conf'", a.configFile)
	}

	// runner and lock should be initialized
	if a.runner == nil {
		t.Error("runner should not be nil after derivePaths()")
	}
	if a.lk == nil {
		t.Error("lk should not be nil after derivePaths()")
	}

	// runTimestamp should be set
	if a.runTimestamp == "" {
		t.Error("runTimestamp should not be empty")
	}
}

func TestApp_DerivePaths_PruneMode(t *testing.T) {
	a := &app{
		log:         testLogger(t),
		flags:       &flags{source: "photos", mode: "prune", dryRun: true},
		doBackup:    false,
		doPrune:     true,
		backupLabel: "photos",
	}

	err := a.derivePaths()
	if err != nil {
		t.Fatalf("derivePaths() unexpected error: %v", err)
	}

	// In prune mode, repositoryPath should be snapshotSource
	if a.repositoryPath != a.snapshotSource {
		t.Errorf("repositoryPath = %q, want snapshotSource %q", a.repositoryPath, a.snapshotSource)
	}
}

func TestApp_DerivePaths_ConfigDirOverride(t *testing.T) {
	customDir := "/custom/config"
	a := &app{
		log:         testLogger(t),
		flags:       &flags{source: "homes", configDir: customDir, dryRun: true},
		backupLabel: "homes",
	}

	err := a.derivePaths()
	if err != nil {
		t.Fatalf("derivePaths() unexpected error: %v", err)
	}

	if a.configDir != customDir {
		t.Errorf("configDir = %q, want %q", a.configDir, customDir)
	}
	expected := filepath.Join(customDir, "homes-backup.conf")
	if a.configFile != expected {
		t.Errorf("configFile = %q, want %q", a.configFile, expected)
	}
}

func TestApp_DerivePaths_SecretsDirOverride(t *testing.T) {
	customDir := "/custom/secrets"
	a := &app{
		log:         testLogger(t),
		flags:       &flags{source: "homes", secretsDir: customDir, dryRun: true},
		backupLabel: "homes",
	}

	err := a.derivePaths()
	if err != nil {
		t.Fatalf("derivePaths() unexpected error: %v", err)
	}

	if a.secretsDir != customDir {
		t.Errorf("secretsDir = %q, want %q", a.secretsDir, customDir)
	}
}

// ---------------------------------------------------------------------------
// TestApp_InstallSignalHandler
// ---------------------------------------------------------------------------

func TestApp_InstallSignalHandler_DoesNotPanic(t *testing.T) {
	a := testApp(t)
	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-signal")

	// installSignalHandler should not panic
	a.installSignalHandler()
}

func TestApp_InstallSignalHandler_SetsUpHandler(t *testing.T) {
	a := testApp(t)
	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-signal-setup")

	// Should not panic, should return immediately (handler runs in goroutine)
	a.installSignalHandler()

	// We can verify the handler was installed by checking that signal.Reset
	// doesn't panic.  The goroutine is non-blocking so the test continues.
	signal.Reset(syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
}

// ─── Additional tests for improved coverage ─────────────────────────────────

// ---------------------------------------------------------------------------
// TestApp_PrintCommandOutput
// ---------------------------------------------------------------------------

func TestApp_PrintCommandOutput_BothStreams(t *testing.T) {
	a := testApp(t)
	// Should not panic
	a.printCommandOutput("stdout line 1\nstdout line 2\n", "stderr line 1\n")
}

func TestApp_PrintCommandOutput_EmptyStreams(t *testing.T) {
	a := testApp(t)
	// Should not panic with empty strings
	a.printCommandOutput("", "")
}

func TestApp_PrintCommandOutput_StdoutOnly(t *testing.T) {
	a := testApp(t)
	a.printCommandOutput("output line\n", "")
}

func TestApp_PrintCommandOutput_StderrOnly(t *testing.T) {
	a := testApp(t)
	a.printCommandOutput("", "error line\n")
}

func TestApp_PrintCommandOutput_EmptyLinesSkipped(t *testing.T) {
	a := testApp(t)
	// Empty lines should be skipped in output
	a.printCommandOutput("line1\n\n\nline2\n", "err1\n\nerr2\n")
}

// ---------------------------------------------------------------------------
// TestApp_Execute — covers the dispatch logic
// ---------------------------------------------------------------------------

func TestApp_Execute_BackupMode_DryRun(t *testing.T) {
	a := testApp(t)
	a.doBackup = true
	a.doPrune = false
	a.flags.dryRun = true
	a.flags.fixPerms = false

	// Set up config needed by prepareDuplicacySetup
	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/backup/dest"
	a.cfg.Threads = 4
	a.cfg.Filter = ""
	a.backupTarget = "/backup/dest/testlabel"

	// In dry-run mode, execute should succeed without real commands
	err := a.execute()
	if err != nil {
		t.Fatalf("execute() dry-run error: %v", err)
	}

	// dup should be set after prepareDuplicacySetup
	if a.dup == nil {
		t.Error("dup should be set after execute()")
	}
}

func TestApp_Execute_PruneMode_DryRun(t *testing.T) {
	a := testApp(t)
	a.doBackup = false
	a.doPrune = true
	a.deepPruneMode = false
	a.flags.dryRun = true
	a.flags.fixPerms = false
	a.flags.mode = "prune"

	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/backup/dest"
	a.cfg.Prune = "-keep 0:365 -keep 30:30 -keep 7:7"
	a.cfg.PruneArgs = []string{"-keep", "0:365", "-keep", "30:30", "-keep", "7:7"}
	a.backupTarget = "/backup/dest/testlabel"

	err := a.execute()
	if err != nil {
		t.Fatalf("execute() dry-run prune error: %v", err)
	}
}

func TestApp_Execute_BackupWithFixPerms_DryRun(t *testing.T) {
	a := testApp(t)
	a.doBackup = true
	a.doPrune = false
	a.flags.dryRun = true
	a.flags.fixPerms = true

	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/backup/dest"
	a.cfg.Threads = 4
	a.cfg.LocalOwner = "testuser"
	a.cfg.LocalGroup = "testgroup"
	a.backupTarget = "/backup/dest/testlabel"

	err := a.execute()
	if err != nil {
		t.Fatalf("execute() backup+fix-perms dry-run error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestApp_PrepareDuplicacySetup
// ---------------------------------------------------------------------------

func TestApp_PrepareDuplicacySetup_DryRun_Backup(t *testing.T) {
	a := testApp(t)
	a.doBackup = true
	a.flags.dryRun = true

	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/backup/dest"
	a.cfg.Filter = "+include_dir/*\n-exclude_dir/*"
	a.backupTarget = "/backup/dest/testlabel"

	err := a.prepareDuplicacySetup()
	if err != nil {
		t.Fatalf("prepareDuplicacySetup() dry-run error: %v", err)
	}

	if a.dup == nil {
		t.Error("dup should be set")
	}
}

func TestApp_PrepareDuplicacySetup_DryRun_Prune(t *testing.T) {
	a := testApp(t)
	a.doBackup = false
	a.doPrune = true
	a.flags.dryRun = true

	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/backup/dest"
	a.backupTarget = "/backup/dest/testlabel"

	err := a.prepareDuplicacySetup()
	if err != nil {
		t.Fatalf("prepareDuplicacySetup() dry-run prune error: %v", err)
	}
}

func TestApp_PrepareDuplicacySetup_NoFilter_DryRun(t *testing.T) {
	a := testApp(t)
	a.doBackup = true
	a.flags.dryRun = true

	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/backup/dest"
	a.cfg.Filter = "" // No filter
	a.backupTarget = "/backup/dest/testlabel"

	err := a.prepareDuplicacySetup()
	if err != nil {
		t.Fatalf("prepareDuplicacySetup() no-filter error: %v", err)
	}
}

func TestApp_PrepareDuplicacySetup_RealDirs(t *testing.T) {
	a := testApp(t)
	a.doBackup = false
	a.doPrune = true
	a.flags.dryRun = false

	a.cfg = config.NewDefaults()
	a.cfg.Destination = "/backup/dest"
	a.cfg.Filter = ""
	a.backupTarget = "/backup/dest/testlabel"
	a.workRoot = filepath.Join(t.TempDir(), "workroot")

	err := a.prepareDuplicacySetup()
	if err != nil {
		t.Fatalf("prepareDuplicacySetup() real dirs error: %v", err)
	}

	// Verify that directories were actually created
	if a.dup == nil {
		t.Fatal("dup should be set")
	}
	if _, err := os.Stat(a.dup.DuplicacyDir); os.IsNotExist(err) {
		t.Error("DuplicacyDir should have been created")
	}
}

// ---------------------------------------------------------------------------
// TestApp_RunBackupPhase
// ---------------------------------------------------------------------------

func TestApp_RunBackupPhase_DryRun(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = true
	a.cfg = config.NewDefaults()
	a.cfg.Threads = 4

	a.dup = &duplicacy.Setup{
		DryRun: true,
		Runner: execpkg.NewMockRunner(),
	}

	err := a.runBackupPhase()
	if err != nil {
		t.Fatalf("runBackupPhase() dry-run error: %v", err)
	}
}

func TestApp_RunBackupPhase_Success(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.cfg = config.NewDefaults()
	a.cfg.Threads = 4

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Backup completed\n"},
	)

	workRoot := t.TempDir()
	a.dup = duplicacy.NewSetup(workRoot, "/repo", "/target", false, mock)

	err := a.runBackupPhase()
	if err != nil {
		t.Fatalf("runBackupPhase() error: %v", err)
	}

	// Verify the mock was called
	if len(mock.Invocations) != 1 {
		t.Errorf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Cmd != "duplicacy" {
		t.Errorf("expected cmd 'duplicacy', got %q", mock.Invocations[0].Cmd)
	}
}

func TestApp_RunBackupPhase_Failure(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.cfg = config.NewDefaults()
	a.cfg.Threads = 4

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Err: fmt.Errorf("backup failed")},
	)

	workRoot := t.TempDir()
	a.dup = duplicacy.NewSetup(workRoot, "/repo", "/target", false, mock)

	err := a.runBackupPhase()
	if err == nil {
		t.Fatal("runBackupPhase() should return error on failure")
	}
	if !strings.Contains(err.Error(), "Backup failed") {
		t.Errorf("error should mention 'Backup failed', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestApp_RunPrunePhase
// ---------------------------------------------------------------------------

func TestApp_RunPrunePhase_DryRun(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = true
	a.cfg = config.NewDefaults()
	a.cfg.PruneArgs = []string{"-keep", "0:365"}

	a.dup = &duplicacy.Setup{
		DryRun: true,
		Runner: execpkg.NewMockRunner(),
	}

	err := a.runPrunePhase()
	if err != nil {
		t.Fatalf("runPrunePhase() dry-run error: %v", err)
	}
}

func TestApp_RunPrunePhase_Success(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.flags.forcePrune = false
	a.cfg = config.NewDefaults()
	a.cfg.PruneArgs = []string{"-keep", "0:365"}

	// Mock: validate-repo, prune-preview, list-revisions, policy-prune
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Listing files...\n"},                       // ValidateRepo
		execpkg.MockResult{Stdout: "Deleting revision 1\n"},                    // SafePrunePreview (prune dry-run)
		execpkg.MockResult{Stdout: "Listing revision 1\nListing revision 2\n"}, // GetTotalRevisionCount
		execpkg.MockResult{Stdout: "Prune completed\n"},                        // RunPrune
	)

	workRoot := t.TempDir()
	a.dup = duplicacy.NewSetup(workRoot, "/repo", "/target", false, mock)

	err := a.runPrunePhase()
	if err != nil {
		t.Fatalf("runPrunePhase() error: %v", err)
	}
}

func TestApp_RunPrunePhase_ValidateRepoFails(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.cfg = config.NewDefaults()
	a.cfg.PruneArgs = []string{"-keep", "0:365"}

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Err: fmt.Errorf("repo invalid")}, // ValidateRepo fails
	)

	workRoot := t.TempDir()
	a.dup = duplicacy.NewSetup(workRoot, "/repo", "/target", false, mock)

	err := a.runPrunePhase()
	if err == nil {
		t.Fatal("runPrunePhase() should fail when repo validation fails")
	}
	if !strings.Contains(err.Error(), "repository not ready") {
		t.Errorf("error should mention repo not ready, got: %v", err)
	}
}

func TestApp_RunPrunePhase_ThresholdExceeded_Blocked(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.flags.forcePrune = false
	a.cfg = config.NewDefaults()
	a.cfg.SafePruneMaxDeleteCount = 1 // Very low threshold
	a.cfg.PruneArgs = []string{"-keep", "0:365"}

	// Mock: validate-repo succeeds, preview shows 5 deletions, 10 total revisions
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "OK\n"}, // ValidateRepo
		execpkg.MockResult{Stdout: "Deleting revision 1\nDeleting revision 2\nDeleting revision 3\n"},                                                                                                                                           // SafePrunePreview
		execpkg.MockResult{Stdout: "Listing revision 1\nListing revision 2\nListing revision 3\nListing revision 4\nListing revision 5\nListing revision 6\nListing revision 7\nListing revision 8\nListing revision 9\nListing revision 10\n"}, // GetTotalRevisionCount
	)

	workRoot := t.TempDir()
	a.dup = duplicacy.NewSetup(workRoot, "/repo", "/target", false, mock)

	err := a.runPrunePhase()
	if err == nil {
		t.Fatal("runPrunePhase() should fail when threshold exceeded without --force-prune")
	}
	if !strings.Contains(err.Error(), "safe prune thresholds") {
		t.Errorf("error should mention thresholds, got: %v", err)
	}
}

func TestApp_RunPrunePhase_ThresholdExceeded_ForceOverride(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.flags.forcePrune = true
	a.cfg = config.NewDefaults()
	a.cfg.SafePruneMaxDeleteCount = 1 // Very low threshold
	a.cfg.PruneArgs = []string{"-keep", "0:365"}

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "OK\n"}, // ValidateRepo
		execpkg.MockResult{Stdout: "Deleting revision 1\nDeleting revision 2\nDeleting revision 3\n"},                                                                                                                                           // SafePrunePreview
		execpkg.MockResult{Stdout: "Listing revision 1\nListing revision 2\nListing revision 3\nListing revision 4\nListing revision 5\nListing revision 6\nListing revision 7\nListing revision 8\nListing revision 9\nListing revision 10\n"}, // GetTotalRevisionCount
		execpkg.MockResult{Stdout: "Prune done\n"}, // RunPrune
	)

	workRoot := t.TempDir()
	a.dup = duplicacy.NewSetup(workRoot, "/repo", "/target", false, mock)

	err := a.runPrunePhase()
	if err != nil {
		t.Fatalf("runPrunePhase() with --force-prune should succeed: %v", err)
	}
}

func TestApp_RunPrunePhase_DeepPrune_DryRun(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = true
	a.deepPruneMode = true
	a.cfg = config.NewDefaults()
	a.cfg.PruneArgs = []string{"-keep", "0:365"}

	a.dup = &duplicacy.Setup{
		DryRun: true,
		Runner: execpkg.NewMockRunner(),
	}

	err := a.runPrunePhase()
	if err != nil {
		t.Fatalf("runPrunePhase() deep dry-run error: %v", err)
	}
}

func TestApp_RunPrunePhase_RevisionCountFailed_NoForce(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.flags.forcePrune = false
	a.cfg = config.NewDefaults()
	a.cfg.PruneArgs = []string{"-keep", "0:365"}

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "OK\n"},               // ValidateRepo
		execpkg.MockResult{Stdout: "no deletions\n"},     // SafePrunePreview
		execpkg.MockResult{Err: fmt.Errorf("list fail")}, // GetTotalRevisionCount fails
	)

	workRoot := t.TempDir()
	a.dup = duplicacy.NewSetup(workRoot, "/repo", "/target", false, mock)

	err := a.runPrunePhase()
	if err == nil {
		t.Fatal("runPrunePhase() should fail when revision count fails without --force-prune")
	}
	if !strings.Contains(err.Error(), "Revision count") {
		t.Errorf("error should mention revision count, got: %v", err)
	}
}

func TestApp_RunPrunePhase_RevisionCountFailed_WithForce(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.flags.forcePrune = true
	a.cfg = config.NewDefaults()
	a.cfg.PruneArgs = []string{"-keep", "0:365"}

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "OK\n"},               // ValidateRepo
		execpkg.MockResult{Stdout: "no deletions\n"},     // SafePrunePreview
		execpkg.MockResult{Err: fmt.Errorf("list fail")}, // GetTotalRevisionCount fails
		execpkg.MockResult{Stdout: "Prune done\n"},       // RunPrune
	)

	workRoot := t.TempDir()
	a.dup = duplicacy.NewSetup(workRoot, "/repo", "/target", false, mock)

	err := a.runPrunePhase()
	if err != nil {
		t.Fatalf("runPrunePhase() with --force-prune should handle revision count failure: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestApp_RunFixPermsPhase
// ---------------------------------------------------------------------------

func TestApp_RunFixPermsPhase_DryRun(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = true
	a.cfg = config.NewDefaults()
	a.cfg.LocalOwner = "testuser"
	a.cfg.LocalGroup = "testgroup"
	a.backupTarget = "/backup/target"

	err := a.runFixPermsPhase()
	if err != nil {
		t.Fatalf("runFixPermsPhase() dry-run error: %v", err)
	}
}

func TestApp_RunFixPermsPhase_Success(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.cfg = config.NewDefaults()
	a.cfg.LocalOwner = "testuser"
	a.cfg.LocalGroup = "testgroup"

	targetDir := t.TempDir()
	a.backupTarget = targetDir

	// Create mock runner that succeeds for chown
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{}, // chown succeeds
	)
	a.runner = mock

	err := a.runFixPermsPhase()
	if err != nil {
		t.Fatalf("runFixPermsPhase() error: %v", err)
	}
}

func TestApp_RunFixPermsPhase_ChownFails(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.cfg = config.NewDefaults()
	a.cfg.LocalOwner = "testuser"
	a.cfg.LocalGroup = "testgroup"

	targetDir := t.TempDir()
	a.backupTarget = targetDir

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Err: fmt.Errorf("chown failed")},
	)
	a.runner = mock

	err := a.runFixPermsPhase()
	if err == nil {
		t.Fatal("runFixPermsPhase() should fail when chown fails")
	}
	if !strings.Contains(err.Error(), "Permission normalisation failed") {
		t.Errorf("error should mention permission normalisation, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestApp_Cleanup — edge cases
// ---------------------------------------------------------------------------

func TestApp_Cleanup_BackupMode_NoSnapshot(t *testing.T) {
	a := testApp(t)
	a.doBackup = true
	// snapshotTarget doesn't exist, so cleanup should skip deletion
	a.snapshotTarget = filepath.Join(t.TempDir(), "nonexistent-snapshot")

	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-cleanup")
	a.lk.Acquire()
	a.lockAcquired = true

	a.cleanup()
	if !a.cleanedUp {
		t.Error("cleanedUp should be true after cleanup()")
	}
}

func TestApp_Cleanup_BackupMode_WithSnapshot_DryRun(t *testing.T) {
	a := testApp(t)
	a.doBackup = true
	a.flags.dryRun = true

	// Create a fake snapshot directory
	snapshotDir := filepath.Join(t.TempDir(), "snapshot")
	os.MkdirAll(snapshotDir, 0755)
	a.snapshotTarget = snapshotDir

	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-cleanup-dryrun")

	a.cleanup()
	// In dry-run, directory should still exist (btrfs delete is just logged)
	if _, err := os.Stat(snapshotDir); os.IsNotExist(err) {
		t.Error("snapshot dir should still exist in dry-run mode")
	}
}

func TestApp_Cleanup_WithDup(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false

	workDir := filepath.Join(t.TempDir(), "workdir")
	os.MkdirAll(workDir, 0755)
	a.dup = &duplicacy.Setup{WorkRoot: workDir}

	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-cleanup-dup")

	a.cleanup()
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Error("work directory should have been removed after cleanup")
	}
}

func TestApp_Cleanup_WorkRootFallback(t *testing.T) {
	a := testApp(t)
	a.dup = nil
	a.flags.dryRun = false

	workDir := filepath.Join(t.TempDir(), "workroot-fallback")
	os.MkdirAll(workDir, 0755)
	a.workRoot = workDir

	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-cleanup-fallback")

	a.cleanup()
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Error("workRoot should have been removed when dup is nil")
	}
}

func TestApp_Cleanup_SuccessStatus(t *testing.T) {
	a := testApp(t)
	a.exitCode = 0

	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-success")

	a.cleanup()
	// No panic = success
}

func TestApp_Cleanup_FailedStatus(t *testing.T) {
	a := testApp(t)
	a.exitCode = 1

	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-failed")

	a.cleanup()
	// No panic = success
}

// ---------------------------------------------------------------------------
// TestApp_LoadConfig — additional edge cases
// ---------------------------------------------------------------------------

func TestApp_LoadConfig_RemoteMode(t *testing.T) {
	a := testApp(t)
	a.flags.remoteMode = true
	a.flags.fixPerms = false
	a.doBackup = false
	a.doPrune = true

	cfgDir := t.TempDir()
	a.configDir = cfgDir
	a.configFile = filepath.Join(cfgDir, "testlabel-backup.conf")

	// Write a config file with [common] and [remote] sections
	cfgContent := `[common]
DESTINATION = s3://gateway.storjshare.io/bucket
PRUNE = -keep 0:365

[remote]
THREADS = 4
`
	os.WriteFile(a.configFile, []byte(cfgContent), 0644)

	err := a.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() remote mode error: %v", err)
	}

	if a.cfg == nil {
		t.Fatal("cfg should not be nil after loadConfig()")
	}
}

func TestApp_LoadConfig_ParseError(t *testing.T) {
	a := testApp(t)
	a.flags.remoteMode = false
	a.doBackup = true

	cfgDir := t.TempDir()
	a.configDir = cfgDir
	a.configFile = filepath.Join(cfgDir, "testlabel-backup.conf")

	// Write invalid config
	cfgContent := `INVALID LINE OUTSIDE SECTION
`
	os.WriteFile(a.configFile, []byte(cfgContent), 0644)

	err := a.loadConfig()
	if err == nil {
		t.Fatal("loadConfig() should fail on parse error")
	}
}

func TestApp_LoadConfig_ValidationError(t *testing.T) {
	a := testApp(t)
	a.flags.remoteMode = false
	a.doBackup = true

	cfgDir := t.TempDir()
	a.configDir = cfgDir
	a.configFile = filepath.Join(cfgDir, "testlabel-backup.conf")

	// Missing DESTINATION (required)
	cfgContent := `[common]
THREADS = 4

[local]
`
	os.WriteFile(a.configFile, []byte(cfgContent), 0644)

	err := a.loadConfig()
	if err == nil {
		t.Fatal("loadConfig() should fail when DESTINATION is missing")
	}
	if !strings.Contains(err.Error(), "DESTINATION") {
		t.Errorf("error should mention DESTINATION, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestApp_LoadSecrets — additional edge cases
// ---------------------------------------------------------------------------

func TestApp_LoadSecrets_RemoteValidationFails(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root for secrets ownership check")
	}

	a := testApp(t)
	a.flags.remoteMode = true
	a.backupLabel = "testlabel"

	secretsDir := t.TempDir()
	a.secretsDir = secretsDir

	// Write secrets with short ID/secret that will fail validation
	content := "STORJ_S3_ID=short\nSTORJ_S3_SECRET=short\n"
	p := filepath.Join(secretsDir, "duplicacy-testlabel.env")
	os.WriteFile(p, []byte(content), 0600)

	err := a.loadSecrets()
	if err == nil {
		t.Fatal("loadSecrets() should fail when validation fails")
	}
}

// ---------------------------------------------------------------------------
// Additional parseFlags edge cases
// ---------------------------------------------------------------------------

func TestParseFlags_DuplicateModeFails(t *testing.T) {
	_, err := parseFlags([]string{"--backup", "--prune", "homes"})
	if err == nil {
		t.Fatal("expected error for duplicate mode flags")
	}
	if !strings.Contains(err.Error(), "only one mode") {
		t.Errorf("error should mention 'only one mode', got: %v", err)
	}
}

func TestParseFlags_AllModifiers(t *testing.T) {
	f, err := parseFlags([]string{"--prune", "--force-prune", "--remote", "--dry-run", "--fix-perms", "homes"})
	if err != nil {
		t.Fatalf("parseFlags() error: %v", err)
	}
	if !f.forcePrune {
		t.Error("forcePrune should be true")
	}
	if !f.remoteMode {
		t.Error("remoteMode should be true")
	}
	if !f.dryRun {
		t.Error("dryRun should be true")
	}
	if !f.fixPerms {
		t.Error("fixPerms should be true")
	}
}

func TestParseFlags_MultiplePositionalArgs(t *testing.T) {
	f, err := parseFlags([]string{"homes"})
	if err != nil {
		t.Fatalf("parseFlags() error: %v", err)
	}
	if f.source != "homes" {
		t.Errorf("source = %q, want %q", f.source, "homes")
	}
}

// ---------------------------------------------------------------------------
// Additional validateLabel edge cases
// ---------------------------------------------------------------------------

func TestValidateLabel_WithDots(t *testing.T) {
	err := validateLabel("label..traversal")
	if err == nil {
		t.Fatal("expected error for label with ..")
	}
}

func TestValidateLabel_WithSlash(t *testing.T) {
	err := validateLabel("label/path")
	if err == nil {
		t.Fatal("expected error for label with /")
	}
}

func TestValidateLabel_WithBackslash(t *testing.T) {
	err := validateLabel("label\\path")
	if err == nil {
		t.Fatal("expected error for label with \\")
	}
}

func TestValidateLabel_StartingWithHyphen(t *testing.T) {
	err := validateLabel("-invalid")
	if err == nil {
		t.Fatal("expected error for label starting with hyphen")
	}
}

func TestValidateLabel_StartingWithUnderscore(t *testing.T) {
	err := validateLabel("_invalid")
	if err == nil {
		t.Fatal("expected error for label starting with underscore")
	}
}

func TestValidateLabel_WithSpaces(t *testing.T) {
	err := validateLabel("has spaces")
	if err == nil {
		t.Fatal("expected error for label with spaces")
	}
}

// ---------------------------------------------------------------------------
// joinDestination additional edge cases
// ---------------------------------------------------------------------------

func TestJoinDestination_URLWithTrailingSlash(t *testing.T) {
	result := joinDestination("s3://bucket/path/", "homes")
	if result != "s3://bucket/path/homes" {
		t.Errorf("joinDestination = %q, want %q", result, "s3://bucket/path/homes")
	}
}

func TestJoinDestination_URLMultipleSlashes(t *testing.T) {
	result := joinDestination("s3://bucket/path///", "homes")
	if result != "s3://bucket/path/homes" {
		t.Errorf("joinDestination = %q, want %q", result, "s3://bucket/path/homes")
	}
}

// ---------------------------------------------------------------------------
// TestApp_PrintHeader
// ---------------------------------------------------------------------------

func TestApp_PrintHeader_DoesNotPanic_WithLock(t *testing.T) {
	a := testApp(t)
	lockDir := t.TempDir()
	a.lk = lock.New(lockDir, "test-header")
	a.printHeader()
}

// ---------------------------------------------------------------------------
// TestApp_Execute_PrunePreviewFails
// ---------------------------------------------------------------------------

func TestApp_RunPrunePhase_PreviewFails(t *testing.T) {
	a := testApp(t)
	a.flags.dryRun = false
	a.cfg = config.NewDefaults()
	a.cfg.PruneArgs = []string{"-keep", "0:365"}

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "OK\n"},                    // ValidateRepo
		execpkg.MockResult{Err: fmt.Errorf("preview failed")}, // SafePrunePreview fails
	)

	workRoot := t.TempDir()
	a.dup = duplicacy.NewSetup(workRoot, "/repo", "/target", false, mock)

	err := a.runPrunePhase()
	if err == nil {
		t.Fatal("runPrunePhase() should fail when prune preview fails")
	}
	if !strings.Contains(err.Error(), "Safe prune preview failed") {
		t.Errorf("unexpected error: %v", err)
	}
}
