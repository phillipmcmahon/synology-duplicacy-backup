# Release Preparation Summary — synology-duplicacy-backup v1.2.0

## Overview

All changes have been implemented, tested, and are ready for review on the
`release/v1.2.0-prep` branch.

---

## Changes Made

### 1. LOCAL_OWNER and LOCAL_GROUP Made Mandatory (Priority 1)

| File | Change |
|---|---|
| `internal/config/config.go` | Removed `DefaultLocalOwner` and `DefaultLocalGroup` constants |
| `internal/config/config.go` | `NewDefaults()` no longer sets LocalOwner/LocalGroup |
| `internal/config/config.go` | `ValidateOwnerGroup()` now returns a clear error when either field is empty |
| `internal/config/config_test.go` | Updated `TestNewDefaults`, `TestApply_EmptyMapKeepsDefaults`, `TestValidateOwnerGroup_EmptyOwner` |
| `internal/config/config_test.go` | Added `TestValidateOwnerGroup_MissingOwnerReturnsError`, `TestValidateOwnerGroup_MissingGroupReturnsError` |
| `examples/homes-backup.conf` | Added comments explaining these fields are REQUIRED |
| `README.md` | Updated config keys table: LOCAL_OWNER and LOCAL_GROUP marked **Yes** (required) |

### 2. MIT LICENSE File Added

| File | Change |
|---|---|
| `LICENSE` | Created with standard MIT license text, copyright holder: Phillip McMahon |

### 3. Version Flag Fixed

| File | Change |
|---|---|
| `cmd/duplicacy-backup/main.go` | Declared `var version` and `var buildTime` with defaults `"dev"` / `"unknown"` |
| `cmd/duplicacy-backup/main.go` | Added `--version` flag handling in early arg scan and in `parseFlags` |
| `cmd/duplicacy-backup/main.go` | Added `--version` to help text |

The Makefile already injects these via `-X main.version=... -X main.buildTime=...`.

### 4. Cleanup Logic Bug Fixed

| File | Change |
|---|---|
| `cmd/duplicacy-backup/main.go` | Changed `defer doCleanup(exitCode)` → `defer func() { doCleanup(exitCode) }()` |

**Root cause:** Go evaluates deferred function arguments at the time of the `defer` statement.
Since `exitCode` was 0 when the defer was registered, the cleanup always reported SUCCESS
regardless of the actual outcome.  The closure captures `exitCode` by reference.

### 5. Tests

All tests pass with race detection:

```bash
go test -v -race ./...
```

- `cmd/duplicacy-backup` — PASS
- `internal/config` — PASS (including new mandatory field tests)
- `internal/duplicacy` — PASS
- `internal/lock` — PASS
- `internal/logger` — PASS
- `internal/permissions` — PASS
- `internal/secrets` — PASS

### 6. Release Preparation

| File | Change |
|---|---|
| `CHANGELOG.md` | Created with v1.2.0 release notes |

---

## Suggested Version

**v1.2.0** — This release adds mandatory security configuration for
LOCAL_OWNER/LOCAL_GROUP, proper licensing, version flag support, and bug fixes.

---

## Build Instructions

```bash
# Build for current platform with version injection
make build

# Cross-compile for all Synology architectures
make synology

# The Makefile automatically injects version from git tags:
#   go build -ldflags "-s -w -X main.version=$(git describe --tags) -X main.buildTime=$(date -u)"
```

### Tagging the Release

```bash
git tag -a v1.2.0 -m "v1.2.0: Mandatory owner/group, licensing, and bug fixes"
git push origin v1.2.0
```

### Verify Version Flag

```bash
./build/duplicacy-backup --version
# Output: duplicacy-backup v1.2.0 (built 2026-04-08T...Z)
```

---

## Breaking Changes for Existing Users

> **LOCAL_OWNER and LOCAL_GROUP are now mandatory.**
>
> If you have an existing `.conf` file that relied on the defaults
> (`phillipmcmahon` / `users`), you must add these lines explicitly:
>
> ```ini
> LOCAL_OWNER=myuser
> LOCAL_GROUP=users
> ```
>
> The script will refuse to start without them.
