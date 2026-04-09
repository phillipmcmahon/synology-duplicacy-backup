# synology-duplicacy-backup

[![Build Synology Binaries](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml/badge.svg)](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml)

A compiled Go replacement for `duplicacy-backup.sh`.

It runs [Duplicacy](https://duplicacy.com/) backups on Synology NAS using btrfs snapshots, with support for local and remote (Storj S3) targets, safe prune thresholds, optional permission fixing, and directory-based locking.

The project builds as a single static binary for Synology-targeted Linux architectures.

## Highlights

- Read-only btrfs snapshots for consistent backups
- Local and remote backup modes
- Safe prune thresholds with optional force override
- Optional local permission normalisation
- Dry-run support for previewing actions
- Structured logging with rotation
- TOML-based per-source configuration

## Quick Start

### 1. Build or download

```bash
# Current platform
make build

# Synology targets
make synology
```

Build outputs are written to `build/`, and GitHub releases publish packaged
Synology tarballs.

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

### 4. Run

```bash
# Backup (default mode)
sudo duplicacy-backup homes

# Safe prune
sudo duplicacy-backup --prune homes

# Remote backup
sudo duplicacy-backup --remote homes

# Preview only
sudo duplicacy-backup --dry-run homes
```

## Common Commands

```bash
# Explicit backup
duplicacy-backup --backup homes

# Deep prune
duplicacy-backup --prune-deep --force-prune homes

# Fix permissions only
duplicacy-backup --fix-perms homes

# Custom config directory
duplicacy-backup --config-dir /opt/etc homes
```

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
