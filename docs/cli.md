# CLI Reference

Use this document when you need the exact command surface and option meaning.
The command name is historical: `duplicacy-backup` still runs scheduled
backups, but it also provides first-class restore, health, diagnostics, update,
and rollback workflows. For routine copyable commands, start with the
[operator cheat sheet](cheatsheet.md).

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
duplicacy-backup restore <plan|list-revisions|run|select> [OPTIONS] <source>
duplicacy-backup update [OPTIONS]
duplicacy-backup health <status|doctor|verify> [OPTIONS] <source>
```

Backup is one workflow among several. `backup`, `prune`, `cleanup-storage`,
`fix-perms`, `config`, `diagnostics`, `health`, restore commands, and
label-scoped `notify test` commands need an explicit `--target <name>`.
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
| `config validate --target <target> <label>` | Validate backup-readiness for the selected target, including source path, storage, repository, and any required storage secrets |
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
| `restore select --target <target> [--path-prefix <path>] <label>` | Primary operator restore flow: choose a restore point, inspect it read-only, or browse revision paths in an interactive tree and confirm guided restore execution |
| `restore plan --target <target> <label>` | Print a safe read-only restore drill plan for the selected label and target |
| `restore list-revisions --target <target> [--limit <count>] <label>` | List visible revisions without executing a restore |
| `restore run --target <target> --revision <id> [--path <relative-path-or-pattern>] [--workspace-root <path>] [--workspace <path>] [--yes] <label>` | Prepare or reuse a drill workspace, restore only there, and never copy back to the live source |

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
use the [operator cheat sheet](cheatsheet.md). For recurring Synology scheduling
patterns, use [workflow-scheduling.md](workflow-scheduling.md).

```bash
# Runtime command: one label, one target, one explicit operation
sudo duplicacy-backup backup --target onsite-usb homes

# Runtime command with modifiers
sudo duplicacy-backup backup --target onsite-usb --json-summary --dry-run homes

# Config command
duplicacy-backup config validate --target onsite-usb homes

# Health command
duplicacy-backup health status --target onsite-usb homes

# Redacted support bundle
duplicacy-backup diagnostics --target onsite-usb homes

# Label-scoped notification test
duplicacy-backup notify test --target onsite-usb homes

# Guided operator restore
duplicacy-backup restore select --target onsite-usb homes

# Start the picker from a useful subtree in a large backup
duplicacy-backup restore select --target onsite-usb --path-prefix phillipmcmahon/code homes

# Expert restore primitives
duplicacy-backup restore plan --target onsite-usb homes
duplicacy-backup restore list-revisions --target onsite-usb homes
duplicacy-backup restore run --target onsite-usb --revision 2403 --path docs/readme.md --yes homes

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
- `restore list-revisions` is a read-only discovery command. It creates a
  temporary Duplicacy workspace unless `--workspace` is supplied.
- `restore run` prepares the drill workspace when needed, executes
  `duplicacy restore` only inside that workspace, and never copies data back.
  If `--workspace` is omitted, the workspace is derived from the restore job:
  `/volume1/restore-drills/<label>-<target>-<restore-point-timestamp>-rev<id>`.
  Use `--workspace-root` to choose the parent folder while keeping the derived
  restore-job folder name. The root must already exist and remains
  operator-managed, which is the recommended pattern for Synology shared-folder
  visibility. Use `--workspace` only when you want an exact workspace path; it
  cannot be combined with `--workspace-root`.
  Use a file path for one file or a Duplicacy pattern such as `docs/*` for a
  subtree. During execution, restore progress is printed to stderr while the
  final report remains on stdout. Successful restores show a compact Duplicacy
  summary instead of every downloaded chunk or file; failed restores keep
  Duplicacy diagnostic lines when emitted, or an explicit no-diagnostics
  message when Duplicacy fails without useful detail.
- Restore-only disaster recovery access does not require `source_path`.
  `source_path` is only live-source and copy-back context.
- `restore select` is the primary operator restore path. It first presents
  restore points, then offers inspect-only, full restore, or tree-based
  selective restore. For restore actions, it previews the explicit commands,
  asks for confirmation, and delegates each selected path or pattern to
  `restore run`. The picker uses an
  interactive tree: use the arrow keys to move, `Right` to expand, `Left` to
  collapse, `Space` to select or clear the current file or subtree, `Tab` to
  inspect the primitive detail pane, `g` to continue, and `q` to cancel. The
  text prompts also accept `q` to cancel before execution.
- `restore plan`, `restore list-revisions`, and `restore run`
  remain the expert and scriptable restore primitives.
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
  `minio://`, and `minios://` storage values. Native `storj://` storage uses
  `storj_key` and `storj_passphrase`. See
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
- Safe restore planning, selection, and workspace execution:
  [restore-drills.md](restore-drills.md)
- Update checksum and attestation trust model:
  [update-trust-model.md](update-trust-model.md)
