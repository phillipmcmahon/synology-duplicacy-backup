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
docker run --rm -v "$PWD":/work -w /work golang:1.26 /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test ./... && /usr/local/go/bin/go vet ./...'

# Full coverage pass in the same environment
docker run --rm -v "$PWD":/work -w /work golang:1.26 /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test -cover ./...'
```

The macOS host environment is not treated as release-representative. Use the
Linux Go 1.26 container for release validation, packaged-binary smoke checks,
and any test runs that depend on Linux locking or filesystem behaviour.

The standard Linux setup is documented in
[`docs/linux-environment.md`](docs/linux-environment.md).

For the full release process, use [`docs/release-playbook.md`](docs/release-playbook.md).

## Current Release Baseline

Current public release baseline: `v4.3.1`

Active release-prep target: `v4.3.2`

Representative Linux Go 1.26 validation for the current release baseline:

- `go test ./...`
- `go vet ./...`
- `go test -cover ./...`

Current Linux Go 1.26 validation snapshot for active release-prep target
`v4.3.2`:

- overall coverage: `81.1%`
- `cmd/duplicacy-backup`: `91.4%`
- `internal/workflow`: `83.1%`
- `internal/duplicacy`: `81.2%`
- `internal/exec`: `97.4%`
- `internal/secrets`: `90.9%`
- `internal/update`: `84.7%`

Additional v4.3.2 release-prep validation:

- Update notification mapping contract tests lock success, failure, and
  notify-test event mappings to structured status values.
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

- `homestorage:/volume1/homes/phillipmcmahon/code/duplicacy-backup/<tag>/`

Use `scripts/mirror-release-assets.sh --tag vX.Y.Z` as the supported mirroring
path. It downloads the published asset set and source archives, then mirrors
them to `homestorage` with a `tar`-over-SSH transfer rather than relying on
plain `scp` wildcard copying.

Use `scripts/verify-release.sh --tag vX.Y.Z` as the supported post-release
verification path. It checks the GitHub release object, release-note headings,
expected asset names, tag commit alignment, and the mirrored artefact set on
`homestorage`.

When you do create a local test package, it must be generated inside the Linux
container, not on the macOS host. All local test packages must be written under
`build/test-packages`; do not create one-off package directories elsewhere
under `build/`.

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
- target-model resolution (`type` + `location`)
- operation-mode derivation
- backup-target derivation
- summary precomputation
- execution-ready plan fields
- execution-ready command strings
- btrfs validation during backup planning
- minimal fix-perms-only planning

Executor tests cover:

- operation-mode rendering for combined flows
- end-to-end dry-run execution for fix-perms-only
- lock lifecycle during execution
- cleanup and prune-policy behaviour through focused workflow helpers

Acceptance coverage for the current target model should always include:

- `filesystem/local`
- `filesystem/remote`
- `object/remote`

That includes both behaviour and output:

- filesystem/remote targets do not load secrets
- object/remote targets do load secrets
- `--fix-perms` is accepted for filesystem targets and rejected for object targets
- runtime and health headers include `Type` and `Location`
- `config validate` keeps `Resolved` identity-only and checks the new model
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
