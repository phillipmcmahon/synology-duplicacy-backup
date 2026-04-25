package restorepicker

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

// LoadPaths reads snapshot-relative paths from an input stream.
// It accepts plain path lists and Duplicacy "list -files" style rows.
func LoadPaths(r io.Reader, pathPrefix string) ([]string, error) {
	var paths []string
	seen := map[string]bool{}
	prefix, err := cleanRelativePath(pathPrefix)
	if err != nil {
		return nil, err
	}
	rawPaths, err := duplicacy.ParseListFilesOutput(r)
	if err != nil {
		return nil, err
	}
	for _, rawPath := range rawPaths {
		path := filepath.ToSlash(strings.TrimSpace(rawPath))
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
