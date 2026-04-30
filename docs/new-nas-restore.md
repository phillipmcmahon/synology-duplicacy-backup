# Restore Onto A New NAS

Use this guide when the original NAS is unavailable and you need to connect a
replacement Synology NAS to existing Duplicacy backup storage.

The goal is not to start scheduled backups. The goal is to:

- install `duplicacy-backup`
- create the minimum config and secrets files
- prove the existing backup repository is reachable
- choose a restore point safely

After those checks pass, use [restore-drills.md](restore-drills.md) for the
actual restore workflow.

## Before You Start

You need:

- root access on the new NAS
- the Duplicacy CLI installed and available on `PATH`
- the correct release tarball for the NAS architecture
- the original backup label name, such as `homes`
- the storage name you want to use, such as `offsite-storj`
- the complete Duplicacy storage value for that storage entry
- any storage credentials needed by that backend

Do not point restore commands directly at live data. This tool restores into a
separate drill workspace first. Copy-back to the live source is a later,
deliberate operator step.

Do not schedule `backup`, `prune`, or `cleanup-storage` on the replacement NAS
until the restore has been inspected and the live source path is intentionally
rebuilt.

## 1. Install The Tool

Download the matching release tarball from GitHub on another machine or
directly on the NAS:

```text
duplicacy-backup_<version>_linux_amd64.tar.gz
duplicacy-backup_<version>_linux_arm64.tar.gz
duplicacy-backup_<version>_linux_armv7.tar.gz
```

Extract it and run the installer:

```bash
tar -xzf duplicacy-backup_<version>_linux_<arch>.tar.gz
cd duplicacy-backup_<version>_linux_<arch>
sudo ./install.sh
```

Confirm the installed command works:

```bash
/usr/local/bin/duplicacy-backup --version
/usr/local/bin/duplicacy-backup --help
```

The default installed paths are:

```text
/usr/local/bin/duplicacy-backup
$HOME/.config/duplicacy-backup/
$HOME/.config/duplicacy-backup/secrets/
```

## 2. Decide Whether You Know The Future Source Root

Restore access does not require the original `source_path`. In a disaster
recovery situation, the important first step is proving that the new NAS can
read the backup repository.

If you know the future live source root, you can include `source_path` in the
config. This gives the tool better copy-back context and lets `config validate`
perform the full source-path readiness check.

Examples:

```text
/volume1/homes
/volume1/music
/volume1/source-plexaudio
```

Create it only when you are intentionally rebuilding that live source root:

```bash
sudo mkdir -p /volume1/homes
```

Restore execution will not write to `source_path`. It restores into a separate
drill workspace derived from the restore job:

```text
/volume1/restore-drills/homes-offsite-storj-20260424-070000-rev2403
```

If `source_path` is omitted, restore commands still work. `source_path` is only
live-source and copy-back context. Provide `--workspace-root` when you want the
derived restore-job folder under a Synology shared folder you choose. Create
that root folder first so its DSM permissions and ownership remain
operator-managed. Provide `--workspace` only when you want to override the
derived workspace with an exact path.

## 3. Create The Backup Config

Create one config file per label:

```text
$HOME/.config/duplicacy-backup/<label>-backup.toml
```

For example:

```bash
mkdir -p "$HOME/.config/duplicacy-backup"
vi "$HOME/.config/duplicacy-backup/homes-backup.toml"
```

### Template: S3-Compatible Remote Storage

Use this for Storj gateway, Amazon S3, MinIO, RustFS, or another
S3-compatible Duplicacy backend.

```toml
label = "homes"
# Optional for restore-only DR access.
# Add this when the future live source root is known.
# source_path = "/volume1/homes"

[common]
threads = 4
filter = "e:^(.*/)?(@eaDir|#recycle|tmp|exclude)/$|^(.*/)?(\\.DS_Store|\\._.*|Thumbs\\.db)$"
prune = "-keep 1:728 -keep 91:364 -keep 28:182 -keep 7:28"
log_retention_days = 30
safe_prune_max_delete_percent = 10
safe_prune_max_delete_count = 25
safe_prune_min_total_for_percent = 20

[health]
freshness_warn_hours = 30
freshness_fail_hours = 48
doctor_warn_after_hours = 48
verify_warn_after_hours = 168

[storage.offsite-storj]
location = "remote"
storage = "s3://gateway.storjshare.io/my-backup-bucket/homes"
```

For Duplicacy `minio://` or `minios://`, keep the same shape and place the
complete Duplicacy storage value in `storage`:

```toml
[storage.offsite-minio]
location = "remote"
storage = "minios://minio.example.net:9000/my-backup-bucket/homes"
```

### Template: Path-Based Storage

Use this when the backup storage is mounted or attached to the new NAS as a
filesystem path.

```toml
label = "homes"
# Optional for restore-only DR access.
# Add this when the future live source root is known.
# source_path = "/volume1/homes"

[common]
threads = 4
filter = "e:^(.*/)?(@eaDir|#recycle|tmp|exclude)/$|^(.*/)?(\\.DS_Store|\\._.*|Thumbs\\.db)$"
prune = "-keep 1:728 -keep 91:364 -keep 28:182 -keep 7:28"
log_retention_days = 30
safe_prune_max_delete_percent = 10
safe_prune_max_delete_count = 25
safe_prune_min_total_for_percent = 20

[health]
freshness_warn_hours = 30
freshness_fail_hours = 48
doctor_warn_after_hours = 48
verify_warn_after_hours = 168

[storage.onsite-usb]
location = "local"
storage = "/volumeUSB1/usbshare/duplicacy/homes"
```

