package health

import (
	"fmt"
	"strings"
	"time"
)

type VerifyRevision struct {
	Revision  int
	CreatedAt time.Time
}

type VerifyResult struct {
	Revision int
	Result   string
	Message  string
}

type VerifyReconciliation struct {
	CheckedRevisionCount int
	PassedRevisionCount  int
	FailedRevisionCount  int
	FailedRevisions      []int
	MissingRevisions     []int
	FailureCodes         []string
	RevisionResults      []RevisionResult
	DetailChecks         []Check
}

func ReconcileVerifyResults(visible []VerifyRevision, results []VerifyResult) VerifyReconciliation {
	visibleByRevision := make(map[int]VerifyRevision, len(visible))
	for _, revision := range visible {
		visibleByRevision[revision.Revision] = revision
	}

	accounted := make(map[int]bool, len(results))
	reconciled := VerifyReconciliation{
		CheckedRevisionCount: len(results),
	}

	for _, result := range results {
		revisionInfo, ok := visibleByRevision[result.Revision]
		if !ok {
			continue
		}
		accounted[result.Revision] = true
		if result.Result == "fail" {
			reconciled.addFailureCode(VerifyFailureIntegrityFailed)
			entry := RevisionResult{
				Revision: result.Revision,
				Result:   result.Result,
				Message:  normaliseVerifyResultMessage(result.Message),
			}
			if !revisionInfo.CreatedAt.IsZero() {
				entry.CreatedAt = formatReportTime(revisionInfo.CreatedAt)
			}
			reconciled.RevisionResults = append(reconciled.RevisionResults, entry)
			reconciled.FailedRevisionCount++
			reconciled.FailedRevisions = append(reconciled.FailedRevisions, result.Revision)
			reconciled.DetailChecks = append(reconciled.DetailChecks, Check{
				Name:    fmt.Sprintf("Revision %d", result.Revision),
				Result:  "fail",
				Message: result.Message,
			})
			continue
		}
		reconciled.PassedRevisionCount++
	}

	for _, revision := range visible {
		if accounted[revision.Revision] {
			continue
		}
		reconciled.addFailureCode(VerifyFailureResultMissing)
		reconciled.MissingRevisions = append(reconciled.MissingRevisions, revision.Revision)
		entry := RevisionResult{
			Revision: revision.Revision,
			Result:   "fail",
			Message:  "No integrity result returned",
		}
		if !revision.CreatedAt.IsZero() {
			entry.CreatedAt = formatReportTime(revision.CreatedAt)
		}
		reconciled.RevisionResults = append(reconciled.RevisionResults, entry)
		reconciled.DetailChecks = append(reconciled.DetailChecks, Check{
			Name:    fmt.Sprintf("Revision %d", revision.Revision),
			Result:  "fail",
			Message: "No integrity result returned",
		})
	}

	return reconciled
}

func (r *VerifyReconciliation) addFailureCode(code string) {
	if code == "" || containsString(r.FailureCodes, code) {
		return
	}
	r.FailureCodes = append(r.FailureCodes, code)
}

func normaliseVerifyResultMessage(message string) string {
	message = strings.TrimSpace(message)
	return strings.TrimSuffix(message, ".")
}
