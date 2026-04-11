# Desk Cheat Sheet

Use `sudo` for almost everything except `config paths`.

## Common Runs

```bash
# Backup now
sudo duplicacy-backup homes

# Backup, then safe prune
sudo duplicacy-backup --backup --prune homes

# Remote backup
sudo duplicacy-backup --target remote homes

# Preview only
sudo duplicacy-backup --dry-run homes

# Preview with detailed logs
sudo duplicacy-backup --verbose --dry-run --backup --prune homes

# Storage cleanup only
sudo duplicacy-backup --cleanup-storage homes

# Fix permissions only
sudo duplicacy-backup --fix-perms homes
```

## Health Checks

```bash
# Fast summary
sudo duplicacy-backup health status homes

# Environment + access
sudo duplicacy-backup health doctor homes

# Integrity across revisions found for this backup
sudo duplicacy-backup health verify homes

# Health JSON
sudo duplicacy-backup health verify --json-summary homes
```

Exit codes:
- `0` healthy
- `1` degraded
- `2` unhealthy

## Config Commands

```bash
# Validate installed config
sudo duplicacy-backup config validate homes

# Explain config
sudo duplicacy-backup config explain homes

# Explain remote config
sudo duplicacy-backup config explain --target remote homes

# Show stable paths
duplicacy-backup config paths homes
```

## Installed Paths

```text
/usr/local/bin/duplicacy-backup
/usr/local/lib/duplicacy-backup/.config/<label>-<target>-backup.toml
/root/.secrets/duplicacy-<label>-<target>.toml
/var/lib/duplicacy-backup/<label>.<target>.json
```

## Rules Of Thumb

- Start with `--dry-run` for anything destructive or unfamiliar.
- Use `--force-prune` only with `--prune`.
- Use `--cleanup-storage` only when no other client is writing to the same storage.
- Use `health status` for quick checks, `health doctor` for diagnostics, and `health verify` for integrity confidence.
- JSON goes to `stdout`; human logs stay on `stderr`.
- Unhealthy `health verify --json-summary` includes `failure_code`, `failure_codes`, and `recommended_action_codes`.
- If health config cannot be read, rely on Synology scheduled-task alerts as the fallback.

## Help

```bash
duplicacy-backup --help
duplicacy-backup --help-full
duplicacy-backup config --help
duplicacy-backup health --help
duplicacy-backup config --help-full
```
