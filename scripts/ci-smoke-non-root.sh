#!/bin/sh

set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/ci-smoke-lib.sh"

BINARY="${BINARY:-/tmp/duplicacy-backup}"
OPERATOR_USER="${OPERATOR_USER:-duplicacyci}"
TARGET="${TARGET:-onsite-ci}"
IMAGE="${BTRFS_IMAGE:-/tmp/duplicacy-backup-ci-non-root.btrfs}"

ci_require_root
ci_ensure_dsm_marker
ci_create_operator_user "$OPERATOR_USER"
ci_install_binary "$BINARY"
ci_mount_btrfs_volume1 "$IMAGE"
ci_write_local_config "$OPERATOR_USER" "$TARGET" "/volume1/duplicacy/homes"
chown -R "$OPERATOR_USER:$(id -gn "$OPERATOR_USER")" /volume1/duplicacy

operator_home="$(getent passwd "$OPERATOR_USER" | awk -F: '{print $6; exit}')"

sudo -u "$OPERATOR_USER" env -u XDG_CONFIG_HOME -u XDG_STATE_HOME HOME="$operator_home" duplicacy-backup --version
sudo -u "$OPERATOR_USER" env -u XDG_CONFIG_HOME -u XDG_STATE_HOME HOME="$operator_home" duplicacy-backup config explain --target "$TARGET" homes
sudo -u "$OPERATOR_USER" env -u XDG_CONFIG_HOME -u XDG_STATE_HOME HOME="$operator_home" duplicacy-backup config paths --target "$TARGET" homes
sudo -u "$OPERATOR_USER" env -u XDG_CONFIG_HOME -u XDG_STATE_HOME HOME="$operator_home" duplicacy-backup diagnostics --target "$TARGET" homes
sudo -u "$OPERATOR_USER" env -u XDG_CONFIG_HOME -u XDG_STATE_HOME HOME="$operator_home" duplicacy-backup restore plan --target "$TARGET" homes
