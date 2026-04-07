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

// --- Tests for ParseFile: missing required sections (P2 fix) ---

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

func TestParseFile_BothSectionsPresent(t *testing.T) {
	p := writeTempConfig(t, `[common]
DESTINATION=/volume1/backups
[local]
THREADS=4
`)
	vals, err := ParseFile(p, "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals["DESTINATION"] != "/volume1/backups" {
		t.Errorf("DESTINATION = %q, want /volume1/backups", vals["DESTINATION"])
	}
	if vals["THREADS"] != "4" {
		t.Errorf("THREADS = %q, want 4", vals["THREADS"])
	}
}

// --- Tests for Apply: invalid numeric values (P2 fix) ---

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

func TestApply_EmptyNumericValue(t *testing.T) {
	cfg := NewDefaults()
	err := cfg.Apply(map[string]string{"THREADS": ""})
	if err == nil {
		t.Fatal("expected error for empty THREADS value, got nil")
	}
}

// --- Tests for strictAtoi ---

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
