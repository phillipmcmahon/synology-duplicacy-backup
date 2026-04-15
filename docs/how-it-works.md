# How It Works

This document is the detailed internal guide for `synology-duplicacy-backup`.

It answers questions like:

- What actually happens when the binary starts?
- Which package owns which decisions?
- Where does config become runtime behaviour?
- Where do operator-facing messages come from?
- If I need to change backup, prune, storage cleanup, or fix-perms behaviour, where do I look?

If you want the short version, start with [architecture.md](architecture.md).
This is the longer walkthrough.

## Mental Model

The application now follows an explicit:

```text
Request -> Plan -> Execute
```

That is the core architectural idea.

- `Request` means: what the user asked for on the CLI.
- `Plan` means: the fully validated, resolved execution contract.
- `Execute` means: the side-effecting runtime path that actually does the work.

The main benefit of this split is that the code no longer mixes:

- argument parsing
- environment validation
- config/secrets loading
- summary rendering
- command execution
- cleanup

inside one large coordinator.

## Top-Level Runtime Flow

At runtime, the application enters through:

- [`cmd/duplicacy-backup/main.go`](../cmd/duplicacy-backup/main.go)

The high-level path is:

```text
main
  -> run
    -> runWithArgs
      -> command.ParseRequest
      -> initLogger
      -> workflow.NewPlanner(...).Build(...)
      -> workflow.NewExecutor(...).Run()
```

In other words:

1. Parse CLI intent.
2. Initialise logging.
3. Build a validated execution plan.
4. Execute the plan.

Supporting packages now keep adjacent concerns together:
- `internal/command` owns CLI request parsing and help text
- `internal/health` owns health reporting, health JSON output, and health presentation
- `internal/notify` owns notification delivery and notify-test reporting
- `internal/presentation` owns shared output formatting and the runtime presenter

## Architecture Overview

```mermaid
flowchart TD
    A["main.go"] --> B["command.ParseRequest"]
    B --> C["Request"]
    C --> D["Planner.Build"]
    D --> E["Plan"]
    E --> F["Executor.Run"]
    F --> G["Presenter"]
    F --> H["Cleanup helpers"]
    F --> I["Prune policy helpers"]
    D --> J["config"]
    D --> K["secrets"]
    D --> L["btrfs validation"]
    F --> M["duplicacy"]
    F --> N["btrfs snapshot ops"]
    F --> O["permissions"]
    F --> P["lock"]
    F --> Q["logger"]
    F --> R["exec runner"]
```

## Main Packages

### Entry point

- [`cmd/duplicacy-backup/main.go`](../cmd/duplicacy-backup/main.go)

This file should stay thin.

It owns:

- application version/build metadata
- runtime/bootstrap wiring
- logger initialisation
- transition from CLI arguments into workflow

It should not own business logic for backup, prune, storage cleanup, or
fix-perms behaviour.

### Command package

- [`internal/command`](../internal/command)

This is the command-surface layer.

It owns:

- request parsing
- CLI help and usage text
- request-level validation

### Workflow package

- [`internal/workflow`](../internal/workflow)

This is the orchestration layer.

It owns:

- runtime/environment seams
- plan building
- runtime execution
- operator-facing message translation
- workflow-specific execution sequencing and policy

This package is now the heart of the application.

### Health package

- [`internal/health`](../internal/health)

This package owns health-specific report modelling and presentation.

It owns:

- health JSON report shaping
- health report status/failure semantics
- health-specific terminal presentation helpers

### Presentation package

- [`internal/presentation`](../internal/presentation)

This package owns shared runtime/config presentation helpers.

It owns:

- runtime presenter behaviour used by backup/prune/fix-perms execution
- config/report line formatting helpers
- shared output-shaping logic that should not live in orchestration code

### Domain packages

These packages do focused work and should stay relatively narrow:

- [`internal/config`](../internal/config)
  Parses and validates config files.
- [`internal/secrets`](../internal/secrets)
  Loads and validates label secrets files.
