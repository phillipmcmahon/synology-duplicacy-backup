# Operations

## Installing on Synology

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

By default, it will:

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

Once the tool is already installed in the standard managed layout, you can also
check for and apply published upgrades with:

```bash
/usr/local/bin/duplicacy-backup update --check-only
sudo /usr/local/bin/duplicacy-backup update --yes
sudo /usr/local/bin/duplicacy-backup update --attestations required --yes
sudo /usr/local/bin/duplicacy-backup update --force --yes
```

### Update trust model

`duplicacy-backup update` verifies the downloaded package checksum before
installation and can optionally verify GitHub release-asset attestations before
extraction. The default remains checksum-only so existing scheduled update jobs
keep working unless you opt into stronger verification.

For the focused trust-model reference, including `--attestations required`,
`--attestations auto`, GitHub CLI auth expectations, and trust boundaries, see
[Update Trust Model](update-trust-model.md).

Unattended update failures can use the same notification providers as the rest
of the tool, but the config is global rather than label-specific. Put update
notification settings in `/usr/local/lib/duplicacy-backup/.config/duplicacy-backup.toml`:

```toml
[update.notify]
notify_on = ["failed"]
interactive = false

[update.notify.ntfy]
url = "https://ntfy.sh"
topic = "duplicacy-updates"
```

Test that route without running a real update:

```bash
/usr/local/bin/duplicacy-backup notify test update --provider ntfy
```

Config and secrets stay in their existing directories during upgrades, so you
do not need to copy TOML files again unless you are intentionally changing
them. In normal day-to-day use, each label has one backup config file and,
when needed, one matching secrets file.

Use `update --force` only when you intentionally want to reinstall the selected
release, for example after repairing a managed install. It still follows normal
interactive rules, so add `--yes` for unattended scheduler or repair jobs.

By default, `update` keeps the newly activated binary and one previous binary;
use `--keep <count>` if you want a different local rollback window.

Update network calls are bounded. If GitHub release metadata lookup or package
download stalls, the command reports which phase timed out instead of waiting
indefinitely.

Under the current target model, every backup target delegates storage to
Duplicacy; storage keys are needed only when the selected backend requires them.
Path-based storage targets only need a secrets file if a notifying target uses
authenticated webhook or `ntfy` delivery.

## Restore Drills

The wrapper does not execute restores yet. Use
[`restore-drills.md`](restore-drills.md) to practise recovery with the Duplicacy
CLI in a separate drill workspace before copying anything back to live source
paths.

Use `restore plan` to print the resolved context and suggested Duplicacy
commands without creating the workspace or running a restore:

```bash
sudo duplicacy-backup restore plan --target onsite-usb homes
```

Use `restore prepare` when you want the wrapper to create the separate drill
workspace and write Duplicacy preferences there, but still leave revision
selection and restore execution manual:

```bash
sudo duplicacy-backup restore prepare --target onsite-usb homes
```

At a high level:

- use `config explain`, `config validate`, and `health status` to confirm the
  label, target, storage value, and repository health
- create an empty restore-drill workspace away from the live source path
- initialise that workspace with Duplicacy snapshot ID `data` and the selected
  target's `storage` value
- restore a full revision or selected paths into the drill workspace
- inspect the restored data before any deliberate copy-back step

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

For the recommended workflow model, naming convention, and a worked scheduling
example across multiple labels and targets, see
[`workflow-scheduling.md`](workflow-scheduling.md).

1. Open **Control Panel > Task Scheduler**
2. Create a **Triggered Task > User-defined script**
3. Set the schedule
4. Run as `root`
5. Use a command such as:

```bash
/usr/local/bin/duplicacy-backup --target onsite-usb --backup homes
```

Recommended scheduled pattern:

- keep backup, prune, health, and fix-perms as separate tasks
- use repeat scheduling for frequent onsite backups where it helps
- avoid routine `--cleanup-storage`
- do not schedule `--force-prune` as a normal recurring task

Example: scheduled backup for label `homes` on target `offsite-storj`

