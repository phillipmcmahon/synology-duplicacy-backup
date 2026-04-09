package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

// newTestLogger creates a logger in a temp dir for tests.
func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	dir := t.TempDir()
	log, err := logger.New(dir, "test", false)
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return log
}

// ---------------------------------------------------------------------------
// CommandRunner – successful execution
// ---------------------------------------------------------------------------

func TestCommandRunner_Run_Success(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	stdout, stderr, err := r.Run(context.Background(), "echo", "hello", "world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout); got != "hello world" {
		t.Errorf("stdout = %q, want %q", got, "hello world")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestCommandRunner_Run_CapturesStderr(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	// sh -c writes to stderr
	stdout, stderr, err := r.Run(context.Background(), "sh", "-c", "echo errout >&2; echo ok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "ok") {
		t.Errorf("stdout should contain 'ok', got %q", stdout)
	}
	if !strings.Contains(stderr, "errout") {
		t.Errorf("stderr should contain 'errout', got %q", stderr)
	}
}

func TestCommandRunner_RunWithInput_Success(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	stdout, _, err := r.RunWithInput(context.Background(), "hello stdin", "cat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout); got != "hello stdin" {
		t.Errorf("stdout = %q, want %q", got, "hello stdin")
	}
}

// ---------------------------------------------------------------------------
// CommandRunner – error handling
// ---------------------------------------------------------------------------

func TestCommandRunner_Run_CommandNotFound(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	_, _, err := r.Run(context.Background(), "nonexistent_binary_xyz_12345")
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("error should contain 'failed', got %q", err.Error())
	}
}

func TestCommandRunner_Run_NonZeroExit(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	stdout, stderr, err := r.Run(context.Background(), "sh", "-c", "echo out; echo err >&2; exit 42")
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	// Output should still be captured even on error
	if !strings.Contains(stdout, "out") {
		t.Errorf("stdout should contain 'out', got %q", stdout)
	}
	if !strings.Contains(stderr, "err") {
		t.Errorf("stderr should contain 'err', got %q", stderr)
	}
}

func TestCommandRunner_Run_ErrorWrapping(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	_, _, err := r.Run(context.Background(), "sh", "-c", "exit 1")
	if err == nil {
		t.Fatal("expected error")
	}
	// Error should contain the command string
	if !strings.Contains(err.Error(), "sh") {
		t.Errorf("error should reference command, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// CommandRunner – context cancellation
// ---------------------------------------------------------------------------

func TestCommandRunner_Run_ContextCancellation(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := r.Run(ctx, "sleep", "10")
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Errorf("context should be deadline exceeded, got %v", ctx.Err())
	}
}

func TestCommandRunner_Run_ContextAlreadyCancelled(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, _, err := r.Run(ctx, "echo", "should not run")
	if err == nil {
		t.Fatal("expected error from already-cancelled context")
	}
}

// ---------------------------------------------------------------------------
// CommandRunner – dry-run mode
// ---------------------------------------------------------------------------

func TestCommandRunner_DryRun_Run(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, true)

	stdout, stderr, err := r.Run(context.Background(), "rm", "-rf", "/")
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	if stdout != "" {
		t.Errorf("dry-run stdout should be empty, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("dry-run stderr should be empty, got %q", stderr)
	}
}

func TestCommandRunner_DryRun_RunWithInput(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, true)

	stdout, stderr, err := r.RunWithInput(context.Background(), "data", "cat")
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("dry-run output should be empty")
	}
}

func TestCommandRunner_DryRun_DoesNotExecute(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, true)

	// This command would fail if actually executed
	_, _, err := r.Run(context.Background(), "nonexistent_binary_xyz_12345")
	if err != nil {
		t.Fatal("dry-run should not attempt to execute the command")
	}
}

// ---------------------------------------------------------------------------
// MockRunner tests
// ---------------------------------------------------------------------------

func TestMockRunner_RecordsInvocations(t *testing.T) {
	mock := NewMockRunner(MockResult{Stdout: "out1"}, MockResult{Stdout: "out2"})

	ctx := context.Background()
	out1, _, _ := mock.Run(ctx, "cmd1", "a", "b")
	out2, _, _ := mock.Run(ctx, "cmd2", "c")

	if out1 != "out1" {
		t.Errorf("first call stdout = %q, want %q", out1, "out1")
	}
	if out2 != "out2" {
		t.Errorf("second call stdout = %q, want %q", out2, "out2")
	}
	if len(mock.Invocations) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Cmd != "cmd1" {
		t.Errorf("invocation[0].Cmd = %q", mock.Invocations[0].Cmd)
	}
	if mock.Invocations[1].Cmd != "cmd2" {
		t.Errorf("invocation[1].Cmd = %q", mock.Invocations[1].Cmd)
	}
}

func TestMockRunner_RunWithInput_RecordsInput(t *testing.T) {
	mock := NewMockRunner(MockResult{Stdout: "result"})

	out, _, _ := mock.RunWithInput(context.Background(), "my input", "cmd", "arg")
	if out != "result" {
		t.Errorf("stdout = %q, want %q", out, "result")
	}
	if len(mock.Invocations) != 1 {
		t.Fatal("expected 1 invocation")
	}
	if mock.Invocations[0].Input != "my input" {
		t.Errorf("input = %q, want %q", mock.Invocations[0].Input, "my input")
	}
}

