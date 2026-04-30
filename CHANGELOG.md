# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Coverage snapshots list aggregate statement coverage plus selected
release-signal packages or packages with notable movement. They are not
exhaustive package coverage tables; use `TESTING.md` for the current full
coverage floor and package-level baseline.

## [Unreleased]

### Added
- Local validation now enforces the `85.0%` coverage floor with
  `scripts/check-coverage-floor.sh`, checking both every coverable package and
  aggregate coverage before changes are pushed or released.
- Direct `internal/workflowcore` tests now cover the neutral metadata,
  environment, request, plan, and run-state primitives added during the
  architecture cleanup.

### Changed
- Release-prep notes now include an explicit operator-impact section, further
  reading placeholders, and coverage lines for `internal/workflowcore`,
  `internal/restore`, `internal/health`, and `internal/operator` so new
  architecture packages remain visible in release evidence.
- The current testing baseline is refreshed to show all coverable packages at
  or above the `85.0%` floor, with aggregate coverage at `87.6%`.
- The release playbook now requires commentary for material coverage movement,
  explicit CLI/config/operator-impact wording, and links to deeper docs when a
  release highlights architecture or process changes.

### Fixed
- Release-prep package coverage extraction now also handles packages reported
  by `go test -cover` without the leading `ok` status field, such as packages
  with no direct test files.
- Release verification now has fixture coverage for the new operator-impact
  section gate so historical releases remain verifiable while future releases
  enforce the strengthened note contract.

## [v10.0.3] - 2026-04-30

### Changed
- Command parsing now records a single canonical command discriminator on the
  request envelope, and dispatch/policy lookup use that discriminator instead
  of independently probing command-family fields.
- Neutral workflow primitives now live in `internal/workflowcore`, giving
  extracted subsystems a shared request, environment, metadata, plan, and
  read-only state home without importing those data types from the workflow
  orchestrator.
- Health command orchestration now lives with health reports and presentation in
  `internal/health`, leaving `internal/workflow` as the shared planning/state
  bridge rather than the owner of health-specific behaviour.
- Command parsing/help files are now split by command family, restore command
  tests are split by plan/run/select responsibilities, and the architecture docs
  now reflect the extracted restore/health subsystem boundaries.
- Command parsing and dispatch now use neutral `workflowcore.Request`
  primitives directly where possible, and registry tests assert parsers set the
  canonical command discriminator during the transition to typed commands.
- Parsers now return typed command values from the command registry, allowing
  entrypoint dispatch to route by registry family and removing the separate
  dispatch registry table.
- Operator-facing error translation now lives in `internal/operator`, with
  workflow keeping only compatibility aliases while runtime files are narrowed.
- Runtime rename readiness now documents why `internal/workflow` remains an
  orchestration package for now, and command/health code uses
  `internal/operator` directly for operator-facing messages.
- Operator and maintainer documentation now reflects the latest package
  ownership boundaries, restore/operator message handling, and colour-capture
  environment variable surface.
- Release-prep coverage extraction now matches package paths exactly, so
  `internal/workflow` release notes are not confused with
  `internal/workflowcore`.
- The root changelog now carries only the active major release line plus
  `Unreleased`; older major-version history lives under `docs/changelog/` so
  release notes stay focused while historical entries remain offline-greppable.

### Fixed
- Empty or malformed internal requests no longer fall through to the runtime
  backup/prune dispatcher; missing dispatch coverage now fails explicitly.

### Validation
- **Local pre-push**: `make validate`
- **Local UI smoke packaging**: `make validate-full`
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Project board audit**: `scripts/project-board-audit.sh`

### Coverage snapshot
- overall coverage: `84.7%`
- `cmd/duplicacy-backup`: `84.7%`
- `internal/workflow`: `84.9%`
- `internal/duplicacy`: `89.7%`
- `internal/exec`: `95.2%`
- `internal/secrets`: `93.1%`

## [v10.0.2] - 2026-04-29

### Changed
- NAS UI smoke bundles now use a stable root-owned smoke binary path for
  sudo-required captures and automatically refresh that binary from the trusted
  extracted bundle layout before the run starts.
- The release playbook now documents the trust boundary for the smoke install
  path so release operators keep the sudoers glob narrow and treat unexpected
  bundle paths as release-process failures.

### Fixed
- Report-style output now has regression coverage that keeps neutral values
  such as `Not required` and `Custom` uncoloured while preserving semantic
  success, warning, and failure colour checks.

