package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

func writeTempSecrets(t *testing.T, content string, perm os.FileMode) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(p, []byte(content), perm); err != nil {
		t.Fatalf("failed to write temp secrets: %v", err)
	}
	return p
}

func validID() string {
	return "ABCDEFGHIJKLMNOPQRSTUVWXYZ01"
}

func validSecret() string {
	return "abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR"
}

func validSecretContent() string {
	return validSecretContentForTarget("offsite-storj")
}

func validSecretContentForTarget(target string) string {
	return "[targets." + target + "]\nstorj_s3_id = \"" + validID() + "\"\nstorj_s3_secret = \"" + validSecret() + "\"\n"
}

func isRoot() bool {
	return os.Getuid() == 0
}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, errors.New("boom")
}

func TestGetSecretsFilePath(t *testing.T) {
	path := GetSecretsFilePath("/root/.secrets", "homes")
	if path != "/root/.secrets/homes-secrets.toml" {
		t.Fatalf("GetSecretsFilePath() = %q", path)
	}
	if GetSecretsFilePath("", "homes") != "homes-secrets.toml" {
		t.Fatal("empty dir should still use .toml file name")
	}
}

func TestLoadSecretsFile_MissingFile(t *testing.T) {
	_, err := LoadSecretsFile("/nonexistent/secrets.toml", "offsite-storj")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v", err)
	}
	var secretsErr *apperrors.SecretsError
	if !errors.As(err, &secretsErr) {
		t.Fatalf("expected SecretsError, got %T", err)
	}
}

func TestLoadSecretsFile_Permissions(t *testing.T) {
	for _, perm := range []os.FileMode{0644, 0666, 0604, 0640} {
		p := writeTempSecrets(t, validSecretContent(), perm)
		if _, err := LoadSecretsFile(p, "offsite-storj"); err == nil {
			t.Fatalf("expected permissions error for %04o", perm)
		}
	}
}

func TestLoadSecretsFile_OwnershipCheck(t *testing.T) {
	p := writeTempSecrets(t, validSecretContent(), 0600)
	_, err := LoadSecretsFile(p, "offsite-storj")
	if isRoot() && err != nil {
		t.Fatalf("unexpected error as root: %v", err)
	}
	if !isRoot() && err == nil {
		t.Fatal("expected ownership error as non-root")
	}
}

func TestParseSecrets_ValidContent(t *testing.T) {
	sec, err := ParseSecrets(strings.NewReader(validSecretContent()), "test", "offsite-storj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec.StorjS3ID != validID() || sec.StorjS3Secret != validSecret() {
		t.Fatalf("sec = %+v", sec)
	}
}

func TestParseSecrets_MissingRequiredKeys(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing id", "[targets.offsite-storj]\nstorj_s3_secret = \"" + validSecret() + "\"\n", "storj_s3_id"},
		{"missing secret", "[targets.offsite-storj]\nstorj_s3_id = \"" + validID() + "\"\n", "storj_s3_secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSecrets(strings.NewReader(tc.body), "test", "offsite-storj")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestParseSecrets_UnknownKeyRejected(t *testing.T) {
	_, err := ParseSecrets(strings.NewReader(validSecretContent()+"extra = \"nope\"\n"), "test", "offsite-storj")
	if err == nil {
		t.Fatal("expected unknown-key error")
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseSecrets_MalformedTOMLRejected(t *testing.T) {
	_, err := ParseSecrets(strings.NewReader("[targets.offsite-storj]\nstorj_s3_id = \"abc\"\nstorj_s3_secret = [\n"), "test", "offsite-storj")
	if err == nil {
		t.Fatal("expected invalid TOML error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid toml") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseSecrets_SourceInErrorMessage(t *testing.T) {
	_, err := ParseSecrets(strings.NewReader(""), "my-secrets.toml", "offsite-storj")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "my-secrets.toml") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseSecrets_ReaderError(t *testing.T) {
	_, err := ParseSecrets(errReader{}, "test.toml", "offsite-storj")
	if err == nil {
		t.Fatal("expected reader error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid toml") && !strings.Contains(strings.ToLower(err.Error()), "boom") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateFileAccess(t *testing.T) {
	if err := ValidateFileAccess("/nonexistent/secrets.toml"); err == nil {
		t.Fatal("expected missing-file error")
	}

	p := writeTempSecrets(t, "storj_s3_id = \"x\"\n", 0644)
	if err := ValidateFileAccess(p); err == nil {
		t.Fatal("expected permissions error")
	}
}

func TestValidate(t *testing.T) {
	if err := (&Secrets{StorjS3ID: validID(), StorjS3Secret: validSecret()}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []Secrets{
		{StorjS3ID: "short", StorjS3Secret: validSecret()},
		{StorjS3ID: validID(), StorjS3Secret: "short"},
		{StorjS3ID: "", StorjS3Secret: ""},
	}
	for _, s := range cases {
		if err := s.Validate(); err == nil {
			t.Fatalf("expected validation error for %+v", s)
		}
	}
}

func TestLoadOptionalHealthWebhookToken(t *testing.T) {
	if token, err := LoadOptionalHealthWebhookToken(filepath.Join(t.TempDir(), "missing.toml"), "offsite-storj"); err != nil || token != "" {
		t.Fatalf("missing token = %q, err = %v", token, err)
	}

	p := writeTempSecrets(t, validSecretContent()+"health_webhook_bearer_token = \"secret-token\"\n", 0600)
	token, err := LoadOptionalHealthWebhookToken(p, "offsite-storj")
	if isRoot() && err != nil {
		t.Fatalf("LoadOptionalHealthWebhookToken() error = %v", err)
	}
	if isRoot() && token != "secret-token" {
		t.Fatalf("token = %q", token)
	}
}

func TestLoadOptionalHealthNtfyToken(t *testing.T) {
	if token, err := LoadOptionalHealthNtfyToken(filepath.Join(t.TempDir(), "missing.toml"), "offsite-storj"); err != nil || token != "" {
		t.Fatalf("missing token = %q, err = %v", token, err)
	}

	p := writeTempSecrets(t, validSecretContent()+"health_ntfy_token = \"ntfy-token\"\n", 0600)
	token, err := LoadOptionalHealthNtfyToken(p, "offsite-storj")
	if isRoot() && err != nil {
		t.Fatalf("LoadOptionalHealthNtfyToken() error = %v", err)
	}
	if isRoot() && token != "ntfy-token" {
		t.Fatalf("token = %q", token)
	}
}

func TestParseSecrets_MissingTargetTable(t *testing.T) {
	_, err := ParseSecrets(strings.NewReader(validSecretContentForTarget("archive-cold")), "test", "offsite-storj")
	if err == nil {
		t.Fatal("expected missing target-table error")
	}
	if !strings.Contains(err.Error(), "[targets.offsite-storj]") {
		t.Fatalf("error = %v", err)
	}
}

func TestMaskedHelpers(t *testing.T) {
	if (&Secrets{StorjS3ID: "ABCDEFGHIJ"}).MaskedID() != "****GHIJ" {
		t.Fatal("unexpected masked ID")
	}
	if (&Secrets{StorjS3Secret: "ABCDEFGHIJ"}).MaskedSecret() != "****GHIJ" {
		t.Fatal("unexpected masked secret")
	}
	if (&Secrets{StorjS3ID: "AB"}).MaskedID() != "****" {
		t.Fatal("short masked ID should collapse")
	}
	if (&Secrets{StorjS3Secret: "AB"}).MaskedSecret() != "****" {
		t.Fatal("short masked secret should collapse")
	}
}
