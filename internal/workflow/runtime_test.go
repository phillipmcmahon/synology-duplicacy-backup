package workflow

import (
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
		if key == "HOME" {
			return "/home/operator"
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

	if got := EffectiveConfigDir(rt); got != "/home/operator/.config/duplicacy-backup" {
		t.Fatalf("EffectiveConfigDir() = %q", got)
	}
	if got := EffectiveSecretsDir(rt); got != "/home/operator/.config/duplicacy-backup/secrets" {
		t.Fatalf("EffectiveSecretsDir() = %q", got)
	}

	rt.Getenv = func(key string) string {
		if key == "DUPLICACY_BACKUP_CONFIG_DIR" {
			return "/override/config"
		}
		if key == "DUPLICACY_BACKUP_SECRETS_DIR" {
			return "/override/secrets"
		}
		if key == "HOME" {
			return "/home/operator"
		}
		return ""
	}
	if got := EffectiveConfigDir(rt); got != "/override/config" {
		t.Fatalf("EffectiveConfigDir(env override) = %q", got)
	}
	if got := EffectiveSecretsDir(rt); got != "/override/secrets" {
		t.Fatalf("EffectiveSecretsDir(env override) = %q", got)
	}

	rt.Getenv = func(key string) string {
		switch key {
		case "XDG_CONFIG_HOME":
			return "/xdg/config"
		case "XDG_STATE_HOME":
			return "/xdg/state"
		case "HOME":
			return "/home/operator"
		default:
			return ""
		}
	}
	dirs := DefaultUserProfileDirs(rt)
	if dirs.ConfigDir != filepath.Join("/xdg/config", "duplicacy-backup") ||
		dirs.SecretsDir != filepath.Join("/xdg/config", "duplicacy-backup", "secrets") ||
		dirs.LogDir != filepath.Join("/xdg/state", "duplicacy-backup", "logs") ||
		dirs.StateDir != filepath.Join("/xdg/state", "duplicacy-backup", "state") ||
		dirs.LockDir != filepath.Join("/xdg/state", "duplicacy-backup", "locks") {
		t.Fatalf("DefaultUserProfileDirs() = %#v", dirs)
	}
}
