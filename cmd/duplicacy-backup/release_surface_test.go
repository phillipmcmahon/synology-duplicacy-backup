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
		".config/",
	}
	for _, token := range required {
		if !strings.Contains(help, token) {
			t.Fatalf("install help missing %q:\n%s", token, help)
		}
	}
}

func TestReleaseDocs_StayAlignedWithCurrentSurface(t *testing.T) {
	root := repoRoot(t)
	expectations := map[string][]string{
		filepath.Join(root, "README.md"): {
			"--cleanup-storage",
			"--fix-perms",
			"/usr/local/lib/duplicacy-backup/.config",
			"/root/.secrets",
			"S3-compatible",
		},
		filepath.Join(root, "docs", "cli.md"): {
			"--cleanup-storage",
			"--fix-perms",
			"S3-compatible",
			"storj_s3_id",
			"storj_s3_secret",
		},
		filepath.Join(root, "docs", "operations.md"): {
			"/usr/local/bin/duplicacy-backup",
			"/usr/local/lib/duplicacy-backup/.config",
			"/root/.secrets",
			"--no-activate",
			"S3-compatible",
		},
		filepath.Join(root, "docs", "configuration.md"): {
			"/root/.secrets/duplicacy-<label>.toml",
			"S3-compatible",
			"storj_s3_id",
			"storj_s3_secret",
		},
		filepath.Join(root, "TESTING.md"): {
			"install script help/output",
			"phase-oriented stderr output",
			"help text in `UsageText`",
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

func TestUsageText_RemoteHelpMatchesCurrentModel(t *testing.T) {
	meta := workflow.DefaultMetadata(scriptName, version, buildTime, logDir)
	rt := workflow.DefaultRuntime()
	usage := workflow.UsageText(meta, rt)

	expected := []string{
		"--remote                 Perform operation against remote S3-compatible target config",
		"Current TOML keys: storj_s3_id and storj_s3_secret",
		"--cleanup-storage        Request storage maintenance:",
		"--fix-perms              Normalise local repository ownership and permissions",
	}
	for _, token := range expected {
		if !strings.Contains(usage, token) {
			t.Fatalf("usage missing %q:\n%s", token, usage)
		}
	}
}
