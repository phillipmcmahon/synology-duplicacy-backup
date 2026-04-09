# Operations

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

- raw binaries
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

### Extract

```bash
tar -xzf duplicacy-backup_v1.2.3_linux_amd64.tar.gz
```

### macOS note

macOS often ships `shasum` instead of `sha256sum`:

```bash
shasum -a 256 duplicacy-backup_v1.2.3_linux_amd64.tar.gz
```
