package restore

import (
	"strings"
	"testing"
)

func TestRestoreOutputForReportSuccessSuppressesPerChunkAndPerFileProgress(t *testing.T) {
	output := strings.Join([]string{
		"Storage set to /volumeUSB2/usbshare/duplicacy/homes",
		"Downloaded chunk 1 size 25348363, 24.61MB/s 00:00:17 5.7%",
		"Downloaded phillipmcmahon/code/archive/file-one.tar.gz (5706777)",
		"Downloaded phillipmcmahon/code/archive/file-two.tar.gz (5202660)",
		"Restored /volume1/restore-drills/homes-onsite-usb-20260423-023000-rev1 to revision 1",
		"Files: 225 total, 411.76M bytes",
		"Downloaded 225 file, 411.76M bytes, 11 chunks",
		"Skipped 0 file, 0 bytes",
		"Total running time: 00:00:18",
	}, "\n")

	got := restoreOutputForReport(output, true)
	for _, want := range []string{
		"Restored /volume1/restore-drills/homes-onsite-usb-20260423-023000-rev1 to revision 1",
		"Files: 225 total, 411.76M bytes",
		"Downloaded 225 file, 411.76M bytes, 11 chunks",
		"Skipped 0 file, 0 bytes",
		"Total running time: 00:00:18",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q:\n%s", want, got)
		}
	}
	for _, noisy := range []string{"Downloaded chunk 1", "Downloaded phillipmcmahon/code/archive/file-one.tar.gz", "Storage set to"} {
		if strings.Contains(got, noisy) {
			t.Fatalf("summary should suppress %q:\n%s", noisy, got)
		}
	}
}

func TestRestoreOutputForReportFailureKeepsDiagnosticsAndSuppressesProgress(t *testing.T) {
	output := strings.Join([]string{
		"Downloaded chunk 1 size 25348363, 24.61MB/s 00:00:17 5.7%",
		"Downloaded phillipmcmahon/code/archive/file-one.tar.gz (5706777)",
		"Failed to download chunk 5: missing chunk 1234",
		"Error restoring phillipmcmahon/code/archive/file-two.tar.gz: permission denied",
	}, "\n")

	got := restoreOutputForReport(output, false)
	for _, want := range []string{
		"Failed to download chunk 5: missing chunk 1234",
		"Error restoring phillipmcmahon/code/archive/file-two.tar.gz: permission denied",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
	for _, noisy := range []string{"Downloaded chunk 1", "Downloaded phillipmcmahon/code/archive/file-one.tar.gz"} {
		if strings.Contains(got, noisy) {
			t.Fatalf("diagnostics should suppress %q:\n%s", noisy, got)
		}
	}
}

func TestRestoreOutputForReportFailureWithoutDiagnosticsUsesExplicitMessage(t *testing.T) {
	output := strings.Join([]string{
		"Storage set to /volumeUSB2/usbshare/duplicacy/homes",
		"Restoring /volume1/restore-drills/homes-onsite-usb-20260423-023000-rev1 to revision 1",
		"Downloaded chunk 1 size 25348363, 24.61MB/s 00:00:17 5.7%",
		"Downloaded phillipmcmahon/code/archive/file-one.tar.gz (5706777)",
		"Total running time: 00:00:18",
	}, "\n")

	got := restoreOutputForReport(output, false)
	want := "restore failed; Duplicacy did not emit diagnostic lines"
	if got != want {
		t.Fatalf("restoreOutputForReport() = %q, want %q", got, want)
	}
}

func TestRestoreDiagnosticPatternDoesNotMatchPartialWords(t *testing.T) {
	output := strings.Join([]string{
		"validation proceeded without terror",
		"Downloaded chunk 1 size 25348363",
	}, "\n")

	got := restoreOutputForReport(output, false)
	want := "restore failed; Duplicacy did not emit diagnostic lines"
	if got != want {
		t.Fatalf("restoreOutputForReport() = %q, want %q", got, want)
	}
}

func TestRestoreDuplicacyOutputFromRestoreRunExtractsMultilineSummary(t *testing.T) {
	output := strings.Join([]string{
		"Restore run for homes/onsite-usb revision 2403",
		"  Section: Duplicacy Summary",
		"    Output             : Restored /workspace to revision 2403",
		"Files: 225 total, 411.76M bytes",
		"Downloaded 225 file, 411.76M bytes, 11 chunks",
		"  Section: Safety",
		"    Live Source        : not modified",
	}, "\n")

	got := restoreDuplicacyOutputFromRestoreRun(output)
	for _, want := range []string{
		"Restored /workspace to revision 2403",
		"Files: 225 total, 411.76M bytes",
		"Downloaded 225 file, 411.76M bytes, 11 chunks",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("extracted output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Live Source") {
		t.Fatalf("extracted output should stop at next section:\n%s", got)
	}
}
