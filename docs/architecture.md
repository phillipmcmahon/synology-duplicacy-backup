# Architecture

## Overview

The binary uses a coordinator pattern centered on the `app` struct in `cmd/duplicacy-backup/main.go`.

Top-level flow:

```text
newApp -> acquireLock -> loadConfig -> loadSecrets -> printHeader -> printSummary -> execute -> cleanup
```

## `newApp()` Initialization Sequence

`newApp()` is decomposed into focused sub-initializers:

```text
initLogger -> parseAppFlags -> validateEnvironment -> derivePaths -> installSignalHandler
```

Each stage has a single responsibility and returns either an exit code or an error.

## Phase Methods

| Method | Responsibility |
|---|---|
| `acquireLock()` | Acquire the directory-based PID lock |
| `loadConfig()` | Parse config, validate values, derive backup target |
| `loadSecrets()` | Load and validate remote secrets |
| `printHeader()` | Emit startup banner |
| `printSummary()` | Emit configuration summary |
| `execute()` | Dispatch backup, prune, and fix-perms phases |
| `prepareDuplicacySetup()` | Build working directory and preferences |
| `runBackupPhase()` | Execute backup |
| `runPrunePhase()` | Preview and execute prune |
| `runFixPermsPhase()` | Apply ownership and permission normalization |
| `cleanup()` | Remove temporary state and print final result |

## Internal Packages

| Package | Purpose |
|---|---|
| `internal/btrfs` | Btrfs validation and snapshot management |
| `internal/config` | Config parsing and validation |
| `internal/duplicacy` | Duplicacy CLI operations |
| `internal/errors` | Structured error types |
| `internal/exec` | Shared command runner and mocks |
| `internal/lock` | Directory-based PID locking |
| `internal/logger` | Structured logging and log cleanup |
| `internal/permissions` | Local repository permission fixing |
| `internal/secrets` | Secrets loading and validation |

## Command Runner

External process execution is centralized behind the `exec.Runner` interface. This keeps command wiring out of the domain packages and makes those packages testable with `MockRunner`.

Benefits:

- consistent dry-run behavior
- centralized stdout/stderr handling
- easier unit testing
- cleaner coordinator logic

## Output Ownership

The coordinator owns operator-facing output. Internal packages do work and return data or structured errors; they do not emit user-facing logs directly.

See the existing [message style guide](../MESSAGE_STYLE_GUIDE.md) for formatting conventions.
