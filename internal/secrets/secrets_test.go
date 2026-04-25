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
	return "[targets." + target + ".keys]\ns3_id = \"" + validID() + "\"\ns3_secret = \"" + validSecret() + "\"\n"
}

func validSecretContentWithTargetValue(target, line string) string {
	return "[targets." + target + "]\n" + line + "\n\n[targets." + target + ".keys]\ns3_id = \"" + validID() + "\"\ns3_secret = \"" + validSecret() + "\"\n"
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

func TestLoadSecretsFile_CurrentUserOwnershipCheck(t *testing.T) {
	p := writeTempSecrets(t, validSecretContent(), 0600)
	if _, err := LoadSecretsFile(p, "offsite-storj"); err != nil {
		t.Fatalf("unexpected ownership error for current user: %v", err)
	}
}

func TestParseSecrets_ValidContent(t *testing.T) {
	sec, err := ParseSecrets(strings.NewReader(validSecretContent()), "test", "offsite-storj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec.Keys["s3_id"] != validID() || sec.Keys["s3_secret"] != validSecret() {
		t.Fatalf("sec = %+v", sec)
	}
}

func TestParseSecrets_AllowsTargetWithoutStorageKeys(t *testing.T) {
	sec, err := ParseSecrets(strings.NewReader("[targets.offsite-storj]\nhealth_ntfy_token = \"optional\"\n"), "test", "offsite-storj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sec.Keys) != 0 {
		t.Fatalf("keys = %+v", sec.Keys)
	}
}

func TestValidate_RejectsEmptyStorageKeyValues(t *testing.T) {
	cases := []struct {
		name string
		sec  Secrets
		want string
	}{
		{"empty key", Secrets{Keys: map[string]string{"": "secret"}}, "names must not be empty"},
		{"blank key", Secrets{Keys: map[string]string{"   ": "secret"}}, "names must not be empty"},
		{"empty value", Secrets{Keys: map[string]string{"s3_secret": ""}}, "s3_secret"},
		{"blank value", Secrets{Keys: map[string]string{"s3_secret": "   "}}, "s3_secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.sec.Validate()
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
	_, err := ParseSecrets(strings.NewReader("[targets.offsite-storj]\nextra = \"nope\"\n"), "test", "offsite-storj")
	if err == nil {
		t.Fatal("expected unknown-key error")
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseSecrets_ReportsAllUppercaseKeys(t *testing.T) {
	body := "[targets.offsite-storj.keys]\nS3_ID = \"abc\"\nS3_SECRET = \"def\"\nS3_ID = \"duplicate\"\n"
	_, err := ParseSecrets(strings.NewReader(body), "test", "offsite-storj")
	if err == nil {
		t.Fatal("expected uppercase-key error")
	}
	for _, want := range []string{"S3_ID", "S3_SECRET", "lower snake case"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
	if strings.Count(err.Error(), "S3_ID") != 1 {
		t.Fatalf("duplicate key should only be reported once: %v", err)
	}
}

func TestParseSecrets_MalformedTOMLRejected(t *testing.T) {
	_, err := ParseSecrets(strings.NewReader("[targets.offsite-storj.keys]\ns3_id = \"abc\"\ns3_secret = [\n"), "test", "offsite-storj")
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

	p := writeTempSecrets(t, "s3_id = \"x\"\n", 0644)
	if err := ValidateFileAccess(p); err == nil {
		t.Fatal("expected permissions error")
	}
}

func TestValidate(t *testing.T) {
	if err := (*Secrets)(nil).Validate(); err != nil {
		t.Fatalf("nil secrets should validate: %v", err)
	}
	if err := (&Secrets{Keys: map[string]string{"s3_id": validID(), "s3_secret": validSecret()}}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := (&Secrets{}).Validate(); err != nil {
		t.Fatalf("empty key set should be allowed: %v", err)
	}
}

func TestLoadOptionalHealthWebhookToken(t *testing.T) {
	if token, err := LoadOptionalHealthWebhookToken("", "offsite-storj"); err != nil || token != "" {
		t.Fatalf("empty-path token = %q, err = %v", token, err)
	}
	if token, err := LoadOptionalHealthWebhookToken(filepath.Join(t.TempDir(), "missing.toml"), "offsite-storj"); err != nil || token != "" {
		t.Fatalf("missing token = %q, err = %v", token, err)
	}

	p := writeTempSecrets(t, validSecretContentWithTargetValue("offsite-storj", "health_webhook_bearer_token = \"secret-token\""), 0600)
	token, err := LoadOptionalHealthWebhookToken(p, "offsite-storj")
	if isRoot() && err != nil {
		t.Fatalf("LoadOptionalHealthWebhookToken() error = %v", err)
	}
	if isRoot() && token != "secret-token" {
		t.Fatalf("token = %q", token)
	}
}

func TestLoadOptionalHealthNtfyToken(t *testing.T) {
	if token, err := LoadOptionalHealthNtfyToken("", "offsite-storj"); err != nil || token != "" {
		t.Fatalf("empty-path token = %q, err = %v", token, err)
	}
	if token, err := LoadOptionalHealthNtfyToken(filepath.Join(t.TempDir(), "missing.toml"), "offsite-storj"); err != nil || token != "" {
		t.Fatalf("missing token = %q, err = %v", token, err)
	}

	p := writeTempSecrets(t, validSecretContentWithTargetValue("offsite-storj", "health_ntfy_token = \"ntfy-token\""), 0600)
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
	if (*Secrets)(nil).MaskedKeys() != "<none>" {
		t.Fatal("nil masked keys should be explicit")
	}
	if (&Secrets{Keys: map[string]string{"s3_id": "ABCDEFGHIJ"}}).MaskedKeys() != "**** (1 key)" {
		t.Fatal("unexpected masked key summary")
	}
	if (&Secrets{Keys: map[string]string{"s3_id": "ABCDEFGHIJ", "s3_secret": "secret"}}).MaskedKeys() != "**** (2 keys)" {
		t.Fatal("unexpected masked keys summary")
	}
	if (&Secrets{}).MaskedKeys() != "<none>" {
		t.Fatal("empty masked keys should be explicit")
	}
}

func TestParseOptionalTargetToken(t *testing.T) {
	webhookSelector := func(section fileTargetSecrets) *string {
		return section.HealthWebhookBearerToken
	}
	ntfySelector := func(section fileTargetSecrets) *string {
		return section.HealthNtfyToken
	}

	t.Run("returns token when present", func(t *testing.T) {
		token, err := parseOptionalTargetToken("[targets.offsite-storj]\nhealth_webhook_bearer_token = \"secret-token\"\n", "test.toml", "offsite-storj", webhookSelector)
		if err != nil {
			t.Fatalf("parseOptionalTargetToken() error = %v", err)
		}
		if token != "secret-token" {
			t.Fatalf("token = %q", token)
		}
	})

	t.Run("returns empty when target missing", func(t *testing.T) {
		token, err := parseOptionalTargetToken("[targets.archive]\nhealth_webhook_bearer_token = \"secret-token\"\n", "test.toml", "offsite-storj", webhookSelector)
		if err != nil {
			t.Fatalf("parseOptionalTargetToken() error = %v", err)
		}
		if token != "" {
			t.Fatalf("token = %q", token)
		}
	})

	t.Run("returns empty when token missing", func(t *testing.T) {
		token, err := parseOptionalTargetToken(validSecretContent(), "test.toml", "offsite-storj", ntfySelector)
		if err != nil {
			t.Fatalf("parseOptionalTargetToken() error = %v", err)
		}
		if token != "" {
			t.Fatalf("token = %q", token)
		}
	})

	t.Run("rejects uppercase keys", func(t *testing.T) {
		_, err := parseOptionalTargetToken("[targets.offsite-storj]\nHEALTH_NTFY_TOKEN = \"secret-token\"\nHEALTH_WEBHOOK_BEARER_TOKEN = \"secret-token\"\n", "test.toml", "offsite-storj", ntfySelector)
		if err == nil || !strings.Contains(err.Error(), "lower snake case") {
			t.Fatalf("err = %v", err)
		}
		for _, want := range []string{"HEALTH_NTFY_TOKEN", "HEALTH_WEBHOOK_BEARER_TOKEN"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want %q", err, want)
			}
		}
	})

	t.Run("rejects invalid toml", func(t *testing.T) {
		_, err := parseOptionalTargetToken("[targets.offsite-storj]\nhealth_webhook_bearer_token = [\n", "test.toml", "offsite-storj", webhookSelector)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid toml") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("rejects unknown keys", func(t *testing.T) {
		_, err := parseOptionalTargetToken("[targets.offsite-storj]\nextra = \"nope\"\n", "test.toml", "offsite-storj", webhookSelector)
		if err == nil || !strings.Contains(err.Error(), "unexpected key") {
			t.Fatalf("err = %v", err)
		}
	})
}
