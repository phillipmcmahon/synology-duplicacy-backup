# CLI Reference

## Usage

```text
duplicacy-backup [OPTIONS] <source>
duplicacy-backup config <validate|explain|paths> [OPTIONS] <source>
duplicacy-backup notify <test> [OPTIONS] <source>
duplicacy-backup update [OPTIONS]
duplicacy-backup health <status|doctor|verify> [OPTIONS] <source>
```

Every runtime, `config`, `notify`, and `health` command needs an explicit
`--target <name>`.
Every runtime command also needs at least one explicit primary operation.

Targets now describe both storage kind and deployment location:

- `type = "filesystem"` or `type = "object"`
- `location = "local"` or `location = "remote"`

Supported combinations are:

- filesystem/local
- filesystem/remote
- object/remote

Primary operations may be combined. When they are, execution order is fixed:

1. `--backup`
2. `--prune`
3. `--cleanup-storage`
4. `--fix-perms`

## Primary Operations

| Flag | Description |
|---|---|
| `--backup` | Request the backup phase |
| `--prune` | Request the threshold-guarded prune phase |
| `--cleanup-storage` | Run exhaustive exclusive storage cleanup |
| `--fix-perms` | Normalise filesystem repository ownership and permissions; can run alone or after backup/prune |

## Modifiers

| Flag | Description |
|---|---|
| `--force-prune` | Override safe prune thresholds, turning prune into forced prune |
| `--target <name>` | Use the named target config; required for every command |
| `--dry-run` | Simulate actions without making changes |
| `--verbose` | Show detailed operational logging and command details |
| `--json-summary` | Write a machine-readable run summary to stdout |
| `--config-dir <path>` | Override the config directory |
| `--secrets-dir <path>` | Override the secrets directory |
| `--version`, `-v` | Show version and build information |
| `--help` | Show help |
| `--help-full` | Show the detailed help reference |

## Config Commands

| Command | Description |
|---|---|
| `config validate --target <target> <label>` | Validate the selected target from that label config and any required object-target secrets |
| `config explain --target <target> <label>` | Show resolved config values for the selected target from that label config |
| `config paths --target <target> <label>` | Show resolved stable config, source, log, and any applicable secrets paths |

## Notify Commands

| Command | Description |
|---|---|
| `notify test --target <target> <label>` | Send a simulated notification through the configured destinations for the selected target |
| `notify test update` | Send a simulated update notification through the global update notification config |

## Update Command

| Command | Description |
|---|---|
| `update [--check-only]` | Check GitHub for the latest published release and, when requested, install it through the packaged installer |

## Health Commands

| Command | Description |
|---|---|
| `health status --target <target> <label>` | Fast read-only health summary for operators and schedulers |
| `health doctor --target <target> <label>` | Read-only environment and storage diagnostic pass |
| `health verify --target <target> <label>` | Read-only integrity check across revisions found for the current label |

## Environment Variables

| Variable | Description |
|---|---|
| `DUPLICACY_BACKUP_CONFIG_DIR` | Override config directory unless `--config-dir` is provided |
| `DUPLICACY_BACKUP_SECRETS_DIR` | Override secrets directory unless `--secrets-dir` is provided |

## Examples

These examples show valid CLI combinations for manual and ad hoc use. For
recommended recurring Synology scheduling patterns, see
[`workflow-scheduling.md`](workflow-scheduling.md) and keep backup, prune,
health, and fix-perms as separate scheduled tasks by default.

