package workflow

import (
	"regexp"
	"strings"
)

var (
	restoreDownloadedSummaryPattern = regexp.MustCompile(`^Downloaded\s+\d+\s+files?[, ]`)
	restoreSkippedSummaryPattern    = regexp.MustCompile(`^Skipped\s+\d+\s+files?[, ]`)
	restoreDiagnosticPattern        = regexp.MustCompile(`(?i)\b(?:error|failed|failure|unable|cannot|denied|corrupt|missing|incomplete|invalid)\b`)
)

func restoreOutputForReport(output string, success bool) string {
	lines := restoreOutputLines(output)
	if len(lines) == 0 {
		return ""
	}
	if success {
		return strings.Join(restoreSuccessSummaryLines(lines), "\n")
	}
	return strings.Join(restoreDiagnosticLines(lines), "\n")
}

func restoreSuccessSummaryLines(lines []string) []string {
	summary := make([]string, 0, 5)
	for _, line := range lines {
		if isRestoreSummaryLine(line) {
			summary = append(summary, line)
		}
	}
	if len(summary) == 0 {
		return []string{"restore completed; detailed Duplicacy output suppressed"}
	}
	return summary
}

func restoreDiagnosticLines(lines []string) []string {
	diagnostics := make([]string, 0, len(lines))
	for _, line := range lines {
		if isNoisyRestoreProgressLine(line) {
			continue
		}
		if restoreDiagnosticPattern.MatchString(line) {
			diagnostics = append(diagnostics, line)
		}
	}
	if len(diagnostics) > 0 {
		return diagnostics
	}
	return []string{"restore failed; Duplicacy did not emit diagnostic lines"}
}

func restoreOutputLines(output string) []string {
	rawLines := strings.Split(strings.TrimSpace(output), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func isRestoreSummaryLine(line string) bool {
	return strings.HasPrefix(line, "Restored ") && strings.Contains(line, " to revision ") ||
		strings.HasPrefix(line, "Files: ") ||
		restoreDownloadedSummaryPattern.MatchString(line) ||
		restoreSkippedSummaryPattern.MatchString(line) ||
		strings.HasPrefix(line, "Total running time:")
}

func isNoisyRestoreProgressLine(line string) bool {
	return strings.HasPrefix(line, "Downloaded chunk ") ||
		(strings.HasPrefix(line, "Downloaded ") && !restoreDownloadedSummaryPattern.MatchString(line))
}
