package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
)

func stubConfigCommandRunner(t *testing.T, results ...execpkg.MockResult) {
	t.Helper()
	original := newConfigCommandRunner
	newConfigCommandRunner = func() execpkg.Runner {
		return execpkg.NewMockRunner(results...)
	}
	t.Cleanup(func() {
		newConfigCommandRunner = original
	})
}

func TestHandleConfigCommand_ValidateConfiguredRemote(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{},
	)
	sourcePath := t.TempDir()
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", sourcePath, "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}

	required := []string{
		"Config validation succeeded for homes/offsite-storj",
		"Section: Resolved",
		"Section: Validation",
		"Label",
		"homes",
		"Target",
		"offsite-storj",
		"Config File",
		"homes-backup.toml",
		"Source Path Access",
		"Present",
		"Btrfs Source",
		"Required Settings",
		"Health Thresholds",
		"Prune Policy",
		"Storage Access",
		"Resolved",
		"Repository Access",
		"Valid",
		"Secrets",
		"Result",
		"Passed",
	}
	for _, token := range required {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	assertResolvedLabels(t, out, "Label", "Target", "Config File")
	assertResolvedValues(t, out, map[string]string{
		"Label":       "homes",
		"Target":      "offsite-storj",
		"Config File": filepath.Join(configDir, "homes-backup.toml"),
	})
	assertValidationLabels(t, out, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, out, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
}

func TestHandleConfigCommand_ValidateConfiguredRemoteWithoutRootRunsSafeChecks(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{},
	)
	sourcePath := t.TempDir()
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", sourcePath, "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	rt := testRuntime()
	rt.Geteuid = func() int { return 1000 }

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt)
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}

	for _, token := range []string{
		"Config validation succeeded for homes/offsite-storj",
		"Btrfs Source",
		"Valid",
		"Storage Access",
		"Resolved",
		"Repository Access",
		"Valid",
		"Secrets",
		"Valid",
		"Result",
		"Passed",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	values := extractValidationValues(t, out)
	if _, ok := values["Privileges"]; ok {
		t.Fatalf("Privileges should not be reported in the non-root-default model:\n%s", out)
	}
	if values["Btrfs Source"] != "Valid" {
		t.Fatalf("Btrfs Source = %q:\n%s", values["Btrfs Source"], out)
	}
	if values["Repository Access"] != "Valid" {
		t.Fatalf("Repository Access = %q:\n%s", values["Repository Access"], out)
	}
	if values["Secrets"] != "Valid" {
		t.Fatalf("Secrets = %q:\n%s", values["Secrets"], out)
	}
	assertAllowedValidationOutcomes(t, out)
}

func TestHandleConfigCommand_ValidateConfiguredLocalDuplicacyWithoutRootRunsSafeChecks(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{},
	)
	sourcePath := t.TempDir()
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-rustfs", localDuplicacyTargetConfig("homes", sourcePath, "s3://rustfs.local/bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "onsite-rustfs")

	rt := testRuntime()
	rt.Geteuid = func() int { return 1000 }

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "onsite-rustfs"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt)
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}

	for _, token := range []string{
		"Config validation succeeded for homes/onsite-rustfs",
		"Storage Access",
		"Resolved",
		"Repository Access",
		"Valid",
		"Target Settings",
		"Valid",
		"Secrets",
		"Valid",
		"Result",
		"Passed",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	values := extractValidationValues(t, out)
	if _, ok := values["Privileges"]; ok {
		t.Fatalf("Privileges should not be reported in the non-root-default model:\n%s", out)
	}
	if values["Repository Access"] != "Valid" {
		t.Fatalf("Repository Access = %q:\n%s", values["Repository Access"], out)
	}
	if values["Secrets"] != "Valid" {
		t.Fatalf("Secrets = %q:\n%s", values["Secrets"], out)
	}
	assertAllowedValidationOutcomes(t, out)
}

