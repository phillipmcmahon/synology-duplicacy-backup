#!/bin/sh

set -eu

INSTALL_ROOT="/usr/local/lib/duplicacy-backup"
BIN_DIR="/usr/local/bin"
CONFIG_GROUP="administrators"
CONFIG_GROUP_EXPLICIT=0
ACTIVATE=1
KEEP=0
BINARY_PATH=""
TEMP_TARGET_PATH=""

usage() {
    cat <<'EOF'
Usage: ./install.sh [OPTIONS]

Install a versioned duplicacy-backup binary into a stable Synology layout:

  <install-root>/
    duplicacy-backup_<version>_linux_<arch>
    current -> duplicacy-backup_<version>_linux_<arch>
    .config/

and create a stable command path:

  <bin-dir>/duplicacy-backup -> <install-root>/current

Config files stay under:

  <install-root>/.config/

Object-target secrets stay under:

  /root/.secrets/

When run as root, the installer ensures that directory exists with
root-only access. It never creates or rewrites individual secrets files.

Options:
  --binary <path>         Install this binary instead of auto-detecting one
  --install-root <path>   Versioned binary directory
                          default: /usr/local/lib/duplicacy-backup
  --bin-dir <path>        Directory for the stable command symlink
                          default: /usr/local/bin
  --config-group <name>   Group granted read/traverse access to .config when run as root
                          default: administrators
  --keep <count>          Keep this many newest installed binaries and prune older ones
                          default: 0 (keep all)
  --no-activate           Copy the binary but do not update the current/bin symlinks
  --help                  Show this help text
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

cleanup_temp_target() {
    if [ -n "$TEMP_TARGET_PATH" ]; then
        rm -f -- "$TEMP_TARGET_PATH"
    fi
}

trap cleanup_temp_target EXIT HUP INT TERM

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

group_exists() {
    group="$1"
    [ -n "$group" ] || return 1
    [ -r /etc/group ] || return 1
    grep -q "^$group:" /etc/group
}

script_dir() {
    CDPATH= cd -- "$(dirname -- "$0")" && pwd
}

nearest_existing_dir() {
    path="$1"
    while [ ! -e "$path" ] && [ "$path" != "/" ]; do
        path=$(dirname "$path")
    done
    if [ -d "$path" ]; then
        printf '%s\n' "$path"
        return 0
    fi
    printf '%s\n' "$(dirname "$path")"
}

ensure_writable_target() {
    target="$1"
    label="$2"
    existing="$(nearest_existing_dir "$target")"

    if [ -w "$existing" ]; then
        return 0
    fi

    if [ "$(id -u)" -ne 0 ]; then
        fail "$label is not writable: $target (nearest existing directory: $existing). Run as root or choose a user-writable path."
    fi
}

find_binary() {
    dir="$1"
    found=""
    count=0
    for candidate in "$dir"/duplicacy-backup_*_linux_*; do
        if [ ! -f "$candidate" ]; then
            continue
        fi
        found="$candidate"
        count=$((count + 1))
    done

    if [ "$count" -eq 0 ]; then
        fail "no versioned duplicacy-backup binary found in $dir"
    fi
    if [ "$count" -gt 1 ]; then
        fail "multiple candidate binaries found in $dir; use --binary <path>"
    fi

    printf '%s\n' "$found"
}

prune_old_binaries() {
    root="$1"
    keep="$2"

    if [ "$keep" -le 0 ]; then
        return 0
    fi

    count=0
    for file in $(ls -1t "$root"/duplicacy-backup_*_linux_* 2>/dev/null); do
        count=$((count + 1))
        if [ "$count" -le "$keep" ]; then
            continue
        fi
        if [ "$(basename "$file")" = "$(readlink "$root/current" 2>/dev/null || true)" ]; then
            continue
        fi
        rm -f -- "$file"
        echo "Pruned old binary: $(basename "$file")"
    done
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --binary)
            [ "$#" -ge 2 ] || fail "--binary requires a value"
            BINARY_PATH="$2"
            shift 2
            ;;
        --install-root)
            [ "$#" -ge 2 ] || fail "--install-root requires a value"
            INSTALL_ROOT="$2"
            shift 2
            ;;
        --bin-dir)
            [ "$#" -ge 2 ] || fail "--bin-dir requires a value"
            BIN_DIR="$2"
            shift 2
            ;;
        --config-group)
            [ "$#" -ge 2 ] || fail "--config-group requires a value"
            CONFIG_GROUP="$2"
            CONFIG_GROUP_EXPLICIT=1
            shift 2
            ;;
        --keep)
            [ "$#" -ge 2 ] || fail "--keep requires a value"
            KEEP="$2"
            shift 2
            ;;
        --no-activate)
            ACTIVATE=0
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