### Validation
- **Local pre-push**: `make validate`
- **Local UI smoke packaging**: `make validate-full`
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **NAS UI surface smoke**: automatic smoke binary refresh verified with
  passwordless sudo, colour capture enabled, and dual-target restore smoke with
  `onsite-garage` plus root-protected `onsite-usb`.
- **Project board audit**: `scripts/project-board-audit.sh`

### Coverage snapshot
- overall coverage: `87.4%`
- `cmd/duplicacy-backup`: `86.0%`
- `internal/workflow`: `85.5%`
- `internal/duplicacy`: `89.7%`
- `internal/exec`: `95.2%`
- `internal/secrets`: `93.1%`

## [v10.0.1] - 2026-04-29

### Changed
- UI surface smoke captures now enable colour by default, so semantic colour
  assertions are active during the standard NAS release smoke path unless an
  operator deliberately sets `CAPTURE_COLOUR=0`.
- The UI smoke package and CI proxy now assert that packaged smoke runners keep
  colour capture enabled by default, preventing future release tests from
  silently skipping colour checks.
- Release-surface documentation checks now tolerate normal Markdown line
  wrapping for storage-key guidance instead of depending on adjacent substring
  matches.

### Fixed
- UI smoke colour assertions now ignore structural section headers such as
  `Section: Resolved`, avoiding false positives while continuing to verify
  semantic validation values.
- `config validate` now colourizes `Source Path Access : Present` as a
  successful green validation value.

### Validation
- **Local pre-push**: `make validate`
- **Local UI smoke packaging**: `make validate-full`
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **NAS UI surface smoke**: `109` captures, `0` unexpected outcomes, colour
  capture enabled, and dual-target restore smoke with `onsite-garage` plus
  root-protected `onsite-usb` restoring `phillipmcmahon/code/*`.
- **Project board audit**: `scripts/project-board-audit.sh`

### Coverage snapshot
- overall coverage: `87.4%`
- `cmd/duplicacy-backup`: `86.0%`
- `internal/workflow`: `85.5%`
- `internal/duplicacy`: `89.7%`
- `internal/exec`: `95.2%`
- `internal/secrets`: `93.1%`

## [v10.0.0] - 2026-04-29

### Changed
- Renamed the deterministic test metadata helper to `MetadataForLogDir`, making
  the production `DefaultMetadataForRuntime` path the only default-named
  metadata constructor.
- Restore selection now calls the canonical Duplicacy list-file parser directly
  and handles parser errors explicitly, removing the old compact helper that
  discarded parser errors.
- Health and config tests now use current `storage` terminology and fixtures
  directly instead of translating legacy-shaped `destination`/`repository`
  snippets.
- Operator and release documentation now avoids retired migration-helper and
  old-schema wording in active guidance, and the cheat sheet aligns local prune
  previews with the root-protected repository policy.
- Added a docs index that gives operators a task-oriented path through install,
  configuration, scheduling, restore, update, troubleshooting, and maintainer
  guidance.
- Trimmed duplicated notification and health JSON detail from the operations
  guide so configuration policy remains the source of truth.
- Slimmed the architecture overview to avoid duplicating the detailed
  how-it-works walkthrough, and split site-specific release mirror details out
  of the release playbook.

### Removed
- Removed retired config-schema parsing for the old `[local]`, `[remote]`,
  `[target]`, `[storage]`, `[capture]`, and `[retention]` layouts. Config files
  now use only the current `[targets.<name>]` model.
- Removed the v8 runtime-profile migration helper, migration smoke job,
  release-package migration asset, and operator migration guide.

### Validation
- **Local pre-push**: `make validate`
- **Local UI smoke packaging**: `make validate-full`
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **NAS UI surface smoke**: `109` captures, `0` unexpected outcomes, and
  dual-target restore smoke with `onsite-garage` plus root-protected
  `onsite-usb` restoring `phillipmcmahon/code/*`.
- **Project board audit**: `scripts/project-board-audit.sh`

### Coverage snapshot
- overall coverage: `87.4%`
- `cmd/duplicacy-backup`: `86.0%`
- `internal/workflow`: `85.5%`
- `internal/duplicacy`: `89.7%`
- `internal/exec`: `95.2%`
- `internal/secrets`: `93.1%`

## Historical Changelogs

Older major-version changelogs are kept offline-greppable under `docs/changelog/`:

- [v9](docs/changelog/v9.md)
- [v8](docs/changelog/v8.md)
- [v7](docs/changelog/v7.md)
- [v6](docs/changelog/v6.md)
- [v5](docs/changelog/v5.md)
- [v4](docs/changelog/v4.md)
- [v3](docs/changelog/v3.md)
- [v2](docs/changelog/v2.md)
- [v1](docs/changelog/v1.md)
