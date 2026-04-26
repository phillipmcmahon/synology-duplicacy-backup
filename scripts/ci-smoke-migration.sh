#!/bin/sh

set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/ci-smoke-lib.sh"

OPERATOR_USER="${OPERATOR_USER:-duplicacyci}"
LEGACY_CONFIG_DIR="${LEGACY_CONFIG_DIR:-/usr/local/lib/duplicacy-backup/.config}"
LEGACY_SECRETS_DIR="${LEGACY_SECRETS_DIR:-/root/.secrets}"

ci_require_root
ci_create_operator_user "$OPERATOR_USER"

mkdir -p "$LEGACY_CONFIG_DIR" "$LEGACY_SECRETS_DIR"
cat >"$LEGACY_CONFIG_DIR/homes-backup.toml" <<'EOF'
label = "homes"
source_path = "/volume1/source"

[targets.onsite-ci]
location = "local"
storage = "/volume1/duplicacy/homes"
EOF
cat >"$LEGACY_SECRETS_DIR/homes-secrets.toml" <<'EOF'
[targets.offsite-ci.keys]
s3_id = "ABCDEFGHIJKLMNOPQRSTUVWXYZ01"
s3_secret = "abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR"
EOF
chmod 0600 "$LEGACY_CONFIG_DIR/homes-backup.toml" "$LEGACY_SECRETS_DIR/homes-secrets.toml"

sh "$SCRIPT_DIR/migrate-runtime-profile.sh" --target-user "$OPERATOR_USER" --dry-run
sh "$SCRIPT_DIR/migrate-runtime-profile.sh" --target-user "$OPERATOR_USER"

operator_home="$(getent passwd "$OPERATOR_USER" | awk -F: '{print $6; exit}')"
operator_group="$(id -gn "$OPERATOR_USER")"
config_dir="$operator_home/.config/duplicacy-backup"
secrets_dir="$config_dir/secrets"

for dir in "$config_dir" "$secrets_dir"; do
    [ -d "$dir" ] || ci_fail "missing migrated directory: $dir"
    [ "$(stat -c '%a' "$dir")" = "700" ] || ci_fail "unexpected mode for $dir"
    [ "$(stat -c '%U:%G' "$dir")" = "$OPERATOR_USER:$operator_group" ] || ci_fail "unexpected owner for $dir"
done

for file in "$config_dir/homes-backup.toml" "$secrets_dir/homes-secrets.toml"; do
    [ -f "$file" ] || ci_fail "missing migrated file: $file"
    [ "$(stat -c '%a' "$file")" = "600" ] || ci_fail "unexpected mode for $file"
    [ "$(stat -c '%U:%G' "$file")" = "$OPERATOR_USER:$operator_group" ] || ci_fail "unexpected owner for $file"
done
