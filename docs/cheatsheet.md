# Operator Cheat Sheet

Run commands as the operator user by default. Use `sudo` only for operations
that need root-level OS access, such as `backup`, `fix-perms`, and managed
install activation with `update --yes` or `rollback --yes`.

Despite its historical name, `duplicacy-backup` is now the operator entrypoint
for backup, prune, health, diagnostics, restore drills, update, and rollback
workflows.

This is the primary home for copyable operator command examples. Use
[`cli.md`](cli.md) when you need the full command surface and option reference.

`backup`, `prune`, `cleanup-storage`, `fix-perms`, `config`, `diagnostics`,
`health`, restore commands, and label-scoped `notify test` commands need an
explicit `--target <name>`.

Target model:

- `location = "local"` or `location = "remote"`
- targets use `storage = "..."`; include the full Duplicacy backend path there
- storage keys are loaded for known Duplicacy backends that require them
- `fix-perms` only works for path-based Duplicacy storage targets

## Common Runs

```bash
# Backup homes to target onsite-usb
sudo duplicacy-backup backup --target onsite-usb homes

# Safe prune homes on target onsite-usb
duplicacy-backup prune --target onsite-usb homes

# Backup homes to target offsite-storj
sudo duplicacy-backup backup --target offsite-storj homes

# Backup homes to a local S3-compatible service
sudo duplicacy-backup backup --target onsite-rustfs homes

# Backup homes to target offsite-usb on a mounted remote filesystem
sudo duplicacy-backup backup --target offsite-usb homes

# Preview a backup of homes to target onsite-usb
sudo duplicacy-backup backup --target onsite-usb --dry-run homes

# Preview prune for homes on target onsite-usb with detailed logs
duplicacy-backup prune --target onsite-usb --verbose --dry-run homes

# Run storage cleanup for homes on target onsite-usb
duplicacy-backup cleanup-storage --target onsite-usb homes

# Fix permissions for homes on target onsite-usb
sudo duplicacy-backup fix-perms --target onsite-usb homes
```

## Health Checks

```bash
# Fast health summary for homes on target onsite-usb
duplicacy-backup health status --target onsite-usb homes

# Run a doctor check for homes on target onsite-usb
duplicacy-backup health doctor --target onsite-usb homes

# Run verify for homes on target onsite-usb
duplicacy-backup health verify --target onsite-usb homes

# Write a JSON verify report for homes on target onsite-usb
duplicacy-backup health verify --json-summary --target onsite-usb homes
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
duplicacy-backup config validate --target onsite-usb homes

# Explain config for homes on target onsite-usb
duplicacy-backup config explain --target onsite-usb homes

# Explain config for homes on target offsite-storj
duplicacy-backup config explain --target offsite-storj homes

# Show paths for homes on target onsite-usb
duplicacy-backup config paths --target onsite-usb homes

# Show paths for homes on target offsite-storj
duplicacy-backup config paths --target offsite-storj homes
```

## Diagnostics

```bash
# Gather a redacted support bundle for homes on target onsite-usb
duplicacy-backup diagnostics --target onsite-usb homes

# Write machine-readable diagnostics for support or automation
duplicacy-backup diagnostics --target offsite-storj --json-summary homes
```

- Use `diagnostics` when you need one pasteable view of resolved config paths, storage scheme, secrets presence, state freshness, last run status, and basic path permissions.
- Diagnostics redacts secret values; it reports whether required storage keys are present, not the key contents.
- It does not run backup, prune, restore, or storage cleanup.

## Installed Paths

```text
/usr/local/bin/duplicacy-backup
$HOME/.config/duplicacy-backup/<label>-backup.toml
$HOME/.config/duplicacy-backup/secrets/<label>-secrets.toml  # Duplicacy storage keys and notification auth tokens when needed
$HOME/.local/state/duplicacy-backup/state/<label>.<target>.json
```

Recommended permissions:
- `$HOME/.config/duplicacy-backup`: user-owned `700`
- `$HOME/.config/duplicacy-backup/<label>-backup.toml`: user-owned `600`
- `$HOME/.config/duplicacy-backup/secrets`: user-owned `700`
- `$HOME/.config/duplicacy-backup/secrets/<label>-secrets.toml`: user-owned `600`

Installer behaviour:
- `install.sh` installs and activates the binary
- `install.sh` never migrates runtime config or secrets automatically
- `migrate-runtime-profile.sh --dry-run` previews root-era config/secrets migration
- `migrate-runtime-profile.sh --move --target-user <user>` moves legacy TOML files into that user's profile

## General Guidelines

### Scheduling

- Start with `--dry-run` for anything destructive or unfamiliar.
- Schedule backup, prune, health, and fix-perms as separate tasks.
- Use Synology repeat scheduling for frequent onsite backups instead of creating several near-identical jobs.
- Use `cleanup-storage` only as manual or exceptional maintenance, and only when no other client is writing to the same storage.
- Use `prune --force` only as an explicit operator action, not as a routine scheduled task.
- See `docs/workflow-scheduling.md` for the recommended Task Scheduler naming pattern and workload cadence.