case "$KEEP" in
    ''|*[!0-9]*)
        fail "--keep must be a non-negative integer"
        ;;
esac

require_command id
require_command dirname
require_command grep
require_command readlink

PKG_DIR="$(script_dir)"

if [ -z "$BINARY_PATH" ]; then
    BINARY_PATH="$(find_binary "$PKG_DIR")"
fi

[ -f "$BINARY_PATH" ] || fail "binary not found: $BINARY_PATH"

ensure_writable_target "$INSTALL_ROOT" "install root"
ensure_writable_target "$BIN_DIR" "binary directory"

BINARY_NAME="$(basename "$BINARY_PATH")"
TARGET_PATH="$INSTALL_ROOT/$BINARY_NAME"
CURRENT_LINK="$INSTALL_ROOT/current"
STABLE_LINK="$BIN_DIR/duplicacy-backup"
CONFIG_DIR="$INSTALL_ROOT/.config"
SECRETS_DIR="/root/.secrets"
APPLIED_CONFIG_GROUP=""

if [ "$(id -u)" -eq 0 ]; then
    if group_exists "$CONFIG_GROUP"; then
        APPLIED_CONFIG_GROUP="$CONFIG_GROUP"
    elif [ "$CONFIG_GROUP_EXPLICIT" -eq 1 ]; then
        fail "config group not found: $CONFIG_GROUP"
    fi
fi

mkdir -p "$INSTALL_ROOT" "$BIN_DIR" "$CONFIG_DIR"
chmod 755 "$INSTALL_ROOT" "$BIN_DIR"
chmod 750 "$CONFIG_DIR"
if [ -n "$APPLIED_CONFIG_GROUP" ]; then
    chown "root:$APPLIED_CONFIG_GROUP" "$CONFIG_DIR"
fi
for config_file in "$CONFIG_DIR"/*-backup.toml; do
    [ -f "$config_file" ] || continue
    chmod 640 "$config_file"
    if [ -n "$APPLIED_CONFIG_GROUP" ]; then
        chown "root:$APPLIED_CONFIG_GROUP" "$config_file"
    fi
done
if [ "$(id -u)" -eq 0 ]; then
    mkdir -p "$SECRETS_DIR"
    chown root:root "$SECRETS_DIR"
    chmod 700 "$SECRETS_DIR"
fi

TEMP_TARGET_PATH="$INSTALL_ROOT/.$BINARY_NAME.$$"
cp "$BINARY_PATH" "$TEMP_TARGET_PATH"
chmod 755 "$TEMP_TARGET_PATH"
mv -f "$TEMP_TARGET_PATH" "$TARGET_PATH"
TEMP_TARGET_PATH=""

if [ "$ACTIVATE" -eq 1 ]; then
    ln -sfn "$BINARY_NAME" "$CURRENT_LINK"
    ln -sfn "$CURRENT_LINK" "$STABLE_LINK"
fi

prune_old_binaries "$INSTALL_ROOT" "$KEEP"

echo "Installed: $TARGET_PATH"
if [ "$ACTIVATE" -eq 1 ]; then
    echo "Activated: $CURRENT_LINK -> $BINARY_NAME"
    echo "Stable command path: $STABLE_LINK"
else
    echo "Installed without activation. Re-run without --no-activate to switch current."
fi
if [ "$KEEP" -gt 0 ]; then
    echo "Retention policy: keeping newest $KEEP installed binaries"
fi
echo "Default config directory: $CONFIG_DIR"
if [ -n "$APPLIED_CONFIG_GROUP" ]; then
    echo "Config access: $CONFIG_DIR owned by root:$APPLIED_CONFIG_GROUP (750); existing *-backup.toml files normalised to 640"
else
    echo "Config access: $CONFIG_DIR set to 750; existing *-backup.toml files normalised to 640"
fi
if [ "$(id -u)" -eq 0 ]; then
    echo "Secrets directory: $SECRETS_DIR ensured as root:root (700); secrets files are not modified"
else
    echo "Secrets directory: $SECRETS_DIR (not modified; run installer as root to create or normalise it)"
fi
echo "Scheduled tasks should call: $STABLE_LINK"
echo "Rollback hint: ln -sfn <older-binary-name> $CURRENT_LINK"
