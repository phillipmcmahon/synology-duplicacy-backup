# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v4.0.0] - 2026-04-13

### Changed
- **Targets now use an explicit storage-kind and deployment-location model**:
  target config now uses `type = "filesystem" | "object"` plus
  `location = "local" | "remote"`, replacing the old overloaded
  `type = "local" | "remote"` scheme.
- **Mounted remote filesystems are now first-class**: destinations such as
  SMB-over-VPN paths can now be modelled as `filesystem/remote` targets
  without loading object-storage secrets or pretending to be local storage.
- **Operator output now surfaces the new target model consistently**: runtime,
  health, `config explain`, and `config paths` now render `Type` and
  `Location`, while `config validate` continues to keep `Resolved` identity-only
  and reports the target-model check under `Target Settings`.

### Fixed
- **Pre-run failure context is now fully aligned with the new target model**:
  when config can be resolved before a run aborts, pre-run failures now show
  `Type` and `Location` alongside `Operation`, `Label`, and `Target`.

### Validation
- **Local**: `go test ./...`
- **Local**: `go vet ./...`
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `85.0%`
  - `cmd/duplicacy-backup`: `86.2%`
  - `internal/workflow`: `84.8%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `84.3%`
  - `internal/secrets`: `81.0%`

## [v3.1.1] - 2026-04-12

### Changed
- **Operator output now starts with explicit identity everywhere**: runtime,
  health, and `config` screens now consistently surface the selected
  `label + target` at the start of operator-facing output, making local and
  remote runs easier to distinguish at a glance.
- **Dry-run phase flow is now more coherent**: prune-only and
  cleanup-storage-only dry-runs now show a dedicated `Setup` phase rather
  than interleaving setup commands into the run header area.
- **Runtime wording is more precise**: `fix-perms` now labels the resolved
  storage path as `Destination`, avoiding the old collision where `Target`
  could mean either the named target or the filesystem path.

### Fixed
- **Pre-run failures now carry useful run scope**: runtime failures that occur
  before the visible run starts now show `Run could not start` together with
  `Operation`, `Label`, and `Target` before the error and final result block.
- **Output-ordering regressions are better protected**: workflow and CLI tests
  now assert ordering and identity for runtime headers, health screens,
  dry-run setup phases, `fix-perms`, and the flat `config explain` /
  `config paths` views.

### Validation
- **Local**: `go test ./...`
- **Local**: `go vet ./...`
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `85.5%`
  - `cmd/duplicacy-backup`: `85.7%`
  - `internal/workflow`: `85.2%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `85.8%`
  - `internal/secrets`: `81.0%`

## [v3.1.0] - 2026-04-12

### Added
- **Stronger config validation preflight**: `config validate` now actively
  validates `source_path` accessibility, Btrfs suitability, local destination
  writability, remote destination resolution, target settings, health
  thresholds, threads, prune policy syntax, and secrets loading for the
  selected `label + target`.
- **Repository-aware config validation**: `config validate` now performs a
  read-only repository readiness probe and reports repository state directly to
  operators before runtime work begins.

### Changed
- **Config validation output is now structured like the rest of the tool**:
  `config validate` now uses `Resolved` and `Validation` sections plus a final
  `Result` line, with semantic colour for success, warning, and failure
  outcomes in interactive terminals.
- **Validation reporting now shows the whole preflight picture**: Parsed
  configs no longer fail fast on the first semantic problem. Independent checks
  continue and report `Valid`, `Invalid (...)`, `Not checked`, and other
  intentional outcomes so operators can fix multiple issues in one pass.
- **Release-facing docs now teach repository readiness explicitly**: README,
  CLI reference, configuration guide, cheat sheet, and the internal
  architecture walkthrough now explain that `config validate` is read-only and
  document the `Repository Access` outcomes operators should expect.
- **Examples now reinforce the explicit `label + target` model**: release
  docs and cheat-sheet comments consistently name both the label and target
  being acted on, without implying any default or hierarchy between targets.

### Fixed
- **Source path mistakes are surfaced before runtime**: `config validate` now
  catches non-existent paths, unreadable paths, and nested non-subvolume paths
  before backup runs fail during Btrfs snapshot preparation.
- **Uninitialised repositories are distinguished from broken access**:
  reachable-but-uninitialised repositories now report
  `Repository Access : Not initialized` with a short remediation hint, while
  true repository access failures report `Repository Access : Invalid (...)`
  without the initialisation guidance.
