# synology-duplicacy-backup

[![Build Synology Binaries](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml/badge.svg)](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml)

A compiled Go replacement for `duplicacy-backup.sh`.

It runs [Duplicacy](https://duplicacy.com/) backups on Synology NAS using btrfs snapshots, with support for local and remote S3-compatible targets, threshold-guarded prune, optional permission fixing, and directory-based locking.

The project builds as a single static binary for Synology-targeted Linux architectures.

## Highlights

- Read-only btrfs snapshots for consistent backups
- Local and remote S3-compatible backup modes
- Threshold-guarded prune with optional forced override
- Optional local permission normalisation
- Dry-run support for previewing actions
- Structured logging with rotation
- TOML-based per-source configuration
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
/usr/local/lib/duplicacy-backup/.config/<source>-backup.toml
```

Example:

```bash
mkdir -p /usr/local/lib/duplicacy-backup/.config
cp examples/homes-backup.toml /usr/local/lib/duplicacy-backup/.config/homes-backup.toml
```

For remote mode, create a matching secrets file under `/root/.secrets`:

```bash
cp examples/duplicacy-homes.toml /root/.secrets/duplicacy-homes.toml
chown root:root /root/.secrets/duplicacy-homes.toml
chmod 600 /root/.secrets/duplicacy-homes.toml
```

The current remote TOML schema uses `storj_s3_id` and `storj_s3_secret`
because those values are passed through to Duplicacy for gateway-backed
S3-compatible storage.

### 4. Run

```bash
# Backup (default mode)
sudo duplicacy-backup homes

# Backup, then safe prune
sudo duplicacy-backup --backup --prune homes

# Forced prune
sudo duplicacy-backup --prune --force-prune homes

# Storage cleanup only
sudo duplicacy-backup --cleanup-storage homes

# Remote backup
sudo duplicacy-backup --remote homes

# Preview only
sudo duplicacy-backup --dry-run homes

# Detailed troubleshooting output
sudo duplicacy-backup --verbose --backup --prune homes

# Machine-readable completion summary on stdout
sudo duplicacy-backup --json-summary --dry-run homes

# Fast health summary
sudo duplicacy-backup health status homes

# Deeper diagnostic report in JSON
sudo duplicacy-backup health doctor --json-summary homes
```

## Common Commands

```bash
# Explicit backup
sudo duplicacy-backup --backup homes

# Safe prune + storage cleanup
sudo duplicacy-backup --prune --cleanup-storage homes

# Fix permissions only
sudo duplicacy-backup --fix-perms homes

# Backup, then forced prune, then storage cleanup, then fix permissions
sudo duplicacy-backup --backup --prune --force-prune --cleanup-storage --fix-perms homes

# Custom config directory
duplicacy-backup --config-dir /opt/etc homes

# Validate resolved installed config and secrets
sudo duplicacy-backup config validate homes

# Explain resolved installed remote config values
sudo duplicacy-backup config explain --remote homes

# Show resolved stable config, secrets, source, and log paths
duplicacy-backup config paths homes

# Fast read-only health summary
sudo duplicacy-backup health status homes

# Read-only diagnostic pass
sudo duplicacy-backup health doctor homes

# Storage confidence check
sudo duplicacy-backup health verify homes
```

When operations are combined, execution order is fixed:
`backup -> prune -> cleanup-storage -> fix-perms`.

Config commands are read-only helpers:
- `config validate` checks the resolved TOML and any configured remote secrets
- `config explain` shows the resolved values for local mode by default, or remote mode with `--remote`
- `config paths` shows the resolved stable config, secrets, source, and log paths

Default output is phase-oriented and intentionally concise. Use `--verbose`
to include detailed operational logging and command details.

`--json-summary` adds a machine-readable completion summary on stdout while
keeping the human-readable phase logs on stderr.

The `health` command family adds read-only confidence checks:
- `health status` gives a fast current-state summary
- `health doctor` checks config, secrets, paths, btrfs prerequisites, locks, and storage reachability
- `health verify` goes further by checking visible storage revisions and freshness against health policy thresholds

Health commands combine local state stored under `/var/lib/duplicacy-backup/<label>.json`
with live Duplicacy storage inspection. When Duplicacy exposes revision creation
times, those storage timestamps are used as the authoritative freshness signal.

Health policy is configured per backup TOML in an optional `[health]` table:
- `freshness_warn_hours`
- `freshness_fail_hours`
- `doctor_warn_after_hours`
- `verify_warn_after_hours`

Optional webhook notifications can be configured in `[health.notify]`, with an
optional `health_webhook_bearer_token` stored in the secrets TOML. Webhooks are
intended for non-interactive health runs; interactive TTY runs do not notify by
default.

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

- [CLI reference](docs/cli.md)
- [Configuration and secrets](docs/configuration.md)
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
