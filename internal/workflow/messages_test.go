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
			name: "prune validate repo",
			err:  apperrors.NewPruneError("validate-repo", errors.New("boom")),
			want: "Cannot perform prune operation — repository not ready.",
		},
		{
			name: "message error",
			err:  NewMessageError("Refusing to continue because safe prune thresholds were exceeded."),
			want: "Refusing to continue because safe prune thresholds were exceeded.",
		},
		{
			name: "config required fields",
			err:  apperrors.NewConfigError("required", errors.New("missing required config variables: DESTINATION, THREADS")),
			want: "missing required config variables: DESTINATION, THREADS.",
		},
		{
			name: "config owner validation",
			err:  apperrors.NewConfigError("local-owner", errors.New("LOCAL_OWNER is mandatory: set it in your .conf file to the non-root user that should own backup files (e.g. LOCAL_OWNER=myuser)")),
			want: "LOCAL_OWNER is mandatory: set it in your .conf file to the non-root user that should own backup files (e.g. LOCAL_OWNER=myuser).",
		},
		{
			name: "secrets validate",
			err:  apperrors.NewSecretsError("validate", errors.New("STORJ_S3_ID must be at least 28 characters (was 5)")),
			want: "STORJ_S3_ID must be at least 28 characters (was 5).",
		},
		{
			name: "secrets permissions",
			err:  apperrors.NewSecretsError("permissions", errors.New("secrets file permissions are 0644, expected 0600: /tmp/test.env")),
			want: "secrets file permissions are 0644, expected 0600: /tmp/test.env.",
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
