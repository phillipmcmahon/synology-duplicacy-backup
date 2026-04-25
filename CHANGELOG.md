# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v7.1.0] - 2026-04-25

### Added
- **New replacement-NAS restore guide**:
  added `docs/new-nas-restore.md` for operators who need to install the tool
  on a new NAS, recreate the minimum config and secrets files, prove existing
  backup storage is reachable, and start restore inspection safely.

### Changed
- **Restore access no longer requires historical `source_path` knowledge**:
  restore-oriented flows can connect to existing Duplicacy storage without a
  configured live source path. `source_path` is now treated as backup-readiness
  and copy-back context, not a hard prerequisite for reading backup data during
  disaster recovery.
- **Default restore workspaces are independent of `source_path`**:
  when `--workspace` is omitted, restore execution derives the drill workspace
  from the restore job itself:
  `/volume1/restore-drills/<label>-<target>-<restore-point-timestamp>-rev<id>`.
  Explicit `--workspace` values are still honoured directly.
- **Restore and DR documentation was realigned**:
  README, CLI, configuration, restore-drill, operations, cheatsheet, and
  how-it-works documentation now distinguish backup-readiness validation from
  restore repository access.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`

### Coverage snapshot
- overall coverage: pending release prep
- `internal/workflow`: pending release prep

## [v7.0.0] - 2026-04-24

### Changed
- **Breaking: restore execution now self-prepares drill workspaces**:
  `restore run` creates or reuses the drill workspace itself and derives the
  default workspace from the selected restore point as
  `<label>-<target>-<restore-point-timestamp>-rev<id>`.
- **Breaking: restore discovery is now named for the operator task**:
  the public revision listing command is `restore list-revisions`, making the
  expert path read as `restore plan`, `restore list-revisions`, then
  `restore run`.
- **Guided restore selection is now the primary operator path**:
  `restore select` presents restore points first, offers inspect-only, full
  restore, or tree-based selective restore, previews the exact `restore run`
  primitives, and asks for confirmation before execution.
- **Restore internals are split by responsibility**:
  restore command dispatch, dependency seams, config context, workspace safety,
  picker prompting, Duplicacy list parsing, and report formatting now live in
  focused workflow files instead of one large restore command file.
- **Restore help is isolated from the general usage text**:
  restore-specific concise and full help moved into a dedicated help file with
  shared command-surface text to reduce future documentation drift.
- **Restore docs and help were realigned with the shipped command surface**:
  operator guidance now consistently describes `restore select`,
  `restore plan`, `restore list-revisions`, and `restore run`, including the
  deterministic drill-workspace naming model and picker controls.

### Removed
- **Removed the standalone `restore prepare` command**:
  restore preparation is now part of restore execution so operators have fewer
  command paths to reason about during recovery.
- **Removed the public `restore files` command**:
  file discovery is now part of the guided `restore select` flow. Expert and
  scripted recovery stay focused on listing revisions and executing explicit
  `restore run` primitives.
- **Removed the restore picker POC command**:
  the standalone proof-of-concept binary and its package script were deleted
  now that the picker is part of the production restore flow.
- **Removed the hidden `restore select --execute` option**:
  guided restore selection now always reviews generated commands and asks for
  confirmation before delegating to `restore run`.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`

### Coverage snapshot
- overall coverage: `83.8%`
- `cmd/duplicacy-backup`: `90.6%`
- `internal/workflow`: `82.8%`
- `internal/restorepicker`: `73.9%`
- `internal/update`: `82.6%`
- `internal/duplicacy`: `81.6%`
- `internal/exec`: `95.2%`
- `internal/secrets`: `92.0%`
- `internal/config`: `87.6%`

## [v6.2.1] - 2026-04-24

### Changed
- **Default restore-drill workspaces became timestamped**:
  restore workspace recommendations added timestamps under `restore-drills` so
  repeated restore drills did not silently reuse the same destination and
  operators could more easily tell when a drill workspace was created.
- **Follow-on restore execution reused the newest matching workspace**:
  restore execution could reuse the newest matching drill workspace for the
  same label and target before falling back to a fresh recommendation.

