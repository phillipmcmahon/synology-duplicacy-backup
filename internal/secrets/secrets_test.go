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

// validSecretContent returns a valid secrets file content string.
func validSecretContent() string {
	return "STORJ_S3_ID=" + validID() + "\nSTORJ_S3_SECRET=" + validSecret() + "\n"
}

// isRoot reports whether the current process is running as root.
func isRoot() bool {
	return os.Getuid() == 0
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

func TestGetSecretsFilePath_DifferentPrefixes(t *testing.T) {
	p1 := GetSecretsFilePath("/root/.secrets", "duplicacy", "homes")
	p2 := GetSecretsFilePath("/root/.secrets", "other", "homes")
	if p1 == p2 {
		t.Error("different prefixes should produce different paths")
	}
}

func TestGetSecretsFilePath_EmptyDir(t *testing.T) {
	path := GetSecretsFilePath("", "duplicacy", "homes")
	if path != "duplicacy-homes.env" {
		t.Errorf("GetSecretsFilePath with empty dir = %q, want %q", path, "duplicacy-homes.env")
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
	p := writeTempSecrets(t, validSecretContent(), 0644)

	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for wrong permissions")
	}
	if !strings.Contains(err.Error(), "0644") {
		t.Errorf("error should mention actual permissions, got: %v", err)
	}
}

func TestLoadSecretsFile_TooPermissive(t *testing.T) {
	p := writeTempSecrets(t, validSecretContent(), 0666)

	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for too-permissive permissions")
	}
	if !strings.Contains(err.Error(), "permissions") {
		t.Errorf("error should mention permissions, got: %v", err)
	}
}

func TestLoadSecretsFile_WorldReadable(t *testing.T) {
	p := writeTempSecrets(t, validSecretContent(), 0604)

	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for world-readable permissions")
	}
}

func TestLoadSecretsFile_GroupReadable(t *testing.T) {
	p := writeTempSecrets(t, validSecretContent(), 0640)

	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error for group-readable permissions")
	}
}

func TestLoadSecretsFile_OwnershipCheck(t *testing.T) {
	p := writeTempSecrets(t, validSecretContent(), 0600)

	if isRoot() {
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

// ---------------------------------------------------------------------------
// ParseSecrets tests – exercise the parser directly via an io.Reader so
// that no root ownership is required.  This replaces the previous
// LoadSecretsFile-based parser tests that were skipped on non-root machines.
// ---------------------------------------------------------------------------

func TestParseSecrets_InvalidFormat_NoEquals(t *testing.T) {
	r := strings.NewReader("INVALID LINE WITHOUT EQUALS\n")
	_, err := ParseSecrets(r, "test")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid line") {
		t.Errorf("error should mention 'invalid line', got: %v", err)
	}
}

func TestParseSecrets_UnknownKey(t *testing.T) {
	r := strings.NewReader("UNKNOWN_KEY=value\n")
	_, err := ParseSecrets(r, "test")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unexpected key") {
		t.Errorf("error should mention 'unexpected key', got: %v", err)
	}
}

func TestParseSecrets_EmptyKey(t *testing.T) {
	r := strings.NewReader("=value\n")
	_, err := ParseSecrets(r, "test")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "missing key") {
		t.Errorf("error should mention 'missing key', got: %v", err)
	}
}

func TestParseSecrets_MissingStorjID(t *testing.T) {
	r := strings.NewReader("STORJ_S3_SECRET=" + validSecret() + "\n")
	_, err := ParseSecrets(r, "test")
	if err == nil {
		t.Fatal("expected error for missing STORJ_S3_ID")
	}
	if !strings.Contains(err.Error(), "STORJ_S3_ID") {
		t.Errorf("error should mention STORJ_S3_ID, got: %v", err)
	}
}

func TestParseSecrets_MissingStorjSecret(t *testing.T) {
	r := strings.NewReader("STORJ_S3_ID=" + validID() + "\n")
	_, err := ParseSecrets(r, "test")
	if err == nil {
		t.Fatal("expected error for missing STORJ_S3_SECRET")
	}
	if !strings.Contains(err.Error(), "STORJ_S3_SECRET") {
		t.Errorf("error should mention STORJ_S3_SECRET, got: %v", err)
	}
}

