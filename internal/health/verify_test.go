package health

import (
	"reflect"
	"testing"
	"time"
)

func TestReconcileVerifyResultsHealthy(t *testing.T) {
	visible := []VerifyRevision{
		{Revision: 8, CreatedAt: time.Date(2026, 4, 15, 8, 0, 0, 0, time.UTC)},
		{Revision: 7, CreatedAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC)},
	}
	results := []VerifyResult{
		{Revision: 8, Result: "pass", Message: "All chunks exist"},
		{Revision: 7, Result: "pass", Message: "All chunks exist"},
	}

	got := ReconcileVerifyResults(visible, results)

	if got.CheckedRevisionCount != 2 || got.PassedRevisionCount != 2 || got.FailedRevisionCount != 0 {
		t.Fatalf("counts = checked %d passed %d failed %d", got.CheckedRevisionCount, got.PassedRevisionCount, got.FailedRevisionCount)
	}
	if len(got.FailureCodes) != 0 || len(got.RevisionResults) != 0 || len(got.DetailChecks) != 0 {
		t.Fatalf("healthy reconciliation included failure data: %+v", got)
	}
}

func TestReconcileVerifyResultsMixedFailedMissingAndUnknown(t *testing.T) {
	visible := []VerifyRevision{
		{Revision: 10, CreatedAt: time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)},
		{Revision: 9, CreatedAt: time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)},
		{Revision: 8, CreatedAt: time.Date(2026, 4, 15, 8, 0, 0, 0, time.UTC)},
	}
	results := []VerifyResult{
		{Revision: 10, Result: "pass", Message: "All chunks exist"},
		{Revision: 9, Result: "fail", Message: "missing chunks."},
		{Revision: 99, Result: "fail", Message: "not visible"},
	}

	got := ReconcileVerifyResults(visible, results)

	if got.CheckedRevisionCount != 3 || got.PassedRevisionCount != 1 || got.FailedRevisionCount != 1 {
		t.Fatalf("counts = checked %d passed %d failed %d", got.CheckedRevisionCount, got.PassedRevisionCount, got.FailedRevisionCount)
	}
	if !reflect.DeepEqual(got.FailedRevisions, []int{9}) {
		t.Fatalf("FailedRevisions = %#v, want []int{9}", got.FailedRevisions)
	}
	if !reflect.DeepEqual(got.MissingRevisions, []int{8}) {
		t.Fatalf("MissingRevisions = %#v, want []int{8}", got.MissingRevisions)
	}
	if !reflect.DeepEqual(got.FailureCodes, []string{VerifyFailureIntegrityFailed, VerifyFailureResultMissing}) {
		t.Fatalf("FailureCodes = %#v", got.FailureCodes)
	}
	if len(got.RevisionResults) != 2 {
		t.Fatalf("RevisionResults = %#v", got.RevisionResults)
	}
	if got.RevisionResults[0].Revision != 9 || got.RevisionResults[0].Message != "missing chunks" ||
		got.RevisionResults[0].CreatedAt != "2026-04-15T09:00:00Z" {
		t.Fatalf("RevisionResults[0] = %+v", got.RevisionResults[0])
	}
	if got.RevisionResults[1].Revision != 8 || got.RevisionResults[1].Message != "No integrity result returned" ||
		got.RevisionResults[1].CreatedAt != "2026-04-15T08:00:00Z" {
		t.Fatalf("RevisionResults[1] = %+v", got.RevisionResults[1])
	}
	if len(got.DetailChecks) != 2 || got.DetailChecks[0].Message != "missing chunks." ||
		got.DetailChecks[1].Message != "No integrity result returned" {
		t.Fatalf("DetailChecks = %#v", got.DetailChecks)
	}
}

func TestReconcileVerifyResultsDeduplicatesFailureCodes(t *testing.T) {
	visible := []VerifyRevision{
		{Revision: 3},
		{Revision: 2},
		{Revision: 1},
	}
	results := []VerifyResult{
		{Revision: 3, Result: "fail", Message: "missing chunks"},
		{Revision: 2, Result: "fail", Message: "missing chunks"},
	}

	got := ReconcileVerifyResults(visible, results)

	if !reflect.DeepEqual(got.FailureCodes, []string{VerifyFailureIntegrityFailed, VerifyFailureResultMissing}) {
		t.Fatalf("FailureCodes = %#v", got.FailureCodes)
	}
	if !reflect.DeepEqual(got.FailedRevisions, []int{3, 2}) {
		t.Fatalf("FailedRevisions = %#v", got.FailedRevisions)
	}
	if !reflect.DeepEqual(got.MissingRevisions, []int{1}) {
		t.Fatalf("MissingRevisions = %#v", got.MissingRevisions)
	}
}
