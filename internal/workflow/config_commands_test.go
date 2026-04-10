package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWorkflowConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "homes-backup.toml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeWorkflowSecrets(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "duplicacy-homes.toml")
	body := "storj_s3_id = \"ABCDEFGHIJKLMNOPQRSTUVWXYZ01\"\nstorj_s3_secret = \"abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR\"\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if os.Getuid() == 0 {
		if err := os.Chown(path, 0, 0); err != nil {
			t.Fatalf("Chown() error = %v", err)
		}
	}
	return path
}

func TestHandleConfigCommand_ValidateConfiguredRemote(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets validation requires root-owned test file")
	}

	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeWorkflowConfig(t, configDir, "[common]\ndestination = \"s3://bucket\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n[remote]\n")
	writeWorkflowSecrets(t, secretsDir)

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}

	required := []string{
		"Config validation succeeded for homes",
		"Local Config",
		"Valid",
		"Remote Config",
		"Remote Secrets",
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
	writeWorkflowConfig(t, configDir, "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n")

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	if !strings.Contains(out, "Remote Config") || !strings.Contains(out, "Not configured") {
		t.Fatalf("output = %q", out)
	}
}

func TestHandleConfigCommand_ValidateRemoteMode(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote secrets validation requires root-owned test file")
	}

	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	writeWorkflowConfig(t, configDir, "[common]\ndestination = \"s3://bucket\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n[remote]\n")
	writeWorkflowSecrets(t, secretsDir)

	req := &Request{ConfigCommand: "validate", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RemoteMode: true}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config validation succeeded for homes", "Remote Config", "Remote Secrets"} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleConfigCommand_ExplainLocalAndPaths(t *testing.T) {
	owner, group := currentUserGroup(t)
	configDir := t.TempDir()
	writeWorkflowConfig(t, configDir, "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n")

	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	rt := testRuntime()

	explainReq := &Request{ConfigCommand: "explain", Source: "homes", ConfigDir: configDir}
	explainOut, err := HandleConfigCommand(explainReq, meta, rt)
	if err != nil {
		t.Fatalf("HandleConfigCommand(explain) error = %v", err)
	}
	for _, token := range []string{"Config explanation for homes", "Local Owner", owner, "Local Group", group, "Destination", "/backups/homes"} {
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
	writeWorkflowConfig(t, configDir, "[common]\ndestination = \"s3://bucket\"\nprune = \"-keep 0:365\"\nthreads = 4\n[remote]\n")
	writeWorkflowSecrets(t, secretsDir)

	req := &Request{ConfigCommand: "explain", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RemoteMode: true}
	out, err := HandleConfigCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err != nil {
		t.Fatalf("HandleConfigCommand() error = %v", err)
	}
	for _, token := range []string{"Config explanation for homes", "Secrets File", "Remote Access Key", "****YZ01", "Remote Secret Key"} {
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
