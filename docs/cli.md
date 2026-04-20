# CLI Reference

## Usage

```text
duplicacy-backup [OPTIONS] <source>
duplicacy-backup config <validate|explain|paths> [OPTIONS] <source>
duplicacy-backup notify <test> [OPTIONS] <source|update>
duplicacy-backup update [OPTIONS]
duplicacy-backup health <status|doctor|verify> [OPTIONS] <source>
```

Runtime, `config`, `health`, and label-scoped `notify test` commands need an
explicit `--target <name>`. `notify test update` and `update` are global
application commands and do not use a target.
Every runtime command also needs at least one explicit primary operation.

Targets describe both storage and deployment location:

- `location = "local"` or `location = "remote"`
- `storage = "..."` is passed directly to Duplicacy

Supported locations are:

- local
- remote

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
| `--target <name>` | Use the named target config; required for runtime, `config`, `health`, and label-scoped `notify test` commands |
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
| `config validate --target <target> <label>` | Validate the selected target from that label config and any required duplicacy-target secrets |
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

These examples show representative syntax. For a fuller operator command list,
use the [desk cheat sheet](cheatsheet.md). For recurring Synology scheduling
patterns, use [workflow-scheduling.md](workflow-scheduling.md).

```bash
# Runtime command: one label, one target, one explicit operation
sudo duplicacy-backup --target onsite-usb --backup homes

# Runtime command with modifiers
sudo duplicacy-backup --target onsite-usb --json-summary --dry-run --backup homes

# Config command
sudo duplicacy-backup config validate --target onsite-usb homes

# Health command
sudo duplicacy-backup health status --target onsite-usb homes

# Label-scoped notification test
sudo duplicacy-backup notify test --target onsite-usb homes

# Global update command
/usr/local/bin/duplicacy-backup update --check-only

# Global update notification test
duplicacy-backup notify test update --provider ntfy --dry-run
```

## Behaviour Notes

- `--help` is intentionally concise; use `--help-full` for detailed command help.
- Every runtime, `config`, `health`, and label-scoped `notify test` command
  needs `--target <name>`.
- Runtime commands also need at least one primary operation flag.
- Combined runtime phases always run in this order:
  `backup -> prune -> cleanup-storage -> fix-perms`.
- `--force-prune` requires `--prune` and only affects prune threshold
  enforcement.
- `--cleanup-storage` runs exhaustive exclusive storage cleanup and should be
  treated as operator-directed maintenance.
- `--json-summary` writes machine-readable output to stdout while human logs
  stay on stderr.
- Health command exit codes are `0` healthy, `1` degraded, `2` unhealthy.
- Duplicacy-target storage keys live under `[targets.<name>.keys]` in the
  label secrets file. For S3-compatible storage this usually means `s3_id`
  and `s3_secret`; see [configuration.md](configuration.md) for ownership,
  permissions, and notification-token details.
- `update` uses the managed install layout under
  `/usr/local/lib/duplicacy-backup` with `/usr/local/bin/duplicacy-backup` as
  the stable command path.
- `update` defaults to `--keep 2`, so the newly activated version and one
  previous version are retained unless you override that policy.

Source-of-truth guides:

- Config files, target model, health policy, notification TOML, and secrets:
  [configuration.md](configuration.md)
- Install, update, rollback, and release verification procedures:
  [operations.md](operations.md)
- Routine Synology scheduling patterns:
  [workflow-scheduling.md](workflow-scheduling.md)
- Update checksum and attestation trust model:
  [update-trust-model.md](update-trust-model.md)
