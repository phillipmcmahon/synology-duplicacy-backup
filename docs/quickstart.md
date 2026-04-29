# Quickstart

This is the shortest useful path from an installed binary to one validated
backup. It assumes a Synology DSM NAS, a Btrfs-backed source, and a target that
uses S3-compatible Duplicacy credentials.

## 1. Create One Config File

Create `$HOME/.config/duplicacy-backup/homes-backup.toml`:

```toml
label = "homes"
source_path = "/volume1/homes"

[common]
threads = 8
filter = "e:(?i)^(.*/)?(@eaDir|#recycle|tmp|exclude|\\.Trash(-[0-9]+)?)/$|(?i)^(.*/)?(\\.DS_Store|\\._.*|Thumbs\\.db)$"
prune = "-keep 91:728 -keep 28:364 -keep 7:182 -keep 1:28"

[health]
freshness_warn_hours = 24
freshness_fail_hours = 48
doctor_warn_after_hours = 48
verify_warn_after_hours = 168

[targets.offsite-s3]
location = "remote"
storage = "s3://s3.example.com/my-backup-bucket/homes"
```

Secure it:

```sh
mkdir -p "$HOME/.config/duplicacy-backup"
chmod 700 "$HOME/.config/duplicacy-backup"
chmod 600 "$HOME/.config/duplicacy-backup/homes-backup.toml"
```

## 2. Create One Secrets File

Create `$HOME/.config/duplicacy-backup/secrets/homes-secrets.toml`:

```toml
[targets.offsite-s3.keys]
s3_id = "replace-with-access-key"
s3_secret = "replace-with-secret-key"
```

Secure it:

```sh
mkdir -p "$HOME/.config/duplicacy-backup/secrets"
chmod 700 "$HOME/.config/duplicacy-backup/secrets"
chmod 600 "$HOME/.config/duplicacy-backup/secrets/homes-secrets.toml"
```

## 3. Validate

```sh
duplicacy-backup config validate --target offsite-s3 homes
```

Validation should report `Result : Passed`. If it fails, fix that before
running a backup.

## 4. Dry Run

```sh
sudo duplicacy-backup backup --target offsite-s3 --dry-run homes
```

Backup uses `sudo` because it creates a Btrfs snapshot and reads the full source
tree. Normal sudo metadata keeps config, secrets, logs, state, and locks rooted
under the operator profile rather than `/root`.

## 5. Real Backup

```sh
sudo duplicacy-backup backup --target offsite-s3 homes
```

After the first backup, check health:

```sh
duplicacy-backup health status --target offsite-s3 homes
```

For scheduling, restore drills, local filesystem repository rules, and update
workflow, continue with the [documentation index](README.md).
