// Package errors provides structured error types for the duplicacy-backup
// application.  Each error type carries a Phase string (identifying where
// the error occurred), an underlying Cause, and a free-form Context map
// for additional debugging information such as file paths, command arguments,
// and configuration values.
//
// The coordinator (cmd/duplicacy-backup) is the sole owner of operator-facing
// messages.  Internal packages return these structured errors instead of
// logging directly, allowing the coordinator to format consistent,
// human-readable output.
package errors

import (
	"fmt"
	"sort"
	"strings"
)

// BackupError represents an error during a backup operation.
type BackupError struct {
	Phase   string
	Cause   error
	Context map[string]string
}

func (e *BackupError) Error() string { return formatError("backup", e.Phase, e.Cause, e.Context) }
func (e *BackupError) Unwrap() error { return e.Cause }

// PruneError represents an error during a prune operation.
type PruneError struct {
	Phase   string
	Cause   error
	Context map[string]string
}

func (e *PruneError) Error() string { return formatError("prune", e.Phase, e.Cause, e.Context) }
func (e *PruneError) Unwrap() error { return e.Cause }

// ConfigError represents an error during configuration loading or validation.
type ConfigError struct {
	Field   string
	Cause   error
	Context map[string]string
}

func (e *ConfigError) Error() string { return formatError("config", e.Field, e.Cause, e.Context) }
func (e *ConfigError) Unwrap() error { return e.Cause }

// SnapshotError represents an error during btrfs snapshot operations.
type SnapshotError struct {
	Phase   string
	Cause   error
	Context map[string]string
}

func (e *SnapshotError) Error() string {
	return formatError("snapshot", e.Phase, e.Cause, e.Context)
}
func (e *SnapshotError) Unwrap() error { return e.Cause }

// SecretsError represents an error during secrets loading or validation.
type SecretsError struct {
	Phase   string
	Cause   error
	Context map[string]string
}

func (e *SecretsError) Error() string { return formatError("secrets", e.Phase, e.Cause, e.Context) }
func (e *SecretsError) Unwrap() error { return e.Cause }

// LockError represents an error during lock acquisition or release.
type LockError struct {
	Phase   string
	Cause   error
	Context map[string]string
}

func (e *LockError) Error() string { return formatError("lock", e.Phase, e.Cause, e.Context) }
func (e *LockError) Unwrap() error { return e.Cause }

// ---------------------------------------------------------------------------
// Constructor helpers
// ---------------------------------------------------------------------------

// NewBackupError creates a BackupError with the given phase, cause, and
// optional context key-value pairs.
func NewBackupError(phase string, cause error, kvs ...string) *BackupError {
	return &BackupError{Phase: phase, Cause: cause, Context: kvPairs(kvs)}
}

// NewPruneError creates a PruneError with the given phase, cause, and
// optional context key-value pairs.
func NewPruneError(phase string, cause error, kvs ...string) *PruneError {
	return &PruneError{Phase: phase, Cause: cause, Context: kvPairs(kvs)}
}

// NewConfigError creates a ConfigError with the given field, cause, and
// optional context key-value pairs.
func NewConfigError(field string, cause error, kvs ...string) *ConfigError {
	return &ConfigError{Field: field, Cause: cause, Context: kvPairs(kvs)}
}

// NewSnapshotError creates a SnapshotError with the given phase, cause, and
// optional context key-value pairs.
func NewSnapshotError(phase string, cause error, kvs ...string) *SnapshotError {
	return &SnapshotError{Phase: phase, Cause: cause, Context: kvPairs(kvs)}
}

// NewSecretsError creates a SecretsError with the given phase, cause, and
// optional context key-value pairs.
func NewSecretsError(phase string, cause error, kvs ...string) *SecretsError {
	return &SecretsError{Phase: phase, Cause: cause, Context: kvPairs(kvs)}
}

// NewLockError creates a LockError with the given phase, cause, and
// optional context key-value pairs.
func NewLockError(phase string, cause error, kvs ...string) *LockError {
	return &LockError{Phase: phase, Cause: cause, Context: kvPairs(kvs)}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// kvPairs converts a flat slice of strings into a map.  Pairs are consumed
// as (key, value) tuples.  An odd trailing key gets an empty value.
func kvPairs(kvs []string) map[string]string {
	if len(kvs) == 0 {
		return nil
	}
	m := make(map[string]string, len(kvs)/2+1)
	for i := 0; i < len(kvs)-1; i += 2 {
		m[kvs[i]] = kvs[i+1]
	}
	if len(kvs)%2 != 0 {
		m[kvs[len(kvs)-1]] = ""
	}
	return m
}

// formatError builds a consistent error string.  The format is:
//
//	<domain>/<phase>: <cause> [key=value, ...]
func formatError(domain, phase string, cause error, ctx map[string]string) string {
	var b strings.Builder
	b.WriteString(domain)
	if phase != "" {
		b.WriteByte('/')
		b.WriteString(phase)
	}
	b.WriteString(": ")
	if cause != nil {
		b.WriteString(cause.Error())
	} else {
		b.WriteString("unknown error")
	}
	if len(ctx) > 0 {
		b.WriteString(" [")
		keys := make([]string, 0, len(ctx))
		for k := range ctx {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s=%s", k, ctx[k])
		}
		b.WriteByte(']')
	}
	return b.String()
}
