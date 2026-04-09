# Testing Guide

## Overview

The test suite is now layered around the same architecture as the application:

- `Request` tests validate CLI intent parsing
- `Plan` tests validate config, secrets, and derived runtime state
- `Executor` tests validate side effects and phase ordering
- `cmd/duplicacy-backup` tests exercise the real `runWithArgs` entrypoint

All external commands are still mocked through `internal/exec.Runner`.

## Quick Start

```bash
# Focused packages for the workflow refactor
go test ./cmd/duplicacy-backup ./internal/workflow

# Run the touched internal packages as well
go test ./cmd/duplicacy-backup ./internal/btrfs ./internal/workflow

# Full suite
go test ./...
```

## Test Layout

| Package | Focus |
|---|---|
| `cmd/duplicacy-backup` | Real entrypoint coverage through `runWithArgs` |
| `internal/workflow` | Request parsing, planning, summary composition, executor flow |
| `internal/btrfs` | Snapshot and volume helper behavior |
| `internal/config` | INI parsing, defaults, validation |
| `internal/duplicacy` | Duplicacy CLI wrapper and prune preview |
| `internal/exec` | Command runner and mock runner behavior |
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

These tests use the same seams as production:

- `geteuid`
- `lookPath`
- `newLock`
- temporary `logDir`

The goal is to validate real top-level behavior without duplicating all
workflow internals in the `cmd` package.

### `internal/workflow`

This package now carries most coordinator-oriented tests.

Request tests cover:

- help/version handled responses
- default backup-mode derivation
- fix-perms-only mode derivation
- invalid flag combinations
- invalid labels

Planner tests cover:

- path derivation
- config loading
- operation-mode derivation
- backup-target derivation
- btrfs validation during backup planning
- minimal fix-perms-only planning

Executor tests cover:

- operation-mode rendering for combined flows
- end-to-end dry-run execution for fix-perms-only
- lock lifecycle during execution

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
tests can assert on real stdout/stderr behavior without depending on logger
internals.

### Real Logger, Temporary Log Directory

Tests use a real logger pointed at a temporary directory rather than mocking the
logger itself. That keeps formatting behavior realistic while avoiding writes to
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
workflow behavior moves under `internal/workflow`, most new coordinator tests
should be added there unless they are specifically about the real entrypoint.
