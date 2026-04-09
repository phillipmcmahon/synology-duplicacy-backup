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
                             (standalone or combined with --backup/--prune; local only)
    --force-prune            Override safe prune thresholds, or authorise --prune-deep
    --remote                 Perform operation against remote target config
    --dry-run                Simulate actions without making changes
    --config-dir <path>      Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>     Override secrets directory (default: /root/.secrets)
    --version, -v            Show version and build information
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

# Fix local permissions only (standalone, no backup)
duplicacy-backup --fix-perms homes

# Backup and fix permissions
duplicacy-backup --fix-perms --backup homes

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
| `LOCAL_OWNER` | **Yes** (local only) | Owner of local backup files (e.g. `myuser`). **Must not be root** — the script runs as root but files should be owned by a regular user for security. Only allowed in `[local]` section; rejected in `[common]` and `[remote]`. |
| `LOCAL_GROUP` | **Yes** (local only) | Group for local backup files (e.g. `users`). Only allowed in `[local]` section; rejected in `[common]` and `[remote]`. |
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
LOG_RETENTION_DAYS=30

[local]
DESTINATION=/volume2/backups
THREADS=4
# LOCAL_OWNER and LOCAL_GROUP are LOCAL-only settings — they control file
# ownership for local backup repositories and are not used in remote mode.
LOCAL_OWNER=myuser
LOCAL_GROUP=users

[remote]
# No LOCAL_OWNER/LOCAL_GROUP here — they are not applicable to remote targets.
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

## Conditional Validations

To reduce startup overhead, certain validations only run when they are actually
needed:

| Validation | Runs When | Skipped When |
|---|---|---|
| `duplicacy` binary check | `--backup` or `--prune` (any mode that invokes Duplicacy) | Standalone `--fix-perms` |
| `btrfs` binary check | `--backup` (snapshot creation) | `--prune`, `--fix-perms` |
| `LOCAL_OWNER`/`LOCAL_GROUP` look-up | `--fix-perms` (file ownership changes) | `--backup` only, `--prune` only |
| `LOCAL_OWNER`/`LOCAL_GROUP` display | `--fix-perms` (standalone or combined) | `--backup` only, `--prune` only, `--remote` |

This means:
- **`--fix-perms homes`** does not require `duplicacy` to be installed.
- **`--backup homes`** does not validate `LOCAL_OWNER`/`LOCAL_GROUP` existence
  on the system (though those fields are still required in the `[local]` config
  section for `--fix-perms` usage).

### Minimal Summary for Standalone Fix-Perms

When running `--fix-perms` without `--backup` or `--prune`, the configuration
summary is trimmed to show only the fields relevant to permission fixing:

| Shown | Hidden |
|---|---|
| Operation Mode | Config File, Backup Label, Mode |
| Destination | Source, Repository, Work Dir |
| Local Owner / Local Group | Threads, Filter, Prune Options |
| Dry Run | Log Retention, Force Prune, Prune thresholds |

When combining `--fix-perms` with `--backup` or `--prune`, the full summary
is displayed including Local Owner and Local Group.

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

## Architecture

### Coordinator Pattern

The program follows a **coordinator pattern** centred on the `app` struct in `cmd/duplicacy-backup/main.go`. The top-level `run()` function delegates to `runWithArgs(cliArgs())`, which creates an `*app` via `newApp()`, then calls a series of clearly-bounded methods in sequence:

```
newApp → acquireLock → loadConfig → loadSecrets → printHeader → printSummary → execute → cleanup
```

`newApp()` itself is decomposed into five sub-initializers:

```
initLogger → parseAppFlags → validateEnvironment → derivePaths → installSignalHandler
```

Each method has a **single concern** and logs + returns errors to the caller. The caller (`run`) checks the error and sets the exit code. This makes the control flow readable in one screen and each phase independently testable.

#### The `app` struct

The `app` struct holds all state for a single run — mode booleans, derived paths, loaded config, secrets, lock, and duplicacy setup. This eliminates deeply-nested closures and makes every dependency explicit.

#### Initialization Sequence (`newApp`)

`newApp(args)` decomposes startup into five focused sub-initializers called in a fixed sequence:

```
initLogger → parseAppFlags → validateEnvironment → derivePaths → installSignalHandler
```

| Sub-initializer | Responsibility |
|---|---|
| `initLogger()` | Create structured logger (colour auto-detect, log-file capture). Falls back to raw stderr on failure. |
| `parseAppFlags(args)` | Handle `--help`/`--version` early exits, parse CLI flags, derive mode booleans (`doBackup`, `doPrune`, etc.) |
| `validateEnvironment()` | Check root privilege, validate label safety, verify binary dependencies (`duplicacy`, `btrfs`), enforce flag combination rules |
| `derivePaths()` | Compute snapshot/work/config/secrets paths, create `CommandRunner` and `Lock` |
| `installSignalHandler()` | Set up SIGINT/SIGHUP/SIGTERM handler for graceful cleanup |

