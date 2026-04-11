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
	writeTargetTestConfig(t, configDir, "homes", "remote", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "remote")

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "remote", RemoteMode: true}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}

	required := []string{
		"Config validation succeeded for homes/remote",
		"Target",
		"remote",
		"Config File",
		"homes-remote-backup.toml",
		"Valid",
		"Secrets",
	}
	for _, token := range required {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleConfigCommand_ValidateLocalOnlyConfig(t *testing.T) {
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "local", localTargetConfig("homes", "/volume1/homes", "/backups", owner, group, 4, "-keep 0:365"))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	if !strings.Contains(out, "Config validation succeeded for homes/local") ||
		!strings.Contains(out, "Target") ||
		!strings.Contains(out, "homes-local-backup.toml") ||
		strings.Contains(out, "Not configured") {
		t.Fatalf("output = %q", out)
	}
}

func TestHandleConfigCommand_ValidateLocalReadOnlyTargetWithoutOwnerGroup(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "local", buildTargetConfig("homes", "local", "local", "/volume1/homes", "/backups", "homes", "", "", 0, ""))

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	if !strings.Contains(out, "Config validation succeeded for homes/local") {
		t.Fatalf("output = %q", out)
	}
}

func TestHandleConfigCommand_ValidateRemoteMode(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets validation requires root-owned test file")
	}

	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "remote", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "remote")

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "remote", RemoteMode: true}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config validation succeeded for homes/remote", "Target", "remote", "Secrets"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleConfigCommand_ExplainLocalAndPaths(t *testing.T) {
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "local", localTargetConfig("homes", "/volume1/homes", "/backups", owner, group, 4, "-keep 0:365"))

	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	rt := testRuntime()

	explainReq := &Request{ConfigCommand: "explain", Source: "homes", ConfigDir: configDir}
	explainOut, err := HandleConfigCommand(explainReq, meta, rt)
	if err != nil {
		t.Fatalf("HandleConfigCommand(explain) error = %v", err)
	}
	for _, token := range []string{"Config explanation for homes/local", "Target", "local", "Local Owner", owner, "Local Group", group, "Destination", "/backups/homes"} {
		if !strings.Contains(explainOut, token) {
			t.Fatalf("explain output missing %q:\n%s", token, explainOut)
		}
	}

	pathsReq := &Request{ConfigCommand: "paths", Source: "homes", ConfigDir: configDir}
	pathsOut, err := HandleConfigCommand(pathsReq, meta, rt)
	if err != nil {
		t.Fatalf("HandleConfigCommand(paths) error = %v", err)
	}
	for _, token := range []string{"Resolved paths for homes", "Config Dir", "Config File", "Secrets File", "Source Path", "Log Dir"} {
		if !strings.Contains(pathsOut, token) {
			t.Fatalf("paths output missing %q:\n%s", token, pathsOut)
		}
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
	writeTargetTestConfig(t, configDir, "homes", "remote", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "remote")

	req := &Request{ConfigCommand: "explain", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "remote", RemoteMode: true}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config explanation for homes/remote", "Secrets File", "duplicacy-homes-remote.toml", "Remote Access Key", "****YZ01", "Remote Secret Key"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleConfigCommand_Unsupported(t *testing.T) {
	_, err := HandleConfigCommand(&Request{ConfigCommand: "unknown", Source: "homes"}, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "unsupported config command") {
		t.Fatalf("err = %v", err)
	}
}
