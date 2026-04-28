package workflow

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"

func formatRevisionCreatedAt(revision duplicacy.RevisionInfo) string {
	if revision.CreatedAt.IsZero() {
		return ""
	}
	return revision.CreatedAt.Format("2006-01-02 15:04:05")
}
