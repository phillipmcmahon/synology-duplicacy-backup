package workflow

import (
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

func extractRestoreFilePaths(output string) []string {
	// strings.Reader cannot produce scanner I/O errors; parser errors are only
	// possible for real reader failures, so the legacy slice-returning helper
	// can safely preserve its compact call sites.
	paths, _ := duplicacy.ParseListFilesOutput(strings.NewReader(output))
	return paths
}

func formatRevisionCreatedAt(revision duplicacy.RevisionInfo) string {
	if revision.CreatedAt.IsZero() {
		return ""
	}
	return revision.CreatedAt.Format("2006-01-02 15:04:05")
}
