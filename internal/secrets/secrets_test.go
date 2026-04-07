package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempSecrets creates a temp secrets file with given content and perms.
func writeTempSecrets(t *testing.T, content string, perm os.FileMode) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test.env")
	if err := os.WriteFile(p, []byte(content), perm); err != nil {
		t.Fatalf("failed to write temp secrets: %v", err)
	}
	return p
}

// validID returns a string of at least MinStorjS3IDLen characters.
func validID() string {
	return "ABCDEFGHIJKLMNOPQRSTUVWXYZ01" // 28 chars
}

// validSecret returns a string of at least MinStorjS3SecretLen characters.
func validSecret() string {
	return "abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR" // 55 chars
}

// ─── GetSecretsFilePath tests ───────────────────────────────────────────────

func TestGetSecretsFilePath(t *testing.T) {
	path := GetSecretsFilePath("/root/.secrets", "duplicacy", "homes")
	expected := "/root/.secrets/duplicacy-homes.env"
	if path != expected {
		t.Errorf("GetSecretsFilePath = %q, want %q", path, expected)
	}
}

func TestGetSecretsFilePath_DifferentLabels(t *testing.T) {
	p1 := GetSecretsFilePath("/root/.secrets", "duplicacy", "homes")
	p2 := GetSecretsFilePath("/root/.secrets", "duplicacy", "photos")
	if p1 == p2 {
		t.Error("different labels should produce different paths")
	}
}

// ─── LoadSecretsFile tests ──────────────────────────────────────────────────

func TestLoadSecretsFile_MissingFile(t *testing.T) {
	_, err := LoadSecretsFile("/nonexistent/secrets.env")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestLoadSecretsFile_WrongPermissions(t *testing.T) {
	content := "STORJ_S3_ID=" + validID() + "\nSTORJ_S3_SECRET=" + validSecret() + "\n"
	p := writeTempSecrets(t, content, 0644)

	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for wrong permissions")
	}
	if !strings.Contains(err.Error(), "0644") {
		t.Errorf("error should mention actual permissions, got: %v", err)
	}
}

func TestLoadSecretsFile_TooPermissive(t *testing.T) {
	content := "STORJ_S3_ID=" + validID() + "\nSTORJ_S3_SECRET=" + validSecret() + "\n"
	p := writeTempSecrets(t, content, 0666)

	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for too-permissive permissions")
	}
	if !strings.Contains(err.Error(), "permissions") {
		t.Errorf("error should mention permissions, got: %v", err)
	}
}

func TestLoadSecretsFile_OwnershipCheck(t *testing.T) {
	// This test verifies the ownership check path.
	// When running as non-root (uid != 0), the check should fail.
	content := "STORJ_S3_ID=" + validID() + "\nSTORJ_S3_SECRET=" + validSecret() + "\n"
	p := writeTempSecrets(t, content, 0600)

	if os.Getuid() == 0 {
		// Running as root - ownership check passes, file should load successfully
		sec, err := LoadSecretsFile(p)
		if err != nil {
			t.Fatalf("unexpected error as root: %v", err)
		}
		if sec.StorjS3ID != validID() {
			t.Errorf("StorjS3ID = %q", sec.StorjS3ID)
		}
	} else {
		// Running as non-root - ownership check should fail
		_, err := LoadSecretsFile(p)
		if err == nil {
			t.Fatal("expected error for non-root ownership")
		}
		if !strings.Contains(err.Error(), "ownership") {
			t.Errorf("error should mention ownership, got: %v", err)
		}
	}
}

func TestLoadSecretsFile_InvalidFormat_NoEquals(t *testing.T) {
	// We need to test the parsing logic, but we can't easily bypass the
	// ownership check without root. We'll test the parsing functions
	// indirectly through the Validate and Masked methods, and test the
	// parsing error paths that we can reach.
	if os.Getuid() != 0 {
		t.Skip("Skipping: requires root for ownership check")
	}

	p := writeTempSecrets(t, "INVALID LINE WITHOUT EQUALS\n", 0600)
	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid line") {
		t.Errorf("error should mention 'invalid line', got: %v", err)
	}
}

func TestLoadSecretsFile_UnknownKey(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping: requires root for ownership check")
	}

	p := writeTempSecrets(t, "UNKNOWN_KEY=value\n", 0600)
	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unexpected key") {
		t.Errorf("error should mention 'unexpected key', got: %v", err)
	}
}

func TestLoadSecretsFile_EmptyKey(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping: requires root for ownership check")
	}

	p := writeTempSecrets(t, "=value\n", 0600)
	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "missing key") {
		t.Errorf("error should mention 'missing key', got: %v", err)
	}
}

func TestLoadSecretsFile_MissingStorjID(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping: requires root for ownership check")
	}

	p := writeTempSecrets(t, "STORJ_S3_SECRET="+validSecret()+"\n", 0600)
	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for missing STORJ_S3_ID")
	}
	if !strings.Contains(err.Error(), "STORJ_S3_ID") {
		t.Errorf("error should mention STORJ_S3_ID, got: %v", err)
	}
}

func TestLoadSecretsFile_MissingStorjSecret(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping: requires root for ownership check")
	}

	p := writeTempSecrets(t, "STORJ_S3_ID="+validID()+"\n", 0600)
	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for missing STORJ_S3_SECRET")
	}
	if !strings.Contains(err.Error(), "STORJ_S3_SECRET") {
		t.Errorf("error should mention STORJ_S3_SECRET, got: %v", err)
	}
}

