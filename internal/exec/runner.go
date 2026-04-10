// Package exec provides a shared command-runner abstraction for executing
// external processes.  It centralises all os/exec usage behind a testable
// [Runner] interface so that downstream packages (btrfs, duplicacy,
// permissions) can be unit-tested without invoking real binaries.
//
// # Interface
//
// [Runner] defines two methods that cover every command-execution pattern
// used by the application:
//
//   - [Runner.Run] – execute a command and capture stdout/stderr.
//   - [Runner.RunWithInput] – same, but also pipe data to stdin.
//
// # Concrete implementation
//
// [CommandRunner] is the production implementation.  It wraps [os/exec]
// with context support, structured logging, consistent error wrapping,
// and an optional dry-run mode.  Construct one with [NewCommandRunner].
//
// # Testing
//
// [MockRunner] is an in-memory test double that records every invocation
// and replays pre-configured responses.  Use [NewMockRunner] in unit tests
// for downstream packages that accept a Runner.
//
// # Migration note
//
// Before this package existed, btrfs, duplicacy, and permissions each
// called [os/exec.Command] directly.  Those packages now accept a Runner
// via their constructors / function parameters, which makes them fully
// testable without real binaries on the PATH.
package exec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

// Runner is the interface for executing external commands.  All packages
// that need to shell out accept a Runner so that tests can substitute a
// [MockRunner] and avoid real process execution.
type Runner interface {
	// Run executes cmd with the given arguments, capturing stdout and stderr.
	// The returned strings contain the full captured output.  A non-nil error
	// is returned when the command exits non-zero or cannot be started.
	Run(ctx context.Context, cmd string, args ...string) (stdout, stderr string, err error)

	// RunInDir is identical to Run but sets the working directory for the
	// child process to dir before execution.  This is required when a tool
	// (such as duplicacy) locates its configuration relative to the current
	// working directory.
	RunInDir(ctx context.Context, dir string, cmd string, args ...string) (stdout, stderr string, err error)

	// RunWithInput is identical to Run but additionally writes input to the
	// command's stdin before waiting for it to complete.
	RunWithInput(ctx context.Context, input string, cmd string, args ...string) (stdout, stderr string, err error)
}

// CommandRunner is the production [Runner] implementation.  It delegates to
// [os/exec.CommandContext], logs every invocation, and optionally operates
// in dry-run mode where no process is actually spawned.
type CommandRunner struct {
	log           *logger.Logger
	dryRun        bool
	debugCommands bool
}

// NewCommandRunner returns a ready-to-use [CommandRunner].
//
// Parameters:
//   - log: structured logger used to record every command invocation.
//   - dryRun: when true, commands are logged but not executed; Run and
//     RunWithInput return empty strings and a nil error.
func NewCommandRunner(log *logger.Logger, dryRun bool) *CommandRunner {
	return &CommandRunner{
		log:           log,
		dryRun:        dryRun,
		debugCommands: true,
	}
}

// SetDebugCommands enables or suppresses raw command debug logging.
func (r *CommandRunner) SetDebugCommands(enabled bool) {
	r.debugCommands = enabled
}

// DebugCommands reports whether raw command debug logging is enabled.
func (r *CommandRunner) DebugCommands() bool {
	return r.debugCommands
}

// Run executes the command, capturing stdout and stderr into strings.
// The context is forwarded to [os/exec.CommandContext] so that callers
// can enforce timeouts or cancellation.
func (r *CommandRunner) Run(ctx context.Context, cmd string, args ...string) (string, string, error) {
	return r.run(ctx, "", "", cmd, args...)
}

// RunInDir executes the command with the working directory set to dir.
func (r *CommandRunner) RunInDir(ctx context.Context, dir string, cmd string, args ...string) (string, string, error) {
	return r.run(ctx, dir, "", cmd, args...)
}

// RunWithInput executes the command with the given string piped to stdin.
func (r *CommandRunner) RunWithInput(ctx context.Context, input string, cmd string, args ...string) (string, string, error) {
	return r.run(ctx, "", input, cmd, args...)
}

// run is the shared implementation for Run, RunInDir, and RunWithInput.
func (r *CommandRunner) run(ctx context.Context, dir string, input string, cmd string, args ...string) (string, string, error) {
	cmdStr := formatCommand(cmd, args)

	if r.dryRun {
		r.log.DryRun("%s", cmdStr)
		return "", "", nil
	}

	if r.debugCommands {
		r.log.Debug("exec: %s", cmdStr)
	}

	c := exec.CommandContext(ctx, cmd, args...)

	if dir != "" {
		c.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	if input != "" {
		c.Stdin = strings.NewReader(input)
	}

	err := c.Run()

	outStr := stdout.String()
	errStr := stderr.String()

	if err != nil {
		return outStr, errStr, fmt.Errorf("command %q failed: %w", cmdStr, err)
	}

	return outStr, errStr, nil
}

// formatCommand builds a human-readable representation of a command for
// logging purposes.
func formatCommand(cmd string, args []string) string {
	if len(args) == 0 {
		return cmd
	}
	return cmd + " " + strings.Join(args, " ")
}

// ---------------------------------------------------------------------------
// MockRunner – test double
// ---------------------------------------------------------------------------

// Invocation records a single command execution for test assertions.
type Invocation struct {
	Ctx   context.Context
	Cmd   string
	Args  []string
	Dir   string // non-empty only for RunInDir calls
	Input string // non-empty only for RunWithInput calls
}

// MockResult holds the pre-configured response for a single command execution.
type MockResult struct {
	Stdout string
	Stderr string
	Err    error
}

// MockRunner is a test double that records invocations and replays
// pre-configured results in FIFO order.  When the result queue is
// exhausted it returns empty strings and nil error.
//
// Example usage in a test:
//
//	mock := exec.NewMockRunner(
//	    exec.MockResult{Stdout: "btrfs\n"},
//	    exec.MockResult{},
//	)
//	err := btrfs.CheckVolume(mock, log, "/volume1", false)
//	assert(mock.Invocations[0].Cmd == "stat")
type MockRunner struct {
	// Invocations records every call to Run or RunWithInput in order.
	Invocations []Invocation

	// results is the FIFO queue of responses.
	results []MockResult
}

// NewMockRunner creates a [MockRunner] pre-loaded with the given results.
// Each call to Run or RunWithInput pops the next result from the queue.
func NewMockRunner(results ...MockResult) *MockRunner {
	return &MockRunner{results: results}
}

// Run records the invocation and returns the next queued result.
func (m *MockRunner) Run(ctx context.Context, cmd string, args ...string) (string, string, error) {
	return m.record(ctx, "", "", cmd, args)
}

// RunInDir records the invocation (including dir) and returns the next queued result.
func (m *MockRunner) RunInDir(ctx context.Context, dir string, cmd string, args ...string) (string, string, error) {
	return m.record(ctx, dir, "", cmd, args)
}

// RunWithInput records the invocation (including input) and returns the
// next queued result.
func (m *MockRunner) RunWithInput(ctx context.Context, input string, cmd string, args ...string) (string, string, error) {
	return m.record(ctx, "", input, cmd, args)
}

func (m *MockRunner) record(ctx context.Context, dir string, input string, cmd string, args []string) (string, string, error) {
	m.Invocations = append(m.Invocations, Invocation{
		Ctx:   ctx,
		Cmd:   cmd,
		Args:  args,
		Dir:   dir,
		Input: input,
	})

	if len(m.results) == 0 {
		return "", "", nil
	}

	r := m.results[0]
	m.results = m.results[1:]
	return r.Stdout, r.Stderr, r.Err
}
