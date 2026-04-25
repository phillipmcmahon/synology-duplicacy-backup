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
| Runtime operations | `RuntimeRequest` | Backup, prune, cleanup-storage, and fix-perms. Should remain the only path into `Planner.Build` and `Executor.Run`. |
| Config | `ConfigRequest` | Validate, explain, and paths. Should not carry runtime execution flags. |
| Health | `HealthRequest` | Status, doctor, and verify. Should separate health command intent from backup execution state. |
| Diagnostics | `DiagnosticsRequest` | Targeted support bundle generation and redaction options. |
| Notify | `NotifyRequest` | Provider, event, severity, scope, summary, and message only. |

## Current State

Restore now uses `RestoreRequest` internally. `HandleRestoreCommand` still
accepts the broad parser `Request`, then immediately projects it into
`RestoreRequest`.

Update and rollback now use `UpdateRequest` and `RollbackRequest` internally.
Their command dispatch still receives the parser `Request`, projects at the
boundary, and then passes only the narrow type into update/rollback execution.
These command paths do not need adapters back to `Request`.

The only intentional bridge back to `Request` is
`RestoreRequest.ConfigRequest()`, which feeds the existing config planner until
planning has its own narrower config-loading contract.

## Next Slices

The recommended order is:

1. Notify and diagnostics, because they already dispatch through focused
   handlers.
2. Config and health, because they share config-loading and validation paths.
3. Runtime operations last, because they interact with the full plan/executor
   lifecycle.
