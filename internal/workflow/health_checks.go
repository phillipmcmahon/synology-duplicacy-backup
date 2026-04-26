package workflow

import (
	"fmt"
	"os"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	healthpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/health"
)

func (h *HealthRunner) runStatusChecks(report *HealthReport, req *HealthRequest, cfg *config.Config, plan *Plan, state *RunState, dup *duplicacy.Setup) []duplicacy.RevisionInfo {
	report.AddCheck("Config file", "pass", plan.ConfigFile)
	defer func() {
		if plan.Secrets != nil {
			report.AddCheck("Secrets", "pass", plan.SecretsFile)
		}
	}()

	stopInspecting := h.presenter.StartStatusActivity("Checking stored revisions")
	revisions, _, err := dup.ListVisibleRevisions()
	stopInspecting()
	if err != nil {
		if req.Command == "verify" {
			report.AddVerifyFailureCode(verifyFailureListingFailed)
		}
		report.AddCheck("Latest revision", "fail", OperatorMessage(err))
		return nil
	}
	report.RevisionCount = len(revisions)
	if len(revisions) == 0 {
		report.AddCheck("Revision count", "warn", "0")
		report.AddCheck("Latest revision", "warn", "No revisions were visible in storage")
		return nil
	}
	report.AddCheck("Revision count", "pass", fmt.Sprintf("%d", len(revisions)))
	latest := revisions[0]
	report.LatestRevision = latest.Revision
	if !latest.CreatedAt.IsZero() {
		report.LatestRevisionAt = formatReportTime(latest.CreatedAt)
		report.AddCheck("Latest revision", "pass", fmt.Sprintf("%d (%s)", latest.Revision, latest.CreatedAt.Format("2006-01-02 15:04:05")))
	} else {
		report.AddCheck("Latest revision", "pass", fmt.Sprintf("%d", latest.Revision))
	}

	if !latest.CreatedAt.IsZero() {
		h.evaluateFreshness(report, cfg.Health, latest.CreatedAt, "Backup freshness")
		return revisions
	}

	if state == nil {
		report.AddCheck("Backup freshness", "warn", "Latest revision time is unavailable and no backup state exists")
		return revisions
	}
	if state.LastSuccessfulBackupAt == "" {
		report.AddCheck("Backup freshness", "warn", "Latest revision time is unavailable and no backup timestamp is recorded")
		return revisions
	}

	if state.LastSuccessfulBackupRevision == latest.Revision {
		report.LatestRevisionAt = state.LastSuccessfulBackupAt
		if parsed, parseErr := time.Parse(time.RFC3339, state.LastSuccessfulBackupAt); parseErr == nil {
			h.evaluateFreshness(report, cfg.Health, parsed, "Backup freshness")
		} else {
			report.AddCheck("Backup freshness", "warn", "Recorded backup timestamp is invalid")
		}
		return revisions
	}

	report.AddCheck("Backup freshness", "warn", fmt.Sprintf("Latest revision %d does not match recorded backup revision %d", latest.Revision, state.LastSuccessfulBackupRevision))
	return revisions
}

func (h *HealthRunner) runDoctorChecks(report *HealthReport, req *HealthRequest, cfg *config.Config, plan *Plan, dup *duplicacy.Setup) {
	verifyMode := req.Command == "verify"
	if _, err := os.Stat(plan.SnapshotSource); err != nil {
		if verifyMode {
			report.AddDisplayCheck("Source path", "info", fmt.Sprintf("Backup-readiness check failed: source path is not accessible: %v", err))
		} else {
			report.AddCheck("Source path", "fail", fmt.Sprintf("Source path is not accessible: %v", err))
		}
	} else {
		report.AddCheck("Source path", "pass", plan.SnapshotSource)
	}
	if verifyMode {
		report.AddDisplayCheck("Btrfs", "info", "Not checked; not required for storage integrity verification")
	} else {
		readiness := h.runBtrfsReadinessChecks(plan)
		switch {
		case readiness.RootErr == nil && readiness.SourceErr == nil:
			report.AddCheck("Btrfs", "pass", "Yes")
		default:
			if readiness.RootErr != nil {
				report.AddCheck("Btrfs root", "fail", OperatorMessage(readiness.RootErr))
			}
			if readiness.SourceErr != nil {
				report.AddCheck("Btrfs source", "fail", OperatorMessage(readiness.SourceErr))
			}
		}
	}

	stopValidating := h.presenter.StartStatusActivity("Validating repository access")
	if err := dup.ValidateRepo(); err != nil {
		stopValidating()
		if req.Command == "verify" {
			report.AddVerifyFailureCode(verifyFailureAccessFailed)
		}
		report.AddCheck("Repository access", "fail", OperatorMessage(err))
	} else {
		stopValidating()
		report.AddCheck("Repository access", "pass", "Validated")
	}

	if req.Command == "doctor" {
		h.evaluateHealthRecency(report, cfg.Health, "doctor", "Last doctor run")
	}
}

type btrfsReadinessReport struct {
	RootErr   error
	SourceErr error
}

func (h *HealthRunner) runBtrfsReadinessChecks(plan *Plan) btrfsReadinessReport {
	return btrfsReadinessReport{
		RootErr:   btrfs.CheckVolume(h.runner, h.meta.RootVolume, false),
		SourceErr: btrfs.CheckVolume(h.runner, plan.SnapshotSource, false),
	}
}

