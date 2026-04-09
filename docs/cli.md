# CLI Reference

## Usage

```text
duplicacy-backup [OPTIONS] <source>
```

If no primary mode is specified, the binary defaults to backup mode.

## Modes

| Flag | Description |
|---|---|
| `--backup` | Perform backup only |
| `--prune` | Perform safe, threshold-guarded policy prune only |
| `--prune-deep` | Perform maintenance prune mode; requires `--force-prune` |

## Modifiers

| Flag | Description |
|---|---|
| `--fix-perms` | Normalise local repository ownership and permissions; can run alone or after backup/prune |
| `--force-prune` | Override safe prune thresholds, or authorize `--prune-deep` |
| `--remote` | Use the remote config and secrets path |
| `--dry-run` | Simulate actions without making changes |
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

# Safe prune
duplicacy-backup --prune homes

# Force prune
duplicacy-backup --prune --force-prune homes

# Deep prune
duplicacy-backup --prune-deep --force-prune homes

# Fix permissions only
duplicacy-backup --fix-perms homes

# Backup then fix permissions
duplicacy-backup --backup --fix-perms homes

# Remote backup preview
duplicacy-backup --remote --dry-run homes
```

## Notes

- config files are TOML files named `<label>-backup.toml`
- remote secrets files are TOML files named `duplicacy-<label>.toml`
- `--fix-perms` is local-only and cannot be combined with `--remote`
- `--prune-deep` requires `--force-prune`
- `--force-prune` requires `--prune` or `--prune-deep`
- standalone `--fix-perms` does not require `duplicacy`
