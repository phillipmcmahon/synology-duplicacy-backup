package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeHelpers(t *testing.T) {
	if err := ValidateLabel(""); err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("ValidateLabel(empty) = %v", err)
	}
	if err := ValidateLabel("../etc"); err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("ValidateLabel(traversal) = %v", err)
	}
	if err := ValidateLabel("bad label"); err == nil || !strings.Contains(err.Error(), "invalid characters") {
		t.Fatalf("ValidateLabel(invalid) = %v", err)
	}
	if err := ValidateLabel("homes_01"); err != nil {
		t.Fatalf("ValidateLabel(valid) error = %v", err)
	}

	rt := testRuntime()
	rt.Getenv = func(key string) string {
		if key == "TEST_ENV_DIR" {
			return "/env/config"
		}
		return ""
	}
	if got := ResolveDir(rt, "/flag/config", "TEST_ENV_DIR", "/default/config"); got != "/flag/config" {
		t.Fatalf("ResolveDir(flag) = %q", got)
	}
	if got := ResolveDir(rt, "", "TEST_ENV_DIR", "/default/config"); got != "/env/config" {
		t.Fatalf("ResolveDir(env) = %q", got)
	}
	if got := ResolveDir(rt, "", "UNSET_ENV_DIR", "/default/config"); got != "/default/config" {
		t.Fatalf("ResolveDir(default) = %q", got)
	}

	base := t.TempDir()
	exePath := filepath.Join(base, "bin", "duplicacy-backup")
	if err := os.MkdirAll(filepath.Dir(exePath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	rt.Executable = func() (string, error) { return exePath, nil }
	rt.EvalSymlinks = func(path string) (string, error) { return path, nil }

	if got := ExecutableConfigDir(rt); got != filepath.Join(filepath.Dir(exePath), ".config") {
		t.Fatalf("ExecutableConfigDir() = %q", got)
	}
	if got := EffectiveConfigDir(rt); got != filepath.Join(filepath.Dir(exePath), ".config") {
		t.Fatalf("EffectiveConfigDir() = %q", got)
	}

	rt.Getenv = func(key string) string {
		if key == "DUPLICACY_BACKUP_CONFIG_DIR" {
			return "/override/config"
		}
		return ""
	}
	if got := EffectiveConfigDir(rt); got != "/override/config" {
		t.Fatalf("EffectiveConfigDir(env override) = %q", got)
	}

	rt.Executable = func() (string, error) { return "", os.ErrNotExist }
	if got := ExecutableConfigDir(rt); got != filepath.Join(".", ".config") {
		t.Fatalf("ExecutableConfigDir(fallback) = %q", got)
	}
}
