# Configuration and Secrets

Use this reference when you are creating or reviewing label config files,
target definitions, health policy, notifications, and secrets. For copyable
daily commands, use the [operator cheat sheet](cheatsheet.md); for restore
procedure, use [restore-drills.md](restore-drills.md).

## Config File Location

By default, the binary resolves config files under the user profile:

```text
$HOME/.config/duplicacy-backup/<label>-backup.toml
```

When `XDG_CONFIG_HOME` is set, the default becomes:

```text
$XDG_CONFIG_HOME/duplicacy-backup/<label>-backup.toml
```

Overrides:

- `--config-dir <path>`
- `DUPLICACY_BACKUP_CONFIG_DIR`

Recommended permissions:

- config directory: owned by the operator user with mode `0700`
- config files: owned by the operator user with mode `0600`

The installer manages the binary only. It does not automatically move runtime
config or secrets. Release packages include `migrate-runtime-profile.sh` for
operators moving from the legacy root-era layout into the user-owned profile.
Run it with `--dry-run` first.

## Config File Format

Each config file defines one source label and one or more named targets.
The expected layout is:

- top-level `label`
- top-level `source_path`
- optional `[common]`
- optional `[health]`
- optional `[health.notify]`
- one or more `[targets.<name>]`
- optional `[targets.<name>.health]`
- optional `[targets.<name>.health.notify]`

## Target Model

Targets use two explicit fields:

- `storage` is the complete Duplicacy storage value
- `location` describes where the storage lives operationally

Supported locations:

- `location = "local"`
- `location = "remote"`

This means local disk paths, remote S3-compatible storage, native Duplicacy
backends, and local S3-compatible services such as RustFS or MinIO all use the
same `storage = "..."` shape. `location` is still important because it tells
operators where the target lives for scheduling, reporting, and permission
management decisions.

Operational rules:

- every target passes `storage` directly to Duplicacy
- do not split storage into `destination` and `repository`; include the full backend path in `storage`
- runtime keys live under `[targets.<name>.keys]` in the secrets file and are loaded for known Duplicacy backends that require them
- `allow_local_accounts`, `local_owner`, `local_group`, and `fix-perms` are only for path-based Duplicacy storage targets

Breaking change note:

- the `type` key has been retired because every target delegates storage to Duplicacy
- target-level `destination` and `repository` keys have been retired; use `storage`
- `requires_network` has been retired

## Migrating From The Old Target Schema

Older configs split storage into `type`, `destination`, and `repository`, and
Storj-over-S3 secrets used Storj-specific key names. The current schema gives
Duplicacy the complete storage value directly and stores backend keys under a
generic `[targets.<name>.keys]` table.

Before, in `homes-backup.toml`:

```toml
[targets.offsite-storj]
type = "object"
location = "remote"
destination = "s3://EU@gateway.storjshare.io/bucket-id"
repository = "homes"
```

Before, in `homes-secrets.toml`:

```toml
[targets.offsite-storj]
storj_s3_id = "..."
storj_s3_secret = "..."
```

After, in `homes-backup.toml`:

```toml
[targets.offsite-storj]
location = "remote"
storage = "s3://EU@gateway.storjshare.io/bucket-id/homes"
```

After, in `homes-secrets.toml`:

```toml
[targets.offsite-storj.keys]
s3_id = "..."
s3_secret = "..."
```

## Config Keys

