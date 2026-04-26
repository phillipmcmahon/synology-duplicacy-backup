package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/command"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func currentUsername(t *testing.T) string {
	t.Helper()
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	output, err := exec.Command("id", "-un").Output()
	if err != nil {
		t.Fatalf("id -un failed: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func TestInstallScript_Syntax(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "install-synology.sh")
	cmd := exec.Command("sh", "-n", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sh -n %s failed: %v\n%s", scriptPath, err, output)
	}
}

func TestRuntimeProfileMigrationScript_Syntax(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "migrate-runtime-profile.sh")
	cmd := exec.Command("sh", "-n", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sh -n %s failed: %v\n%s", scriptPath, err, output)
	}
}

func TestMirrorReleaseScript_Syntax(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "mirror-release-assets.sh")
	cmd := exec.Command("sh", "-n", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sh -n %s failed: %v\n%s", scriptPath, err, output)
	}
}

func TestMirrorReleaseScript_HelpMentionsCurrentFlow(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "mirror-release-assets.sh")
	cmd := exec.Command("sh", scriptPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mirror-release help failed: %v\n%s", err, output)
	}

	help := string(output)
	required := []string{
		"--tag",
		"phillipmcmahon/synology-duplicacy-backup",
		"homestorage",
		"/volume1/homes/phillipmcmahon/code/duplicacy-backup",
		"Source code (zip)",
		"Source code (tar.gz)",
	}
	for _, token := range required {
		if !strings.Contains(help, token) {
			t.Fatalf("mirror-release help missing %q:\n%s", token, help)
		}
	}
}

func TestVerifyReleaseScript_Syntax(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "verify-release.sh")
	cmd := exec.Command("sh", "-n", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sh -n %s failed: %v\n%s", scriptPath, err, output)
	}
}

func TestVerifyReleaseScript_HelpMentionsCurrentFlow(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "verify-release.sh")
	cmd := exec.Command("sh", scriptPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify-release help failed: %v\n%s", err, output)
	}

	help := string(output)
	required := []string{
		"--tag",
		"phillipmcmahon/synology-duplicacy-backup",
		"homestorage",
		"Highlights",
		"Validation",
		"Coverage",
	}
	for _, token := range required {
		if !strings.Contains(help, token) {
			t.Fatalf("verify-release help missing %q:\n%s", token, help)
		}
	}
}

func TestInstallScript_HelpMentionsCurrentLayout(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "install-synology.sh")
	cmd := exec.Command("sh", scriptPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install help failed: %v\n%s", err, output)
	}

	help := string(output)
	required := []string{
		"/usr/local/lib/duplicacy-backup",
		"$HOME/.config/duplicacy-backup",
		"$HOME/.local/state/duplicacy-backup",
		"migrate-runtime-profile.sh",
		"--no-activate",
		"--keep",
	}
	for _, token := range required {
		if !strings.Contains(help, token) {
			t.Fatalf("install help missing %q:\n%s", token, help)
		}
	}
}

func TestRuntimeProfileMigrationScript_HelpMentionsLegacyAndUserProfilePaths(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "migrate-runtime-profile.sh")
	cmd := exec.Command("sh", scriptPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migration help failed: %v\n%s", err, output)
	}

	help := string(output)
	required := []string{
		"/usr/local/lib/duplicacy-backup/.config",
		"/root/.secrets",
		"$HOME/.config/duplicacy-backup",
		"--target-user",
		"--move",
		"--dry-run",
	}
	for _, token := range required {
		if !strings.Contains(help, token) {
			t.Fatalf("migration help missing %q:\n%s", token, help)
		}
	}
}

type runtimeMigrationFixture struct {
	scriptPath    string
	legacyConfig  string
	legacySecrets string
	targetHome    string
	configDir     string
	secretsDir    string
	configSource  string
	secretsSource string
}