Set safe permissions on the config:

```bash
chmod 700 "$HOME/.config/duplicacy-backup"
chmod 600 "$HOME/.config/duplicacy-backup/homes-backup.toml"
```

## 4. Create The Secrets File

Create one secrets file per label only when the selected storage needs storage
credentials or notification tokens:

```text
$HOME/.config/duplicacy-backup/secrets/<label>-secrets.toml
```

For S3-compatible storage, use Duplicacy's generic key names:

```toml
[storage.offsite-storj.keys]
s3_id = "your-access-key-id"
s3_secret = "your-secret-access-key"
```

For `minio://`, `minios://`, `s3://`, and `s3c://`, the key names are still:

```toml
s3_id = "..."
s3_secret = "..."
```

For native Duplicacy `storj://` storage, use:

```toml
[storage.offsite-storj-native.keys]
storj_key = "your-storj-access-grant"
storj_passphrase = "your-storj-passphrase"
```

Path-based storage usually does not need storage secrets. If no storage entry needs
credentials or notification tokens, you can skip the label secrets file.

Set safe permissions if you created a secrets file:

```bash
mkdir -p "$HOME/.config/duplicacy-backup/secrets"
chmod 700 "$HOME/.config/duplicacy-backup/secrets"
chmod 600 "$HOME/.config/duplicacy-backup/secrets/homes-secrets.toml"
```

## 5. Check The Config Shape

First confirm the tool resolves the storage entry you expect:

```bash
duplicacy-backup config explain --storage offsite-storj homes
```

If you configured `source_path` and created that future live source root, you
can also run full backup-readiness validation:

```bash
duplicacy-backup config validate --storage offsite-storj homes
```

You want to see:

```text
Config             : Valid
Repository Access  : Valid
Storage Settings    : Valid
Result             : Passed
```

If `source_path` is intentionally omitted, `config validate` may fail the
source-path check. That does not mean restore access is blocked. Continue with
`restore list-revisions`, which is the restore-specific repository connection
test.

If repository access is `Not initialized`, stop and check the `storage` value.
On a replacement NAS you should be connecting to an existing repository, not
creating a new empty one.

If secrets are invalid, check that:

- the secrets file name matches the label
- the storage name matches the config
- keys are under `[storage.<name>.keys]`
- S3-compatible backends use `s3_id` and `s3_secret`
- native `storj://` storage uses `storj_key` and `storj_passphrase`

Print a redacted diagnostics bundle before the first restore or backup on the
new NAS:

```bash
duplicacy-backup diagnostics --storage offsite-storj homes
```

Use this as a final sanity check that the new NAS is reading the intended
config file, storage name, storage value, secrets file, state directory, and runtime
paths. It is non-mutating and safe to paste into a support conversation because
secret values are redacted.

## 6. Prove You Can See Restore Points

List available revisions without restoring data:

```bash
duplicacy-backup restore list-revisions --storage offsite-storj homes
```

This proves the new NAS can initialise a temporary Duplicacy workspace and read
the existing backup history.

You can also print the read-only restore plan:

```bash
duplicacy-backup restore plan --storage offsite-storj homes
```

The plan should show:

- the expected config file
- the intended source path, or a clear restore-only message if it is omitted
- the exact storage value
- the secrets file, if the backend needs one
- the safe drill workspace pattern

## 7. Inspect Before Restoring

Start the guided restore flow:

```bash
duplicacy-backup restore select --storage offsite-storj homes
```

Choose a restore point, then choose inspect-only first. This lets you browse
the revision contents without restoring data.
Use `q` at restore-select prompts or inside the tree picker to cancel before
execution. During an active restore, `Ctrl-C` cancels the running Duplicacy
process, keeps the drill workspace, does not delete restored files, and reports
progress.

For very large repositories, start under a known subtree:

```bash
duplicacy-backup restore select \
  --storage offsite-storj \
  --path-prefix phillipmcmahon/code \
  homes
```

When you are ready to restore, use the same guided flow or the explicit
primitive:

```bash
duplicacy-backup restore run \
  --storage offsite-storj \
  --revision <revision> \
  --path "relative/path/or/pattern" \
  --yes \
  homes
```

Restored data goes into the drill workspace, not the live source path.

## New NAS Checklist

- Install `duplicacy-backup`.
- Confirm Duplicacy CLI is installed and on `PATH`.
- Create `$HOME/.config/duplicacy-backup/<label>-backup.toml`.
- Create `$HOME/.config/duplicacy-backup/secrets/<label>-secrets.toml` if the backend needs keys.
- Run `config explain`.
- Optional: add `source_path`, create the future live source root, then run
  `config validate` for full backup-readiness checks.
- Run `restore list-revisions` and confirm restore points are visible.
- Run `restore select` in inspect-only mode before restoring.
- Restore into the drill workspace first.
- Copy back to the live source only after inspection.