Each sub-initializer has a **single responsibility**, follows a **consistent error pattern** (return `int` exit code or `error`), and is **independently testable** — unit tests can construct a partial `*app` and call any sub-initializer in isolation.

#### Phase methods (on `*app`)

| Method | Responsibility |
|---|---|
| `acquireLock()` | Create and acquire directory-based PID lock |
| `loadConfig()` | Parse INI config, validate, build prune args, check btrfs volumes |
| `loadSecrets()` | Load secrets file (remote mode only) |
| `printHeader()` | Emit startup banner |
| `printSummary()` | Emit configuration summary |
| `execute()` | Dispatch to backup/prune/fix-perms phase methods |
| `prepareDuplicacySetup()` | Create snapshot, init duplicacy working env |
| `runBackupPhase()` | Execute duplicacy backup |
| `runPrunePhase()` | Preview, enforce thresholds, execute prune |
| `runFixPermsPhase()` | Normalise ownership/permissions |
| `cleanup()` | Idempotent: delete snapshot, remove work dir, release lock, print result |
| `fail(err)` | Log error and set exitCode to 1 |

#### Free functions (no `app` state needed)

| Function | Purpose |
|---|---|
| `parseFlags` | Parse CLI arguments into a `flags` struct |
| `validateLabel` | Reject unsafe label characters (path traversal prevention) |
| `resolveDir` | Priority resolution: flag > env var > default |
| `joinDestination` | Append label to a destination, preserving URL schemes |
| `executableConfigDir` | Locate config dir relative to the binary |
| `printUsage` | Emit help text |

### Command Execution (`internal/exec`)

All external process execution (btrfs, duplicacy, chown) is centralised behind the `exec.Runner` interface:

```go
type Runner interface {
    Run(ctx context.Context, cmd string, args ...string) (stdout, stderr string, err error)
    RunWithInput(ctx context.Context, input string, cmd string, args ...string) (stdout, stderr string, err error)
}
```

| Type | Purpose |
|---|---|
| `CommandRunner` | Production implementation — wraps `os/exec` with context, logging, dry-run support |
| `MockRunner` | Test double — records invocations and replays pre-configured results (FIFO) |

The `app` struct creates a single `CommandRunner` in `newApp()` and passes it to `btrfs`, `duplicacy`, and `permissions` packages. This design:

- **Eliminates direct `os/exec` calls** in domain packages
- **Enables unit testing** without real binaries on `PATH` — tests inject a `MockRunner`
- **Provides consistent logging** — every command is logged before execution
- **Supports dry-run** at the runner level — commands are logged but not executed
- **Adds context support** — callers can enforce timeouts or cancellation

#### Using MockRunner in tests

```go
mock := exec.NewMockRunner(
    exec.MockResult{Stdout: "btrfs\n"},  // first call returns this
    exec.MockResult{},                    // second call succeeds silently
    exec.MockResult{Err: errors.New("fail")}, // third call fails
)

// Pass mock to any function that accepts exec.Runner
err := btrfs.CheckVolume(mock, "/volume1", false)

// Assert invocations
assert(mock.Invocations[0].Cmd == "stat")
assert(mock.Invocations[1].Cmd == "btrfs")
```

### Output Ownership (Phase 3)

Internal packages (`btrfs`, `duplicacy`, `permissions`) **never log directly**. They return structured errors and raw command output; the coordinator in `main.go` owns all operator-facing messages.

| Package | Returns | Coordinator responsibility |
|---|---|---|
| `btrfs` | `*errors.SnapshotError` | Log phase messages around snapshot create/delete |
| `duplicacy` | `(stdout, stderr string, error)` | Pipe output through logger, wrap errors with context |
| `permissions` | `*errors.PermissionsError` | Log phase messages around permission fixing |

Structured error types live in `internal/errors` and carry phase, field, cause, and context:

```go
err := btrfs.CreateSnapshot(runner, src, dst, false)
if err != nil {
    var snapErr *errors.SnapshotError
    if errors.As(err, &snapErr) {
        log.Error("Snapshot creation failed for %s: %v", snapErr.Field, snapErr.Cause)
    }
}
```

See [MESSAGE_STYLE_GUIDE.md](MESSAGE_STYLE_GUIDE.md) for the message formatting conventions.

