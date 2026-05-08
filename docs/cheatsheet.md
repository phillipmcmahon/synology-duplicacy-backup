# Operator Cheat Sheet

Run commands as the operator user by default. Use `sudo` only for operations
that need root-level OS access, such as `backup`, local filesystem repository
`prune`, `cleanup-storage`, `health status`, `health doctor`, or
`health verify`, and managed install activation with `update --yes` or
`rollback --yes`.

For scheduled tasks, prefer running the DSM task as the operator user and using
a narrow `/etc/sudoers.d` rule plus `sudo -n` only for those exact
root-required commands. See [`privilege-model.md`](privilege-model.md#least-privilege-sudo-for-scheduled-tasks).
Do not schedule profile-using commands directly as `root`; direct root
execution is rejected unless explicit profile roots are supplied.

Despite its historical name, `duplicacy-backup` is now the operator entrypoint
for backup, prune, health, diagnostics, restore drills, update, and rollback
workflows.

This is the primary home for copyable operator command examples. Use
[`cli.md`](cli.md) when you need the full command surface and option reference.

`backup`, `prune`, `cleanup-storage`, `config`, `diagnostics`,
`health`, restore commands, and label-scoped `notify test` commands need an
explicit `--storage <name>`.

The selected storage must already be initialized with the Duplicacy CLI.
`duplicacy-backup` validates and uses existing repositories; it does not create
new ones.

Use [Configuration and secrets](configuration.md) for storage model and secrets
setup.

## Common Runs

```bash
# Backup homes to storage onsite-usb
sudo duplicacy-backup backup --storage onsite-usb homes

# Safe prune homes on storage onsite-usb
sudo duplicacy-backup prune --storage onsite-usb homes

# Preview a backup of homes to storage onsite-usb
sudo duplicacy-backup backup --storage onsite-usb --dry-run homes

# Preview prune for homes on storage onsite-usb with detailed logs
sudo duplicacy-backup prune --storage onsite-usb --verbose --dry-run homes

# Run storage cleanup for homes on storage onsite-usb
sudo duplicacy-backup cleanup-storage --storage onsite-usb homes
```

Use `cleanup-storage` only as manual or exceptional maintenance, and only when
no other client is writing to the same storage. Use `prune --force` only as an
explicit operator action, not as a routine scheduled task.

## Health Checks

```bash
# Fast health summary for homes on local filesystem storage onsite-usb
sudo duplicacy-backup health status --storage onsite-usb homes

# Run a doctor check for homes on local filesystem storage onsite-usb
sudo duplicacy-backup health doctor --storage onsite-usb homes

# Run verify for homes on local filesystem storage onsite-usb
sudo duplicacy-backup health verify --storage onsite-usb homes

# Write a JSON verify report for homes on local filesystem storage onsite-usb
sudo duplicacy-backup health verify --json-summary --storage onsite-usb homes
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
# Validate config for homes on storage onsite-usb
sudo duplicacy-backup config validate --storage onsite-usb homes

# Explain config for homes on storage offsite-storj
duplicacy-backup config explain --storage offsite-storj homes

# Show resolved paths for homes on storage offsite-storj
duplicacy-backup config paths --storage offsite-storj homes
```

## Diagnostics

```bash
# Gather a redacted support bundle for homes on storage onsite-usb
duplicacy-backup diagnostics --storage onsite-usb homes

# Write machine-readable diagnostics for support or automation
duplicacy-backup diagnostics --storage offsite-storj --json-summary homes
```

- Use `diagnostics` when you need one pasteable view of resolved config paths, storage scheme, secrets presence, state freshness, last run status, and basic path permissions.
- Diagnostics redacts secret values; it reports whether required storage keys are present, not the key contents.
- It does not run backup, prune, restore, or storage cleanup.

## Restore Drills

```bash
# Confirm the selected repository before a restore drill
duplicacy-backup config explain --storage onsite-usb homes
sudo duplicacy-backup config validate --storage onsite-usb homes
sudo duplicacy-backup health status --storage onsite-usb homes

# Guided operator restore
sudo duplicacy-backup restore select --storage onsite-usb homes

# Start the picker under a known subtree in a large backup
sudo duplicacy-backup restore select --storage onsite-usb --path-prefix phillipmcmahon/code homes
```

At the restore-point prompt, enter the displayed choice number, or type a
revision ID with `r<id>` or `rev <id>`. Restore into the drill workspace first;
copy back only after inspection. Use [Restore drills](restore-drills.md) for
expert primitives, selective restore examples, and the copy-back procedure.

## Notifications

```bash
# Test label-scoped notification delivery for one storage
duplicacy-backup notify test --storage onsite-usb homes

# Test global update notification delivery
duplicacy-backup notify test update --provider ntfy --dry-run
```

Notification setup, auth tokens, and policy live in
[Configuration and secrets](configuration.md#notifications).

## Updates

```bash
duplicacy-backup update --check-only
sudo duplicacy-backup update --yes
duplicacy-backup rollback --check-only
sudo duplicacy-backup rollback --yes
```

Use [Operations](operations.md#upgrade-and-rollback) for install, update, and
rollback workflow. Use [Update trust model](update-trust-model.md) for checksum
and attestation choices.

## Key Paths

```text
/usr/local/bin/duplicacy-backup
$HOME/.config/duplicacy-backup/<label>-backup.toml
$HOME/.config/duplicacy-backup/secrets/<label>-secrets.toml
$HOME/.local/state/duplicacy-backup/state/<label>.<storage>.json
```

Use [Operations](operations.md#runtime-locations-after-install) for install
layout and runtime path details.

## Rules Of Thumb

- Start with `--dry-run` for destructive or unfamiliar commands.
- Initialize new storage with Duplicacy before validating it here.
- Schedule backup, prune, health, and diagnostics as separate DSM tasks.
- Use `sudo -n` only for root-required scheduled commands.
- Use `--json-summary` when automation needs machine-readable output.
- Use `duplicacy-backup --help-full` or [CLI reference](cli.md) for full syntax.

Detailed guides:

- [Workflow and scheduling](workflow-scheduling.md)
- [Configuration and secrets](configuration.md)
- [Privilege model](privilege-model.md)
- [Restore drills](restore-drills.md)
- [Operations](operations.md)
- [Update trust model](update-trust-model.md)

## Help

```bash
duplicacy-backup --help
duplicacy-backup --help-full
duplicacy-backup config --help
duplicacy-backup update --help
duplicacy-backup restore --help-full
```
