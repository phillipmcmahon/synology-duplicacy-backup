# CLI Reference

## Usage

```text
duplicacy-backup [OPTIONS] <source>
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
| `--config-dir <path>` | Override the config directory |
| `--secrets-dir <path>` | Override the secrets directory |
| `--version`, `-v` | Show version and build information |
| `--help` | Show help |

## Environment Variables

| Variable | Description |
|---|---|
| `DUPLICACY_BACKUP_CONFIG_DIR` | Override config directory unless `--config-dir` is provided |
| `DUPLICACY_BACKUP_SECRETS_DIR` | Override secrets directory unless `--secrets-dir` is provided |

## Examples

```bash
# Default backup
duplicacy-backup homes

# Explicit backup
duplicacy-backup --backup homes

# Backup, then safe prune
duplicacy-backup --backup --prune homes

# Force prune
duplicacy-backup --prune --force-prune homes

# Storage cleanup only
duplicacy-backup --cleanup-storage homes

# Backup, then safe prune, then storage cleanup
duplicacy-backup --backup --prune --cleanup-storage homes

# Backup, then forced prune, then storage cleanup
duplicacy-backup --backup --prune --force-prune --cleanup-storage homes

# Fix permissions only
duplicacy-backup --fix-perms homes

# Backup, then fix permissions
duplicacy-backup --backup --fix-perms homes

# Backup, then safe prune, then fix permissions
duplicacy-backup --prune --backup --fix-perms homes

# Remote backup preview
duplicacy-backup --remote --dry-run homes

# Verbose troubleshooting run
duplicacy-backup --verbose --backup --prune homes
```

## Notes

- config files are TOML files named `<label>-backup.toml`
- remote secrets files are TOML files named `duplicacy-<label>.toml`
- current remote secrets keys are `storj_s3_id` and `storj_s3_secret`
- `--fix-perms` is local-only and cannot be combined with `--remote`
- combined phases all run in the same target mode for a single invocation; `--remote` applies to every requested phase
- `--prune` is shown as `Safe prune` unless `--force-prune` is supplied, in which case it is shown as `Forced prune`
- `--cleanup-storage` is a standalone maintenance operation and may also be combined with prune
- `--cleanup-storage` runs `duplicacy prune -exhaustive -exclusive`, so it should be used only when no other client is actively writing to the same storage
- `--force-prune` only affects prune threshold enforcement
- `--force-prune` requires `--prune`
- standalone `--fix-perms` does not require `duplicacy`
- default output is concise and phase-oriented; use `--verbose` for detailed operational logs
