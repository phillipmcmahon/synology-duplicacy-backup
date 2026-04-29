# Workflow Package Boundary Review

This note records the package-boundary review for `internal/workflow`.

## Design Rule

Design simplicity and operational clarity take priority over backwards
compatibility. If a CLI surface, config shape, output format, workflow, or
internal API needs to change to make the model clearer and safer, prefer the
cleaner contract over compatibility shims.

That rule applies to package boundaries too: split packages when the resulting
shape is simpler to reason about, not merely because a file count is high.

## Current Shape

`internal/workflow` remains the orchestration package. It wires parser requests,
resolved config, secrets, Duplicacy operations, health checks, restore drills,
runtime execution, reporting, locks, and notification handoffs.

The production files group naturally by command family:

| Family | Files | Notes |
|---|---:|---|
| runtime/core | 17 | `Plan`, `Planner`, `Executor`, runtime seams, state, summary, prune, cleanup, privilege, messages |
| restore | 12 | Restore orchestration, context, workspace, prompts, progress, parsing, reports |
| health | 7 | Health runner, preparation, checks, state, notifications, root-profile warnings |
| config | 3 | Config validate/explain/paths and config-specific request projection |
| notify | 3 | Notify command handling and runtime notification payloads |
| update | 3 | Update request/status/notification glue around `internal/update` |
| diagnostics | 2 | Diagnostics request and report orchestration |
| rollback | 1 | Rollback request projection |

The prefix discipline is working: command-family responsibilities are visible
from filenames, and domain-specific logic already lives in narrower packages
such as `internal/health`, `internal/notify`, `internal/update`,
`internal/presentation`, `internal/duplicacy`, `internal/config`,
`internal/secrets`, and `internal/btrfs`.

## Shared Types That Block A Split

The main split blockers are not file names. They are shared workflow contracts:

| Shared item | Why it blocks package movement |
|---|---|
| `Env` | Every command family uses the same OS/process/time/env seams for tests and DSM behaviour. |
| `Metadata` | Carries runtime paths, owner metadata, binary identity, root volume, and log/lock paths. |
| `Plan` | Now section-owned by `Request`, `Config`, and `Paths`, but still shared by runtime, config, health, diagnostics, restore, reporting, and notification code. |
| Request projections | `ConfigPlanRequest`, `RuntimeRequest`, `RestoreRequest`, `HealthRequest`, `NotifyRequest`, update/rollback requests provide the command-family boundary. |
| Error helpers | `RequestError`, `MessageError`, and `OperatorMessage` keep command surfaces consistent. |
| Privilege policy | Direct-root rejection and sudo-required local repository checks span config, health, restore, prune, and cleanup-storage. |
| Presentation/logging | Runtime command presenter and health/restore output share vocabulary and operator-facing conventions. |

`Plan` section ownership removes the worst ambiguity, but moving a command
family before the remaining ownership questions are resolved would still either
create import cycles or force a broad `workflow/core` package that is just the
old package under a different name.

## Post-Plan Core Boundary Pass

After `#255`, the smallest plausible shared core is:

| Candidate core item | Decision |
|---|---|
| `Env` | Core candidate. It is a genuine cross-command DSM/test seam. |
| `Metadata` | Core candidate. It carries runtime paths, app identity, root volume, and profile ownership. |
| `RequestError`, `MessageError`, `OperatorMessage` | Core candidate only if subpackages emit operator-facing errors directly. |
| Local repository privilege predicates | Core candidate only if restore/config/health/runtime all move out; otherwise keep in workflow to avoid policy drift. |
| `Plan` | Not a first core move. Keep in workflow until a real subpackage proves it needs the whole runtime plan contract. |
| `ConfigPlanRequest` | Not a first core move. It is a planner input, and moving it too early would pull planner ownership into core. |
| Runtime command presenter/logging | Not core. Subpackages should return reports/results unless there is a clear reason for them to own live output. |

