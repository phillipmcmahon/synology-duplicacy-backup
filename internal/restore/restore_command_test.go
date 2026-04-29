package restore

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/restorepicker"
)

var (
	restorePromptOutput      io.Writer = os.Stdout
	runRestoreSelectPicker             = defaultRestoreDeps().RunSelectPicker
	runRestoreInspectPicker            = defaultRestoreDeps().RunInspectPicker
	restoreWorkspaceNow                = defaultRestoreDeps().Now
	testRestoreWorkspaceRoot           = defaultRestoreDeps().RestoreWorkspaceRoot
	newRestoreCommandRunner            = defaultRestoreDeps().NewRunner
	restoreProgress                    = defaultRestoreDeps().Progress
)

func restoreHandleCommand(req *Request, meta Metadata, rt Env) (string, error) {
	restoreReq := NewRestoreRequest(req)
	return handleRestoreCommand(&restoreReq, meta, rt, RestoreDeps{
		NewRunner:            newRestoreCommandRunner,
		PromptOutput:         restorePromptOutput,
		Now:                  restoreWorkspaceNow,
		RestoreWorkspaceRoot: testRestoreWorkspaceRoot,
		RunSelectPicker:      runRestoreSelectPicker,
		RunInspectPicker:     runRestoreInspectPicker,
		Progress:             restoreProgress,
	})
}

func restoreSelectRuntime(t *testing.T, input string) Env {
	t.Helper()
	rt := testRuntime()
	tempDir := t.TempDir()
	stdin, err := os.CreateTemp(tempDir, "stdin-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	if _, err := stdin.WriteString(input); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if _, err := stdin.Seek(0, 0); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}
	t.Cleanup(func() { _ = stdin.Close() })
	rt.Stdin = func() *os.File { return stdin }
	rt.StdinIsTTY = func() bool { return true }
	rt.TempDir = func() string { return tempDir }
	return rt
}

func nonRootRestoreRuntime() Env {
	rt := testRuntime()
	rt.Geteuid = func() int { return 1000 }
	return rt
}

func suppressRestorePrompts(t *testing.T) {
	t.Helper()
	old := restorePromptOutput
	var output strings.Builder
	restorePromptOutput = &output
	t.Cleanup(func() { restorePromptOutput = old })
}

func captureRestorePrompts(t *testing.T) *strings.Builder {
	t.Helper()
	old := restorePromptOutput
	var output strings.Builder
	restorePromptOutput = &output
	t.Cleanup(func() { restorePromptOutput = old })
	return &output
}

func stubRestoreSelectPicker(t *testing.T, stub func([]string, restorepicker.AppOptions) ([]string, error)) {
	t.Helper()
	old := runRestoreSelectPicker
	runRestoreSelectPicker = stub
	t.Cleanup(func() { runRestoreSelectPicker = old })
}

func stubRestoreInspectPicker(t *testing.T, stub func([]string, restorepicker.AppOptions) error) {
	t.Helper()
	old := runRestoreInspectPicker
	runRestoreInspectPicker = stub
	t.Cleanup(func() { runRestoreInspectPicker = old })
}

func stubRestoreWorkspaceTime(t *testing.T, ts time.Time) {
	t.Helper()
	old := restoreWorkspaceNow
	restoreWorkspaceNow = func() time.Time { return ts }
	t.Cleanup(func() { restoreWorkspaceNow = old })
}

func setupRestoreWorkspaceRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "restore-drills")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", root, err)
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatalf("Chmod(%q) error = %v", root, err)
	}
	return root
}

func stubRestoreProgress(t *testing.T, progress RestoreProgress) {
	t.Helper()
	old := restoreProgress
	restoreProgress = progress
	t.Cleanup(func() { restoreProgress = old })
}
