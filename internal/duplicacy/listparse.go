package duplicacy

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// ParseListFilesOutput extracts snapshot-relative paths from Duplicacy
// "list -files" output. Duplicacy emits human-oriented status and summary
// lines around file rows, so this parser intentionally uses conservative
// heuristics based on the Duplicacy CLI output shape exercised by the restore
// smoke tests.
func ParseListFilesOutput(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	var paths []string
	seen := map[string]bool{}
	for scanner.Scan() {
		path := filepath.ToSlash(strings.TrimSpace(extractListFilePath(scanner.Text())))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Duplicacy list output: %w", err)
	}
	return paths, nil
}

func extractListFilePath(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || isIgnoredListFilesLine(line) {
		return ""
	}
	fields, remainder := splitLeadingFields(line, 3)
	if len(fields) < 3 || strings.TrimSpace(remainder) == "" {
		return line
	}
	if _, err := strconv.ParseInt(fields[0], 10, 64); err != nil {
		return line
	}
	if !looksLikeDate(fields[1]) || !looksLikeTime(fields[2]) {
		return line
	}
	return strings.TrimSpace(stripOptionalListFileDigest(remainder))
}

func stripOptionalListFileDigest(remainder string) string {
	fields, path := splitLeadingFields(remainder, 1)
	if len(fields) != 1 || strings.TrimSpace(path) == "" {
		return remainder
	}
	if strings.ContainsAny(fields[0], `/\`) || !looksLikeHexDigest(fields[0]) {
		return remainder
	}
	return path
}

func isIgnoredListFilesLine(line string) bool {
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
	if len(value) != len("15:04:05") && len(value) != len("15:04") {
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
