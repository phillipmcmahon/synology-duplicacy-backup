# Testing Guide

## Overview

The test suite follows the same layered architecture as the application:

- `Request` tests validate CLI intent parsing
- `Plan` tests validate config, secrets, and derived runtime state
- `Executor` tests validate side effects and phase ordering
- `cmd/duplicacy-backup` tests exercise the real `runWithArgs` entrypoint

All external commands are still mocked through `internal/exec.Runner`.

## Quick Start

```bash
# Representative Linux Go 1.26 validation
GO_CONTAINER_IMAGE="$(awk 'toupper($1) == "FROM" { print $2; exit }' tools/release-validation/Dockerfile)"
docker run --rm -v "$PWD":/work -w /work "$GO_CONTAINER_IMAGE" /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test ./... && /usr/local/go/bin/go vet ./... && /usr/local/go/bin/go run honnef.co/go/tools/cmd/staticcheck ./...'

# Full coverage pass in the same environment
docker run --rm -v "$PWD":/work -w /work "$GO_CONTAINER_IMAGE" /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test -cover ./...'
```

The macOS host environment is not treated as release-representative. Use the
Linux Go 1.26 container for release validation, packaged-binary smoke checks,
and any test runs that depend on Linux locking or filesystem behaviour.

The standard Linux setup is documented in
[`docs/linux-environment.md`](docs/linux-environment.md).

For the full release process, use [`docs/release-playbook.md`](docs/release-playbook.md).

## Current Release Baseline

Current public release baseline: `v7.1.2`

Active release-prep target: none

The baseline block is refreshed during release prep; `make release-prep`
should be the reminder to update it before publishing.

Representative Linux Go 1.26 validation for the current release baseline:

- `go test ./...`
- `go vet ./...`
- `go run honnef.co/go/tools/cmd/staticcheck ./...`
- `go test -cover ./...`

Current Linux Go 1.26 development validation snapshot:

- `go test ./...`
- `go vet ./...`
- `go run honnef.co/go/tools/cmd/staticcheck ./...`
- `go test -cover ./...`
- overall coverage: `83.5%`
- `cmd/duplicacy-backup`: `82.4%`
- `internal/workflow`: `82.8%`
- `internal/restorepicker`: `74.9%`
- `internal/update`: `82.6%`
- `internal/duplicacy`: `79.5%`
- `internal/exec`: `95.2%`
- `internal/secrets`: `92.0%`
- `internal/config`: `88.0%`

Additional v7.1.2 validation:

- Restore output coverage now checks compact success summaries, diagnostic-only
  failure output, and explicit messaging when Duplicacy emits no diagnostics.
- Restore interrupt coverage now checks the Ctrl-C reporting contract:
  workspace retained, no automatic deletion, completed path count, active path,
  and live-source safety.
- Restore command coverage now checks cancellation versus interruption
  semantics so clean operator exits remain exit code 0 while active restore
  interruption remains exit code 1.
- Full documentation review confirmed the public help surface describes the
  structured NAS smoke-test bundle layout and active-restore interrupt
  behaviour.

Additional v7.1.0 validation:

- New NAS restore documentation now covers installing the tool on replacement
  hardware, recreating minimum config/secrets files, and proving restore access
  before rebuilding live source paths.
- Restore access coverage now allows restore-oriented flows without
  `source_path`, while backup/full `config validate` still requires source
  readiness.
- Restore workspace coverage now pins the default
  `/volume1/restore-drills/<label>-<target>-<restore-point-timestamp>-rev<id>`
  naming model independently of `source_path`.
- Restore help and documentation now consistently describe `source_path` as
  live-source and copy-back context, not a prerequisite for reading backup
  data.

Additional v7.0.0 validation:

- Release validation covers the breaking restore command simplification:
  `restore prepare`, `restore files`, and the hidden `restore select
  --execute` path are removed from the public command surface.
- Restore execution coverage now includes self-prepared drill workspaces using
  the deterministic `<label>-<target>-<restore-point-timestamp>-rev<id>`
  workspace naming model.
- Restore help and documentation checks pin the current operator flow:
  `restore select`, `restore plan`, `restore list-revisions`, and
  `restore run`.

Additional v6.2.1 validation:

- Restore drill defaults now cover timestamped workspace recommendations and
  restore execution with a derived workspace when `restore run` omits
  `--workspace`.