func newRuntimeMigrationFixture(t *testing.T) runtimeMigrationFixture {
	t.Helper()
	tempDir := t.TempDir()
	fixture := runtimeMigrationFixture{
		scriptPath:    filepath.Join(repoRoot(t), "scripts", "migrate-runtime-profile.sh"),
		legacyConfig:  filepath.Join(tempDir, "legacy-config"),
		legacySecrets: filepath.Join(tempDir, "legacy-secrets"),
		targetHome:    filepath.Join(tempDir, "operator-home"),
	}
	fixture.configDir = filepath.Join(fixture.targetHome, ".config", "duplicacy-backup")
	fixture.secretsDir = filepath.Join(fixture.configDir, "secrets")
	fixture.configSource = filepath.Join(fixture.legacyConfig, "homes-backup.toml")
	fixture.secretsSource = filepath.Join(fixture.legacySecrets, "homes-secrets.toml")

	if err := os.MkdirAll(fixture.legacyConfig, 0755); err != nil {
		t.Fatalf("MkdirAll(legacy config) failed: %v", err)
	}
	if err := os.MkdirAll(fixture.legacySecrets, 0755); err != nil {
		t.Fatalf("MkdirAll(legacy secrets) failed: %v", err)
	}
	if err := os.WriteFile(fixture.configSource, []byte("label = \"homes\"\n"), 0644); err != nil {
		t.Fatalf("WriteFile(config) failed: %v", err)
	}
	if err := os.WriteFile(fixture.secretsSource, []byte("[targets.onsite.keys]\n"), 0644); err != nil {
		t.Fatalf("WriteFile(secrets) failed: %v", err)
	}
	return fixture
}

func TestRuntimeProfileMigrationScript_CopiesTomlAndSecuresPermissions(t *testing.T) {
	fixture := newRuntimeMigrationFixture(t)

	cmd := exec.Command("sh", fixture.scriptPath,
		"--target-user", currentUsername(t),
		"--target-home", fixture.targetHome,
		"--legacy-config-dir", fixture.legacyConfig,
		"--legacy-secrets-dir", fixture.legacySecrets,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migration script failed: %v\n%s", err, output)
	}

	for _, path := range []string{fixture.configDir, fixture.secretsDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) failed: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0700 {
			t.Fatalf("%s perms = %04o, want 0700", path, got)
		}
	}
	for _, path := range []string{
		filepath.Join(fixture.configDir, "homes-backup.toml"),
		filepath.Join(fixture.secretsDir, "homes-secrets.toml"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) failed: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("%s perms = %04o, want 0600", path, got)
		}
	}
	if _, err := os.Stat(fixture.configSource); err != nil {
		t.Fatalf("source config should remain after copy mode: %v", err)
	}
	if _, err := os.Stat(fixture.secretsSource); err != nil {
		t.Fatalf("source secrets should remain after copy mode: %v", err)
	}
}

func TestRuntimeProfileMigrationScript_MoveRemovesLegacyFiles(t *testing.T) {
	fixture := newRuntimeMigrationFixture(t)

	cmd := exec.Command("sh", fixture.scriptPath,
		"--target-user", currentUsername(t),
		"--target-home", fixture.targetHome,
		"--legacy-config-dir", fixture.legacyConfig,
		"--legacy-secrets-dir", fixture.legacySecrets,
		"--move",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migration script failed: %v\n%s", err, output)
	}

	if _, err := os.Stat(fixture.configSource); !os.IsNotExist(err) {
		t.Fatalf("source config exists or unexpected error after --move: %v", err)
	}
	if _, err := os.Stat(fixture.secretsSource); !os.IsNotExist(err) {
		t.Fatalf("source secrets exists or unexpected error after --move: %v", err)
	}
	for _, path := range []string{
		filepath.Join(fixture.configDir, "homes-backup.toml"),
		filepath.Join(fixture.secretsDir, "homes-secrets.toml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("destination missing after --move: %s: %v", path, err)
		}
	}
}

func TestRuntimeProfileMigrationScript_RootShellRequiresTargetUser(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-shell migration guard only applies when tests run as root")
	}
	scriptPath := filepath.Join(repoRoot(t), "scripts", "migrate-runtime-profile.sh")
	cmd := exec.Command("sh", scriptPath, "--dry-run")
	cmd.Env = append(os.Environ(), "SUDO_USER=root")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("migration script unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "root shell migration needs --target-user") {
		t.Fatalf("output = %q", output)
	}
}

