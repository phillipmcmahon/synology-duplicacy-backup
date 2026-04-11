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
			want: "Backup failed while running duplicacy backup; review the Duplicacy command details above",
		},
		{
			name: "backup write preferences translated",
			err:  apperrors.NewBackupError("write-preferences", errors.New("failed to write preferences file: permission denied"), "path", "/tmp/work/.duplicacy/preferences"),
			want: "Backup setup failed: could not write preferences at /tmp/work/.duplicacy/preferences; check work-directory permissions",
		},
		{
			name: "prune validate repo",
			err:  apperrors.NewPruneError("validate-repo", errors.New("boom")),
			want: "Repository is not ready",
		},
		{
			name: "prune revision count fallback cause",
			err:  apperrors.NewPruneError("revision-count", errors.New("failed to list revisions for percentage calculation (fail-closed)")),
			want: "failed to list revisions for percentage calculation (fail-closed); use --force-prune to override percentage-threshold enforcement if needed",
		},
		{
			name: "message error",
			err:  NewMessageError("Refusing to continue because safe prune thresholds were exceeded."),
			want: "Refusing to continue because safe prune thresholds were exceeded",
		},
		{
			name: "snapshot check volume cause",
			err:  apperrors.NewSnapshotError("check-volume", errors.New("path is not on a btrfs filesystem"), "path", "/volume1/homes"),
			want: "Btrfs validation failed for /volume1/homes: path is not on a btrfs filesystem",
		},
		{
			name: "snapshot create includes paths and hint",
			err:  apperrors.NewSnapshotError("create", errors.New("boom"), "source", "/volume1/homes", "target", "/volume1/homes-snap"),
			want: "Failed to create snapshot from /volume1/homes to /volume1/homes-snap; check btrfs health, free space, and source path validity",
		},
		{
			name: "config required fields",
			err:  apperrors.NewConfigError("required", errors.New("missing required config values: destination, threads")),
			want: "missing required config values: destination, threads",
		},
		{
			name: "config owner validation",
			err:  apperrors.NewConfigError("local-owner", errors.New("target.local_owner is mandatory: set it under [target] to the non-root user that should own backup files (e.g. local_owner = \"myuser\")")),
			want: "target.local_owner is mandatory: set it under [target] to the non-root user that should own backup files (e.g. local_owner = \"myuser\")",
		},
		{
			name: "config missing remote table includes remote hint",
			err:  apperrors.NewConfigError("section-target", errors.New("config file /tmp/homes-backup.toml is missing required [remote] table for current mode"), "section", "remote"),
			want: "config file /tmp/homes-backup.toml is missing required [remote] table for current mode; create a dedicated <label>-remote-backup.toml file or choose a different --target",
		},
		{
			name: "secrets validate",
			err:  apperrors.NewSecretsError("validate", errors.New("storj_s3_id must be at least 28 characters (was 5)")),
			want: "storj_s3_id must be at least 28 characters (was 5)",
		},
		{
			name: "secrets permissions",
			err:  apperrors.NewSecretsError("permissions", errors.New("secrets file permissions are 0644, expected 0600: /tmp/test.toml")),
			want: "secrets file permissions are 0644, expected 0600: /tmp/test.toml; run chmod 600 on the target secrets file",
		},
		{
			name: "secrets stat includes creation hint",
			err:  apperrors.NewSecretsError("stat", errors.New("secrets file not found"), "path", "/root/.secrets/duplicacy-homes-remote.toml"),
			want: "Remote secrets file not found: /root/.secrets/duplicacy-homes-remote.toml; create duplicacy-<label>-<target>.toml under /root/.secrets or override the directory with --secrets-dir",
		},
		{
			name: "permissions chown includes target hint",
			err:  apperrors.NewPermissionsError("chown", errors.New("chown failed"), "target", "/backups/homes"),
			want: "Fix permissions failed while changing ownership under /backups/homes; check that the target exists and that the owner/group values are valid on this NAS",
		},
		{
			name: "lock held",
			err:  apperrors.NewLockError("held", errors.New("another backup is already running (PID: 123)")),
			want: "Cannot start run because another backup is already running (PID: 123); wait for the active run to finish or clear a stale lock after verifying no backup is running",
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

func TestOperatorMessage_AdditionalBranches(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "backup create dirs",
			err:  apperrors.NewBackupError("create-dirs", errors.New("boom"), "path", "/tmp/work/.duplicacy"),
			want: "Backup setup failed: could not create the Duplicacy work directory at /tmp/work/.duplicacy; check parent-directory permissions and free space",
		},
		{
			name: "backup write filters",
			err:  apperrors.NewBackupError("write-filters", errors.New("boom"), "path", "/tmp/work/.duplicacy/filters"),
			want: "Backup setup failed: could not write filters at /tmp/work/.duplicacy/filters; check work-directory permissions",
		},
		{
			name: "backup set permissions",
			err:  apperrors.NewBackupError("set-permissions", errors.New("boom"), "path", "/tmp/work"),
			want: "Backup setup failed: could not set permissions in /tmp/work; check ownership and filesystem permissions on the work directory",
		},
		{
			name: "prune revision list",
			err:  apperrors.NewPruneError("revision-list", errors.New("boom")),
			want: "Could not inspect storage revisions",
		},
		{
			name: "prune revision check",
			err:  apperrors.NewPruneError("revision-check", errors.New("boom")),
			want: "Integrity check did not complete",
		},
		{
			name: "prune safe preview",
			err:  apperrors.NewPruneError("safe-preview", errors.New("boom")),
			want: "Safe prune preview failed; review the Duplicacy command details above and verify repository access",
		},
		{
			name: "prune run",
			err:  apperrors.NewPruneError("run", errors.New("boom")),
			want: "Prune failed while applying the retention policy; review the Duplicacy command details above",
		},
		{
			name: "prune cleanup storage",
			err:  apperrors.NewPruneError("cleanup-storage", errors.New("boom")),
			want: "Storage cleanup failed while running exhaustive exclusive prune; review the Duplicacy command details above and confirm no other client is using the storage",
		},
		{
			name: "snapshot delete",
			err:  apperrors.NewSnapshotError("delete", errors.New("boom"), "target", "/volume1/homes-snap"),
			want: "Failed to delete snapshot subvolume /volume1/homes-snap; check whether the snapshot still exists and whether btrfs can access it",
		},
		{
			name: "snapshot check volume cause without path",
			err:  apperrors.NewSnapshotError("check-volume", errors.New("not btrfs")),
			want: "not btrfs",
		},
		{
			name: "snapshot default falls back to error",
			err:  apperrors.NewSnapshotError("other", errors.New("snapshot generic failure")),
			want: "snapshot/other: snapshot generic failure",
		},
		{
			name: "permissions chmod",
			err:  apperrors.NewPermissionsError("chmod", errors.New("boom"), "target", "/backups/homes"),
			want: "Fix permissions failed while applying directory or file modes under /backups/homes; check filesystem permissions and whether the target tree is accessible",
		},
		{
			name: "permissions default",
			err:  apperrors.NewPermissionsError("other", errors.New("boom")),
			want: "Fix permissions failed; review the path, owner, and group settings",
		},
		{
			name: "config open with hint",
			err:  apperrors.NewConfigError("open", errors.New("cannot open"), "path", "/tmp/homes-backup.toml"),
			want: "Config file not found: /tmp/homes-backup.toml; create the TOML file or override the location with --config-dir",
		},
		{
			name: "config missing local section",
			err:  apperrors.NewConfigError("section-target", errors.New("config file /tmp/homes-backup.toml is missing required [local] table for current mode"), "section", "local"),
			want: "config file /tmp/homes-backup.toml is missing required [local] table for current mode; create a dedicated <label>-local-backup.toml file",
		},
		{
			name: "secrets ownership",
			err:  apperrors.NewSecretsError("ownership", errors.New("secrets file ownership is 1000:1000, expected 0:0 (root:root): /tmp/test.toml")),
			want: "secrets file ownership is 1000:1000, expected 0:0 (root:root): /tmp/test.toml; run chown root:root on the target secrets file",
		},
		{
			name: "secrets parse",
			err:  apperrors.NewSecretsError("parse", errors.New("unexpected key \"bad\" in secrets file /tmp/test.toml")),
			want: "unexpected key \"bad\" in secrets file /tmp/test.toml; verify the TOML syntax and the allowed remote credential keys",
		},
		{
			name: "lock create parent",
			err:  apperrors.NewLockError("create-parent", errors.New("boom"), "path", "/var/lock"),
			want: "Cannot create the lock directory parent at /var/lock; check that the lock parent path exists and is writable by root",
		},
		{
			name: "lock stale retry",
			err:  apperrors.NewLockError("stale-retry", errors.New("boom"), "path", "/var/lock/backup-homes.lock.d"),
			want: "Could not acquire the lock at /var/lock/backup-homes.lock.d after removing a stale lock; check filesystem permissions and verify that no other backup process is running",
		},
		{
			name: "lock default with cause",
			err:  apperrors.NewLockError("other", errors.New("permission denied")),
			want: "Lock acquisition failed: permission denied",
		},
		{
			name: "lock default no cause",
			err:  apperrors.NewLockError("other", nil),
			want: "Lock acquisition failed",
		},
		{
			name: "generic error strips full stop",
			err:  errors.New("Something went wrong."),
			want: "Something went wrong",
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

func TestMessageHelpers(t *testing.T) {
	if got := (&MessageError{Message: "boom"}).Error(); got != "boom" {
		t.Fatalf("MessageError.Error() = %q", got)
	}
	if got := withHint("", "check permissions"); got != "check permissions" {
		t.Fatalf("withHint(empty message) = %q", got)
	}
	if got := withHint("failed", ""); got != "failed" {
		t.Fatalf("withHint(empty hint) = %q", got)
	}
	if got := statusLinef("  hello %s  ", "world"); got != "hello world" {
		t.Fatalf("statusLinef() = %q", got)
	}
}