func TestLoadSecretsFile_CommentsAndBlankLines(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping: requires root for ownership check")
	}

	content := "# This is a comment\n\nSTORJ_S3_ID=" + validID() + "\n# Another comment\nSTORJ_S3_SECRET=" + validSecret() + "\n"
	p := writeTempSecrets(t, content, 0600)

	sec, err := LoadSecretsFile(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec.StorjS3ID != validID() {
		t.Errorf("StorjS3ID = %q", sec.StorjS3ID)
	}
	if sec.StorjS3Secret != validSecret() {
		t.Errorf("StorjS3Secret = %q", sec.StorjS3Secret)
	}
}

func TestLoadSecretsFile_QuoteStripping(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping: requires root for ownership check")
	}

	content := "STORJ_S3_ID=\"" + validID() + "\"\nSTORJ_S3_SECRET=\"" + validSecret() + "\"\n"
	p := writeTempSecrets(t, content, 0600)

	sec, err := LoadSecretsFile(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec.StorjS3ID != validID() {
		t.Errorf("StorjS3ID = %q, want quotes stripped", sec.StorjS3ID)
	}
}

// ─── Validate tests ─────────────────────────────────────────────────────────

func TestValidate_Valid(t *testing.T) {
	s := &Secrets{StorjS3ID: validID(), StorjS3Secret: validSecret()}
	if err := s.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_IDTooShort(t *testing.T) {
	s := &Secrets{StorjS3ID: "short", StorjS3Secret: validSecret()}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for short ID")
	}
	if !strings.Contains(err.Error(), "STORJ_S3_ID") {
		t.Errorf("error should mention STORJ_S3_ID, got: %v", err)
	}
}

func TestValidate_SecretTooShort(t *testing.T) {
	s := &Secrets{StorjS3ID: validID(), StorjS3Secret: "short"}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for short secret")
	}
	if !strings.Contains(err.Error(), "STORJ_S3_SECRET") {
		t.Errorf("error should mention STORJ_S3_SECRET, got: %v", err)
	}
}

func TestValidate_BothTooShort(t *testing.T) {
	s := &Secrets{StorjS3ID: "x", StorjS3Secret: "y"}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for both too short")
	}
	// Should fail on first check (ID)
	if !strings.Contains(err.Error(), "STORJ_S3_ID") {
		t.Errorf("error should mention STORJ_S3_ID, got: %v", err)
	}
}

func TestValidate_ExactMinLength(t *testing.T) {
	id := strings.Repeat("a", MinStorjS3IDLen)
	secret := strings.Repeat("b", MinStorjS3SecretLen)
	s := &Secrets{StorjS3ID: id, StorjS3Secret: secret}
	if err := s.Validate(); err != nil {
		t.Errorf("exact min length should be valid: %v", err)
	}
}

func TestValidate_OneBelowMinLength(t *testing.T) {
	id := strings.Repeat("a", MinStorjS3IDLen-1)
	secret := strings.Repeat("b", MinStorjS3SecretLen)
	s := &Secrets{StorjS3ID: id, StorjS3Secret: secret}
	if err := s.Validate(); err == nil {
		t.Fatal("one below min ID length should fail")
	}
}

func TestValidate_EmptyStrings(t *testing.T) {
	s := &Secrets{StorjS3ID: "", StorjS3Secret: ""}
	if err := s.Validate(); err == nil {
		t.Fatal("empty strings should fail validation")
	}
}

// ─── MaskedID tests ─────────────────────────────────────────────────────────

func TestMaskedID_Normal(t *testing.T) {
	s := &Secrets{StorjS3ID: "ABCDEFGHIJ"}
	masked := s.MaskedID()
	if masked != "****GHIJ" {
		t.Errorf("MaskedID = %q, want ****GHIJ", masked)
	}
}

func TestMaskedID_ExactlyFourChars(t *testing.T) {
	s := &Secrets{StorjS3ID: "ABCD"}
	masked := s.MaskedID()
	if masked != "****ABCD" {
		t.Errorf("MaskedID = %q, want ****ABCD", masked)
	}
}

func TestMaskedID_TooShort(t *testing.T) {
	s := &Secrets{StorjS3ID: "AB"}
	masked := s.MaskedID()
	if masked != "****" {
		t.Errorf("MaskedID = %q, want ****", masked)
	}
}

func TestMaskedID_Empty(t *testing.T) {
	s := &Secrets{StorjS3ID: ""}
	masked := s.MaskedID()
	if masked != "****" {
		t.Errorf("MaskedID = %q, want ****", masked)
	}
}

// ─── MaskedSecret tests ────────────────────────────────────────────────────

func TestMaskedSecret_Normal(t *testing.T) {
	s := &Secrets{StorjS3Secret: "ABCDEFGHIJ"}
	masked := s.MaskedSecret()
	if masked != "****GHIJ" {
		t.Errorf("MaskedSecret = %q, want ****GHIJ", masked)
	}
}

func TestMaskedSecret_TooShort(t *testing.T) {
	s := &Secrets{StorjS3Secret: "AB"}
	masked := s.MaskedSecret()
	if masked != "****" {
		t.Errorf("MaskedSecret = %q, want ****", masked)
	}
}

func TestMaskedSecret_Empty(t *testing.T) {
	s := &Secrets{StorjS3Secret: ""}
	masked := s.MaskedSecret()
	if masked != "****" {
		t.Errorf("MaskedSecret = %q, want ****", masked)
	}
}

// ─── allowedSecretKeys tests ────────────────────────────────────────────────

func TestAllowedSecretKeys(t *testing.T) {
	if !allowedSecretKeys["STORJ_S3_ID"] {
		t.Error("STORJ_S3_ID should be allowed")
	}
	if !allowedSecretKeys["STORJ_S3_SECRET"] {
		t.Error("STORJ_S3_SECRET should be allowed")
	}
	if allowedSecretKeys["RANDOM_KEY"] {
		t.Error("RANDOM_KEY should not be allowed")
	}
}