### Fixed
- **Restore picker controls are clearer about the continue action**:
  the picker now explains that `g` continues with the current selection and
  generates the restore commands, which makes the keybinding explicit in both
  the UI and the restore guidance.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`

### Coverage snapshot
- overall coverage: `81.8%`
- `cmd/duplicacy-backup`: `90.6%`
- `internal/workflow`: `83.6%`
- `internal/update`: `82.6%`
- `internal/duplicacy`: `81.6%`
- `internal/exec`: `95.2%`
- `internal/secrets`: `92.0%`
- `internal/config`: `87.6%`

## [v6.2.0] - 2026-04-24

### Added
- **Restore select now ships with an interactive tree picker**:
  operators can browse restore contents with the keyboard, expand and collapse
  directories with the arrow keys, toggle files or whole subtrees with the
  space bar, inspect the generated restore primitives in a dedicated detail
  pane, and continue or cancel without dropping into a bespoke command
  language.

### Changed
- **Restore selection keeps the existing explicit restore contract**:
  `restore select` now uses the tree picker as its interaction layer, but it
  still compiles back into explicit `restore run` commands and optional
  delegated execution rather than introducing hidden restore semantics.
- **Restore guidance now matches the shipped picker workflow**:
  concise help, full help, and operator documentation consistently describe the
  tree-based restore flow and no longer refer to the removed text-browser
  commands.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`

### Coverage snapshot
- overall coverage: `81.8%`
- `cmd/duplicacy-backup`: `90.6%`
- `internal/workflow`: `83.5%`
- `internal/update`: `82.6%`
- `internal/duplicacy`: `81.6%`
- `internal/exec`: `95.2%`
- `internal/secrets`: `92.0%`
- `internal/config`: `87.6%`

## [v6.1.1] - 2026-04-22

### Fixed
- **CLI help command surface**:
  concise and full help now list the current command groups consistently,
  including diagnostics, rollback, restore drills, notification tests, and
  managed install commands.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `86.0%`
  - `cmd/duplicacy-backup`: `90.6%`
  - `internal/workflow`: `84.9%`
  - `internal/update`: `82.6%`
  - `internal/duplicacy`: `81.2%`
  - `internal/exec`: `95.2%`
  - `internal/secrets`: `92.0%`
  - `internal/config`: `87.6%`

## [v6.1.0] - 2026-04-22

### Added
- **Redacted operator diagnostics command**:
  `duplicacy-backup diagnostics --target <target> <label>` gathers resolved
  config paths, storage scheme, secrets presence, state freshness, last run
  details, and basic path-permission context without running backup, prune,
  restore, or storage cleanup. `--json-summary` emits the same support-bundle
  context as machine-readable JSON.
- **Managed install rollback command**:
  `duplicacy-backup rollback --check-only` inspects retained managed-install
  versions, while `sudo duplicacy-backup rollback --yes` activates the newest
  previous retained binary by updating the managed `current` symlink. Operators
  can use `--version <tag>` to select a specific retained version.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `86.0%`
  - `cmd/duplicacy-backup`: `90.6%`
  - `internal/workflow`: `84.9%`
  - `internal/update`: `82.6%`
  - `internal/duplicacy`: `81.2%`
  - `internal/exec`: `95.2%`
  - `internal/secrets`: `92.0%`
  - `internal/config`: `87.6%`

## [v6.0.0] - 2026-04-21

### Added
- **Restore workspace preparation became available**:
  the wrapper gained a safe drill-workspace preparation step that wrote
  Duplicacy preferences and left revision selection, restore execution, and
  copy-back as explicit manual steps.

### Changed
- **Breaking: runtime operations are now first-class CLI commands**:
  `backup`, `prune`, `cleanup-storage`, and `fix-perms` replace the old
  top-level operation flags. For example, use
  `duplicacy-backup backup --target onsite-usb homes` instead of
  `duplicacy-backup --target onsite-usb --backup homes`. This is an
  intentional breaking command-line change to keep the invocation model
  consistent with `config`, `health`, `notify`, `restore`, and `update`.
  Existing Synology Task Scheduler entries that use the old operation flags
  must be updated before upgrading.
- **Forced prune now uses command-local syntax**:
  use `duplicacy-backup prune --target <target> --force <label>` instead of
  `--force-prune`.
- **Restore planning internals are clearer and safer to extend**:
  restore planning paths use an explicit config-only planner path, clearer
  state and workspace helpers, and shared report sections ahead of future
  restore subcommands.

