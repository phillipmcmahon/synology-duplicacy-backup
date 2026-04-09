package errors

import (
	"errors"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Error() formatting tests
// ---------------------------------------------------------------------------

func TestBackupError_Format(t *testing.T) {
	cause := errors.New("command failed")
	e := NewBackupError("run", cause, "threads", "4", "target", "s3://bucket")
	got := e.Error()
	if !strings.HasPrefix(got, "backup/run: command failed") {
		t.Errorf("unexpected prefix: %s", got)
	}
	if !strings.Contains(got, "threads=4") {
		t.Errorf("missing context key threads: %s", got)
	}
	if !strings.Contains(got, "target=s3://bucket") {
		t.Errorf("missing context key target: %s", got)
	}
}

func TestPruneError_Format(t *testing.T) {
	cause := errors.New("threshold exceeded")
	e := NewPruneError("preview", cause, "deleteCount", "30", "maxCount", "25")
	got := e.Error()
	if !strings.HasPrefix(got, "prune/preview: threshold exceeded") {
		t.Errorf("unexpected format: %s", got)
	}
	if !strings.Contains(got, "deleteCount=30") {
		t.Errorf("missing context: %s", got)
	}
}

func TestConfigError_Format(t *testing.T) {
	cause := errors.New("value out of range")
	e := NewConfigError("THREADS", cause, "value", "99")
	got := e.Error()
	if !strings.HasPrefix(got, "config/THREADS: value out of range") {
		t.Errorf("unexpected format: %s", got)
	}
	if !strings.Contains(got, "value=99") {
		t.Errorf("missing context: %s", got)
	}
}

func TestSnapshotError_Format(t *testing.T) {
	cause := errors.New("btrfs command failed")
	e := NewSnapshotError("create", cause, "source", "/vol1/homes", "target", "/vol1/homes-snap")
	got := e.Error()
	if !strings.HasPrefix(got, "snapshot/create: btrfs command failed") {
		t.Errorf("unexpected format: %s", got)
	}
	if !strings.Contains(got, "source=/vol1/homes") {
		t.Errorf("missing context: %s", got)
	}
}

func TestSecretsError_Format(t *testing.T) {
	cause := errors.New("file not found")
	e := NewSecretsError("load", cause, "path", "/root/.secrets/duplicacy-homes.toml")
	got := e.Error()
	if !strings.HasPrefix(got, "secrets/load: file not found") {
		t.Errorf("unexpected format: %s", got)
	}
	if !strings.Contains(got, "path=/root/.secrets/duplicacy-homes.toml") {
		t.Errorf("missing context: %s", got)
	}
}

func TestPermissionsError_Format(t *testing.T) {
	cause := errors.New("chown failed")
	e := NewPermissionsError("chown", cause, "target", "/backup/homes", "owner", "admin:users")
	got := e.Error()
	if !strings.HasPrefix(got, "permissions/chown: chown failed") {
		t.Errorf("unexpected format: %s", got)
	}
}

func TestLockError_Format(t *testing.T) {
	cause := errors.New("already held")
	e := NewLockError("acquire", cause, "pid", "1234", "path", "/var/lock/backup-homes.lock.d")
	got := e.Error()
	if !strings.HasPrefix(got, "lock/acquire: already held") {
		t.Errorf("unexpected format: %s", got)
	}
	if !strings.Contains(got, "pid=1234") {
		t.Errorf("missing context: %s", got)
	}
}

// ---------------------------------------------------------------------------
// Unwrap tests
// ---------------------------------------------------------------------------

func TestBackupError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	e := NewBackupError("run", cause)
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return the original cause")
	}
}

func TestPruneError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	e := NewPruneError("validate", cause)
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return the original cause")
	}
}

func TestConfigError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	e := NewConfigError("DESTINATION", cause)
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return the original cause")
	}
}

func TestSnapshotError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	e := NewSnapshotError("delete", cause)
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return the original cause")
	}
}

func TestSecretsError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	e := NewSecretsError("validate", cause)
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return the original cause")
	}
}

func TestPermissionsError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	e := NewPermissionsError("chmod", cause)
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return the original cause")
	}
}

