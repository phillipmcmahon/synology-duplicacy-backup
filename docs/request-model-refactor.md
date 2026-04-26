# Request Model Refactor

This note records the refactor from one broad workflow `Request` type toward
command-specific input models.

The goal is design clarity, not a command-line compatibility layer. The CLI
parser may continue to produce the broad `Request` while each workflow handler
projects it into the smallest input type it needs.

## Why

The original `Request` type collected intent for every command: runtime
operations, restore, config, health, diagnostics, notify, update, and rollback.
That made dispatch simple, but it also encouraged handlers to depend on fields
that were unrelated to their command.

Command-specific request models make each workflow contract easier to review:

- restore code sees restore fields only
- update code sees update fields only
- diagnostics code sees diagnostics fields only
- runtime planning remains separate from non-runtime commands

## Design Rule

The parser still returns a broad `Request` dispatch envelope because one CLI
parser handles every command family. That broad shape should stay at the
workflow boundary.

The workflow rule is:

1. Dispatch receives the parsed `Request`.
2. The handler immediately projects it into the command-specific request type.
3. Internal helpers use the narrow type, not the parser envelope.
4. Config loading uses `ConfigPlanRequest` when only label, target, config dir,
   and secrets dir are needed.
5. Runtime execution uses `RuntimeRequest` with one `RuntimeMode`.

No command-surface changes were introduced by this refactor.

## Target Request Types

| Area | Target input model | Notes |
|---|---|---|
| Restore | `RestoreRequest` | Complete. Carries label, target, config/secrets dirs, workspace, revision, path, prefix, limit, dry-run, JSON, and confirmation intent. |
| Update | `UpdateRequest` | Complete. Carries config dir, version, keep count, attestations, check-only, force, and confirmation. |
| Rollback | `RollbackRequest` | Complete. Carries version, check-only, and confirmation. |
| Diagnostics | `DiagnosticsRequest` | Complete. Carries label, target, config/secrets dirs, and JSON output intent. |
| Notify | `NotifyRequest` | Complete. Carries label/target when needed, config/secrets dirs, provider, event, severity, scope, summary, message, dry-run, and JSON output intent. |
| Config planning | `ConfigPlanRequest` | Complete. Carries only label, target, config dir, and secrets dir for planner config loading. |
| Config | `ConfigRequest` | Complete. Carries validate, explain, and paths command intent without runtime execution flags. |
| Health | `HealthRequest` | Complete. Carries status, doctor, and verify intent without backup execution state. |
| Runtime operations | `RuntimeRequest` | Complete. Carries one `RuntimeMode` plus label, target, config/secrets dirs, dry-run, JSON, verbose, force-prune, and default notice. |

## Current State

Restore, update, rollback, notify, diagnostics, config, health, and runtime
all use command-specific request types internally.

The broad parser `Request` remains in `internal/workflow` as the parser
envelope returned by `internal/command.ParseRequest` and consumed by top-level
dispatch. It is not a compatibility layer. Residual uses should be limited to:

- handler entry points that project immediately
- projector constructors such as `NewRestoreRequest` or `NewRuntimeRequest`
- parser tests and workflow tests that exercise the dispatch boundary

It should not be threaded into lower-level command helpers, planner internals,
or executor internals.

`RuntimeRequest` is now the input to `Planner.Build`, `Planner.FailureContext`,
runtime failure presentation, and pre-run failure notifications. The previous
runtime mode booleans have been replaced at the runtime boundary by one
`RuntimeMode`.
