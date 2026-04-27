package workflow

import (
	"os/user"
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

func TestRuntimeUserProfileDefaultsTolerateZeroRuntime(t *testing.T) {
	t.Setenv("DUPLICACY_BACKUP_CONFIG_DIR", "")
	t.Setenv("DUPLICACY_BACKUP_SECRETS_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	dirs := DefaultUserProfileDirs(Runtime{})
	if dirs.ConfigDir == "" || dirs.SecretsDir == "" || dirs.LogDir == "" || dirs.StateDir == "" || dirs.LockDir == "" {
		t.Fatalf("DefaultUserProfileDirs(Runtime{}) = %#v", dirs)
	}
	if got := ResolveDir(Runtime{}, "", "DUPLICACY_BACKUP_CONFIG_DIR", "/fallback"); got != "/fallback" {
		t.Fatalf("ResolveDir(Runtime{}) = %q, want /fallback", got)
	}
}

func TestRuntimeUserProfileDefaultsUseSudoOperatorHome(t *testing.T) {
	rt := Runtime{
		Geteuid: func() int { return 0 },
		Getenv: func(key string) string {
			switch key {
			case "HOME":
				return "/root"
			case "SUDO_USER":
				return "phillipmcmahon"
			case "SUDO_UID":
				return "1026"
			case "SUDO_GID":
				return "100"
			default:
				return ""
			}
		},
		UserLookup: func(name string) (*user.User, error) {
			if name != "phillipmcmahon" {
				t.Fatalf("UserLookup(%q), want phillipmcmahon", name)
			}
			return &user.User{Username: name, HomeDir: "/var/services/homes/phillipmcmahon"}, nil
		},
	}

	dirs := DefaultUserProfileDirs(rt)
	if dirs.ConfigDir != "/var/services/homes/phillipmcmahon/.config/duplicacy-backup" {
		t.Fatalf("ConfigDir = %q", dirs.ConfigDir)
	}
	if dirs.SecretsDir != "/var/services/homes/phillipmcmahon/.config/duplicacy-backup/secrets" {
		t.Fatalf("SecretsDir = %q", dirs.SecretsDir)
	}
	if dirs.LogDir != "/var/services/homes/phillipmcmahon/.local/state/duplicacy-backup/logs" {
		t.Fatalf("LogDir = %q", dirs.LogDir)
	}
	if dirs.StateDir != "/var/services/homes/phillipmcmahon/.local/state/duplicacy-backup/state" {
		t.Fatalf("StateDir = %q", dirs.StateDir)
	}

	meta := DefaultMetadataForRuntime("duplicacy-backup", "9.1.0", "now", rt)
	if !meta.HasProfileOwner || meta.ProfileOwnerUID != 1026 || meta.ProfileOwnerGID != 100 {
		t.Fatalf("profile owner = %t %d:%d, want true 1026:100", meta.HasProfileOwner, meta.ProfileOwnerUID, meta.ProfileOwnerGID)
	}
}

func TestDefaultMetadataForRuntimeUsesLookupGIDWhenSudoGIDMissing(t *testing.T) {
	rt := Runtime{
		Geteuid: func() int { return 0 },
		Getenv: func(key string) string {
			switch key {
			case "HOME":
				return "/root"
			case "SUDO_USER":
				return "phillipmcmahon"
			case "SUDO_UID":
				return "1026"
			default:
				return ""
			}
		},
		UserLookup: func(name string) (*user.User, error) {
			if name != "phillipmcmahon" {
				t.Fatalf("UserLookup(%q), want phillipmcmahon", name)
			}
			return &user.User{Username: name, HomeDir: "/var/services/homes/phillipmcmahon", Gid: "100"}, nil
		},
	}

	meta := DefaultMetadataForRuntime("duplicacy-backup", "9.1.0", "now", rt)
	if !meta.HasProfileOwner || meta.ProfileOwnerUID != 1026 || meta.ProfileOwnerGID != 100 {
		t.Fatalf("profile owner = %t %d:%d, want true 1026:100", meta.HasProfileOwner, meta.ProfileOwnerUID, meta.ProfileOwnerGID)
	}
}

func TestRuntimeUserProfileDefaultsIncompleteSudoMetadataUsesRootHome(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
	}{
		{
			name: "missing sudo uid",
			env: map[string]string{
				"HOME":      "/root",
				"SUDO_USER": "phillipmcmahon",
			},
		},
		{
			name: "malformed sudo uid",
			env: map[string]string{
				"HOME":      "/root",
				"SUDO_USER": "phillipmcmahon",
				"SUDO_UID":  "not-a-uid",
			},
		},
		{
			name: "missing sudo user",
			env: map[string]string{
				"HOME":     "/root",
				"SUDO_UID": "1026",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := Runtime{
				Geteuid: func() int { return 0 },
				Getenv: func(key string) string {
					return tc.env[key]
				},
				UserLookup: func(name string) (*user.User, error) {
					t.Fatalf("UserLookup(%q) should not be called for incomplete sudo metadata", name)
					return nil, nil
				},
			}

			dirs := DefaultUserProfileDirs(rt)
			if dirs.ConfigDir != "/root/.config/duplicacy-backup" {
				t.Fatalf("ConfigDir = %q", dirs.ConfigDir)
			}
			if dirs.SecretsDir != "/root/.config/duplicacy-backup/secrets" {
				t.Fatalf("SecretsDir = %q", dirs.SecretsDir)
			}
		})
	}
}

func TestRuntimeUserProfileDefaultsDirectRootUsesRootHome(t *testing.T) {
	rt := Runtime{
		Geteuid: func() int { return 0 },
		Getenv: func(key string) string {
			if key == "HOME" {
				return "/root"
			}
			return ""
		},
		UserLookup: func(name string) (*user.User, error) {
			t.Fatalf("UserLookup(%q) should not be called for direct root", name)
			return nil, nil
		},
	}

	dirs := DefaultUserProfileDirs(rt)
	if dirs.ConfigDir != "/root/.config/duplicacy-backup" {
		t.Fatalf("ConfigDir = %q", dirs.ConfigDir)
	}
	if dirs.SecretsDir != "/root/.config/duplicacy-backup/secrets" {
		t.Fatalf("SecretsDir = %q", dirs.SecretsDir)
	}

	meta := DefaultMetadataForRuntime("duplicacy-backup", "9.1.0", "now", rt)
	if meta.HasProfileOwner {
		t.Fatalf("direct root profile owner = %d:%d, want unset", meta.ProfileOwnerUID, meta.ProfileOwnerGID)
	}
}