- [`internal/btrfs`](../internal/btrfs)
  Validates btrfs locations and manages snapshots.
- [`internal/duplicacy`](../internal/duplicacy)
  Prepares and runs Duplicacy commands.
- [`internal/permissions`](../internal/permissions)
  Applies ownership and permission normalization for targets that allow local accounts.
- [`internal/lock`](../internal/lock)
  Directory-based PID locking.
- [`internal/logger`](../internal/logger)
  Structured log formatting and log cleanup.
- [`internal/exec`](../internal/exec)
  Shared command execution abstraction and test mocks.
- [`internal/errors`](../internal/errors)
  Typed internal error contracts.

## Request Phase

The request phase lives in:

- [`internal/command/request.go`](../internal/command/request.go)
- [`internal/command/usage.go`](../internal/command/usage.go)
- [`internal/workflow/request.go`](../internal/workflow/request.go)

The job of the request phase is to answer:

> What did the operator ask for?

It does not answer:

> Is that possible on this machine?
> Where are the files?
> What are the exact commands?

### What `Request` contains

The `Request` type contains user intent only:

- requested operations such as backup, prune, and storage cleanup
- `--fix-perms`
- `--force-prune` as a prune-threshold override
- `--target <name>` as the explicit destination selector
- `--dry-run`
- config/secrets directory overrides
- source label

It also derives convenience booleans such as:

- `DoBackup`
- `DoPrune`
- `DoCleanupStore`
- `FixPermsOnly`

Those operation flags can be combined. The CLI order does not matter; runtime
execution order is fixed as:

1. backup
2. prune
3. cleanup-storage
4. fix-perms

`--cleanup-storage` requests `duplicacy prune -exhaustive -exclusive` as a
standalone maintenance step. `--force-prune` only affects prune threshold
enforcement, so these are all valid and distinct intents:

- safe prune
- storage cleanup
- forced prune
- safe prune + storage cleanup
- forced prune + storage cleanup

Those are still request-level concepts because they describe intent, not machine state.

### What happens in `ParseRequest`

`ParseRequest` performs:

1. `--help` and `--version` early handling
2. raw flag parsing
3. explicit operation validation
4. request validation
5. source-label validation

### Why this matters

The request phase is intentionally cheap and non-invasive.

It does not:

- initialise work directories
- read config files
- check for root
- check for `duplicacy`
- acquire a lock

That keeps the CLI boundary predictable and easy to test.

## Runtime and Metadata Seams

The runtime abstraction lives in:

- [`internal/workflow/runtime.go`](../internal/workflow/runtime.go)

It provides injectable functions for:

- effective user id
- `PATH` lookups
- lock construction
- source-lock construction
- time
- temp dir
- PID
- environment variables
- executable discovery
- symlink evaluation
- signal registration

This is the main seam that makes entrypoint and workflow tests practical without
mocking whole packages.

`Metadata` holds stable application-level constants like:

- script name
- version
- build time
- root volume
- lock directory parent
- log directory

## Label-Target Model

The operational identity is now:

```text
label + target
```

Examples:

- `homes/onsite-usb`
- `homes/offsite-storj`

That identity flows through:

- config selection: `<label>-backup.toml`
- secrets selection: `<label>-secrets.toml` plus `[targets.<name>]`
- state files: `<label>.<target>.json`
- machine JSON summaries: `label` plus `target`
- repository-phase locking: one lock per label-target pair

This lets one source label keep multiple independent destinations without
forcing revision parity or schedule alignment between them.

Each target also resolves into a separate two-axis model:

```text
type + location
```

- `type` is the storage kind: `filesystem` or `object`
- `location` is the deployment location: `local` or `remote`

Supported combinations are intentionally limited to:

- filesystem/local
- filesystem/remote
- object/remote

This is what makes mounted remote filesystems first-class. A path reached over
VPN or SMB can now stay a filesystem target while still being modelled as
remote operationally.

## Plan Phase

The plan phase lives mainly in:

- [`internal/workflow/planner.go`](../internal/workflow/planner.go)
- [`internal/workflow/plan.go`](../internal/workflow/plan.go)
- [`internal/workflow/summary.go`](../internal/workflow/summary.go)

The job of the planner is to answer:

> Given this request and this machine, what exactly should execution do?

### Planning rules

Planning is allowed to:

- inspect the environment
- validate prerequisites
- read config
- read secrets
- derive paths
- derive operation mode text
- derive summary lines
- derive execution-ready command strings

Planning is not allowed to:

- create directories
- acquire locks
- create snapshots
- run Duplicacy operations
- change permissions
- delete anything

That is an important design rule.

### What `Planner.Build` does

`Build` performs these steps:

1. `validateEnvironment(req)`
2. `derivePlan(req)`
3. `loadConfig(plan)`
4. `loadSecrets(plan)` when the selected target uses object storage
5. `populateCommands(plan)`
6. `SummaryLines(plan)`

### `validateEnvironment`

This checks:

- root execution
- `duplicacy` availability when backup/prune work is requested
- `btrfs` availability when backup work is requested

### `derivePlan`

This creates the first concrete runtime shape:

- backup label
- timestamp
- temp work root
- snapshot source and target
- repository path
- config path
- secrets path
- mode display
- whether Duplicacy setup and snapshots are needed

This is where abstract user intent becomes machine-specific paths.

### `loadConfig`

This is where config becomes behaviour.

It:

- checks the config file exists
- parses the label config structure
  - top-level `label`
  - top-level `source_path`
  - optional `[common]`
  - optional `[health]`
  - optional `[health.notify]`
    - generic webhook JSON for health outcomes and opt-in runtime failures
    - optional native `[health.notify.ntfy]` destination
  - one or more `[targets.<name>]`
  - optional `[targets.<name>.health]`
  - optional `[targets.<name>.health.notify]`
- applies values into `config.Config`
- validates required keys
- validates thresholds
- validates the target model:
  - storage kind
  - deployment location
  - allowed combinations
- validates owner/group if `--fix-perms` is active
- builds prune args
- validates backup thread rules
- validates btrfs placement for backup mode

After this step, the plan is populated with things the executor can use directly:

- `Threads`
- `Filter`
- `FilterLines`
- `OwnerGroup`
- `PruneArgs`
- `LogRetentionDays`
- safe-prune thresholds
- operation mode string
- storage type
- location

Operationally, `source_path` is expected to be the real Btrfs volume or
subvolume root for the label. Fine-grained inclusion and exclusion under that
root is handled by Duplicacy filters, not by pointing `source_path` at an
arbitrary nested child directory.

### `loadSecrets`

This only runs for object targets.

It:

- resolves the exact secrets path
- loads the file
- validates ownership/permissions
- validates required secret values

The resulting `Secrets` object is attached to the plan.

Filesystem targets do not call `loadSecrets`, even when their `location` is
`remote`. That is a deliberate consequence of the new model: storage kind
drives credential requirements, not deployment location.

### `populateCommands`

This step is one of the most important recent improvements.

The plan now carries many execution-ready command descriptions, such as:

- snapshot create/delete
- work-dir creation/removal
- preferences write
- filter write
- work-dir permission fixes
- backup command
- repo validation
- prune preview
- policy prune
- storage cleanup
- fix-perms commands

These strings are used for:

- dry-run output
- tests
- keeping executor logic focused on sequencing instead of reconstructing command descriptions

### What the `Plan` now represents

The `Plan` is no longer just “resolved config plus some flags.”

It is the execution contract.

It contains:

- mode decisions
- resolved paths
- loaded secrets
- summary-ready values
- ownership and prune thresholds
- execution-ready command descriptions
- cleanup-relevant paths

The more complete the plan is, the less the executor has to know about request parsing or config internals.

## Execute Phase

The execution phase lives mainly in:

