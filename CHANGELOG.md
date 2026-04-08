# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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