### Design goals

- **`run()` readable in one screen** — the entire orchestration is visible at a glance
- **Single concern per phase** — each method does one thing
- **Output ownership** — internal packages return data; the coordinator formats all user-facing messages
- **Testable** — unit tests can construct an `*app` with stubbed fields, inject a `MockRunner` for command isolation, and swap package-level function variables (`geteuid`, `lookPath`, `newLock`) to test the full coordinator pipeline via `runWithArgs()`
- **Structured errors** — typed errors with context enable precise error handling at the coordinator level

---

## Directory Structure

```
synology-duplicacy-backup/
├── cmd/
│   └── duplicacy-backup/
│       ├── main.go              # Entry point and coordinator (app struct)
│       ├── main_test.go         # Unit tests for coordinator + free functions
│       └── integration_test.go  # Integration tests for multi-phase pipelines
├── internal/
│   ├── btrfs/
│   │   ├── btrfs.go             # BTRFS volume checks and snapshot management
│   │   └── btrfs_test.go        # Unit tests with MockRunner
│   ├── config/
│   │   └── config.go            # INI config parser and validation
│   ├── duplicacy/
│   │   ├── duplicacy.go         # Duplicacy CLI wrapper (backup, prune, list)
│   │   └── duplicacy_test.go    # Unit tests with MockRunner
│   ├── errors/
│   │   ├── errors.go            # Structured error types (BackupError, SnapshotError, etc.)
│   │   └── errors_test.go       # Unit tests for error types
│   ├── exec/
│   │   ├── runner.go            # Runner interface, CommandRunner, and MockRunner
│   │   ├── runner_test.go       # Unit tests for runner implementations
│   │   └── integration_test.go  # Integration tests for command execution
│   ├── lock/
│   │   └── lock.go              # Directory-based PID locking
│   ├── logger/
│   │   └── logger.go            # Structured logging with colour and rotation
│   ├── permissions/
│   │   ├── permissions.go       # Local repo ownership/permission fixing
│   │   └── permissions_test.go  # Unit tests with MockRunner
│   └── secrets/
│       └── secrets.go           # Secrets file loading and validation (ParseSecrets + ValidateFileAccess)
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

## Verifying Release Downloads

Each release includes:

- the raw binary
- a `.tar.gz` archive containing the binary, `README.md`, and `LICENSE`
- a `.sha256` file for each asset
- a combined `SHA256SUMS.txt` file

Verifying downloads ensures the file has not been corrupted or tampered with.

### Verify a Single Downloaded File

Download the asset and its matching `.sha256` file, then run:

```bash
sha256sum -c duplicacy-backup_v1.2.3_linux_amd64.tar.gz.sha256
```

Expected output:

```
duplicacy-backup_v1.2.3_linux_amd64.tar.gz: OK
```

You can do the same for a raw binary:

```bash
sha256sum -c duplicacy-backup_v1.2.3_linux_amd64.sha256
```

### Verify Against the Full Checksum Manifest

Download `SHA256SUMS.txt` and the asset you want to verify, then run:

```bash
sha256sum -c SHA256SUMS.txt --ignore-missing
```

Expected output:

```
duplicacy-backup_v1.2.3_linux_amd64.tar.gz: OK
```

### Inspect Archive Contents Before Extraction

To list the contents of an archive without extracting it:

```bash
tar -tzf duplicacy-backup_v1.2.3_linux_amd64.tar.gz
```

Expected layout:

```
duplicacy-backup_v1.2.3_linux_amd64/
duplicacy-backup_v1.2.3_linux_amd64/duplicacy-backup_v1.2.3_linux_amd64
duplicacy-backup_v1.2.3_linux_amd64/README.md
duplicacy-backup_v1.2.3_linux_amd64/LICENSE
```

### Extract the Archive

```bash
tar -xzf duplicacy-backup_v1.2.3_linux_amd64.tar.gz
```

### macOS Note

macOS typically provides `shasum` rather than `sha256sum`. To print the hash of a downloaded file:

```bash
shasum -a 256 duplicacy-backup_v1.2.3_linux_amd64.tar.gz
```

Then compare the output with either:

- the matching `.sha256` file, or
- the entry for that file in `SHA256SUMS.txt`

---

## Development

All Go code **must** be formatted with `gofmt` — the CI lint job enforces this on every push and PR.

```bash
gofmt -w .          # auto-format all files
go vet ./...        # static analysis
go test -race ./... # run tests with race detector
```

A pre-commit hook is provided in `scripts/pre-commit` — see [CONTRIBUTING.md](CONTRIBUTING.md) for setup instructions.

---

## License

MIT
