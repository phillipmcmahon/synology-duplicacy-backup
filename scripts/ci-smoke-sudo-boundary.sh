#!/bin/sh

set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/ci-smoke-lib.sh"

BINARY="${BINARY:-/tmp/duplicacy-backup}"
OPERATOR_USER="${OPERATOR_USER:-operator}"
TARGET="${TARGET:-offsite-ci}"
IMAGE="${BTRFS_IMAGE:-/tmp/duplicacy-backup-ci-sudo.btrfs}"

ci_require_root
ci_ensure_dsm_marker
ci_create_operator_user "$OPERATOR_USER"
ci_allow_passwordless_sudo "$OPERATOR_USER"
ci_install_binary "$BINARY"
ci_mount_btrfs_volume1 "$IMAGE"
ci_write_remote_config_with_secrets "$OPERATOR_USER" "$TARGET"

operator_home="$(getent passwd "$OPERATOR_USER" | awk -F: '{print $6; exit}')"
explain_output="$(sudo -H -u "$OPERATOR_USER" sudo -n duplicacy-backup config explain --target "$TARGET" homes)"
printf '%s\n' "$explain_output"
printf '%s\n' "$explain_output" | grep -F "$operator_home/.config/duplicacy-backup/homes-backup.toml" >/dev/null
if printf '%s\n' "$explain_output" | grep -F "/root/.config/duplicacy-backup" >/dev/null; then
    ci_fail "sudo config explain resolved the root profile instead of the operator profile"
fi

sudo -H -u "$OPERATOR_USER" sudo -n duplicacy-backup backup --target "$TARGET" --dry-run homes