```bash
/usr/local/bin/duplicacy-backup --target offsite-storj --backup homes
```

Example: scheduled prune for label `homes` on target `offsite-storj`

```bash
/usr/local/bin/duplicacy-backup --target offsite-storj --prune homes
```

Example: scheduled fix-perms for label `homes` on target `onsite-usb`

```bash
/usr/local/bin/duplicacy-backup --target onsite-usb --fix-perms homes
```

Example: scheduled health summary for label `homes` on target `onsite-usb`

```bash
/usr/local/bin/duplicacy-backup health status --target onsite-usb homes
```

Example: scheduled JSON integrity verification for label `homes` on target `onsite-usb`

```bash
/usr/local/bin/duplicacy-backup health verify --json-summary --target onsite-usb homes
```

The health JSON report is meant for automation rather than terminal reading.
It exposes structured fields such as:

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
integrity issues are found, `revision_results` is included so you can inspect
the failing revisions.

Unhealthy verify runs also emit machine-focused classification fields:

- `failure_code`
- `failure_codes`
- `recommended_action_codes`

Health notifications include those same remediation fields in the payload
`details` whenever the health report contains them. Webhook consumers can use
those fields directly, and `ntfy` still keeps the visible message focused on
the operator-readable summary.

`--target <name>` selects one named target from the label config. Runtime,
`config`, `health`, `restore plan`, `restore prepare`, and label-scoped
`notify test` commands require an explicit target. Global update commands and
`notify test update` do not.
Targets define both `storage` and `location`. The configured `storage` value is
passed directly through to Duplicacy. Use `location = "local"` or
`location = "remote"` to describe where the target lives operationally, and add
`[targets.<name>.keys]` only when the selected Duplicacy backend needs runtime
keys.

The secrets schema also supports optional
`health_webhook_bearer_token` and `health_ntfy_token` values for authenticated
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
- update failure notifications are opt-in through the global `[update.notify]`
  config
- interactive runs do not notify unless explicitly enabled
- success events do not notify unless an update outcome such as `succeeded` is
  explicitly listed in `[update.notify].notify_on`
- repeated matching failures notify on each matching scheduled run

If you need deduplication, reminder cadence, or escalation, handle that in the
receiving system rather than in `duplicacy-backup` itself for now.

Health commands are read-only and do not prompt for confirmation. They are
designed to be run separately from backup jobs so schedulers and external
monitoring can check freshness and environment health without mutating backup
state.

As an operator guideline, prefer separate scheduled tasks for backup, prune,
health, and fix-perms rather than chaining everything together into one
recurring job.

Treat `--cleanup-storage` and `--force-prune` as explicit operator actions
rather than routine scheduled work.

## Release Verification

Releases include:

- `.tar.gz` archives
- per-file `.sha256` files
- `SHA256SUMS.txt`
- GitHub release attestations for the release and assets

### Verify a single file

```bash
sha256sum -c duplicacy-backup_<version>_linux_amd64.tar.gz.sha256
```

### Verify against the full manifest

```bash
sha256sum -c SHA256SUMS.txt --ignore-missing
```

### Inspect archive contents

```bash
tar -tzf duplicacy-backup_<version>_linux_amd64.tar.gz
```

### Verify the GitHub release attestation

```bash
gh release verify v<version> --repo phillipmcmahon/synology-duplicacy-backup
```

### Verify a downloaded asset against the release attestation

```bash
gh release verify-asset v<version> ./duplicacy-backup_<version>_linux_amd64.tar.gz \
  --repo phillipmcmahon/synology-duplicacy-backup
```

The extracted directory now includes:

- the versioned binary
- `install.sh`
- `README.md`
- `LICENSE`

### Extract

```bash
tar -xzf duplicacy-backup_<version>_linux_amd64.tar.gz
```

### macOS note

macOS often ships `shasum` instead of `sha256sum`:

```bash
shasum -a 256 duplicacy-backup_<version>_linux_amd64.tar.gz
```
