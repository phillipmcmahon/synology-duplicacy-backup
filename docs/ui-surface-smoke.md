# UI Surface Smoke Testing

Use this package during release-candidate validation to capture every supported
command surface on the NAS. The goal is not only command success. The capture
set is the operator UI evidence for wording, sections, labels, colours,
remediation guidance, expected failures, and JSON-summary behaviour.

## Generate the Bundle

From the repository root on the development machine:

```sh
sh ./scripts/package-ui-surface-smoke.sh
```

The generated archive is written under:

```text
build/test-packages/release/<run-id>/<run-id>_bundle.tar.gz
```

Use explicit metadata when preparing a release candidate:

```sh
RUN_ID="ui-surface-smoke-$(date -u '+%Y%m%d%H%M%S')"

sh ./scripts/package-ui-surface-smoke.sh \
  --run-id "$RUN_ID" \
  --version "$(git describe --tags --always --dirty)-$RUN_ID" \
  --build-time "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
  --goos linux \
  --goarch amd64
```

## CI Coverage

GitHub Actions runs `scripts/ci-smoke-ui-surface.sh` as a release-gating proxy
test. That CI job:

- runs the UI smoke script syntax and content checks
- builds the Linux `amd64` UI smoke bundle
- verifies the bundle contains `setup-env.sh`, `run-ui-surface-smoke.sh`, and
  the smoke instructions
- verifies the bundle checksum and rejects macOS metadata files

GitHub Actions also runs `scripts/ci-smoke-non-root.sh` against a btrfs
loopback fixture. That fixture deliberately changes the path-based local
repository to `root:root`/`0700` before the non-root checks so the CI run
exercises the same locked-down local filesystem posture used on the NAS. The
script asserts that config, health, restore, and prune surfaces report
`requires sudo: local filesystem repository is root-protected` and do not leak
raw `permission denied` storage errors.

CI deliberately does **not** run the full command capture matrix because that
requires the real NAS profile, configured repositories, sudo policy, and restore
workspace. Treat CI as proof that the automation is packaged and runnable; treat
the NAS capture as proof that the operator UI is correct against production-like
storage.

## Run on the NAS

Copy the bundle to the NAS, extract it, then run from the extracted bundle
directory:

```sh
. ./setup-env.sh

export LABEL="${LABEL:-homes}"
export STORAGE_REMOTE="${STORAGE_REMOTE:-offsite-storj}"
export STORAGE_OBJECT="${STORAGE_OBJECT:-onsite-garage}"
export STORAGE_LOCAL="${STORAGE_LOCAL:-onsite-usb}"
export WORKSPACE_ROOT="${WORKSPACE_ROOT:-/volume1/restore-drills}"
export MANAGED_BIN="${MANAGED_BIN:-/usr/local/bin/duplicacy-backup}"
export SMOKE_SUDO_BIN="${SMOKE_SUDO_BIN:-/usr/local/lib/duplicacy-backup/smoke/duplicacy-backup-smoke}"

./run-ui-surface-smoke.sh
```

The runner automatically:

- captures stdout, stderr, exit code, command metadata, and a `summary.tsv`
- writes captures under `captures/<timestamp>/`
- writes `ui-surface-captures-<timestamp>.tar.gz` in the bundle root
- enables colour captures by default; set `CAPTURE_COLOUR=0` only when you
  deliberately want plain output
- injects `DUPLICACY_BACKUP_FORCE_COLOUR=1` for colour captures
- passes the colour override plus `DUPLICACY_BACKUP_CONFIG_DIR` and
  `DUPLICACY_BACKUP_SECRETS_DIR` through `sudo -n` for root-required captures
  using the stable smoke binary path in `SMOKE_SUDO_BIN`
- resolves managed update/rollback checks through `MANAGED_BIN`,
  `/usr/local/bin/duplicacy-backup`, or `PATH`
- exits non-zero if a command marked `pass`/`fail` has an unexpected outcome

## Sudo Policy for Smoke Runs

The bundle binary path changes for every smoke package, so sudo-required smoke
commands do not run that unpacked path directly. Instead, the runner refreshes
the current bundle binary into a stable, root-owned smoke path before running
the suite:

```sh
. ./setup-env.sh

sudo -n install -d -o root -g root -m 0755 "$(dirname -- "$SMOKE_SUDO_BIN")"
sudo -n install -o root -g root -m 0755 "$BIN" "$SMOKE_SUDO_BIN"
```