- Restore-picker help and restore guidance now pin the `g` action as
  “generate the restore commands” so the picker controls and operator docs
  stay aligned.

Additional v6.2.0 validation:

- Restore select now has production Linux coverage for the tree picker
  interaction layer, including collapse and expand behaviour, root selection,
  selective restore primitive compilation, independent detail-pane scrolling,
  and rejection of Duplicacy summary footer lines as selectable restore paths.
- Restore help and documentation checks now pin the shipped tree-picker
  controls and operator guidance so the release surface stays aligned with the
  production picker workflow.

Additional v6.1.1 validation:

- CLI help rendering checks cover concise `--help` and detailed
  `--help-full` output for the refreshed command groups.
- Command-entrypoint and release-surface tests now pin diagnostics, rollback,
  restore drill, notification, update, and managed-install help coverage.

Additional v6.1.0 validation:

- Diagnostics command tests cover config-path reporting, state freshness,
  last-run status, path-permission context, JSON summary output, and redaction
  of storage URL credentials and query secrets.
- Rollback command tests cover retained-version discovery, newest-previous
  selection, explicit version selection, check-only reporting, root gating, and
  managed `current` symlink activation.
- Command-entrypoint tests cover the new `diagnostics` and `rollback`
  dispatch paths, including non-root safe modes and privileged install changes.

Additional v6.0.0 validation:

- CLI parser, release-surface, and command-entrypoint tests cover the breaking
  runtime command model:
  `backup`, `prune`, `cleanup-storage`, and `fix-perms`.
- Tests assert old top-level runtime operation flags are rejected rather than
  preserved as compatibility syntax.
- Restore tests cover the current restore surface: `restore select`,
  `restore plan`, `restore list-revisions`, and `restore run`.
- Restore workspace tests cover local preference generation, unsafe workspace
  rejection, non-empty workspace rejection, root-gated remote secret loading,
  read-only revision listing, picker inspection, workspace-only restore
  execution, and interactive command generation with guarded delegated
  execution. NAS smoke also covers real Duplicacy file-list rows and
  directory/subtree restore patterns.

Additional v4.4.1 validation:

- Operator documentation now includes a lightweight troubleshooting entry point
  for common scheduled-task, repository-readiness, health, notification,
  update, and privilege-related support cases.
- Release process guidance now treats NAS mirroring and full verifier output as
  standard release closure evidence.
- Concise update help now shows the default retention, version-selection, and
  attestation settings operators need before scheduling updates.

Additional v5.0.0 validation:

- Config semantics now use direct Duplicacy `storage` values and generic
  storage keys.

Additional v5.1.0 validation:

- Duplicacy storage scheme handling is covered by focused `StorageSpec` tests.
- Runtime JSON, health reports, and notification payload tests assert the
  retired `storage_type` field is no longer emitted.
- Plan section views have a focused regression test to ensure request, config,
  path, and display data remain available as distinct reviewable groups.
- Notification event IDs have a focused contract test so request validation and
  payload builders share the same supported event list.
- Notification provider registry tests cover built-in provider lookup and
  destination construction.
- State persistence tests now cover the shared mutate-and-save helper used by
  runtime state and health recency updates.
- Planner tests confirm URL-like storage values load storage keys when the
  selected backend needs them while remaining operationally local or remote
  according to `location`.
- Runtime failure, config command, summary, and notification tests confirm
  targets preserve `Location` in operator-facing output and webhook payloads
  without exposing a redundant target type.
- Config tests confirm the retired `type`, `destination`, and `repository`
  schema is rejected with migration-focused messages.
- Required-value validation now points operators at modern `common.*` and
  `targets.<name>.*` keys instead of the retired single-target key names.

Additional #114, #115, and #128 validation:

- CI now runs module-pinned Staticcheck validation with
  `honnef.co/go/tools/cmd/staticcheck`; Dependabot updates the tool version
  through Go module metadata.
- Dependabot now monitors Go modules, GitHub Actions, and the release
  validation Go container image.
- Staticcheck findings are resolved rather than suppressed, including unused
  helpers, deprecated title casing, error style, and one unused test assignment.
- Command redaction tests now cover env-style `KEY=value` wrappers for token,
  secret access key, and webhook URL values.

Additional #110 validation:

- Update attestation tests cover `off`, `auto`, and `required` verifier modes,
  including missing GitHub CLI, successful verification, and failed verification
  before extraction/install.
