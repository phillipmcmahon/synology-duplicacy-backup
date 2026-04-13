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

Recommended Synology permissions for the installed layout:

- config directory: `root:administrators` with mode `750`
- config files: `root:administrators` with mode `640`

The bundled installer applies that policy automatically to the default
`.config` directory and any existing `*-backup.toml` files when it is run as
`root`. Use `./install.sh --config-group <name>` if you want a different
trusted operator group.

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

## Target Model

Targets now use two explicit axes:

- `type` describes the storage mechanics
- `location` describes where the storage lives operationally

Supported combinations:

- `type = "filesystem"` with `location = "local"`
- `type = "filesystem"` with `location = "remote"`
- `type = "object"` with `location = "remote"`

This means a mounted filesystem path over VPN can be modelled honestly as
remote without forcing object-storage behaviour.

Operational rules:

- filesystem targets use filesystem path semantics
- object targets use URL-style storage semantics
- only object targets load secrets
- only filesystem targets allow `allow_local_accounts`, `local_owner`,
  `local_group`, and `--fix-perms`

Breaking change note:

- old `type = "local"` and `type = "remote"` values are no longer supported
- `requires_network` has been retired

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
| `targets.<name>.type` | Yes | Storage kind: `filesystem` or `object` |
| `targets.<name>.location` | Yes | Deployment location: `local` or `remote` |
| `targets.<name>.destination` | Yes unless inherited from `common` | Filesystem path for `filesystem` targets, or S3-compatible gateway URL for `object` targets |
| `targets.<name>.repository` | No | Repository name; defaults to `label` |
| `targets.<name>.filter` | No | Target-specific filter override |
| `targets.<name>.threads` | No | Target-specific thread override |
| `targets.<name>.prune` | No | Target-specific prune override |
| `targets.<name>.allow_local_accounts` | Needed for filesystem owner/group operations | Explicitly allows local owner/group management |
| `targets.<name>.local_owner` | Needed for filesystem `--fix-perms` | Non-root owner to apply |
| `targets.<name>.local_group` | Needed for filesystem `--fix-perms` | Non-root group to apply |

## Health Policy

The optional `[health]` table controls read-only health checks:

| Key | Required | Description |
|---|---|---|
| `freshness_warn_hours` | No | Warn when the latest known successful backup is older than this |
| `freshness_fail_hours` | No | Fail when the latest known successful backup is older than this |
| `doctor_warn_after_hours` | No | Warn when `health doctor` has not been run recently |
| `verify_warn_after_hours` | No | Warn when `health verify` has not been run recently |

The optional `[health.notify]` table controls notifications for
non-interactive health runs and selected runtime failures. It can deliver to a
generic webhook destination, native `ntfy`, or both. Targets may override
any of these values under `[targets.<name>.health]` and
`[targets.<name>.health.notify]`:

| Key | Required | Description |
|---|---|---|
| `webhook_url` | No | Webhook destination URL |
| `[ntfy]` | No | Native `ntfy` delivery block |
| `notify_on` | No | Statuses that should notify; defaults to `["degraded", "unhealthy"]` |
| `send_for` | No | Commands or operations that may notify. Allowed values are `status`, `doctor`, `verify`, `backup`, `prune`, and `cleanup-storage`. Defaults to `["doctor", "verify"]` |
| `interactive` | No | Allow notifications from interactive TTY runs; defaults to `false` |

Runtime operations stay opt-in. Adding `backup`, `prune`, or
`cleanup-storage` to `send_for` enables notification delivery for those failure
events while keeping the default health-only behaviour for existing configs.

Initial destination patterns:

- Synology scheduled-task email only
  Good baseline for raw scheduled job failure with no extra services.
- Synology scheduled-task email plus native `ntfy`
  Recommended low-cost setup for near-time operator alerts.
- Generic webhook destination
  Suitable for future providers such as Slack, Discord, Node-RED, `n8n`, or a
  custom receiver.

