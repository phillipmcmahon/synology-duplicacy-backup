package workflow

import (
	"errors"
	"testing"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

func TestOperatorMessage_BackupRun(t *testing.T) {
	err := apperrors.NewBackupError("run", errors.New("boom"))
	if got := OperatorMessage(err); got != "Backup failed." {
		t.Fatalf("OperatorMessage() = %q", got)
	}
}

func TestOperatorMessage_PruneValidateRepo(t *testing.T) {
	err := apperrors.NewPruneError("validate-repo", errors.New("boom"))
	if got := OperatorMessage(err); got != "Cannot perform prune operation — repository not ready." {
		t.Fatalf("OperatorMessage() = %q", got)
	}
}

func TestOperatorMessage_MessageError(t *testing.T) {
	err := NewMessageError("Refusing to continue because safe prune thresholds were exceeded.")
	if got := OperatorMessage(err); got != "Refusing to continue because safe prune thresholds were exceeded." {
		t.Fatalf("OperatorMessage() = %q", got)
	}
}

func TestOperatorMessage_ConfigErrorUsesCause(t *testing.T) {
	err := apperrors.NewConfigError("threads", errors.New("THREADS must be a power of 2 and <= 16 (was 3)"))
	if got := OperatorMessage(err); got != "THREADS must be a power of 2 and <= 16 (was 3)" {
		t.Fatalf("OperatorMessage() = %q", got)
	}
}

func TestOperatorMessage_SecretsErrorUsesCause(t *testing.T) {
	err := apperrors.NewSecretsError("validate", errors.New("STORJ_S3_ID must be at least 28 characters (was 5)"))
	if got := OperatorMessage(err); got != "STORJ_S3_ID must be at least 28 characters (was 5)" {
		t.Fatalf("OperatorMessage() = %q", got)
	}
}

func TestOperatorMessage_LockErrorPrefixed(t *testing.T) {
	err := apperrors.NewLockError("held", errors.New("another backup is already running (PID: 123)"))
	if got := OperatorMessage(err); got != "Lock acquisition failed: another backup is already running (PID: 123)" {
		t.Fatalf("OperatorMessage() = %q", got)
	}
}
