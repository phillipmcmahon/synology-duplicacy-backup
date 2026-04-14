package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
		"/root/.secrets/",
		"--no-activate",
		"--keep",
		"--config-group",
		".config/",
	}
	for _, token := range required {
		if !strings.Contains(help, token) {
			t.Fatalf("install help missing %q:\n%s", token, help)
		}
	}
}

func TestInstallScript_NormalisesConfigPermissions(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "install-synology.sh")
	tempDir := t.TempDir()
	installRoot := filepath.Join(tempDir, "install-root")
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(filepath.Join(installRoot, ".config"), 0755); err != nil {
		t.Fatalf("MkdirAll(.config) failed: %v", err)
	}
	configFile := filepath.Join(installRoot, ".config", "homes-backup.toml")
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
		"--config-group", "staff",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install script failed: %v\n%s", err, output)
	}

	configInfo, err := os.Stat(filepath.Join(installRoot, ".config"))
	if err != nil {
		t.Fatalf("Stat(.config) failed: %v", err)
	}
	if got := configInfo.Mode().Perm(); got != 0750 {
		t.Fatalf(".config perms = %04o, want 0750", got)
	}

	fileInfo, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("Stat(config file) failed: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0640 {
		t.Fatalf("config file perms = %04o, want 0640", got)
	}

	installOutput := string(output)
	if !strings.Contains(installOutput, "Secrets directory: /root/.secrets") {
		t.Fatalf("install output missing secrets directory guidance:\n%s", installOutput)
	}
	if os.Geteuid() == 0 {
		if !strings.Contains(installOutput, "ensured as root:root (700); secrets files are not modified") {
			t.Fatalf("install output missing root secrets guidance:\n%s", installOutput)
		}
	} else {
		if !strings.Contains(installOutput, "run installer as root to create or normalise it") {
			t.Fatalf("install output missing non-root secrets guidance:\n%s", installOutput)
		}
	}
}

func TestReleaseDocs_StayAlignedWithCurrentSurface(t *testing.T) {
	root := repoRoot(t)
	expectations := map[string][]string{
		filepath.Join(root, "README.md"): {
			"--cleanup-storage",
			"--fix-perms",
			"--json-summary",
			"config validate",
			"notify test",
			"health status",
			"/var/lib/duplicacy-backup/<label>.<target>.json",
			"[health.notify]",
			"/usr/local/lib/duplicacy-backup/.config",
			"/root/.secrets",
			"S3-compatible",
		},
		filepath.Join(root, "docs", "cli.md"): {
			"--cleanup-storage",
			"--fix-perms",
			"--json-summary",
			"config validate",
			"notify test",
			"health status",
			"health doctor",
			"health verify",
			"0` healthy, `1` degraded, `2` unhealthy",
			"S3-compatible",
			"storj_s3_id",
			"storj_s3_secret",
			"--target <name>",
		},
		filepath.Join(root, "docs", "operations.md"): {
			"/usr/local/bin/duplicacy-backup",
			"/usr/local/lib/duplicacy-backup/.config",
			"/root/.secrets",
			"--no-activate",
			"health status --target onsite-usb homes",
			"health verify --json-summary --target onsite-usb homes",
			"S3-compatible",
		},
		filepath.Join(root, "docs", "configuration.md"): {
			"/root/.secrets/<label>-secrets.toml",
			"S3-compatible",
			"storj_s3_id",
			"storj_s3_secret",
			"[health]",
			"[health.notify]",
			"health_webhook_bearer_token",
			"health_ntfy_token",
			"/var/lib/duplicacy-backup/<label>.<target>.json",
		},
		filepath.Join(root, "TESTING.md"): {
			"install script help/output",
			"phase-oriented stderr output",
			"health command help and output shape",
			"JSON summaries for both run and health commands",
			"help text in `UsageText`",
			"release tarball smoke checks",
			"mirror-release-assets.sh",
		},
		filepath.Join(root, "docs", "release-playbook.md"): {
			"mirror-release-assets.sh",
			"tar -cf - .",
			"Source code (zip)",
			"Source code (tar.gz)",
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
	usage := workflow.FullUsageText(meta, rt)

	expected := []string{
		"config <validate|explain|paths>",
		"notify <test>",
		"health <status|doctor|verify>",
		"--target <name>          Perform operation against the named target config (required)",
		"--json-summary           Write a machine-readable run summary to stdout",
		"health status            Fast read-only health summary for operators and schedulers",
		"notify test             Send a clearly marked simulated notification through the configured providers",
		"health verify            Read-only integrity check across revisions found for the current label",
		"Target-specific run and health state are stored under:",
		"health_webhook_bearer_token",
		"health_ntfy_token",
		"Use [targets.<name>] tables with:",
		"--cleanup-storage        Request storage maintenance:",
		"--fix-perms              Normalise filesystem repository ownership and permissions",
	}
	for _, token := range expected {
		if !strings.Contains(usage, token) {
			t.Fatalf("usage missing %q:\n%s", token, usage)
		}
	}
}