Optional `[health.notify.ntfy]` keys:

| Key | Required | Description |
|---|---|---|
| `url` | No | Base `ntfy` URL; defaults to `https://ntfy.sh` |
| `topic` | Yes | `ntfy` topic name |

Notification payloads are generic JSON, not vendor-specific payloads. Every payload
includes shared fields such as:
- `version`
- `event_id`
- `timestamp`
- `host`
- `severity`
- `category`
- `event`
- `summary`
- `label`
- `target`
- `storage_type`
- `location`
- `status`

Health payloads also include `check` where relevant, runtime payloads include
`operation`, and event-specific structured context is carried under `details`.
This keeps the built-in generic output suitable for future providers such as
Discord, Slack, Node-RED, or `n8n` without hard-coding every one of them up
front, while native `ntfy` support covers the low-cost Synology path directly.

### Notification Signal Expectations

The v1 notification model keeps signal tight by default:

- `notify_on` defaults to `["degraded", "unhealthy"]`
- `send_for` defaults to `["doctor", "verify"]`
- runtime failures are opt-in
- interactive TTY runs do not notify unless `interactive = true`
- success events do not notify

There is no built-in deduplication, reminder cadence, or escalation policy in
v1. If a scheduled run fails repeatedly and still matches your notification
policy, it will notify on each matching run. Keep scheduler frequency sensible
and use the receiving system's own suppression or grouping features if you need
further noise control.

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
send_for = ["doctor", "verify", "backup", "prune", "cleanup-storage"]
interactive = false

[health.notify.ntfy]
url = "https://ntfy.sh"
topic = "duplicacy-backup-alerts"

[targets.onsite-usb]
type = "filesystem"
location = "local"
destination = "/volume2/backups"
repository = "homes"
allow_local_accounts = true
local_owner = "myuser"
local_group = "users"

[targets.offsite-usb]
type = "filesystem"
location = "remote"
destination = "/volume1/duplicacy/duplicacy"
repository = "homes"
allow_local_accounts = true
local_owner = "AJT"
local_group = "users"

[targets.offsite-storj]
type = "object"
location = "remote"
destination = "s3://gateway.storjshare.io/my-backup-bucket"
repository = "homes"

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

Object targets load credentials from:

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
health_ntfy_token = "optional-ntfy-bearer-token"
```

Requirements:

- owned by `root:root`
- secrets directory permissions `0700`
- permissions `0600`
- storage credentials are only needed for object targets
- notification auth tokens may be present for any target
- a `[targets.<name>]` section may contain only `health_webhook_bearer_token` and/or `health_ntfy_token` when no storage credentials are needed for that target
- `storj_s3_id` must be at least 28 characters
- `storj_s3_secret` must be at least 53 characters

The current schema uses `storj_s3_id` and `storj_s3_secret` because those
values are passed through to Duplicacy for gateway-backed target storage.

When run as `root`, the bundled installer ensures `/root/.secrets` exists with
mode `700`, but it does not create or rewrite any individual secrets files.

Filesystem targets, whether local or remote, do not load secrets and therefore
do not need a matching secrets file.

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
| target local-account consistency | `config validate`, filesystem `--fix-perms` |
| Btrfs `source_path` check | `config validate`, backup |
| destination accessibility check | `config validate` |
| repository readiness probe | `config validate` |
| `local_owner` / `local_group` validation | filesystem `--fix-perms` |
| target secrets loading | `object` targets |

## Output Model

Human-facing screens now make the selected target shape explicit:

- runtime headers show `Label`, `Target`, `Type`, and `Location`
- health headers show `Check`, `Label`, `Target`, `Type`, and `Location`
- `config explain` and `config paths` show `Type` and `Location`

`config validate` intentionally keeps its `Resolved` section identity-only:

- `Label`
- `Target`
- `Config File`

The new target-model checks are surfaced under `Target Settings`.

`config validate` keeps repository probing read-only. It does not initialise
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