- Update failure notification classification maps attestation verification
  failures to the dedicated `update_attestation_failed` event.
- CLI parser, command adapter, help text, and operator documentation now cover
  `--attestations off|auto|required`.

Previous Linux Go 1.26 validation snapshot for `v4.3.2`:

- overall coverage: `81.0%`
- `cmd/duplicacy-backup`: `91.9%`
- `internal/workflow`: `83.1%`
- `internal/duplicacy`: `81.2%`
- `internal/exec`: `97.4%`
- `internal/secrets`: `90.9%`
- `internal/update`: `82.3%`

Additional v4.3.2 release-prep validation:

- Update notification mapping contract tests lock success, failure, and
  notify-test event mappings to structured status values.
- Update HTTP timeout tests verify release metadata and asset download requests
  carry explicit context deadlines and return operator-friendly timeout errors.
- CLI adapter tests lock the mapping from update-domain status values to
  workflow notification statuses.
- Update package coverage is back above the 80% minimum after focused tests for
  release lookup, package extraction, install execution, and report handling.
- Release-prep notes were generated under `build/release-prep/v4.3.2/`.

Current Linux Go 1.26 validation snapshot for `v4.3.1`:

- overall coverage: `79.8%`
- `cmd/duplicacy-backup`: `91.4%`
- `internal/workflow`: `82.7%`
- `internal/duplicacy`: `81.2%`
- `internal/config`: `83.8%`
- `internal/secrets`: `90.9%`
- `internal/update`: `69.8%`

Additional v4.3.1 release smoke:

- The updater regression deletes the caller working directory before launching
  the installer and confirms the installer runs from the extracted package
  directory without `getcwd` warnings.
- Linux amd64 test packaging produced
  `duplicacy-backup_4.3.1-getcwd.1_linux_amd64.tar.gz` successfully for the
  working-directory fix.

## Packaging Rule

Release artefacts are built by GitHub Actions from the pushed release tag.
Local packaging is optional and is only for test-package generation. After a
release is published, the canonical GitHub-built artefacts and the two
GitHub-generated source archives should be downloaded and mirrored to:

- `homestorage:/volume1/homes/phillipmcmahon/code/duplicacy-backup/latest/<tag>/`

Use `scripts/finalize-release.sh --tag vX.Y.Z` as the supported post-release
closure path. It mirrors the published asset set and source archives to
`homestorage` under `latest/<tag>`, moves older release directories under
`archive/<tag>`, runs the full release verifier, and prints a release-issue
closure summary.

Use `scripts/mirror-release-assets.sh --tag vX.Y.Z` only when repairing the
mirror step directly. Use `scripts/verify-release.sh --tag vX.Y.Z` when you
need to rerun full verification without re-mirroring.

When you do create a local test package, it must be generated inside the Linux
container, not on the macOS host. All local test packages must be written under
the structured `build/test-packages` tree:

- `build/test-packages/release/` for standard `duplicacy-backup` package output
- `build/test-packages/poc/<name>/` for experimental or proof-of-concept bundles

Do not create one-off package directories elsewhere under `build/`, and do not
drop new artefacts flat into the root of `build/test-packages`.

That includes:

- binary build
- tarball creation
- checksum generation
- packaged-artefact smoke checks
- binary architecture verification

The macOS host may orchestrate the container run and inspect the resulting
files, but it should not create the final Linux test archives itself.

Use the standard packaging scripts for local test-package flows:

- `scripts/package-linux-docker.sh`
- `scripts/package-linux-artifact.sh`
- `make package-synology`

`scripts/package-linux-artifact.sh` now enforces the Linux-only packaging rule directly and will
fail on non-Linux hosts. On macOS, the supported entrypoint is
`scripts/package-linux-docker.sh`.

## Test Layout

| Package | Focus |
|---|---|
| `cmd/duplicacy-backup` | Real entrypoint coverage through `runWithArgs` |
| `internal/workflow` | Request parsing, planning, summary composition, executor flow |
| `internal/btrfs` | Snapshot and volume helper behaviour |
| `internal/config` | TOML parsing, defaults, validation |
| `internal/duplicacy` | Duplicacy CLI wrapper and prune preview |
| `internal/exec` | Command runner and mock runner behaviour |
| `internal/logger` | Log formatting, colour handling, rotation |
| `internal/permissions` | Permission normalization |
| `internal/secrets` | File validation, parsing, masking |