```bash
# Backup homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup homes

# Backup then safe prune homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup --prune homes

# Forced prune for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --prune --force-prune homes

# Storage cleanup for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --cleanup-storage homes

# Backup, safe prune, and storage cleanup for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup --prune --cleanup-storage homes

# Backup, forced prune, and storage cleanup for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup --prune --force-prune --cleanup-storage homes

# Fix permissions for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --fix-perms homes

# Backup then fix permissions for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup --fix-perms homes

# Backup homes to target offsite-usb mounted over VPN
sudo duplicacy-backup --target offsite-usb --backup homes

# Fix permissions for homes on target offsite-usb
sudo duplicacy-backup --target offsite-usb --fix-perms homes

# Backup, safe prune, and fix permissions for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --prune --backup --fix-perms homes

# Preview backing up homes to target offsite-storj
sudo duplicacy-backup --target offsite-storj --dry-run --backup homes

# Verbose backup and prune for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --verbose --backup --prune homes

# JSON summary for a dry-run backup of homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --json-summary --dry-run --backup homes

# Validate config for homes on target onsite-usb
sudo duplicacy-backup config validate --target onsite-usb homes

# Explain config for homes on target offsite-storj
sudo duplicacy-backup config explain --target offsite-storj homes

# Show paths for homes on target onsite-usb
duplicacy-backup config paths --target onsite-usb homes

# Fast health summary for homes on target onsite-usb
sudo duplicacy-backup health status --target onsite-usb homes

# JSON doctor report for homes on target onsite-usb
sudo duplicacy-backup health doctor --json-summary --target onsite-usb homes

# Send a simulated notification through the configured providers for homes on target onsite-usb
sudo duplicacy-backup notify test --target onsite-usb homes

# Preview the global update notification route without running an update
duplicacy-backup notify test update --provider ntfy --dry-run

# Verify homes on target offsite-storj
sudo duplicacy-backup health verify --target offsite-storj homes

# Check whether a newer published release is available
/usr/local/bin/duplicacy-backup update --check-only

# Download and install the latest published release
sudo /usr/local/bin/duplicacy-backup update --yes

# Reinstall the selected release even if it is already current
sudo /usr/local/bin/duplicacy-backup update --force --yes
```

## Notes

- `--help` is intentionally concise; use `--help-full` for the detailed reference
- `config --help` is intentionally concise; use `config --help-full` for the detailed config reference
- config files are TOML files named `<label>-backup.toml`
- object-target storage credentials are read from `/root/.secrets/<label>-secrets.toml`
- one config file and, when needed, one secrets file cover a whole label
- object-target secrets for Storj-backed S3-compatible storage use `storj_s3_id` and `storj_s3_secret`; any target may also use optional `health_webhook_bearer_token` / `health_ntfy_token`
- `--fix-perms` is filesystem-target aware and cannot be combined with an object target
- combined phases all run against one selected target from a label config for a single invocation
- `--prune` is shown as `Safe prune` unless `--force-prune` is supplied, in which case it is shown as `Forced prune`
- `--cleanup-storage` is a standalone maintenance operation and may also be combined with prune
- `--cleanup-storage` runs `duplicacy prune -exhaustive -exclusive`, so it should be used only when no other client is actively writing to the same storage
- `--force-prune` only affects prune threshold enforcement
- `--force-prune` requires `--prune`
- interactive terminal runs ask for confirmation before forced prune and cleanup-storage
- non-interactive runs continue without confirmation so scheduled jobs are unaffected
- standalone `--fix-perms` does not require `duplicacy`
- `config validate` works on one selected target from a label config at a time
- `config validate --target <name>` requires that selected target config be valid, that backup-required settings such as destination, threads, prune policy, and local-account semantics are valid, and that any Btrfs, secrets, and repository checks that the current user can perform succeed
- `config validate` never initialises storage or changes repository state
- non-root `config validate` remains useful, but root-only checks may be reported as `Not checked`
- repository readiness is reported as exactly one of:
  - `Repository Access : Valid`
  - `Repository Access : Not initialized`
  - `Repository Access : Invalid (...)`
