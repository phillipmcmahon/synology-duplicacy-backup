package restore

import (
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/restorepicker"
)

func TestRestoreDepsWithDefaultsPreservesOverridesAndFillsMissing(t *testing.T) {
	now := time.Date(2026, 4, 25, 18, 0, 0, 0, time.UTC)
	progress := noopRestoreProgress{}
	deps := RestoreDeps{
		Now:                  func() time.Time { return now },
		RestoreWorkspaceRoot: "/custom/root",
		Progress:             progress,
	}.withDefaults()

	if deps.Now().IsZero() || !deps.Now().Equal(now) {
		t.Fatalf("Now override not preserved")
	}
	if deps.RestoreWorkspaceRoot != "/custom/root" {
		t.Fatalf("RestoreWorkspaceRoot = %q", deps.RestoreWorkspaceRoot)
	}
	if deps.Progress == nil {
		t.Fatal("Progress should be populated")
	}
	if deps.NewRunner == nil || deps.PromptOutput == nil || deps.RunSelectPicker == nil || deps.RunInspectPicker == nil {
		t.Fatalf("defaults not populated: %#v", deps)
	}
}

func TestDefaultRestoreDepsSafeBranches(t *testing.T) {
	deps := defaultRestoreDeps()
	if deps.NewRunner() == nil {
		t.Fatal("default runner should be populated")
	}
	if deps.Now().IsZero() {
		t.Fatal("default Now should be populated")
	}
	deps.Progress.PrintStatus("noop")
	deps.Progress.StartActivity("noop")()
	deps.Progress.StartSelectionActivity(1, 1, "docs/readme.md")()

	opts := restorepicker.AppOptions{PathPrefix: "../outside"}
	if _, err := deps.RunSelectPicker([]string{"docs/readme.md"}, opts); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("RunSelectPicker unsafe prefix err = %v", err)
	}
	if err := deps.RunInspectPicker([]string{"docs/readme.md"}, opts); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("RunInspectPicker unsafe prefix err = %v", err)
	}
}
