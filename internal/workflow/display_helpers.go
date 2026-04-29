package workflow

import "strings"

func modeDisplay(targetName string) string {
	if targetName != "" {
		return targetName
	}
	return "not supplied"
}

func nonEmptyLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func splitNonEmptyLines(value string) []string {
	return nonEmptyLines(value)
}