- `config explain` and `config paths` surface `Type` and `Location` for the selected target
- `config explain` does not load object-target secrets by default; it stays read-only and identity-focused while still showing the expected secrets-file path
- `config validate` keeps `Resolved` identity-only and reports the target-model outcome under `Target Settings`
- `config validate` also reports `Privileges` as `Full` or `Limited` so it is obvious when root-only checks may be skipped
- `config paths` includes secrets paths only for object targets
- there is no implicit target selection; every runtime, `config`, and `health` command must pass `--target <name>`
- `notify test` uses the existing label and target config, sends a clearly marked synthetic notification, and can target `webhook`, `ntfy`, or `all`
- `notify test update` uses the global app config at `<config-dir>/duplicacy-backup.toml` and does not require a label, target, or storage secrets
- `update` checks GitHub for the latest published release by default, downloads the matching Linux package for the current platform, verifies its checksum, and reuses the packaged `install.sh`
- `update --check-only` shows the current version, target version, asset, and managed install paths without downloading anything
- `update --force` reinstalls the selected release even when it is already current; it does not skip interactive confirmation unless `--yes` is also supplied
- `update` expects the standard managed layout under `/usr/local/lib/duplicacy-backup` with `/usr/local/bin/duplicacy-backup` as the stable command path
- `update` defaults to `--keep 2`, so the newly activated version and one previous version are retained unless you override that policy
- `update` can send failure notifications from the global `[update.notify]` config without reading label storage secrets
- default output is concise and phase-oriented; use `--verbose` for detailed operational logs
- `--json-summary` writes a machine-readable completion summary to stdout while human-readable logs stay on stderr
- `--json-summary` also applies to `health` commands and writes a machine-readable health report to stdout while human-readable health output stays on stderr
- runtime and health headers now identify `Label`, `Target`, `Type`, and `Location` before work begins
- health commands are read-only and never prompt for confirmation
- health commands use target-specific state under `/var/lib/duplicacy-backup/<label>.<target>.json` together with live Duplicacy storage inspection
- when `duplicacy list` exposes revision creation times, health freshness uses those storage timestamps as the authoritative freshness signal
- `health status` reports revision count plus the latest revision and freshness
- `health verify` uses `duplicacy check -persist` in the current repository context to validate the revisions found for the current label
- health JSON stays machine-focused and omits the rendered check list shown in stderr output
- healthy `health verify` JSON includes summary fields such as `revision_count`, `latest_revision`, `latest_revision_at`, `checked_revision_count`, `passed_revision_count`, `failed_revision_count`, `failed_revisions`, `last_doctor_run_at`, and `last_verify_run_at`
- unhealthy `health verify` JSON also emits `failure_code`, `failure_codes`, and `recommended_action_codes` so automation can classify the failure without parsing human text
- `health verify` emits `revision_results` only when failures or incomplete integrity attribution need investigation
- optional shared health policy lives in `[health]`, with per-target overrides under `[targets.<name>.health]`
- optional shared notification settings live in `[health.notify]`, with per-target overrides under `[targets.<name>.health.notify]`
- native `ntfy` delivery can be configured under `[health.notify.ntfy]` or `[targets.<name>.health.notify.ntfy]`
- `send_for` may include `status`, `doctor`, `verify`, `backup`, `prune`, and `cleanup-storage`; runtime operations are opt-in
- optional health webhook authentication can be provided as `health_webhook_bearer_token` in the secrets TOML; native `ntfy` can use `health_ntfy_token`
- notification auth tokens are target-scoped in the secrets file, so authenticated delivery must be configured under each notifying `[targets.<name>]` section
- update notification settings are global under `[update.notify]`; they are intentionally separate from label and target notification settings
- notification payloads are generic JSON with shared identity fields such as `label`, `target`, `storage_type`, and `location`
- native `ntfy` is the recommended low-cost alert destination on Synology; generic webhook remains available for future providers and bridges
- default health exit codes are `0` healthy, `1` degraded, `2` unhealthy
- installed Synology runtime commands and installed-config inspection commands should normally be run with `sudo`; `config paths` is the main normal-user exception
- if config cannot be read at all, built-in notifications are not expected to work; treat Synology scheduled-task monitoring as the fallback alert path for hard startup/environment failures
- keep `source_path` pointed at the real Btrfs volume or subvolume for the label; use Duplicacy filters to include or exclude directories beneath that root

## Notification Test Semantics

Use `notify test` when you want to validate the notification path for a
specific label and target without waiting for a real backup or health event.
Use `notify test update` when you want to validate the global update
notification route without performing a real update.

What it validates:

- the selected label and target resolve correctly
- the requested provider is configured for that target
- target-scoped auth tokens and destination settings can be loaded
- the provider accepts a clearly marked synthetic notification
- the global update notification route is reachable, when using
  `notify test update`

What it does not validate:

- that a real backup, prune, or health condition has occurred
- that a real self-update has occurred
- that DSM scheduled-task email is working
- that your normal `notify_on` or `send_for` policy would naturally emit for a
  real event
- that a remote provider UI will render the message exactly the same way in
  every client

Provider expectations:

- `ntfy`
  The command sends a real synthetic message to the configured topic using the
  same target-scoped token handling as normal notifications. Public topics can
  be tested without reading the label secrets file, even for object targets.
  Token-protected topics still need readable token access, and malformed token
  config still fails. Expect a visible test notification with `category = test`
  and the selected severity.
- `webhook`
  The command sends the same generic JSON payload shape used by real runtime and
  health notifications. The receiving system is responsible for accepting,
  displaying, or translating that payload.

Recommended operator flow:

1. Confirm the selected target has the provider configured.
2. Start with `notify test --dry-run` if you want to inspect the resolved
   destinations and synthetic payload details without sending anything.
3. Run `notify test` without `--dry-run` to send the real synthetic message.
4. Confirm the message arrives in the receiving system.
