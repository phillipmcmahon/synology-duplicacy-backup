# synology-duplicacy-backup

[![Build Synology Binaries](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml/badge.svg)](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml)

A Go replacement for `duplicacy-backup.sh`.

It runs [Duplicacy](https://duplicacy.com/) backups on Synology NAS using btrfs snapshots, with support for named per-label targets, threshold-guarded prune, optional permission fixing, and directory-based locking.

The project builds as a single static binary for Synology-targeted Linux architectures.

## Target Model

Each named target now describes two separate things:

- `type`: the storage mechanics
- `location`: where that storage lives operationally

Supported combinations are:

- `type = "filesystem"` with `location = "local"`
- `type = "filesystem"` with `location = "remote"`
- `type = "object"` with `location = "remote"`

This lets the tool represent cases such as a mounted SMB path over VPN as a
real remote destination without pretending it is object storage or forcing it
into the old `local` shortcut.

In practice:

- filesystem targets use path semantics and do not load secrets
- object targets use URL-style storage semantics and do load secrets
- `--fix-perms` is only supported for filesystem targets
- runtime, health, `config explain`, and `config paths` now surface `Type` and
  `Location` in operator-facing output

## Highlights

- Read-only btrfs snapshots for consistent backups
- Named targets for onsite and offsite backups
- Threshold-guarded prune with optional forced override
- Optional ownership and permission normalisation
- Dry-run support for previewing actions
- Structured logging with rotation
- TOML-based per-label configuration with named targets
- Read-only health, doctor, and verify checks for automation

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
layout and upgrade workflow.

### 3. Create config

With the recommended installer layout, the default config location is:

```text
/usr/local/lib/duplicacy-backup/.config/<label>-backup.toml
```

Example:

```bash
mkdir -p /usr/local/lib/duplicacy-backup/.config
cp examples/homes-backup.toml /usr/local/lib/duplicacy-backup/.config/homes-backup.toml
chown root:administrators /usr/local/lib/duplicacy-backup/.config
chmod 750 /usr/local/lib/duplicacy-backup/.config
chown root:administrators /usr/local/lib/duplicacy-backup/.config/homes-backup.toml
chmod 640 /usr/local/lib/duplicacy-backup/.config/homes-backup.toml
```

When it is run as `root`, the Synology installer also:

- normalises `.config` to `root:administrators` with mode `750`
- normalises any existing `*-backup.toml` files there to mode `640`
- ensures `/root/.secrets` exists as `root:root` with mode `700`

Use `./install.sh --config-group <name>` if you want a different trusted
operator group for config access. The installer never creates or rewrites
individual secrets files.

For labels with object-storage targets, or for any target that needs
authenticated notification delivery, create a matching label secrets file under
`/root/.secrets` and add target-specific entries inside it:

```bash
mkdir -p /root/.secrets
chown root:root /root/.secrets
chmod 700 /root/.secrets
cp examples/homes-secrets.toml /root/.secrets/homes-secrets.toml
chown root:root /root/.secrets/homes-secrets.toml
chmod 600 /root/.secrets/homes-secrets.toml
```

The current secrets TOML schema uses `storj_s3_id` and `storj_s3_secret`
because those values are passed through to Duplicacy for gateway-backed
S3-compatible storage.

The matching backup TOML now models targets explicitly with `type` and
`location`. For example:

```toml
[targets.onsite-usb]
type = "filesystem"
location = "local"
destination = "/volumeUSB1/usbshare/duplicacy"
repository = "homes"
allow_local_accounts = true
local_owner = "myuser"
local_group = "users"

[targets.offsite-usb]
type = "filesystem"
location = "remote"
destination = "/volume1/duplicacy/duplicacy"
repository = "homes"
allow_local_accounts = true
local_owner = "myuser"
local_group = "users"

[targets.offsite-storj]
type = "object"
location = "remote"
destination = "s3://gateway.storjshare.io/my-backup-bucket"
repository = "homes"
```

### 4. Validate and run

Start with validation and a dry run before scheduling anything:

```bash
# Validate the selected target from the homes config
sudo duplicacy-backup config validate --target onsite-usb homes

# Preview a backup without changing storage
sudo duplicacy-backup --target onsite-usb --dry-run --backup homes

# Run a backup
sudo duplicacy-backup --target onsite-usb --backup homes

# Check backup freshness and repository status
sudo duplicacy-backup health status --target onsite-usb homes
```

For day-to-day commands, use the [desk cheat sheet](docs/cheatsheet.md). For
complete syntax, use the [CLI reference](docs/cli.md). For recurring Synology
Task Scheduler jobs, prefer separate scheduled tasks for backup, prune, health,
and fix-perms; see [Workflow and scheduling](docs/workflow-scheduling.md).

## Operator Map

Use the documentation by task:

| Task | Start here |
|---|---|
| Install or upgrade the binary | [Operations](docs/operations.md) |
| Run common commands | [Desk cheat sheet](docs/cheatsheet.md) |
| Diagnose failed runs or confusing status output | [Troubleshooting](docs/troubleshooting.md) |
| Check exact CLI syntax | [CLI reference](docs/cli.md) |
| Configure labels, targets, health, notifications, and secrets | [Configuration and secrets](docs/configuration.md) |
| Plan Synology Task Scheduler jobs | [Workflow and scheduling](docs/workflow-scheduling.md) |
| Understand update trust and attestations | [Update trust model](docs/update-trust-model.md) |
| Prepare or verify a release | [Release playbook](docs/release-playbook.md) |

Core operating rules:

- Runtime, `config`, `health`, and label-scoped `notify test` commands require
  an explicit `--target <name>`.
- Runtime commands also require at least one operation flag such as `--backup`,
  `--prune`, `--cleanup-storage`, or `--fix-perms`.
- When operations are combined, execution order is fixed:
  `backup -> prune -> cleanup-storage -> fix-perms`.
- Object targets load storage secrets; filesystem targets do not.
- `--fix-perms` applies only to filesystem targets.
- `--json-summary` writes machine-readable output to stdout while human logs
  stay on stderr.
- `health status`, `health doctor`, and `health verify` use target-specific
  state under `/var/lib/duplicacy-backup/<label>.<target>.json`.
- Health and selected runtime notifications are configured under
  `[health.notify]` in the label config.
- `update --check-only` is safe for routine inspection of published updates.
- `update` keeps the newly activated binary and one previous binary by default;
  use `--keep <count>` if you want a different local rollback window.

## Documentation

- [Desk cheat sheet](docs/cheatsheet.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Operations](docs/operations.md)
- [Configuration and secrets](docs/configuration.md)
- [Workflow and scheduling](docs/workflow-scheduling.md)
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

- Go 1.26+
- `make`

## License

MIT
