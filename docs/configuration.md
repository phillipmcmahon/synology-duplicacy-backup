# Configuration and Secrets

## Config File Location

By default the binary resolves config files relative to the executable:

```text
<binary-dir>/.config/<source>-backup.conf
```

Example:

```text
/usr/local/bin/duplicacy-backup
/usr/local/bin/.config/homes-backup.conf
```

Overrides:

- `--config-dir <path>`
- `DUPLICACY_BACKUP_CONFIG_DIR`

## Config File Format

Config files use INI-style sections:

- `[common]`
- `[local]`
- `[remote]`

The active runtime loads `[common]` plus either `[local]` or `[remote]`.

## Config Keys

| Key | Required | Description |
|---|---|---|
| `DESTINATION` | Yes | Backup destination path or S3 URL |
| `THREADS` | Yes for backup | Duplicacy threads; power of 2, max 16 |
| `PRUNE` | Yes for prune | Duplicacy prune retention arguments |
| `FILTER` | No | Duplicacy filter patterns |
| `LOCAL_OWNER` | Yes when `--fix-perms` is used locally | Non-root local owner |
| `LOCAL_GROUP` | Yes when `--fix-perms` is used locally | Local group |
| `LOG_RETENTION_DAYS` | No | Log retention days; default `30` |
| `SAFE_PRUNE_MAX_DELETE_PERCENT` | No | Default `10` |
| `SAFE_PRUNE_MAX_DELETE_COUNT` | No | Default `25` |
| `SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT` | No | Default `20` |

## Example Config

```ini
[common]
PRUNE=-keep 1:728 -keep 91:364 -keep 28:182 -keep 7:28
FILTER=e:^(.*/)?(@eaDir|#recycle|tmp|exclude)/$|^(.*/)?(\.DS_Store|\._.*|Thumbs\.db)$
LOG_RETENTION_DAYS=30

[local]
DESTINATION=/volume2/backups
THREADS=4
LOCAL_OWNER=myuser
LOCAL_GROUP=users

[remote]
DESTINATION=s3://gateway.storjshare.io/my-backup-bucket
THREADS=8
```

## Secrets

Remote mode loads secrets from:

```text
/root/.secrets/duplicacy-<label>.env
```

Overrides:

- `--secrets-dir <path>`
- `DUPLICACY_BACKUP_SECRETS_DIR`

Example:

```env
STORJ_S3_ID=your-access-key-id
STORJ_S3_SECRET=your-secret-access-key
```

Requirements:

- owned by `root:root`
- permissions `0600`
- only `STORJ_S3_ID` and `STORJ_S3_SECRET` are allowed
- `STORJ_S3_ID` must be at least 28 characters
- `STORJ_S3_SECRET` must be at least 53 characters

## Safe Prune Thresholds

| Threshold | Default | Description |
|---|---|---|
| Max delete count | 25 | Maximum revisions a prune may delete |
| Max delete percent | 10% | Maximum percentage of revisions a prune may delete |
| Min total for % check | 20 | Percentage threshold only applies at or above this total revision count |

Use `--force-prune` to override threshold enforcement.

## Conditional Validation

| Validation | Runs when |
|---|---|
| `duplicacy` binary check | backup or prune |
| `btrfs` binary check | backup |
| `LOCAL_OWNER` / `LOCAL_GROUP` validation | local `--fix-perms` |
| remote secrets loading | `--remote` |
