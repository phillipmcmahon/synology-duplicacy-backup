# Architecture

## Overview

The application follows an explicit `Request -> Plan -> Execute` flow.

That split keeps the entrypoint small, keeps planning non-mutating, and makes
tests easier to write around stable boundaries instead of one large
coordinator type.

## Top-Level Flow

`cmd/duplicacy-backup/main.go` is now thin wiring only:

```text
runWithArgs
  -> ParseRequest
  -> initLogger
  -> Planner.Build
  -> Executor.Run
```

## Request

`internal/workflow/request.go` owns CLI intent.

It is responsible for:

- parsing flags and the source label
- handling `--help` and `--version`
- deriving mode booleans
- validating flag combinations
- validating the backup label

The `Request` type is intentionally small. It describes what the user asked
for, not what the application has resolved from the filesystem or config yet.

## Plan

`internal/workflow/planner.go` turns a `Request` into a validated `Plan`.

It is responsible for:

- root and binary dependency checks
- path derivation
- config loading and validation
- secrets loading and validation
- backup-target derivation
- backup-mode btrfs validation
- execution-ready derived values such as:
  - operation mode
  - mode display
  - summary lines
  - dry-run and mode flags
  - prune/filter display values
  - ownership and threshold values

The important design rule is that planning does not mutate operational state.
It can inspect the environment and run validations, but it does not acquire
locks, create work directories, create snapshots, or change permissions.

## Execute

`internal/workflow/executor.go` owns side effects.

It is responsible for:

- signal handling
- log cleanup
- lock acquisition and release
- runtime sequencing
- Duplicacy working-directory setup
- backup, prune, and fix-perms execution
- final cleanup and result output

This keeps operator-facing runtime behavior in one place and makes phase order
easy to follow.

`Executor` now delegates presentation work to a small presenter and relies on
the plan for most execution decisions rather than repeatedly reaching back into
raw request/config data.

## Why This Shape

The main goal of the refactor was simplicity, not framework-building.

The codebase now has:

- a thin entrypoint in `cmd/duplicacy-backup`
- one orchestration package in `internal/workflow`
- unchanged domain packages for config, secrets, btrfs, duplicacy, locking,
  permissions, logging, and process execution

That gives the application clear boundaries without over-splitting the design
into many small packages.

## Main Packages

| Package | Purpose |
|---|---|
| `internal/workflow` | Request parsing, planning, execution, and summary composition |
| `internal/btrfs` | Btrfs validation and snapshot management |
| `internal/config` | Config parsing and validation |
| `internal/duplicacy` | Duplicacy CLI operations |
| `internal/errors` | Structured internal error types |
| `internal/exec` | Shared command runner and mocks |
| `internal/lock` | Directory-based PID locking |
| `internal/logger` | Structured logging and log cleanup |
| `internal/permissions` | Local repository permission fixing |
| `internal/secrets` | Secrets loading and validation |

## Command Runner

External process execution is centralized behind `internal/exec.Runner`.

That keeps shelling-out logic out of the domain packages and gives the workflow
layer one consistent way to run:

- `btrfs`
- `duplicacy`
- `chown`

The same abstraction is also what makes unit tests practical with
`exec.MockRunner`.

## Output Ownership

Operator-facing output is still owned by the top-level execution layer.
Domain packages return data or structured errors; they do not print their own
status messages.

The workflow layer also owns final error translation. Internal packages can
return rich typed errors while the workflow decides the final operator-facing
message. That keeps message formatting consistent and avoids spreading
user-facing tone across multiple packages.
