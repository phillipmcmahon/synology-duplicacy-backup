#!/bin/sh

set -eu

LEGACY_CONFIG_DIR="/usr/local/lib/duplicacy-backup/.config"
LEGACY_SECRETS_DIR="/root/.secrets"
TARGET_USER=""
TARGET_HOME=""
CONFIG_DIR=""
SECRETS_DIR=""
MOVE=0
DRY_RUN=0
FORCE=0

usage() {
    cat <<'EOF'
Usage: ./migrate-runtime-profile.sh [OPTIONS]

Migrate legacy root-era runtime files into the non-root user profile:

  config  : /usr/local/lib/duplicacy-backup/.config/*.toml
        -> $HOME/.config/duplicacy-backup/

  secrets : /root/.secrets/*.toml
        -> $HOME/.config/duplicacy-backup/secrets/

The script copies by default. Use --move only after reviewing the dry run.
Destination directories are created with 0700 and migrated TOML files are set
to 0600. When run as root, destination files are chowned to the target user.

Options:
  --target-user <name>       User that should own the migrated files
                             default: SUDO_USER when run via sudo, otherwise current user
                             required when running from a direct root shell
  --target-home <path>       Home directory for the target user
                             default: inferred from passwd or HOME
  --legacy-config-dir <path> Legacy config directory
                             default: /usr/local/lib/duplicacy-backup/.config
  --legacy-secrets-dir <path>
                             Legacy secrets directory
                             default: /root/.secrets
  --config-dir <path>        Destination config directory
                             default: <target-home>/.config/duplicacy-backup
  --secrets-dir <path>       Destination secrets directory
                             default: <config-dir>/secrets
  --move                     Remove each source file after successful copy
  --force                    Overwrite existing destination files
  --dry-run                  Print planned actions without changing files
  --help                     Show this help text
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

info() {
    echo "$*"
}

run() {
    if [ "$DRY_RUN" -eq 1 ]; then
        printf 'Dry run:'
        for arg in "$@"; do
            printf ' %s' "$arg"
        done
        printf '\n'
        return 0
    fi
    "$@"
}

current_user() {
    id -un
}

resolve_target_user() {
    if [ -n "$TARGET_USER" ]; then
        printf '%s\n' "$TARGET_USER"
        return 0
    fi
    if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ] && [ "${SUDO_USER:-}" != "root" ]; then
        printf '%s\n' "$SUDO_USER"
        return 0
    fi
    if [ "$(id -u)" -eq 0 ]; then
        fail "root shell migration needs --target-user <operator-user> or sudo from the operator account"
    fi
    current_user
}

passwd_home() {
    user="$1"
    if command -v getent >/dev/null 2>&1; then
        getent passwd "$user" | awk -F: '{print $6; exit}'
        return 0
    fi
    awk -F: -v user="$user" '$1 == user {print $6; exit}' /etc/passwd
}

resolve_target_home() {
    user="$1"
    if [ -n "$TARGET_HOME" ]; then
        printf '%s\n' "$TARGET_HOME"
        return 0
    fi
    if [ "$user" = "$(current_user)" ] && [ -n "${HOME:-}" ]; then
        printf '%s\n' "$HOME"
        return 0
    fi
    home="$(passwd_home "$user")"
    [ -n "$home" ] || fail "could not infer home for user $user; pass --target-home <path>"
    printf '%s\n' "$home"
}

ensure_target_allowed() {
    user="$1"
    if [ "$(id -u)" -eq 0 ]; then
        return 0
    fi
    if [ "$user" = "$(current_user)" ]; then
        return 0
    fi
    fail "non-root migration can only target the current user; run with sudo or choose --target-user $(current_user)"
}

ensure_dir() {
    dir="$1"
    user="$2"
    run mkdir -p "$dir"
    run chmod 700 "$dir"
    if [ "$(id -u)" -eq 0 ]; then
        run chown "$user" "$dir"
    fi
}

migrate_file() {
    src="$1"
    dst="$2"
    user="$3"

    [ -f "$src" ] || return 0
    if [ -e "$dst" ] && [ "$FORCE" -ne 1 ]; then
        fail "destination already exists: $dst (use --force to overwrite)"
    fi

    info "Migrating: $src -> $dst"
    run cp "$src" "$dst"
    run chmod 600 "$dst"
    if [ "$(id -u)" -eq 0 ]; then
        run chown "$user" "$dst"
    fi
    if [ "$MOVE" -eq 1 ]; then
        run rm -f "$src"
    fi
}

migrate_dir_toml() {
    src_dir="$1"
    dst_dir="$2"
    user="$3"
    label="$4"
    count=0

    if [ ! -d "$src_dir" ]; then
        info "$label source not found, skipping: $src_dir"
        return 0
    fi

    ensure_dir "$dst_dir" "$user"
    for src in "$src_dir"/*.toml; do
        [ -f "$src" ] || continue
        dst="$dst_dir/$(basename "$src")"
        migrate_file "$src" "$dst" "$user"
        count=$((count + 1))
    done
    info "$label files considered: $count"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --target-user)
            [ "$#" -ge 2 ] || fail "--target-user requires a value"
            TARGET_USER="$2"
            shift 2
            ;;
        --target-home)
            [ "$#" -ge 2 ] || fail "--target-home requires a value"
            TARGET_HOME="$2"
            shift 2
            ;;
        --legacy-config-dir)
            [ "$#" -ge 2 ] || fail "--legacy-config-dir requires a value"
            LEGACY_CONFIG_DIR="$2"
            shift 2
            ;;
        --legacy-secrets-dir)
            [ "$#" -ge 2 ] || fail "--legacy-secrets-dir requires a value"
            LEGACY_SECRETS_DIR="$2"
            shift 2
            ;;
        --config-dir)
            [ "$#" -ge 2 ] || fail "--config-dir requires a value"
            CONFIG_DIR="$2"
            shift 2
            ;;
        --secrets-dir)
            [ "$#" -ge 2 ] || fail "--secrets-dir requires a value"
            SECRETS_DIR="$2"
            shift 2
            ;;
        --move)
            MOVE=1
            shift
            ;;
        --force)
            FORCE=1
            shift
            ;;
        --dry-run)
            DRY_RUN=1
            shift
            ;;
        --help)
            usage
            exit 0
            ;;
        *)
            fail "unknown option: $1"
            ;;
    esac
done

TARGET_USER="$(resolve_target_user)"
TARGET_HOME="$(resolve_target_home "$TARGET_USER")"
ensure_target_allowed "$TARGET_USER"

if [ -z "$CONFIG_DIR" ]; then
    CONFIG_DIR="$TARGET_HOME/.config/duplicacy-backup"
fi
if [ -z "$SECRETS_DIR" ]; then
    SECRETS_DIR="$CONFIG_DIR/secrets"
fi

info "Target user       : $TARGET_USER"
info "Target home       : $TARGET_HOME"
info "Destination config: $CONFIG_DIR"
info "Destination secrets: $SECRETS_DIR"
if [ "$MOVE" -eq 1 ]; then
    info "Mode              : move after successful copy"
else
    info "Mode              : copy only"
fi

migrate_dir_toml "$LEGACY_CONFIG_DIR" "$CONFIG_DIR" "$TARGET_USER" "Config"
migrate_dir_toml "$LEGACY_SECRETS_DIR" "$SECRETS_DIR" "$TARGET_USER" "Secrets"

info "Migration complete"
