# Architecture

This is the short architecture overview for the project.

If you want the detailed internal walkthrough, including the runtime path,
package boundaries, and where specific responsibilities now live, see
[how-it-works.md](how-it-works.md).

## Overview

The application follows an explicit `Request -> Plan -> Execute` flow.

That split keeps the entrypoint small, keeps planning non-mutating, and makes
tests easier to write around stable boundaries instead of one large
coordinator type.

## Top-Level Flow

`cmd/duplicacy-backup/main.go` is now thin wiring only:

```text
runWithArgs
  -> command.ParseRequest
  -> handled help/version output, or dispatchRequest
       -> config / notify / update / health / runtime
```

Only the runtime backup/prune/cleanup/fix-perms path goes through the full
`Planner.Build -> Executor.Run` sequence. Config, notify, update, and health
commands dispatch to their own narrower handlers so they do not inherit runtime
requirements such as root access, logger setup, or target storage secrets unless
that command actually needs them.

## Request

`internal/command` owns CLI intent and help/usage generation.

It is responsible for:

- parsing flags and the source label
- handling `--help` and `--version`
- deriving requested operation flags
- validating flag combinations
- validating the backup label

`internal/notify` owns notification payload types, provider delivery, and
notify-test reporting.

`internal/health` owns health report modelling, health-specific terminal
presentation, and JSON health report serialization.

`internal/presentation` owns shared operator-facing text formatting and the
runtime presenter used by workflow execution.

The `Request` type is intentionally small. It describes what the user asked
for, not what the application has resolved from the filesystem or config yet.
Requested operations can be combined, but execution order is fixed later in
the workflow so CLI flag order never changes runtime sequencing.

## Plan

For runtime operations, `internal/workflow/planner.go` turns a `Request` into a
validated `Plan`.

It is responsible for:

- root and binary dependency checks
- path derivation
- config loading and validation
- secrets loading and validation
- target-model resolution:
  - `storage = "..."`
  - `location = local | remote`
- backup-target derivation
- backup-mode btrfs validation
- execution-ready derived values such as:
  - operation mode
  - mode display
  - summary lines
  - dry-run and mode flags
  - prune/filter display values
  - ownership and threshold values
  - execution-ready command strings and cleanup inputs

The important design rule is that planning does not mutate operational state.
It can inspect the environment and run validations, but it does not acquire
locks, create work directories, create snapshots, or change permissions.

That is also where the storage semantics are decided. The planner uses
`type`, not `location`, to decide whether a target uses filesystem paths or
Duplicacy storage URLs, whether secrets should be loaded, and whether
`--fix-perms` is allowed.

## Execute

`internal/workflow/executor.go` owns side effects.

It is responsible for:

- signal handling
- log cleanup
- lock acquisition and release
- runtime sequencing
- Duplicacy working-directory setup
- backup, prune, storage cleanup, and fix-perms execution
- final cleanup and result output

This keeps operator-facing runtime behaviour in one place and makes phase order
easy to follow.

`Executor` now delegates presentation work to a small presenter, cleanup to
focused workflow helpers, and prune preview policy to dedicated workflow code.
It relies on the plan for execution decisions rather than repeatedly reaching
back into raw request/config data.

## Why This Shape

The main goal of the refactor was simplicity, not framework-building.

The codebase now has:

- a thin entrypoint in `cmd/duplicacy-backup`
- a command-surface package in `internal/command`
- a health package in `internal/health`
- a notify package in `internal/notify`
- an update package in `internal/update`
- a presentation package in `internal/presentation`
- one orchestration package in `internal/workflow`
- focused domain packages for config, secrets, btrfs, duplicacy, locking,
  permissions, logging, and process execution

That gives the application clear boundaries without over-splitting the design
into many small packages.

## Package Ownership Guidelines

When we add or change behaviour, the default question should be:

> Can this logic live in a focused package first?

The rule of thumb is:

- `internal/workflow` should coordinate work that is already defined elsewhere.
- Domain-oriented packages should own the logic that is specific to their area.
- `cmd/duplicacy-backup` should stay as thin entrypoint wiring.

In practice that means:

- put CLI parsing and help changes in `internal/command`
- put health-report and health-presentation logic in `internal/health`
- put notification delivery and provider logic in `internal/notify`
- put shared runtime/config formatting in `internal/presentation`
- put config, secrets, btrfs, duplicacy, permissions, and locking behaviour in their existing domain packages

`internal/workflow` is the place where those pieces are sequenced together.
It should own:

- request-to-plan orchestration
- execution sequencing
- workflow policy decisions that span multiple domains
- final operator-facing message translation

It should not be the default home for new provider logic, parser logic,
formatting logic, or health-specific semantics just because those features
happen to be used during execution.

## Main Packages

| Package | Purpose |
|---|---|
| `internal/command` | CLI request parsing and help / usage text |
| `internal/health` | Health reports, health JSON output, and health presentation |
| `internal/notify` | Notification payloads, provider delivery, and notify-test reports |
| `internal/presentation` | Shared output formatting and runtime presentation helpers |
| `internal/update` | Self-update planning, package verification, and installer execution |
| `internal/workflow` | Planning, execution, and summary composition |
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

Secrets should not be passed to external commands in argv. If a future command
does need a sensitive value, prefer environment variables or stdin. The command
runner redacts common sensitive flag patterns in debug and dry-run command
logs as a safety net, but redaction is not a substitute for keeping secrets out
of process arguments.

## Output Ownership

Operator-facing output is still owned by the top-level execution layer.
Domain packages return data or structured errors; they do not print their own
status messages.

The workflow layer also owns final error translation. Internal packages can
return rich typed errors while the workflow decides the final operator-facing
message. That keeps message formatting consistent and avoids spreading
user-facing tone across multiple packages.
