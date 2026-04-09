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

### Config location after install

With the recommended layout, the effective default config file path becomes:

```text
/usr/local/lib/duplicacy-backup/.config/<source>-backup.toml
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
/usr/local/bin/duplicacy-backup homes
```

Example: remote backup followed by local prune

```bash
/usr/local/bin/duplicacy-backup --remote homes && /usr/local/bin/duplicacy-backup --prune homes
```

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