- **Config validation regressions are better protected**: workflow and CLI
  tests now lock the `Resolved` contract, constrain the allowed validation
  outcome vocabulary, and cover initialised, uninitialised, inaccessible, and
  dependency-blocked repository states.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `85.7%`
  - `cmd/duplicacy-backup`: `85.6%`
  - `internal/workflow`: `85.5%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `85.8%`

## [v3.0.0] - 2026-04-11

### Added
- **Named target operating model**: One backup label can now define multiple
  named targets inside a single `<label>-backup.toml` file, with one matching
  `<label>-secrets.toml` file for any target credentials. This makes
  configurations such as `homes/onsite-usb` and `homes/offsite-storj`
  first-class without forcing separate label files.
- **Target-specific runtime identity**: Health and run state now live under
  `/var/lib/duplicacy-backup/<label>.<target>.json`, and machine-readable JSON
  now carries explicit `label` and `target` identity for both run and health
  summaries.
- **Short operator cheat sheet for the new model**: The desk cheat sheet and
  install examples now document the explicit named-target workflow end to end,
  including config, secrets, health, and scheduler usage.

### Changed
- **CLI model is now fully explicit**: Every runtime, `config`, and `health`
  command now requires `--target <name>`, and runtime commands must also pass
  at least one explicit primary operation such as `--backup`, `--prune`,
  `--cleanup-storage`, or `--fix-perms`. The old implicit backup behaviour and
  target guessing have been removed.
- **Configuration is now per label rather than per label-target file**: The
  preferred operational layout is now one config file per label with shared
  `[common]` and `[health]` settings plus one or more `[targets.<name>]`
  tables. Secrets follow the same label-level structure with target-scoped
  entries under `[targets.<name>]`.
- **Operator language polished around the new target model**: Human-facing
  output now consistently uses natural target-oriented wording such as
  `Backup state`, `Config file`, `Secrets`, and `Backup freshness`, and it no
  longer leaks old-model `local` / `remote` terminology in normal screens.
- **Machine JSON now preserves exact user target labels**: `mode` and `target`
  now use the exact lower-case target label supplied in config, rather than
  title-casing or falling back to legacy local/remote names.
- **Release-facing docs aligned to the new architecture**: README, CLI,
  configuration, operations, testing guidance, installer help text, and the
  internal walkthrough now describe the same explicit named-target operating
  model.
- **Coverage raised for release readiness**: The target-first refactor now has
  release-ready coverage in the packages most exposed to operator experience,
  with `cmd/duplicacy-backup` at `83.7%` and `internal/config` at `84.3%`.

### Fixed
- **Independent target histories now stay independent**: Local and offsite
  targets under the same label no longer share runtime state, health recency,
  or revision expectations, so natural drift between destinations is handled
  correctly.
- **Snapshot naming collisions removed for same-label targets**: Snapshot names
  now include the target plus a unique suffix, avoiding same-second collisions
  when different targets for the same label run close together.
- **Locking now matches real operational boundaries**: The source snapshot
  phase remains label-scoped, while repository work and state updates are
  target-scoped, allowing safer same-label multi-target behaviour.
- **Interactive and JSON output now tell the same story**: Final wording,
  duration rendering, and target labelling are aligned across terminal output,
  JSON summaries, and health reports.

### Notes
- **Release recommendation**: This is a substantial operator-facing
  architecture shift and should be released as the first version of the named
  target model, with release notes calling out the explicit `--target`
  requirement and the new one-config-per-label layout.
- **Version constant** updated to `3.0.0` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v2.1.7] - 2026-04-11

### Added
- **Release-prep automation**: Added `make release-prep` plus a strict
  `scripts/release-prep.sh` flow that validates the release tree in Linux Go
  1.26, captures coverage, and writes a draft release-notes bundle under
  `build/release-prep/`.
- **Release environment guides**: Added concise repo documentation for the
  Linux validation environment and the standard public release process in
  `docs/linux-environment.md` and `docs/release-playbook.md`.

### Changed
- **Health verify wording polished**: Operator-facing health output now uses
  shorter and more natural verify labels and summaries such as `Revisions
  checked`, `Revisions passed`, `Revisions failed`, and `All revisions
  validated`, with cleaner Btrfs wording in healthy doctor output.
- **Unhealthy verify JSON tightened**: Unhealthy `health verify` now emits
  stable machine failure fields including `failure_code`, `failure_codes`, and
  `recommended_action_codes`, while keeping the healthy JSON surface compact.
- **Unhealthy verify screen simplified**: Hard verify failures now use shorter
  terminal messages such as `Could not inspect storage revisions`,
  `Repository is not ready`, and `Revision inspection failed`, without
  repeating low-value recency lines in the middle of the failure story.
- **Examples and operator docs clarified**: README, CLI, operations, examples,
  testing guidance, and the desk cheat sheet now reflect the settled health
  model, Linux-only packaging rules, and the current release process.

### Fixed
- **Verify failure reporting is easier to automate**: Unhealthy `health verify`
  no longer leaks rendered counter values into JSON issues, and the unhealthy
  machine contract now stays concise and predictable for monitoring.
- **Verify unhappy-path coverage expanded**: `internal/workflow` now covers
  controlled no-revision, failed-revision, missing-attribution, listing-failure,
  and verify-access-failure scenarios more thoroughly.

### Notes
- **Version constant** updated to `2.1.7` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v2.1.6] - 2026-04-11

### Added
- **Health command family**: Added `health status`, `health doctor`, and
  `health verify` as read-only operational confidence checks for Synology
  backup environments.
- **Health policy and notifications**: Added optional `[health]` and
  `[health.notify]` config tables plus optional
  `health_webhook_bearer_token` support in secrets TOML files.
- **Health state tracking**: Added per-label health/runtime state under
  `/var/lib/duplicacy-backup/<label>.json` to record recent successful runs
  and health-check cadence.
- **Integrity verification reporting**: `health verify` now inventories the
  revisions found for the current backup, runs `duplicacy check -persist`, and reports per-label
  revision counts plus failed revisions in both the human UI and JSON output.
  Healthy JSON runs now stay compact and only include per-revision detail when
  failures need investigation.
- **Desk cheat sheet**: Added a print-friendly quick reference under
  `docs/cheatsheet.md` with common commands, health checks, config commands,
  and installed-path reminders for day-to-day use.
- **Linux-only packaging automation**: Added shared packaging scripts that
  build, package, checksum, and smoke-test Synology release archives inside a
  Linux environment, plus `Makefile` targets for the supported Synology
  architecture matrix.

### Changed
- **Health screen layout polished**: Health output now uses the same block
  discipline as the rest of the tool, with shorter labels, clearer operator
  wording, compact recency strings, and structured alert reporting.
- **Health JSON contract tightened**: Health JSON now omits rendered check
  lines, always emits stable verify failure-summary fields, and uses explicit
  machine timestamps such as `last_doctor_run_at` and `last_verify_run_at`.
- **Verbose health output quieted**: Health runs no longer dump raw `exec:`
  command lines into the middle of the structured screen layout.
- **Health result colouring fixed**: `Healthy`, `Degraded`, and `Unhealthy`
  now use the expected semantic result colours in interactive terminals.
- **Operational alerting boundary clarified**: Built-in health webhooks are
  treated as a secondary alert path when config loads successfully, while
  Synology scheduled-task failure monitoring is the primary fallback for
  broken-environment and startup failures.
- **GitHub Actions packaging flow stabilised**: The release workflow now uses
  the shared Linux packaging script with safer argument handling for optional
  `GOARM` values, matching the local packaging process more closely.

### Fixed
- **Webhook CI tests aligned with production secrets rules**: Health webhook
  tests no longer depend on non-root temp secrets files or implicit
  `/root/.secrets` lookups, so the GitHub Actions test job now exercises the
  real webhook logic without failing on runner-specific file ownership and
  permission behaviour.
- **Webhook listener tests stabilised**: Health webhook tests now use a more
  controlled local-listener helper and skip cleanly in restricted environments,
  avoiding CI failures caused by listener setup rather than application logic.
- **Release CI now reflects real application health behaviour more reliably**:
  the webhook/listener test fixes remove false-negative release failures caused
  by runner constraints rather than actual application regressions.

### Notes
- **Version constant** updated to `2.1.6` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v2.1.5] - 2026-04-11

### Notes
- **Superseded by `v2.1.6`** before a successful public release was published.

## [v2.1.4] - 2026-04-10
### Notes
- **Superseded by `v2.1.6`** before a successful public release was published.

## [v2.1.3] - 2026-04-10

### Added
- **Config inspection commands**: Added `config validate`, `config explain`,
  and `config paths` so operators can check resolved config, secrets, and
  stable filesystem paths before running a real job.
- **Machine-readable run summaries**: Added `--json-summary` to emit a JSON
  completion summary on stdout while keeping the existing human-readable logs
  on stderr.

### Changed
- **Help surface simplified**: `--help` and `config --help` are now concise
  quick-reference views, with `--help-full` and `config --help-full` providing
  the detailed reference.
- **Interactive safety rails added**: TTY runs now ask for confirmation before
  forced prune and storage cleanup, while non-interactive runs continue
  without prompts.
- **Run timing aligned with visible output**: Start-block timestamps, final
  duration lines, and JSON `duration_seconds` now describe the same visible
  run window and use truncation rather than rounding.
- **Release validation hardened**: Linux Go 1.26 is now the representative
  validation environment for testing and packaging, and release automation
  includes packaged-artefact smoke checks.
- **Workflow coverage raised for release prep**: `internal/workflow` now
  validates at 90.1%% statement coverage in the Linux Go 1.26 release
  environment, with expanded tests across request parsing, planning,
  observability, presentation, safety rails, and execution flow.

### Notes
- **Version constant** updated to `2.1.3` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v2.1.2] - 2026-04-10

### Changed
- **Interactive activity line aligned with normal log output**: The live
  TTY-only status indicator for long-running phases now keeps the standard
  timestamp and `[INFO]` prefix instead of rendering as an unprefixed line.
- **Long-running phase activity expanded**: Backup now uses the same live
  interactive activity pattern as prune, storage cleanup, and fix permissions
  so all potentially quiet phases give a clearer sense of progress on a TTY.

### Notes
- **Version constant** updated to `2.1.2` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v2.1.1] - 2026-04-10

### Changed
- **Long-running phase status lines added**: Prune, storage cleanup, and fix
  permissions now emit in-phase status updates before slower work begins so the
  UI no longer appears stalled while repository inspection or permission
  changes are in progress.
- **Duration reporting expanded**: Prune, storage cleanup, and fix permissions
  now include per-phase duration lines, and the final completion block now
  reports total run duration.
- **Phase block presentation standardized**: Removed the storage-cleanup-only
  `Action` row so all phase blocks follow the same cleaner structure.

### Notes
- **Version constant** updated to `2.1.1` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v2.1.0] - 2026-04-10

### Added
- **Composable operations with fixed execution order**: Primary operations can
  now be combined in one invocation and always run in a predictable sequence:
  backup, prune, storage cleanup, then fix permissions.
- **Standalone storage cleanup operation**: Added `--cleanup-storage` as a
  first-class maintenance operation for `duplicacy prune -exhaustive -exclusive`
  without tying it to retention-policy pruning.

### Changed
- **Prune semantics clarified**: `--force-prune` now overrides only safe prune
  threshold enforcement. Storage cleanup is no longer presented as a stronger
  form of prune.
- **Runtime output redesigned**: Normal output is now phase-oriented and much
  quieter by default, with clearer start/completion framing, user-focused phase
  blocks, and lower-noise verbose and dry-run detail rendering.
- **CLI validation tightened**: Extra positional arguments now fail fast
  instead of being silently ignored.
- **Help and docs updated**: Help text, README, CLI reference, operations, and
  internal architecture/testing docs now reflect the combined-operation model
  and current output behaviour.

### Notes
- **Version constant** updated to `2.1.0` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v2.0.0] - 2026-04-09

### Changed
- **Config and secrets migrated to TOML**: Legacy INI config files
  (`<label>-backup.conf`) and `.env` secrets files (`duplicacy-<label>.env`)
  are replaced by TOML-only files:
  - config: `<label>-backup.toml`
  - secrets: `duplicacy-<label>.toml`
- **Schema keys renamed to lower snake case**: Config keys now use TOML-style
  names such as `destination`, `threads`, `local_owner`, and
  `safe_prune_max_delete_percent`. Remote secrets now use `storj_s3_id` and
  `storj_s3_secret`.

### Migration
- Convert and rename both files before upgrading. Example:
  - `homes-backup.conf` -> `homes-backup.toml`
  - `duplicacy-homes.env` -> `duplicacy-homes.toml`
  - `DESTINATION=/backups` -> `destination = "/backups"`
  - `STORJ_S3_ID=...` -> `storj_s3_id = "..."`

### Notes
- **Version constant** updated to `2.0.0` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v1.10.3] - 2026-04-09

### Changed
- **Installer config path clarified**: Documentation now makes it explicit that
  the stable command path `/usr/local/bin/duplicacy-backup` resolves its default
  config directory under `/usr/local/lib/duplicacy-backup/.config/` because the
  installed command is a symlink to the real versioned binary.
- **Planner output trimmed for fast-fail checks**: Removed short-lived config
  and secrets load start/success messages so missing-config and missing-secrets
  failures surface with less noise.
- **Version constant** updated to `1.10.3` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v1.10.2] - 2026-04-09

### Changed
- **Release packaging simplified**: GitHub releases now publish only the
  packaged `.tar.gz` archives rather than separate raw binary downloads. The
  tarballs are now the canonical install unit and include the versioned binary,
  `install.sh`, `README.md`, and `LICENSE`.
- **Checksum generation cleaned up**: Per-file `.sha256` files and
  `SHA256SUMS.txt` are now generated only for the actual tarball release
  artefacts, avoiding hashes for checksum files or intermediate raw binaries.
- **Version constant** updated to `1.10.2` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v1.10.1] - 2026-04-09

### Added
- **Detailed internal architecture guide**: Added `docs/how-it-works.md`, a
  long-form internal walkthrough covering the `Request -> Plan -> Execute`
  runtime model, package responsibilities, backup/prune/fix-perms flows,
  cleanup lifecycle, and where to change specific behaviours.

### Changed
- **Quieter prune output**: Safe prune operations no longer print the full raw
  `[REVISION-LIST]` dump from `duplicacy list`, while still keeping revision
  counting, threshold enforcement, and the summarized preview lines.
- **Docs navigation improved**: The README and short architecture guide now
  link directly to the new detailed internal walkthrough for easier re-entry
  into the codebase after larger refactors.
- **Version constant** updated to `1.10.1` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v1.10.0] - 2026-04-09

### Added
- **Explicit `Request -> Plan -> Execute` workflow architecture**: Added the
  new `internal/workflow` package to separate CLI intent parsing, execution
  planning, runtime presentation, and side-effecting execution into clearer
  layers.
- **Richer execution plan contract**: `Plan` now carries more execution-ready
  fields, including precomputed owner/group values, summary-ready metadata, and
  dry-run command strings used by the executor and tests.
- **Broader workflow test coverage**: Added focused planner, executor, summary,
  and message translation tests, plus stronger `runWithArgs` assertions around
  end-to-end stderr output and completion messaging.

### Changed
- **`main.go` slimmed to entrypoint wiring**: The command entrypoint now mostly
  boots metadata/runtime dependencies, parses requests, builds a plan, and hands
  off to workflow execution rather than owning the full coordinator logic.
- **Executor responsibilities reduced**: Cleanup, prune-policy handling, and
  fix-permissions execution were split into dedicated workflow files so the
  executor is primarily responsible for orchestration and lifecycle ordering.
- **Operator messaging tightened**: Final error translation and runtime status
  lines now follow a more consistent sentence-style contract, with workflow
  owning operator-facing wording more explicitly.
- **Typed error handling extended**: Domain-package errors are translated more
  deliberately through the workflow message layer, reducing reliance on raw
  fallback strings.
- **Architecture and testing docs refreshed**: `docs/architecture.md`,
  `TESTING.md`, and related references were updated to reflect the new workflow
  boundaries and message-style rules.
- **Version constant** updated to `1.10.0` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v1.9.0] - 2026-04-09

### Added
- **Function-variable seams for full-pipeline testability**: Package-level
  variables `cliArgs`, `geteuid`, `lookPath`, and `newLock` can be swapped
  during tests to simulate non-root users, missing binaries, and lock failures
  without modifying `os.Args` or requiring real system state.
- **`runWithArgs()` entry point**: The coordinator's `run()` function now
  delegates to `runWithArgs(args []string) int`, enabling direct testing of the
  full coordinator pipeline with controlled arguments and mocked dependencies.
- **`captureOutput()` test helper**: New helper in `main_test.go` that redirects
  `os.Stdout` and `os.Stderr` during tests for assertion on console output.
- **Six new coordinator pipeline tests**: `TestRunWithArgs_HelpReturnsZero`,
  `_VersionReturnsZero`, `_InvalidFlagReturnsOne`, `_NonRootReturnsOne`,
  `_ConfigLoadFailureReturnsTranslatedError`,
  `_LockAcquisitionFailureReturnsTranslatedError` — covering early-exit paths,
  error translation, and exit codes.
- **`TestParseSecrets_ReaderError`**: Verifies `ParseSecrets` correctly handles
  underlying `io.Reader` failures via a new `errReader` stub.

### Changed
- **Documentation reorganised**: The README was simplified to a concise quick-start
  guide. Detailed reference material was extracted into four new files under `docs/`:
  - `docs/architecture.md` — coordinator pattern, app struct, error handling
  - `docs/cli.md` — full CLI usage, flags, modes, and modifiers
  - `docs/configuration.md` — config file format, secrets, environment overrides
  - `docs/operations.md` — Synology Task Scheduler setup, log locations, tips
- **TESTING.md updated**: Added documentation for function-variable seams,
  `runWithArgs()`, `captureOutput()` helper, and updated test metrics
  (487 tests passing, 90.1% statement coverage).
- **Version constant** updated to `1.9.0` in source (overridden by `-ldflags`
  at build time for release binaries).

## [v1.8.2] - 2026-04-09

### Fixed
- **P2: RunInDir test fails on symlinked temp roots** — `runner_test.go` now
  normalises both the `pwd` output and the expected directory through
  `filepath.EvalSymlinks` before comparison, preventing false failures when
  `/tmp` is a symlink (e.g. macOS, some Linux distros).
- **P3: Secrets parser coverage artificially low** — Extracted `ParseSecrets()`
  and `ValidateFileAccess()` from `LoadSecretsFile()` in `internal/secrets`.
  Parser tests now exercise `ParseSecrets` directly via `io.Reader`, removing
  the root-only skip guards.  Coverage rose from ~50 % to 86 %.
- **P3: Integration tests incomplete** — Added seven new coordinator
  end-to-end tests (`TestIntegration_RunCoordinator_*`) exercising the full
  `acquireLock → loadConfig → loadSecrets → execute → cleanup` pipeline for
  prune, backup, and fix-perms dry-run modes, plus error-propagation and
  idempotent-cleanup scenarios.

### Added
- **`secrets.ParseSecrets(r io.Reader, source string)`** — exported parser
  function testable without file ownership checks.
- **`secrets.ValidateFileAccess(path string)`** — exported access-control
  validator, separated from parsing for independent testing.
- **`TestParseSecrets_*` test suite** — 13 parser tests that run on any
  machine (no root required), plus `TestParseSecrets_DuplicateKey` and
  `TestParseSecrets_SourceInErrorMessage` for additional edge coverage.
- **`TestValidateFileAccess_*` tests** — dedicated unit tests for the new
  access-validation function.
- **`itestApp()` helper** — reusable factory for coordinator integration
  tests, reducing boilerplate.

## [v1.8.1] - 2026-04-09

### Fixed
- **P3 follow-up: help text now reflects effective config dir** — `printUsage()`
  now uses the new `effectiveConfigDir()` function, which mirrors the runtime
  `resolveDir()` logic by checking `DUPLICACY_BACKUP_CONFIG_DIR` before falling
  back to the executable-relative default. The label changed from "Default:" to
  "Effective default:" for clarity.

### Added
- **`effectiveConfigDir()` function** — encapsulates the env-var-aware config
  directory resolution, keeping the help text and runtime in sync and preventing
  future drift.
- **Comprehensive tests for P1 and P3 fixes** — `run()` level integration tests
  (`TestRun_HelpReturnsZero`, `TestRun_VersionReturnsZero`), `effectiveConfigDir`
  unit tests, help output content verification, and version output format checks.

## [v1.8.0] - 2026-04-09

### Added
- **Structured error types** (`internal/errors`): New package providing typed
  errors (`BackupError`, `PruneError`, `SnapshotError`, `PermissionsError`,
  `ConfigError`, `SecretsError`, `LockError`) with phase, field, cause, and
  context map for precise error handling at the coordinator level.
- **Message style guide** (`MESSAGE_STYLE_GUIDE.md`): Documents the conventions
  for operator-facing messages — sentence case with punctuation, phase bracketing,
  dry-run prefixes, and structured error usage.

### Changed
- **Output ownership consolidation** (Phase 3): Internal packages (`btrfs`,
  `duplicacy`, `permissions`) no longer accept a `*logger.Logger` parameter or
  log directly. They return structured errors and raw command output (stdout,
  stderr). The coordinator in `main.go` now owns all operator-facing messages.
- **`duplicacy.RunBackup`**, **`RunPrune`**, **`RunDeepPrune`** now return
  `(stdout, stderr string, error)` instead of just `error`.
- **`duplicacy.GetTotalRevisionCount`** now returns `(int, string, error)`,
  including the raw command output.
- **`duplicacy.Cleanup`** now returns `error` instead of nothing.
- **`duplicacy.PrunePreview`** struct gains `Output` and `RevisionOutput`
  fields for raw command output.
- **`btrfs.CheckVolume`**, **`CreateSnapshot`**, **`DeleteSnapshot`** no longer
  accept a logger parameter; they return `*errors.SnapshotError`.
- **`permissions.Fix`** no longer accepts a logger parameter; returns
  `*errors.PermissionsError`.
- **`duplicacy.NewSetup`** no longer accepts a logger parameter.
- Phase messages (start/end) are now emitted by the coordinator around each
  operation phase.

## [v1.7.5] - 2026-04-09

### Fixed
- **Prune preview deletion counting**: Fixed the deletion counting regex to
  correctly detect "Deleting snapshot data at revision X" lines from Duplicacy's
  prune dry-run output. Previously, the regex only matched "Deleting revision X"
  but Duplicacy outputs "Deleting snapshot data at revision X", causing the
  "Preview Deletes" count to always show 0. The regex now correctly handles any
  text between the delete verb and "revision", matching all known Duplicacy
  output formats.

## [v1.7.4] - 2026-04-08

### Fixed
- **Snapshot field hidden in non-backup modes**: The "Snapshot" field in the
  configuration summary is now only displayed when a BTRFS snapshot is actually
  created (i.e., during backup operations). Previously, prune and other
  non-backup modes showed `Snapshot: /volume1/homes` which was identical to the
  `Source` field and misleading — no snapshot exists in those modes. The field is
  now omitted entirely when no snapshot is created, eliminating user confusion.

## [v1.7.3] - 2026-04-08

### Fixed
- **Config summary display**: Renamed misleading "Repository" label to "Snapshot"
  in the config summary output. The "Repository" label was confusing because it
  displayed the local BTRFS snapshot path (or source path in prune mode), not the
  backup destination. The backup destination was already correctly shown as
  "Destination". In prune-only mode, the old "Repository" field showed the same
  value as "Source" (since no snapshot is created), which users mistakenly
  interpreted as a bug. The new "Snapshot" label accurately describes the field.

## [v1.7.2] - 2026-04-08

### Fixed
- **`--force-prune` validation**: `--force-prune` used without `--prune` or
  `--prune-deep` now emits an error and exits immediately instead of logging a
  warning and continuing with a backup. This prevents accidental backup runs
  when the user intended a forced prune operation.

## [v1.7.1] - 2026-04-08

### Added
- **`--version` / `-v` flag**: displays version and build information, a standard
  CLI feature that works without root privileges or any dependencies.
- **Usage help on errors**: when invalid flags or missing arguments are provided,
  the full usage help is now printed after the error message, guiding users to
  correct their command.

### Changed
- Version constant set to `1.7.1` in source (overridden by `-ldflags` at build
  time for release binaries).
- `README.md` updated to document the new `--version` / `-v` flag.

## [v1.7.0] - 2026-04-08

### Added
- **Release archives**: each release now includes `.tar.gz` archives containing
  the binary, `README.md`, and `LICENSE` for every platform target.
- **Per-asset checksums**: individual `.sha256` files are generated for every
  release asset (raw binaries and archives) alongside the existing combined
  `SHA256SUMS.txt`.
- **Verification documentation**: new "Verifying Release Downloads" section in
  `README.md` with step-by-step instructions for checksum verification, archive
  inspection, and macOS usage notes.

### Changed
- Build workflow (`build.yml`) updated:
  - Binary naming convention changed from `duplicacy-backup-{os}-{arch}` to
    `duplicacy-backup_{version}_{os}_{arch}` to include the version tag.
  - Added explicit `permissions: contents: read` to lint, test, and build jobs.
  - Packaging step creates a directory archive with binary, README, and LICENSE.
  - Release step generates individual `.sha256` files for every asset before
    creating the combined `SHA256SUMS.txt`.

## [v1.6.6] - 2026-04-08

### Fixed
- Comprehensive fix for all error/warning message formatting. Moved logger
  initialisation to occur immediately after colour detection — before flag
  parsing, label validation, dependency checks, and flag-combination validation.
  This ensures **every** error and warning message now goes through the logger,
  gaining consistent timestamp prefixes, colour formatting, and log-file capture.
- Previously affected messages (now fixed):
  - `[ERROR] Must be run as root.`
  - `[ERROR] unknown option --reset` (and all flag-parsing errors)
  - `[ERROR] Invalid source label: ...`
  - `[ERROR] Required command 'duplicacy' not found`
  - `[ERROR] Required command 'btrfs' not found (needed for backup snapshots)`
  - `[ERROR] --prune-deep requires --force-prune`
- The only remaining raw-stderr message is the logger-init-failure fallback,
  which is unavoidable since the logger itself failed to initialise.

### Notes
- No functional changes to validation logic — only output formatting
- Affected file: `cmd/duplicacy-backup/main.go`

## [v1.6.5] - 2026-04-08

### Fixed
- Fixed error message formatting for `--fix-perms` + `--remote` validation: moved the
  check after logger initialisation so it now uses `log.Error()` instead of raw
  `fmt.Fprintln`. The error message now displays with the standard timestamp prefix
  and red colour formatting, consistent with all other error messages.
- Also moved `--force-prune` warning to use `log.Warn()` for the same consistency.

### Notes
- No functional changes to validation logic itself — only the output formatting
- Affected file: `cmd/duplicacy-backup/main.go`

## [v1.6.4] - 2026-04-08

### Fixed
- Added missing operation mode string for "Prune deep + fix permissions" combination.
  Previously, when running `--prune-deep --fix-perms`, the operation mode display
  fell through to "Prune deep" without acknowledging the fix-perms flag. The new
  `else if` branch ensures the combined mode is correctly reported.

### Notes
- Single-line fix in `cmd/duplicacy-backup/main.go` operation mode derivation logic
- No functional changes to backup, prune, or permission operations themselves

## [v1.6.3] - 2026-04-08

### Changed
- Replaced hardcoded separator strings with `log.PrintSeparator()` calls for consistency
- Normalised "Backup Script Started" heading to sentence case ("Backup script started")

### Notes
- Cosmetic-only changes to `cmd/duplicacy-backup/main.go`
- No functional or behavioural changes

## [v1.6.2] - 2026-04-08

### Changed
- Aligned the Go binary's runtime output more closely with the original shell script
- Matched summary labels, field selection, and operation-mode display more closely to the shell script
- Adjusted fix-perms logging to mirror the shell script wording and line layout
- Tuned logger separators and colour presentation to better match the shell script style
- Streamed `duplicacy`, `btrfs`, and `chown` subprocess output without Go-specific wrapper prefixes
- Updated several dry-run, cleanup, validation, and failure messages for closer shell script parity

### Notes
- This release focuses on output/style consistency between the Go implementation and the shell script
- Supported runtime behaviour is intended to remain unchanged


## [1.6.1] - 2026-04-08

### Changed
- **Conditional display of Local Owner/Group:** The "Local Owner" and
  "Local Group" fields are now only shown in the configuration summary when
  `--fix-perms` is active. Plain backup and prune operations no longer display
  these fields, keeping the output focused on relevant settings.
- **Minimal summary for standalone fix-perms:** When running `--fix-perms`
  alone (without `--backup` or `--prune`), the configuration summary is trimmed
  to show only: Operation Mode, Destination, Local Owner, Local Group, and
  Dry Run. Backup-specific settings (threads, filters, prune options,
  thresholds, etc.) are hidden since they are not relevant.
- **Operation Mode printed first:** The "Operation Mode" line now appears at
  the top of the configuration summary for immediate visibility.

### Added
- New display-context tests: `TestDisplayContext_FixPermsOnly_MinimalSummary`,
  `TestDisplayContext_BackupOnly_NoOwnerGroup`,
  `TestDisplayContext_PruneOnly_NoOwnerGroup`,
  `TestDisplayContext_BackupWithFixPerms_ShowsOwnerGroup`,
  `TestDisplayContext_PruneWithFixPerms_ShowsOwnerGroup`,
  `TestDisplayContext_RemoteBackup_NoOwnerGroup`.

### Note
- Patch release: output formatting changes only. No changes to validation
  logic, command behaviour, or configuration file format.

## [1.6.0] - 2026-04-08

### Changed
- **Conditional owner/group validation:** `ValidateOwnerGroup` (which checks
  that `LOCAL_OWNER` and `LOCAL_GROUP` are valid, non-root, and exist on the
  system) is now only called when `--fix-perms` is supplied. Plain backup and
  prune operations no longer perform these potentially expensive user/group
  look-ups, reducing startup overhead on systems where the backup user is
  configured but not needed for the current operation.
- **Conditional duplicacy binary check:** The `duplicacy` binary look-up
  (`exec.LookPath`) is now only performed when `doBackup` or `doPrune` is true.
  Standalone `--fix-perms` no longer requires `duplicacy` to be installed,
  since it only calls `chown`/`chmod`.

### Added
- New mode-derivation tests: `TestModeDerivation_FixPermsOnlySkipsBackupAndPrune`,
  `TestModeDerivation_BackupRequiresDuplicacy`,
  `TestModeDerivation_PruneRequiresDuplicacy`,
  `TestModeDerivation_FixPermsWithBackupRequiresBoth`.
- New config validation tests: `TestValidateOwnerGroup_SkippableForBackupOnly`,
  `TestValidateOwnerGroup_RequiredForFixPerms`.

### Note
- Minor version bump: validation behaviour has changed (validations are now
  skipped in certain modes), but no breaking changes to existing workflows.
  All operations that previously passed validation will continue to work.
  Operations that previously failed due to missing `LOCAL_OWNER`/`LOCAL_GROUP`
  in backup-only or prune-only mode will now succeed.

## [1.5.2] - 2026-04-08

### Changed
- **Improved output:** `LOCAL_OWNER` and `LOCAL_GROUP` fields are now completely
  hidden in remote mode output instead of showing `<n/a>` placeholders. This
  mirrors how S3 credentials are only shown in remote mode, keeping the output
  clean and relevant to each operation mode.

## [1.5.1] - 2026-04-08

### Changed
- **BTRFS validation now conditional:** The `btrfs` command lookup and
  `btrfs.CheckVolume` calls are now only performed when actually needed
  (backup operations that create/delete BTRFS snapshots). Previously, BTRFS
  validation ran unconditionally at startup and for all backup/prune modes,
  even when BTRFS was not involved.
  - `--remote --prune` no longer requires or validates BTRFS.
  - `--fix-perms` standalone no longer requires the `btrfs` command.
  - `--prune` (local or remote) no longer validates BTRFS volumes.
  - Backup operations (`--backup` or default mode) continue to validate
    BTRFS as before, since they create read-only snapshots.

## [1.5.0] - 2026-04-08

### Fixed
- **Hidden backup bug:** `--fix-perms` alone no longer triggers a full backup.
  Previously, running `--fix-perms homes` would silently default to backup mode
  and execute a complete Duplicacy backup before fixing permissions. Now
  `--fix-perms` is a standalone operation when no explicit mode (`--backup`,
  `--prune`, `--prune-deep`) is specified.
- **Remote mode error:** `--fix-perms --remote` now exits with a hard error
  instead of printing a warning and continuing. The error message reads:
  `"--fix-perms is only valid for local backups; cannot be used with --remote"`.
- **Redundant setup skipped:** When running `--fix-perms` standalone, BTRFS
  snapshot creation and Duplicacy working environment setup are skipped entirely,
  avoiding unnecessary operations and potential failures on systems without
  Duplicacy repositories.

### Added
- Clear logging for permission fix operations: operation mode header
  (`"Fix permissions only"`), target path display before execution, and elapsed
  time reporting (`"Permissions fixed in Xs"`).
- Combinable modes: `--fix-perms --backup` and `--fix-perms --prune` now work
  together, running the requested operation followed by permission fixing.
- New unit tests: `TestParseFlags_FixPermsAloneDoesNotDefaultToBackup`,
  `TestParseFlags_FixPermsWithBackupSetsBothFlags`,
  `TestParseFlags_FixPermsWithRemoteSetsFlags`,
  `TestParseFlags_NoFlagsDefaultsToBackup`.

### Changed
- **Behaviour change:** `--fix-perms` without an explicit mode no longer implies
  `--backup`. Users who relied on `--fix-perms homes` to perform both backup and
  permission fixing must now use `--fix-perms --backup homes`.
- Operation mode display now shows combined modes (e.g. `"Backup + fix
  permissions"`, `"Prune safe + fix permissions"`, `"Fix permissions only"`).

