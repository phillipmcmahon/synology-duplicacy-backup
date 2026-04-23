# CLI Reference

## Usage

```text
duplicacy-backup backup [OPTIONS] <source>
duplicacy-backup prune [OPTIONS] <source>
duplicacy-backup cleanup-storage [OPTIONS] <source>
duplicacy-backup fix-perms [OPTIONS] <source>
duplicacy-backup config <validate|explain|paths> [OPTIONS] <source>
duplicacy-backup diagnostics [OPTIONS] <source>
duplicacy-backup notify <test> [OPTIONS] <source|update>
duplicacy-backup rollback [OPTIONS]
duplicacy-backup restore <plan|prepare|revisions|files|run|select> [OPTIONS] <source>
duplicacy-backup update [OPTIONS]
duplicacy-backup health <status|doctor|verify> [OPTIONS] <source>
```

`backup`, `prune`, `cleanup-storage`, `fix-perms`, `config`, `diagnostics`,
`health`, restore commands, and label-scoped `notify test`
commands need an explicit `--target <name>`.
`notify test update`, `update`, and `rollback` are global application commands
and do not use a target.

Targets describe both storage and deployment location:

- `location = "local"` or `location = "remote"`
- `storage = "..."` is passed directly to Duplicacy

Supported locations are:

- local
- remote

Runtime operations are first-class commands. The old top-level operation flags
are not supported.

## Runtime Commands

| Command | Description |
|---|---|
| `backup --target <target> <label>` | Run a backup for the selected label and target |
| `prune --target <target> [--force] <label>` | Run threshold-guarded prune, or forced prune with `--force` |
| `cleanup-storage --target <target> <label>` | Run exhaustive exclusive storage cleanup |
| `fix-perms --target <target> <label>` | Normalise path-based storage ownership and permissions |

## Modifiers

| Flag | Description |
|---|---|
| `--force` | Override safe prune thresholds for `prune` |
| `--target <name>` | Use the named target config; required for `backup`, `prune`, `cleanup-storage`, `fix-perms`, `config`, `diagnostics`, `health`, restore commands, and label-scoped `notify test` commands |
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
| `config validate --target <target> <label>` | Validate the selected target from that label config and any required storage secrets |
| `config explain --target <target> <label>` | Show resolved config values for the selected target from that label config |
| `config paths --target <target> <label>` | Show resolved stable config, source, log, and any applicable secrets paths |

## Diagnostics Command

| Command | Description |
|---|---|
| `diagnostics --target <target> <label>` | Print a redacted support bundle with resolved config, storage, secrets, state, and path-permission context |
| `diagnostics --target <target> --json-summary <label>` | Write the diagnostics report as JSON |

## Notify Commands

| Command | Description |
|---|---|
| `notify test --target <target> <label>` | Send a simulated notification through the configured destinations for the selected target |
| `notify test update` | Send a simulated update notification through the global update notification config |

## Restore Commands

| Command | Description |
|---|---|
| `restore plan --target <target> <label>` | Print a safe read-only restore drill plan for the selected label and target |
| `restore prepare --target <target> [--workspace <path>] <label>` | Create the safe drill workspace and write Duplicacy preferences without executing a restore |
| `restore revisions --target <target> [--limit <count>] <label>` | List visible revisions without executing a restore |
| `restore files --target <target> --revision <id> [--path <relative-path>] [--limit <count>] <label>` | List files in one revision without executing a restore |
| `restore run --target <target> --revision <id> [--path <relative-path-or-pattern>] --workspace <path> [--yes] <label>` | Restore into a prepared workspace only; never copy back to the live source |
| `restore select --target <target> [--execute] <label>` | Guide revision and path selection; without `--execute`, print primitive commands only; with `--execute`, confirm and delegate to `restore run` |

## Update Command

| Command | Description |
|---|---|
| `update [--check-only]` | Check GitHub for the latest published release and, when requested, install it through the packaged installer |

## Rollback Command

| Command | Description |
|---|---|
| `rollback --check-only` | Inspect retained managed-install versions and the selected rollback target |
| `rollback [--version <tag>] --yes` | Activate the newest previous retained version, or a specific retained version |

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
sudo duplicacy-backup backup --target onsite-usb homes

# Runtime command with modifiers
sudo duplicacy-backup backup --target onsite-usb --json-summary --dry-run homes