### Config and Validation

- Keep `source_path` pointed at the real Btrfs volume or subvolume for the label when the NAS will run backups, then use Duplicacy filters to include or exclude nested directories beneath that root.
- For restore-only disaster recovery access, `source_path` can be omitted until the future live source root is known.
- Use `storage` for Duplicacy backend behaviour and `location` for operator meaning; do not use `location` to decide whether secrets are needed.
- `config validate` is read-only. It does not initialise repositories or change storage state.
- Non-root `config validate` is still useful, but checks that require root may show `Not checked`.
- `Repository Access : Valid` means the selected repository is ready to use.
- `Repository Access : Not initialized` means the storage is reachable but that repository has not been initialised yet.
- `Repository Access : Invalid (...)` means repository access is broken, not merely uninitialised.
- `config explain` and `config paths` show `Location` for the selected target.
- `config explain` does not load storage secrets; it stays read-only and still shows the expected secrets-file path when the selected backend needs one.
- `config paths` only includes secrets paths when the selected backend needs secrets.

### Health and Output

- Use `health status` for quick checks, `health doctor` for diagnostics, and `health verify` for integrity confidence.
- Unhealthy `health verify --json-summary` includes `failure_code`, `failure_codes`, and `recommended_action_codes`.
- JSON goes to `stdout`; human logs stay on `stderr`.

### Restore Drills

- `duplicacy-backup restore run` prepares or reuses a drill workspace, restores only there, and never copies data back to the live source.
- On an existing NAS, start every drill with `config explain`, `config validate`, and `health status` for the exact label and target.
- On a replacement NAS where `source_path` is not known yet, use `config explain` and `restore list-revisions` to prove restore access first.
- Use snapshot ID `data` for repositories created by this tool.
- `restore select` is the primary operator restore flow. It presents restore points first, supports inspect-only, full restore, or selective restore, then previews the matching explicit commands before asking for confirmation.
- Use `q` at restore-select text prompts or inside the tree picker to cancel before execution. During an active restore, `Ctrl-C` cancels Duplicacy, keeps the drill workspace, does not delete restored files, and reports progress.
- `restore run` prints progress/status to stderr during execution and writes the final restore report to stdout.
- Restore into the drill workspace first, inspect the data, then copy back deliberately with `rsync --dry-run` first.
- `restore plan`, `restore list-revisions`, and `restore run` remain the expert and scriptable restore primitives.
- See [`restore-drills.md`](restore-drills.md) for the full safe procedure.

```bash
# Confirm the selected repository before a restore drill
duplicacy-backup config explain --target onsite-usb homes
duplicacy-backup config validate --target onsite-usb homes
duplicacy-backup health status --target onsite-usb homes

# Guided operator restore
duplicacy-backup restore select --target onsite-usb homes

# Start the picker under a known subtree in a large backup
duplicacy-backup restore select --target onsite-usb --path-prefix phillipmcmahon/code homes

# Expert restore primitives
duplicacy-backup restore plan --target onsite-usb homes
duplicacy-backup restore list-revisions --target onsite-usb homes
duplicacy-backup restore run \
  --target onsite-usb \
  --revision 2403 \
  --path "phillipmcmahon/Documents/tax.pdf" \
  --yes \
  homes

# Restore a directory subtree into the drill workspace
duplicacy-backup restore run \
  --target onsite-usb \
  --revision 2403 \
  --path "phillipmcmahon/code/*" \
  --yes \
  homes
```

### Notifications and Secrets

- Each label has one backup config file and, when needed, one matching secrets file. Those files can contain settings for multiple targets under that label.
- S3-compatible storage uses `s3_id` and `s3_secret` under
  `[targets.<name>.keys]`; native `storj://` storage uses `storj_key` and
  `storj_passphrase`.
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
duplicacy-backup rollback --check-only
sudo duplicacy-backup rollback --yes
```

- `--attestations required` needs GitHub CLI on `PATH` and stops before extraction/install if release-asset attestation verification fails.
- `--attestations auto` verifies when `gh` is available, stops on verification failure, and otherwise continues with checksum-only verification when `gh` is missing.
- The default is `--attestations off`, so existing scheduled update jobs keep checksum-only behaviour unless you opt in.
- `update` keeps the newly activated binary and one previous binary by default; use `--keep <count>` to change that local rollback window.
- `rollback --check-only` shows retained versions without changing symlinks.
- `rollback --yes` activates the newest previous retained version; use `--version <tag>` when you need a specific retained version.

## Help

```bash
duplicacy-backup --help
duplicacy-backup --help-full
duplicacy-backup config --help
duplicacy-backup notify --help
duplicacy-backup update --help
duplicacy-backup rollback --help
duplicacy-backup diagnostics --help
duplicacy-backup health --help
duplicacy-backup config --help-full
```
