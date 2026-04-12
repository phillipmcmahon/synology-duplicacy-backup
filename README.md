# synology-duplicacy-backup

[![Build Synology Binaries](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml/badge.svg)](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml)

A compiled Go replacement for `duplicacy-backup.sh`.

It runs [Duplicacy](https://duplicacy.com/) backups on Synology NAS using btrfs snapshots, with support for named per-label targets, threshold-guarded prune, optional permission fixing, and directory-based locking.

The project builds as a single static binary for Synology-targeted Linux architectures.

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
Synology tarballs. CI smoke-tests each packaged tarball before release by
verifying archive contents, checksum validation, binary `--version` / `--help`,
and installer `--help`.

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
```

For labels with targets that need credentials, create a matching label secrets file
under `/root/.secrets` and add target-specific entries inside it:

```bash
cp examples/homes-secrets.toml /root/.secrets/homes-secrets.toml
chown root:root /root/.secrets/homes-secrets.toml
chmod 600 /root/.secrets/homes-secrets.toml
```

The current secrets TOML schema uses `storj_s3_id` and `storj_s3_secret`
because those values are passed through to Duplicacy for gateway-backed
S3-compatible storage.

### 4. Run

```bash
# Backup homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup homes

# Backup then safe prune homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup --prune homes

# Forced prune homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --prune --force-prune homes

# Storage cleanup for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --cleanup-storage homes

# Backup homes to target offsite-storj
sudo duplicacy-backup --target offsite-storj --backup homes

# Preview backing up homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --dry-run --backup homes

# Verbose backup and prune for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --verbose --backup --prune homes

# JSON summary for a dry-run backup of homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --json-summary --dry-run --backup homes

# Fast health summary for homes on target onsite-usb
sudo duplicacy-backup health status --target onsite-usb homes

# JSON doctor report for homes on target onsite-usb
sudo duplicacy-backup health doctor --json-summary --target onsite-usb homes
```

## Common Commands

```bash
# Backup homes to target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup homes

# Safe prune and storage cleanup for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --prune --cleanup-storage homes

# Fix permissions for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --fix-perms homes

# Backup, forced prune, storage cleanup, and fix permissions for homes on target onsite-usb
sudo duplicacy-backup --target onsite-usb --backup --prune --force-prune --cleanup-storage --fix-perms homes

# Backup homes to target onsite-usb using a custom config directory
duplicacy-backup --target onsite-usb --config-dir /opt/etc --backup homes

# Validate config for homes on target onsite-usb
sudo duplicacy-backup config validate --target onsite-usb homes

# Explain resolved values for homes on target offsite-storj
sudo duplicacy-backup config explain --target offsite-storj homes

# Show resolved paths for homes on target onsite-usb
duplicacy-backup config paths --target onsite-usb homes

# Fast health summary for homes on target onsite-usb
sudo duplicacy-backup health status --target onsite-usb homes

# Read-only doctor pass for homes on target onsite-usb
sudo duplicacy-backup health doctor --target onsite-usb homes

# Integrity check for homes on target onsite-usb
sudo duplicacy-backup health verify --target onsite-usb homes
```

When operations are combined, execution order is fixed:
`backup -> prune -> cleanup-storage -> fix-perms`.

Config commands are read-only helpers:
- `config validate` checks the selected target from a label config and any configured target secrets
- `config explain` shows the resolved values for the selected target
- `config paths` shows the resolved stable config, source, log, and any applicable secrets paths

Every runtime, `config`, and `health` command requires an explicit `--target <name>`.
Every runtime command must also include at least one explicit operation flag
such as `--backup`, `--prune`, `--cleanup-storage`, or `--fix-perms`.

Default output is phase-oriented and intentionally concise. Use `--verbose`
to include detailed operational logging and command details.

`--json-summary` adds a machine-readable completion summary on stdout while
keeping the human-readable phase logs on stderr.

The `health` command family adds read-only confidence checks:
- `health status` gives a fast current-state summary
- `health doctor` checks config, secrets, paths, btrfs prerequisites, locks, and storage reachability
- `health verify` goes further by validating the revisions found for the current backup with `duplicacy check -persist`

Health commands combine target-specific state stored under `/var/lib/duplicacy-backup/<label>.<target>.json`
with live Duplicacy storage inspection. When Duplicacy exposes revision creation
times, those storage timestamps are used as the authoritative freshness signal.
`health verify` also records how many revisions were checked, how many
passed, and which revisions failed integrity validation. The JSON report keeps
summary counts on healthy runs and includes per-revision detail when failures
need to be diagnosed. Unhealthy verify JSON also emits structured failure and
recommended-action codes for automation. Health JSON is machine-focused: it
emits summary fields, timestamps, and machine codes rather than the rendered
check lines shown in the interactive UI.

Health policy is configured per backup TOML in an optional top-level `[health]`
table, with optional per-target overrides under `[targets.<name>.health]`:
- `freshness_warn_hours`
- `freshness_fail_hours`
- `doctor_warn_after_hours`
- `verify_warn_after_hours`

Optional webhook notifications can be configured in `[health.notify]`, with
optional per-target overrides in `[targets.<name>.health.notify]`. An optional
`health_webhook_bearer_token` can be stored in the target secrets TOML.
Webhooks are intended for non-interactive health runs; interactive TTY runs do
not notify by default.

If the environment is broken early enough that the backup TOML cannot be read,
built-in webhook delivery is not expected to work because the webhook policy
itself lives in that config. For those hard-failure cases, rely on Synology
scheduled-task monitoring and its mail/notification integration as the primary
fallback alert path.

`--help` now shows a concise quick-reference view. Use `--help-full` for the
detailed CLI reference, and `config --help-full` for the detailed config
subcommand reference.

Interactive terminal runs ask for confirmation before forced prune and
storage cleanup. Non-interactive runs continue unchanged so scheduled jobs are
not blocked.

For the installed Synology layout, runtime operations and installed-config
inspection commands should normally be run with `sudo`. The main exception is
`config paths`, which is useful as a normal-user discovery command.

## Documentation

- [Desk cheat sheet](docs/cheatsheet.md)
- [CLI reference](docs/cli.md)
- [Configuration and secrets](docs/configuration.md)
- [Linux validation and packaging environment](docs/linux-environment.md)
- [Release playbook](docs/release-playbook.md)
- [Architecture](docs/architecture.md)
- [How it works internally](docs/how-it-works.md)
- [Operations](docs/operations.md)
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
