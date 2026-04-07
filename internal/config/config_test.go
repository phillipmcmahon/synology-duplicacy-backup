package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper to write a temp config file and return its path.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test.conf")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return p
}

// ─── ParseFile tests ─────────────────────────────────────────────────────────

func TestParseFile_ValidConfig(t *testing.T) {
	p := writeTempConfig(t, `[common]
DESTINATION=/volume1/backups
FILTER="-e \.DS_Store"
[local]
THREADS=4
LOCAL_OWNER=admin
LOCAL_GROUP=users
`)
	vals, err := ParseFile(p, "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expect := map[string]string{
		"DESTINATION": "/volume1/backups",
		"FILTER":      `-e \.DS_Store`,
		"THREADS":     "4",
		"LOCAL_OWNER": "admin",
		"LOCAL_GROUP": "users",
	}
	for k, want := range expect {
		if got := vals[k]; got != want {
			t.Errorf("vals[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestParseFile_TargetSectionOverridesCommon(t *testing.T) {
	p := writeTempConfig(t, `[common]
DESTINATION=/volume1/backups
THREADS=2
[local]
THREADS=8
`)
	vals, err := ParseFile(p, "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals["THREADS"] != "8" {
		t.Errorf("expected THREADS=8 (local override), got %q", vals["THREADS"])
	}
}

func TestParseFile_IgnoresUnrelatedSections(t *testing.T) {
	p := writeTempConfig(t, `[common]
DESTINATION=/volume1/backups
[remote]
THREADS=2
[local]
THREADS=8
`)
	vals, err := ParseFile(p, "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// THREADS should be 8 from [local], not 2 from [remote]
	if vals["THREADS"] != "8" {
		t.Errorf("expected THREADS=8, got %q", vals["THREADS"])
	}
}

func TestParseFile_CommentsAndBlankLinesIgnored(t *testing.T) {
	p := writeTempConfig(t, `
# This is a comment
[common]
# Another comment
DESTINATION=/volume1/backups

[local]
THREADS=4
`)
	vals, err := ParseFile(p, "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals["DESTINATION"] != "/volume1/backups" {
		t.Errorf("DESTINATION = %q", vals["DESTINATION"])
	}
}

func TestParseFile_QuoteStripping(t *testing.T) {
	p := writeTempConfig(t, `[common]
DESTINATION="/volume1/backups"
[local]
THREADS=4
`)
	vals, err := ParseFile(p, "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals["DESTINATION"] != "/volume1/backups" {
		t.Errorf("expected quotes stripped, got %q", vals["DESTINATION"])
	}
}

func TestParseFile_MissingCommonSection(t *testing.T) {
	p := writeTempConfig(t, `[local]
DESTINATION=/volume1/backups
`)
	_, err := ParseFile(p, "local")
	if err == nil {
		t.Fatal("expected error for missing [common] section, got nil")
	}
	if !strings.Contains(err.Error(), "[common]") {
		t.Errorf("error should mention [common], got: %v", err)
	}
}

func TestParseFile_MissingTargetSection_Local(t *testing.T) {
	p := writeTempConfig(t, `[common]
DESTINATION=/volume1/backups
`)
	_, err := ParseFile(p, "local")
	if err == nil {
		t.Fatal("expected error for missing [local] section, got nil")
	}
	if !strings.Contains(err.Error(), "[local]") {
		t.Errorf("error should mention [local], got: %v", err)
	}
}

func TestParseFile_MissingTargetSection_Remote(t *testing.T) {
	p := writeTempConfig(t, `[common]
DESTINATION=s3://bucket
[local]
THREADS=4
`)
	_, err := ParseFile(p, "remote")
	if err == nil {
		t.Fatal("expected error for missing [remote] section, got nil")
	}
	if !strings.Contains(err.Error(), "[remote]") {
		t.Errorf("error should mention [remote], got: %v", err)
	}
}

func TestParseFile_DuplicateSection(t *testing.T) {
	p := writeTempConfig(t, `[common]
DESTINATION=/volume1/backups
[local]
THREADS=4
[local]
THREADS=8
`)
	_, err := ParseFile(p, "local")
	if err == nil {
		t.Fatal("expected error for duplicate section")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate, got: %v", err)
	}
}

func TestParseFile_ContentOutsideSection(t *testing.T) {
	p := writeTempConfig(t, `DESTINATION=/volume1/backups
[common]
THREADS=4
[local]
THREADS=2
`)
	_, err := ParseFile(p, "local")
	if err == nil {
		t.Fatal("expected error for content outside section")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Errorf("error should mention 'outside', got: %v", err)
	}
}

func TestParseFile_InvalidLineNoEquals(t *testing.T) {
	p := writeTempConfig(t, `[common]
DESTINATION /volume1/backups
[local]
THREADS=4
`)
	_, err := ParseFile(p, "local")
	if err == nil {
		t.Fatal("expected error for invalid line")
	}
	if !strings.Contains(err.Error(), "key=value") {
		t.Errorf("error should mention key=value, got: %v", err)
	}
}

func TestParseFile_EmptyKey(t *testing.T) {
	p := writeTempConfig(t, `[common]
=/volume1/backups
[local]
THREADS=4
`)
	_, err := ParseFile(p, "local")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "missing key") {
		t.Errorf("error should mention 'missing key', got: %v", err)
	}
}

func TestParseFile_UnknownKey(t *testing.T) {
	p := writeTempConfig(t, `[common]
DESTINATION=/volume1/backups
UNKNOWN_KEY=foo
[local]
THREADS=4
`)
	_, err := ParseFile(p, "local")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "not permitted") {
		t.Errorf("error should mention 'not permitted', got: %v", err)
	}
}

func TestParseFile_InvalidKeyPattern(t *testing.T) {
	p := writeTempConfig(t, `[common]
lower_case=value
[local]
THREADS=4
`)
	_, err := ParseFile(p, "local")
	if err == nil {
		t.Fatal("expected error for invalid key pattern")
	}
	if !strings.Contains(err.Error(), "invalid config key") {
		t.Errorf("error should mention 'invalid config key', got: %v", err)
	}
}

func TestParseFile_NonexistentFile(t *testing.T) {
	_, err := ParseFile("/nonexistent/config.conf", "local")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// ─── Apply tests ─────────────────────────────────────────────────────────────

func TestApply_ValidNumericValues(t *testing.T) {
	cfg := NewDefaults()
	vals := map[string]string{
		"THREADS":                          "8",
		"LOG_RETENTION_DAYS":               "14",
		"SAFE_PRUNE_MAX_DELETE_COUNT":      "50",
		"SAFE_PRUNE_MAX_DELETE_PERCENT":    "20",
		"SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT": "10",
	}
	if err := cfg.Apply(vals); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Threads != 8 {
		t.Errorf("Threads = %d, want 8", cfg.Threads)
	}
	if cfg.LogRetentionDays != 14 {
		t.Errorf("LogRetentionDays = %d, want 14", cfg.LogRetentionDays)
	}
	if cfg.SafePruneMaxDeleteCount != 50 {
		t.Errorf("SafePruneMaxDeleteCount = %d, want 50", cfg.SafePruneMaxDeleteCount)
	}
	if cfg.SafePruneMaxDeletePercent != 20 {
		t.Errorf("SafePruneMaxDeletePercent = %d, want 20", cfg.SafePruneMaxDeletePercent)
	}
	if cfg.SafePruneMinTotalForPercent != 10 {
		t.Errorf("SafePruneMinTotalForPercent = %d, want 10", cfg.SafePruneMinTotalForPercent)
	}
}

func TestApply_StringValues(t *testing.T) {
	cfg := NewDefaults()
	vals := map[string]string{
		"DESTINATION": "/volume1/backups",
		"FILTER":      "-e *.tmp",
		"LOCAL_OWNER": "admin",
		"LOCAL_GROUP": "staff",
		"PRUNE":       "-keep 0:365 -keep 30:180",
	}
	if err := cfg.Apply(vals); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Destination != "/volume1/backups" {
		t.Errorf("Destination = %q", cfg.Destination)
	}
	if cfg.Filter != "-e *.tmp" {
		t.Errorf("Filter = %q", cfg.Filter)
	}
	if cfg.LocalOwner != "admin" {
		t.Errorf("LocalOwner = %q", cfg.LocalOwner)
	}
	if cfg.LocalGroup != "staff" {
		t.Errorf("LocalGroup = %q", cfg.LocalGroup)
	}
	if cfg.Prune != "-keep 0:365 -keep 30:180" {
		t.Errorf("Prune = %q", cfg.Prune)
	}
}

func TestApply_EmptyMapKeepsDefaults(t *testing.T) {
	cfg := NewDefaults()
	if err := cfg.Apply(map[string]string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LocalOwner != DefaultLocalOwner {
		t.Errorf("LocalOwner = %q, want default %q", cfg.LocalOwner, DefaultLocalOwner)
	}
	if cfg.LogRetentionDays != DefaultLogRetentionDays {
		t.Errorf("LogRetentionDays = %d, want default %d", cfg.LogRetentionDays, DefaultLogRetentionDays)
	}
}

func TestApply_InvalidThreadsNotANumber(t *testing.T) {
	cfg := NewDefaults()
	err := cfg.Apply(map[string]string{"THREADS": "ten"})
	if err == nil {
		t.Fatal("expected error for THREADS=ten, got nil")
	}
	if !strings.Contains(err.Error(), "THREADS") {
		t.Errorf("error should mention THREADS, got: %v", err)
	}
}

func TestApply_NegativeLogRetention(t *testing.T) {
	cfg := NewDefaults()
	err := cfg.Apply(map[string]string{"LOG_RETENTION_DAYS": "-5"})
	if err == nil {
		t.Fatal("expected error for negative LOG_RETENTION_DAYS, got nil")
	}
	if !strings.Contains(err.Error(), "LOG_RETENTION_DAYS") {
		t.Errorf("error should mention LOG_RETENTION_DAYS, got: %v", err)
	}
}

func TestApply_InvalidPruneDeleteCount(t *testing.T) {
	cfg := NewDefaults()
	err := cfg.Apply(map[string]string{"SAFE_PRUNE_MAX_DELETE_COUNT": "abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric SAFE_PRUNE_MAX_DELETE_COUNT, got nil")
	}
	if !strings.Contains(err.Error(), "SAFE_PRUNE_MAX_DELETE_COUNT") {
		t.Errorf("error should mention SAFE_PRUNE_MAX_DELETE_COUNT, got: %v", err)
	}
}

func TestApply_InvalidPruneDeletePercent(t *testing.T) {
	cfg := NewDefaults()
	err := cfg.Apply(map[string]string{"SAFE_PRUNE_MAX_DELETE_PERCENT": "50%"})
	if err == nil {
		t.Fatal("expected error for SAFE_PRUNE_MAX_DELETE_PERCENT=50%, got nil")
	}
}

func TestApply_InvalidMinTotalForPercent(t *testing.T) {
	cfg := NewDefaults()
	err := cfg.Apply(map[string]string{"SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT": "nope"})
	if err == nil {
		t.Fatal("expected error for invalid SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT")
	}
}

func TestApply_EmptyNumericValue(t *testing.T) {
	cfg := NewDefaults()
	err := cfg.Apply(map[string]string{"THREADS": ""})
	if err == nil {
		t.Fatal("expected error for empty THREADS value, got nil")
	}
}

// ─── NewDefaults tests ───────────────────────────────────────────────────────

func TestNewDefaults(t *testing.T) {
	cfg := NewDefaults()
	if cfg.LocalOwner != DefaultLocalOwner {
		t.Errorf("LocalOwner = %q, want %q", cfg.LocalOwner, DefaultLocalOwner)
	}
	if cfg.LocalGroup != DefaultLocalGroup {
		t.Errorf("LocalGroup = %q, want %q", cfg.LocalGroup, DefaultLocalGroup)
	}
	if cfg.LogRetentionDays != DefaultLogRetentionDays {
		t.Errorf("LogRetentionDays = %d, want %d", cfg.LogRetentionDays, DefaultLogRetentionDays)
	}
	if cfg.SafePruneMaxDeletePercent != DefaultSafePruneMaxDeletePercent {
		t.Errorf("SafePruneMaxDeletePercent = %d, want %d", cfg.SafePruneMaxDeletePercent, DefaultSafePruneMaxDeletePercent)
	}
	if cfg.SafePruneMaxDeleteCount != DefaultSafePruneMaxDeleteCount {
		t.Errorf("SafePruneMaxDeleteCount = %d, want %d", cfg.SafePruneMaxDeleteCount, DefaultSafePruneMaxDeleteCount)
	}
	if cfg.SafePruneMinTotalForPercent != DefaultSafePruneMinTotalForPercent {
		t.Errorf("SafePruneMinTotalForPercent = %d, want %d", cfg.SafePruneMinTotalForPercent, DefaultSafePruneMinTotalForPercent)
	}
}

// ─── ValidateRequired tests ──────────────────────────────────────────────────

func TestValidateRequired_AllPresent(t *testing.T) {
	cfg := &Config{Destination: "/vol", Threads: 4, Prune: "-keep 0:30"}
	if err := cfg.ValidateRequired(true, true); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRequired_MissingDestination(t *testing.T) {
	cfg := &Config{Threads: 4, Prune: "-keep 0:30"}
	err := cfg.ValidateRequired(true, true)
	if err == nil {
		t.Fatal("expected error for missing DESTINATION")
	}
	if !strings.Contains(err.Error(), "DESTINATION") {
		t.Errorf("error should mention DESTINATION, got: %v", err)
	}
}

func TestValidateRequired_MissingThreadsOnlyWhenBackup(t *testing.T) {
	cfg := &Config{Destination: "/vol", Prune: "-keep 0:30"}
	// No backup → threads not required
	if err := cfg.ValidateRequired(false, true); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// With backup → threads required
	err := cfg.ValidateRequired(true, true)
	if err == nil {
		t.Fatal("expected error for missing THREADS with backup")
	}
	if !strings.Contains(err.Error(), "THREADS") {
		t.Errorf("error should mention THREADS, got: %v", err)
	}
}

func TestValidateRequired_MissingPruneOnlyWhenPrune(t *testing.T) {
	cfg := &Config{Destination: "/vol", Threads: 4}
	if err := cfg.ValidateRequired(true, false); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	err := cfg.ValidateRequired(true, true)
	if err == nil {
		t.Fatal("expected error for missing PRUNE with prune enabled")
	}
	if !strings.Contains(err.Error(), "PRUNE") {
		t.Errorf("error should mention PRUNE, got: %v", err)
	}
}

// ─── ValidateThresholds tests ────────────────────────────────────────────────

func TestValidateThresholds_Valid(t *testing.T) {
	cfg := NewDefaults()
	if err := cfg.ValidateThresholds(); err != nil {
		t.Errorf("unexpected error with defaults: %v", err)
	}
}

func TestValidateThresholds_PercentTooHigh(t *testing.T) {
	cfg := NewDefaults()
	cfg.SafePruneMaxDeletePercent = 101
	if err := cfg.ValidateThresholds(); err == nil {
		t.Fatal("expected error for percent > 100")
	}
}

func TestValidateThresholds_NegativeCount(t *testing.T) {
	cfg := NewDefaults()
	cfg.SafePruneMaxDeleteCount = -1
	if err := cfg.ValidateThresholds(); err == nil {
		t.Fatal("expected error for negative count")
	}
}

func TestValidateThresholds_NegativeMinTotal(t *testing.T) {
	cfg := NewDefaults()
	cfg.SafePruneMinTotalForPercent = -1
	if err := cfg.ValidateThresholds(); err == nil {
		t.Fatal("expected error for negative min total")
	}
}

func TestValidateThresholds_NegativeLogRetention(t *testing.T) {
	cfg := NewDefaults()
	cfg.LogRetentionDays = -1
	if err := cfg.ValidateThresholds(); err == nil {
		t.Fatal("expected error for negative log retention")
	}
}

func TestValidateThresholds_ZeroValues(t *testing.T) {
	cfg := &Config{}
	if err := cfg.ValidateThresholds(); err != nil {
		t.Errorf("zero values should be valid: %v", err)
	}
}

// ─── ValidateOwnerGroup tests ────────────────────────────────────────────────

func TestValidateOwnerGroup_Valid(t *testing.T) {
	cfg := &Config{LocalOwner: "admin", LocalGroup: "users"}
	if err := cfg.ValidateOwnerGroup(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateOwnerGroup_InvalidOwner(t *testing.T) {
	cfg := &Config{LocalOwner: "admin/bad", LocalGroup: "users"}
	if err := cfg.ValidateOwnerGroup(); err == nil {
		t.Fatal("expected error for invalid owner")
	}
}

func TestValidateOwnerGroup_InvalidGroup(t *testing.T) {
	cfg := &Config{LocalOwner: "admin", LocalGroup: "us ers"}
	if err := cfg.ValidateOwnerGroup(); err == nil {
		t.Fatal("expected error for invalid group")
	}
}

func TestValidateOwnerGroup_EmptyOwner(t *testing.T) {
	cfg := &Config{LocalOwner: "", LocalGroup: "users"}
	if err := cfg.ValidateOwnerGroup(); err == nil {
		t.Fatal("expected error for empty owner")
	}
}

// ─── ValidateThreads tests ──────────────────────────────────────────────────

func TestValidateThreads_Valid(t *testing.T) {
	for _, n := range []int{1, 2, 4, 8, 16} {
		cfg := &Config{Threads: n}
		if err := cfg.ValidateThreads(); err != nil {
			t.Errorf("ValidateThreads(%d) unexpected error: %v", n, err)
		}
	}
}

func TestValidateThreads_Invalid(t *testing.T) {
	for _, n := range []int{0, -1, 3, 5, 6, 7, 9, 17, 32} {
		cfg := &Config{Threads: n}
		if err := cfg.ValidateThreads(); err == nil {
			t.Errorf("ValidateThreads(%d) expected error, got nil", n)
		}
	}
}

// ─── BuildPruneArgs tests ───────────────────────────────────────────────────

func TestBuildPruneArgs_NonEmpty(t *testing.T) {
	cfg := &Config{Prune: "-keep 0:365 -keep 30:180"}
	cfg.BuildPruneArgs()
	expected := []string{"-keep", "0:365", "-keep", "30:180"}
	if len(cfg.PruneArgs) != len(expected) {
		t.Fatalf("PruneArgs len = %d, want %d", len(cfg.PruneArgs), len(expected))
	}
	for i, a := range cfg.PruneArgs {
		if a != expected[i] {
			t.Errorf("PruneArgs[%d] = %q, want %q", i, a, expected[i])
		}
	}
}

func TestBuildPruneArgs_Empty(t *testing.T) {
	cfg := &Config{Prune: ""}
	cfg.BuildPruneArgs()
	if cfg.PruneArgs != nil {
		t.Errorf("PruneArgs = %v, want nil", cfg.PruneArgs)
	}
}

// ─── strictAtoi tests ───────────────────────────────────────────────────────

func TestStrictAtoi_Valid(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"100", 100},
		{"9999", 9999},
	}
	for _, tt := range tests {
		n, err := strictAtoi(tt.input)
		if err != nil {
			t.Errorf("strictAtoi(%q) unexpected error: %v", tt.input, err)
		}
		if n != tt.expected {
			t.Errorf("strictAtoi(%q) = %d, want %d", tt.input, n, tt.expected)
		}
	}
}

func TestStrictAtoi_Invalid(t *testing.T) {
	invalids := []string{
		"", "abc", "ten", "-1", "-100", "3.14", "12abc", "1 2", " 5", "5 ",
	}
	for _, s := range invalids {
		_, err := strictAtoi(s)
		if err == nil {
			t.Errorf("strictAtoi(%q) expected error, got nil", s)
		}
	}
}

func TestStrictAtoi_LeadingZeros(t *testing.T) {
	// Leading zeros should be accepted (parsed as decimal)
	n, err := strictAtoi("007")
	if err != nil {
		t.Fatalf("strictAtoi(\"007\") unexpected error: %v", err)
	}
	if n != 7 {
		t.Errorf("strictAtoi(\"007\") = %d, want 7", n)
	}
}