- [`internal/workflow/executor.go`](../internal/workflow/executor.go)
- [`internal/workflow/cleanup.go`](../internal/workflow/cleanup.go)
- [`internal/workflow/prune.go`](../internal/workflow/prune.go)
- [`internal/workflow/permissions_exec.go`](../internal/workflow/permissions_exec.go)

The job of the executor is to answer:

> Given this plan, in what order do we perform the side effects?

### What `Executor` owns

`Executor` owns:

- signal handling
- log retention cleanup
- lock acquisition
- header/summary printing
- Duplicacy setup
- backup execution
- prune execution
- fix-perms execution
- final cleanup
- exit code

It should not need to recalculate planning decisions.

### `Executor.Run`

The rough path is:

1. install signal handler
2. defer cleanup
3. log any default-mode notice
4. clean old logs
5. acquire lock
6. print header
7. print summary
8. execute operational phases
9. print success and exit `0`

If any step fails:

- the error is translated by workflow-owned messaging
- `exitCode` is set to `1`
- deferred cleanup still runs

## Presentation Layer

Presentation is handled by:

- [`internal/presentation/runtime.go`](../internal/presentation/runtime.go)

This package exists so `Executor` does not have to mix sequencing with formatting.

The presenter owns:

- startup header
- configuration summary
- command stdout/stderr streaming
- prune preview summary lines
- final completion block

This is intentionally small, but it helps keep runtime flow readable.

## Error Translation

Operator-facing message translation is handled by:

- [`internal/workflow/messages.go`](../internal/workflow/messages.go)

This is an important boundary.

The design rule is:

- domain packages return typed/internal errors
- workflow owns final operator-facing wording

That keeps message tone and punctuation consistent.

`config validate` also follows this rule. It performs a read-only readiness
probe for the selected repository and reports operator-facing outcomes such as
`Valid`, `Not initialized`, and `Invalid (...)` without initialising storage
or mutating repository state.

### Main error families

The translator understands:

- `RequestError`
- `MessageError`
- `ConfigError`
- `SecretsError`
- `LockError`
- `BackupError`
- `PruneError`
- `SnapshotError`
- `PermissionsError`

If an error is not explicitly translated, the workflow falls back to a normalized sentence version of `err.Error()`.

### Why this is useful

Without this layer, user-facing wording gets scattered across:

- config parsing
- secrets loading
- locking
- backup/prune execution
- cleanup

With this layer, output consistency has one main owner.

## Backup Flow

When backup mode is active, the runtime path is roughly:

1. planner validates environment and config
2. executor acquires lock
3. executor creates a read-only btrfs snapshot
4. executor creates the Duplicacy work directory
5. executor writes preferences
6. executor writes filters when configured
7. executor fixes work-dir permissions
8. executor runs `duplicacy backup`
9. cleanup deletes the snapshot and work directory
10. lock is released

The actual snapshot and Duplicacy work is delegated to:

- [`internal/btrfs`](../internal/btrfs)
- [`internal/duplicacy`](../internal/duplicacy)

## Prune Flow

When prune mode is active, the runtime path is roughly:

1. planner validates environment and config
2. executor acquires lock
3. executor prepares Duplicacy setup
4. executor validates repository access
5. executor runs safe-prune preview
6. executor enforces count/percentage thresholds
7. executor runs policy prune
8. executor optionally runs storage cleanup
9. cleanup removes work directory
10. lock is released

The interesting part here is that prune policy is enforced in workflow code, not buried inside the Duplicacy package.

That means:

- Duplicacy package gathers preview data
- workflow decides whether to continue

This is a good boundary because threshold enforcement is application policy, not a raw command concern.

## Fix-Perms Flow

When `--fix-perms` is active, the runtime path depends on mode:

- backup + fix-perms
- prune + fix-perms
- fix-perms only

The actual permission application is delegated to:

- [`internal/permissions`](../internal/permissions)

The workflow layer decides:

- when to run it
- which target path to use
- what summary and dry-run output to show

## Cleanup Lifecycle

Cleanup is handled in:

- [`internal/workflow/cleanup.go`](../internal/workflow/cleanup.go)

