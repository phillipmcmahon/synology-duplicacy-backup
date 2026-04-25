package workflow

import (
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

func extractRestoreFilePaths(output string) []string {
	paths, _ := duplicacy.ParseListFilesOutput(strings.NewReader(output))
	return paths
}

func formatRevisionCreatedAt(revision duplicacy.RevisionInfo) string {
	if revision.CreatedAt.IsZero() {
		return ""
	}
	return revision.CreatedAt.Format("2006-01-02 15:04:05")
}
