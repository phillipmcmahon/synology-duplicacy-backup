# Request Model Refactor

This note tracks the incremental refactor from one broad workflow `Request`
type toward command-specific input models.

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

## Migration Shape

The migration should stay incremental:

1. Keep parser behaviour stable.
2. Add a narrow request projection at a workflow boundary.
3. Move internal helpers for that command to the narrow type.
4. Keep adapters back to `Request` only where older planner code still needs
   the broad shape.
5. Remove each adapter once the planner or downstream code has a narrower
   contract of its own.

No command-surface changes should be introduced by this refactor unless they
are separately approved.

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
| Runtime operations | `RuntimeRequest` | Backup, prune, cleanup-storage, and fix-perms. Should remain the only path into `Planner.Build` and `Executor.Run`. |

## Current State

Restore now uses `RestoreRequest` internally. `HandleRestoreCommand` still
accepts the broad parser `Request`, then immediately projects it into
`RestoreRequest`.

Update and rollback now use `UpdateRequest` and `RollbackRequest` internally.
Their command dispatch still receives the parser `Request`, projects at the
boundary, and then passes only the narrow type into update/rollback execution.
These command paths do not need adapters back to `Request`.

Notify and diagnostics now use `NotifyRequest` and `DiagnosticsRequest`
internally.

Config planning now uses `ConfigPlanRequest`. Restore, notify, diagnostics,
config, and health command paths all feed planner config loading through that
narrow contract instead of adapting back to `Request`.

## Next Slices

The recommended order is:

1. Runtime operations last, because they interact with the full plan/executor
   lifecycle.
