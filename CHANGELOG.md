# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