| Key | Required | Description |
|---|---|---|
| `label` | Yes | Source label used on the CLI |
| `source_path` | Required for backup and full `config validate`; optional for restore-only DR access | Btrfs source root for this label; must be a snapshot-safe volume or subvolume when configured |
| `common.filter` | No | Default Duplicacy filter pattern |
| `common.threads` | Yes for backup unless set on the target | Duplicacy threads; power of 2, max 16 |
| `common.prune` | Yes for prune unless set on the target | Duplicacy prune policy |
| `common.log_retention_days` | No | Log retention days; default `30` |
| `common.safe_prune_max_delete_percent` | No | Default `10` |
| `common.safe_prune_max_delete_count` | No | Default `25` |
| `common.safe_prune_min_total_for_percent` | No | Default `20` |
| `targets.<name>.location` | Yes | Deployment location: `local` or `remote` |
| `targets.<name>.storage` | Yes | Complete Duplicacy storage value, including the repository/path component you want Duplicacy to use |
| `targets.<name>.filter` | No | Target-specific filter override |
| `targets.<name>.threads` | No | Target-specific thread override |
| `targets.<name>.prune` | No | Target-specific prune override |
| `targets.<name>.allow_local_accounts` | Needed for path-based owner/group operations | Explicitly allows local owner/group management |
| `targets.<name>.local_owner` | Needed for path-based `fix-perms` | Non-root owner to apply |
| `targets.<name>.local_group` | Needed for path-based `fix-perms` | Non-root group to apply |

## Health Policy

The optional `[health]` table controls read-only health checks:

| Key | Required | Description |
|---|---|---|
| `freshness_warn_hours` | No | Warn when the latest known successful backup is older than this |
| `freshness_fail_hours` | No | Fail when the latest known successful backup is older than this |
| `doctor_warn_after_hours` | No | Warn when `health doctor` has not been run recently |
| `verify_warn_after_hours` | No | Warn when `health verify` has not been run recently |

## Notifications

The optional `[health.notify]` table controls notifications for
non-interactive health runs and selected runtime failures. It can deliver to a
generic webhook destination, native `ntfy`, or both. Targets can override
these values under `[targets.<name>.health]` and
`[targets.<name>.health.notify]`:

| Key | Required | Description |
|---|---|---|
| `webhook_url` | No | Webhook destination URL |
| `[ntfy]` | No | Native `ntfy` delivery block |
| `notify_on` | No | Statuses that should notify; defaults to `["degraded", "unhealthy"]` |
| `send_for` | No | Commands or operations that may notify. Allowed values are `status`, `doctor`, `verify`, `backup`, `prune`, and `cleanup-storage`. Defaults to `["doctor", "verify"]` |
| `interactive` | No | Allow notifications from interactive TTY runs; defaults to `false` |

Runtime operations stay opt-in. Adding `backup`, `prune`, or
`cleanup-storage` to `send_for` enables notifications for those failure
events while preserving the default health-only behaviour for existing
configs.

Initial destination patterns:

- Synology scheduled-task email only:
  Good baseline for raw scheduled job failure with no extra services.
- Synology scheduled-task email plus native `ntfy`:
  Recommended low-cost setup for near-time operator alerts.
- Generic webhook destination:
  Suitable for future providers such as Slack, Discord, Node-RED, `n8n`, or a
  custom receiver.

Optional `[health.notify.ntfy]` keys:

| Key | Required | Description |
|---|---|---|
| `url` | No | Base `ntfy` URL; defaults to `https://ntfy.sh` |
| `topic` | Yes | `ntfy` topic name |

Notification authentication is target-scoped. If a webhook or `ntfy`
destination requires authentication, store `health_webhook_bearer_token` and/or
`health_ntfy_token` under the matching `[targets.<name>]` section in the
secrets file. If several targets notify to the same authenticated destination,
repeat the token in each target section that needs to send notifications.

Notification payloads are generic JSON, not vendor-specific message formats.
Every payload includes shared fields such as:
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
- `location`
- `status`

Health payloads also include `check` where relevant, runtime payloads include
`operation`, and event-specific structured context is carried under `details`.
That keeps the built-in generic output suitable for future providers such as
Discord, Slack, Node-RED, or `n8n` without hard-coding each one up front,
while native `ntfy` support covers the low-cost Synology path directly.

### Update Notifications

Self-update notifications are global application settings, not label or target
settings. Put them in:

```text
<config-dir>/duplicacy-backup.toml
```

Example:

