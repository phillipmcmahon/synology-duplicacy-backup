# Configuration and Secrets

## Config File Location

By default the binary resolves config files relative to the executable:

```text
<binary-dir>/.config/<label>-backup.toml
```

With the recommended installer layout from [`operations.md`](operations.md),
the effective default becomes:

```text
/usr/local/lib/duplicacy-backup/.config/homes-backup.toml
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

Each config file defines one source label plus one or more named targets.
The preferred layout is:

- top-level `label`
- top-level `source_path`
- optional `[common]`
- optional `[health]`
- optional `[health.notify]`
- one or more `[targets.<name>]`
- optional `[targets.<name>.health]`
- optional `[targets.<name>.health.notify]`

## Config Keys

| Key | Required | Description |
|---|---|---|
| `label` | Yes | Source label used on the CLI |
| `source_path` | Yes | Btrfs source root for this label; must be a snapshot-safe volume or subvolume |
| `common.destination` | No | Default destination for targets that do not set their own |
| `common.filter` | No | Default Duplicacy filter pattern |
| `common.threads` | Yes for backup unless set on the target | Duplicacy threads; power of 2, max 16 |
| `common.prune` | Yes for prune unless set on the target | Duplicacy prune policy |
| `common.log_retention_days` | No | Log retention days; default `30` |
| `common.safe_prune_max_delete_percent` | No | Default `10` |
| `common.safe_prune_max_delete_count` | No | Default `25` |
| `common.safe_prune_min_total_for_percent` | No | Default `20` |
| `targets.<name>.type` | Yes | Internal target capability: `local` or `remote` |
| `targets.<name>.destination` | Yes unless inherited from `common` | Backup destination path or S3-compatible gateway URL |
| `targets.<name>.repository` | No | Repository name; defaults to `label` |
| `targets.<name>.filter` | No | Target-specific filter override |
| `targets.<name>.threads` | No | Target-specific thread override |
| `targets.<name>.prune` | No | Target-specific prune override |
| `targets.<name>.allow_local_accounts` | Needed for local owner/group operations | Explicitly allows local owner/group management |
| `targets.<name>.local_owner` | Needed for local `--fix-perms` | Non-root owner to apply |
| `targets.<name>.local_group` | Needed for local `--fix-perms` | Non-root group to apply |
| `targets.<name>.requires_network` | No | Explicitly marks network-dependent targets |

## Health Policy

The optional `[health]` table controls read-only health checks:

| Key | Required | Description |
|---|---|---|
| `freshness_warn_hours` | No | Warn when the latest known successful backup is older than this |
| `freshness_fail_hours` | No | Fail when the latest known successful backup is older than this |
| `doctor_warn_after_hours` | No | Warn when `health doctor` has not been run recently |
| `verify_warn_after_hours` | No | Warn when `health verify` has not been run recently |

The optional `[health.notify]` table controls webhook notifications for
non-interactive health runs. Targets may override any of these values under
`[targets.<name>.health]` and `[targets.<name>.health.notify]`:

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

[health.notify]
webhook_url = "https://example.invalid/hooks/duplicacy-backup"
notify_on = ["degraded", "unhealthy"]
send_for = ["doctor", "verify"]
interactive = false

[targets.onsite-usb]
type = "local"
destination = "/volume2/backups"
repository = "homes"
allow_local_accounts = true
local_owner = "myuser"
local_group = "users"

[targets.offsite-storj]
type = "remote"
destination = "s3://gateway.storjshare.io/my-backup-bucket"
repository = "homes"
requires_network = true

[targets.offsite-storj.health]
verify_warn_after_hours = 336
```

## Source Path Rule

`source_path` is the snapshot root for the label. It must be a Btrfs volume or
subvolume that can be used directly as the source of a read-only snapshot.

Good examples:

- `/volume1/source-homes`
- `/volume1/source-media-audio`

Bad example:

- `/volume1/source-homes/private-user-data`

That nested path may exist on Btrfs storage, but if it is not itself a Btrfs
volume or subvolume it cannot be used as the backup snapshot source.

When you need to include or exclude directories beneath the snapshot root, keep
`source_path` pointed at the real Btrfs root location and use Duplicacy
filters under `common.filter` or `targets.<name>.filter` to shape what is
actually backed up.

## Secrets

Targets that need credentials load them from:

```text
/root/.secrets/<label>-secrets.toml
```

Overrides:

- `--secrets-dir <path>`
- `DUPLICACY_BACKUP_SECRETS_DIR`

Example:

```toml
[targets.offsite-storj]
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
values are passed through to Duplicacy for gateway-backed target storage.

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

`health status`, `health doctor`, and `health verify` combine this target-specific state
with live Duplicacy storage inspection. When `duplicacy list` exposes revision
creation times, those storage timestamps are used as the authoritative
freshness signal.

## Conditional Validation

| Validation | Runs when |
|---|---|
| `duplicacy` binary check | backup or prune |
| `btrfs` binary check | backup |
| threads validation | `config validate`, backup |
| prune policy syntax validation | `config validate`, prune |
| target local-account consistency | `config validate`, local `--fix-perms` |
| Btrfs `source_path` check | `config validate`, backup |
| destination accessibility check | `config validate` |
| repository readiness probe | `config validate` |
| `local_owner` / `local_group` validation | local `--fix-perms` |
| target secrets loading | targets that require secrets |

`config validate` keeps repository probing read-only. It does not initialize
storage, create repositories, or modify config/state. Repository readiness is
reported as:

- `Repository Access : Valid`
- `Repository Access : Not initialized`
- `Repository Access : Invalid (...)`

## Current File Naming

The preferred operational layout is one config file per label plus
one secrets file per label:

```text
homes-backup.toml
homes-secrets.toml
```

Every runtime, `config`, and `health` command requires an explicit
`--target <name>`.