## [1.4.2] - 2026-04-08

### Fixed
- `LOCAL_OWNER` and `LOCAL_GROUP` are now restricted to the `[local]` section
  only. Previously they were also accepted in `[common]`, but since they only
  apply to local operations this was misleading. The config parser now rejects
  them in both `[common]` and `[remote]` sections with a clear error message.
- Remote mode (`--remote`) output now shows `<n/a>` for "Local Owner" and
  "Local Group" fields instead of empty/undefined values, providing clean and
  sensible output when these fields are not applicable.

### Changed
- Updated example configuration and README documentation to reflect that
  `LOCAL_OWNER`/`LOCAL_GROUP` must only appear in `[local]` section.

### Note
- **Breaking change for configs with LOCAL_OWNER/LOCAL_GROUP in [common]:**
  Move these keys to the `[local]` section. The parser will now reject them
  in `[common]` with a clear error message indicating the required section.

## [1.4.1] - 2026-04-08

### Changed
- `LOCAL_OWNER` and `LOCAL_GROUP` are now explicitly scoped to LOCAL operations
  only. They are validated and enforced only when running in local mode (default),
  and are skipped entirely in remote mode (`--remote`).
- The config parser now rejects `LOCAL_OWNER` and `LOCAL_GROUP` if they appear
  in the `[remote]` section, preventing configuration confusion. These keys are
  only permitted in `[common]` or `[local]` sections.
