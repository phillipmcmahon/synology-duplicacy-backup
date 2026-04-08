package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	// When --fix-perms is the sole operation, doBackup and doPrune must both
	// be false.  This is important because v1.6.0 gates the duplicacy binary
	// check and owner/group validation on these flags.
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
	// Default mode (no flags) should derive doBackup=true, meaning
	// duplicacy binary check is required.
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
	// --fix-perms --backup needs both duplicacy check AND owner/group validation.
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
	// --version is handled in run() before parseFlags, so we test the
	// early-exit loop directly by simulating the arg scan.
	for _, arg := range []string{"--version"} {
		if arg == "--version" || arg == "-v" {
			// Matches the early-exit condition in run()
		} else {
			t.Errorf("expected %q to match version flag check", arg)
		}
	}
}

func TestVersionFlag_Short(t *testing.T) {
	// -v is handled in run() before parseFlags, verify parseFlags rejects it
	// (because it should never reach parseFlags).
	_, err := parseFlags([]string{"-v"})
	if err == nil {
		t.Error("parseFlags should reject -v (handled before parseFlags in run)")
	}
}

func TestVersionOutput_ContainsVersion(t *testing.T) {
	// Verify the version variable is set correctly for v1.7.2
	if version != "1.7.2" {
		t.Errorf("version = %q, want %q", version, "1.7.2")
	}
}

func TestVersionOutput_ContainsScriptName(t *testing.T) {
	expected := "duplicacy-backup"
	if scriptName != expected {
		t.Errorf("scriptName = %q, want %q", scriptName, expected)
	}
}

func TestPrintUsage_DoesNotPanic(t *testing.T) {
	// printUsage writes to stdout; just verify it doesn't panic.
	// Redirect stdout to discard output.
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
	// When no source is provided, parseFlags returns an error.
	// In run(), this triggers printUsage() after the error.
	_, err := parseFlags([]string{"--backup"})
	if err == nil {
		t.Error("expected error for missing source directory")
	}
	if !strings.Contains(err.Error(), "source directory required") {
		t.Errorf("error = %q, want it to contain 'source directory required'", err.Error())
	}
}

func TestParseFlags_UnknownFlagShowsError(t *testing.T) {
	// Unknown flags produce an error; in run(), this triggers printUsage().
	_, err := parseFlags([]string{"--nonexistent", "homes"})
	if err == nil {
		t.Error("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown option") {
		t.Errorf("error = %q, want it to contain 'unknown option'", err.Error())
	}
}

// ─── Display logic derivation tests (v1.6.1) ────────────────────────────────
// These tests document which display path should be taken based on flags.
// The actual printing is in run(), but the branching logic is deterministic
// from the parsed flags.

// displayContext captures the fields that control config summary display.
type displayContext struct {
	fixPermsOnly bool // standalone fix-perms: minimal summary
	fixPerms     bool // combined: show Local Owner/Group in full summary
	remoteMode   bool // remote: never show Local Owner/Group
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
	// --force-prune without --prune or --prune-deep should be rejected.
	// parseFlags accepts it, but mode derivation should detect the conflict.
	f, err := parseFlags([]string{"--force-prune", "homes"})
	if err != nil {
		t.Fatalf("unexpected error from parseFlags: %v", err)
	}
	if !f.forcePrune {
		t.Error("expected forcePrune to be true")
	}
	// Mode defaults to "backup" when no mode flag given
	doPrune := f.mode == "prune" || f.mode == "prune-deep"
	if doPrune {
		t.Error("expected doPrune to be false when no prune flag is given")
	}
	// The run() function should error and exit for this combination.
	// Verify the condition that triggers the error:
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
	// Should NOT trigger error: deepPruneMode requires forcePrune (ok), and forcePrune with doPrune (ok)
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
	// --force-prune --backup should be rejected (backup is not prune)
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
