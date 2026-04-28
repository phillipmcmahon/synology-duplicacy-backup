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
targets.

## Run on the NAS

Copy the bundle to the NAS, extract it, then run from the extracted bundle
directory:

```sh
. ./setup-env.sh

export LABEL="${LABEL:-homes}"
export TARGET_REMOTE="${TARGET_REMOTE:-offsite-storj}"
export TARGET_OBJECT="${TARGET_OBJECT:-onsite-garage}"
export TARGET_LOCAL="${TARGET_LOCAL:-onsite-usb}"
export WORKSPACE_ROOT="${WORKSPACE_ROOT:-/volume1/restore-drills}"
export MANAGED_BIN="${MANAGED_BIN:-/usr/local/bin/duplicacy-backup}"

CAPTURE_COLOUR=1 ./run-ui-surface-smoke.sh
```

The runner automatically:

- captures stdout, stderr, exit code, command metadata, and a `summary.tsv`
- writes captures under `captures/<timestamp>/`
- writes `ui-surface-captures-<timestamp>.tar.gz` in the bundle root
- injects `DUPLICACY_BACKUP_FORCE_COLOUR=1` for colour captures
- passes the colour override through `sudo -n` for root-required captures
- resolves managed update/rollback checks through `MANAGED_BIN`,
  `/usr/local/bin/duplicacy-backup`, or `PATH`
- exits non-zero if a command marked `pass`/`fail` has an unexpected outcome

## Optional Restore Run

The default run skips actual restore execution. To include a small real restore
surface, choose a narrow snapshot-relative path. The runner auto-selects the
latest visible revision for `RESTORE_TARGET` unless `RESTORE_REVISION` is set
explicitly:

```sh
RUN_RESTORE=1 \
RESTORE_TARGET="$TARGET_REMOTE" \
RESTORE_PATH='phillipmcmahon/code/*' \
CAPTURE_COLOUR=1 \
./run-ui-surface-smoke.sh
```

For fully automated release smoke bundles, bake the restore defaults into the
bundle when packaging:

```sh
scripts/package-ui-surface-smoke.sh \
  --default-run-restore 1 \
  --default-restore-target offsite-storj \
  --default-restore-path 'phillipmcmahon/code/*'
```

The restore command uses an explicit smoke workspace rather than the normal
operator-derived restore workspace. The workspace name is:

```text
<label>-<target>-<snapshot-ts>-rev<revision>-smoke-<shortsha>-<run-ts>
```

For example:

```text
homes-offsite-garage-20260427-010000-rev1-smoke-4ec4f55-20260428-112500
```

The `smoke` marker makes the workspace obviously test-owned, the short commit
identifies the build under test, and the run timestamp prevents an existing
workspace from hiding restore behaviour by letting Duplicacy skip work. Set
`RESTORE_WORKSPACE` only when you deliberately want to override this generated
path. Smoke workspaces can be listed or removed with:

```sh
find "$WORKSPACE_ROOT" -maxdepth 1 -type d -name '*-smoke-*'
```

With `RUN_RESTORE=1`, the real restore and restore dry-run captures are
expected to succeed. The runner also asserts that both restore reports include
`-ignore-owner`, which protects non-root drill restores from Duplicacy UID/GID
replay failures while keeping copy-back manual. If `RESTORE_REVISION` is
omitted, the capture includes a `restore_revision_auto_select` step showing the
selected revision. If `RESTORE_REVISION` is provided, the runner captures a
`restore_revision_lookup` listing so the smoke workspace can still include the
snapshot timestamp when the revision appears in the listing. Increase
`RESTORE_REVISION_LOOKUP_LIMIT` if smoke workspaces show `unknown-snapshot` for
revisions older than the default 200-entry lookup.

## Optional Interactive Checks

The tree picker is interactive and cannot be fully driven by the
non-interactive runner. Capture it manually when UI changes touch restore
selection:

```sh
. ./setup-env.sh
mkdir -p captures/manual

CAPTURE_COLOUR=1 "$BIN" restore select --target "$TARGET_REMOTE" "$LABEL" \
  2>&1 | tee captures/manual/restore_select_remote.txt

CAPTURE_COLOUR=1 sudo -n "$BIN" restore select --target "$TARGET_LOCAL" "$LABEL" \
  2>&1 | tee captures/manual/restore_select_local_sudo.txt
```

Only run the local target command when the local repository is intentionally
root-protected and sudoers allows the application without a password.

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
  report-style commands remain plain and readable
- colour semantics are consistent: errors red, warnings yellow, labels cyan,
  successful semantic results green
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