func (h *HealthRunner) runVerifyChecks(report *HealthReport, cfg *config.Config, dup *duplicacy.Setup, revisions []duplicacy.RevisionInfo) {
	if report.HasVerifyFailureCode(verifyFailureListingFailed) {
		report.RevisionCount = 0
		report.CheckedRevisionCount = 0
		report.PassedRevisionCount = 0
		report.FailedRevisionCount = 0
		report.AddDisplayCheck("Revisions checked", "fail", "0")
		report.AddDisplayCheck("Revisions passed", "pass", "0")
		report.AddDisplayCheck("Revisions failed", "pass", "0")
		report.AddCheck("Integrity check", "fail", "Revision inspection failed")
		h.populateHealthRecencyTimestamp(report, "verify")
		return
	}

	if len(revisions) == 0 {
		report.AddVerifyFailureCode(verifyFailureNoRevisionsFound)
		report.AddDisplayCheck("Revisions checked", "fail", "0")
		report.AddDisplayCheck("Revisions passed", "pass", "0")
		report.AddDisplayCheck("Revisions failed", "pass", "0")
		report.AddCheck("Integrity check", "fail", "No revisions were found for this backup")
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}

	status := fmt.Sprintf("Checking revision integrity for this backup (%d total)", len(revisions))
	stopVerifying := h.presenter.StartStatusActivity(status)
	results, _, err := dup.CheckVisibleRevisions()
	stopVerifying()
	if err != nil {
		report.AddVerifyFailureCode(verifyFailureAccessFailed)
		report.CheckedRevisionCount = 0
		report.PassedRevisionCount = 0
		report.FailedRevisionCount = 0
		report.AddDisplayCheck("Revisions checked", "fail", "0")
		report.AddDisplayCheck("Revisions passed", "pass", "0")
		report.AddDisplayCheck("Revisions failed", "pass", "0")
		report.AddCheck("Integrity check", "fail", "Integrity check did not complete")
		h.populateHealthRecencyTimestamp(report, "verify")
		return
	}

	reconciled := healthpkg.ReconcileVerifyResults(healthVerifyRevisions(revisions), healthVerifyResults(results))
	report.CheckedRevisionCount = reconciled.CheckedRevisionCount
	report.PassedRevisionCount = reconciled.PassedRevisionCount
	report.FailedRevisionCount = reconciled.FailedRevisionCount
	report.FailedRevisions = append(report.FailedRevisions, reconciled.FailedRevisions...)
	report.RevisionResults = append(report.RevisionResults, reconciled.RevisionResults...)
	for _, code := range reconciled.FailureCodes {
		report.AddVerifyFailureCode(code)
	}

	verifiedResult := "pass"
	if len(reconciled.MissingRevisions) > 0 {
		verifiedResult = "fail"
	}
	report.AddDisplayCheck("Revisions checked", verifiedResult, fmt.Sprintf("%d", report.CheckedRevisionCount))
	report.AddDisplayCheck("Revisions passed", "pass", fmt.Sprintf("%d", report.PassedRevisionCount))
	failedResult := "pass"
	if report.FailedRevisionCount > 0 {
		failedResult = "fail"
	}
	failedSummary := fmt.Sprintf("%d", report.FailedRevisionCount)
	if report.FailedRevisionCount > 0 {
		failedSummary = fmt.Sprintf("%d (%s)", report.FailedRevisionCount, healthpkg.SummariseRevisionIDs(report.FailedRevisions, 4))
	}
	report.AddDisplayCheck("Revisions failed", failedResult, failedSummary)

	if len(reconciled.MissingRevisions) > 0 {
		report.AddCheck("Integrity check", "fail", healthpkg.IntegrityCheckFailureMessage(report.FailedRevisions, reconciled.MissingRevisions))
		for _, check := range reconciled.DetailChecks {
			report.AddDisplayCheck(check.Name, check.Result, check.Message)
		}
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}
	if report.FailedRevisionCount > 0 {
		report.AddCheck("Integrity check", "fail", healthpkg.IntegrityCheckFailureMessage(report.FailedRevisions, nil))
		for _, check := range reconciled.DetailChecks {
			report.AddDisplayCheck(check.Name, check.Result, check.Message)
		}
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}
	report.AddCheck("Integrity check", "pass", "All revisions validated")
	h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
}

func healthVerifyRevisions(revisions []duplicacy.RevisionInfo) []healthpkg.VerifyRevision {
	converted := make([]healthpkg.VerifyRevision, 0, len(revisions))
	for _, revision := range revisions {
		converted = append(converted, healthpkg.VerifyRevision{
			Revision:  revision.Revision,
			CreatedAt: revision.CreatedAt,
		})
	}
	return converted
}

func healthVerifyResults(results []duplicacy.RevisionCheckResult) []healthpkg.VerifyResult {
	converted := make([]healthpkg.VerifyResult, 0, len(results))
	for _, result := range results {
		converted = append(converted, healthpkg.VerifyResult{
			Revision: result.Revision,
			Result:   result.Result,
			Message:  result.Message,
		})
	}
	return converted
}