Allow the operator account to refresh and run only that stable smoke binary
without a password. Replace `<operator>` with the NAS account running the smoke
suite, including the `/var/services/homes/<operator>/...` path. `SETENV` is
intentionally limited to the smoke binary execution so colour captures can pass
`DUPLICACY_BACKUP_FORCE_COLOUR=1`, `DUPLICACY_BACKUP_CONFIG_DIR`, and
`DUPLICACY_BACKUP_SECRETS_DIR` through sudo without allowing arbitrary root
commands:

```sh
sudo tee /etc/sudoers.d/duplicacy-backup-smoke >/dev/null <<'EOF'
Cmnd_Alias DUPLICACY_BACKUP_SMOKE_INSTALL = \
    /usr/bin/install -d -o root -g root -m 0755 /usr/local/lib/duplicacy-backup/smoke, \
    /usr/bin/install -o root -g root -m 0755 /var/services/homes/<operator>/exclude/testing/*/*_bundle/extracted/duplicacy-backup_*_linux_amd64/duplicacy-backup_*_linux_amd64 /usr/local/lib/duplicacy-backup/smoke/duplicacy-backup-smoke
Cmnd_Alias DUPLICACY_BACKUP_SMOKE = /usr/local/lib/duplicacy-backup/smoke/duplicacy-backup-smoke *

<operator> ALL=(root) NOPASSWD: DUPLICACY_BACKUP_SMOKE_INSTALL
<operator> ALL=(root) NOPASSWD: SETENV: DUPLICACY_BACKUP_SMOKE
EOF
sudo chmod 0440 /etc/sudoers.d/duplicacy-backup-smoke
sudo visudo -cf /etc/sudoers.d/duplicacy-backup-smoke
```

Keep the smoke binary root-owned. Do not grant passwordless sudo to arbitrary
bundle paths under the operator home directory, because those paths are
operator-writable and would make the sudo rule too broad. The install rule is
still high-trust: it allows the smoke operator to replace the fixed smoke binary
with a bundle build from the testing area, then execute that binary as root.
Use it only for trusted release validation accounts.

The runner fails before capturing commands if it cannot refresh
`SMOKE_SUDO_BIN` or cannot execute it with `sudo -n`. You can check the sudo
rule before running the full suite:

```sh
sudo -n \
  DUPLICACY_BACKUP_FORCE_COLOUR=1 \
  DUPLICACY_BACKUP_CONFIG_DIR="$CFG" \
  DUPLICACY_BACKUP_SECRETS_DIR="$SEC" \
  "$SMOKE_SUDO_BIN" --version
```

## Optional Restore Run

The default run skips actual restore execution. To include small real restore
surfaces, choose a narrow snapshot-relative path. The runner auto-selects the
latest visible revision for each restore storage unless `RESTORE_REVISION` is
set explicitly:

```sh
RUN_RESTORE=1 \
RESTORE_STORAGE="$STORAGE_REMOTE" \
RESTORE_PATH='phillipmcmahon/code/*' \
CAPTURE_COLOUR=1 \
./run-ui-surface-smoke.sh
```

To exercise both object/remote and root-protected local filesystem restore in
one release smoke run, provide multiple storage entries and name the storage entries that need
sudo:

```sh
RUN_RESTORE=1 \
RESTORE_STORAGE="$STORAGE_OBJECT $STORAGE_LOCAL" \
RESTORE_USE_SUDO_STORAGE="$STORAGE_LOCAL" \
RESTORE_PATH='phillipmcmahon/code/*' \
CAPTURE_COLOUR=1 \
./run-ui-surface-smoke.sh
```

`RESTORE_USE_SUDO=1` is still available for single-storage runs where the only
restore storage is root-protected local filesystem storage.

For fully automated release smoke bundles, bake the restore defaults into the
bundle when packaging:

```sh
scripts/package-ui-surface-smoke.sh \
  --default-run-restore 1 \
  --default-restore-storage 'onsite-garage onsite-usb' \
  --default-restore-use-sudo-storage 'onsite-usb' \
  --default-restore-path 'phillipmcmahon/code/*'
```

Use `--default-restore-use-sudo 1` only for bundles whose default restore
storage is a root-protected local filesystem repository.

The restore automation creates a smoke-owned root namespace first:

```text
<workspace-root>/ui-smoke-<shortsha>-<run-ts>/
```

Inside that namespace, it creates case roots such as:

```text
default/
revision-storage-run/
storage-revision-snapshot/
same-revision-cross-storage/
data-restore-<storage>/
```

The template-matrix cases run `restore run --dry-run` with different
`--workspace-template` combinations to prove that `{label}`, `{storage}`,
`{snapshot_timestamp}`, `{revision}`, and `{run_timestamp}` are rendered
consistently. The real data restore uses a smoke-marked derived workspace:

```text
<label>-rev<revision>-<storage>-smoke-<run_timestamp>
```

The `ui-smoke` and `smoke` markers make the workspace obviously test-owned, the
short commit identifies the build under test, and the run timestamp prevents an
existing workspace from hiding restore behaviour by letting Duplicacy skip
work. Smoke workspaces can be listed or removed with:

```sh
find "$WORKSPACE_ROOT" -maxdepth 1 -type d -name 'ui-smoke-*'
```

With `RUN_RESTORE=1`, the template dry-runs, real restore, restore content
presence check, and restore dry-run captures are expected to succeed for every
storage entry in `RESTORE_STORAGE`. The runner also asserts that restore reports
include `-ignore-owner`, which protects non-root drill restores from Duplicacy
UID/GID replay failures while keeping copy-back manual. If `RESTORE_REVISION`
is omitted, each storage capture includes a `restore_revision_auto_select` step
showing the selected revision. If `RESTORE_REVISION` is provided, the runner
captures a `restore_revision_lookup` listing for each storage entry before running the
template matrix.

## Optional Interactive Checks

The tree picker is interactive and cannot be fully driven by the
non-interactive runner. Capture it manually when UI changes touch restore
selection:

```sh
. ./setup-env.sh
mkdir -p captures/manual

CAPTURE_COLOUR=1 "$BIN" restore select --storage "$STORAGE_REMOTE" "$LABEL" \
  2>&1 | tee captures/manual/restore_select_remote.txt

CAPTURE_COLOUR=1 sudo -n "$SMOKE_SUDO_BIN" restore select --storage "$STORAGE_LOCAL" "$LABEL" \
  2>&1 | tee captures/manual/restore_select_local_sudo.txt
```

Only run the local storage command when the local repository is intentionally
root-protected and sudoers allows the stable smoke binary without a password.

## Optional Notification Checks

Notification checks may send real messages. They are skipped by default:

```sh
RUN_NOTIFY=1 CAPTURE_COLOUR=1 ./run-ui-surface-smoke.sh
```

## Review Checklist

Review `summary.tsv` first:

- no `UNEXPECTED_FAIL` or `UNEXPECTED_PASS`
- local path-based repository commands that require sudo are `EXPECTED_FAIL`
  when run as the operator
- managed update/rollback captures use the installed managed command, not only
  the unpacked smoke binary

Review the `.txt` captures for UI consistency:

- canonical labels and values are defined in
  [`internal/presentation/vocabulary.go`](../internal/presentation/vocabulary.go);
  identity mappings there are intentional and should be reviewed like any
  other vocabulary entry
- shared labels use the same display form, such as `Config File`,
  `Source Path`, `Repository Access`, `Storage Access`, and `Dry Run`
- repeated values use the same vocabulary, such as `Valid`, `Validated`,
  `Writable`, `Present`, `Requires sudo`, `Healthy`, `Degraded`, and
  `Unhealthy`
- local repository sudo guidance uses the shared phrase:
  `Requires sudo: local filesystem repository is root-protected`, or the
  command-prefixed form such as
  `restore list-revisions requires sudo: local filesystem repository is
  root-protected`
- root-protected local repository captures do not fall through to raw
  `permission denied`/`EACCES` storage errors
- timestamped runtime and health output remains framed/log-style, while
  report-style commands colour only shared semantic status values
- colour semantics are consistent: errors red, warnings yellow, labels cyan,
  successful semantic results green, ordinary values white, and neutral
  outcomes such as `Not required` plain
- when `CAPTURE_COLOUR=1`, the runner enforces colour semantics for config
  validation captures: success values such as `Present`, `Valid`, `Resolved`,
  `Writable`, and `Passed` must be green; warning values such as
  `Requires sudo` must be yellow; failure values such as `Invalid (...)` and
  `Failed` must be red
- `--json-summary` captures end with valid JSON even when stderr logs are
  present earlier in the combined capture
- restore success shows the compact Duplicacy summary, not the full raw file or
  chunk stream

Useful local review commands after pulling the capture archive back:

```sh
tar -xzf ui-surface-captures-<timestamp>.tar.gz
column -t -s "$(printf '\t')" captures/<timestamp>/summary.tsv | less
grep -R "UNEXPECTED" captures/<timestamp>/summary.tsv
grep -R "Dry run\\|must be run as root\\|command not found" captures/<timestamp> || true
```

For colour review:

```sh
cat -v captures/<timestamp>/10_config_validate_local_operator_requires_sudo.txt | less
cat -v captures/<timestamp>/60_prune_dry_run_local_operator_requires_sudo.txt | less
```

Look for visible `^[` escape sequences from `cat -v`.
