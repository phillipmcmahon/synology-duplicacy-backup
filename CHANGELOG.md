# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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