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
- leave `/root/.secrets` untouched

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

Example: backup followed by safe prune for target `offsite-storj`

```bash
/usr/local/bin/duplicacy-backup --target offsite-storj --backup --prune homes
```

Example: forced prune for target `offsite-storj`

```bash
/usr/local/bin/duplicacy-backup --target offsite-storj --prune --force-prune homes
```

Example: storage cleanup for target `offsite-storj`

```bash
/usr/local/bin/duplicacy-backup --target offsite-storj --cleanup-storage homes
```

Example: scheduled health summary

```bash
/usr/local/bin/duplicacy-backup health status --target onsite-usb homes
```

Example: scheduled JSON integrity verification for monitoring

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
The current secrets schema uses `storj_s3_id` and `storj_s3_secret` for
gateway-backed S3-compatible storage, with optional
`health_webhook_bearer_token` support for authenticated health notifications.

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
