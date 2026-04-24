package workflow

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

func extractRestoreFilePaths(output string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isIgnoredRestoreListLine(line) {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(extractRestoreFilePath(line)))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func extractRestoreFilePath(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || isIgnoredRestoreListLine(line) {
		return ""
	}
	fields, remainder := splitLeadingFields(line, 4)
	if len(fields) < 4 || strings.TrimSpace(remainder) == "" {
		return line
	}
	if _, err := strconv.ParseInt(fields[0], 10, 64); err != nil {
		return line
	}
	if !looksLikeDate(fields[1]) || !looksLikeTime(fields[2]) || !looksLikeHexDigest(fields[3]) {
		return line
	}
	return strings.TrimSpace(remainder)
}

func isIgnoredRestoreListLine(line string) bool {
	line = strings.TrimSpace(line)
	switch {
	case line == "":
		return true
	case strings.HasPrefix(line, "Files: "):
		return true
	case strings.HasPrefix(line, "Total size: "):
		return true
	case strings.HasPrefix(line, "Repository set to "):
		return true
	case strings.HasPrefix(line, "Storage set to "):
		return true
	case strings.HasPrefix(line, "Loaded "):
		return true
	case strings.HasPrefix(line, "Parsing "):
		return true
	case strings.HasPrefix(line, "Restoring "):
		return true
	case strings.HasPrefix(line, "Restored "):
		return true
	case strings.HasPrefix(line, "Skipped "):
		return true
	case strings.HasPrefix(line, "Downloaded "):
		return true
	case strings.HasPrefix(line, "Total running time: "):
		return true
	case strings.HasPrefix(line, "Snapshot ") && strings.Contains(line, " created at "):
		return true
	default:
		return false
	}
}

func splitLeadingFields(value string, count int) ([]string, string) {
	remainder := strings.TrimLeftFunc(value, unicode.IsSpace)
	fields := make([]string, 0, count)
	for len(fields) < count && remainder != "" {
		fieldEnd := strings.IndexFunc(remainder, unicode.IsSpace)
		if fieldEnd < 0 {
			fields = append(fields, remainder)
			return fields, ""
		}
		fields = append(fields, remainder[:fieldEnd])
		remainder = strings.TrimLeftFunc(remainder[fieldEnd:], unicode.IsSpace)
	}
	return fields, remainder
}

func looksLikeDate(value string) bool {
	if len(value) != len("2006-01-02") {
		return false
	}
	for i, r := range value {
		switch i {
		case 4, 7:
			if r != '-' {
				return false
			}
		default:
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func looksLikeTime(value string) bool {
	if len(value) != len("15:04:05") {
		return false
	}
	for i, r := range value {
		switch i {
		case 2, 5:
			if r != ':' {
				return false
			}
		default:
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func looksLikeHexDigest(value string) bool {
	if len(value) < 8 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func formatRevisionCreatedAt(revision duplicacy.RevisionInfo) string {
	if revision.CreatedAt.IsZero() {
		return ""
	}
	return revision.CreatedAt.Format("2006-01-02 15:04:05")
}
