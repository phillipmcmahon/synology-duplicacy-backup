package workflow

import (
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
			&Plan{Location: locationLocal},
			restoreRunInputs{
				Revision:    8,
				Workspace:   "/volume1/restore-drills/homes-onsite-usb-20260425-130000-rev8",
				RestorePath: "phillipmcmahon/code/*",
			},
			time.Date(2026, 4, 25, 15, 54, 56, 0, time.UTC),
		)
	})

	want := "  Restore safety       : workspace only; live source will not be modified; copy-back is manual"
	if !strings.Contains(output, want) {
		t.Fatalf("restore safety warning alignment mismatch\nwant substring: %q\noutput:\n%s", want, output)
	}
	if strings.Contains(output, "  Restore safety      :") {
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