### Removed
- **Old runtime operation flags are no longer supported**:
  `--backup`, `--prune`, `--cleanup-storage`, `--fix-perms`, and
  `--force-prune` are not accepted as the top-level runtime invocation model.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `86.2%`
  - `cmd/duplicacy-backup`: `92.6%`
  - `internal/workflow`: `84.8%`
  - `internal/update`: `83.5%`
  - `internal/duplicacy`: `81.2%`
  - `internal/exec`: `95.2%`
  - `internal/secrets`: `92.0%`
  - `internal/config`: `87.6%`

## [v5.1.2] - 2026-04-21

### Fixed
- **Update install now fails early when not run as root**:
  non-root update attempts that would install a release now stop before release
  download or attestation verification and explain that the command should be
  re-run with `sudo`; `update --check-only` remains available without root.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `86.1%`
  - `cmd/duplicacy-backup`: `92.6%`
  - `internal/workflow`: `84.5%`
  - `internal/update`: `83.5%`
  - `internal/duplicacy`: `81.2%`
  - `internal/exec`: `95.2%`
  - `internal/secrets`: `92.0%`
  - `internal/config`: `87.6%`

## [v5.1.1] - 2026-04-21

### Added
- **Restore planning is now available as a read-only command**:
  `duplicacy-backup restore plan --target <target> <label>` resolves the
  selected label and target, shows storage/config/secrets/state context, and
  prints safe Duplicacy drill commands without creating directories, running a
  restore, or copying data back to the live source path.

### Fixed
- **S3-compatible Duplicacy schemes now load storage credentials**:
  `s3c://`, `minio://`, and `minios://` targets require the same
  `s3_id` and `s3_secret` keys as `s3://`, so local object-storage targets
  such as MinIO or Garage no longer validate without credentials.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `86.1%`
  - `cmd/duplicacy-backup`: `92.4%`
  - `internal/workflow`: `84.5%`
  - `internal/update`: `83.5%`
  - `internal/duplicacy`: `81.2%`
  - `internal/exec`: `95.2%`
  - `internal/secrets`: `92.0%`
  - `internal/config`: `87.6%`

## [v5.1.0] - 2026-04-21

### Changed
- **Duplicacy storage semantics now live in the Duplicacy integration layer**:
  backend scheme detection, local-path handling, storage-secret requirements,
  and config validation now use a focused `StorageSpec` type instead of
  workflow-owned switches.
- **Runtime plans now expose focused section views**: request, config, path,
  and display data can be inspected as distinct groups, and planner config
  application now flows through focused helper methods rather than a long
  assignment block.
- **Source-label command parsing is now shared**: runtime, config, health, and
  notify parsing all use one common source/flag loop with command-specific
  extras.
- **Notification event IDs are now centralised**: supported runtime, health,
  test, and update event IDs live in `internal/notify` and request validation
  uses that single list.
- **Notification providers now use a small provider abstraction**: destination
  discovery and delivery dispatch are grouped by provider, making future
  Discord, Slack, Node-RED, or Apprise integrations easier to add.
- **Concise CLI help uses template replacement for the script name** to avoid
  brittle positional formatting as examples grow.
- **State persistence now uses one mutate-and-save helper** for runtime and
  health recency updates, reducing drift between state write paths.

### Removed
- **Redundant `storage_type` output has been removed**: runtime JSON summaries,
  health reports, and notification payloads now report target identity through
  `target`, `location`, and configured `storage` rather than repeating the
  retired type model.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `85.9%`
  - `cmd/duplicacy-backup`: `92.7%`
  - `internal/workflow`: `84.3%`
  - `internal/update`: `83.5%`
  - `internal/duplicacy`: `81.2%`
  - `internal/exec`: `95.2%`
  - `internal/secrets`: `92.0%`
  - `internal/config`: `87.6%`

## [v5.0.0] - 2026-04-20

### Added
- **Dependabot now monitors routine dependency and toolchain updates**:
  weekly grouped PRs cover Go modules, GitHub Actions, and the release
  validation Go container image.

### Changed
- **Staticcheck is now module-pinned**: CI and release validation run
  `honnef.co/go/tools/cmd/staticcheck` from Go module metadata so Dependabot
  can update the Staticcheck toolchain through normal Go dependency PRs.
- **Duplicacy backend configuration is now provider-neutral**: backup targets
  use a direct `storage` value, and target secrets now use generic
  `[targets.<name>.keys]` entries that are passed through to Duplicacy
  preferences.