func TestLockError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	e := NewLockError("acquire", cause)
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return the original cause")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestError_NilCause(t *testing.T) {
	e := NewBackupError("run", nil)
	got := e.Error()
	if !strings.Contains(got, "unknown error") {
		t.Errorf("nil cause should produce 'unknown error': %s", got)
	}
}

func TestError_EmptyPhase(t *testing.T) {
	cause := errors.New("something broke")
	e := NewBackupError("", cause)
	got := e.Error()
	if !strings.HasPrefix(got, "backup: something broke") {
		t.Errorf("empty phase should omit slash: %s", got)
	}
}

func TestError_NoContext(t *testing.T) {
	cause := errors.New("failed")
	e := NewBackupError("run", cause)
	got := e.Error()
	if strings.Contains(got, "[") {
		t.Errorf("no context should omit brackets: %s", got)
	}
}

func TestError_OddContextPairs(t *testing.T) {
	cause := errors.New("failed")
	e := NewBackupError("run", cause, "orphan_key")
	got := e.Error()
	if !strings.Contains(got, "orphan_key=") {
		t.Errorf("odd trailing key should get empty value: %s", got)
	}
}

func TestError_ContextSortedAlphabetically(t *testing.T) {
	cause := errors.New("failed")
	e := NewBackupError("run", cause, "zebra", "z", "alpha", "a", "middle", "m")
	got := e.Error()
	alphaIdx := strings.Index(got, "alpha=a")
	middleIdx := strings.Index(got, "middle=m")
	zebraIdx := strings.Index(got, "zebra=z")
	if alphaIdx > middleIdx || middleIdx > zebraIdx {
		t.Errorf("context keys should be sorted alphabetically: %s", got)
	}
}

// ---------------------------------------------------------------------------
// kvPairs helper tests
// ---------------------------------------------------------------------------

func TestKvPairs_Empty(t *testing.T) {
	m := kvPairs(nil)
	if m != nil {
		t.Errorf("nil input should return nil map, got %v", m)
	}
}

func TestKvPairs_EvenPairs(t *testing.T) {
	m := kvPairs([]string{"a", "1", "b", "2"})
	if m["a"] != "1" || m["b"] != "2" {
		t.Errorf("unexpected map: %v", m)
	}
}

func TestKvPairs_OddPairs(t *testing.T) {
	m := kvPairs([]string{"a", "1", "orphan"})
	if m["a"] != "1" {
		t.Errorf("first pair wrong: %v", m)
	}
	if v, ok := m["orphan"]; !ok || v != "" {
		t.Errorf("orphan key should have empty value: %v", m)
	}
}

// ---------------------------------------------------------------------------
// errors.As type assertion tests
// ---------------------------------------------------------------------------

func TestErrorsAs_BackupError(t *testing.T) {
	var target *BackupError
	e := NewBackupError("run", errors.New("fail"))
	if !errors.As(e, &target) {
		t.Error("errors.As should match *BackupError")
	}
}

func TestErrorsAs_PruneError(t *testing.T) {
	var target *PruneError
	e := NewPruneError("run", errors.New("fail"))
	if !errors.As(e, &target) {
		t.Error("errors.As should match *PruneError")
	}
}

func TestErrorsAs_ConfigError(t *testing.T) {
	var target *ConfigError
	e := NewConfigError("THREADS", errors.New("fail"))
	if !errors.As(e, &target) {
		t.Error("errors.As should match *ConfigError")
	}
}

func TestErrorsAs_SnapshotError(t *testing.T) {
	var target *SnapshotError
	e := NewSnapshotError("create", errors.New("fail"))
	if !errors.As(e, &target) {
		t.Error("errors.As should match *SnapshotError")
	}
}

func TestErrorsAs_SecretsError(t *testing.T) {
	var target *SecretsError
	e := NewSecretsError("load", errors.New("fail"))
	if !errors.As(e, &target) {
		t.Error("errors.As should match *SecretsError")
	}
}

func TestErrorsAs_PermissionsError(t *testing.T) {
	var target *PermissionsError
	e := NewPermissionsError("chown", errors.New("fail"))
	if !errors.As(e, &target) {
		t.Error("errors.As should match *PermissionsError")
	}
}

func TestErrorsAs_LockError(t *testing.T) {
	var target *LockError
	e := NewLockError("acquire", errors.New("fail"))
	if !errors.As(e, &target) {
		t.Error("errors.As should match *LockError")
	}
}