```toml
[update.notify]
notify_on = ["failed"]
interactive = false

[update.notify.ntfy]
url = "https://ntfy.sh"
topic = "duplicacy-updates"
```

`notify_on` defaults to `["failed"]`. You can also opt into `succeeded`,
`current`, and `reinstall-requested` outcomes. Update notifications do not
read label storage secrets. Public `ntfy` topics and unauthenticated webhooks
therefore work without a `<label>-secrets.toml` file.

### Notification Signal Expectations

The v1 notification model keeps signal tight by default:

- `notify_on` defaults to `["degraded", "unhealthy"]`
- `send_for` defaults to `["doctor", "verify"]`
- runtime failures are opt-in
- update failures are opt-in through the global `[update.notify]` config
- interactive TTY runs do not notify unless `interactive = true`
- success events do not notify unless an update outcome such as `succeeded` is
  explicitly listed in `[update.notify].notify_on`

There is no built-in deduplication, reminder cadence, or escalation policy in
v1. If a scheduled run fails repeatedly and still matches your notification
policy, it will notify on each matching run. Keep scheduler frequency sensible
and use the receiving system's own suppression or grouping features if you
need more noise control.

### Simulated Notification Sends

`notify test` uses the configured label and target exactly as a real runtime or
health notification would, but it sends a clearly marked synthetic event with
`category = test`. `notify test update` uses the global update notification
config and simulates an update event without running the updater.

This is useful for validating:

- destination resolution for the selected target
- target-scoped notification auth
- provider reachability and acceptance
- global update notification routing, when using `notify test update`

It does not prove that:

- a real backup, prune, or health event has occurred
- a real self-update has occurred
- DSM scheduled-task email is working
- your normal `notify_on` and `send_for` policy would emit for a real event

Provider-specific expectations:

- native `ntfy` sends a real message to the configured topic using the same
  target-scoped token handling as live notifications; public topics can be
  tested without reading the label secrets file, but token-protected topics
  still need readable token access
- generic webhook sends the same generic JSON payload shape used by live
  notifications, leaving rendering and translation to the receiver

Recommended operator flow:

1. Confirm the selected target has the provider configured.
2. Start with `notify test --dry-run` if you want to inspect the resolved
   destinations and synthetic payload details without sending anything.
3. Run `notify test` without `--dry-run` to send the real synthetic message.
4. Confirm the message arrives in the receiving system.

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
location = "local"
storage = "/volume2/backups/homes"
allow_local_accounts = true
local_owner = "myuser"
local_group = "users"

[targets.offsite-usb]
location = "remote"
storage = "/volume1/duplicacy/duplicacy/homes"
allow_local_accounts = true
local_owner = "myuser"
local_group = "users"

[targets.offsite-storj]
location = "remote"
storage = "s3://gateway.storjshare.io/my-backup-bucket/homes"

[targets.offsite-storj.health]
verify_warn_after_hours = 336

[targets.onsite-rustfs]
location = "local"
storage = "s3://rustfs.local/my-backup-bucket/homes"

[targets.onsite-minio]
location = "local"
storage = "minio://garage@192.168.202.24:3900/garage/homes"
```

## Source Path Rule

`source_path` is the snapshot root for the label when the NAS is expected to
run backups. It must be a Btrfs volume or subvolume that can be used directly
as the source of a read-only snapshot.

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

Restore-only disaster recovery access can omit `source_path`. Restore commands
only need the label, target, storage value, and any storage secrets to read
existing Duplicacy data. Restore workspaces are derived from the restore job:
`/volume1/restore-drills/<label>-<target>-<restore-point-timestamp>-rev<id>`.
Use `--workspace-root` to place those derived job folders under an existing
operator-managed root, such as a Synology shared folder.
When `source_path` is omitted, restore output marks the live source as
unavailable and copy-back previews are disabled. Add `source_path` later when
the live source root has been rebuilt and the NAS is ready for backup
validation.

## Secrets

Known Duplicacy backends that require runtime storage keys load them from, and
authenticated notification delivery can optionally read target-scoped tokens
from:

```text
$HOME/.config/duplicacy-backup/secrets/<label>-secrets.toml
```

Overrides:

- `--secrets-dir <path>`
- `DUPLICACY_BACKUP_SECRETS_DIR`

Example:

```toml
[targets.offsite-storj]
health_webhook_bearer_token = "optional-webhook-bearer-token"
health_ntfy_token = "optional-ntfy-bearer-token"

