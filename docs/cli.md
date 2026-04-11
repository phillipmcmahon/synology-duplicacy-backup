# CLI Reference

## Usage

```text
duplicacy-backup [OPTIONS] <source>
duplicacy-backup config <validate|explain|paths> [OPTIONS] <source>
duplicacy-backup health <status|doctor|verify> [OPTIONS] <source>
```

If no primary operation is specified, the binary defaults to backup mode.

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
| `--fix-perms` | Normalise local repository ownership and permissions; can run alone or after backup/prune |

## Modifiers

| Flag | Description |
|---|---|
| `--force-prune` | Override safe prune thresholds, turning prune into forced prune |
| `--remote` | Use the remote S3-compatible config and secrets path |
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
| `config validate <label>` | Validate resolved local config and, when configured, remote config and secrets |
| `config explain <label>` | Show resolved config values for local mode by default |
| `config explain --remote <label>` | Show resolved config values for remote mode |
| `config paths <label>` | Show resolved stable config, secrets, source, and log paths |

## Health Commands

| Command | Description |
|---|---|
| `health status <label>` | Fast read-only health summary for operators and schedulers |
| `health doctor <label>` | Read-only environment and storage diagnostic pass |
| `health verify <label>` | Read-only integrity check across revisions found for the current label |

## Environment Variables

| Variable | Description |
|---|---|
| `DUPLICACY_BACKUP_CONFIG_DIR` | Override config directory unless `--config-dir` is provided |
| `DUPLICACY_BACKUP_SECRETS_DIR` | Override secrets directory unless `--secrets-dir` is provided |

## Examples

```bash
# Default backup
sudo duplicacy-backup homes

# Explicit backup
sudo duplicacy-backup --backup homes

# Backup, then safe prune
sudo duplicacy-backup --backup --prune homes

# Force prune
sudo duplicacy-backup --prune --force-prune homes

# Storage cleanup only
sudo duplicacy-backup --cleanup-storage homes

# Backup, then safe prune, then storage cleanup
sudo duplicacy-backup --backup --prune --cleanup-storage homes

# Backup, then forced prune, then storage cleanup
sudo duplicacy-backup --backup --prune --force-prune --cleanup-storage homes

# Fix permissions only
sudo duplicacy-backup --fix-perms homes

# Backup, then fix permissions
sudo duplicacy-backup --backup --fix-perms homes

# Backup, then safe prune, then fix permissions
sudo duplicacy-backup --prune --backup --fix-perms homes

# Remote backup preview
sudo duplicacy-backup --remote --dry-run homes

# Verbose troubleshooting run
sudo duplicacy-backup --verbose --backup --prune homes

# Machine-readable completion summary
sudo duplicacy-backup --json-summary --dry-run homes

# Validate local config and, when present, remote config/secrets
sudo duplicacy-backup config validate homes

# Explain remote config
sudo duplicacy-backup config explain --remote homes

# Show resolved paths
duplicacy-backup config paths homes

# Fast health summary
sudo duplicacy-backup health status homes

# Read-only health diagnostics in JSON
sudo duplicacy-backup health doctor --json-summary homes

# Remote integrity verification
sudo duplicacy-backup health verify --remote homes
```

## Notes

- `--help` is intentionally concise; use `--help-full` for the detailed reference
- `config --help` is intentionally concise; use `config --help-full` for the detailed config reference
- config files are TOML files named `<label>-backup.toml`
- remote secrets files are TOML files named `duplicacy-<label>.toml`
- current remote secrets keys are `storj_s3_id`, `storj_s3_secret`, and optional `health_webhook_bearer_token`
- `--fix-perms` is local-only and cannot be combined with `--remote`
- combined phases all run in the same target mode for a single invocation; `--remote` applies to every requested phase
- `--prune` is shown as `Safe prune` unless `--force-prune` is supplied, in which case it is shown as `Forced prune`
- `--cleanup-storage` is a standalone maintenance operation and may also be combined with prune
- `--cleanup-storage` runs `duplicacy prune -exhaustive -exclusive`, so it should be used only when no other client is actively writing to the same storage
- `--force-prune` only affects prune threshold enforcement
- `--force-prune` requires `--prune`
- interactive terminal runs ask for confirmation before forced prune and cleanup-storage
- non-interactive runs continue without confirmation so scheduled jobs are unaffected
- standalone `--fix-perms` does not require `duplicacy`
- `config validate` always validates local config; if a `[remote]` table is present it also validates remote config and secrets
- `config validate --remote` requires remote config and remote secrets to be valid
- default output is concise and phase-oriented; use `--verbose` for detailed operational logs
- `--json-summary` writes a machine-readable completion summary to stdout while human-readable logs stay on stderr
- `--json-summary` also applies to `health` commands and writes a machine-readable health report to stdout while human-readable health output stays on stderr
- health commands are read-only and never prompt for confirmation
- health commands use local state under `/var/lib/duplicacy-backup/<label>.json` together with live Duplicacy storage inspection
- when `duplicacy list` exposes revision creation times, health freshness uses those storage timestamps as the authoritative freshness signal
- `health status` reports revision count plus the latest revision and freshness
- `health verify` uses `duplicacy check -persist` in the current repository context to validate the revisions found for the current label
- health JSON stays machine-focused and omits the rendered check list shown in stderr output
- healthy `health verify` JSON includes summary fields such as `revision_count`, `latest_revision`, `latest_revision_at`, `checked_revision_count`, `passed_revision_count`, `failed_revision_count`, `failed_revisions`, `last_doctor_run_at`, and `last_verify_run_at`
- `health verify` emits `revision_results` only when failures or incomplete integrity attribution need investigation
- optional per-backup health policy lives in `[health]`
- optional webhook notification settings live in `[health.notify]`
- optional health webhook authentication can be provided as `health_webhook_bearer_token` in the secrets TOML
- default health exit codes are `0` healthy, `1` degraded, `2` unhealthy
- installed Synology runtime commands and installed-config inspection commands should normally be run with `sudo`; `config paths` is the main normal-user exception
- if config cannot be read at all, built-in health webhooks are not expected to work; treat Synology scheduled-task monitoring as the fallback alert path for hard startup/environment failures
