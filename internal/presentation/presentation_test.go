package presentation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func newPresentationTestLogger(t *testing.T) (*logger.Logger, string) {
	t.Helper()
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	return log, logDir
}

func readPresentationLog(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 log file, found %d", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	return string(data)
}

func TestFormatLinesAndValidationReport(t *testing.T) {
	lines := []Line{{Label: "Config File", Value: "/tmp/homes-backup.toml"}}
	got := FormatLines("Run Summary:", lines)
	if !strings.Contains(got, "Run Summary:") || !strings.Contains(got, "Config File") {
		t.Fatalf("FormatLines() = %q", got)
	}

	report := FormatValidationReport(
		"Config validation",
		[]Line{{Label: "Label", Value: "homes"}},
		[]Line{{Label: "Privileges", Value: "Limited"}, {Label: "Result", Value: "Valid"}},
		"Passed",
		false,
	)
	for _, token := range []string{"Section: Resolved", "Section: Validation", "Privileges", "Limited", "Result", "Passed"} {
		if !strings.Contains(report, token) {
			t.Fatalf("FormatValidationReport() missing %q:\n%s", token, report)
		}
	}
}

func TestRuntimePresenterPrintsHeaderAndBackupSummary(t *testing.T) {
	log, logDir := newPresentationTestLogger(t)
	p := NewRuntimePresenter(func() time.Time {
		return time.Date(2026, 4, 15, 10, 1, 4, 0, time.UTC)
	}, log, false)

	p.PrintHeader(HeaderData{
		StartedAt: time.Date(2026, 4, 15, 10, 1, 0, 0, time.UTC),
		Operation: "Backup",
		Label:     "homes",
		Target:    "onsite-usb1",
		Location:  "local",
	})
	p.PrintSummary([]Line{{Label: "Config File", Value: "/tmp/homes-backup.toml"}})
	p.PrintBackupResult("Backup for /volume1/homes at revision 2361 completed\nFiles: 10 total, 42 bytes; 1 new, 10 bytes\nTotal running time: 00:00:03\n", "", false)
	p.PrintCompletion(0, time.Date(2026, 4, 15, 10, 1, 0, 0, time.UTC))
	log.Close()

	output := readPresentationLog(t, logDir)
	for _, token := range []string{
		"Run started - 2026-04-15 10:01:00",
		"Operation", "Backup",
		"Config File", "/tmp/homes-backup.toml",
		"Revision", "2361",
		"Files", "10 total, 42 bytes; 1 new, 10 bytes",
		"Duration", "00:00:03",
		"Run completed - 2026-04-15 10:01:04",
	} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}
}

func TestRuntimePresenterPrunePreviewAndForcedOutput(t *testing.T) {
	log, logDir := newPresentationTestLogger(t)
	p := NewRuntimePresenter(func() time.Time { return time.Date(2026, 4, 15, 10, 2, 0, 0, time.UTC) }, log, false)

	p.PrintPrunePreview(&duplicacy.PrunePreview{
		DeleteCount:     3,
		TotalRevisions:  12,
		DeletePercent:   25,
		PercentEnforced: true,
	}, 20)
	p.PrintCommandOutput("line one\n", "warning line\n", true)
	log.Close()

	output := readPresentationLog(t, logDir)
	for _, token := range []string{"Preview Deletes", "3", "Preview Total Revs", "12", "Preview Delete %", "25", "line one", "warning line"} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}
}

func TestRuntimePresenterPreRunStatusAndValidationColourBranches(t *testing.T) {
	log, logDir := newPresentationTestLogger(t)
	p := NewRuntimePresenter(func() time.Time {
		return time.Date(2026, 4, 15, 10, 3, 0, 0, time.UTC)
	}, log, false)

	p.PrintPreRunFailure(PreRunFailureData{
		Operation: "Backup",
		Label:     "homes",
		Target:    "onsite-usb1",
		Location:  "local",
	})
	p.PrintPhase("Permissions")
	stop := p.StartStatusActivity("Checking permissions")
	stop()
	p.PrintDuration(time.Date(2026, 4, 15, 10, 3, 1, 0, time.UTC))
	log.Close()

	output := readPresentationLog(t, logDir)
	for _, token := range []string{
		"Run could not start",
		"Operation", "Backup",
		"Phase: Permissions",
		"Status", "Checking permissions",
		"Duration", "00:00:00",
	} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}

	valueCases := []string{"Invalid (missing)", "Not checked", "Not initialized", "Requires sudo", "Limited", "Readable", "Writable", "Resolved", "Parsed", "Full", "Custom"}
	for _, value := range valueCases {
		if got := ColourizeValidationValue(value, false); got != value {
			t.Fatalf("ColourizeValidationValue(%q, false) = %q", value, got)
		}
	}
	for _, value := range []string{"Passed", "Failed", "Skipped"} {
		if got := ColourizeValidationResult(value, false); got != value {
			t.Fatalf("ColourizeValidationResult(%q, false) = %q", value, got)
		}
	}
}

func TestDisplayLabelUsesSharedOperatorVocabulary(t *testing.T) {
	cases := map[string]string{
		"Config file":       "Config File",
		"Source path":       "Source Path",
		"Repository access": "Repository Access",
		"Revision count":    "Revision Count",
		"Integrity check":   "Integrity Check",
		"Unknown check":     "Unknown check",
	}

	for input, want := range cases {
		if got := DisplayLabel(input); got != want {
			t.Fatalf("DisplayLabel(%q) = %q, want %q", input, got, want)
		}
	}
}
