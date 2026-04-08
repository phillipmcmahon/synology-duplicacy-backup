package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDir(t *testing.T) {
	const envKey = "TEST_RESOLVE_DIR_ENV"

	tests := []struct {
		name       string
		flagValue  string
		envValue   string
		defaultDir string
		expected   string
	}{
		{"flag wins over env and default", "/from-flag", "/from-env", "/default", "/from-flag"},
		{"env wins over default", "", "/from-env", "/default", "/from-env"},
		{"default when no flag or env", "", "", "/default", "/default"},
		{"flag wins over env", "/from-flag", "/from-env", "/default", "/from-flag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv(envKey)
			if tt.envValue != "" {
				os.Setenv(envKey, tt.envValue)
				defer os.Unsetenv(envKey)
			}
			got := resolveDir(tt.flagValue, envKey, tt.defaultDir)
			if got != tt.expected {
				t.Errorf("resolveDir(%q, %q, %q) = %q, want %q", tt.flagValue, envKey, tt.defaultDir, got, tt.expected)
			}
		})
	}
}

func TestParseFlags_ConfigDir(t *testing.T) {
	f, err := parseFlags([]string{"--config-dir", "/custom/config", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.configDir != "/custom/config" {
		t.Errorf("configDir = %q, want %q", f.configDir, "/custom/config")
	}
	if f.source != "homes" {
		t.Errorf("source = %q, want %q", f.source, "homes")
	}
}

func TestParseFlags_SecretsDir(t *testing.T) {
	f, err := parseFlags([]string{"--secrets-dir", "/custom/secrets", "--remote", "homes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.secretsDir != "/custom/secrets" {
		t.Errorf("secretsDir = %q, want %q", f.secretsDir, "/custom/secrets")
	}
	if !f.remoteMode {
		t.Error("expected remoteMode to be true")
	}
}

func TestParseFlags_ConfigDirMissingValue(t *testing.T) {
	_, err := parseFlags([]string{"--config-dir"})
	if err == nil {
		t.Error("expected error for --config-dir without value")
	}
}

func TestParseFlags_SecretsDirMissingValue(t *testing.T) {
	_, err := parseFlags([]string{"--secrets-dir"})
	if err == nil {
		t.Error("expected error for --secrets-dir without value")
	}
}

func TestExecutableConfigDir(t *testing.T) {
	dir := executableConfigDir()
	// The result must end with ".config"
	if !strings.HasSuffix(dir, ".config") {
		t.Errorf("executableConfigDir() = %q, expected suffix .config", dir)
	}
	// The result should be an absolute path (test binary has an absolute path)
	if !filepath.IsAbs(dir) {
		t.Errorf("executableConfigDir() = %q, expected absolute path", dir)
	}
	// The parent of .config should be the directory containing the test binary
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable() failed: %v", err)
	}
	exe, _ = filepath.EvalSymlinks(exe)
	expected := filepath.Join(filepath.Dir(exe), ".config")
	if dir != expected {
		t.Errorf("executableConfigDir() = %q, want %q", dir, expected)
	}
}

func TestValidateLabel(t *testing.T) {
	valid := []string{
		"homes", "photos", "my-backup", "test_label", "A1", "a",
		"homes-2024", "UPPER", "MiXeD_Case-99",
	}
	for _, label := range valid {
		if err := validateLabel(label); err != nil {
			t.Errorf("validateLabel(%q) returned unexpected error: %v", label, err)
		}
	}

	invalid := []struct {
		label string
		desc  string
	}{
		{"", "empty label"},
		{"../etc", "parent directory traversal"},
		{"foo/bar", "forward slash"},
		{"foo\\bar", "backslash"},
		{"...", "dots only"},
		{".hidden", "starts with dot"},
		{"-starts-with-hyphen", "starts with hyphen"},
		{"_starts-with-underscore", "starts with underscore"},
		{"has spaces", "contains space"},
		{"label;rm", "contains semicolon"},
		{"label\ttab", "contains tab"},
		{"../../../etc/passwd", "deep path traversal"},
		{"foo..bar", "contains double dot"},
	}
	for _, tt := range invalid {
		if err := validateLabel(tt.label); err == nil {
			t.Errorf("validateLabel(%q) [%s] expected error, got nil", tt.label, tt.desc)
		}
	}
}

func TestParseFlags_RejectsTraversalLabel(t *testing.T) {
	// parseFlags itself doesn't validate labels, but we verify the label is captured
	// so that validateLabel can reject it in run()
	f, err := parseFlags([]string{"../etc"})
	if err != nil {
		t.Fatalf("parseFlags should not reject positional args: %v", err)
	}
	if err := validateLabel(f.source); err == nil {
		t.Error("expected validateLabel to reject '../etc' label")
	}
}

func TestJoinDestination(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		label       string
		expected    string
	}{
		{
			name:        "S3 URL preserves double slash",
			destination: "s3://EU@gateway.storjshare.io/42b50c98a4a49314ccc038fea46cec3c",
			label:       "homes",
			expected:    "s3://EU@gateway.storjshare.io/42b50c98a4a49314ccc038fea46cec3c/homes",
		},
		{
			name:        "S3 URL with trailing slash",
			destination: "s3://EU@gateway.storjshare.io/bucket/",
			label:       "photos",
			expected:    "s3://EU@gateway.storjshare.io/bucket/photos",
		},
		{
			name:        "HTTPS URL preserves scheme",
			destination: "https://example.com/backup",
			label:       "docs",
			expected:    "https://example.com/backup/docs",
		},
		{
			name:        "HTTP URL preserves scheme",
			destination: "http://nas.local/share",
			label:       "data",
			expected:    "http://nas.local/share/data",
		},
		{
			name:        "local path uses filepath.Join",
			destination: "/volume1/backups",
			label:       "homes",
			expected:    "/volume1/backups/homes",
		},
		{
			name:        "local path with trailing slash",
			destination: "/volume1/backups/",
			label:       "homes",
			expected:    "/volume1/backups/homes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinDestination(tt.destination, tt.label)
			if got != tt.expected {
				t.Errorf("joinDestination(%q, %q) = %q, want %q",
					tt.destination, tt.label, got, tt.expected)
			}
		})
	}
}

func TestParseFlags_UnknownOption(t *testing.T) {
	unknowns := []string{"--unknown", "--help", "--version", "-x"}
	for _, opt := range unknowns {
		_, err := parseFlags([]string{opt})
		if err == nil {
			t.Errorf("parseFlags(%q) should return error for unknown option", opt)
		}
	}
}
