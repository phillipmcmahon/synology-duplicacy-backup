#!/bin/sh

set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/ci-smoke-lib.sh"

BINARY="${BINARY:-/tmp/duplicacy-backup}"
OPERATOR_USER="${OPERATOR_USER:-duplicacyci}"
STORAGE="${STORAGE:-offsite-ci}"
IMAGE="${BTRFS_IMAGE:-/tmp/duplicacy-backup-ci-sudo.btrfs}"

cleanup() {
    ci_cleanup_btrfs_volume1 "$IMAGE"
}
trap cleanup EXIT
trap 'cleanup; exit 130' HUP INT TERM

ci_require_root
ci_ensure_dsm_marker
ci_create_operator_user "$OPERATOR_USER"
ci_allow_passwordless_sudo "$OPERATOR_USER"
ci_install_binary "$BINARY"
ci_install_fake_duplicacy
ci_mount_btrfs_volume1 "$IMAGE"
ci_write_remote_config_with_secrets "$OPERATOR_USER" "$STORAGE"

operator_home="$(getent passwd "$OPERATOR_USER" | awk -F: '{print $6; exit}')"
operator_uid="$(id -u "$OPERATOR_USER")"
operator_gid="$(id -g "$OPERATOR_USER")"

explain_output="$(env -u XDG_CONFIG_HOME -u XDG_STATE_HOME SUDO_USER="$OPERATOR_USER" SUDO_UID="$operator_uid" SUDO_GID="$operator_gid" HOME=/root duplicacy-backup config explain --storage "$STORAGE" homes)"
printf '%s\n' "$explain_output"
printf '%s\n' "$explain_output" | grep -F "$operator_home/.config/duplicacy-backup/homes-backup.toml" >/dev/null
if printf '%s\n' "$explain_output" | grep -F "/root/.config/duplicacy-backup" >/dev/null; then
    ci_fail "sudo config explain resolved the root profile instead of the operator profile"
fi

backup_output="$(env -u XDG_CONFIG_HOME -u XDG_STATE_HOME SUDO_USER="$OPERATOR_USER" SUDO_UID="$operator_uid" SUDO_GID="$operator_gid" HOME=/root duplicacy-backup backup --storage "$STORAGE" --dry-run homes 2>&1)"
printf '%s\n' "$backup_output"
printf '%s\n' "$backup_output" | grep -F "duplicacy backup -stats -threads 1" >/dev/null
printf '%s\n' "$backup_output" | grep -F "Success" >/dev/null
if printf '%s\n' "$backup_output" | grep -F "/root/.config/duplicacy-backup" >/dev/null; then
    ci_fail "sudo backup resolved the root profile instead of the operator profile"
fi
