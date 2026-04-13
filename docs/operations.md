# Operations

## Installing On Synology

The recommended install layout keeps the real binary versioned while exposing a
stable command path for Synology Task Scheduler.

Example installed layout:

```text
/usr/local/lib/duplicacy-backup/
  duplicacy-backup_<version>_linux_<arch>
  current -> duplicacy-backup_<version>_linux_<arch>
  .config/

/usr/local/bin/
  duplicacy-backup -> /usr/local/lib/duplicacy-backup/current
```

Why this layout works well:

- scheduled tasks always call `/usr/local/bin/duplicacy-backup`
- the real binary still includes the version in its filename
- upgrades only need a symlink update, not a Task Scheduler edit
- the default config directory stays stable at:
  `/usr/local/lib/duplicacy-backup/.config`

### Install from a release tarball

After extracting a release tarball, run:

```bash
sudo ./install.sh
```

By default this will:

- copy the versioned binary into `/usr/local/lib/duplicacy-backup`
- create or update `current`
- create or update `/usr/local/bin/duplicacy-backup`
- create `/usr/local/lib/duplicacy-backup/.config` if needed
- when run as `root`, normalise `/usr/local/lib/duplicacy-backup/.config` to
  `root:administrators` with mode `750`
- when run as `root`, normalise any existing `*-backup.toml` files in that
  directory to `root:administrators` with mode `640`
- when run as `root`, ensure `/root/.secrets` exists as `root:root` with mode
  `700`
- never create, rewrite, or chmod individual secrets files

### Installer options

```bash
./install.sh --help
```

Useful examples:

```bash
# Install but keep the current active binary unchanged
sudo ./install.sh --no-activate

# Use custom install locations
sudo ./install.sh --install-root /volume1/tools/duplicacy-backup --bin-dir /usr/local/bin

# Use a different trusted operator group for config access
sudo ./install.sh --config-group users

# Keep only the 3 newest installed binaries
sudo ./install.sh --keep 3
```

### Upgrade and rollback

Upgrading is the same as a fresh install:

1. extract the new release tarball
2. run `sudo ./install.sh`
3. confirm `/usr/local/bin/duplicacy-backup --version`

Config and secrets stay in their existing directories, so upgrades do not
require copying TOML files again unless you are intentionally changing them.
The intended day-2 layout is one config file per label and, when needed, one
secrets file per label.

Under the current target model, only `type = "object"` targets need a secrets
file for storage credentials. Filesystem targets, whether local or remote, only
need one if a notifying target uses authenticated webhook or `ntfy` delivery.

To install a new binary without switching immediately:

```bash
sudo ./install.sh --no-activate
```

To roll back after an upgrade:

```bash
cd /usr/local/lib/duplicacy-backup
ls -1 duplicacy-backup_*_linux_*
sudo ln -sfn <older-binary-name> current
```

### Config location after install

With the recommended layout, the effective default config file path becomes:

```text
/usr/local/lib/duplicacy-backup/.config/<label>-backup.toml
```

Recommended Synology permissions:

- config directory: `root:administrators` with mode `750`
- config files: `root:administrators` with mode `640`
- secrets directory: `root:root` with mode `700`
- secrets files: `root:root` with mode `600`

You can still override this with:

- `--config-dir`
- `DUPLICACY_BACKUP_CONFIG_DIR`

## Synology Task Scheduler

1. Open **Control Panel > Task Scheduler**
2. Create a **Triggered Task > User-defined script**
3. Set the schedule
4. Run as `root`
5. Use a command such as:

```bash
/usr/local/bin/duplicacy-backup --target onsite-usb --backup homes
```

Example: backup followed by safe prune for label `homes` on target `offsite-storj`

```bash
/usr/local/bin/duplicacy-backup --target offsite-storj --backup --prune homes
```

Example: forced prune for label `homes` on target `offsite-storj`

```bash
/usr/local/bin/duplicacy-backup --target offsite-storj --prune --force-prune homes
```

Example: storage cleanup for label `homes` on target `offsite-storj`

```bash
/usr/local/bin/duplicacy-backup --target offsite-storj --cleanup-storage homes
```

Example: scheduled health summary for label `homes` on target `onsite-usb`

```bash
/usr/local/bin/duplicacy-backup health status --target onsite-usb homes
```

Example: scheduled JSON integrity verification for label `homes` on target `onsite-usb`

```bash
/usr/local/bin/duplicacy-backup health verify --json-summary --target onsite-usb homes
```

The health JSON report is intended for automation rather than terminal
rendering. It exposes structured fields such as:

- `status`
- `revision_count`
- `latest_revision`
- `latest_revision_at`
- `checked_revision_count`
- `passed_revision_count`
- `failed_revision_count`
- `failed_revisions`
- `last_doctor_run_at`
- `last_verify_run_at`

Healthy verify runs keep the JSON compact and omit per-revision detail. When
integrity issues are found, `revision_results` is included so the failing
revisions can be diagnosed.

Unhealthy verify runs also emit machine-focused classification fields:

- `failure_code`
- `failure_codes`
- `recommended_action_codes`

`--target <name>` selects one named target from the label config. Every
runtime, `config`, and `health` command requires an explicit target.
Targets now define both `type` and `location`, so mounted remote filesystems
can be modelled as `type = "filesystem"` with `location = "remote"` without
loading secrets.
The current secrets schema uses `storj_s3_id` and `storj_s3_secret` for
gateway-backed S3-compatible storage, with optional
`health_webhook_bearer_token` and `health_ntfy_token` support for authenticated
notifications. Those notification auth tokens are target-scoped in the secrets
file, so repeat them under each notifying target that needs authenticated
delivery.
For low-cost operator alerting, a good baseline is Synology scheduled-task
email for raw job failures plus native `ntfy` delivery for richer degraded,
unhealthy, and selected runtime-failure events. Generic webhook delivery stays
available for future providers and external automation.

Notification noise control in v1 is intentionally simple:

- default health notifications are only for `degraded` and `unhealthy`
- runtime notifications are opt-in through `send_for`
- interactive runs do not notify unless explicitly enabled
- success events do not notify
- repeated matching failures notify on each matching scheduled run

If you need deduplication, reminder cadence, or escalation, handle that in the
receiving system rather than in `duplicacy-backup` itself for now.

Health commands are read-only and do not prompt for confirmation. They are
designed to be run separately from backup jobs so schedulers and external
monitoring can check freshness and environment health without mutating backup
state.

## Release Verification

Releases include:

- `.tar.gz` archives
- per-file `.sha256` files
- `SHA256SUMS.txt`

### Verify a single file

```bash
sha256sum -c duplicacy-backup_v1.2.3_linux_amd64.tar.gz.sha256
```

### Verify against the full manifest

```bash
sha256sum -c SHA256SUMS.txt --ignore-missing
```

### Inspect archive contents

```bash
tar -tzf duplicacy-backup_v1.2.3_linux_amd64.tar.gz
```

The extracted directory now includes:

- the versioned binary
- `install.sh`
- `README.md`
- `LICENSE`

### Extract

```bash
tar -xzf duplicacy-backup_v1.2.3_linux_amd64.tar.gz
```

### macOS note

macOS often ships `shasum` instead of `sha256sum`:

```bash
shasum -a 256 duplicacy-backup_v1.2.3_linux_amd64.tar.gz
```
