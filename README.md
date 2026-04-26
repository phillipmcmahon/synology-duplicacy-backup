# synology-duplicacy-backup

[![Build Synology Binaries](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml/badge.svg)](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml)

A Synology-focused operations wrapper around [Duplicacy](https://duplicacy.com/).
The name is historical: backup is still the core scheduled workload, but the
application now also owns restore drills, health checks, diagnostics, managed
updates, and rollback workflows.

It runs Duplicacy backups on Synology NAS using btrfs snapshots, with support
for named per-label targets, threshold-guarded prune, optional permission
fixing, and directory-based locking. The same command surface also gives
operators safe restore workflows, read-only repository health checks, redacted
support bundles, and managed install maintenance.

The project builds as a single static binary for Synology-targeted Linux architectures.

## Production Requirements

This tool is production-scoped to Synology DSM with Btrfs-backed `/volume*`
storage. Backups require the configured `source_path` to be a Btrfs volume or
subvolume root because snapshot consistency is a correctness requirement, not
an optional optimisation.

Operational commands refuse to run on non-Synology systems. The Linux container
workflow in [`docs/linux-environment.md`](docs/linux-environment.md) is for
development, validation, and packaging only; it is not a generic-Linux
production support statement.

## Target Model

Each named target describes two things:

- `storage`: the complete Duplicacy storage value
- `location`: where that storage lives operationally

Supported locations are:

- `location = "local"`
- `location = "remote"`

This lets the tool pass every storage backend directly to Duplicacy, including
local disk paths, S3-compatible services such as Storj gateway, and local
S3-compatible services such as RustFS or MinIO. `location` remains useful for
operator scheduling, reporting, and deciding whether local permission-management
operations are appropriate.

In practice:

- targets use `storage = "..."`; do not split storage into `destination` and `repository`
- runtime keys live under `[targets.<name>.keys]` in the secrets file and are
  loaded for known Duplicacy backends that require them; S3-compatible
  Duplicacy schemes `s3://`, `s3c://`, `minio://`, and `minios://` use
  `s3_id` and `s3_secret`, while native `storj://` uses `storj_key` and
  `storj_passphrase`
- `fix-perms` is only supported for path-based Duplicacy storage targets
- runtime, health, `config explain`, and `config paths` surface target
  location in operator-facing output

## Highlights

- Read-only btrfs snapshots for consistent backups
- Named targets for onsite and offsite backups
- Guided full and selective restore drills into safe workspaces
- Read-only health, doctor, and verify checks for automation
- Redacted diagnostics bundles for operator support
- Managed update and rollback commands for installed NAS deployments
- Threshold-guarded prune with optional forced override
- Optional ownership and permission normalisation
- Dry-run support for previewing actions
- Structured logging with rotation
- TOML-based per-label configuration with named targets

## Quick Start

### 1. Build or download

```bash
# Current platform
make build

# Synology targets
make synology
```

Build outputs are written to `build/`, and GitHub releases publish packaged
Synology tarballs. Before release, CI smoke-tests each packaged tarball by
checking the archive contents, checksum validation, binary `--version` and
`--help`, and installer `--help`.

Release preparation should follow [`docs/release-playbook.md`](docs/release-playbook.md).
The standard Linux validation and packaging environment is documented in
[`docs/linux-environment.md`](docs/linux-environment.md).

### 2. Install on Synology

```bash
# After extracting a release tarball on the NAS
sudo ./install.sh
```

See [`docs/operations.md`](docs/operations.md) for the recommended install
layout and upgrade workflow. See [`docs/privilege-model.md`](docs/privilege-model.md)
for the non-root default model and the commands that still require root.
For the breaking v8 runtime-profile migration, use
[`docs/v8-migration.md`](docs/v8-migration.md).

### 3. Create config

The default config location is user-owned:

```text
$HOME/.config/duplicacy-backup/<label>-backup.toml
```

Example:

```bash
mkdir -p "$HOME/.config/duplicacy-backup"
cp examples/homes-backup.toml "$HOME/.config/duplicacy-backup/homes-backup.toml"
chmod 700 "$HOME/.config/duplicacy-backup"
chmod 600 "$HOME/.config/duplicacy-backup/homes-backup.toml"
```

The installer manages the binary only. For upgrades from the old root-era
layout, follow [`docs/v8-migration.md`](docs/v8-migration.md). The migration
helper is packaged in the release tarball; it is not installed by
`duplicacy-backup update`.

For labels with Duplicacy storage targets, or for any target that needs
authenticated notification delivery, create a matching label secrets file under
`$HOME/.config/duplicacy-backup/secrets` and add target-specific entries inside
it:

```bash
mkdir -p "$HOME/.config/duplicacy-backup/secrets"
cp examples/homes-secrets.toml "$HOME/.config/duplicacy-backup/secrets/homes-secrets.toml"
chmod 700 "$HOME/.config/duplicacy-backup/secrets"
chmod 600 "$HOME/.config/duplicacy-backup/secrets/homes-secrets.toml"
```

Duplicacy storage keys live under `[targets.<name>.keys]` and are written to
the generated Duplicacy preferences file using the exact key names Duplicacy
expects, such as `s3_id` and `s3_secret` for S3-compatible storage.

The matching backup TOML models targets explicitly with `location` and
`storage`. For example:

```toml
[targets.onsite-usb]
location = "local"
storage = "/volumeUSB1/usbshare/duplicacy/homes"
allow_local_accounts = true
local_owner = "myuser"
local_group = "users"

[targets.offsite-usb]
location = "remote"
storage = "/volume1/duplicacy/duplicacy/homes"
allow_local_accounts = true
local_owner = "myuser"
local_group = "users"

[targets.offsite-storj]
location = "remote"
storage = "s3://gateway.storjshare.io/my-backup-bucket/homes"

[targets.onsite-rustfs]
location = "local"
storage = "s3://rustfs.local/my-backup-bucket/homes"
```

### 4. Validate and run

Start with validation and a dry run before scheduling anything:

```bash
# Validate the selected target from the homes config
duplicacy-backup config validate --target onsite-usb homes

# Preview a backup without changing storage
sudo duplicacy-backup backup --target onsite-usb --dry-run homes

# Run a backup
sudo duplicacy-backup backup --target onsite-usb homes

# Check backup freshness and repository status
duplicacy-backup health status --target onsite-usb homes

# Gather a redacted support bundle for one label and target
duplicacy-backup diagnostics --target onsite-usb homes

# Start the guided operator restore flow
duplicacy-backup restore select --target onsite-usb homes
duplicacy-backup restore select --target onsite-usb --path-prefix phillipmcmahon/code homes

# Expert or scripted restore path
duplicacy-backup restore plan --target onsite-usb homes
duplicacy-backup restore list-revisions --target onsite-usb homes
duplicacy-backup restore run --target onsite-usb --revision 2403 --path docs/readme.md --yes homes
```

When a root-required command is invoked with normal `sudo` metadata from the
operator account, default config, secrets, log, state, and lock paths still
resolve to the operator profile. A direct root shell, or a stripped environment
without complete sudo metadata, uses `/root` unless explicit profile paths are
supplied.

For day-to-day commands, use the [operator cheat sheet](docs/cheatsheet.md). For
complete syntax, use the [CLI reference](docs/cli.md). For recurring Synology
Task Scheduler jobs, prefer separate scheduled tasks for backup, prune, health,
and fix-perms; see [Workflow and scheduling](docs/workflow-scheduling.md).

## Operator Map

Use the documentation by task:

| Task | Start here |
|---|---|
| Install or upgrade the binary | [Operations](docs/operations.md) |
| Confirm platform and storage requirements | [Requirements](docs/requirements.md) |
| Run common commands | [Operator cheat sheet](docs/cheatsheet.md) |
| Diagnose failed runs or confusing status output | [Troubleshooting](docs/troubleshooting.md) |
| Check exact CLI syntax | [CLI reference](docs/cli.md) |
| Configure labels, targets, health, notifications, and secrets | [Configuration and secrets](docs/configuration.md) |
| Plan Synology Task Scheduler jobs | [Workflow and scheduling](docs/workflow-scheduling.md) |
| Restore onto a replacement NAS | [Restore onto a new NAS](docs/new-nas-restore.md) |
| Practise full or selective restores safely | [Restore drills](docs/restore-drills.md) |
| Understand update trust and attestations | [Update trust model](docs/update-trust-model.md) |
| Prepare or verify a release | [Release playbook](docs/release-playbook.md) |

Core operating rules:

- `backup`, `prune`, `cleanup-storage`, `fix-perms`, `config`,
  `diagnostics`, `health`, restore commands, and label-scoped `notify test`
  commands require an explicit `--target <name>`.
- Runtime operations are first-class commands. Use `backup`, `prune`,
  `cleanup-storage`, or `fix-perms`; old top-level operation flags are not
  supported.
- Storage keys are loaded for known Duplicacy backends that require them.
- `fix-perms` applies only to path-based Duplicacy storage targets.
- `--json-summary` writes machine-readable output to stdout while human logs
  stay on stderr.
- `health status`, `health doctor`, and `health verify` use target-specific
  state under `$HOME/.local/state/duplicacy-backup/state/<label>.<target>.json`
  by default.
- `diagnostics` prints a redacted label-target support bundle with resolved
  paths, storage scheme, state freshness, and permission summaries.
- `restore select` is the primary operator restore path. It presents restore
  points first, then supports inspect-only, full restore, or tree-based
  selective restore. It previews the exact commands, asks for confirmation,
  and delegates to `restore run`, which prepares the drill workspace when
  needed.
- `restore plan` is read-only. It prints the resolved context and Duplicacy
  commands for a safe drill workspace, but it does not execute restores.
- `restore list-revisions` is read-only. It lists visible backup revisions
  without restoring data.
- `restore run` prepares or reuses a drill workspace, restores only there, and
  never copies data back to the live source. Use `--path` for one file or a
  Duplicacy pattern such as `docs/*` for a subtree.
- Restore workspaces default to
  `/volume1/restore-drills/<label>-<target>-<restore-point-timestamp>-rev<id>`.
  `source_path` is only live-source and copy-back context. Use
  `--workspace-root` to place derived job folders under an existing
  operator-managed shared-folder root.
- `restore select` uses a tree picker with arrow-key navigation, `Space` to
  toggle files or subtrees, `Tab` to inspect the primitive detail pane, `g`
  to continue with the current selection and generate the restore commands,
  and `q` to cancel. Restore-select text prompts also accept `q` before
  execution. During an active restore, `Ctrl-C` cancels the running Duplicacy
  process, keeps the drill workspace, does not delete restored files, and
  reports completed paths plus the active path.
- `restore run` prints coloured status/progress to stderr during execution and
  leaves the final restore report on stdout. Successful reports keep Duplicacy
  totals and suppress per-chunk or per-file download noise; failures keep
  Duplicacy diagnostic lines when they are emitted, otherwise they state that
  Duplicacy did not provide diagnostics.
- `restore plan`, `restore list-revisions`, and `restore run`
  remain the expert and scriptable restore primitives.
- Health and selected runtime notifications are configured under
  `[health.notify]` in the label config.
- `update --check-only` is safe for routine inspection of published updates.
- `update` keeps the newly activated binary and one previous binary by default;
  use `--keep <count>` if you want a different local rollback window.
- `rollback --check-only` inspects retained managed-install versions;
  `rollback --yes` activates the newest previous retained version.

## Documentation

- [Operator cheat sheet](docs/cheatsheet.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Operations](docs/operations.md)
- [Requirements](docs/requirements.md)
- [Configuration and secrets](docs/configuration.md)
- [v8 migration guide](docs/v8-migration.md)
- [Workflow and scheduling](docs/workflow-scheduling.md)
- [Restore onto a new NAS](docs/new-nas-restore.md)
- [Restore drills](docs/restore-drills.md)
- [CLI reference](docs/cli.md)
- [Update trust model](docs/update-trust-model.md)
- [Linux validation and packaging environment](docs/linux-environment.md)
- [Release playbook](docs/release-playbook.md)
- [Architecture](docs/architecture.md)
- [How it works internally](docs/how-it-works.md)
- [Contributing](CONTRIBUTING.md)
- [Testing](TESTING.md)

## Prerequisites

### Synology NAS

- Btrfs filesystem
- Duplicacy CLI installed and in `PATH`
- Root access for scheduled execution

### Build machine

- Go 1.26.x
- `make`

## License

MIT
