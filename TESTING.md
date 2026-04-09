# Testing Guide

## Overview

The project uses Go's standard `testing` package with no external test
frameworks. Every internal package has a dedicated `*_test.go` file, and
the coordinator (`cmd/duplicacy-backup`) has both unit tests
(`main_test.go`) and integration tests (`integration_test.go`).

**All external commands are mocked** — no real binaries (`duplicacy`,
`btrfs`, `chown`, `chmod`) are invoked during testing.

| Metric | Value |
|--------|-------|
| Total tests passing | **487** |
| Skipped tests | 5 (environment-specific, see below) |
| Overall statement coverage | **90.1 %** |

## Quick Start

```bash
# Run all tests
go test ./... -count=1

# Verbose output
go test ./... -count=1 -v

# Coverage report (terminal)
go test ./... -coverprofile=coverage.out -count=1
go tool cover -func=coverage.out

# Coverage report (browser)
go tool cover -html=coverage.out

# Single package
go test ./internal/secrets/... -count=1 -v

# Single test
go test ./cmd/duplicacy-backup/... -run TestApp_RunPrunePhase -v
```

### Prerequisites

- **Go 1.21+** (uses `testing` stdlib only — no external dependencies)
- Tests run as a regular user; no root privileges required (see
  [Root-gated tests](#root-gated-tests) for the few exceptions).

## Test Organisation

| Package | Test File(s) | Tests | Coverage | Focus |
|---------|-------------|------:|--------:|-------|
| `cmd/duplicacy-backup` | `main_test.go`, `integration_test.go` | 173 | 83.7 % | Coordinator logic, flag parsing, phase execution, end-to-end flows |
| `internal/btrfs` | `btrfs_test.go` | 12 | 100 % | Snapshot create/delete, volume checks |
| `internal/config` | `config_test.go` | 66 | 99.3 % | INI parsing, defaults, validation |
| `internal/duplicacy` | `duplicacy_test.go` | 53 | 92.2 % | Duplicacy CLI wrapper, prune preview |
| `internal/errors` | `errors_test.go` | 29 | 100 % | Structured error types |
| `internal/exec` | `runner_test.go` | 35 | 100 % | CommandRunner, MockRunner, RunInDir |
| `internal/lock` | `lock_test.go` | 20 | 92.6 % | Directory-based locking |
| `internal/logger` | `logger_test.go` | 43 | 93.2 % | Structured logging, file output, colours |
| `internal/permissions` | `permissions_test.go` | 8 | 100 % | chown/chmod operations |
| `internal/secrets` | `secrets_test.go` | 48 | 88.1 % | Secrets parsing, file validation, masking |

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

Each `Invocation` records `Cmd`, `Args`, and `Dir` (the working directory
for `RunInDir` calls), so tests can assert both the commands issued and
where they were executed.

### captureOutput() Helper

The `captureOutput(t, fn)` helper in `main_test.go` redirects `os.Stdout`
and `os.Stderr` to pipes, runs the provided function, then returns the
captured output as strings. This is used by the `TestRunWithArgs_*` tests
to suppress and inspect output from the full coordinator pipeline:

```go
_, stderr := captureOutput(t, func() {
    code := runWithArgs([]string{"--fix-perms", "--config-dir", configDir, "homes"})
    // assert on code
})
// assert on stderr content
```

### testApp() Helper (unit tests)

The `testApp(t)` function in `main_test.go` creates a minimal `*app` with
safe defaults:

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

### itestApp() Factory (integration tests)

The `itestApp(t)` function in `integration_test.go` builds a fully-wired
`*app` for end-to-end integration tests. It goes further than `testApp()`
by also:

- Writing a valid INI config file to a temp directory
- Writing a valid secrets file (with correct `0600` permissions)
- Pre-loading configuration and secrets into the app struct
- Wiring up a `MockRunner` with pre-queued results for the expected
  command sequence

This allows integration tests to exercise the full coordinator pipeline
(`acquireLock → loadConfig → loadSecrets → execute → cleanup`) without
any real external dependencies.

```go
func TestIntegration_RunCoordinator_PruneDryRun(t *testing.T) {
    a, _ := itestApp(t)
    a.flags.doPrune = true
    // ... exercise the full pipeline
}
```

Seven `TestIntegration_RunCoordinator_*` tests cover:

| Test | What it exercises |
|------|-------------------|
| `_PruneDryRun` | Full dry-run prune workflow |
| `_FixPermsOnlyDryRun` | Fix-permissions-only dry-run flow |
| `_BackupDryRun` | Backup dry-run path |
| `_ExecuteError_CleanupStillRuns` | Cleanup runs even when execute fails |
| `_CleanupIdempotent` | Calling cleanup multiple times is safe |
| `_PrintCommandOutput` | printCommandOutput handles various inputs |
| `_LockAcquisition` | Lock acquire / release cycle |

### Logger Tests

Logger tests use `newTestLogger(t)` which returns a logger, its log file
path, and the temp directory:

```go
lg, logPath, _ := newTestLogger(t)
lg.Info("hello %s", "world")
lg.Close()

content := readLogFile(t, logPath)
// assert on content
```

## Key Test Approaches

### Function-Variable Seams and `runWithArgs`

The coordinator (`main.go`) uses **package-level function variables** as
test seams for dependencies that cannot be injected through the `app`
struct:

| Variable | Production value | What it enables |
|----------|-----------------|-----------------|
| `cliArgs` | `func() []string { return os.Args[1:] }` | Decouple argument source from `os.Args` |
| `geteuid` | `os.Geteuid` | Simulate non-root / root in tests |
| `lookPath` | `exec.LookPath` | Stub binary-existence checks |
| `newLock` | `lock.New` | Redirect lock directory to temp dirs |

The `run()` function delegates to `runWithArgs(args)`, which accepts an
explicit argument slice. This allows tests to exercise the **full
coordinator pipeline** (argument parsing → validation → config loading →
lock acquisition → error translation) without manipulating `os.Args`:

```go
// Test that --help exits cleanly through the full pipeline
captureOutput(t, func() {
    if code := runWithArgs([]string{"--help"}); code != 0 {
        t.Errorf("want 0, got %d", code)
    }
})
```

Tests swap the function variables, call `runWithArgs`, and restore the
originals via `defer`:

```go
oldGeteuid := geteuid
geteuid = func() int { return 1000 }  // simulate non-root
defer func() { geteuid = oldGeteuid }()
```

Six `TestRunWithArgs_*` tests cover: `--help`, `--version`, invalid
flags, non-root rejection, config-load failure (with error message
assertion), and lock-acquisition failure.

### Symlink Normalisation in Runner Tests

`TestCommandRunner_RunInDir_SetsWorkingDirectory` verifies that `RunInDir`
correctly sets the working directory for a child process. On systems where
`/tmp` is a symlink (e.g., macOS `/tmp → /private/tmp`, some Linux
distros), a naïve string comparison between the expected path and `pwd`
output would fail.

The fix applies `filepath.EvalSymlinks()` to **both** the expected
directory and the actual `pwd` output before comparison, ensuring the test
passes regardless of symlink topology:

```go
expected, _ := filepath.EvalSymlinks(dir)
actual, _ := filepath.EvalSymlinks(strings.TrimSpace(stdout))
if actual != expected { … }
```

### Separated ParseSecrets / ValidateFileAccess Architecture

The secrets package was refactored from a monolithic `LoadSecretsFile()`
into two independently testable functions:

| Function | Responsibility |
|----------|---------------|
| `ValidateFileAccess(path)` | OS-level checks: file exists, `0600` perms, `root:root` ownership |
| `ParseSecrets(r io.Reader, source)` | Pure parsing: reads key-value pairs, validates keys, strips quotes |

`LoadSecretsFile` now simply calls `ValidateFileAccess` then opens the
file and passes it to `ParseSecrets`.

**Why this matters for testing:**

- `ParseSecrets` accepts an `io.Reader`, so tests use
  `strings.NewReader()` — no filesystem, no root privileges needed.
- 13 dedicated `TestParseSecrets_*` tests cover valid input, missing keys,
  unknown keys, duplicate keys, malformed lines, empty files, and
  error-message source attribution.
- `ValidateFileAccess` has its own 3-test suite for missing files,
  wrong permissions, and ownership checks.
- This refactoring raised secrets coverage from **~50 % → 86.4 %** and
  eliminated 10 previously-skipped root-gated parser tests.

### Root-Gated Tests

A small number of tests require `uid:gid = 0:0` file ownership and call
`t.Skip("requires root")` when run as a non-root user. Currently 5 tests
are skipped in a non-root environment — all related to `ValidateFileAccess`
ownership verification and environment validation that checks real system
paths. This is expected and keeps CI green without privileged containers.

## Mocked Dependencies

| Dependency | Mock Layer | What is mocked |
|-----------|-----------|----------------|
| Duplicacy CLI | `MockRunner` | backup, prune, list, deep-prune |
| BTRFS | `MockRunner` | subvolume snapshot, delete, show |
| Permissions | `MockRunner` | chown, chmod via find |
| Lock | Real filesystem | mkdir-based directory locks (temp dirs) |
| Secrets (file access) | Real filesystem | temp files with controlled permissions |
| Secrets (parser) | `io.Reader` | `strings.NewReader`, `errReader` for I/O errors |
| Config | Real filesystem | temp INI files |

## Coverage Summary (v1.8.2, April 2026)

| Package | Coverage |
|---------|--------:|
| `cmd/duplicacy-backup` | 83.7 % |
| `internal/btrfs` | 100.0 % |
| `internal/config` | 99.3 % |
| `internal/duplicacy` | 92.2 % |
| `internal/errors` | 100.0 % |
| `internal/exec` | 100.0 % |
| `internal/lock` | 92.6 % |
| `internal/logger` | 93.2 % |
| `internal/permissions` | 100.0 % |
| `internal/secrets` | 88.1 % |
| **Total** | **90.1 %** |

### Notable Coverage Improvements (v1.8.x)

| Area | Before | After | Key change |
|------|-------:|------:|------------|
| Secrets | ~50 % | 88.1 % | ParseSecrets/ValidateFileAccess split; errReader test |
| Logger | 14.8 % | 93.2 % | Rewrote test suite with 43 tests |
| Coordinator | 52.6 % | 83.7 % | Added integration tests via itestApp; runWithArgs seam tests |
| **Overall** | **~66 %** | **90.1 %** | |

## Testing Philosophy

1. **No external test frameworks** — stdlib `testing` only, keeping
   dependencies minimal for a NAS appliance binary.
2. **Mock at the process boundary** — the `exec.Runner` interface is the
   single seam between the application and the OS. Tests never shell out.
3. **Structured errors, not log assertions** — internal packages return
   typed errors (`*errors.BackupError`, `*errors.SnapshotError`, etc.)
   that tests can inspect with `errors.As`. Tests never assert on log
   output from internal packages.
4. **Safe by default** — `testApp()` and `itestApp()` set `dryRun: true`
   so a forgotten assertion can never accidentally mutate real state.
5. **Deterministic mocks** — `MockRunner` returns results in FIFO order.
   Tests declare exactly the command sequence they expect.
6. **Filesystem-portable** — path comparisons use `filepath.EvalSymlinks`
   where symlinks may differ between environments.

## Adding New Tests

1. Follow existing naming: `TestFunctionName_Scenario`
2. Use `testApp(t)` for coordinator unit tests
3. Use `itestApp(t)` for coordinator integration tests
4. Use `exec.MockRunner` for any code that shells out
5. Return structured errors (`internal/errors`) — don't log in packages
6. Run `gofmt -w .` before committing
7. The pre-commit hook runs `go test ./...` automatically
