# CLI Reference

## Usage

```text
duplicacy-backup [OPTIONS] <source>
duplicacy-backup config <validate|explain|paths> [OPTIONS] <source>
duplicacy-backup health <status|doctor|verify> [OPTIONS] <source>
```

Every runtime, `config`, and `health` command requires an explicit
`--target <name>`.
Every runtime command must also pass at least one explicit primary operation.

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

# Verify homes on target offsite-storj
sudo duplicacy-backup health verify --target offsite-storj homes
```

## Notes

- `--help` is intentionally concise; use `--help-full` for the detailed reference
- `config --help` is intentionally concise; use `config --help-full` for the detailed config reference
- config files are TOML files named `<label>-backup.toml`
- object-target secrets are read from `/root/.secrets/<label>-secrets.toml`
- one config file and, when needed, one secrets file cover a whole label
- current secrets keys for Storj-backed S3-compatible targets are `storj_s3_id`, `storj_s3_secret`, and optional `health_webhook_bearer_token`
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
- `config validate --target <name>` requires that selected target config be valid, that backup-required settings such as destination, threads, prune policy, and local-account semantics are valid, that the label `source_path` is a valid Btrfs snapshot source, that any required object-target secrets be valid, and that the selected repository be checked with a read-only readiness probe
- `config validate` never initialises storage or changes repository state
- repository readiness is reported as exactly one of:
  - `Repository Access : Valid`
  - `Repository Access : Not initialized`
  - `Repository Access : Invalid (...)`
- `config explain` and `config paths` surface `Type` and `Location` for the selected target
- `config validate` keeps `Resolved` identity-only and reports the target-model outcome under `Target Settings`
- `config paths` includes secrets paths only for object targets
- there is no implicit target selection; every runtime, `config`, and `health` command must pass `--target <name>`
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
- optional shared webhook notification settings live in `[health.notify]`, with per-target overrides under `[targets.<name>.health.notify]`
- `send_for` may include `status`, `doctor`, `verify`, `backup`, `prune`, and `cleanup-storage`; runtime operations are opt-in
- optional health webhook authentication can be provided as `health_webhook_bearer_token` in the secrets TOML
- webhook payloads are generic JSON with shared identity fields such as `label`, `target`, `storage_type`, and `location`
- default health exit codes are `0` healthy, `1` degraded, `2` unhealthy
- installed Synology runtime commands and installed-config inspection commands should normally be run with `sudo`; `config paths` is the main normal-user exception
- if config cannot be read at all, built-in health webhooks are not expected to work; treat Synology scheduled-task monitoring as the fallback alert path for hard startup/environment failures
- keep `source_path` pointed at the real Btrfs volume or subvolume for the label; use Duplicacy filters to include or exclude directories beneath that root
