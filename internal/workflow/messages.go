package workflow

import (
	"errors"
	"fmt"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

// MessageError represents a workflow-owned operator message that should be
// logged as-is without additional translation.
type MessageError struct {
	Message string
}

func (e *MessageError) Error() string {
	return e.Message
}

func NewMessageError(format string, args ...interface{}) error {
	return &MessageError{Message: fmt.Sprintf(format, args...)}
}

// OperatorMessage translates internal errors into consistent operator-facing
// messages. Domain packages can keep returning rich typed errors while the
// workflow layer remains the single owner of final wording.
func OperatorMessage(err error) string {
	if err == nil {
		return ""
	}

	var msgErr *MessageError
	if errors.As(err, &msgErr) {
		return msgErr.Message
	}

	var requestErr *RequestError
	if errors.As(err, &requestErr) {
		return requestErr.Error()
	}

	var backupErr *apperrors.BackupError
	if errors.As(err, &backupErr) {
		switch backupErr.Phase {
		case "create-dirs":
			return "Failed to create Duplicacy directories."
		case "write-preferences":
			return "Failed to write preferences."
		case "write-filters":
			return "Failed to write filter file."
		case "set-permissions":
			return "Failed to set permissions in the Duplicacy work directory."
		case "run":
			return "Backup failed."
		default:
			return backupErr.Error()
		}
	}

	var pruneErr *apperrors.PruneError
	if errors.As(err, &pruneErr) {
		switch pruneErr.Phase {
		case "validate-repo":
			return "Cannot perform prune operation — repository not ready."
		case "safe-preview":
			return "Safe prune preview failed."
		case "run":
			return "Policy prune failed."
		case "deep-prune":
			return "Deep prune failed."
		default:
			return pruneErr.Error()
		}
	}

	var snapshotErr *apperrors.SnapshotError
	if errors.As(err, &snapshotErr) {
		switch snapshotErr.Phase {
		case "create":
			return "Failed to create snapshot."
		case "delete":
			return "Failed to delete snapshot subvolume."
		default:
			return snapshotErr.Error()
		}
	}

	var permissionsErr *apperrors.PermissionsError
	if errors.As(err, &permissionsErr) {
		return "Permission normalisation failed."
	}

	var configErr *apperrors.ConfigError
	if errors.As(err, &configErr) {
		if configErr.Cause != nil {
			return configErr.Cause.Error()
		}
		return configErr.Error()
	}

	var secretsErr *apperrors.SecretsError
	if errors.As(err, &secretsErr) {
		if secretsErr.Cause != nil {
			return secretsErr.Cause.Error()
		}
		return secretsErr.Error()
	}

	var lockErr *apperrors.LockError
	if errors.As(err, &lockErr) {
		if lockErr.Cause != nil {
			return fmt.Sprintf("Lock acquisition failed: %s", lockErr.Cause.Error())
		}
		return "Lock acquisition failed."
	}

	return err.Error()
}
