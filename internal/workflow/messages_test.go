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
