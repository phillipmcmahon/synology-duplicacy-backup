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
local_owner = "AJT"
local_group = "users"

[targets.offsite-storj]
type = "object"
location = "remote"
destination = "s3://gateway.storjshare.io/my-backup-bucket"
repository = "homes"
```

### 4. Run

These examples show valid manual and ad hoc commands. For recurring Synology
Task Scheduler jobs, prefer separate scheduled tasks for backup, prune, health,
and fix-perms; see [Workflow and scheduling](docs/workflow-scheduling.md).

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

# Send a simulated notification through the configured providers for homes on target onsite-usb
sudo duplicacy-backup notify test --target onsite-usb homes

# Check whether a newer published release is available
duplicacy-backup update --check-only

# Download and install the latest published release
sudo duplicacy-backup update --yes
```

`notify test` uses the existing label and target config, sends a clearly marked
synthetic notification, and is meant to validate provider delivery and auth. It
does not prove that a backup, prune, or health condition has occurred, and it
does not exercise scheduler email from DSM.

## Common Commands

These are useful operator commands, including combined manual maintenance
runs. For recommended recurring scheduler patterns, use the dedicated guide
instead of treating this list as a schedule template:
[Workflow and scheduling](docs/workflow-scheduling.md).

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

# Send a simulated notification through the configured providers for homes on target onsite-usb
sudo duplicacy-backup notify test --target onsite-usb homes

# Check whether a newer published release is available
duplicacy-backup update --check-only

# Download and install the latest published release
sudo duplicacy-backup update --yes
```

When operations are combined, execution order is fixed:
`backup -> prune -> cleanup-storage -> fix-perms`.

Config commands are read-only helpers:
- `config validate` checks the selected target from a label config, validates backup-required settings such as destination, threads, prune policy, and local-account consistency, and performs read-only Btrfs, secrets, and repository readiness checks when the current user has enough access
- `config explain` shows the resolved values for the selected target, including `Type` and `Location`, without loading object-target secrets by default
- `config paths` shows the resolved stable config, source, log, and any applicable secrets paths, including `Type` and `Location`

`config validate` never initialises storage or changes repository state. Its
repository readiness probe reports one of three operator-facing outcomes:
- `Repository Access : Valid`
- `Repository Access : Not initialized`
- `Repository Access : Invalid (...)`

When `config validate` is run without the privileges needed for root-only
checks, those lines are reported as `Not checked` instead of failing the whole
validation.

`update` checks GitHub for the latest published release, downloads the matching
Linux package for the current platform, verifies the checksum, and reuses the
packaged `install.sh` to activate the new version. `update --check-only` is
safe for routine inspection. Installing through `update` expects the standard
managed layout under `/usr/local/lib/duplicacy-backup` and `/usr/local/bin`.

Every runtime, `config`, and `health` command requires an explicit `--target <name>`.
Every runtime command must also include at least one explicit operation flag
such as `--backup`, `--prune`, `--cleanup-storage`, or `--fix-perms`.

Runtime and health headers now identify the selected work as:

- `Label`
- `Target`
- `Type`
- `Location`

Default output is phase-oriented and intentionally concise. Use `--verbose`
to include detailed operational logging and command details.

`--json-summary` adds a machine-readable completion summary on stdout while
keeping the human-readable phase logs on stderr.

The `health` command family adds read-only confidence checks:
- `health status` gives a fast current-state summary
- `health doctor` checks config, secrets, paths, btrfs prerequisites, locks, and storage reachability
- `health verify` goes further by validating the revisions found for the current backup with `duplicacy check -persist`

Secrets are only loaded for object targets. Filesystem targets, whether local
or remote, operate directly on the configured destination path and therefore do
not require a secrets file.

`source_path` must point at the Btrfs root location you intend to snapshot for
that label. In practice, that means a Btrfs volume or subvolume such as
`/volume1/source-homes`, not an arbitrary nested directory such as
`/volume1/source-homes/private-user-data`. Inclusion and exclusion beneath that snapshot
root should be handled with Duplicacy filters rather than by narrowing
`source_path` to a non-subvolume child path.

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

Optional notifications can be configured in `[health.notify]`, with
optional per-target overrides in `[targets.<name>.health.notify]`. An optional
`health_webhook_bearer_token` or `health_ntfy_token` can be stored in the
target secrets TOML under the matching `[targets.<name>]` table. Notification
auth tokens are target-scoped, so if multiple targets notify to the same
authenticated destination, repeat the token in each target section that needs
it.
Notifications are intended for non-interactive health runs and selected runtime
failures; interactive TTY runs do not notify by default. Runtime events are
opt-in through `send_for = ["backup", "prune", "cleanup-storage"]`, while the
default remains health-only. The generic JSON payload can be forwarded to
generic webhook destinations without baking a vendor-specific
format into the core application.

For a low-cost `email + ntfy` setup, keep Synology scheduled-task email enabled
for raw job failures and use native `[health.notify.ntfy]` delivery for health
and selected runtime alerts.

In v1, notification noise control is intentionally simple: success events do
not notify, runtime alerts are opt-in, interactive runs stay quiet by default,
and repeated scheduled failures notify on each matching run. If you need
deduplication or escalation, handle that in the receiving system.

If the environment is broken early enough that the backup TOML cannot be read,
built-in notification delivery is not expected to work because the notification policy
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
- [Workflow and scheduling](docs/workflow-scheduling.md)
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