The first extraction candidate is still restore. A restore split would need one
of two shapes:

1. `internal/workflow` keeps resolving config, plan, secrets, privilege policy,
   and logging, then calls a smaller restore package with an already-resolved
   execution context.
2. A new core package owns enough primitives for restore to resolve everything
   itself.

The second shape is not acceptable yet because the core would immediately
absorb too much: runtime profile resolution, metadata, config planning,
operator errors, local repository privilege checks, progress output, and report
formatting. That is not a cleaner model.

The first shape is more promising, but it requires a preparatory refactor:
separate restore's pure planning/reporting helpers from restore's workflow
resolution. Only the pure helpers should move first.

## Boundary Options

### Option A: Keep One Workflow Package

Keep `internal/workflow` as the orchestration package and continue extracting
domain logic into focused packages.

This preserves the current low-friction call graph and avoids a fake split.
The downside is that command-family dependencies remain enforced by review and
filename discipline rather than by Go import boundaries.

### Option B: Extract `internal/workflow/core`

Move `Env`, `Metadata`, `Plan`, request errors, privilege policy, and common
helpers into a shared core package, then move command families into
subpackages.

This gives explicit package boundaries, but it risks creating a large core
package that every subpackage imports. That would be more ceremony without much
clarity unless the shared core is deliberately small.

### Option C: Move One Command Family At A Time

Move one family, most likely restore, after its dependencies can point inward
to a small core rather than sideways to workflow internals.

Restore is the best candidate because it is already cohesive and has a clear
external UI. It is not ready to move until the shared strategy for `Plan`,
`Env`, `Metadata`, restore progress, request errors, and local repository
privilege policy is explicit.

## Decision

Do not split `internal/workflow` in this story.

The current package is large but still coherent as orchestration. After the
`#255` Plan refactor, the split is more feasible, but a physical split today
would still mostly move shared primitives around and create risk without
improving operator behaviour. The better design is:

- Keep `internal/workflow` as the orchestration package for now.
- Keep pushing domain logic down into focused packages first.
- Treat the `#255` Plan section ownership as completed groundwork.
- Revisit a restore subpackage only after restore's pure helpers are separated
  from workflow resolution, or when restore can depend on a deliberately small
  shared core rather than importing half of workflow by another name.

This is a design-clarity decision, not a backwards-compatibility compromise.
If a later split makes the model simpler, it should be allowed to change
internal APIs freely.

## Revisit Criteria

Reopen package splitting when at least one of these is true:

- Treat `substantial new behaviour` as a rough tripwire: one new
  command-family capability, three or more new production files, or about
  400+ lines of orchestration in the same family.
- Treat `starts reaching into` as two or more new cross-family helper calls,
  or use of another family's prefixed files instead of shared primitives.
- Treat `small shared core` as fewer than about eight exported primitives at
  first extraction; if the candidate needs `Plan`, planner, presenters, and
  handlers together, the split is too broad.
- A command family needs substantial new behaviour and would otherwise add many
  more files to `internal/workflow`.
- Restore, health, update, or runtime code starts reaching into another command
  family's files instead of shared primitives.
- `Env`, `Metadata`, request errors, and privilege policy have a small,
  clearly named home that is not just a dumping ground.
- Restore planning/reporting helpers can move without also moving config
  resolution, privilege policy, runtime profile handling, and live progress
  output.
- A proposed split can be made in reviewable mechanical commits while keeping
  tests green after each commit.

## Preferred Migration Sequence If Reopened

1. Keep the `Plan` section-owned shape explicit; do not reintroduce flat-field
   compatibility.
2. Audit actual imports and call sites for the candidate family.
3. Split pure helpers from workflow resolution inside the current package.
4. Define the smallest shared core API needed by that family.
5. Move tests with the package they validate.
6. Move one file cluster at a time and keep `go test ./...` green after each
   cluster.
7. Update `docs/architecture.md` and this review note with the new boundary.
