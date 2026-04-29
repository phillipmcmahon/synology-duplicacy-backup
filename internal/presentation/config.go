package presentation

import (
	"fmt"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func FormatLines(title string, lines []Line) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	for _, line := range lines {
		fmt.Fprintf(&b, "  %-20s : %s\n", line.Label, line.Value)
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
	switch {
	case strings.HasPrefix(value, "Invalid ("):
		return logger.ColourizeForLevel(logger.ERROR, value, enableColour)
	case value == ValueNotChecked || value == "Not initialized" || value == ValueRequiresSudo:
		return logger.ColourizeForLevel(logger.WARNING, value, enableColour)
	case value == "Limited":
		return logger.ColourizeForLevel(logger.WARNING, value, enableColour)
	case value == ValueValid,
		value == ValuePresent,
		value == ValueReadable,
		value == ValueWritable,
		value == ValueResolved,
		value == ValueParsed,
		value == "Full":
		return logger.ColourizeForLevel(logger.SUCCESS, value, enableColour)
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