[targets.offsite-storj.keys]
s3_id = "your-access-key-id"
s3_secret = "your-secret-access-key"

[targets.onsite-usb]
health_ntfy_token = "optional-ntfy-bearer-token"
```

Requirements:

- owned by the user running `duplicacy-backup`
- secrets directory permissions `0700`
- permissions `0600`
- storage keys live under `[targets.<name>.keys]` and are loaded for known Duplicacy backends that require them
- notification auth tokens may be present for any target
- a `[targets.<name>]` section may contain only `health_webhook_bearer_token` and/or `health_ntfy_token` when no storage credentials are needed for that target
- notification auth tokens are target-scoped; repeat them under each notifying target that needs authenticated delivery

Storage keys under `[targets.<name>.keys]` are passed through to Duplicacy as
runtime preference keys. Use the key names Duplicacy expects for the selected
storage value. S3-compatible Duplicacy schemes `s3://`, `s3c://`,
`minio://`, and `minios://` use `s3_id` and `s3_secret`:

```toml
[targets.onsite-minio.keys]
s3_id = "your-access-key-id"
s3_secret = "your-secret-access-key"
```

Native Duplicacy `storj://` storage uses Storj-specific Duplicacy keys:

```toml
[targets.offsite-storj-native.keys]
storj_key = "your-storj-access-grant"
storj_passphrase = "your-storj-passphrase"
```

The bundled installer does not create or rewrite secrets directories or files.

Path-based storage targets do not load storage keys.
They only need a matching secrets file if a notifying target uses
`health_webhook_bearer_token` and/or `health_ntfy_token`.

## Safe Prune Thresholds

| Threshold | Default | Description |
|---|---|---|
| Max delete count | 25 | Maximum revisions a prune may delete |
| Max delete percent | 10% | Maximum percentage of revisions a prune may delete |
| Min total for % check | 20 | Percentage threshold only applies at or above this total revision count |

Use `prune --force` to override threshold enforcement.

## Health State

Successful runs update a target-specific state file under:

```text
$HOME/.local/state/duplicacy-backup/state/<label>.<target>.json
```

When `XDG_STATE_HOME` is set, the state root is
`$XDG_STATE_HOME/duplicacy-backup/state`.

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
| target local-account consistency | `config validate`, path-based `fix-perms` |
| Btrfs `source_path` check | `config validate`, backup; not required for restore-only access |
| storage accessibility check | `config validate` |
| repository readiness probe | `config validate` |
| `local_owner` / `local_group` validation | path-based `fix-perms` |
| target secrets loading | selected storage scheme requires keys; validation then expects `[targets.<name>.keys]` in the secrets file |

## Output Model

Human-facing screens now make the selected target shape explicit:

- runtime headers show `Label`, `Target`, and `Location`
- health headers show `Check`, `Label`, `Target`, and `Location`
- `config explain` and `config paths` show `Location`
- `config explain` stays read-only by default and does not load storage secrets
- `config validate` includes `Privileges`, reported as `Full` or `Limited`

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

When `config validate` is run without the privileges needed for root-only
checks, lines such as `Btrfs Source`, `Secrets`, or `Repository Access` may be
reported as `Not checked` instead of failing the whole validation.

## Current File Naming

The preferred operational layout is one backup config file per label and, when
needed, one matching secrets file:

```text
homes-backup.toml
homes-secrets.toml
```

Every runtime, `config`, and `health` command requires an explicit
`--target <name>`.