- Moved `LOCAL_OWNER`/`LOCAL_GROUP` from `[common]` to `[local]` in the example
  configuration file to better reflect their LOCAL-only scope.
- Updated README documentation to clarify that these fields are local-only and
  not relevant for remote backup targets.

### Note
- Patch release clarifying the scope of LOCAL_OWNER/LOCAL_GROUP. No breaking
  changes — existing configurations with these keys in [common] continue to work.

## [1.4.0] - 2026-04-08

### Added
- Validation to verify LOCAL_OWNER and LOCAL_GROUP exist on the system before
  use. The configuration validator now calls `user.Lookup` and `user.LookupGroup`
  to confirm the specified user and group are present, returning a clear error
  message if either is missing. This catches configuration typos and deployment
  issues at startup rather than at backup time.

### Note
- Minor version release adding new validation functionality. No breaking changes.

## [1.3.3] - 2026-04-08

### Fixed
- Fixed validation to properly reject 'root' as LOCAL_OWNER/LOCAL_GROUP as
  documented (security fix). The configuration validator now performs a
  case-insensitive check and returns a clear error when 'root' is specified,
  enforcing the documented non-root requirement for backup file ownership.

### Note
- Patch release addressing a P2 security validation bug where the code was not
  enforcing the documented requirement that LOCAL_OWNER and LOCAL_GROUP must not
  be set to 'root'. Running backups as root poses unnecessary privilege
  escalation risk on Synology NAS devices.

