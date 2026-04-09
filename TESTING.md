# Testing Guide

## Quick Start

```bash
# Run all tests
go test ./... -count=1

# Run with verbose output
go test ./... -count=1 -v

# Run with coverage report
go test ./... -coverprofile=coverage.out -count=1
go tool cover -func=coverage.out          # per-function summary
go tool cover -html=coverage.out          # browser-based report

# Run a single package
go test ./internal/logger/... -count=1 -v

# Run a single test
go test ./cmd/duplicacy-backup/... -run TestApp_RunPrunePhase -v
```

## Test Organisation

| Package | Test File(s) | Focus |
|---------|-------------|-------|
| `cmd/duplicacy-backup` | `main_test.go`, `integration_test.go` | Coordinator logic, flag parsing, phase execution |
| `internal/btrfs` | `btrfs_test.go` | Snapshot create/delete, volume checks |
| `internal/config` | `config_test.go` | INI parsing, defaults, validation |
| `internal/duplicacy` | `duplicacy_test.go` | Duplicacy CLI wrapper, prune preview |
| `internal/errors` | `errors_test.go` | Structured error types |
| `internal/exec` | `runner_test.go` | Command runner, mock runner |
| `internal/lock` | `lock_test.go` | Directory-based locking |
| `internal/logger` | `logger_test.go` | Structured logging, file output, colours |
| `internal/permissions` | `permissions_test.go` | chown/chmod operations |
| `internal/secrets` | `secrets_test.go` | Secrets file loading, validation, masking |

## Testing Patterns

### MockRunner

All external command execution flows through the `exec.Runner` interface.
Tests use `exec.MockRunner` to provide deterministic responses:

```go
mock := execpkg.NewMockRunner(
    execpkg.MockResult{Stdout: "OK\n"},                          // first call
    execpkg.MockResult{Err: fmt.Errorf("something broke")},      // second call
)

// Results are consumed in FIFO order.
// After all results are consumed, further calls return ("", "", nil).
// mock.Invocations records every call for assertion.
```

### testApp() Helper (cmd/duplicacy-backup)

The `testApp(t)` function creates a minimal `*app` with safe defaults:
- A real logger writing to a temp directory
- `dryRun: true` by default (safe for unit tests)
- Empty `MockRunner` on `a.runner`
- Temp directories for config, secrets, and work root

Override fields as needed:

```go
a := testApp(t)
a.flags.dryRun = false
a.cfg = config.NewDefaults()
a.dup = duplicacy.NewSetup(workRoot, "/repo", "/target", false, mock)
```

### Logger Tests

Logger tests use `newTestLogger(t)` which returns a logger, its log file
path, and the temp directory. The `readLogFile(t, path)` helper reads the
log file for content assertions:

```go
lg, logPath, _ := newTestLogger(t)
lg.Info("hello %s", "world")
lg.Close()

content := readLogFile(t, logPath)
// assert on content
```

### Integration Tests

Integration tests in `integration_test.go` use the `TestIntegration_` prefix
and exercise end-to-end flows like full backup dry-runs. They use `testApp()`
but set up more complete state (config, secrets, flags).

### Secrets Tests and Root

Some secrets tests require `uid:gid = 0:0` file ownership, which is only
achievable as root. These tests call `t.Skip("requires root")` when run as
a non-root user. This is expected and keeps CI green without privileged
containers.

## Mocked Dependencies

The project mocks **all** external dependencies via the `exec.Runner`
interface. No real binaries (`duplicacy`, `btrfs`, `chown`, `chmod`) are
called during unit tests. The mock layer covers:

- **Duplicacy CLI** — backup, prune, list, deep-prune
- **BTRFS** — subvolume snapshot, subvolume delete, subvolume show
- **Permissions** — chown, chmod via find
- **Lock** — mkdir-based directory locks (uses real filesystem)
- **Secrets** — file-based (uses real temp files with controlled permissions)

## Coverage Summary (as of April 2026)

| Package | Before | After |
|---------|--------|-------|
| `cmd/duplicacy-backup` | 52.6% | 76.7% |
| `internal/btrfs` | 100% | 100% |
| `internal/config` | 99.3% | 99.3% |
| `internal/duplicacy` | 92.2% | 92.2% |
| `internal/errors` | 100% | 100% |
| `internal/exec` | 100% | 100% |
| `internal/lock` | 92.6% | 92.6% |
| `internal/logger` | 14.8% | 93.2% |
| `internal/permissions` | 100% | 100% |
| `internal/secrets` | 40.0% | 40.0% |
| **Total** | **66.2%** | **84.2%** |

**Key improvements:**
- **Logger:** 14.8% → 93.2% (+78.4pp) — rewrote test file with 45+ tests
- **cmd/duplicacy-backup:** 52.6% → 76.7% (+24.1pp) — added 50+ tests for phases, output, edge cases
- **Secrets:** Additional edge-case tests added (coverage limited by root-only ownership checks)
- **Total test count:** ~364 → 450 tests

## Adding New Tests

1. Follow existing naming: `TestFunctionName_Scenario`
2. Use `testApp(t)` for coordinator tests
3. Use `exec.MockRunner` for any code that shells out
4. Return structured errors (`internal/errors`) — don't log in packages
5. Run `gofmt -w .` before committing
6. The pre-commit hook runs `go test ./...` automatically