func TestInstallScript_DoesNotManageRuntimeConfigOrSecrets(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "install-synology.sh")
	tempDir := t.TempDir()
	installRoot := filepath.Join(tempDir, "install-root")
	binDir := filepath.Join(tempDir, "bin")
	configDir := filepath.Join(installRoot, ".config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll(.config) failed: %v", err)
	}
	configFile := filepath.Join(configDir, "homes-backup.toml")
	if err := os.WriteFile(configFile, []byte("label = \"homes\"\n"), 0644); err != nil {
		t.Fatalf("WriteFile(config) failed: %v", err)
	}
	binaryPath := filepath.Join(tempDir, "duplicacy-backup_9.9.9_linux_amd64")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("WriteFile(binary) failed: %v", err)
	}

	cmd := exec.Command("sh", scriptPath,
		"--binary", binaryPath,
		"--install-root", installRoot,
		"--bin-dir", binDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install script failed: %v\n%s", err, output)
	}

	configInfo, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("Stat(.config) failed: %v", err)
	}
	if got := configInfo.Mode().Perm(); got != 0755 {
		t.Fatalf(".config perms = %04o, want unchanged 0755", got)
	}

	fileInfo, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("Stat(config file) failed: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0644 {
		t.Fatalf("config file perms = %04o, want unchanged 0644", got)
	}

	installOutput := string(output)
	for _, token := range []string{
		"Runtime config default: $HOME/.config/duplicacy-backup",
		"Runtime secrets default: $HOME/.config/duplicacy-backup/secrets",
		"Runtime config and secrets are not migrated automatically",
		"Migration helper: ./migrate-runtime-profile.sh --dry-run --target-user <operator-user>",
	} {
		if !strings.Contains(installOutput, token) {
			t.Fatalf("install output missing %q:\n%s", token, installOutput)
		}
	}
}

func TestReleaseDocs_StayAlignedWithCurrentSurface(t *testing.T) {
	root := repoRoot(t)
	expectations := map[string][]string{
		filepath.Join(root, "README.md"): {
			"cleanup-storage",
			"fix-perms",
			"--json-summary",
			"config validate",
			"notify test",
			"restore plan",
			"restore list-revisions",
			"update --check-only",
			"health status",
			"$HOME/.local/state/duplicacy-backup/state/<label>.<target>.json",
			"[health.notify]",
			"$HOME/.config/duplicacy-backup",
			"$HOME/.config/duplicacy-backup/secrets",
			"S3-compatible",
		},
		filepath.Join(root, "docs", "cli.md"): {
			"cleanup-storage",
			"fix-perms",
			"--json-summary",
			"config validate",
			"notify test",
			"restore plan",
			"restore list-revisions",
			"update [OPTIONS]",
			"health status",
			"health doctor",
			"health verify",
			"0` healthy, `1` degraded, `2` unhealthy",
			"Storage keys",
			"[targets.<name>.keys]",
			"s3_secret",
			"--target <name>",
		},
		filepath.Join(root, "docs", "operations.md"): {
			"/usr/local/bin/duplicacy-backup",
			"$HOME/.config/duplicacy-backup",
			"$HOME/.local/state/duplicacy-backup",
			"--no-activate",
			"duplicacy-backup update --check-only",
			"health status --target onsite-usb homes",
			"health verify --json-summary --target onsite-usb homes",
			"restore plan --target onsite-usb homes",
			"restore run --target onsite-usb --revision 2403 --path docs/readme.md --yes homes",
			"storage keys are needed only when the selected backend requires them",
		},
		filepath.Join(root, "docs", "configuration.md"): {
			"$HOME/.config/duplicacy-backup/secrets/<label>-secrets.toml",
			"Duplicacy storage",
			"[targets.<name>.keys]",
			"s3_secret",
			"[health]",
			"[health.notify]",
			"health_webhook_bearer_token",
			"health_ntfy_token",
			"$HOME/.local/state/duplicacy-backup/state/<label>.<target>.json",
		},
		filepath.Join(root, "TESTING.md"): {
			"install script help/output",
			"phase-oriented stderr output",
			"health command help and output shape",
			"JSON summaries for both run and health commands",
			"help text in `UsageText`",
			"release tarball smoke checks",
			"mirror-release-assets.sh",
			"verify-release.sh",
		},
		filepath.Join(root, "docs", "release-playbook.md"): {
			"mirror-release-assets.sh",
			"verify-release.sh",
			"Release Tracking Conventions",
			"Ready` -> `In Progress` -> `Done`",
			"Suggested release-prep checklist",
			"tar -cf - .",
			"Source code (zip)",
			"Source code (tar.gz)",
		},
		filepath.Join(root, ".github", "ISSUE_TEMPLATE", "release-prep.md"): {
			"Release Prep",
			"Prepare v",
			"version metadata updated",
			"Linux Go 1.26 validation passed",
			"release-prep notes generated",
		},
	}

	for path, required := range expectations {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) failed: %v", path, err)
		}
		text := string(data)
		for _, token := range required {
			if !strings.Contains(text, token) {
				t.Fatalf("%s missing %q", path, token)
			}
		}
	}
}