func TestMockRunner_ReturnsError(t *testing.T) {
	want := errors.New("boom")
	mock := NewMockRunner(MockResult{Err: want})

	_, _, err := mock.Run(context.Background(), "cmd")
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

func TestMockRunner_ExhaustedQueue(t *testing.T) {
	mock := NewMockRunner(MockResult{Stdout: "first"})

	mock.Run(context.Background(), "cmd1")
	// Second call has no queued result — should return empty/nil
	out, errout, err := mock.Run(context.Background(), "cmd2")
	if out != "" || errout != "" || err != nil {
		t.Errorf("exhausted queue should return empty/nil, got out=%q err=%q err=%v", out, errout, err)
	}
	if len(mock.Invocations) != 2 {
		t.Errorf("expected 2 invocations, got %d", len(mock.Invocations))
	}
}

func TestMockRunner_EmptyResults(t *testing.T) {
	mock := NewMockRunner()

	out, errout, err := mock.Run(context.Background(), "anything")
	if out != "" || errout != "" || err != nil {
		t.Errorf("empty mock should return empty/nil")
	}
}

// ---------------------------------------------------------------------------
// CommandRunner – RunInDir
// ---------------------------------------------------------------------------

func TestCommandRunner_RunInDir_SetsWorkingDirectory(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	dir := t.TempDir()
	stdout, _, err := r.RunInDir(context.Background(), dir, "pwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(stdout)
	if got != dir {
		t.Errorf("RunInDir working directory = %q, want %q", got, dir)
	}
}

func TestCommandRunner_RunInDir_EmptyDirUsesDefault(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	// Empty dir should not fail — behaves like Run (inherits parent's cwd)
	_, _, err := r.RunInDir(context.Background(), "", "echo", "ok")
	if err != nil {
		t.Fatalf("RunInDir with empty dir should not error: %v", err)
	}
}

func TestCommandRunner_RunInDir_ErrorOnBadDir(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, false)

	_, _, err := r.RunInDir(context.Background(), "/nonexistent_dir_xyz_99999", "echo", "hi")
	if err == nil {
		t.Fatal("expected error for nonexistent working directory")
	}
}

func TestCommandRunner_RunInDir_DryRun(t *testing.T) {
	log := newTestLogger(t)
	r := NewCommandRunner(log, true)

	stdout, stderr, err := r.RunInDir(context.Background(), "/whatever", "rm", "-rf", "/")
	if err != nil {
		t.Fatalf("dry-run RunInDir should not error: %v", err)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("dry-run output should be empty")
	}
}

// ---------------------------------------------------------------------------
// MockRunner – RunInDir
// ---------------------------------------------------------------------------

func TestMockRunner_RunInDir_RecordsDir(t *testing.T) {
	mock := NewMockRunner(MockResult{Stdout: "ok"})

	out, _, _ := mock.RunInDir(context.Background(), "/my/work/dir", "duplicacy", "backup")
	if out != "ok" {
		t.Errorf("stdout = %q, want %q", out, "ok")
	}
	if len(mock.Invocations) != 1 {
		t.Fatal("expected 1 invocation")
	}
	inv := mock.Invocations[0]
	if inv.Dir != "/my/work/dir" {
		t.Errorf("Dir = %q, want %q", inv.Dir, "/my/work/dir")
	}
	if inv.Cmd != "duplicacy" {
		t.Errorf("Cmd = %q, want %q", inv.Cmd, "duplicacy")
	}
}

func TestMockRunner_Run_HasEmptyDir(t *testing.T) {
	mock := NewMockRunner(MockResult{})

	mock.Run(context.Background(), "echo", "hi")
	if len(mock.Invocations) != 1 {
		t.Fatal("expected 1 invocation")
	}
	if mock.Invocations[0].Dir != "" {
		t.Errorf("Run should record empty Dir, got %q", mock.Invocations[0].Dir)
	}
}

func TestMockRunner_RunInDir_EmptyDir(t *testing.T) {
	mock := NewMockRunner(MockResult{})

	mock.RunInDir(context.Background(), "", "echo", "hi")
	if mock.Invocations[0].Dir != "" {
		t.Errorf("RunInDir with empty dir should record empty Dir, got %q", mock.Invocations[0].Dir)
	}
}

func TestMockRunner_MixedRunAndRunInDir(t *testing.T) {
	mock := NewMockRunner(MockResult{}, MockResult{}, MockResult{})

	mock.Run(context.Background(), "cmd1")
	mock.RunInDir(context.Background(), "/work", "cmd2")
	mock.RunWithInput(context.Background(), "data", "cmd3")

	if len(mock.Invocations) != 3 {
		t.Fatalf("expected 3 invocations, got %d", len(mock.Invocations))
	}
	// Run: no Dir, no Input
	if mock.Invocations[0].Dir != "" || mock.Invocations[0].Input != "" {
		t.Errorf("Run invocation should have empty Dir and Input")
	}
	// RunInDir: has Dir, no Input
	if mock.Invocations[1].Dir != "/work" || mock.Invocations[1].Input != "" {
		t.Errorf("RunInDir invocation: Dir=%q Input=%q", mock.Invocations[1].Dir, mock.Invocations[1].Input)
	}
	// RunWithInput: no Dir, has Input
	if mock.Invocations[2].Dir != "" || mock.Invocations[2].Input != "data" {
		t.Errorf("RunWithInput invocation: Dir=%q Input=%q", mock.Invocations[2].Dir, mock.Invocations[2].Input)
	}
}

// ---------------------------------------------------------------------------
// formatCommand tests
// ---------------------------------------------------------------------------

func TestFormatCommand_NoArgs(t *testing.T) {
	if got := formatCommand("ls", nil); got != "ls" {
		t.Errorf("formatCommand = %q, want %q", got, "ls")
	}
}

func TestFormatCommand_WithArgs(t *testing.T) {
	got := formatCommand("btrfs", []string{"subvolume", "snapshot", "-r", "/src", "/dst"})
	want := "btrfs subvolume snapshot -r /src /dst"
	if got != want {
		t.Errorf("formatCommand = %q, want %q", got, want)
	}
}
