#!/bin/sh

set -eu

ci_fail() {
    echo "Error: $*" >&2
    exit 1
}

ci_require_root() {
    [ "$(id -u)" -eq 0 ] || ci_fail "this smoke script must run as root"
}

ci_ensure_dsm_marker() {
    mkdir -p /etc.defaults
    if [ ! -e /etc/synoinfo.conf ] && [ ! -e /etc.defaults/VERSION ]; then
        printf 'majorversion="7"\nminorversion="2"\n' >/etc.defaults/VERSION
    fi
}

ci_install_btrfs_tools() {
    if command -v btrfs >/dev/null 2>&1 && command -v mkfs.btrfs >/dev/null 2>&1; then
        return 0
    fi
    # GitHub-hosted Ubuntu runners usually need this install. It is intentionally
    # local to the btrfs smoke jobs so non-btrfs jobs do not pay the apt cost.
    apt-get update -qq
    apt-get install -y -qq btrfs-progs
}

ci_create_operator_user() {
    user="$1"
    if id "$user" >/dev/null 2>&1; then
        return 0
    fi
    if getent group "$user" >/dev/null 2>&1; then
        useradd -m -s /bin/bash -g "$user" "$user"
        return 0
    fi
    useradd -m -s /bin/bash "$user"
}

ci_allow_passwordless_sudo() {
    user="$1"
    # CAUTION: ephemeral CI runners only; never use this on persistent systems.
    printf '%s ALL=(root) NOPASSWD:ALL\n' "$user" >/etc/sudoers.d/duplicacy-backup-ci-"$user"
    chmod 0440 /etc/sudoers.d/duplicacy-backup-ci-"$user"
}

ci_install_binary() {
    src="$1"
    [ -x "$src" ] || ci_fail "binary not found or not executable: $src"
    install -m 0755 "$src" /usr/local/bin/duplicacy-backup
}

ci_install_fake_duplicacy() {
    cat >/usr/local/bin/duplicacy <<'EOF'
#!/bin/sh
echo "duplicacy ci stub"
EOF
    chmod 0755 /usr/local/bin/duplicacy
}

ci_mount_btrfs_volume1() {
    image="$1"
    mountpoint="/volume1"

    ci_install_btrfs_tools
    truncate -s 512M "$image"
    mkfs.btrfs -q -f "$image"
    mkdir -p "$mountpoint"
    mount -o loop "$image" "$mountpoint"
    touch "$mountpoint/.duplicacy-backup-ci-loopback"
    btrfs subvolume create "$mountpoint/source" >/dev/null
    mkdir -p "$mountpoint/duplicacy/homes"
}

ci_cleanup_btrfs_volume1() {
    image="${1:-}"
    mountpoint="/volume1"

    # Only unmount /volume1 when this smoke helper created the loopback mount.
    # This avoids touching a real NAS volume if someone runs the script locally.
    if [ -e "$mountpoint/.duplicacy-backup-ci-loopback" ]; then
        umount "$mountpoint" 2>/dev/null || true
    fi

    if [ -n "$image" ]; then
        if command -v losetup >/dev/null 2>&1; then
            losetup -j "$image" 2>/dev/null | awk -F: '{print $1}' | while IFS= read -r loopdev; do
                [ -n "$loopdev" ] || continue
                losetup -d "$loopdev" 2>/dev/null || true
            done
        fi
        rm -f "$image"
    fi
}

ci_write_local_config() {
    user="$1"
    target="$2"
    storage="$3"
    home="$(getent passwd "$user" | awk -F: '{print $6; exit}')"
    config_dir="$home/.config/duplicacy-backup"

    mkdir -p "$config_dir"
    cat >"$config_dir/homes-backup.toml" <<EOF
label = "homes"
source_path = "/volume1/source"

[common]
threads = 1
prune = "-keep 0:365"

[targets.$target]
location = "local"
storage = "$storage"
EOF
    chown -R "$user:$(id -gn "$user")" "$config_dir"
    find "$config_dir" -type d -exec chmod 0700 {} +
    find "$config_dir" -type f -exec chmod 0600 {} +
}

ci_write_remote_config_with_secrets() {
    user="$1"
    target="$2"
    home="$(getent passwd "$user" | awk -F: '{print $6; exit}')"
    config_dir="$home/.config/duplicacy-backup"
    secrets_dir="$config_dir/secrets"

    mkdir -p "$secrets_dir"
    cat >"$config_dir/homes-backup.toml" <<EOF
label = "homes"
source_path = "/volume1/source"

[common]
threads = 1
prune = "-keep 0:365"

[targets.$target]
location = "remote"
storage = "s3://EU@example.invalid/duplicacy-backup-ci/homes"
EOF
    cat >"$secrets_dir/homes-secrets.toml" <<EOF
[targets.$target.keys]
s3_id = "ABCDEFGHIJKLMNOPQRSTUVWXYZ01"
s3_secret = "abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR"
EOF
    chown -R "$user:$(id -gn "$user")" "$config_dir"
    find "$config_dir" -type d -exec chmod 0700 {} +
    find "$config_dir" -type f -exec chmod 0600 {} +
}
