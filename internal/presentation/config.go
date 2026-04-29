package presentation

import (
	"fmt"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func FormatLines(title string, lines []Line) string {
	return FormatLinesWithSemanticColour(title, lines, false)
}

func FormatLinesWithSemanticColour(title string, lines []Line, enableColour bool) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	for _, line := range lines {
		fmt.Fprintf(&b, "  %-20s : %s\n", line.Label, ColourizeSemanticValue(line.Value, enableColour))
	}
	return b.String()
}

func FormatValidationReport(title string, resolved, validation []Line, result string, enableColour bool) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	writeSection(&b, "Resolved", resolved, false, enableColour)
	writeSection(&b, "Validation", validation, true, enableColour)
	fmt.Fprintf(&b, "  %-20s : %s\n", "Result", ColourizeValidationResult(result, enableColour))
	return b.String()
}

func ColourizeValidationValue(value string, enableColour bool) string {
	return ColourizeSemanticValue(value, enableColour)
}

func ColourizeSemanticValue(value string, enableColour bool) string {
	switch {
	case strings.HasPrefix(value, "Invalid ("),
		strings.HasPrefix(value, "Unreadable ("):
		return logger.ColourizeForLevel(logger.ERROR, value, enableColour)
	case strings.HasPrefix(value, ValueRequiresSudo):
		return logger.ColourizeForLevel(logger.WARNING, value, enableColour)
	case value == ValueNotChecked,
		value == ValueNotConfigured,
		value == ValueNotEnabled,
		value == ValueNotInitialized,
		value == ValueLimited,
		value == ValueDegraded,
		value == ValueSkipped:
		return logger.ColourizeForLevel(logger.WARNING, value, enableColour)
	case value == ValueValid,
		value == ValuePresent,
		value == ValueReadable,
		value == ValueWritable,
		value == ValueResolved,
		value == ValueParsed,
		value == ValuePassed,
		value == ValueHealthy,
		value == ValueValidated,
		value == "Available",
		strings.HasPrefix(value, "Available ("),
		value == "Success",
		value == "Full":
		return logger.ColourizeForLevel(logger.SUCCESS, value, enableColour)
	case value == ValueFailed,
		value == ValueUnhealthy:
		return logger.ColourizeForLevel(logger.ERROR, value, enableColour)
	default:
		return value
	}
}

func ColourizeValidationResult(value string, enableColour bool) string {
	switch value {
	case "Passed":
		return logger.ColourizeForLevel(logger.SUCCESS, value, enableColour)
	case "Failed":
		return logger.ColourizeForLevel(logger.ERROR, value, enableColour)
	default:
		return value
	}
}

func writeSection(b *strings.Builder, name string, lines []Line, semanticValues bool, enableColour bool) {
	fmt.Fprintf(b, "  Section: %s\n", name)
	for _, line := range lines {
		value := line.Value
		if semanticValues {
			value = ColourizeValidationValue(value, enableColour)
		}
		fmt.Fprintf(b, "    %-18s : %s\n", line.Label, value)
	}
}