func TestParseSecrets_CommentsAndBlankLines(t *testing.T) {
	content := "# This is a comment\n\nSTORJ_S3_ID=" + validID() + "\n# Another comment\nSTORJ_S3_SECRET=" + validSecret() + "\n"
	sec, err := ParseSecrets(strings.NewReader(content), "test")
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

func TestParseSecrets_QuoteStripping(t *testing.T) {
	content := "STORJ_S3_ID=\"" + validID() + "\"\nSTORJ_S3_SECRET=\"" + validSecret() + "\"\n"
	sec, err := ParseSecrets(strings.NewReader(content), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec.StorjS3ID != validID() {
		t.Errorf("StorjS3ID = %q, want quotes stripped", sec.StorjS3ID)
	}
}

func TestParseSecrets_EmptyInput(t *testing.T) {
	_, err := ParseSecrets(strings.NewReader(""), "test")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if !strings.Contains(err.Error(), "STORJ_S3_ID") {
		t.Errorf("error should mention missing key, got: %v", err)
	}
}

func TestParseSecrets_WhitespaceHandling(t *testing.T) {
	content := "  STORJ_S3_ID = " + validID() + "  \n  STORJ_S3_SECRET = " + validSecret() + "  \n"
	sec, err := ParseSecrets(strings.NewReader(content), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec.StorjS3ID != validID() {
		t.Errorf("StorjS3ID = %q, want trimmed value", sec.StorjS3ID)
	}
}

func TestParseSecrets_PartialQuotes(t *testing.T) {
	// Only opening quote - should NOT be stripped
	content := "STORJ_S3_ID=\"" + validID() + "\nSTORJ_S3_SECRET=" + validSecret() + "\n"
	sec, err := ParseSecrets(strings.NewReader(content), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Partial quote should remain
	if !strings.HasPrefix(sec.StorjS3ID, "\"") {
		t.Errorf("partial opening quote should not be stripped, got: %q", sec.StorjS3ID)
	}
}

func TestParseSecrets_ValidContent(t *testing.T) {
	sec, err := ParseSecrets(strings.NewReader(validSecretContent()), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec.StorjS3ID != validID() {
		t.Errorf("StorjS3ID = %q, want %q", sec.StorjS3ID, validID())
	}
	if sec.StorjS3Secret != validSecret() {
		t.Errorf("StorjS3Secret = %q, want %q", sec.StorjS3Secret, validSecret())
	}
}

func TestParseSecrets_DuplicateKey(t *testing.T) {
	// Second value should win
	content := "STORJ_S3_ID=" + validID() + "\nSTORJ_S3_ID=NEWVALUE_FOR_DUPLICATE_TEST_XX\nSTORJ_S3_SECRET=" + validSecret() + "\n"
	sec, err := ParseSecrets(strings.NewReader(content), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec.StorjS3ID != "NEWVALUE_FOR_DUPLICATE_TEST_XX" {
		t.Errorf("StorjS3ID = %q, want last value", sec.StorjS3ID)
	}
}

func TestParseSecrets_SourceInErrorMessage(t *testing.T) {
	_, err := ParseSecrets(strings.NewReader(""), "my-secrets.env")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "my-secrets.env") {
		t.Errorf("error should include source name, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ValidateFileAccess tests
// ---------------------------------------------------------------------------

func TestValidateFileAccess_MissingFile(t *testing.T) {
	err := ValidateFileAccess("/nonexistent/secrets.env")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestValidateFileAccess_WrongPermissions(t *testing.T) {
	p := writeTempSecrets(t, "data", 0644)
	err := ValidateFileAccess(p)
	if err == nil {
		t.Fatal("expected error for wrong permissions")
	}
	if !strings.Contains(err.Error(), "0644") {
		t.Errorf("error should mention actual permissions, got: %v", err)
	}
}

func TestValidateFileAccess_OwnershipCheck(t *testing.T) {
	p := writeTempSecrets(t, "data", 0600)
	err := ValidateFileAccess(p)
	if isRoot() {
		if err != nil {
			t.Fatalf("unexpected error as root: %v", err)
		}
	} else {
		if err == nil {
			t.Fatal("expected error for non-root ownership")
		}
		if !strings.Contains(err.Error(), "ownership") {
			t.Errorf("error should mention ownership, got: %v", err)
		}
	}
}

func TestLoadSecretsFile_StatError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nonexistent.env")
	_, err := LoadSecretsFile(p)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
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

func TestValidate_SecretOneBelowMin(t *testing.T) {
	id := strings.Repeat("a", MinStorjS3IDLen)
	secret := strings.Repeat("b", MinStorjS3SecretLen-1)
	s := &Secrets{StorjS3ID: id, StorjS3Secret: secret}
	if err := s.Validate(); err == nil {
		t.Fatal("one below min secret length should fail")
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

func TestMaskedID_ThreeChars(t *testing.T) {
	s := &Secrets{StorjS3ID: "ABC"}
	masked := s.MaskedID()
	if masked != "****" {
		t.Errorf("MaskedID = %q, want ****", masked)
	}
}

func TestMaskedID_FiveChars(t *testing.T) {
	s := &Secrets{StorjS3ID: "ABCDE"}
	masked := s.MaskedID()
	if masked != "****BCDE" {
		t.Errorf("MaskedID = %q, want ****BCDE", masked)
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

func TestMaskedSecret_ExactlyFourChars(t *testing.T) {
	s := &Secrets{StorjS3Secret: "ABCD"}
	masked := s.MaskedSecret()
	if masked != "****ABCD" {
		t.Errorf("MaskedSecret = %q, want ****ABCD", masked)
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

func TestAllowedSecretKeys_ExactCount(t *testing.T) {
	if len(allowedSecretKeys) != 2 {
		t.Errorf("expected exactly 2 allowed keys, got %d", len(allowedSecretKeys))
	}
}
