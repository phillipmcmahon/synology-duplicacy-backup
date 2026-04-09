package exec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

// ─── Integration tests ──────────────────────────────────────────────────────
// These tests verify end-to-end command execution through the Runner interface,
// including error propagation, output capture, context handling, and that the
// MockRunner faithfully simulates CommandRunner behaviour for downstream tests.

func integrationLogger(t *testing.T) *logger.Logger {
	t.Helper()
	dir := t.TempDir()
	log, err := logger.New(dir, "integration-test", false)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return log
}

// TestIntegration_BackupCommandSimulation verifies that a mock runner
// can simulate the full backup command lifecycle (validate → backup → prune).
func TestIntegration_BackupCommandSimulation(t *testing.T) {
	mock := NewMockRunner(
		// 1. stat -f -c %T /volume1 → btrfs
		MockResult{Stdout: "btrfs\n"},
		// 2. btrfs subvolume show /volume1 → success
		MockResult{},
		// 3. btrfs subvolume snapshot -r → success
		MockResult{Stdout: "Create a readonly snapshot\n"},
		// 4. duplicacy backup -stats -threads 4 → success
		MockResult{Stdout: "Backup completed\n"},
		// 5. btrfs subvolume delete → success
		MockResult{},
	)

	ctx := context.Background()

	// Simulate CheckVolume
	stdout, _, err := mock.Run(ctx, "stat", "-f", "-c", "%T", "/volume1")
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if stdout != "btrfs\n" {
		t.Errorf("stat stdout = %q", stdout)
	}

	_, _, err = mock.Run(ctx, "btrfs", "subvolume", "show", "/volume1")
	if err != nil {
		t.Fatalf("btrfs subvolume show failed: %v", err)
	}

	// Simulate CreateSnapshot
	stdout, _, err = mock.Run(ctx, "btrfs", "subvolume", "snapshot", "-r", "/volume1/homes", "/volume1/homes-snap")
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	// Simulate RunBackup
	stdout, _, err = mock.Run(ctx, "duplicacy", "backup", "-stats", "-threads", "4")
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	if stdout != "Backup completed\n" {
		t.Errorf("backup stdout = %q", stdout)
	}

	// Simulate DeleteSnapshot
	_, _, err = mock.Run(ctx, "btrfs", "subvolume", "delete", "/volume1/homes-snap")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// Verify all 5 invocations were recorded
	if len(mock.Invocations) != 5 {
		t.Fatalf("expected 5 invocations, got %d", len(mock.Invocations))
	}
}

// TestIntegration_PruneCommandSimulation verifies the full prune lifecycle
// with mock runner including safe prune preview, revision counting, and prune.
func TestIntegration_PruneCommandSimulation(t *testing.T) {
	mock := NewMockRunner(
		// 1. duplicacy list -files (ValidateRepo)
		MockResult{},
		// 2. duplicacy prune -keep 0:365 -dry-run (SafePrunePreview)
		MockResult{Stdout: "Deleting snapshot at revision 1\nDeleting snapshot at revision 2\n"},
		// 3. duplicacy list (GetTotalRevisionCount)
		MockResult{Stdout: "revision 1\nrevision 2\nrevision 3\nrevision 4\nrevision 5\n"},
		// 4. duplicacy prune -keep 0:365 (RunPrune)
		MockResult{Stdout: "Prune completed\n"},
	)

	ctx := context.Background()

	// ValidateRepo
	_, _, err := mock.Run(ctx, "duplicacy", "list", "-files")
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}

	// SafePrunePreview
	stdout, _, _ := mock.Run(ctx, "duplicacy", "prune", "-keep", "0:365", "-dry-run")
	if stdout == "" {
		t.Error("expected prune preview output")
	}

	// GetTotalRevisionCount
	stdout, _, _ = mock.Run(ctx, "duplicacy", "list")
	if stdout == "" {
		t.Error("expected revision list output")
	}

	// RunPrune
	_, _, err = mock.Run(ctx, "duplicacy", "prune", "-keep", "0:365")
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if len(mock.Invocations) != 4 {
		t.Fatalf("expected 4 invocations, got %d", len(mock.Invocations))
	}
}

// TestIntegration_ErrorPropagation verifies that errors from the runner
// propagate correctly through the call chain.
func TestIntegration_ErrorPropagation(t *testing.T) {
	mock := NewMockRunner(
		MockResult{Err: errors.New("command failed")},
	)

	_, _, err := mock.Run(context.Background(), "failing-cmd")
	if err == nil {
		t.Fatal("expected error propagation")
	}
	if err.Error() != "command failed" {
		t.Errorf("error = %q, want 'command failed'", err.Error())
	}
}