- **Config parsing and operator guidance now use current schema terms**:
  single-target fallback detection is named explicitly, target health override
  resolution is simpler, and missing required value errors point to
  `common.*` / `targets.<name>.*` keys.

### Removed
- **The Storj/S3-specific object target schema has been removed**:
  `type = "object"` plus `storj_s3_id` / `storj_s3_secret` is replaced by the
  standardised `storage` plus generic storage keys model.
- **The target `type` key has been retired**: every target delegates storage
  handling to Duplicacy, so operator config now uses only `location` and
  `storage` for target identity.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `85.7%`
  - `cmd/duplicacy-backup`: `92.7%`
  - `internal/workflow`: `84.1%`
  - `internal/update`: `83.5%`
  - `internal/duplicacy`: `81.1%`
  - `internal/exec`: `95.2%`
  - `internal/secrets`: `92.0%`
  - `internal/config`: `87.3%`

## [v4.5.0] - 2026-04-18

### Added
- **Local object-storage targets are now supported**: targets can use
  `type = "object"` with `location = "local"` for local S3-compatible object
  stores such as RustFS or MinIO while still preserving object-storage URL and
  credential semantics.

### Changed
- **Target documentation now separates storage mechanics from operational
  location more clearly**: README, configuration, CLI, cheat sheet,
  operations, scheduling, and how-it-works guidance all list `object/local` as
  a supported combination and explain that `type`, not `location`, controls
  storage credential loading.
- **Object-storage summaries use neutral credential labels**: verbose runtime
  and config output now refer to storage access and secret keys rather than
  remote-specific key names, which keeps local object-storage reporting
  accurate.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `85.8%`
  - `cmd/duplicacy-backup`: `92.7%`
  - `internal/workflow`: `83.8%`
  - `internal/update`: `83.5%`
  - `internal/exec`: `95.2%`
  - `internal/secrets`: `93.3%`
  - `internal/config`: `88.1%`

## [v4.4.1] - 2026-04-18

### Added
- **A lightweight operator troubleshooting guide is now available**:
  `docs/troubleshooting.md` gives operators a direct symptom-to-next-action
  entry point for scheduled task failures, repository readiness surprises,
  health verify failures, notification mismatches, attestation failures,
  privilege-limited validation, logger permission errors, and update layout
  issues.

### Changed
- **Documentation discovery is clearer**: the README task map and documentation
  list now include the troubleshooting guide, and the broader documentation set
  has been tightened around flow, readability, ownership, and source-of-truth
  boundaries.
- **Concise help now surfaces defaults more consistently**: update help now
  calls out defaults such as `--keep 2`, latest-version selection, and
  attestation mode without requiring `--help-full`.
- **Release process checks are harder to miss**: release guidance now treats
  NAS mirroring and full verification as standard closure gates, and the
  finalization helper captures the release URL, mirror path, verification
  result, and attestation result for the release issue.
- **Update attestation reporting uses structured result state internally**:
  attestation outcomes remain human-readable in reports while being represented
  by stable status values for future machine-readable output.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `85.8%`
  - `cmd/duplicacy-backup`: `92.7%`
  - `internal/workflow`: `83.8%`
  - `internal/update`: `83.5%`
  - `internal/exec`: `95.2%`
  - `internal/secrets`: `93.3%`
  - `internal/config`: `88.1%`

## [v4.4.0] - 2026-04-17

### Added
- **Self-update can verify release-asset attestations before install**:
  `duplicacy-backup update --attestations required` now fails before
  extraction or install unless GitHub CLI can verify the downloaded tarball
  against the release attestation. `--attestations auto` verifies when `gh` is
  available and otherwise continues with checksum-only verification, while the
  default `off` mode preserves existing scheduled NAS update jobs.
- **A focused update trust-model guide is now available**:
  `docs/update-trust-model.md` explains what the updater verifies, which
  authorities it still trusts, and when operators should perform an
  out-of-band review.

### Changed
- **Update failure notifications now classify attestation failures explicitly**:
  update notification events can distinguish attestation verification failures
  from checksum, download, and install failures.
- **CI now runs Staticcheck**: the lint workflow and release validation guidance
  pin `honnef.co/go/tools/cmd/staticcheck@v0.7.0` so static analysis findings
  are caught before release.
- **Staticcheck findings are resolved rather than suppressed**: unused helpers
  were removed, deprecated title-casing calls were replaced, error strings were
  normalized, and one unused test assignment was cleaned up.
