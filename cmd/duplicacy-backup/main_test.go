package main

import (
	"os"
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