## [1.3.2] - 2026-04-08

### Changed
- Updated GitHub Actions workflow to remove deprecated elements:
  - `actions/checkout` v4 → v6
  - `actions/setup-go` v5 → v6
  - `actions/upload-artifact` v4 → v6
  - `actions/download-artifact` v4 → v6
  - `softprops/action-gh-release` v1 → v2
  - Added `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true` environment variable

### Note
- Patch release with no code changes. Updates CI/CD workflow action versions
  to their latest major releases, removing deprecated Node.js runtime warnings.

## [1.3.1] - 2026-04-08

### Changed
- CI/CD workflow enhancements now active (linting, SHA256 checksums).

### Note
- Patch release with no code changes. This release validates the enhanced CI/CD
  pipeline introduced in v1.3.0, including `go vet`/`gofmt` lint checks and
  SHA256SUMS.txt generation with published release binaries.

## [1.3.0] - 2026-04-08

### Added
- Overflow detection in `strictAtoi` for very large integer config values (CQ-3).
- SHA256 checksum file (`SHA256SUMS.txt`) generated and included with release binaries (CI-1).
- `go vet` and `gofmt` lint checks in CI pipeline (CI-2).
- New tests: `TestStrictAtoi_ValidValues`, `TestStrictAtoi_RejectsInvalid`,
  `TestStrictAtoi_OverflowDetection`, `TestRedactSecrets_RedactsCredentials`,
  `TestRedactSecrets_PreservesNonSecretFields`, `TestRedactSecrets_NullKeysUnchanged`,
  `TestWritePreferences_DryRun_RedactsSecrets`, `TestParseFlags_UnknownOption`.