- **Command logging redaction is safer for future wrappers**: env-style
  `KEY=value` arguments now have explicit coverage for token, secret access
  key, and webhook URL values.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**:
  `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **GitHub Actions**: Build and Release workflow passed for commits `0aa68d0`,
  `37ee1a1`, and `8480912`.
- **Coverage snapshot**:
  - overall coverage: `85.7%`
  - `cmd/duplicacy-backup`: `92.7%`
  - `internal/workflow`: `83.8%`
  - `internal/update`: `83.3%`
  - `internal/exec`: `95.2%`
  - `internal/secrets`: `93.3%`
  - `internal/config`: `88.1%`

## [v4.3.6] - 2026-04-17

### Changed
- **GitHub releases now use immutable release attestations**: the tag-triggered
  release workflow publishes via GitHub CLI so release assets are created before
  the immutable release is published and can be verified with GitHub release
  attestation commands.
- **Release verification now checks the release attestation**:
  `scripts/verify-release.sh` verifies the GitHub release itself before
  verifying each downloaded asset against that release.
- **Release guidance now matches the attestation model**: the release playbook
  and operations guide describe immutable releases, release-level verification,
  and asset-level verification with GitHub CLI.
- **Release notes are preserved before immutable publication**: the release
  playbook now tags with `--cleanup=verbatim`, and the workflow validates that
  annotated tag notes still contain `Highlights`, `Validation`, and `Coverage`
  before publishing the immutable release.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Release workflow validation**: workflow YAML parsed successfully; local
  `actionlint` was unavailable.
- **Coverage snapshot**:
  - overall coverage: `85.4%`
  - `cmd/duplicacy-backup`: `92.7%`
  - `internal/workflow`: `83.3%`
  - `internal/update`: `83.5%`
  - `internal/exec`: `93.7%`
  - `internal/secrets`: `93.3%`

## [v4.3.5] - 2026-04-17

### Superseded
- The v4.3.5 tag was pushed, but the GitHub release workflow failed before a
  release was published because tag cleanup stripped Markdown release-note
  headings. Its intended release notes are folded into v4.3.6.

## [v4.3.4] - 2026-04-17

### Superseded
- The v4.3.4 tag was pushed, but the GitHub release workflow failed before a
  release was published. Its intended release notes are folded into v4.3.6.

## [v4.3.3] - 2026-04-17

### Superseded
- The v4.3.3 tag was pushed, but the GitHub release workflow failed before a
  release was published. Its intended release notes are folded into v4.3.6.

## [v4.3.2] - 2026-04-16

### Changed
- **Update internals are split into clearer units**: release lookup, package
  verification/extraction, install execution, and report rendering now live in
  focused update package files. This keeps self-update behaviour easier to
  review as the feature grows.
- **Update network calls now have explicit timeouts**: GitHub release metadata
  lookups and release asset downloads use bounded request contexts and return
  phase-specific timeout errors for operators.
- **Update notification classification now uses structured status values**:
  update success and failure notifications no longer depend on parsing
  human-readable report text, reducing the chance that wording-only changes
  alter notification behaviour.
- **Update status is now adapted at the command boundary**: `internal/update`
  owns its domain status values, while `cmd/duplicacy-backup` maps them to
  workflow notification statuses.
- **Verify health reconciliation is separated from orchestration**: integrity
  result matching now has a dedicated helper path, making edge cases around
  missing or mismatched verify results easier to test and reason about.
- **Documentation now reflects the current command dispatch path**: the
  architecture notes describe how parsed CLI requests are routed through
  dispatch, presentation, and update modules.
- **Local test-package outputs are standardized**: operator test packages are
  expected under `build/test-packages`, avoiding ad-hoc build artefact
  locations.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `81.0%`
  - `cmd/duplicacy-backup`: `91.9%`
  - `internal/workflow`: `83.1%`
  - `internal/update`: `82.3%`
  - `internal/exec`: `97.4%`
  - `internal/secrets`: `90.9%`

## [v4.3.1] - 2026-04-16

### Fixed
- **Self-update installer no longer inherits an unsafe caller directory**:
  `duplicacy-backup update` now launches the packaged installer from the
  extracted package directory, preventing repeated Synology shell `getcwd`
  warnings when the operator's current directory has disappeared or become
  inaccessible.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Updater missing-working-directory regression**: unit coverage now deletes
  the caller working directory before launching the installer and asserts the
  installer runs from the extracted package directory without `getcwd` noise.
- **Coverage snapshot**:
  - overall coverage: `79.8%`
  - `cmd/duplicacy-backup`: `91.4%`
  - `internal/workflow`: `82.7%`
  - `internal/update`: `69.8%`

## [v4.3.0] - 2026-04-16

### Added
- **Global self-update notifications**: `update` can now send failure
  notifications through a global `[update.notify]` config, separate from
  label/target backup settings and without reading storage secrets.
- **Update notification testing**: `notify test update` sends a simulated
  update notification through the global app notification config without
  running a real update.

### Changed
- **Update notification config stays global by design**: update alerts use
  `<config-dir>/duplicacy-backup.toml` and do not require a backup label,
  target, or object-storage secrets unless a future authenticated notification
  provider explicitly needs its own token source.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **NAS update-notification smoke**: an amd64 test package successfully sent
  `notify test update` through the real ntfy topic on `homestorage`.
- **Coverage snapshot**:
  - overall coverage: `79.7%`
  - `cmd/duplicacy-backup`: `91.4%`
  - `internal/workflow`: `82.7%`
  - `internal/config`: `83.8%`

## [v4.2.2] - 2026-04-16

### Fixed
- **Forced self-update can now reinstall the active version**: the packaged
  installer stages the replacement binary inside the install root and then
  renames it into place, avoiding Linux `Text file busy` failures when
  `update --force` reinstalls the currently running version.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Linux running-binary installer regression**: the installer replaced a
  versioned target path while that target was executing, proving the staged
  rename path avoids `Text file busy`.
- **Coverage snapshot**:
  - overall coverage: `79.9%`
  - `cmd/duplicacy-backup`: `90.5%`
  - `internal/workflow`: `83.2%`
  - `internal/update`: `68.7%`

## [v4.2.1] - 2026-04-16

### Fixed
- **Updater command resolution now works through `PATH`**: `duplicacy-backup
  update` now resolves the invoked stable command through `PATH` before
  validating the managed install layout, so normal shell usage no longer tries
  to resolve a missing binary from the current working directory.
- **Updater can reinstall the selected release on demand**: `update --force`
  now performs the install path even when the selected release is already
  current, giving operators a repair path for managed installs without waiting
  for a newer release.
- **Update examples now prefer the stable absolute path**: operator examples use
  `/usr/local/bin/duplicacy-backup` for update commands, which is clearer and
  safer for unattended Synology Task Scheduler jobs.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `79.9%`
  - `cmd/duplicacy-backup`: `90.5%`
  - `internal/workflow`: `83.2%`
  - `internal/update`: `68.7%`

## [v4.2.0] - 2026-04-16

### Added
- **Managed self-update command**: `duplicacy-backup update` can now check the
  latest published GitHub release, select the matching Linux asset, verify the
  per-asset checksum, extract the package, and hand installation over to the
  packaged `install.sh`.
- **Safe update controls for operators**: `update --check-only` reports the
  planned update without downloading or installing, `update --yes` enables
  non-interactive installs, `--version <tag>` pins a specific release, and
  `--keep <count>` controls installed-binary retention with a default of
  `--keep 2`.

### Changed
- **Updater uses the existing managed install model**: self-update only runs
  from the supported stable command layout and reuses the installer for
  symlink switching, config-directory handling, and retention pruning instead
  of duplicating install logic inside Go.
- **Operator documentation now includes the update path**: the README, CLI
  reference, and operations guide now document `update --check-only`,
  `update --yes`, the managed-layout expectation, and the default retention
  policy.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **NAS managed-layout smoke**: `update --check-only` and `update --yes`
  upgraded a temporary `homestorage` install from `4.1.7` to `4.1.8`.
- **Coverage snapshot**:
  - overall coverage: `79.7%`
  - `cmd/duplicacy-backup`: `90.5%`
  - `internal/workflow`: `83.2%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `84.4%`
  - `internal/secrets`: `90.9%`

