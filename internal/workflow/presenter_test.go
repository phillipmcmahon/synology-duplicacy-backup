package workflow

import (
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func newPresenterTestLogger(t *testing.T) (*logger.Logger, string) {
	t.Helper()
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	return log, logDir
}

func TestPresenterSummaryAndBackupResult(t *testing.T) {
	log, logDir := newPresenterTestLogger(t)
	rt := testRuntime()
	rt.Now = func() time.Time { return time.Date(2026, 4, 10, 16, 47, 54, 100_000_000, time.UTC) }
	presenter := NewPresenter(DefaultMetadata("duplicacy-backup", "1.0.0", "now", logDir), rt, log, false)

	presenter.PrintHeader(&Plan{OperationMode: "Backup", BackupLabel: "homes", Target: "onsite-usb", StorageType: storageTypeDuplicacy, Location: locationLocal}, time.Date(2026, 4, 10, 16, 47, 50, 900_000_000, time.UTC), "")
	presenter.PrintSummary(&Plan{Summary: []SummaryLine{{Label: "Config File", Value: "/tmp/homes-backup.toml"}}})
	presenter.PrintBackupResult("Backup for /volume1/homes at revision 2361 completed\nFiles: 10 total, 42 bytes; 1 new, 10 bytes\nTotal running time: 00:00:03\n", "", false)
	presenter.PrintDuration(time.Date(2026, 4, 10, 16, 47, 50, 900_000_000, time.UTC))
	log.Close()

	output := readSingleLogFile(t, logDir)
	for _, token := range []string{"Run started - 2026-04-10 16:47:50", "Operation", "Backup", "Label", "homes", "Target", "onsite-usb", "Location", locationLocal, "Run Summary:", "Config File", "Revision", "2361", "Files", "Duration", "00:00:03"} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}
	if strings.Contains(output, "Mode") {
		t.Fatalf("output should not include Mode:\n%s", output)
	}
}

func TestPresenterCommandOutputVerboseAndForce(t *testing.T) {
	log, logDir := newPresenterTestLogger(t)
	presenter := NewPresenter(DefaultMetadata("duplicacy-backup", "1.0.0", "now", logDir), testRuntime(), log, true)

	presenter.PrintCommandOutput("Repository set to /volume1/homes\n", "warning line\n", false)
	presenter.PrintBackupResult("raw line\n", "stderr line\n", true)
	log.Close()

	output := readSingleLogFile(t, logDir)
	for _, token := range []string{"Output", "Repository set to /volume1/homes", "warning line", "raw line", "stderr line"} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}
}

func TestPresenterPreRunFailurePlanIncludesStorageIdentity(t *testing.T) {
	log, logDir := newPresenterTestLogger(t)
	presenter := NewPresenter(DefaultMetadata("duplicacy-backup", "1.0.0", "now", logDir), testRuntime(), log, false)

	presenter.PrintPreRunFailurePlan(&Plan{
		OperationMode: "Fix permissions",
		BackupLabel:   "homes",
		Target:        "offsite-storj",
		StorageType:   storageTypeDuplicacy,
		Location:      locationRemote,
	})
	log.Close()

	output := readSingleLogFile(t, logDir)
	for _, token := range []string{"Run could not start", "Operation", "Fix permissions", "Label", "homes", "Target", "offsite-storj", "Location", locationRemote} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}
}
