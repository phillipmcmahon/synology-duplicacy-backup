package workflow

import (
	"os"
	"strings"
	"testing"
)

func TestHandleConfigCommand_ValidateConfiguredRemote(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets validation requires root-owned test file")
	}

	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}

	required := []string{
		"Config validation succeeded for homes/offsite-storj",
		"Target",
		"offsite-storj",
		"Config File",
		"homes-backup.toml",
		"Valid",
		"Secrets",
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
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", "/volume1/homes", "/backups", owner, group, 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config validation succeeded for homes/onsite-usb", "Target", "onsite-usb"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleConfigCommand_ValidateLocalReadOnlyTargetWithoutOwnerGroup(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", "local", "/volume1/homes", "/backups", "homes", "", "", 0, ""))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	if !strings.Contains(out, "Config validation succeeded for homes/onsite-usb") {
		t.Fatalf("output = %q", out)
	}
}

func TestHandleConfigCommand_ValidateExplicitRemoteTarget(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets validation requires root-owned test file")
	}

	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config validation succeeded for homes/offsite-storj", "Target", "offsite-storj", "Secrets"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
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
