# Operations

## Installing on Synology

The recommended install layout keeps the real binary versioned while exposing a
stable command path for Synology Task Scheduler.

Example installed layout:

```text
/usr/local/lib/duplicacy-backup/
  duplicacy-backup_<version>_linux_<arch>
  current -> duplicacy-backup_<version>_linux_<arch>

/usr/local/bin/
  duplicacy-backup -> /usr/local/lib/duplicacy-backup/current
```

Why this layout works well:

- scheduled tasks always call `/usr/local/bin/duplicacy-backup`
- the real binary still includes the version in its filename
- upgrades only need a symlink update, not a Task Scheduler edit
- runtime config stays in the user profile by default:
  `$HOME/.config/duplicacy-backup`

### Install from a release tarball

After extracting a release tarball, run:

```bash
sudo ./install.sh
```

By default, it will:

- copy the versioned binary into `/usr/local/lib/duplicacy-backup`
- create or update `current`
- create or update `/usr/local/bin/duplicacy-backup`
- leave runtime config, secrets, logs, state, and locks in the user profile
- treat runtime config and secrets as operator-owned files

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

Once the tool is already installed in the standard managed layout, you can also
check for and apply published upgrades with:

```bash
/usr/local/bin/duplicacy-backup update --check-only
sudo /usr/local/bin/duplicacy-backup update --yes
sudo /usr/local/bin/duplicacy-backup update --attestations required --yes
sudo /usr/local/bin/duplicacy-backup update --force --yes
```

Inspect retained rollback candidates before changing anything:

```bash
/usr/local/bin/duplicacy-backup rollback --check-only
```

Activate the newest previous retained version when you need to back out a
managed update:

```bash
sudo /usr/local/bin/duplicacy-backup rollback --yes
```

Use an explicit retained version when incident response calls for a specific
target:

```bash
sudo /usr/local/bin/duplicacy-backup rollback --version v5.1.1 --yes
```

### Update trust model

`duplicacy-backup update` verifies the downloaded package checksum before
installation and can optionally verify GitHub release-asset attestations before
extraction. The default remains checksum-only so existing scheduled update jobs
keep working unless you opt into stronger verification.

For the focused trust-model reference, including `--attestations required`,
`--attestations auto`, GitHub CLI auth expectations, and trust boundaries, see
[Update Trust Model](update-trust-model.md).

Update notification policy is configured globally in
`$HOME/.config/duplicacy-backup/duplicacy-backup.toml`; see
[Configuration and secrets](configuration.md#update-notifications).

Config and secrets stay in their existing directories during upgrades, so you
do not need to copy TOML files again unless you are intentionally changing
them. In normal day-to-day use, each label has one backup config file and,
when needed, one matching secrets file.
In normal operation, storage keys are needed only when the selected backend requires them;
see [Configuration and secrets](configuration.md#secrets) for the target-scoped
secrets model.

Use `update --force` only when you intentionally want to reinstall the selected
release, for example after repairing a managed install. It still follows normal
interactive rules, so add `--yes` for unattended scheduler or repair jobs.

By default, `update` keeps the newly activated binary and one previous binary;
use `--keep <count>` if you want a different local rollback window.

`rollback` uses that retained-binary window. It does not download anything and
does not modify config or secrets; activation changes only the managed
`current` symlink under `/usr/local/lib/duplicacy-backup`.

Update network calls are bounded. If GitHub release metadata lookup or package
download stalls, the command reports which phase timed out instead of waiting
indefinitely.

The rollback command is preferred for normal managed installs. If the command
itself is unavailable during an emergency, the manual fallback is:

```bash
cd /usr/local/lib/duplicacy-backup
ls -1 duplicacy-backup_*_linux_*
sudo ln -sfn <older-binary-name> current
```

## Restore Drills

Restore is important enough to have its own operator guide. Start with
[`restore-drills.md`](restore-drills.md) for routine restore practice on an
existing NAS, or [`new-nas-restore.md`](new-nas-restore.md) when rebuilding a
replacement NAS.

The operational rule is simple: restore into a drill workspace first, inspect
the result, and copy back manually only after review. For most live operator
restores, start with:

```bash
duplicacy-backup restore select --target offsite-storj homes
```

Use `sudo` for restore commands only when the selected target is a
root-protected local filesystem repository, such as an onsite USB repository.
Object storage and remote mounted filesystem repositories normally restore as
the operator user.

For scriptable restore primitives such as
`restore plan --target onsite-usb homes` and
`restore run --target onsite-usb --revision 2403 --path docs/readme.md --yes homes`,
use [Restore drills](restore-drills.md).

### Runtime locations after install

The install location does not define runtime config. The effective defaults are
owned by the user running the command:

```text
$HOME/.config/duplicacy-backup/<label>-backup.toml
$HOME/.config/duplicacy-backup/secrets/<label>-secrets.toml
$HOME/.local/state/duplicacy-backup/
```

Recommended permissions:

- config directory: user-owned with mode `0700`
- config files: user-owned with mode `0600`
- secrets directory: user-owned with mode `0700`
- secrets files: user-owned with mode `0600`

For root-required commands launched with normal `sudo` metadata from the
operator account, defaults still resolve to the operator-owned runtime profile.
`0600` secrets owned by the sudoing operator account are accepted for those
commands.

If you run from a direct root shell, or with a stripped environment that lacks
complete sudo metadata, profile-using commands are rejected unless explicit
profile roots are supplied. This prevents config, secrets, logs, state, and
locks from silently falling back to `/root`.

You can still override this with:

- `--config-dir`
- `DUPLICACY_BACKUP_CONFIG_DIR`
- `--secrets-dir`
- `DUPLICACY_BACKUP_SECRETS_DIR`
- `XDG_STATE_HOME`

For direct-root expert use, prefer the command-line `--config-dir` and
`--secrets-dir` flags plus `XDG_STATE_HOME` so the intended runtime profile is
visible in the command itself.

## Synology Task Scheduler

Scheduling has its own focused guide:
[`workflow-scheduling.md`](workflow-scheduling.md).

The short version is:

- run scheduled tasks as the operator user
- prefix only root-required commands with `sudo -n`
- keep backup, prune, health, and diagnostics as separate tasks
- avoid routine `cleanup-storage`
- never schedule `prune --force` as a normal recurring task

Example scheduled backup:

```bash
sudo -n /usr/local/bin/duplicacy-backup backup --target onsite-usb homes
```

Example scheduled health check for an object or remote mounted filesystem
repository:

```bash
/usr/local/bin/duplicacy-backup health verify --json-summary --target offsite-storj homes
```

Example health summary for a root-protected local filesystem repository:

```bash
sudo -n /usr/local/bin/duplicacy-backup health status --target onsite-usb homes
sudo -n /usr/local/bin/duplicacy-backup health verify --json-summary --target onsite-usb homes
```

Use `sudo -n` for local filesystem repositories because repository metadata is
root-protected. Object repositories and remote mounted filesystem repositories
can run health checks as the operator user when credentials or mount
permissions allow access. For sudoers templates, cadence slots, task naming,
and multi-label examples, use
[`workflow-scheduling.md`](workflow-scheduling.md).

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