func TestHandleConfigCommand_ValidateLocalPathRepositoryWithoutRootRequiresSudo(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
	)
	sourcePath := t.TempDir()
	destinationRoot := t.TempDir()
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", sourcePath, destinationRoot, owner, group, 4, "-keep 0:365"))

	rt := testRuntime()
	rt.Geteuid = func() int { return 1000 }

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt)
	if err == nil {
		t.Fatal("HandleConfigCommand() expected error")
	}
	if !strings.Contains(OperatorMessage(err), "rerun config validate with sudo from the operator account") {
		t.Fatalf("OperatorMessage(err) = %q", OperatorMessage(err))
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/onsite-usb", "Repository Access", "Requires sudo", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
	values := extractValidationValues(t, report)
	if values["Repository Access"] != "Requires sudo" {
		t.Fatalf("Repository Access = %q:\n%s", values["Repository Access"], report)
	}
	assertValidationLabels(t, report, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertAllowedValidationOutcomes(t, report)
}

func TestHandleConfigCommand_ValidateRequiresExplicitTarget(t *testing.T) {
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildLabelConfig("homes", "onsite-usb", locationLocal, "/volume1/homes", "/backups/homes", owner, group, 4, "-keep 0:365", `
[targets.offsite-storj]
location = "remote"
storage = "s3://bucket/homes"
`))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir}
	if _, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime()); err == nil || !strings.Contains(err.Error(), "requires an explicit target selection") {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
}