## [v4.1.8] - 2026-04-15

### Fixed
- **Config validation now accepts `--verbose` consistently**: `config validate`
  no longer rejects `--verbose` as an unknown option, so operators can use the
  same verbosity flag on config validation that they already use elsewhere in
  the CLI.
- **Non-root runtime and health failures now report the real privilege
  requirement first**: runtime operations such as prune, cleanup, and
  fix-perms, plus health commands such as `health status`, now surface
  `Must be run as root` or `Health commands must be run as root` before any
  `/var/log` logger-init failure can obscure the real problem.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `79.8%`
  - `cmd/duplicacy-backup`: `90.7%`
  - `internal/workflow`: `82.1%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `84.4%`
  - `internal/secrets`: `81.8%`

## [v4.1.7] - 2026-04-15

### Changed
- **The command and orchestration surface is now split into clearer packages**:
  CLI request parsing now lives in `internal/command`, notification delivery
  now lives in `internal/notify`, presentation formatting now lives in
  `internal/presentation`, and the health reporting model now lives in
  `internal/health`, which reduces drift risk in both `main.go` and the
  remaining workflow orchestration layer.
- **The health path is decomposed further without changing behaviour**:
  health bootstrap, checks, state handling, and notification gating are now
  split into smaller workflow units so the orchestration path is easier to
  navigate and extend.
- **The maintainer docs now reflect the real ownership boundaries**:
  `docs/how-it-works.md` and `docs/architecture.md` now point change-oriented
  guidance at the extracted packages and explicitly document the rule that new
  feature logic should land in focused domain packages before `workflow`.

### Fixed
- **Newly extracted package boundaries now have stronger direct test coverage**:
  `internal/health` and `internal/presentation` now have focused package-level
  tests so the refactor is protected by behaviour checks at the new boundary,
  not only through higher-level workflow tests.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `79.9%`
  - `cmd/duplicacy-backup`: `93.7%`
  - `internal/workflow`: `82.1%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `84.4%`
  - `internal/secrets`: `81.8%`

