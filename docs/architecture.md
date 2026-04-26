# Architecture

This is the short architecture overview for the project.

If you want the detailed internal walkthrough, including the runtime path,
package boundaries, and where specific responsibilities now live, see
[how-it-works.md](how-it-works.md).

## Overview

The application follows an explicit command-specific request model.

The parser still produces one dispatch envelope, but each workflow command
immediately projects that envelope into the narrow input type it actually
needs. Only runtime backup, prune, and cleanup-storage operations
continue into the `RuntimeRequest -> Plan -> Execute` path.

## Top-Level Flow

`cmd/duplicacy-backup/main.go` is now thin wiring only:

```text
runWithArgs
  -> command.ParseRequest
  -> handled help/version output, or dispatchRequest
       -> config / diagnostics / notify / restore / rollback / update / health / runtime
```

Only the runtime backup/prune/cleanup path goes through the full
`Planner.Build -> Executor.Run` sequence. Config, diagnostics, notify,
restore, rollback, update, and health commands dispatch to their own narrower
handlers so they do not inherit runtime requirements such as root access,
logger setup, or target storage secrets unless that command actually needs
them.

## Request

`internal/command` owns CLI intent and help/usage generation.

It is responsible for:

- parsing flags and the source label
- handling `--help` and `--version`
- deriving requested runtime commands
- validating command-specific modifiers
- validating the backup label

`internal/notify` owns notification payload types, provider delivery, and
notify-test reporting.

`internal/health` owns health report modelling, health-specific terminal
presentation, and JSON health report serialization.

`internal/presentation` owns shared operator-facing text formatting and the
runtime presenter used by workflow execution.

The parser `Request` remains the dispatch envelope between `internal/command`
and `internal/workflow`. It describes the raw CLI intent before workflow code
has resolved config, secrets, paths, or state.

Workflow handlers should not pass that broad shape deeper into command logic.
The boundary pattern is:

- project the parsed request into a command-specific request type
- validate and execute against that narrow type
- use `ConfigPlanRequest` when a command only needs label, target, config dir,
  and secrets dir resolution
- use `RuntimeRequest` only for backup, prune, and cleanup-storage

This keeps restore, update, rollback, notify, diagnostics, config, health, and
runtime concerns separated even though they share one command-line parser.

## Plan

For runtime operations, `internal/workflow/planner.go` turns a
`RuntimeRequest` into a validated `Plan`.

The `Plan` exposes smaller section views for review and tests:

- `PlanRequest` carries the resolved operator intent and mode flags
- `PlanConfig` carries values resolved from the selected label/target config
- `PlanPaths` carries derived filesystem and config/secrets paths
- `PlanDisplay` carries operator-facing command and summary strings

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

Duplicacy storage semantics live in `internal/duplicacy.StorageSpec`. The
planner uses that domain helper to decide whether storage keys should be
loaded.

Notification destinations use the same pattern at a smaller scale:
`internal/notify` owns provider lookup, destination construction, and delivery
dispatch for webhook and ntfy. New providers should be added there first, then
surfaced through config and command parsing.

## Execute

`internal/workflow/executor.go` owns side effects.

It is responsible for:

- signal handling
- log cleanup
- lock acquisition and release
- runtime sequencing
- Duplicacy working-directory setup
- backup, prune, and storage cleanup execution
- final cleanup and result output

This keeps operator-facing runtime behaviour in one place and makes phase order
easy to follow.

`Executor` now delegates presentation work to a small presenter, cleanup to
focused workflow helpers, and prune preview policy to dedicated workflow code.
It relies on the plan for execution decisions rather than repeatedly reaching
back into raw request/config data.

## Restore Subsystem

Restore is a first-class subsystem, not a single command handler. The workflow
package keeps the restore contract close to the other command orchestration,
while the tree picker lives in its own package.

| File or package | Responsibility |
|---|---|
| `internal/workflow/restore_command.go` | Top-level restore command orchestration and handoff to restore primitives |
| `internal/workflow/restore_context.go` | Shared resolved restore state such as config, storage, secrets, and plan context |
| `internal/workflow/restore_deps.go` | Dependency injection seam for clocks, runners, prompts, picker, and workspace defaults |
| `internal/workflow/restore_workspace.go` | Workspace resolution, derived workspace naming, and safety validation |
| `internal/workflow/restore_prompt.go` | Revision-first text prompts, confirmation, and cancellation handling |
| `internal/workflow/restore_parse.go` | Duplicacy revision and path parsing helpers |
| `internal/workflow/restore_format.go` | Operator-facing restore plan and report formatting |
| `internal/workflow/restore_reports.go` | Preview and result report models |
| `internal/restorepicker` | Interactive tree picker built on tview/tcell; compiles selections back to explicit restore primitives |

The guardrail is that `restore select` remains a convenience layer. It must
resolve to the same explicit `restore run` primitives used by expert and
scripted workflows.

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

## Future Watch

Two architecture pressure points are worth keeping visible:

- If `internal/workflow` grows another subsystem comparable in size to restore,
  health, or update, consider splitting that subsystem into a focused
  subpackage rather than continuing to expand the orchestration package.
- `Plan` currently exposes section views while still storing flat fields. If
  plan mutation or review complexity grows again, consider a deliberate sprint
  to make those section views the composed storage model rather than only a
  read-side view.

## Main Packages

| Package | Purpose |
|---|---|
| `internal/command` | CLI request parsing and help / usage text |
| `internal/health` | Health reports, health JSON output, and health presentation |
| `internal/notify` | Notification payloads, provider delivery, and notify-test reports |
| `internal/presentation` | Shared output formatting and runtime presentation helpers |
| `internal/update` | Self-update planning, package verification, installer execution, and managed rollback activation |
| `internal/workflow` | Planning, execution, diagnostics, and summary composition |
| `internal/btrfs` | Btrfs validation and snapshot management |
| `internal/config` | Config parsing and validation |
| `internal/duplicacy` | Duplicacy CLI operations |
| `internal/errors` | Structured internal error types |
| `internal/exec` | Shared command runner and mocks |
| `internal/lock` | Directory-based PID locking |
| `internal/logger` | Structured logging and log cleanup |
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
