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
- INI-based per-source configuration

## Quick Start

### 1. Build

```bash
# Current platform
make build

# Synology targets
make synology
```

Build outputs are written to `build/`.

### 2. Deploy

```bash
scp build/duplicacy-backup-linux-amd64 admin@synology:/usr/local/bin/duplicacy-backup
ssh admin@synology 'chmod +x /usr/local/bin/duplicacy-backup'
```

### 3. Create config

By default the binary looks for:

```text
<binary-dir>/.config/<source>-backup.conf
```

Example:

```bash
mkdir -p /usr/local/bin/.config
cp examples/homes-backup.conf /usr/local/bin/.config/homes-backup.conf
```

For remote mode, create a matching secrets file under `/root/.secrets`:

```bash
cp examples/duplicacy-homes.env.example /root/.secrets/duplicacy-homes.env
chown root:root /root/.secrets/duplicacy-homes.env
chmod 600 /root/.secrets/duplicacy-homes.env
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
- [Operations](docs/operations.md)
- [Contributing](CONTRIBUTING.md)
- [Testing](TESTING.md)
- [Message style guide](MESSAGE_STYLE_GUIDE.md)

## Prerequisites

### Synology NAS

- Btrfs filesystem
- Duplicacy CLI installed and in `PATH`
- Root access for scheduled execution

### Build machine

- Go 1.24+
- `make`

## License

MIT