## [v4.1.6] - 2026-04-14

### Changed
- **CLI coverage is now much stronger at the real entrypoint**: the top-level
  `cmd/duplicacy-backup` tests now cover `notify test` end-to-end behaviour,
  plus logger-initialisation failure handling for both health and runtime
  command paths.

### Fixed
- **Optional notification-token parsing now has less duplication and better
  behavioural coverage**: the shared `internal/secrets` token-loading path now
  exercises meaningful edge cases around missing targets, missing tokens,
  uppercase keys, malformed TOML, and unknown keys while keeping the code
  simpler.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `83.9%`
  - `cmd/duplicacy-backup`: `94.7%`
  - `internal/workflow`: `82.8%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `84.4%`
  - `internal/secrets`: `81.8%`

## [v4.1.5] - 2026-04-14

### Added
- **Release asset mirroring is now a first-class scripted workflow**:
  `scripts/mirror-release-assets.sh` now downloads the published GitHub assets
  and source archives for a tagged release, then mirrors the full artefact set
  to `homestorage` with a `tar`-over-SSH transfer that avoids the Synology
  `scp` edge cases we hit in real use.
- **Post-release verification is now scripted end to end**:
  `scripts/verify-release.sh` now verifies the published GitHub release object,
  required release-note headings, expected packaged asset names, tag commit
  alignment, and the mirrored artefact set on `homestorage`.

### Changed
- **The repo now reflects live release state more cleanly after publish**:
  the testing guide now updates the current public release baseline after a
  release is published instead of continuing to read as though the same version
  is still only an active prep target.
- **Release tracking conventions are now documented and templated**: the
  release playbook now records the lightweight issue/board workflow we’ve been
  using, and the repo now includes a dedicated `Release Prep` issue template.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `83.1%`
  - `cmd/duplicacy-backup`: `74.5%`
  - `internal/workflow`: `82.9%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `84.4%`
  - `internal/secrets`: `76.2%`

## [v4.1.4] - 2026-04-14

### Added
- **Simulated notification sends are now a first-class operator workflow**:
  `duplicacy-backup notify test --target <name> <label>` now sends a clearly
  marked synthetic notification through the configured providers for the
  selected target, with support for `--provider`, `--severity`, `--summary`,
  `--message`, `--dry-run`, and `--json-summary`.

### Changed
- **Notification test guidance now explains what the command proves**: the
  README, CLI reference, configuration guide, and cheat sheet now explain that
  simulated sends validate provider delivery and auth for the selected target,
  while clearly separating that from real backup, prune, health, and DSM email
  behaviour.

