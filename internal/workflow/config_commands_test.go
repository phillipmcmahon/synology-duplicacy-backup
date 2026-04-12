package workflow

import (
	"os"
	"path/filepath"
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

func stubConfigDestinationHostResolver(t *testing.T, fn func(string) ([]string, error)) {
	t.Helper()
	original := resolveConfigDestinationHost
	resolveConfigDestinationHost = fn
	t.Cleanup(func() {
		resolveConfigDestinationHost = original
	})
}

func TestHandleConfigCommand_ValidateConfiguredRemote(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets validation requires root-owned test file")
	}

	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
	)
	stubConfigDestinationHostResolver(t, func(host string) ([]string, error) {
		return []string{"203.0.113.10"}, nil
	})

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
		"Target",
		"offsite-storj",
		"Config File",
		"homes-backup.toml",
		"Valid",
		"Source Path",
		sourcePath,
		"Source Path Access",
		"Readable",
		"Btrfs Source",
		"Required Settings",
		"Health Thresholds",
		"Threads",
		"Prune Policy",
		"Destination Access",
		"Secrets",
		"Result",
		"Passed",
	}
	for _, token := range required {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleConfigCommand_ValidateRequiresExplicitTarget(t *testing.T) {
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildLabelConfig("homes", "onsite-usb", "local", "/volume1/homes", "/backups", "homes", owner, group, 4, "-keep 0:365", `
[targets.offsite-storj]
type = "remote"
destination = "s3://bucket"
repository = "homes"
requires_network = true
`))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir}
	if _, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime()); err == nil || !strings.Contains(err.Error(), "requires an explicit target selection") {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
}

func TestHandleConfigCommand_ValidateExplicitLocalTarget(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
	)

	sourcePath := t.TempDir()
	destination := t.TempDir()
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", sourcePath, destination, owner, group, 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config validation succeeded for homes/onsite-usb", "Section: Resolved", "Section: Validation", "Target", "onsite-usb", "Source Path", sourcePath, "Destination", destination, "Source Path Access", "Readable", "Btrfs Source", "Valid", "Required Settings", "Health Thresholds", "Destination Access", "Writable", "Threads", "Valid (4)", "Prune Policy", "Local Accounts", "Result", "Passed"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleConfigCommand_ValidateLocalReadOnlyTargetWithoutOwnerGroup(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
	)

	sourcePath := t.TempDir()
	destination := t.TempDir()
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", "local", sourcePath, destination, "homes", "", "", 4, ""))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	if !strings.Contains(out, "Config validation succeeded for homes/onsite-usb") {
		t.Fatalf("output = %q", out)
	}
	if !strings.Contains(out, "Local Accounts") || !strings.Contains(out, "Not enabled") {
		t.Fatalf("output = %q", out)
	}
}

func TestHandleConfigCommand_ValidateExplicitRemoteTarget(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets validation requires root-owned test file")
	}

	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
	)
	stubConfigDestinationHostResolver(t, func(host string) ([]string, error) {
		return []string{"203.0.113.10"}, nil
	})

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
	for _, token := range []string{"Config validation succeeded for homes/offsite-storj", "Section: Resolved", "Section: Validation", "Target", "offsite-storj", "Source Path Access", "Readable", "Required Settings", "Health Thresholds", "Secrets", "Destination Access", "Resolved (bucket)", "Threads", "Prune Policy", "Result", "Passed"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleConfigCommand_ValidateReportsInvalidHealthThresholds(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
	)

	sourcePath := t.TempDir()
	destination := t.TempDir()
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildLabelConfig("homes", "onsite-usb", "local", sourcePath, destination, "homes", owner, group, 4, "-keep 0:365", `
[health]
doctor_warn_after_hours = 48000000
`))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(OperatorMessage(err), "Config validation failed for homes/onsite-usb") {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/onsite-usb", "Section: Resolved", "Section: Validation", "Health Thresholds", "Invalid (health.doctor_warn_after_hours must be less than or equal to", "Destination Access", "Writable", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
}

func TestHandleConfigCommand_ValidateFailsWhenSourcePathIsNotBtrfsSubvolume(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{Err: os.ErrInvalid},
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
	for _, token := range []string{"Config validation failed for homes/onsite-usb", "Section: Resolved", "Section: Validation", "Source Path Access", "Readable", "Btrfs Source", "Invalid (Btrfs validation failed", "Destination Access", "Writable", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
}

func TestHandleConfigCommand_ValidateFailsWhenLocalDestinationDoesNotExist(t *testing.T) {
	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
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
	for _, token := range []string{"Config validation failed for homes/onsite-usb", "Btrfs Source", "Valid", "Destination Access", "Invalid (local destination does not exist", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
}

func TestHandleConfigCommand_ValidateFailsWhenRemoteDestinationHostCannotResolve(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets validation requires root-owned test file")
	}

	stubConfigCommandRunner(t,
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
	)
	stubConfigDestinationHostResolver(t, func(host string) ([]string, error) {
		return nil, os.ErrNotExist
	})

	sourcePath := t.TempDir()
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", sourcePath, "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	_, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(OperatorMessage(err), "Config validation failed for homes/offsite-storj") {
		t.Fatalf("HandleConfigCommand() err = %v", err)
	}
	report := ConfigCommandOutput(err)
	for _, token := range []string{"Config validation failed for homes/offsite-storj", "Destination Access", "Invalid (remote destination host could not be resolved", "Secrets", "Valid", "Result", "Failed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("report missing %q:\n%s", token, report)
		}
	}
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
	for _, token := range []string{"Config explanation for homes/onsite-usb", "Target", "onsite-usb", "Local Owner", owner, "Local Group", group, "Destination", "/backups/homes"} {
		if !strings.Contains(explainOut, token) {
			t.Fatalf("explain output missing %q:\n%s", token, explainOut)
		}
	}

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
}

func TestHandleConfigCommand_ExplainRemoteMasksSecrets(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets validation requires root-owned test file")
	}

	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{ConfigCommand: "explain", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config explanation for homes/offsite-storj", "Secrets File", "homes-secrets.toml", "Remote Access Key", "****YZ01", "Remote Secret Key"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleConfigCommand_PathsRemoteIncludesSecrets(t *testing.T) {
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
		{"Resolved (example.invalid)", "\033["},
		{"Not checked", "\033["},
		{"Invalid (boom)", "\033["},
	} {
		got := colourizeConfigValidationValue(tc.value, true)
		if !strings.Contains(got, tc.want) || !strings.Contains(got, tc.value) {
			t.Fatalf("colourizeConfigValidationValue(%q) = %q", tc.value, got)
		}
	}
}
