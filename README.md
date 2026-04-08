# synology-duplicacy-backup

[![Build Synology Binaries](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml/badge.svg)](https://github.com/phillipmcmahon/synology-duplicacy-backup/actions/workflows/build.yml)

A compiled Go replacement for the `duplicacy-backup.sh` script. Performs [Duplicacy](https://duplicacy.com/) backups on Synology NAS using **btrfs snapshots**, with support for local and remote (Storj S3) backup targets, safe pruning with threshold guards, and directory-based concurrency locking.

Produces a **single static binary** — no dependencies, no shell interpreter, no runtime needed on the Synology.

---

## Features

| Feature | Description |
|---|---|
| **BTRFS snapshots** | Creates read-only btrfs snapshots before backup for point-in-time consistency |
| **Local & remote modes** | Backs up to local paths or S3-compatible remote storage (Storj) |
| **Safe pruning** | Threshold-guarded prune with configurable max delete count and percentage |
| **Deep pruning** | Maintenance mode with exhaustive + exclusive prune (requires `--force-prune`) |
| **Concurrency control** | Directory-based PID locking prevents concurrent runs for the same label |
| **Permission fixing** | Optional ownership/permission normalisation for local repositories |
| **Dry-run mode** | Simulate all operations without making changes |
| **Structured logging** | Timestamped logs to `/var/log/` with automatic rotation |
| **INI-style config** | Per-source config files with `[common]`, `[local]`, and `[remote]` sections |
| **Secrets management** | Strict per-repo secrets files with permission/ownership validation |
| **Cross-compilation** | Build for `linux/amd64`, `linux/arm64`, and `linux/armv7` (all Synology models) |

---

## Quick Start

### 1. Build

```bash
# Build for your current platform
make build

# Cross-compile for all Synology architectures
make synology

# Build for a specific Synology architecture
make synology-amd64    # DS920+, DS1621+, DS1821+, etc.
make synology-arm64    # DS223, DS423, etc.
make synology-arm      # Older ARM-based models
```

Binaries are output to the `build/` directory.

### 2. Deploy to Synology

```bash
# Copy binary to your NAS
scp build/duplicacy-backup-linux-amd64 admin@synology:/usr/local/bin/duplicacy-backup
ssh admin@synology 'chmod +x /usr/local/bin/duplicacy-backup'
```

### 3. Create Configuration

Place your configuration file alongside the binary in a `.config/` subdirectory:

```bash
# On the Synology – assuming the binary is at /usr/local/bin/duplicacy-backup
mkdir -p /usr/local/bin/.config

# Create config (see examples/homes-backup.conf)
vi /usr/local/bin/.config/homes-backup.conf
```

The default config directory is `<binary-dir>/.config/`, so the config file travels with the binary. For example, if the binary is at `/volume1/homes/user/bin/duplicacy-backup`, the config file is expected at `/volume1/homes/user/bin/.config/homes-backup.conf`.

> **Override:** Use `--config-dir <path>` or set `DUPLICACY_BACKUP_CONFIG_DIR` to use a different directory.

### 4. Set Up Secrets (Remote Mode Only)

```bash
mkdir -p /root/.secrets
vi /root/.secrets/duplicacy-homes.env

# Set strict permissions
chown root:root /root/.secrets/duplicacy-homes.env
chmod 600 /root/.secrets/duplicacy-homes.env
```

> **Override:** Use `--secrets-dir <path>` or set `DUPLICACY_BACKUP_SECRETS_DIR` to use a different directory.

### 5. Run

```bash
# Backup (default mode)
sudo duplicacy-backup homes

# Prune with safety thresholds
sudo duplicacy-backup --prune homes

# Remote backup
sudo duplicacy-backup --remote homes

# Dry-run to preview actions
sudo duplicacy-backup --dry-run homes
```

---

## Command Reference

```
Usage: duplicacy-backup [OPTIONS] <source>

DEFAULT BEHAVIOUR:
    No mode specified = backup only

MODES:
    --backup                 Perform backup only
    --prune                  Perform safe, threshold-guarded policy prune only
    --prune-deep             Perform maintenance prune mode (requires --force-prune):
                             policy prune + exhaustive exclusive prune

MODIFIERS:
    --fix-perms              Normalise local repository ownership and permissions
    --force-prune            Override safe prune thresholds, or authorise --prune-deep
    --remote                 Perform operation against remote target config
    --dry-run                Simulate actions without making changes
    --config-dir <path>      Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>     Override secrets directory (default: /root/.secrets)
    --help                   Show this help message

ENVIRONMENT VARIABLES:
    DUPLICACY_BACKUP_CONFIG_DIR   Override config directory (--config-dir takes precedence)
    DUPLICACY_BACKUP_SECRETS_DIR  Override secrets directory (--secrets-dir takes precedence)
```

### Examples

```bash
# Basic backup of /volume1/homes
duplicacy-backup homes

# Explicit backup mode
duplicacy-backup --backup homes

# Safe prune (respects thresholds)
duplicacy-backup --prune homes

# Force prune (override thresholds)
duplicacy-backup --prune --force-prune homes

# Deep maintenance prune
duplicacy-backup --prune-deep --force-prune homes

# Fix local permissions
duplicacy-backup --fix-perms homes

# Remote backup with dry-run
duplicacy-backup --remote --dry-run homes

# Use custom config directory
duplicacy-backup --config-dir /opt/etc homes

# Use custom secrets directory for remote
duplicacy-backup --secrets-dir /opt/secrets --remote homes
```

---

## Configuration

### Config File Format

INI-style with `[common]`, `[local]`, and `[remote]` sections. The binary looks for config files relative to the executable's location:

```
<binary-dir>/.config/<source>-backup.conf
```

This means the config directory travels with the binary — useful for portable Synology deployments. Override with `--config-dir <path>` or `DUPLICACY_BACKUP_CONFIG_DIR` environment variable.

### Config Keys

| Key | Required | Description |
|---|---|---|
| `DESTINATION` | Yes | Backup target path or S3 URL |
| `THREADS` | Yes (backup) | Number of threads (power of 2, max 16) |
| `PRUNE` | Yes (prune) | Duplicacy prune retention arguments |
| `FILTER` | No | Duplicacy filter pattern using regex syntax (`e:` prefix to exclude, `i:` to include) |
| `LOCAL_OWNER` | **Yes** | Owner of local backup files (e.g. `myuser`). **Must not be root** — the script runs as root but files should be owned by a regular user for security. |
| `LOCAL_GROUP` | **Yes** | Group for local backup files (e.g. `users`). |
| `LOG_RETENTION_DAYS` | No | Days to keep log files (default: `30`) |
| `SAFE_PRUNE_MAX_DELETE_PERCENT` | No | Max deletion percentage (default: `10`) |
| `SAFE_PRUNE_MAX_DELETE_COUNT` | No | Max deletion count (default: `25`) |
| `SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT` | No | Min revisions before % check applies (default: `20`) |

### Example Config

```ini
[common]
PRUNE=-keep 1:728 -keep 91:364 -keep 28:182 -keep 7:28
# Duplicacy filter pattern (regex syntax: "e:" = exclude, "i:" = include)
# Use "|" to combine multiple patterns in a single expression.
# See: https://github.com/gilbertchen/duplicacy/wiki/Include-Exclude-Patterns
FILTER=e:^(.*/)?(@eaDir|#recycle|tmp|exclude)/$|^(.*/)?(\.DS_Store|\._.*|Thumbs\.db)$
LOCAL_OWNER=myuser
LOCAL_GROUP=users
LOG_RETENTION_DAYS=30

[local]
DESTINATION=/volume2/backups
THREADS=4

[remote]
DESTINATION=s3://gateway.storjshare.io/my-backup-bucket
THREADS=8
```

---

## Secrets File Format

For remote mode, secrets are loaded from `/root/.secrets/duplicacy-<label>.env`:

```env
STORJ_S3_ID=your-access-key-id
STORJ_S3_SECRET=your-secret-access-key
```

**Requirements:**
- File must be owned by `root:root`
- File permissions must be `0600`
- Only `STORJ_S3_ID` and `STORJ_S3_SECRET` keys are allowed
- `STORJ_S3_ID` must be at least 28 characters
- `STORJ_S3_SECRET` must be at least 53 characters

---

## Safe Prune Thresholds

The safe prune system prevents accidental mass deletion of revisions:

| Threshold | Default | Description |
|---|---|---|
| Max delete count | 25 | Maximum revisions a single prune can delete |
| Max delete percent | 10% | Maximum percentage of total revisions to delete |
| Min total for % check | 20 | Minimum revisions before percentage check is enforced |

Use `--force-prune` to override these thresholds when needed.

---

## Directory Structure

```
synology-duplicacy-backup/
├── cmd/
│   └── duplicacy-backup/
│       └── main.go              # Entry point and CLI orchestration
├── internal/
│   ├── btrfs/
│   │   └── btrfs.go             # BTRFS volume checks and snapshot management
│   ├── config/
│   │   └── config.go            # INI config parser and validation
│   ├── duplicacy/
│   │   └── duplicacy.go         # Duplicacy CLI wrapper (backup, prune, list)
│   ├── lock/
│   │   └── lock.go              # Directory-based PID locking
│   ├── logger/
│   │   └── logger.go            # Structured logging with colour and rotation
│   ├── permissions/
│   │   └── permissions.go       # Local repo ownership/permission fixing
│   └── secrets/
│       └── secrets.go           # Secrets file loading and validation
├── examples/
│   ├── homes-backup.conf        # Example configuration file
│   └── duplicacy-homes.env.example  # Example secrets file
├── CHANGELOG.md                 # Release history
├── LICENSE                      # MIT license
├── Makefile                     # Build targets including Synology cross-compilation
├── go.mod                       # Go module definition
├── .gitignore
└── README.md
```

---

## Prerequisites

### Build Machine
- Go 1.24+ (for cross-compilation)
- `make`

### Synology NAS
- BTRFS filesystem (most Synology models with DSM 7+)
- [Duplicacy CLI](https://duplicacy.com/) installed and accessible in `$PATH`
- Root access

---

## Scheduling (Synology Task Scheduler)

1. Open **Control Panel > Task Scheduler**
2. Create > **Triggered Task > User-defined script**
3. Set schedule (e.g., daily at 2:00 AM)
4. User: `root`
5. Script:

```bash
/usr/local/bin/duplicacy-backup homes
```

For remote backup + local prune in sequence:

```bash
/usr/local/bin/duplicacy-backup --remote homes && /usr/local/bin/duplicacy-backup --prune homes
```

---

## License

MIT