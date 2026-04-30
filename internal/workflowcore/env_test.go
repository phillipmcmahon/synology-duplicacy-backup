package workflowcore

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
)

func TestMetadataForLogDirUsesSiblingStateAndLockRoots(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")

	meta := MetadataForLogDir("duplicacy-backup", "1.2.3", "now", logDir)

	if meta.ScriptName != "duplicacy-backup" || meta.Version != "1.2.3" || meta.BuildTime != "now" {
		t.Fatalf("metadata identity not preserved: %+v", meta)
	}
	if meta.LogDir != logDir {
		t.Fatalf("LogDir = %q, want %q", meta.LogDir, logDir)
	}
	if want := filepath.Join(filepath.Dir(logDir), "logs-state"); meta.StateDir != want {
		t.Fatalf("StateDir = %q, want %q", meta.StateDir, want)
	}
	if want := filepath.Join(filepath.Dir(logDir), "logs-locks"); meta.LockParent != want {
		t.Fatalf("LockParent = %q, want %q", meta.LockParent, want)
	}
	if meta.RootVolume != "/volume1" {
		t.Fatalf("RootVolume = %q, want /volume1", meta.RootVolume)
	}
}

func TestDefaultUserProfileDirsUsesXDGAndHomeFallbacks(t *testing.T) {
	rt := Env{Getenv: mapEnv(map[string]string{
		"XDG_CONFIG_HOME": "/profiles/operator/config",
		"XDG_STATE_HOME":  "/profiles/operator/state",
	})}

	dirs := DefaultUserProfileDirs(rt)

	if dirs.ConfigDir != "/profiles/operator/config/duplicacy-backup" {
		t.Fatalf("ConfigDir = %q", dirs.ConfigDir)
	}
	if dirs.SecretsDir != "/profiles/operator/config/duplicacy-backup/secrets" {
		t.Fatalf("SecretsDir = %q", dirs.SecretsDir)
	}
	if dirs.LogDir != "/profiles/operator/state/duplicacy-backup/logs" {
		t.Fatalf("LogDir = %q", dirs.LogDir)
	}
	if dirs.StateDir != "/profiles/operator/state/duplicacy-backup/state" {
		t.Fatalf("StateDir = %q", dirs.StateDir)
	}
	if dirs.LockDir != "/profiles/operator/state/duplicacy-backup/locks" {
		t.Fatalf("LockDir = %q", dirs.LockDir)
	}
}

func TestDefaultMetadataForEnvUsesSudoOperatorProfileOwner(t *testing.T) {
	rt := Env{
		Geteuid: func() int { return 0 },
		Getenv: mapEnv(map[string]string{
			"SUDO_USER":       "operator",
			"SUDO_UID":        "1026",
			"SUDO_GID":        "100",
			"XDG_CONFIG_HOME": "/operator/config",
			"XDG_STATE_HOME":  "/operator/state",
		}),
		UserLookup: func(name string) (*user.User, error) {
			if name != "operator" {
				return nil, errors.New("unexpected user")
			}
			return &user.User{HomeDir: "/home/operator", Gid: "100"}, nil
		},
	}

	meta := DefaultMetadataForEnv("duplicacy-backup", "dev", "build", rt)

	if !meta.HasProfileOwner {
		t.Fatalf("expected profile owner")
	}
	if meta.ProfileOwnerUID != 1026 || meta.ProfileOwnerGID != 100 {
		t.Fatalf("profile owner = %d:%d, want 1026:100", meta.ProfileOwnerUID, meta.ProfileOwnerGID)
	}
	if meta.LogDir != "/operator/state/duplicacy-backup/logs" {
		t.Fatalf("LogDir = %q", meta.LogDir)
	}
}

func TestDefaultMetadataForEnvFallsBackToLookupGroup(t *testing.T) {
	rt := Env{
		Geteuid: func() int { return 0 },
		Getenv: mapEnv(map[string]string{
			"SUDO_USER": "operator",
			"SUDO_UID":  "1026",
			"HOME":      "/root",
		}),
		UserLookup: func(name string) (*user.User, error) {
			return &user.User{HomeDir: "/home/operator", Gid: "100"}, nil
		},
	}

	meta := DefaultMetadataForEnv("duplicacy-backup", "dev", "build", rt)

	if !meta.HasProfileOwner || meta.ProfileOwnerGID != 100 {
		t.Fatalf("metadata profile owner = %+v, want gid from lookup", meta)
	}
	if meta.StateDir != "/home/operator/.local/state/duplicacy-backup/state" {
		t.Fatalf("StateDir = %q", meta.StateDir)
	}
}

