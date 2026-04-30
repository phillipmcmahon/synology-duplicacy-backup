package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

func TestInstallScript_Syntax(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "install-synology.sh")
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
		"--no-activate",
		"--keep",
	}
	for _, token := range required {
		if !strings.Contains(help, token) {
			t.Fatalf("install help missing %q:\n%s", token, help)
		}
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
		"Runtime config and secrets are operator-owned files",
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
			"--json-summary",
			"config validate",
			"notify test",
			"restore plan",
			"restore list-revisions",
			"update --check-only",
			"health status",
			"$HOME/.local/state/duplicacy-backup/state/<label>.<storage>.json",
			"[health.notify]",
			"$HOME/.config/duplicacy-backup",
			"$HOME/.config/duplicacy-backup/secrets",
			"S3-compatible",
		},
		filepath.Join(root, "docs", "cli.md"): {
			"cleanup-storage",
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
			"[storage.<name>.keys]",
			"s3_secret",
			"--storage <name>",
		},
		filepath.Join(root, "docs", "operations.md"): {
			"/usr/local/bin/duplicacy-backup",
			"$HOME/.config/duplicacy-backup",
			"$HOME/.local/state/duplicacy-backup",
			"--no-activate",
			"duplicacy-backup update --check-only",
			"health status --storage onsite-usb homes",
			"health verify --json-summary --storage onsite-usb homes",
			"restore plan --storage onsite-usb homes",
			"restore run --storage onsite-usb --revision 2403 --path docs/readme.md --yes homes",
		},
		filepath.Join(root, "docs", "configuration.md"): {
			"$HOME/.config/duplicacy-backup/secrets/<label>-secrets.toml",
			"Duplicacy storage",
			"[storage.<name>.keys]",
			"s3_secret",
			"[health]",
			"[health.notify]",
			"health_webhook_bearer_token",
			"health_ntfy_token",
			"$HOME/.local/state/duplicacy-backup/state/<label>.<storage>.json",
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

	regexExpectations := map[string][]string{
		filepath.Join(root, "docs", "operations.md"): {
			`Storage keys are needed only when the\s+selected backend requires them`,
		},
	}

	for path, required := range regexExpectations {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) failed: %v", path, err)
		}
		text := string(data)
		for _, pattern := range required {
			if !regexp.MustCompile(pattern).MatchString(text) {
				t.Fatalf("%s missing pattern %q", path, pattern)
			}
		}
	}
}

func TestUsageText_TargetHelpMatchesCurrentModel(t *testing.T) {
	meta := workflow.MetadataForLogDir(scriptName, version, buildTime, logDir)
	rt := workflow.DefaultEnv()
	usage := command.FullUsageText(meta, rt)

	expected := []string{
		"backup [OPTIONS] <source>",
		"prune [OPTIONS] <source>",
		"cleanup-storage [OPTIONS] <source>",
		"config <validate|explain|paths>",
		"diagnostics [OPTIONS] <source>",
		"notify <test> [OPTIONS] <source|update>",
		"rollback [OPTIONS]",
		"restore <plan|list-revisions|run|select> [OPTIONS] <source>",
		"update [OPTIONS]",
		"health <status|doctor|verify>",
		"--storage <name>        Select the named storage config where the command uses label storage",
		"--json-summary           Write a machine-readable command summary to stdout",
		"--check-only             Inspect update or rollback without changing install",
		"--keep <count>           Update retention count (default: 2)",
		"--attestations <mode>    Update release attestation mode",
		"COMMAND OVERVIEW:",
		"Runtime operations      Run or maintain one configured storage entry",
		"Config and inspection   Read, explain, validate, or diagnose configured storage",
		"Notifications           Send explicit synthetic notification checks",
		"Restore drills          Restore from snapshots without writing to the live source",
		"Managed install         Manage the installed application binary",
		"health status         Fast read-only health summary for operators and schedulers",
		"backup                Run a backup for the selected label and storage",
		"prune                 Run threshold-guarded prune for the selected label and storage",
		"cleanup-storage       Request storage maintenance:",
		"diagnostics           Print a redacted support bundle for one label and storage",
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
		"Storage-specific run and health state are stored under:",
		"health_webhook_bearer_token",
		"health_ntfy_token",
		"Use [storage.<name>.keys] tables with Duplicacy key names such as:",
	}
	for _, token := range expected {
		if !strings.Contains(usage, token) {
			t.Fatalf("usage missing %q:\n%s", token, usage)
		}
	}
}