func TestUsageText_TargetHelpMatchesCurrentModel(t *testing.T) {
	meta := workflow.DefaultMetadata(scriptName, version, buildTime, logDir)
	rt := workflow.DefaultRuntime()
	usage := command.FullUsageText(meta, rt)

	expected := []string{
		"backup [OPTIONS] <source>",
		"prune [OPTIONS] <source>",
		"cleanup-storage [OPTIONS] <source>",
		"fix-perms [OPTIONS] <source>",
		"config <validate|explain|paths>",
		"diagnostics [OPTIONS] <source>",
		"notify <test> [OPTIONS] <source|update>",
		"rollback [OPTIONS]",
		"restore <plan|list-revisions|run|select> [OPTIONS] <source>",
		"update [OPTIONS]",
		"health <status|doctor|verify>",
		"--target <name>          Select the named target config where the command uses a label target",
		"--json-summary           Write a machine-readable command summary to stdout",
		"--check-only             Inspect update or rollback without changing install",
		"--keep <count>           Update retention count (default: 2)",
		"--attestations <mode>    Update release attestation mode",
		"COMMAND OVERVIEW:",
		"Runtime operations      Run, maintain, or repair one configured label target",
		"Config and inspection   Read, explain, validate, or diagnose configured targets",
		"Notifications           Send explicit synthetic notification checks",
		"Restore drills          Restore from snapshots without writing to the live source",
		"Managed install         Manage the installed application binary",
		"health status         Fast read-only health summary for operators and schedulers",
		"backup                Run a backup for the selected label and target",
		"prune                 Run threshold-guarded prune for the selected label and target",
		"cleanup-storage       Request storage maintenance:",
		"fix-perms             Normalise path-based storage ownership and permissions",
		"diagnostics           Print a redacted support bundle for one label and target",
		"notify test           Send a simulated notification through configured providers",
		"notify test update    Send a simulated update notification through global update config",
		"restore plan          Print a read-only Duplicacy restore-drill plan without executing a restore",
		"restore list-revisions",
		"List visible backup revisions without executing a restore",
		"restore run           Prepare or reuse a drill workspace, then restore a revision, file, or pattern only there",
		"restore select        Guided TUI restore path: choose a restore point, inspect it, select files/subtrees, and confirm drill restore execution",
		"--path-prefix <path>     Restore select: start browsing under a snapshot-relative prefix",
		"update                Check GitHub for a newer published release and install it through the packaged installer",
		"rollback              Inspect or activate a retained managed-install version",
		"health verify         Read-only integrity check across revisions found for the current label",
		"Target-specific run and health state are stored under:",
		"health_webhook_bearer_token",
		"health_ntfy_token",
		"Use [targets.<name>.keys] tables with Duplicacy key names such as:",
	}
	for _, token := range expected {
		if !strings.Contains(usage, token) {
			t.Fatalf("usage missing %q:\n%s", token, usage)
		}
	}
}
