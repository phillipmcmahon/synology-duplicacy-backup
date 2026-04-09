package workflow

import (
	"errors"
	"fmt"
	"strings"

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
		return normaliseOperatorSentence(msgErr.Message)
	}

	var requestErr *RequestError
	if errors.As(err, &requestErr) {
		return normaliseOperatorSentence(requestErr.Error())
	}

	var backupErr *apperrors.BackupError
	if errors.As(err, &backupErr) {
		switch backupErr.Phase {
		case "create-dirs":
			return normaliseOperatorSentence("Failed to create Duplicacy directories.")
		case "write-preferences":
			return normaliseOperatorSentence("Failed to write preferences.")
		case "write-filters":
			return normaliseOperatorSentence("Failed to write filter file.")
		case "set-permissions":
			return normaliseOperatorSentence("Failed to set permissions in the Duplicacy work directory.")
		case "run":
			return normaliseOperatorSentence("Backup failed.")
		default:
			return normaliseOperatorSentence(backupErr.Error())
		}
	}

	var pruneErr *apperrors.PruneError
	if errors.As(err, &pruneErr) {
		switch pruneErr.Phase {
		case "validate-repo":
			return normaliseOperatorSentence("Cannot perform prune operation — repository not ready.")
		case "safe-preview":
			return normaliseOperatorSentence("Safe prune preview failed.")
		case "run":
			return normaliseOperatorSentence("Policy prune failed.")
		case "deep-prune":
			return normaliseOperatorSentence("Deep prune failed.")
		case "revision-count":
			if pruneErr.Cause != nil {
				return normaliseOperatorSentence(pruneErr.Cause.Error())
			}
			return normaliseOperatorSentence(pruneErr.Error())
		default:
			return normaliseOperatorSentence(pruneErr.Error())
		}
	}

	var snapshotErr *apperrors.SnapshotError
	if errors.As(err, &snapshotErr) {
		switch snapshotErr.Phase {
		case "create":
			return normaliseOperatorSentence("Failed to create snapshot.")
		case "delete":
			return normaliseOperatorSentence("Failed to delete snapshot subvolume.")
		case "check-volume":
			if snapshotErr.Cause != nil {
				return normaliseOperatorSentence(snapshotErr.Cause.Error())
			}
			return normaliseOperatorSentence(snapshotErr.Error())
		default:
			return normaliseOperatorSentence(snapshotErr.Error())
		}
	}

	var permissionsErr *apperrors.PermissionsError
	if errors.As(err, &permissionsErr) {
		return normaliseOperatorSentence("Permission normalisation failed.")
	}

	var configErr *apperrors.ConfigError
	if errors.As(err, &configErr) {
		return normaliseOperatorSentence(operatorConfigMessage(configErr))
	}

	var secretsErr *apperrors.SecretsError
	if errors.As(err, &secretsErr) {
		return normaliseOperatorSentence(operatorSecretsMessage(secretsErr))
	}

	var lockErr *apperrors.LockError
	if errors.As(err, &lockErr) {
		if lockErr.Cause != nil {
			return normaliseOperatorSentence(fmt.Sprintf("Lock acquisition failed: %s", lockErr.Cause.Error()))
		}
		return normaliseOperatorSentence("Lock acquisition failed.")
	}

	return normaliseOperatorSentence(err.Error())
}

func operatorConfigMessage(err *apperrors.ConfigError) string {
	switch err.Field {
	case "open",
		"read",
		"section-common",
		"section-target",
		"required",
		"threads",
		"log-retention-days",
		"safe-prune-max-delete-percent",
		"safe-prune-max-delete-count",
		"safe-prune-min-total-for-percent",
		"local-owner",
		"local-group",
		"parse":
		if err.Cause != nil {
			return err.Cause.Error()
		}
	}
	return err.Error()
}

func operatorSecretsMessage(err *apperrors.SecretsError) string {
	switch err.Phase {
	case "stat", "permissions", "ownership", "parse", "read", "required", "open", "validate":
		if err.Cause != nil {
			return err.Cause.Error()
		}
	}
	return err.Error()
}

func normaliseOperatorSentence(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	switch message[len(message)-1] {
	case '.', '!', '?', ':':
		return message
	default:
		return message + "."
	}
}

func statusLinef(format string, args ...interface{}) string {
	return normaliseOperatorSentence(fmt.Sprintf(format, args...))
}
