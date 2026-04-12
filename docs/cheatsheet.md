# Desk Cheat Sheet

Use `sudo` for almost everything except `config paths`.

Every command must pass an explicit `--target <name>`.
Every runtime command must also pass at least one explicit operation flag such
as `--backup`, `--prune`, `--cleanup-storage`, or `--fix-perms`.

## Common Runs

```bash
# Backup homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup homes

# Backup then safe prune homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup --prune homes

# Backup homes to target offsite-storj
sudo duplicacy-backup --target offsite-storj --backup homes

# Preview backing up homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --dry-run --backup homes

# Preview backup and prune for homes on target onsite-usb with detailed logs
sudo duplicacy-backup --target onsite-usb --verbose --dry-run --backup --prune homes

# Storage cleanup for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --cleanup-storage homes

# Fix permissions for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --fix-perms homes
```

## Health Checks

```bash
# Fast health summary for homes on target onsite-usb
sudo duplicacy-backup health status --target onsite-usb homes

# Doctor check for homes on target onsite-usb
sudo duplicacy-backup health doctor --target onsite-usb homes

# Verify homes on target onsite-usb
sudo duplicacy-backup health verify --target onsite-usb homes

# JSON verify report for homes on target onsite-usb
sudo duplicacy-backup health verify --json-summary --target onsite-usb homes
```

Exit codes:
- `0` healthy
- `1` degraded
- `2` unhealthy

## Config Commands

```bash
# Validate config for homes on target onsite-usb
sudo duplicacy-backup config validate --target onsite-usb homes

# Explain config for homes on target onsite-usb
sudo duplicacy-backup config explain --target onsite-usb homes

# Explain config for homes on target offsite-storj
sudo duplicacy-backup config explain --target offsite-storj homes

# Show paths for homes on target onsite-usb
duplicacy-backup config paths --target onsite-usb homes

# Show paths for homes on target offsite-storj
duplicacy-backup config paths --target offsite-storj homes
```

## Installed Paths

```text
/usr/local/bin/duplicacy-backup
/usr/local/lib/duplicacy-backup/.config/<label>-backup.toml
/root/.secrets/<label>-secrets.toml
/var/lib/duplicacy-backup/<label>.<target>.json
```

## Rules Of Thumb

- Start with `--dry-run` for anything destructive or unfamiliar.
- Use `--force-prune` only with `--prune`.
- Use `--cleanup-storage` only when no other client is writing to the same storage.
- Use `health status` for quick checks, `health doctor` for diagnostics, and `health verify` for integrity confidence.
- JSON goes to `stdout`; human logs stay on `stderr`.
- One config file covers a whole label; one secrets file covers a whole label.
- `config paths` only shows secrets paths for targets that actually use them.
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