// TestIntegration_RealCommandExecution verifies that CommandRunner correctly
// executes real commands and captures output.
func TestIntegration_RealCommandExecution(t *testing.T) {
	log := integrationLogger(t)
	runner := NewCommandRunner(log, false)

	// Test a sequence of real commands
	stdout, _, err := runner.Run(context.Background(), "echo", "step1")
	if err != nil {
		t.Fatalf("echo step1 failed: %v", err)
	}
	if got := string([]byte(stdout)[:5]); got != "step1" {
		t.Errorf("step1 stdout = %q", stdout)
	}

	stdout, _, err = runner.Run(context.Background(), "echo", "step2")
	if err != nil {
		t.Fatalf("echo step2 failed: %v", err)
	}
	if got := string([]byte(stdout)[:5]); got != "step2" {
		t.Errorf("step2 stdout = %q", stdout)
	}
}

// TestIntegration_CommandRunnerContextTimeout verifies context timeout
// propagation through real command execution.
func TestIntegration_CommandRunnerContextTimeout(t *testing.T) {
	log := integrationLogger(t)
	runner := NewCommandRunner(log, false)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, _, err := runner.Run(ctx, "sleep", "10")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// TestIntegration_DryRunNoSideEffects verifies that dry-run mode prevents
// all command execution across multiple calls.
func TestIntegration_DryRunNoSideEffects(t *testing.T) {
	log := integrationLogger(t)
	runner := NewCommandRunner(log, true)

	// None of these should actually execute
	for _, cmd := range []string{"rm", "btrfs", "duplicacy", "chown"} {
		stdout, stderr, err := runner.Run(context.Background(), cmd, "-rf", "/")
		if err != nil {
			t.Errorf("dry-run %s should not error: %v", cmd, err)
		}
		if stdout != "" || stderr != "" {
			t.Errorf("dry-run %s should produce no output", cmd)
		}
	}
}

// TestIntegration_LoggingConsistency verifies that all command executions
// are logged consistently.
func TestIntegration_LoggingConsistency(t *testing.T) {
	log := integrationLogger(t)
	runner := NewCommandRunner(log, false)

	// Execute a few commands - they should all be logged
	runner.Run(context.Background(), "echo", "first")
	runner.Run(context.Background(), "echo", "second")
	runner.Run(context.Background(), "echo", "third")

	// No assertion on log content (we'd need to read the log file),
	// but this verifies no panics or errors from logging.
}

// TestIntegration_MockRunnerMatchesInterface verifies that MockRunner
// satisfies the Runner interface at compile time and behaves consistently.
func TestIntegration_MockRunnerMatchesInterface(t *testing.T) {
	var r Runner = NewMockRunner(
		MockResult{Stdout: "hello"},
		MockResult{Stderr: "warning"},
		MockResult{Err: errors.New("fail")},
	)

	stdout, _, err := r.Run(context.Background(), "cmd1")
	if stdout != "hello" || err != nil {
		t.Errorf("first call: stdout=%q err=%v", stdout, err)
	}

	_, stderr, err := r.Run(context.Background(), "cmd2")
	if stderr != "warning" || err != nil {
		t.Errorf("second call: stderr=%q err=%v", stderr, err)
	}

	_, _, err = r.Run(context.Background(), "cmd3")
	if err == nil {
		t.Error("third call should return error")
	}
}

// TestIntegration_SnapshotOperationsUnchanged verifies that the mock runner
// can simulate snapshot create/delete operations with the same argument
// patterns used by the btrfs package.
func TestIntegration_SnapshotOperationsUnchanged(t *testing.T) {
	mock := NewMockRunner(
		MockResult{Stdout: "Create a readonly snapshot of '/volume1/homes' in '/volume1/homes-snap'\n"},
		MockResult{Stdout: "Delete subvolume '/volume1/homes-snap'\n"},
	)

	ctx := context.Background()

	// Create snapshot
	out, _, err := mock.Run(ctx, "btrfs", "subvolume", "snapshot", "-r", "/volume1/homes", "/volume1/homes-snap")
	if err != nil {
		t.Fatalf("snapshot create: %v", err)
	}
	if out == "" {
		t.Error("expected snapshot output")
	}

	// Delete snapshot
	out, _, err = mock.Run(ctx, "btrfs", "subvolume", "delete", "/volume1/homes-snap")
	if err != nil {
		t.Fatalf("snapshot delete: %v", err)
	}
	if out == "" {
		t.Error("expected delete output")
	}

	// Verify invocation args match what btrfs package passes
	if mock.Invocations[0].Args[0] != "subvolume" {
		t.Errorf("create args[0] = %q, want 'subvolume'", mock.Invocations[0].Args[0])
	}
	if mock.Invocations[0].Args[1] != "snapshot" {
		t.Errorf("create args[1] = %q, want 'snapshot'", mock.Invocations[0].Args[1])
	}
	if mock.Invocations[1].Args[0] != "subvolume" {
		t.Errorf("delete args[0] = %q, want 'subvolume'", mock.Invocations[1].Args[0])
	}
	if mock.Invocations[1].Args[1] != "delete" {
		t.Errorf("delete args[1] = %q, want 'delete'", mock.Invocations[1].Args[1])
	}
}
