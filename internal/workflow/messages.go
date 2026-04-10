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
		return normaliseOperatorSentence(operatorBackupMessage(backupErr))
	}

	var pruneErr *apperrors.PruneError
	if errors.As(err, &pruneErr) {
		return normaliseOperatorSentence(operatorPruneMessage(pruneErr))
	}

	var snapshotErr *apperrors.SnapshotError
	if errors.As(err, &snapshotErr) {
		return normaliseOperatorSentence(operatorSnapshotMessage(snapshotErr))
	}

	var permissionsErr *apperrors.PermissionsError
	if errors.As(err, &permissionsErr) {
		return normaliseOperatorSentence(operatorPermissionsMessage(permissionsErr))
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
		return normaliseOperatorSentence(operatorLockMessage(lockErr))
	}

	return normaliseOperatorSentence(err.Error())
}

func operatorBackupMessage(err *apperrors.BackupError) string {
	switch err.Phase {
	case "create-dirs":
		return withHint(
			fmt.Sprintf("Backup setup failed: could not create the Duplicacy work directory at %s", valueOrUnknown(err.Context["path"])),
			"check parent-directory permissions and free space",
		)
	case "write-preferences":
		return withHint(
			fmt.Sprintf("Backup setup failed: could not write preferences at %s", valueOrUnknown(err.Context["path"])),
			"check work-directory permissions",
		)
	case "write-filters":
		return withHint(
			fmt.Sprintf("Backup setup failed: could not write filters at %s", valueOrUnknown(err.Context["path"])),
			"check work-directory permissions",
		)
	case "set-permissions":
		return withHint(
			fmt.Sprintf("Backup setup failed: could not set permissions in %s", valueOrUnknown(err.Context["path"])),
			"check ownership and filesystem permissions on the work directory",
		)
	case "run":
		return withHint("Backup failed while running duplicacy backup", "review the Duplicacy command details above")
	default:
		return err.Error()
	}
}

func operatorPruneMessage(err *apperrors.PruneError) string {
	switch err.Phase {
	case "validate-repo":
		return withHint(
			"Cannot run prune because the Duplicacy repository is not ready",
			"run a backup first or verify the storage path and repository state",
		)
	case "safe-preview":
		return withHint(
			"Safe prune preview failed",
			"review the Duplicacy command details above and verify repository access",
		)
	case "run":
		return withHint(
			"Prune failed while applying the retention policy",
			"review the Duplicacy command details above",
		)
	case "cleanup-storage":
		return withHint(
			"Storage cleanup failed while running exhaustive exclusive prune",
			"review the Duplicacy command details above and confirm no other client is using the storage",
		)
	case "revision-count":
		if err.Cause != nil {
			return withHint(err.Cause.Error(), "use --force-prune to override percentage-threshold enforcement if needed")
		}
		return err.Error()
	default:
		return err.Error()
	}
}

func operatorSnapshotMessage(err *apperrors.SnapshotError) string {
	switch err.Phase {
	case "create":
		return withHint(
			fmt.Sprintf("Failed to create snapshot from %s to %s", valueOrUnknown(err.Context["source"]), valueOrUnknown(err.Context["target"])),
			"check btrfs health, free space, and source path validity",
		)
	case "delete":
		return withHint(
			fmt.Sprintf("Failed to delete snapshot subvolume %s", valueOrUnknown(err.Context["target"])),
			"check whether the snapshot still exists and whether btrfs can access it",
		)
	case "check-volume":
		if path := err.Context["path"]; path != "" && err.Cause != nil {
			return fmt.Sprintf("Btrfs validation failed for %s: %s", path, err.Cause.Error())
		}
		if err.Cause != nil {
			return err.Cause.Error()
		}
		return err.Error()
	default:
		return err.Error()
	}
}

func operatorPermissionsMessage(err *apperrors.PermissionsError) string {
	target := valueOrUnknown(err.Context["target"])
	switch err.Phase {
	case "chown":
		return withHint(
			fmt.Sprintf("Fix permissions failed while changing ownership under %s", target),
			"check that the target exists and that the owner/group values are valid on this NAS",
		)
	case "chmod":
		return withHint(
			fmt.Sprintf("Fix permissions failed while applying directory or file modes under %s", target),
			"check filesystem permissions and whether the target tree is accessible",
		)
	default:
		return withHint("Fix permissions failed", "review the path, owner, and group settings")
	}
}

func operatorConfigMessage(err *apperrors.ConfigError) string {
	switch err.Field {
	case "open":
		if path := err.Context["path"]; path != "" {
			return withHint(
				fmt.Sprintf("Config file not found: %s", path),
				"create the TOML file or override the location with --config-dir",
			)
		}
	case "section-target":
		if err.Cause != nil {
			switch err.Context["section"] {
			case "remote":
				return withHint(err.Cause.Error(), "add a [remote] table for --remote runs or drop --remote")
			case "local":
				return withHint(err.Cause.Error(), "add a [local] table for local runs")
			default:
				return err.Cause.Error()
			}
		}
	case "read",
		"section-common",
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
	case "stat":
		if path := err.Context["path"]; path != "" {
			return withHint(
				fmt.Sprintf("Remote secrets file not found: %s", path),
				"create duplicacy-<label>.toml under /root/.secrets or override the directory with --secrets-dir",
			)
		}
	case "permissions":
		if err.Cause != nil {
			return withHint(err.Cause.Error(), "run chmod 600 on the remote secrets file")
		}
	case "ownership":
		if err.Cause != nil {
			return withHint(err.Cause.Error(), "run chown root:root on the remote secrets file")
		}
	case "parse":
		if err.Cause != nil {
			return withHint(err.Cause.Error(), "verify the TOML syntax and the allowed remote credential keys")
		}
	case "read", "required", "open", "validate":
		if err.Cause != nil {
			return err.Cause.Error()
		}
	}
	return err.Error()
}

func operatorLockMessage(err *apperrors.LockError) string {
	switch err.Phase {
	case "held":
		if err.Cause != nil {
			return withHint(
				fmt.Sprintf("Cannot start run because %s", err.Cause.Error()),
				"wait for the active run to finish or clear a stale lock after verifying no backup is running",
			)
		}
		return withHint(
			"Cannot start run because another backup already holds the lock",
			"wait for the active run to finish or clear a stale lock after verifying no backup is running",
		)
	case "create-parent":
		return withHint(
			fmt.Sprintf("Cannot create the lock directory parent at %s", valueOrUnknown(err.Context["path"])),
			"check that the lock parent path exists and is writable by root",
		)
	case "stale-retry":
		return withHint(
			fmt.Sprintf("Could not acquire the lock at %s after removing a stale lock", valueOrUnknown(err.Context["path"])),
			"check filesystem permissions and verify that no other backup process is running",
		)
	default:
		if err.Cause != nil {
			return fmt.Sprintf("Lock acquisition failed: %s", err.Cause.Error())
		}
		return "Lock acquisition failed"
	}
}

func normaliseOperatorSentence(message string) string {
	message = strings.TrimSpace(message)
	return strings.TrimSuffix(message, ".")
}

func statusLinef(format string, args ...interface{}) string {
	return strings.TrimSpace(fmt.Sprintf(format, args...))
}

func withHint(message, hint string) string {
	if message == "" {
		return strings.TrimSpace(hint)
	}
	if hint == "" {
		return strings.TrimSpace(message)
	}
	return fmt.Sprintf("%s; %s", strings.TrimSpace(message), strings.TrimSpace(hint))
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "<unknown>"
	}
	return value
}
