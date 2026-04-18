# Desk Cheat Sheet

Use `sudo` for most installed NAS operations. `config paths`, `update --check-only`,
and dry-run notification tests are common normal-user exceptions.

Runtime, `config`, `health`, and label-scoped `notify test` commands need an
explicit `--target <name>`. Runtime commands also need at least one explicit
operation flag such as `--backup`, `--prune`, `--cleanup-storage`, or
`--fix-perms`.

Target model:

- `type = "filesystem"` or `type = "object"`
- `location = "local"` or `location = "remote"`
- supported combinations are `filesystem/local`, `filesystem/remote`, and `object/remote`
- only object targets need storage secrets
- `--fix-perms` only works for filesystem targets

## Common Runs

```bash
# Backup homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup homes

# Backup then safe prune homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup --prune homes

# Backup homes to target offsite-storj
sudo duplicacy-backup --target offsite-storj --backup homes

# Backup homes to target offsite-usb on a mounted remote filesystem
sudo duplicacy-backup --target offsite-usb --backup homes

# Preview a backup of homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --dry-run --backup homes

# Preview backup and prune for homes on target onsite-usb with detailed logs
sudo duplicacy-backup --target onsite-usb --verbose --dry-run --backup --prune homes

# Run storage cleanup for homes on target onsite-usb
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

# Run a doctor check for homes on target onsite-usb
sudo duplicacy-backup health doctor --target onsite-usb homes

# Run verify for homes on target onsite-usb
sudo duplicacy-backup health verify --target onsite-usb homes

# Write a JSON verify report for homes on target onsite-usb
sudo duplicacy-backup health verify --json-summary --target onsite-usb homes
```

Exit codes:
- `0` healthy
- `1` degraded
- `2` unhealthy

Runtime, config, notify, and update failures exit `1`. Health commands keep
their health-specific exit-code contract, so a health pre-run failure such as a
logger or privilege problem exits `2`.

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
/root/.secrets/<label>-secrets.toml  # object credentials and notification auth tokens when needed
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

## General Guidelines

### Scheduling

- Start with `--dry-run` for anything destructive or unfamiliar.
- Schedule backup, prune, health, and fix-perms as separate tasks unless you have a clear reason to combine them.
- Use Synology repeat scheduling for frequent onsite backups instead of creating several near-identical jobs.
- Use `--cleanup-storage` only as manual or exceptional maintenance, and only when no other client is writing to the same storage.
- Use `--force-prune` only as an explicit operator action with `--prune`, not as a routine scheduled task.
- See `docs/workflow-scheduling.md` for the recommended Task Scheduler naming pattern and workload cadence.

### Config and Validation

- Keep `source_path` pointed at the real Btrfs volume or subvolume for the label, then use Duplicacy filters to include or exclude nested directories beneath that root.
- Use `type` for storage behaviour and `location` for operator meaning; do not use `location` to decide whether secrets are needed.
- `config validate` is read-only. It does not initialise repositories or change storage state.
- Non-root `config validate` is still useful, but checks that require root may show `Not checked`.
- `Repository Access : Valid` means the selected repository is ready to use.
- `Repository Access : Not initialized` means the destination is reachable but that repository has not been initialised yet.
- `Repository Access : Invalid (...)` means repository access is broken, not merely uninitialised.
- `config explain` and `config paths` show `Type` and `Location` for the selected target.
- `config explain` does not load object-target secrets by default; it stays read-only and still shows the expected secrets-file path.
- `config paths` only includes secrets paths when the selected target is an object target.

### Health and Output

- Use `health status` for quick checks, `health doctor` for diagnostics, and `health verify` for integrity confidence.
- Unhealthy `health verify --json-summary` includes `failure_code`, `failure_codes`, and `recommended_action_codes`.
- JSON goes to `stdout`; human logs stay on `stderr`.

### Notifications and Secrets

- Each label has one backup config file and, when needed, one matching secrets file. Those files can contain settings for multiple targets under that label.
- `[health.notify]` can opt runtime failure notifications in with `send_for = ["backup", "prune", "cleanup-storage"]`.
- `notify test` validates provider delivery and auth for the selected target; it does not prove that a real backup or health event occurred.
- `notify test update` validates the global update notification route without running a real update.
- Configure native `ntfy` delivery under `[health.notify.ntfy]`; generic webhook output remains available for future providers and bridges.
- Configure update failure alerts under `[update.notify]` in `<config-dir>/duplicacy-backup.toml`; this is separate from label and target notification settings.
- Authenticated webhook and `ntfy` tokens are target-scoped in the secrets file, so repeat them under each notifying target that needs auth.
- If notification config cannot be read, fall back to Synology scheduled-task alerts.

## Updates

```bash
duplicacy-backup update --check-only
sudo duplicacy-backup update --yes
sudo duplicacy-backup update --attestations required --yes
sudo duplicacy-backup update --force --yes
```

- `--attestations required` needs GitHub CLI on `PATH` and stops before extraction/install if release-asset attestation verification fails.
- `--attestations auto` verifies when `gh` is available, stops on verification failure, and otherwise continues with checksum-only verification when `gh` is missing.
- The default is `--attestations off`, so existing scheduled update jobs keep checksum-only behaviour unless you opt in.
- `update` keeps the newly activated binary and one previous binary by default; use `--keep <count>` to change that local rollback window.

## Help

```bash
duplicacy-backup --help
duplicacy-backup --help-full
duplicacy-backup config --help
duplicacy-backup notify --help
duplicacy-backup update --help
duplicacy-backup health --help
duplicacy-backup config --help-full
```
