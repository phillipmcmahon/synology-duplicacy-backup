# Configuration and Secrets

## Config File Location

By default the binary resolves config files relative to the executable:

```text
<binary-dir>/.config/<label>-<target>-backup.toml
```

With the recommended installer layout from [`operations.md`](operations.md),
the effective default becomes:

```text
/usr/local/lib/duplicacy-backup/.config/homes-local-backup.toml
/usr/local/lib/duplicacy-backup/.config/homes-remote-backup.toml
```

If you are using the stable installer path:

```text
/usr/local/bin/duplicacy-backup
```

the config still resolves under:

```text
/usr/local/lib/duplicacy-backup/.config/
```

because `/usr/local/bin/duplicacy-backup` is a symlink to the real installed
binary under `/usr/local/lib/duplicacy-backup/`.

Overrides:

- `--config-dir <path>`
- `DUPLICACY_BACKUP_CONFIG_DIR`

## Config File Format

Each config file defines one label-target pair. The target model is explicit:

- top-level `label`
- top-level `source_path`
- `[target]`
- `[storage]`
- `[capture]`
- `[retention]`
- optional `[health]`
- optional `[notify]`

## Config Keys

| Key | Required | Description |
|---|---|---|
| `label` | Yes | Source label used on the CLI |
| `source_path` | Yes | Real source directory for this label |
| `target.name` | Yes | Target name such as `local` or `remote` |
| `target.type` | Yes | `local` or `remote` |
| `target.allow_local_accounts` | Needed for local owner/group operations | Explicitly allows local owner/group management |
| `storage.destination` | Yes | Backup destination path or S3-compatible gateway URL |
| `storage.repository` | No | Repository name; defaults to `label` |
| `capture.threads` | Yes for backup | Duplicacy threads; power of 2, max 16 |
| `capture.filter` | No | Duplicacy filter patterns |
| `retention.prune` or `retention.keep` | Yes for prune | Duplicacy retention policy |
| `retention.log_retention_days` | No | Log retention days; default `30` |
| `retention.safe_prune_max_delete_percent` | No | Default `10` |
| `retention.safe_prune_max_delete_count` | No | Default `25` |
| `retention.safe_prune_min_total_for_percent` | No | Default `20` |

## Health Policy

The optional `[health]` table controls read-only health checks:

| Key | Required | Description |
|---|---|---|
| `freshness_warn_hours` | No | Warn when the latest known successful backup is older than this |
| `freshness_fail_hours` | No | Fail when the latest known successful backup is older than this |
| `doctor_warn_after_hours` | No | Warn when `health doctor` has not been run recently |
| `verify_warn_after_hours` | No | Warn when `health verify` has not been run recently |

The optional `[notify]` table controls webhook notifications for
non-interactive health runs:

| Key | Required | Description |
|---|---|---|
| `webhook_url` | No | Webhook destination URL |
| `notify_on` | No | Statuses that should notify; defaults to `["degraded", "unhealthy"]` |
| `send_for` | No | Health commands that may notify; defaults to `["doctor", "verify"]` |
| `interactive` | No | Allow notifications from interactive TTY runs; defaults to `false` |

## Example Config

```toml
label = "homes"
source_path = "/volume1/homes"

[target]
name = "local"
type = "local"
allow_local_accounts = true
local_owner = "myuser"
local_group = "users"

[storage]
destination = "/volume2/backups"
repository = "homes"

[capture]
threads = 4
filter = "e:^(.*/)?(@eaDir|#recycle|tmp|exclude)/$|^(.*/)?(\\.DS_Store|\\._.*|Thumbs\\.db)$"

[retention]
keep = ["1:728", "91:364", "28:182", "7:28"]
log_retention_days = 30
safe_prune_max_delete_percent = 10
safe_prune_max_delete_count = 25
safe_prune_min_total_for_percent = 20

[health]
freshness_warn_hours = 30
freshness_fail_hours = 48
doctor_warn_after_hours = 48
verify_warn_after_hours = 168

[notify]
webhook_url = "https://example.invalid/hooks/duplicacy-backup"
notify_on = ["degraded", "unhealthy"]
send_for = ["doctor", "verify"]
interactive = false
```

## Secrets

Remote mode loads gateway credentials from:

```text
/root/.secrets/duplicacy-<label>-<target>.toml
```

Overrides:

- `--secrets-dir <path>`
- `DUPLICACY_BACKUP_SECRETS_DIR`

Example:

```toml
storj_s3_id = "your-access-key-id"
storj_s3_secret = "your-secret-access-key"
health_webhook_bearer_token = "optional-webhook-bearer-token"
```

Requirements:

- owned by `root:root`
- permissions `0600`
- only `storj_s3_id`, `storj_s3_secret`, and optional `health_webhook_bearer_token` are allowed
- `storj_s3_id` must be at least 28 characters
- `storj_s3_secret` must be at least 53 characters

The current schema uses `storj_s3_id` and `storj_s3_secret` because those
values are passed through to Duplicacy for gateway-backed remote storage.

## Safe Prune Thresholds

| Threshold | Default | Description |
|---|---|---|
| Max delete count | 25 | Maximum revisions a prune may delete |
| Max delete percent | 10% | Maximum percentage of revisions a prune may delete |
| Min total for % check | 20 | Percentage threshold only applies at or above this total revision count |

Use `--force-prune` to override threshold enforcement.

## Health State

Successful runs update a target-specific state file under:

```text
/var/lib/duplicacy-backup/<label>.<target>.json
```

`health status`, `health doctor`, and `health verify` combine this local state
with live Duplicacy storage inspection. When `duplicacy list` exposes revision
creation times, those storage timestamps are used as the authoritative
freshness signal.

## Conditional Validation

| Validation | Runs when |
|---|---|
| `duplicacy` binary check | backup or prune |
| `btrfs` binary check | backup |
| `local_owner` / `local_group` validation | local `--fix-perms` |
| target secrets loading | remote targets and any target that requires secrets |

## Current File Naming

The preferred operational layout is one file per label-target pair:

```text
homes-local-backup.toml
homes-remote-backup.toml
duplicacy-homes-remote.toml
```
