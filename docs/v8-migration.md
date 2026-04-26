# v8 Migration Guide

## Purpose

Version 8.0.0 is a breaking operational upgrade. The application now defaults
to an operator-owned runtime profile instead of root/system-owned config,
secrets, logs, state, and lock paths.

This guide is for an existing NAS upgrading from a pre-v8 install.

## What Changed

Before v8, many commands commonly depended on root-owned paths such as:

```text
/usr/local/lib/duplicacy-backup/.config
/root/.secrets
/var/log
/var/lib/duplicacy-backup
/var/lock
```

From v8 onward, default runtime paths are under the invoking user's profile:

```text
$HOME/.config/duplicacy-backup
$HOME/.config/duplicacy-backup/secrets
$HOME/.local/state/duplicacy-backup
```

Root is still required for operations that need privileged OS access, such as
backup snapshots, `fix-perms`, and managed install activation. Restore, health,
diagnostics, config, notify, prune, and cleanup can run as the operator user
when that user can access the selected config, secrets, storage, state, logs,
locks, and restore workspace.

## Important Packaging Detail

`duplicacy-backup update` updates the managed binary only. It does not install
helper scripts into `/usr/local/lib/duplicacy-backup/current`.

The migration helper is packaged inside the release tarball:

```text
duplicacy-backup_8.0.0_linux_amd64.tar.gz
  duplicacy-backup_8.0.0_linux_amd64/
    duplicacy-backup_8.0.0_linux_amd64
    install.sh
    migrate-runtime-profile.sh
    LICENSE
    README.md
```

To migrate, extract the v8 tarball and run the helper from the extracted
directory. Use `sh` to run the script so the command also works on filesystems
mounted with `noexec`.

## 1. Upgrade The Managed Binary

Managed install activation writes under `/usr/local`, so run update with
`sudo`:

```bash
sudo duplicacy-backup update --attestations required
```

Confirm the installed version:

```bash
duplicacy-backup --version
```

Expected version:

```text
v8.0.0
```

## 2. Extract The Release Package

If the release has been mirrored to the NAS, use the mirrored package:

```bash
MIRROR=/volume1/homes/phillipmcmahon/code/duplicacy-backup/latest/v8.0.0
PKG="$MIRROR/duplicacy-backup_8.0.0_linux_amd64.tar.gz"
WORK=/volume1/homes/phillipmcmahon/exclude/testing/v8-migration

mkdir -p "$WORK"
tar -xzf "$PKG" -C "$WORK"

PKGDIR="$WORK/duplicacy-backup_8.0.0_linux_amd64"
ls -al "$PKGDIR"
```

If you use a temporary directory instead, remember that some NAS `/tmp`
mounts may prevent direct execution. Running the helper through `sh` avoids
that problem.

## 3. Dry-Run The Migration

Run this from any directory after setting `PKGDIR`:

```bash
sudo sh "$PKGDIR/migrate-runtime-profile.sh" \
  --target-user phillipmcmahon \
  --dry-run
```

The dry run should show:

- target user
- target group
- target home
- destination config directory
- destination secrets directory
- preflight result
- planned `mkdir`, `chmod`, `cp`, and `chown` operations

By default, the helper copies:

```text
/usr/local/lib/duplicacy-backup/.config/*.toml
  -> $HOME/.config/duplicacy-backup/

/root/.secrets/*.toml
  -> $HOME/.config/duplicacy-backup/secrets/
```

It creates destination directories with mode `0700`, copies TOML files with
source timestamps preserved where supported, then sets migrated files to mode
`0600`. When run as root, it sets migrated directories and files to the target
user and that user's primary group.

## 4. Copy Into The Operator Profile

If the dry run looks correct, copy the files:

```bash
sudo sh "$PKGDIR/migrate-runtime-profile.sh" \
  --target-user phillipmcmahon
```

This leaves the legacy files in place. That is the safest first migration
step.

If the helper reports destination collisions, no files are copied or moved.
Review the listed destination files, then either move them aside or rerun with
`--force` only when overwriting is intentional.

If you already migrated with an earlier v8 helper and see mixed groups such as
`root` or `administrators`, normalize the migrated profile explicitly:

```bash
TARGET_USER=phillipmcmahon
TARGET_GROUP="$(id -gn "$TARGET_USER")"
PROFILE="/var/services/homes/$TARGET_USER/.config/duplicacy-backup"

sudo chown -R "$TARGET_USER:$TARGET_GROUP" "$PROFILE"
sudo find "$PROFILE" -type d -exec chmod 700 {} +
sudo find "$PROFILE" -type f -exec chmod 600 {} +
```

The parent `.config` directory may be managed by Synology ACLs. The important
runtime profile is the `duplicacy-backup` directory and its contents.

## 5. Validate As The Operator User

Run these without `sudo`:

```bash
duplicacy-backup config paths --target onsite-usb homes
duplicacy-backup config validate --target onsite-usb homes
duplicacy-backup diagnostics --target onsite-usb homes
duplicacy-backup health status --target onsite-usb homes
```

Check that the reported config and secrets paths are under:

```text
$HOME/.config/duplicacy-backup
$HOME/.config/duplicacy-backup/secrets
```

For restore validation:

```bash
duplicacy-backup restore select --target onsite-usb homes
```

For backup validation, keep using `sudo` because backup snapshots require
privileged filesystem access:

```bash
sudo duplicacy-backup backup --target onsite-usb --dry-run homes
```

## 6. Update Scheduled Tasks

Review Synology Task Scheduler or cron entries after migration.

Keep `sudo` or root scheduling only for commands that need OS privilege:

- `backup`
- `fix-perms`
- managed `update` activation
- managed `rollback` activation

Prefer the operator user for non-root-capable commands:

- `restore select`
- `restore run`
- `health status`
- `health doctor`
- `health verify`
- `diagnostics`
- `config`
- `notify test`
- `prune`, when storage ownership allows it
- `cleanup-storage`, when storage ownership allows it

If a local path-based storage tree was created by a root-era install, prune or
cleanup may still need storage ownership repair before running as the operator
user.

## 7. Optional: Move Legacy Files

Only after successful validation, remove legacy source files as part of the
migration:

```bash
sudo sh "$PKGDIR/migrate-runtime-profile.sh" \
  --target-user phillipmcmahon \
  --move
```

Use `--move` only when you are confident the operator-profile config and
secrets are correct.

## 8. Rollback Considerations

`duplicacy-backup rollback` can reactivate a retained pre-v8 binary, but it
does not move config or secrets back to root-era paths.

If you need to roll back the binary after migrating runtime files, either:

- pass explicit `--config-dir` and `--secrets-dir` values supported by the
  older binary, or
- keep the legacy files in place until the v8 migration has been validated.

For that reason, prefer copy mode first. Use `--move` only after confidence is
high.

## Troubleshooting

If direct script execution fails with `Permission denied`, run it through `sh`:

```bash
sudo sh "$PKGDIR/migrate-runtime-profile.sh" --target-user phillipmcmahon --dry-run
```

If config is not found after migration, confirm the command is being run as the
operator user, not through stale `sudo` muscle memory:

```bash
whoami
duplicacy-backup config paths --target onsite-usb homes
```

If a command still needs root-owned config temporarily, pass explicit paths:

```bash
sudo duplicacy-backup config paths \
  --config-dir /usr/local/lib/duplicacy-backup/.config \
  --secrets-dir /root/.secrets \
  --target onsite-usb homes
```

Treat this as a temporary diagnostic path, not the v8 steady state.