func TestHandleConfigCommand_ValidateExplicitLocalTarget(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{},
	)

	sourcePath := t.TempDir()
	destinationRoot := t.TempDir()
	destination := filepath.Join(destinationRoot, "homes")
	if err := os.MkdirAll(filepath.Join(destination, "snapshots"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", sourcePath, destinationRoot, owner, group, 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config validation succeeded for homes/onsite-usb", "Section: Resolved", "Section: Validation", "Label", "homes", "Target", "onsite-usb", "Source Path Access", "Present", "Btrfs Source", "Valid", "Required Settings", "Health Thresholds", "Storage Access", "Writable", "Threads", "Valid", "Prune Policy", "Target Settings", "Repository Access", "Secrets", "Not required", "Result", "Passed"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	assertResolvedLabels(t, out, "Label", "Target", "Config File")
	assertResolvedValues(t, out, map[string]string{
		"Label":       "homes",
		"Target":      "onsite-usb",
		"Config File": filepath.Join(configDir, "homes-backup.toml"),
	})
	assertValidationLabels(t, out, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, out, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
}

func TestHandleConfigCommand_ValidateLocalReadOnlyTargetWithoutOwnerGroup(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{},
	)

	sourcePath := t.TempDir()
	destinationRoot := t.TempDir()
	destination := filepath.Join(destinationRoot, "homes")
	if err := os.MkdirAll(filepath.Join(destination, "snapshots"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, filepath.Join(destinationRoot, "homes"), "", "", 4, ""))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	if !strings.Contains(out, "Config validation succeeded for homes/onsite-usb") {
		t.Fatalf("output = %q", out)
	}
	if !strings.Contains(out, "Target Settings") || !strings.Contains(out, "Valid") {
		t.Fatalf("output = %q", out)
	}
	assertResolvedLabels(t, out, "Label", "Target", "Config File")
	assertResolvedValues(t, out, map[string]string{
		"Label":       "homes",
		"Target":      "onsite-usb",
		"Config File": filepath.Join(configDir, "homes-backup.toml"),
	})
	assertValidationLabels(t, out, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, out, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
	if strings.Contains(out, "Local Owner") || strings.Contains(out, "Local Group") {
		t.Fatalf("output = %q", out)
	}
	if !strings.Contains(out, "Repository Access") || !strings.Contains(out, "Valid") {
		t.Fatalf("output = %q", out)
	}
}

func TestHandleConfigCommand_ValidateExplicitRemoteTarget(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{},
	)
	sourcePath := t.TempDir()
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", sourcePath, "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config validation succeeded for homes/offsite-storj", "Section: Resolved", "Section: Validation", "Label", "homes", "Target", "offsite-storj", "Source Path Access", "Present", "Required Settings", "Health Thresholds", "Secrets", "Storage Access", "Resolved", "Threads", "Valid", "Prune Policy", "Result", "Passed"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	assertResolvedLabels(t, out, "Label", "Target", "Config File")
	assertResolvedValues(t, out, map[string]string{
		"Label":       "homes",
		"Target":      "offsite-storj",
		"Config File": filepath.Join(configDir, "homes-backup.toml"),
	})
	assertValidationLabels(t, out, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, out, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
}

func TestHandleConfigCommand_ValidateReportsInvalidHealthThresholds(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{Err: os.ErrPermission},
	)

	sourcePath := t.TempDir()
	destination := t.TempDir()
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildLabelConfig("homes", "onsite-usb", locationLocal, sourcePath, filepath.Join(destination, "homes"), owner, group, 4, "-keep 0:365", `
[health]
doctor_warn_after_hours = 48000000
`))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(OperatorMessage(err), "Config validation failed for homes/onsite-usb") {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/onsite-usb", "Section: Resolved", "Section: Validation", "Health Thresholds", "Invalid (health.doctor_warn_after_hours must be less than or equal to", "Storage Access", "Writable", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
	assertResolvedLabels(t, report, "Label", "Target", "Config File")
	assertResolvedValues(t, report, map[string]string{
		"Label":       "homes",
		"Target":      "onsite-usb",
		"Config File": filepath.Join(configDir, "homes-backup.toml"),
	})
	assertValidationLabels(t, report, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, report, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
}

func TestHandleConfigCommand_ValidateFailsWhenSourcePathIsNotBtrfsSubvolume(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "257\n"},
	)

	owner, group := currentUserGroup(t)
	sourcePath := t.TempDir()
	nestedSourcePath := filepath.Join(sourcePath, "private-user-data")
	if err := os.MkdirAll(nestedSourcePath, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", nestedSourcePath, t.TempDir(), owner, group, 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(OperatorMessage(err), "Config validation failed for homes/onsite-usb") {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/onsite-usb", "Section: Resolved", "Section: Validation", "Source Path Access", "Present", "Btrfs Source", "Invalid (Btrfs validation failed", "Storage Access", "Writable", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
	assertResolvedLabels(t, report, "Label", "Target", "Config File")
	assertResolvedValues(t, report, map[string]string{
		"Label":       "homes",
		"Target":      "onsite-usb",
		"Config File": filepath.Join(configDir, "homes-backup.toml"),
	})
	assertValidationLabels(t, report, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, report, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
}

func TestHandleConfigCommand_ValidateDoesNotRequireNonRootSourceReadAccess(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{},
	)

	owner, group := currentUserGroup(t)
	sourcePath := filepath.Join(t.TempDir(), "protected-source")
	if err := os.Mkdir(sourcePath, 0000); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sourcePath, 0700) })

	destinationRoot := t.TempDir()
	destination := filepath.Join(destinationRoot, "homes")
	if err := os.MkdirAll(filepath.Join(destination, "snapshots"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", sourcePath, destinationRoot, owner, group, 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v\n%s", err, ConfigCommandOutput(err))
	}
	for _, token := range []string{"Source Path Access", "Present", "Btrfs Source", "Valid", "Result", "Passed"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "source_path is not readable") {
		t.Fatalf("config validate should not require source directory read access:\n%s", out)
	}
}

func TestHandleConfigCommand_ValidateFailsWhenLocalDestinationDoesNotExist(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{Stderr: "Storage has not been initialized yet; initialize the storage first\n", Err: os.ErrInvalid},
	)

	owner, group := currentUserGroup(t)
	sourcePath := t.TempDir()
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", sourcePath, filepath.Join(t.TempDir(), "missing-destination"), owner, group, 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(OperatorMessage(err), "Config validation failed for homes/onsite-usb") {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/onsite-usb", "Btrfs Source", "Valid", "Storage Access", "Invalid (duplicacy local storage parent does not exist", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
	assertResolvedLabels(t, report, "Label", "Target", "Config File")
	assertResolvedValues(t, report, map[string]string{
		"Label":       "homes",
		"Target":      "onsite-usb",
		"Config File": filepath.Join(configDir, "homes-backup.toml"),
	})
	assertValidationLabels(t, report, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, report, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
}