func TestDefaultMetadataForEnvFailClosedForMalformedSudoMetadata(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "missing sudo user", env: map[string]string{"SUDO_UID": "1026", "SUDO_GID": "100"}},
		{name: "root sudo user", env: map[string]string{"SUDO_USER": "root", "SUDO_UID": "0", "SUDO_GID": "0"}},
		{name: "bad uid", env: map[string]string{"SUDO_USER": "operator", "SUDO_UID": "bad", "SUDO_GID": "100"}},
		{name: "bad gid", env: map[string]string{"SUDO_USER": "operator", "SUDO_UID": "1026", "SUDO_GID": "bad"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := Env{Geteuid: func() int { return 0 }, Getenv: mapEnv(tt.env)}
			meta := DefaultMetadataForEnv("duplicacy-backup", "dev", "build", rt)
			if meta.HasProfileOwner {
				t.Fatalf("HasProfileOwner = true, want fail-closed false")
			}
		})
	}
}

func TestHasSudoOperatorRequiresRootAndSudoMetadata(t *testing.T) {
	tests := []struct {
		name string
		euid int
		env  map[string]string
		want bool
	}{
		{name: "operator through sudo", euid: 0, env: map[string]string{"SUDO_USER": "operator", "SUDO_UID": "1026"}, want: true},
		{name: "non root", euid: 1026, env: map[string]string{"SUDO_USER": "operator", "SUDO_UID": "1026"}},
		{name: "missing uid", euid: 0, env: map[string]string{"SUDO_USER": "operator"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := Env{Geteuid: func() int { return tt.euid }, Getenv: mapEnv(tt.env)}
			if got := HasSudoOperator(rt); got != tt.want {
				t.Fatalf("HasSudoOperator() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveAndEffectiveDirs(t *testing.T) {
	rt := Env{Getenv: mapEnv(map[string]string{
		"CUSTOM_DIR":                       "/from/env",
		"DUPLICACY_BACKUP_CONFIG_DIR":      "/runtime/config",
		"DUPLICACY_BACKUP_SECRETS_DIR":     "/runtime/secrets",
		"HOME":                             "/home/operator",
		"UNRELATED_DUPLICACY_BACKUP_VALUE": "ignored",
	})}

	if got := ResolveDir(rt, "/from/flag", "CUSTOM_DIR", "/default"); got != "/from/flag" {
		t.Fatalf("ResolveDir flag = %q", got)
	}
	if got := ResolveDir(rt, "", "CUSTOM_DIR", "/default"); got != "/from/env" {
		t.Fatalf("ResolveDir env = %q", got)
	}
	if got := ResolveDir(rt, "", "MISSING", "/default"); got != "/default" {
		t.Fatalf("ResolveDir default = %q", got)
	}
	if got := EffectiveConfigDir(rt); got != "/runtime/config" {
		t.Fatalf("EffectiveConfigDir = %q", got)
	}
	if got := EffectiveSecretsDir(rt); got != "/runtime/secrets" {
		t.Fatalf("EffectiveSecretsDir = %q", got)
	}
}

func TestValidationHelpers(t *testing.T) {
	valid := []string{"homes", "homes_1", "homes-1"}
	for _, value := range valid {
		if err := ValidateLabel(value); err != nil {
			t.Fatalf("ValidateLabel(%q) unexpected error: %v", value, err)
		}
		if err := ValidateTargetName(value); err != nil {
			t.Fatalf("ValidateTargetName(%q) unexpected error: %v", value, err)
		}
	}

	invalid := []string{"", "../homes", "bad/name", "bad\\name", "-bad", "bad name"}
	for _, value := range invalid {
		if err := ValidateLabel(value); err == nil {
			t.Fatalf("ValidateLabel(%q) expected error", value)
		}
		if err := ValidateTargetName(value); err == nil {
			t.Fatalf("ValidateTargetName(%q) expected error", value)
		}
	}
}

func TestEnvEUIDAndSignalSetFallbacks(t *testing.T) {
	if got := EnvEUID(Env{Geteuid: func() int { return 42 }}); got != 42 {
		t.Fatalf("EnvEUID = %d, want 42", got)
	}

	want := []os.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM}
	if got := SignalSet(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SignalSet = %#v, want %#v", got, want)
	}
}

func mapEnv(values map[string]string) func(string) string {
	return func(name string) string {
		return values[name]
	}
}