### Fixed
- **Notification provider tests are now hermetic in CI**: the send-all notify
  test now uses isolated config and secrets paths, so GitHub Actions no longer
  depends on any host-local secrets state when validating the notification
  command surface.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `83.1%`
  - `cmd/duplicacy-backup`: `74.5%`
  - `internal/workflow`: `82.9%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `84.4%`
  - `internal/secrets`: `76.2%`

## [v4.1.3] - 2026-04-14

### Changed
- **Documentation now reads more naturally across the operator guides**: the
  README, configuration guide, operations guide, CLI reference, workflow guide,
  cheat sheet, testing guide, and supporting internal docs were tightened so
  guidance is clearer and less mechanical without changing the underlying
  behaviour.

### Fixed
- **Config-command parse failures no longer try to open `/var/log` first**:
  request and config-command parse errors such as missing `--target` now report
  the real CLI problem directly instead of surfacing a misleading
  `Failed to initialise logger` error on non-root runs.
- **CLI regression coverage now protects non-logger parse paths**:
  top-level tests now cover both generic parse failures and `config validate`
  parse failures when logger initialisation is unavailable, so this behaviour
  remains stable in CI.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `84.4%`
  - `cmd/duplicacy-backup`: `85.0%`
  - `internal/workflow`: `84.0%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `84.4%`
  - `internal/secrets`: `76.2%`

## [v4.1.2] - 2026-04-13

### Added
- **Near-time notification delivery is now a first-class operator feature**:
  `duplicacy-backup` now emits a shared notification event model for selected
  runtime failures and health outcomes, supports native `ntfy` delivery for the
  recommended low-cost Synology path, and keeps generic webhook delivery
  available for future providers and external automation.
- **Workflow and scheduling guidance is now a first-class part of the product**:
  the new workflow guide documents the supported Synology Task Scheduler model,
  including task naming, workload separation, timing rules, a worked
  multi-label example, and a quick-reference task table for day-to-day setup.

### Changed
- **`config validate` now makes its privilege level explicit**: validation
  output now shows `Privileges : Full` or `Privileges : Limited`, so operators
  can immediately see whether root-only checks may have been skipped.
- **Operator docs now distinguish manual command combinations from recommended
  recurring schedules**: the README, CLI reference, operations guide, and desk
  cheat sheet now point recurring Synology scheduler usage at the dedicated
  workflow guidance instead of letting combined manual examples read like
  schedule templates.

### Fixed
- **Release validation is now stable for the notification test path**: the
  pre-run notification CLI test now synchronizes its HTTP capture correctly,
  so Linux CI can validate the shipped notification surface reliably.
- **Top-level CLI notification tests no longer depend on loopback delivery**:
  the `cmd` package now stubs pre-run notification dispatch directly, leaving
  payload construction and transport coverage to lower-level workflow tests and
  making CI release validation deterministic.

### Validation
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `84.4%`
  - `cmd/duplicacy-backup`: `85.7%`
  - `internal/workflow`: `84.0%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `84.4%`
  - `internal/secrets`: `76.2%`

## [v4.0.1] - 2026-04-13

### Changed
- **The Synology installer now establishes the intended permission layout**:
  when run as `root`, `install.sh` now normalises the stable config directory
  to trusted-operator access, ensures `/root/.secrets` exists with root-only
  directory permissions, and exposes a new `--config-group` option for sites
  that use a different operator group.
- **Install and operator docs now describe the permission model explicitly**:
  the README, operations guide, configuration guide, and desk cheat sheet now
  explain the expected config and secrets ownership/mode policy and clarify
  that the installer never rewrites individual secrets files.

### Fixed
- **Non-root `config validate` is now more truthful on protected systems**:
  permission-denied config reads now report an access problem instead of
  claiming the file is missing, and root-only checks such as Btrfs validation,
  object-target secrets validation, and repository probes now report
  `Not checked` instead of surfacing misleading failures.
- **Installer regression coverage now matches real release environments**:
  install-script tests now accept both non-root and root execution paths, so
  Linux Go 1.26 release validation exercises the installer output correctly.

### Validation
- **Local**: `go test ./...`
- **Local**: `go vet ./...`
- **Linux Go 1.26**: `go test ./...`
- **Linux Go 1.26**: `go vet ./...`
- **Linux Go 1.26**: `go test -cover ./...`
- **Coverage snapshot**:
  - overall coverage: `85.1%`
  - `cmd/duplicacy-backup`: `86.2%`
  - `internal/workflow`: `84.8%`
  - `internal/duplicacy`: `81.2%`
  - `internal/config`: `84.3%`
  - `internal/secrets`: `81.0%`

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
