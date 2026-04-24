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
- the target name you want to use, such as `offsite-storj`
- the complete Duplicacy storage value for that target
- any storage credentials needed by that backend

Do not point restore commands directly at live data. This tool restores into a
separate drill workspace first. Copy-back to the live source is a later,
deliberate operator step.

Do not schedule `backup`, `prune`, `cleanup-storage`, or `fix-perms` on the
replacement NAS until the restore has been inspected and the live source path
is intentionally rebuilt.

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
/usr/local/lib/duplicacy-backup/.config/
/root/.secrets/
```

## 2. Create The Intended Source Root

The config needs a `source_path` even before you copy restored data back. On a
new NAS, set it to the intended final source root for the label.

Examples:

```text
/volume1/homes
/volume1/music
/volume1/source-plexaudio
```

Create it if it does not already exist:

```bash
sudo mkdir -p /volume1/homes
```

Restore execution will not write to this path. It uses the source volume to
derive a separate drill workspace such as:

```text
/volume1/restore-drills/homes-offsite-storj-20260424-070000-rev2403
```

## 3. Create The Backup Config

Create one config file per label:

```text
/usr/local/lib/duplicacy-backup/.config/<label>-backup.toml
```

For example:

```bash
sudo vi /usr/local/lib/duplicacy-backup/.config/homes-backup.toml
```

### Template: S3-Compatible Remote Storage

Use this for Storj gateway, Amazon S3, MinIO, RustFS, or another
S3-compatible Duplicacy backend.

```toml
label = "homes"
source_path = "/volume1/homes"

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

[targets.offsite-storj]
location = "remote"
storage = "s3://gateway.storjshare.io/my-backup-bucket/homes"
```

For Duplicacy `minio://` or `minios://`, keep the same shape and place the
complete Duplicacy storage value in `storage`:

```toml
[targets.offsite-minio]
location = "remote"
storage = "minios://minio.example.net:9000/my-backup-bucket/homes"
```

### Template: Path-Based Storage

Use this when the backup storage is mounted or attached to the new NAS as a
filesystem path.

```toml
label = "homes"
source_path = "/volume1/homes"

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

[targets.onsite-usb]
location = "local"
storage = "/volumeUSB1/usbshare/duplicacy/homes"
allow_local_accounts = true
local_owner = "myuser"
local_group = "users"
```

Set safe permissions on the config:

```bash
sudo chown root:administrators /usr/local/lib/duplicacy-backup/.config
sudo chmod 750 /usr/local/lib/duplicacy-backup/.config
sudo chown root:administrators /usr/local/lib/duplicacy-backup/.config/homes-backup.toml
sudo chmod 640 /usr/local/lib/duplicacy-backup/.config/homes-backup.toml
```

## 4. Create The Secrets File

Create one secrets file per label only when the selected target needs storage
credentials or notification tokens:

```text
/root/.secrets/<label>-secrets.toml
```

For S3-compatible storage, use Duplicacy's generic key names:

```toml
[targets.offsite-storj.keys]
s3_id = "your-access-key-id"
s3_secret = "your-secret-access-key"
```

For `minio://`, `minios://`, `s3://`, and `s3c://`, the key names are still:

```toml
s3_id = "..."
s3_secret = "..."
```

Path-based storage usually does not need storage secrets. If no target needs
credentials or notification tokens, you can skip the label secrets file.

Set safe permissions if you created a secrets file:

```bash
sudo mkdir -p /root/.secrets
sudo chown root:root /root/.secrets
sudo chmod 700 /root/.secrets
sudo chown root:root /root/.secrets/homes-secrets.toml
sudo chmod 600 /root/.secrets/homes-secrets.toml
```

## 5. Validate The Config And Repository

First confirm the tool resolves the target you expect:

```bash
sudo duplicacy-backup config explain --target offsite-storj homes
```

Check that the config, source path, storage settings, secrets, and repository
readiness are valid:

```bash
sudo duplicacy-backup config validate --target offsite-storj homes
```

You want to see:

```text
Config             : Valid
Repository Access  : Valid
Target Settings    : Valid
Result             : Passed
```

If repository access is `Not initialized`, stop and check the `storage` value.
On a replacement NAS you should be connecting to an existing repository, not
creating a new empty one.

If secrets are invalid, check that:

- the secrets file name matches the label
- the target name matches the config
- keys are under `[targets.<name>.keys]`
- S3-compatible backends use `s3_id` and `s3_secret`

## 6. Prove You Can See Restore Points

List available revisions without restoring data:

```bash
sudo duplicacy-backup restore list-revisions --target offsite-storj homes
```

This proves the new NAS can initialise a temporary Duplicacy workspace and read
the existing backup history.

You can also print the read-only restore plan:

```bash
sudo duplicacy-backup restore plan --target offsite-storj homes
```

The plan should show:

- the expected config file
- the intended source path
- the exact storage value
- the secrets file, if the backend needs one
- the safe drill workspace pattern

## 7. Inspect Before Restoring

Start the guided restore flow:

```bash
sudo duplicacy-backup restore select --target offsite-storj homes
```

Choose a restore point, then choose inspect-only first. This lets you browse
the revision contents without restoring data.

For very large repositories, start under a known subtree:

```bash
sudo duplicacy-backup restore select \
  --target offsite-storj \
  --path-prefix phillipmcmahon/code \
  homes
```

When you are ready to restore, use the same guided flow or the explicit
primitive:

```bash
sudo duplicacy-backup restore run \
  --target offsite-storj \
  --revision <revision> \
  --path "relative/path/or/pattern" \
  --yes \
  homes
```

Restored data goes into the drill workspace, not the live source path.

## New NAS Checklist

- Install `duplicacy-backup`.
- Confirm Duplicacy CLI is installed and on `PATH`.
- Create the intended `source_path` directory.
- Create `/usr/local/lib/duplicacy-backup/.config/<label>-backup.toml`.
- Create `/root/.secrets/<label>-secrets.toml` if the backend needs keys.
- Run `config explain`.
- Run `config validate` and confirm repository access is `Valid`.
- Run `restore list-revisions` and confirm restore points are visible.
- Run `restore select` in inspect-only mode before restoring.
- Restore into the drill workspace first.
- Copy back to the live source only after inspection.