It is deliberately idempotent.

That matters because cleanup can run from:

- normal deferred exit
- error exit
- signal path

Cleanup currently handles:

- snapshot deletion
- snapshot directory removal
- Duplicacy work directory removal
- lock release
- final completion output

The executor tracks whether cleanup already ran so it can safely be called more than once.

## Logging and Output

Logging is handled by:

- [`internal/logger`](../internal/logger)

Workflow output uses the logger for:

- headers
- summary lines
- dry-run command lines
- streamed subprocess output
- warnings/errors
- final result blocks

Current message rules are:

- workflow owns final operator wording
- operator-facing messages should be concise and consistent
- status lines should not force terminal punctuation
- domain packages should avoid owning final tone/style

## Testing Strategy

The refactor changed the testing model too.

The main test layers are now:

### Request tests

These verify:

- flag parsing
- default mode behaviour
- label validation
- help/version handling
- invalid combinations

### Planner tests

These verify:

- config and secrets loading
- path derivation
- command-string population
- summary-ready values
- plan shape

### Executor tests

These verify:

- lifecycle ordering
- prune enforcement
- cleanup behaviour
- dry-run behaviour
- phase dispatch

### Entrypoint tests

These verify the real `runWithArgs` path end to end for representative cases.

See:

- [`TESTING.md`](../TESTING.md)

## Where To Change Things

If you want to change a specific behaviour, start here:

### CLI behaviour

- [`internal/command/request.go`](../internal/command/request.go)
- [`internal/command/usage.go`](../internal/command/usage.go)
- [`internal/workflow/request.go`](../internal/workflow/request.go)

### Path derivation and execution contract

- [`internal/workflow/planner.go`](../internal/workflow/planner.go)
- [`internal/workflow/plan.go`](../internal/workflow/plan.go)

### Summary rendering

- [`internal/workflow/summary.go`](../internal/workflow/summary.go)
- [`internal/presentation/runtime.go`](../internal/presentation/runtime.go)

### Health reports and health presentation

- [`internal/health/report.go`](../internal/health/report.go)
- [`internal/health/presenter.go`](../internal/health/presenter.go)

### Operator-facing error text

- [`internal/workflow/messages.go`](../internal/workflow/messages.go)

### Backup and prune sequencing

- [`internal/workflow/executor.go`](../internal/workflow/executor.go)
- [`internal/workflow/prune.go`](../internal/workflow/prune.go)

### Cleanup behaviour

- [`internal/workflow/cleanup.go`](../internal/workflow/cleanup.go)

### Duplicacy CLI setup and commands

- [`internal/duplicacy`](../internal/duplicacy)

### Config and secrets behaviour

- [`internal/config`](../internal/config)
- [`internal/secrets`](../internal/secrets)

## Practical Reading Order

If you have been away from the codebase and need to re-orient quickly, this is the reading order I would recommend:

1. [`cmd/duplicacy-backup/main.go`](../cmd/duplicacy-backup/main.go)
2. [`internal/command/request.go`](../internal/command/request.go)
3. [`internal/command/usage.go`](../internal/command/usage.go)
4. [`internal/workflow/request.go`](../internal/workflow/request.go)
5. [`internal/workflow/planner.go`](../internal/workflow/planner.go)
6. [`internal/workflow/plan.go`](../internal/workflow/plan.go)
7. [`internal/workflow/executor.go`](../internal/workflow/executor.go)
8. [`internal/presentation/runtime.go`](../internal/presentation/runtime.go)
9. [`internal/workflow/messages.go`](../internal/workflow/messages.go)

That path usually gives the clearest mental model with the least jumping around.

## Short Summary

If you want the shortest possible internal description:

- `Request` captures CLI intent.
- `Planner` turns intent into a validated execution contract.
- `Executor` performs the side effects in order.
- `Presenter` owns runtime rendering.
- `messages.go` owns final operator-facing wording.
- domain packages do focused work and return data or typed errors.

That is now the core shape of the application.
