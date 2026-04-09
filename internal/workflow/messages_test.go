package workflow

import (
	"errors"
	"testing"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

func TestOperatorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "backup run",
			err:  apperrors.NewBackupError("run", errors.New("boom")),
			want: "Backup failed.",
		},
		{
			name: "backup write preferences translated",
			err:  apperrors.NewBackupError("write-preferences", errors.New("failed to write preferences file: permission denied")),
			want: "Failed to write preferences.",
		},
		{
			name: "prune validate repo",
			err:  apperrors.NewPruneError("validate-repo", errors.New("boom")),
			want: "Cannot perform prune operation — repository not ready.",
		},
		{
			name: "prune revision count fallback cause",
			err:  apperrors.NewPruneError("revision-count", errors.New("failed to list revisions for percentage calculation (fail-closed)")),
			want: "failed to list revisions for percentage calculation (fail-closed).",
		},
		{
			name: "message error",
			err:  NewMessageError("Refusing to continue because safe prune thresholds were exceeded."),
			want: "Refusing to continue because safe prune thresholds were exceeded.",
		},
		{
			name: "snapshot check volume cause",
			err:  apperrors.NewSnapshotError("check-volume", errors.New("path is not on a btrfs filesystem")),
			want: "path is not on a btrfs filesystem.",
		},
		{
			name: "config required fields",
			err:  apperrors.NewConfigError("required", errors.New("missing required config values: destination, threads")),
			want: "missing required config values: destination, threads.",
		},
		{
			name: "config owner validation",
			err:  apperrors.NewConfigError("local-owner", errors.New("local_owner is mandatory: set it in your TOML config under [local] to the non-root user that should own backup files (e.g. local_owner = \"myuser\")")),
			want: "local_owner is mandatory: set it in your TOML config under [local] to the non-root user that should own backup files (e.g. local_owner = \"myuser\").",
		},
		{
			name: "secrets validate",
			err:  apperrors.NewSecretsError("validate", errors.New("storj_s3_id must be at least 28 characters (was 5)")),
			want: "storj_s3_id must be at least 28 characters (was 5).",
		},
		{
			name: "secrets permissions",
			err:  apperrors.NewSecretsError("permissions", errors.New("secrets file permissions are 0644, expected 0600: /tmp/test.toml")),
			want: "secrets file permissions are 0644, expected 0600: /tmp/test.toml.",
		},
		{
			name: "lock held",
			err:  apperrors.NewLockError("held", errors.New("another backup is already running (PID: 123)")),
			want: "Lock acquisition failed: another backup is already running (PID: 123).",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OperatorMessage(tt.err); got != tt.want {
				t.Fatalf("OperatorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