## Current Approach

### `cmd/duplicacy-backup`

`main_test.go` now stays narrow on purpose. It covers the actual entrypoint:

- `--help`
- `--version`
- invalid flag handling
- non-root rejection
- config-load failure translation
- lock-acquisition failure translation
- backup dry-run success
- fix-perms-only dry-run success
- representative install/help/doc consistency checks

These tests use the same seams as production:

- `geteuid`
- `lookPath`
- `newLock`
- temporary `logDir`

The goal is to validate real top-level behaviour without duplicating all
workflow internals in the `cmd` package.

### `internal/workflow`

This package now carries most coordinator-oriented tests.

Request tests cover:

- help/version handled responses
- explicit operation requirement
- fix-perms-only mode derivation
- invalid flag combinations
- invalid labels

Planner tests cover:

- path derivation
- config loading
- target-model resolution (`storage` + `location`)
- operation-mode derivation
- backup-target derivation
- summary precomputation
- execution-ready plan fields
- execution-ready command strings
- btrfs validation during backup planning
- minimal fix-perms-only planning

Executor tests cover:

- operation-mode rendering for first-class runtime commands
- end-to-end dry-run execution for fix-perms-only
- lock lifecycle during execution
- cleanup and prune-policy behaviour through focused workflow helpers

Acceptance coverage for the current target model should always include:

- path-based storage with `location = "local"`
- path-based storage with `location = "remote"`
- URL-like storage with `location = "local"`
- URL-like storage with `location = "remote"`

That includes both behaviour and output:

- path-based storage targets do not load storage keys
- URL-like storage targets load storage keys only when the selected backend requires them
- `fix-perms` is accepted for path-based storage targets and rejected for URL-like storage targets
- runtime and health headers include `Location`
- `config validate` keeps `Resolved` identity-only and checks target settings
  under `Target Settings`

Workflow tests also cover:

- operator-message translation
- summary layout for fixed-perms-only and offsite-target flows
- logger activity rendering for interactive TTY runs

### `internal/btrfs`

The btrfs package now explicitly tests dry-run volume validation as well as the
existing stat / filesystem / subvolume paths.

## Test Utilities

### `exec.MockRunner`

All external command execution still flows through `internal/exec.Runner`.
Tests use `exec.MockRunner` to provide deterministic command results and to
assert on invocations.

### `captureOutput`

The `cmd` package keeps a small `captureOutput` helper so the `runWithArgs`
tests can assert on real stdout/stderr behaviour without depending on logger
internals.

### Real Logger, Temporary Log Directory

Tests use a real logger pointed at a temporary directory rather than mocking the
logger itself. That keeps formatting behaviour realistic while avoiding writes to
system log paths.

## Design Intent

The main testing goal of the refactor is to stop treating the old monolithic
coordinator as the only seam. The suite should now match the code structure:

```text
Request -> Plan -> Execute -> runWithArgs
```

That makes failures easier to locate and lets new features land in the layer
they actually belong to.

The test split is also meant to keep `cmd/duplicacy-backup` small. As more
workflow behaviour moves under `internal/workflow`, most new coordinator tests
should be added there unless they are specifically about the real entrypoint.

As the plan gets richer, new tests should prefer asserting plan fields and
workflow translations directly instead of reconstructing execution behaviour from
raw request/config state in the `cmd` package.

## Operator Message Style

Operator-facing wording is owned by `internal/workflow`.

Rules:

- translated operator messages should be concise and consistent
- status lines should not force terminal punctuation
- domain packages should return typed errors rather than pre-formatted final wording
- `internal/workflow/messages.go` is the translation contract for final stderr text

When adding a new high-value error path, prefer:

1. return a typed domain error from the package
2. translate it in `internal/workflow/messages.go`
3. add or update a table-driven translation test
4. add a `runWithArgs` assertion if the message is part of the real entrypoint UX

## Release-Gated Surfaces

The following are treated as release-sensitive operator surfaces and should get
targeted coverage whenever they change:

- help text in `UsageText`
- health command help and output shape
- install script help/output
- README / CLI / operations examples for the current flag set
- phase-oriented stderr output for normal, verbose, dry-run, and failure paths
- JSON summaries for both run and health commands
- release tarball smoke checks: archive contents, checksum validation, binary help/version, and installer help
