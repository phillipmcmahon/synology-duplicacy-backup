package workflow

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func TestRestoreProgressSafetyWarningAlignment(t *testing.T) {
	output := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		defer log.Close()

		progress := NewRestoreProgress(
			Metadata{},
			Runtime{Now: func() time.Time { return time.Unix(0, 0).UTC() }},
			log,
		)

		progress.PrintRunStart(
			&RestoreRequest{Label: "homes", TargetName: "onsite-usb"},
			&Plan{Config: PlanConfig{Location: locationLocal}},
			restoreRunInputs{
				Revision:    8,
				Workspace:   "/volume1/restore-drills/homes-onsite-usb-20260425-130000-rev8",
				RestorePath: "phillipmcmahon/code/*",
			},
			time.Date(2026, 4, 25, 15, 54, 56, 0, time.UTC),
		)
	})

	want := "  Restore Safety       : workspace only; live source will not be modified; copy-back is manual"
	if !strings.Contains(output, want) {
		t.Fatalf("restore safety warning alignment mismatch\nwant substring: %q\noutput:\n%s", want, output)
	}
	if strings.Contains(output, "  Restore Safety      :") {
		t.Fatalf("restore safety warning used the old short alignment\noutput:\n%s", output)
	}
}

func TestRestoreProgressInterruptedExplainsWorkspaceAndCleanup(t *testing.T) {
	output := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		defer log.Close()

		progress := NewRestoreProgress(
			Metadata{},
			Runtime{Now: func() time.Time { return time.Unix(0, 0).UTC() }},
			log,
		)

		progress.PrintInterrupted(restoreInterruptInfo{
			Signal:      "interrupt",
			Workspace:   "/volume1/restore-drills/homes-onsite-usb-20260425-130000-rev8",
			Completed:   6,
			Total:       11,
			CurrentPath: "phillipmcmahon/documents/misc/*",
		})
	})

	for _, want := range []string{
		"Restore interrupted  : received interrupt; cancelling active Duplicacy restore",
		"Workspace retained   : /volume1/restore-drills/homes-onsite-usb-20260425-130000-rev8",
		"Cleanup              : no restored files were deleted automatically",
		"Completed paths      : 6 of 11",
		"Current path         : phillipmcmahon/documents/misc/*",
		"Live source          : not modified",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("interrupt output missing %q\noutput:\n%s", want, output)
		}
	}
}

func TestRestoreProgressSelectionStartStatusActivityAndCompletion(t *testing.T) {
	output := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		defer log.Close()

		progress := NewRestoreProgress(
			Metadata{},
			Runtime{Now: func() time.Time { return time.Date(2026, 4, 25, 16, 0, 0, 0, time.UTC) }},
			log,
		)

		progress.PrintSelectionStart(
			&RestoreRequest{Label: "homes", TargetName: "onsite-usb"},
			&Plan{Config: PlanConfig{Location: locationLocal}},
			8,
			"/volume1/restore-drills/homes-onsite-usb-20260425-130000-rev8",
			3,
			time.Date(2026, 4, 25, 16, 0, 0, 0, time.UTC),
		)
		progress.PrintStatus("Preparing drill workspace")
		progress.StartActivity("Restoring selected path")()
		progress.StartSelectionActivity(1, 3, "phillipmcmahon/code/*")()
		progress.PrintRunCompletion(false, time.Date(2026, 4, 25, 16, 0, 0, 0, time.UTC))
	})

	for _, want := range []string{
		"Operation", "Restore selection",
		"Restore paths", "3",
		"Status", "Preparing drill workspace",
		"Status", "Restoring selected path",
		"Status", "Restoring selection 1 of 3: phillipmcmahon/code/*",
		"Result", "Failed",
		"Code", "1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput:\n%s", want, output)
		}
	}
}

func TestRestoreProgressActivitiesDescribeFullAndSelectiveRuns(t *testing.T) {
	full := restoreProgressActivity(restoreRunInputs{Revision: 8})
	if full != "Restoring revision 8 into drill workspace" {
		t.Fatalf("restoreProgressActivity(full) = %q", full)
	}

	selective := restoreProgressActivity(restoreRunInputs{Revision: 8, RestorePath: "phillipmcmahon/code/*"})
	if selective != "Restoring selected path from revision 8 into drill workspace" {
		t.Fatalf("restoreProgressActivity(selective) = %q", selective)
	}

	longPath := strings.Repeat("a", 100)
	status := restoreSelectionProgressActivity(2, 12, longPath)
	if !strings.HasPrefix(status, "Restoring selection 2 of 12: ") {
		t.Fatalf("restoreSelectionProgressActivity() = %q, want selection prefix", status)
	}
	if len(strings.TrimPrefix(status, "Restoring selection 2 of 12: ")) != 90 {
		t.Fatalf("short path length mismatch in %q", status)
	}
	if !strings.HasSuffix(status, "...") {
		t.Fatalf("restoreSelectionProgressActivity() = %q, want ellipsis", status)
	}
}

func TestRestoreInterruptTrackerClampsCurrentProgressAndMarksUnstartedInterrupt(t *testing.T) {
	tracker := &restoreInterruptTracker{}
	tracker.setCurrent(0, 3, "")
	info := tracker.markInterrupted(os.Interrupt)
	if info.Completed != 0 {
		t.Fatalf("Completed = %d, want clamped zero", info.Completed)
	}
	if info.CurrentPath != "<full revision>" {
		t.Fatalf("CurrentPath = %q, want full revision marker", info.CurrentPath)
	}

	tracker = &restoreInterruptTracker{}
	info = tracker.markInterrupted(os.Interrupt)
	if info.Total != 1 {
		t.Fatalf("Total = %d, want default 1 for unstarted interrupt", info.Total)
	}
	if restoreInterruptProgress(info) != "0 of 1" {
		t.Fatalf("restoreInterruptProgress() = %q, want 0 of 1", restoreInterruptProgress(info))
	}
}

func TestRestoreProgressNoopIsSafe(t *testing.T) {
	progress := NewRestoreProgress(Metadata{}, Runtime{}, nil)
	progress.PrintRunStart(nil, nil, restoreRunInputs{}, time.Time{})
	progress.PrintSelectionStart(nil, nil, 0, "", 0, time.Time{})
	progress.PrintStatus("status")
	progress.StartActivity("activity")()
	progress.StartSelectionActivity(1, 1, "docs/readme.md")()
	progress.PrintInterrupted(restoreInterruptInfo{})
	progress.PrintRunCompletion(true, time.Time{})

	noop := noopRestoreProgress{}
	noop.PrintRunStart(nil, nil, restoreRunInputs{}, time.Time{})
	noop.PrintSelectionStart(nil, nil, 0, "", 0, time.Time{})
	noop.PrintStatus("status")
	noop.PrintInterrupted(restoreInterruptInfo{})
	noop.PrintRunCompletion(false, time.Time{})
}