# Config command
sudo duplicacy-backup config validate --target onsite-usb homes

# Health command
sudo duplicacy-backup health status --target onsite-usb homes

# Redacted support bundle
duplicacy-backup diagnostics --target onsite-usb homes

# Label-scoped notification test
sudo duplicacy-backup notify test --target onsite-usb homes

# Restore planning command
sudo duplicacy-backup restore plan --target onsite-usb homes

# Restore workspace preparation command
sudo duplicacy-backup restore prepare --target onsite-usb homes

# Restore revision and file inspection
sudo duplicacy-backup restore revisions --target onsite-usb homes
sudo duplicacy-backup restore files --target onsite-usb --revision 2403 --path docs homes

# Guided command generation without executing a restore
sudo duplicacy-backup restore select --target onsite-usb homes

# Guided selection with guarded execution through restore run
sudo duplicacy-backup restore select --target onsite-usb --execute homes

# Restore into the prepared workspace only
sudo duplicacy-backup restore run --target onsite-usb --revision 2403 --path docs/readme.md --workspace /volume1/restore-drills/homes-onsite-usb --yes homes

# Global update command
/usr/local/bin/duplicacy-backup update --check-only

# Managed rollback inspection
/usr/local/bin/duplicacy-backup rollback --check-only

# Global update notification test
duplicacy-backup notify test update --provider ntfy --dry-run
```

## Behaviour Notes

- `--help` is intentionally concise; use `--help-full` for detailed command help.
- Every `backup`, `prune`, `cleanup-storage`, `fix-perms`, `config`,
  `diagnostics`, `health`, restore commands, and label-scoped `notify test`
  command needs `--target <name>`.
- Runtime operations are first-class commands; old top-level operation flags
  such as `--backup` and `--prune` are not supported.
- `restore plan` is read-only. It resolves the selected target and prints
  Duplicacy commands for a separate drill workspace; it does not create
  directories, write preferences, run `duplicacy restore`, or copy data back.
- `restore prepare` creates the separate drill workspace and writes
  `.duplicacy/preferences` there. It rejects the live source path,
  source-child workspaces, and non-empty workspaces, and still does not run
  `duplicacy restore` or copy data back.
- `restore revisions` and `restore files` are read-only discovery commands.
  They create a temporary Duplicacy workspace unless `--workspace` is supplied.
- `restore run` executes `duplicacy restore` only inside a prepared workspace.
  It never restores over the live source and never copies data back. Use a
  file path for one file or a Duplicacy pattern such as `docs/*` for a
  subtree.
- `restore select` is an interactive guide over the primitive restore commands.
  Without `--execute`, it prints the exact commands and stops. With
  `--execute`, it requires a prepared workspace, asks for confirmation, and
  delegates to `restore run`.
- `prune --force` overrides prune threshold enforcement.
- `cleanup-storage` runs exhaustive exclusive storage cleanup and should be
  treated as operator-directed maintenance.
- `--json-summary` writes machine-readable output to stdout while human logs
  stay on stderr.
- `diagnostics` is non-mutating. It gathers resolved label-target context,
  redacts secret values, and is intended for support bundles.
- Health command exit codes are `0` healthy, `1` degraded, `2` unhealthy.
- Storage keys live under `[targets.<name>.keys]` in the label secrets file
  when the selected backend requires them. For S3-compatible storage this
  means `s3_id` and `s3_secret`, including `s3://`, `s3c://`,
  `minio://`, and `minios://` storage values; see
  [configuration.md](configuration.md) for ownership, permissions, and
  notification-token details.
- `update` uses the managed install layout under
  `/usr/local/lib/duplicacy-backup` with `/usr/local/bin/duplicacy-backup` as
  the stable command path.
- `update` defaults to `--keep 2`, so the newly activated version and one
  previous version are retained unless you override that policy.
- `rollback --check-only` is non-mutating. Actual rollback activation requires
  root and changes only the managed `current` symlink.

Source-of-truth guides:

- Config files, target model, health policy, notification TOML, and secrets:
  [configuration.md](configuration.md)
- Install, update, rollback, and release verification procedures:
  [operations.md](operations.md)
- Routine Synology scheduling patterns:
  [workflow-scheduling.md](workflow-scheduling.md)
- Update checksum and attestation trust model:
  [update-trust-model.md](update-trust-model.md)