func TestHandleConfigCommand_ValidateFailsWhenDuplicacyStorageIsInvalid(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
	)
	sourcePath := t.TempDir()
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", sourcePath, "not-a-duplicacy-url", 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "offsite-storj"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(OperatorMessage(err), "Config validation failed for homes/offsite-storj") {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/offsite-storj", "Storage Access", "Invalid (duplicacy local storage must be an absolute path or a URL-like storage target", "Repository Access", "Not checked", "Secrets", "Not required", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
	assertResolvedLabels(t, report, "Label", "Target", "Config File")
	assertResolvedValues(t, report, map[string]string{
		"Label":       "homes",
		"Target":      "offsite-storj",
		"Config File": filepath.Join(configDir, "homes-backup.toml"),
	})
	assertValidationLabels(t, report, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, report, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
}

func TestHandleConfigCommand_ValidateFailsWhenS3SecretsUseLegacyStorjKeys(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
	)

	sourcePath := t.TempDir()
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", sourcePath, "s3://bucket/homes", 4, "-keep 0:365"))
	secretsPath := filepath.Join(secretsDir, "homes-secrets.toml")
	body := "[targets.offsite-storj.keys]\nstorj_s3_id = \"ABCDEFGHIJKLMNOPQRSTUVWXYZ01\"\nstorj_s3_secret = \"abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR\"\n"
	if err := os.WriteFile(secretsPath, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/offsite-storj", "Storage Access", "Resolved", "Repository Access", "Not checked", "Secrets", "Invalid (storage \"s3\" requires s3_id and s3_secret in [targets.<name>.keys])", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
	assertValidationLabels(t, report, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
}

func TestHandleConfigCommand_ValidateFailsWhenLocalRepositoryIsNotInitialized(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{Stderr: "Storage has not been initialized yet; initialize the storage first\n", Err: os.ErrInvalid},
	)

	sourcePath := t.TempDir()
	destinationRoot := t.TempDir()
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", sourcePath, destinationRoot, owner, group, 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(OperatorMessage(err), "initialize the repository before running backups") {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/onsite-usb", "Repository Access", "Not initialized", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
	assertValidationLabels(t, report, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, report, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
	assertAllowedValidationOutcomes(t, report)
}

func TestHandleConfigCommand_ValidateFailsWhenRemoteRepositoryIsNotInitialized(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{Stderr: "Storage has not been initialized yet; initialize the storage first\n", Err: os.ErrInvalid},
	)
	sourcePath := t.TempDir()
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", sourcePath, "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(OperatorMessage(err), "initialize the repository before running backups") {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/offsite-storj", "Repository Access", "Not initialized", "Storage Access", "Resolved", "Secrets", "Valid", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
	assertValidationLabels(t, report, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, report, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
	assertAllowedValidationOutcomes(t, report)
}

func TestHandleConfigCommand_ValidateFailsWhenLocalRepositoryIsInaccessible(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
	)

	sourcePath := t.TempDir()
	destinationRoot := t.TempDir()
	repositoryPath := filepath.Join(destinationRoot, "homes")
	if err := os.WriteFile(repositoryPath, []byte("not-a-directory"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", sourcePath, destinationRoot, owner, group, 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil {
		t.Fatal("HandleConfigCommand() expected error")
	}
	if strings.Contains(OperatorMessage(err), "initialize the repository before running backups") {
		t.Fatalf("OperatorMessage(err) = %q", OperatorMessage(err))
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/onsite-usb", "Storage Access", "Invalid (duplicacy local storage must be a directory", "Repository Access", "Not checked", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
	assertValidationLabels(t, report, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, report, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
	assertAllowedValidationOutcomes(t, report)
}

func TestHandleConfigCommand_ValidateFailsWhenRemoteRepositoryIsInaccessible(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{Stderr: "permission denied\n", Err: os.ErrPermission},
	)
	sourcePath := t.TempDir()
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", sourcePath, "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil {
		t.Fatal("HandleConfigCommand() expected error")
	}
	if strings.Contains(OperatorMessage(err), "initialize the repository before running backups") {
		t.Fatalf("OperatorMessage(err) = %q", OperatorMessage(err))
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/offsite-storj", "Storage Access", "Resolved", "Secrets", "Valid", "Repository Access", "Invalid (Repository is not ready)", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
	assertValidationLabels(t, report, "Config", "Required Settings", "Threads", "Prune Policy", "Health Thresholds", "Source Path Access", "Btrfs Source", "Storage Access", "Repository Access", "Target Settings", "Secrets")
	assertValidationExcludesLabels(t, report, "Source Path", "Destination", "Destination Host", "Local Owner", "Local Group")
	assertAllowedValidationOutcomes(t, report)
}

func TestHandleConfigCommand_ExplainLocalAndPaths(t *testing.T) {
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", "/volume1/homes", "/backups", owner, group, 4, "-keep 0:365"))

	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	rt := testRuntime()

	explainReq := &Request{ConfigCommand: "explain", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	explainOut, err := HandleConfigCommand(explainReq, meta, rt)
	if err != nil {
		t.Fatalf("HandleConfigCommand(explain) error = %v", err)
	}
	for _, token := range []string{"Config explanation for homes/onsite-usb", "Target", "onsite-usb", "Local Owner", owner, "Local Group", group, "Storage", "/backups/homes"} {
		if !strings.Contains(explainOut, token) {
			t.Fatalf("explain output missing %q:\n%s", token, explainOut)
		}
	}
	assertFlatLabels(t, explainOut, "Label", "Target", "Location", "Config File", "Source", "Storage", "Threads", "Prune Policy", "Allow Local Accounts", "Local Owner", "Local Group")

	pathsReq := &Request{ConfigCommand: "paths", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	pathsOut, err := HandleConfigCommand(pathsReq, meta, rt)
	if err != nil {
		t.Fatalf("HandleConfigCommand(paths) error = %v", err)
	}
	for _, token := range []string{"Resolved paths for homes", "Config Dir", "Config File", "Source Path", "Log Dir"} {
		if !strings.Contains(pathsOut, token) {
			t.Fatalf("paths output missing %q:\n%s", token, pathsOut)
		}
	}
	if strings.Contains(pathsOut, "Secrets File") || strings.Contains(pathsOut, "Secrets Dir") {
		t.Fatalf("paths output should omit secrets for local-only targets:\n%s", pathsOut)
	}
	if strings.Contains(pathsOut, "Work Dir") || strings.Contains(pathsOut, "Snapshot") {
		t.Fatalf("paths output should only contain stable paths:\n%s", pathsOut)
	}
	assertFlatLabels(t, pathsOut, "Label", "Target", "Location", "Config Dir", "Config File", "Source Path", "Log Dir")
}

func TestHandleConfigCommand_ExplainRemoteDoesNotRequireSecretsAccess(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket", 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "explain", Source: "homes", ConfigDir: configDir, RequestedTarget: "offsite-storj"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config explanation for homes/offsite-storj", "Secrets File", "homes-secrets.toml"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	for _, token := range []string{"Remote Access Key", "Remote Secret Key"} {
		if strings.Contains(out, token) {
			t.Fatalf("output should not include %q:\n%s", token, out)
		}
	}
	assertFlatLabels(t, out, "Label", "Target", "Location", "Config File", "Source", "Storage", "Threads", "Prune Policy", "Secrets File")
}

func TestHandleConfigCommand_ExplainLocalDuplicacyDoesNotRequireSecretsAccess(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-rustfs", localDuplicacyTargetConfig("homes", "/volume1/homes", "s3://rustfs.local/bucket", 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "explain", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-rustfs"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config explanation for homes/onsite-rustfs", "Location", "local", "Storage", "s3://rustfs.local/bucket", "Secrets File", "homes-secrets.toml"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	for _, token := range []string{"Storage Access Key", "Storage Secret Key", "Remote Access Key", "Remote Secret Key"} {
		if strings.Contains(out, token) {
			t.Fatalf("output should not include %q:\n%s", token, out)
		}
	}
	assertFlatLabels(t, out, "Label", "Target", "Location", "Config File", "Source", "Storage", "Threads", "Prune Policy", "Secrets File")
}

func TestHandleConfigCommand_ExplainLocalMinioIncludesSecretsFile(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-garage", buildTargetConfig("homes", "onsite-garage", locationLocal, "/volume1/homes", "minio://garage@192.168.202.24:3900/garage/homes", "", "", 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "explain", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-garage"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config explanation for homes/onsite-garage", "Storage", "minio://garage@192.168.202.24:3900/garage/homes", "Secrets File", "homes-secrets.toml"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	assertFlatLabels(t, out, "Label", "Target", "Location", "Config File", "Source", "Storage", "Threads", "Prune Policy", "Secrets File")
}

func TestHandleConfigCommand_ExplainIncludesFilterWhenConfigured(t *testing.T) {
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", fmt.Sprintf(`label = "homes"
source_path = "/volume1/homes"

[common]
threads = 4
filter = "-e *.tmp"
prune = "-keep 0:365"

[targets.onsite-usb]
location = "local"
storage = "/backups/homes"
allow_local_accounts = true
local_owner = %q
local_group = %q
`, owner, group))

	req := &Request{ConfigCommand: "explain", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	if !strings.Contains(out, "Filter") || !strings.Contains(out, "-e *.tmp") {
		t.Fatalf("output missing filter:\n%s", out)
	}
	assertFlatLabels(t, out, "Label", "Target", "Location", "Config File", "Source", "Storage", "Threads", "Filter", "Prune Policy", "Allow Local Accounts", "Local Owner", "Local Group")
}

func TestHandleConfigCommand_PathsDuplicacyStorageIncludesSecrets(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket", 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "paths", Source: "homes", ConfigDir: configDir, RequestedTarget: "offsite-storj"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand(paths remote) error = %v", err)
	}
	for _, token := range []string{"Resolved paths for homes", "Secrets Dir", "Secrets File", "homes-secrets.toml"} {
		if !strings.Contains(out, token) {
			t.Fatalf("remote paths output missing %q:\n%s", token, out)
		}
	}
	assertFlatLabels(t, out, "Label", "Target", "Location", "Config Dir", "Config File", "Source Path", "Log Dir", "Secrets Dir", "Secrets File")
}

func TestHandleConfigCommand_PathsLocalDuplicacyIncludesSecrets(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-rustfs", localDuplicacyTargetConfig("homes", "/volume1/homes", "s3://rustfs.local/bucket", 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "paths", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-rustfs"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand(paths local duplicacy) error = %v", err)
	}
	for _, token := range []string{"Resolved paths for homes", "Location", "local", "Secrets Dir", "Secrets File", "homes-secrets.toml"} {
		if !strings.Contains(out, token) {
			t.Fatalf("local duplicacy paths output missing %q:\n%s", token, out)
		}
	}
	assertFlatLabels(t, out, "Label", "Target", "Location", "Config Dir", "Config File", "Source Path", "Log Dir", "Secrets Dir", "Secrets File")
}

func TestHandleConfigCommand_Unsupported(t *testing.T) {
	_, err := HandleConfigCommand(&Request{ConfigCommand: "unknown", Source: "homes"}, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "unsupported config command") {
		t.Fatalf("err = %v", err)
	}
}

func TestColourizeConfigValidationValue(t *testing.T) {
	if got := colourizeConfigValidationValue("Invalid (boom)", false); got != "Invalid (boom)" {
		t.Fatalf("got %q", got)
	}
	for _, tc := range []struct {
		value string
		want  string
	}{
		{"Valid", "\033["},
		{"Resolved", "\033["},
		{"Not checked", "\033["},
		{"Invalid (boom)", "\033["},
	} {
		got := colourizeConfigValidationValue(tc.value, true)
		if !strings.Contains(got, tc.want) || !strings.Contains(got, tc.value) {
			t.Fatalf("colourizeConfigValidationValue(%q) = %q", tc.value, got)
		}
	}
}

func TestConfigValidateValidationSectionUsesAllowedOutcomes(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Stdout: "256\n"},
		execpkg.MockResult{},
	)
	owner, group := currentUserGroup(t)
	sourcePath := t.TempDir()
	localDestination := t.TempDir()
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	localHomesRepo := filepath.Join(localDestination, "homes")
	if err := os.MkdirAll(filepath.Join(localHomesRepo, "snapshots"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	localReadmediaRepo := filepath.Join(localDestination, "readmedia")
	if err := os.MkdirAll(filepath.Join(localReadmediaRepo, "snapshots"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", sourcePath, localDestination, owner, group, 4, "-keep 0:365"))
	writeTargetTestConfig(t, configDir, "readmedia", "onsite-usb", buildTargetConfig("readmedia", "onsite-usb", locationLocal, sourcePath, filepath.Join(localDestination, "readmedia"), "", "", 4, ""))
	writeTargetTestConfig(t, configDir, "archives", "offsite-storj", remoteTargetConfig("archives", sourcePath, "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "archives", "offsite-storj")

	t.Run("local-enabled", func(t *testing.T) {
		req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
		out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
		if err != nil {
			t.Fatalf("HandleConfigCommand() error = %v", err)
		}
		assertAllowedValidationOutcomes(t, out)
	})

	t.Run("local-not-enabled", func(t *testing.T) {
		req := &Request{ConfigCommand: "validate", Source: "readmedia", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
		out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
		if err != nil {
			t.Fatalf("HandleConfigCommand() error = %v", err)
		}
		assertAllowedValidationOutcomes(t, out)
	})

	t.Run("remote", func(t *testing.T) {
		req := &Request{ConfigCommand: "validate", Source: "archives", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
		out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
		if err != nil {
			t.Fatalf("HandleConfigCommand() error = %v", err)
		}
		assertAllowedValidationOutcomes(t, out)
	})
}

func TestConfigValidateValidationSectionRejectsStatusPayloadHybrids(t *testing.T) {
	report := strings.Join([]string{
		"Config validation succeeded for homes/offsite-storj",
		"  Section: Resolved",
		"    Target             : offsite-storj",
		"  Section: Validation",
		"    Threads            : Valid (16)",
		"    Storage Access     : Resolved (gateway.example.test)",
		"    Target Settings    : Valid (backup:users)",
		"  Result               : Passed",
		"",
	}, "\n")

	values := extractValidationValues(t, report)
	for label, value := range values {
		if allowedValidationOutcome(value) {
			t.Fatalf("expected hybrid status payload %q for %q to be rejected", value, label)
		}
	}
}

func assertAllowedValidationOutcomes(t *testing.T, report string) {
	t.Helper()
	values := extractValidationValues(t, report)
	if len(values) == 0 {
		t.Fatalf("no validation values found in report:\n%s", report)
	}
	for label, value := range values {
		if !allowedValidationOutcome(value) {
			t.Fatalf("validation value for %q used unsupported outcome %q:\n%s", label, value, report)
		}
	}
}

func assertResolvedLabels(t *testing.T, report string, want ...string) {
	t.Helper()
	labels := extractSectionLabels(t, report, "Resolved")
	if len(labels) != len(want) {
		t.Fatalf("resolved section labels = %v, want %v:\n%s", labels, want, report)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("resolved section labels = %v, want %v:\n%s", labels, want, report)
		}
	}
}

func assertValidationLabels(t *testing.T, report string, want ...string) {
	t.Helper()
	labels := extractSectionLabels(t, report, "Validation")
	if len(labels) != len(want) {
		t.Fatalf("validation section labels = %v, want %v:\n%s", labels, want, report)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("validation section labels = %v, want %v:\n%s", labels, want, report)
		}
	}
}

func assertResolvedValues(t *testing.T, report string, want map[string]string) {
	t.Helper()
	values := extractSectionValues(t, report, "Resolved")
	if len(values) != len(want) {
		t.Fatalf("resolved section values = %v, want %v:\n%s", values, want, report)
	}
	for label, expected := range want {
		if values[label] != expected {
			t.Fatalf("resolved section value for %q = %q, want %q:\n%s", label, values[label], expected, report)
		}
	}
}

func assertValidationExcludesLabels(t *testing.T, report string, forbidden ...string) {
	t.Helper()
	values := extractSectionValues(t, report, "Validation")
	for _, label := range forbidden {
		if _, ok := values[label]; ok {
			t.Fatalf("validation section unexpectedly contained %q:\n%s", label, report)
		}
	}
}

func assertFlatLabels(t *testing.T, report string, want ...string) {
	t.Helper()
	labels := extractFlatLabels(t, report)
	if len(labels) != len(want) {
		t.Fatalf("flat labels = %v, want %v:\n%s", labels, want, report)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("flat labels = %v, want %v:\n%s", labels, want, report)
		}
	}
}

func extractValidationValues(t *testing.T, report string) map[string]string {
	t.Helper()
	return extractSectionValues(t, report, "Validation")
}

func extractSectionLabels(t *testing.T, report string, section string) []string {
	t.Helper()
	lines := extractSectionLines(t, report, section)
	labels := make([]string, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), " : ", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected %s line format %q in report:\n%s", section, line, report)
		}
		labels = append(labels, strings.TrimSpace(parts[0]))
	}
	return labels
}

func extractSectionValues(t *testing.T, report string, section string) map[string]string {
	t.Helper()
	lines := extractSectionLines(t, report, section)
	values := map[string]string{}
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), " : ", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected %s line format %q in report:\n%s", section, line, report)
		}
		values[strings.TrimSpace(parts[0])] = parts[1]
	}
	return values
}

func extractSectionLines(t *testing.T, report string, section string) []string {
	t.Helper()
	lines := strings.Split(report, "\n")
	inSection := false
	var collected []string
	for _, line := range lines {
		switch {
		case line == "  Section: "+section:
			inSection = true
			continue
		case inSection && strings.HasPrefix(line, "  Section: "):
			return collected
		case inSection && strings.HasPrefix(line, "  Result"):
			return collected
		case !inSection || strings.TrimSpace(line) == "":
			continue
		case strings.HasPrefix(line, "    "):
			collected = append(collected, line)
		default:
			t.Fatalf("unexpected %s section line %q in report:\n%s", section, line, report)
		}
	}
	return collected
}

func extractFlatLabels(t *testing.T, report string) []string {
	t.Helper()
	lines := strings.Split(report, "\n")
	labels := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "  Section: ") || strings.HasPrefix(line, "  Result") {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(line), " : ", 2)
		if len(parts) != 2 {
			continue
		}
		labels = append(labels, strings.TrimSpace(parts[0]))
	}
	if len(labels) == 0 {
		t.Fatalf("no flat labels found in report:\n%s", report)
	}
	return labels
}

var invalidValidationOutcomePattern = regexp.MustCompile(`^Invalid \(.+\)$`)

func allowedValidationOutcome(value string) bool {
	switch value {
	case "Valid", "Present", "Writable", "Resolved", "Parsed", "Not checked", "Not configured", "Not enabled", "Not required", "Requires sudo":
		return true
	case "Not initialized":
		return true
	default:
		return invalidValidationOutcomePattern.MatchString(value)
	}
}