### Changed
- Updated Go version from 1.19 to 1.24 in `go.mod`, CI workflow, and README (DEP-1).
- `doCleanup` now logs warnings on `os.RemoveAll` failures instead of silently
  ignoring them (ERR-2).

### Fixed
- `strictAtoi` integer overflow: parsing extremely large numeric strings no longer
  silently wraps around; an explicit overflow error is returned (CQ-3).
- Trailing blank line removed from `main.go` (CQ-4).
- Unreachable `--help` and `--version` dead code removed from `parseFlags`;
  these flags are already handled before `parseFlags` is called (CQ-5).
- Secrets (S3 ID and secret) are now redacted in dry-run log output, preventing
  credential leakage to terminal/log files (SEC-2).

### Security
- Dry-run mode no longer prints raw S3 credentials; values are replaced with
  `"REDACTED"` in log output (SEC-2).

## [1.2.0] - 2026-04-08

### Added
- MIT LICENSE file.
- `--version` flag to display version and build time.
- CHANGELOG.md for tracking releases.
- Mandatory validation for `LOCAL_OWNER` and `LOCAL_GROUP` configuration fields.
- New tests for mandatory owner/group validation (`TestValidateOwnerGroup_MissingOwnerReturnsError`,
  `TestValidateOwnerGroup_MissingGroupReturnsError`).

### Changed
- **BREAKING:** `LOCAL_OWNER` and `LOCAL_GROUP` are now **mandatory** configuration fields.
  They no longer have default values.  Every `.conf` file must explicitly set both fields
  to a non-root user/group for security.  The script will refuse to start if they are missing.
- Updated example configuration with clear comments explaining the mandatory fields.
- Updated README documentation to reflect the new requirements.

### Fixed
- Cleanup logic bug: `defer doCleanup(exitCode)` captured the exit code by value
  (always 0); replaced with `defer func() { doCleanup(exitCode) }()` so the
  actual exit status is reported.
- Version flag: declared `version` and `buildTime` variables in `main.go` so
  that `go build -ldflags "-X main.version=... -X main.buildTime=..."` works
  correctly.

### Removed
- `DefaultLocalOwner` and `DefaultLocalGroup` constants (replaced by mandatory validation).
