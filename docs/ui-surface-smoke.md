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
surface, choose a revision and a narrow snapshot-relative path:

```sh
RUN_RESTORE=1 \
RESTORE_TARGET="$TARGET_REMOTE" \
RESTORE_REVISION=1 \
RESTORE_PATH='phillipmcmahon/code/*' \
CAPTURE_COLOUR=1 \
./run-ui-surface-smoke.sh
```

The restore command always uses `--workspace-root "$WORKSPACE_ROOT"` and
restores into the derived drill workspace. If that workspace already exists,
Duplicacy may report files as skipped rather than downloaded; that still
validates the command path and output shape.

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

- shared labels use the same display form, such as `Config File`,
  `Source Path`, `Repository Access`, `Storage Access`, and `Dry Run`
- repeated values use the same vocabulary, such as `Valid`, `Validated`,
  `Writable`, `Present`, `Requires sudo`, `Healthy`, `Degraded`, and
  `Unhealthy`
- local repository sudo guidance uses the shared phrase:
  `Requires sudo; path-based local repository storage is protected by OS
  filesystem permissions; rerun with sudo from the operator account`
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
