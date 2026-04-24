package restorepicker

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// LoadPaths reads snapshot-relative paths from an input stream.
// It accepts plain path lists and Duplicacy "list -files" style rows.
func LoadPaths(r io.Reader, pathPrefix string) ([]string, error) {
	scanner := bufio.NewScanner(r)
	var paths []string
	seen := map[string]bool{}
	prefix, err := cleanRelativePath(pathPrefix)
	if err != nil {
		return nil, err
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || isIgnoredListLine(line) {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(extractPath(line)))
		if path == "" {
			continue
		}
		path, err = cleanRelativePath(path)
		if err != nil {
			return nil, err
		}
		if prefix != "" && path != prefix && !strings.HasPrefix(path, prefix+"/") {
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	slices.Sort(paths)
	return paths, nil
}

func FilterPaths(paths []string, pathPrefix string) ([]string, error) {
	prefix, err := cleanRelativePath(pathPrefix)
	if err != nil {
		return nil, err
	}
	var filtered []string
	seen := map[string]bool{}
	for _, path := range paths {
		path, err := cleanRelativePath(path)
		if err != nil {
			return nil, err
		}
		if prefix != "" && path != prefix && !strings.HasPrefix(path, prefix+"/") {
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		filtered = append(filtered, path)
	}
	sort.Strings(filtered)
	return filtered, nil
}

func cleanRelativePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if filepath.IsAbs(value) {
		return "", fmt.Errorf("path must be snapshot-relative: %s", value)
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must stay inside the snapshot: %s", value)
	}
	return filepath.ToSlash(cleaned), nil
}

func extractPath(line string) string {
	if isIgnoredListLine(line) {
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

func isIgnoredListLine(line string) bool {
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
			if i >= len(value) {
				return false
			}
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
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
