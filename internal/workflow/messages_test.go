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
			want: "Cannot run prune because the Duplicacy repository is not ready; run a backup first or verify the storage path and repository state",
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
			err:  apperrors.NewConfigError("local-owner", errors.New("local_owner is mandatory: set it in your TOML config under [local] to the non-root user that should own backup files (e.g. local_owner = \"myuser\")")),
			want: "local_owner is mandatory: set it in your TOML config under [local] to the non-root user that should own backup files (e.g. local_owner = \"myuser\")",
		},
		{
			name: "config missing remote table includes remote hint",
			err:  apperrors.NewConfigError("section-target", errors.New("config file /tmp/homes-backup.toml is missing required [remote] table for current mode"), "section", "remote"),
			want: "config file /tmp/homes-backup.toml is missing required [remote] table for current mode; add a [remote] table for --remote runs or drop --remote",
		},
		{
			name: "secrets validate",
			err:  apperrors.NewSecretsError("validate", errors.New("storj_s3_id must be at least 28 characters (was 5)")),
			want: "storj_s3_id must be at least 28 characters (was 5)",
		},
		{
			name: "secrets permissions",
			err:  apperrors.NewSecretsError("permissions", errors.New("secrets file permissions are 0644, expected 0600: /tmp/test.toml")),
			want: "secrets file permissions are 0644, expected 0600: /tmp/test.toml; run chmod 600 on the remote secrets file",
		},
		{
			name: "secrets stat includes creation hint",
			err:  apperrors.NewSecretsError("stat", errors.New("secrets file not found"), "path", "/root/.secrets/duplicacy-homes.toml"),
			want: "Remote secrets file not found: /root/.secrets/duplicacy-homes.toml; create duplicacy-<label>.toml under /root/.secrets or override the directory with --secrets-dir",
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
