# Desk Cheat Sheet

Use `sudo` for almost everything except `config paths`.

Every command must pass an explicit `--target <name>`.
Every runtime command must also pass at least one explicit operation flag such
as `--backup`, `--prune`, `--cleanup-storage`, or `--fix-perms`.

Target model:

- `type = "filesystem"` or `type = "object"`
- `location = "local"` or `location = "remote"`
- supported combinations are `filesystem/local`, `filesystem/remote`, and `object/remote`
- only object targets use secrets
- `--fix-perms` only works for filesystem targets

## Common Runs

```bash
# Backup homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup homes

# Backup then safe prune homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup --prune homes

# Backup homes to target offsite-storj
sudo duplicacy-backup --target offsite-storj --backup homes

# Backup homes to target offsite-usb mounted over VPN
sudo duplicacy-backup --target offsite-usb --backup homes

# Preview backing up homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --dry-run --backup homes

# Preview backup and prune for homes on target onsite-usb with detailed logs
sudo duplicacy-backup --target onsite-usb --verbose --dry-run --backup --prune homes

# Storage cleanup for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --cleanup-storage homes

# Fix permissions for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --fix-perms homes

# Fix permissions for homes on target offsite-usb
sudo duplicacy-backup --target offsite-usb --fix-perms homes
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
/root/.secrets/<label>-secrets.toml  # object credentials and/or notification auth tokens
/var/lib/duplicacy-backup/<label>.<target>.json
```

Recommended permissions:
- `/usr/local/lib/duplicacy-backup/.config`: `root:administrators` `750`
- `/usr/local/lib/duplicacy-backup/.config/<label>-backup.toml`: `root:administrators` `640`
- `/root/.secrets`: `root:root` `700`
- `/root/.secrets/<label>-secrets.toml`: `root:root` `600`

Installer behaviour:
- `install.sh` creates or normalises `/usr/local/lib/duplicacy-backup/.config`
- when run as `root`, `install.sh` also creates or normalises `/root/.secrets`
- `install.sh` never writes or rewrites individual secrets files

## Rules Of Thumb

- Start with `--dry-run` for anything destructive or unfamiliar.
- Schedule backup, prune, health, and fix-perms as separate tasks unless you have a specific reason to combine them.
- Use Synology repeat scheduling for frequent onsite backups rather than creating many near-identical jobs.
- Use `--force-prune` only with `--prune`.
- Use `--cleanup-storage` only when no other client is writing to the same storage.
- Keep `source_path` set to the real Btrfs volume or subvolume for the label, and use Duplicacy filters to include or exclude nested directories beneath that root.
- Use `type` for storage behaviour and `location` for operator meaning; do not use `location` to decide whether secrets are needed.
- `config validate` is read-only. It does not initialise repositories or change storage state.
- non-root `config validate` can still be useful, but root-only checks may show `Not checked`.
- `Repository Access : Valid` means the selected repository is ready to use.
- `Repository Access : Not initialized` means the destination is reachable but that repository has not been initialised yet.
- `Repository Access : Invalid (...)` means repository access is broken, not merely uninitialised.
- Use `health status` for quick checks, `health doctor` for diagnostics, and `health verify` for integrity confidence.
- JSON goes to `stdout`; human logs stay on `stderr`.
- One config file covers a whole label; one secrets file covers a whole label.
- `config explain` and `config paths` show `Type` and `Location` for the selected target.
- `config paths` only shows secrets paths for object targets.
- Unhealthy `health verify --json-summary` includes `failure_code`, `failure_codes`, and `recommended_action_codes`.
- `[health.notify]` can opt runtime failure notifications in with `send_for = ["backup", "prune", "cleanup-storage"]`.
- Native `ntfy` delivery is configured under `[health.notify.ntfy]`; generic webhook output remains available for future providers and bridges.
- Authenticated webhook and `ntfy` tokens are target-scoped in the secrets file; repeat them under each notifying target that needs auth.
- If health config cannot be read, rely on Synology scheduled-task alerts as the fallback.
- See `docs/workflow-scheduling.md` for the recommended Task Scheduler naming pattern and workload cadence.

## Help

```bash
duplicacy-backup --help
duplicacy-backup --help-full
duplicacy-backup config --help
duplicacy-backup health --help
duplicacy-backup config --help-full
```
