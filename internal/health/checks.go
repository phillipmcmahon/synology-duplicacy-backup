package health

import (
	"fmt"
	"os"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/operator"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func (h *HealthRunner) runStatusChecks(report *Report, req *HealthRequest, cfg *config.Config, plan *Plan, state *RunState, dup *duplicacy.Setup) []duplicacy.RevisionInfo {
	report.AddCheck("Config file", "pass", plan.Paths.ConfigFile)
	defer func() {
		if plan.Secrets != nil {
			report.AddCheck("Secrets", "pass", plan.Paths.SecretsFile)
		}
	}()

	stopInspecting := h.presenter.StartStatusActivity("Checking stored revisions")
	revisions, _, err := dup.ListVisibleRevisions()
	stopInspecting()
	if err != nil {
		if req.Command == "verify" {
			report.AddVerifyFailureCode(verifyFailureListingFailed)
		}
		report.AddCheck("Latest revision", "fail", operator.Message(err))
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

func (h *HealthRunner) runLocalRepositorySudoStatusChecks(report *Report, req *HealthRequest, plan *Plan) {
	report.AddCheck("Config file", "pass", plan.Paths.ConfigFile)
	if plan.Secrets != nil {
		report.AddCheck("Secrets", "pass", plan.Paths.SecretsFile)
	}
	// Doctor and verify add the repository-access row in runDoctorChecks,
	// after reporting the source-path and Btrfs readiness context.
	if req.Command == "status" {
		report.AddCheck("Repository access", "fail", localRepositoryHealthSudoMessage())
	}
}

func (h *HealthRunner) runDoctorChecks(report *Report, req *HealthRequest, cfg *config.Config, plan *Plan, dup *duplicacy.Setup) {
	verifyMode := req.Command == "verify"
	if _, err := os.Stat(plan.Paths.SnapshotSource); err != nil {
		if verifyMode {
			report.AddDisplayCheck("Source path", "info", fmt.Sprintf("Backup-readiness check failed: source path is not accessible: %v", err))
		} else {
			report.AddCheck("Source path", "fail", fmt.Sprintf("Source path is not accessible: %v", err))
		}
	} else {
		report.AddCheck("Source path", "pass", plan.Paths.SnapshotSource)
	}
	if verifyMode {
		report.AddDisplayCheck("Btrfs", "info", "Not checked; backup-readiness validation is not required for storage integrity verification")
	} else {
		readiness := h.runBtrfsReadinessChecks(plan)
		switch {
		case readiness.RootErr == nil && readiness.SourceErr == nil:
			report.AddCheck("Btrfs", "pass", "Yes")
		default:
			if readiness.RootErr != nil {
				report.AddCheck("Btrfs root", "fail", operator.Message(readiness.RootErr))
			}
			if readiness.SourceErr != nil {
				report.AddCheck("Btrfs source", "fail", operator.Message(readiness.SourceErr))
			}
		}
	}

	if req.Command == "doctor" {
		h.evaluateHealthRecency(report, cfg.Health, "doctor", "Last doctor run")
	}

	stopValidating := h.presenter.StartStatusActivity("Validating repository access")
	if localRepositoryRequiresSudo(cfg, h.rt) {
		stopValidating()
		if req.Command == "verify" {
			report.AddVerifyFailureCode(verifyFailureAccessFailed)
		}
		report.AddCheck("Repository access", "fail", localRepositoryHealthSudoMessage())
	} else if err := dup.ValidateRepo(); err != nil {
		stopValidating()
		if req.Command == "verify" {
			report.AddVerifyFailureCode(verifyFailureAccessFailed)
		}
		report.AddCheck("Repository access", "fail", operator.Message(err))
	} else {
		stopValidating()
		report.AddCheck("Repository access", "pass", "Validated")
	}
}

func localRepositoryHealthSudoMessage() string {
	return presentation.LocalRepositoryRequiresSudoMessage("")
}

func localRepositoryRequiresSudo(cfg *config.Config, rt Env) bool {
	return cfg != nil && cfg.UsesRootProtectedLocalRepository() && workflow.EnvEUID(rt) != 0
}

type btrfsReadinessReport struct {
	RootErr   error
	SourceErr error
}

func (h *HealthRunner) runBtrfsReadinessChecks(plan *Plan) btrfsReadinessReport {
	return btrfsReadinessReport{
		RootErr:   btrfs.CheckVolume(h.runner, h.meta.RootVolume, false),
		SourceErr: btrfs.CheckVolume(h.runner, plan.Paths.SnapshotSource, false),
	}
}

func (h *HealthRunner) runVerifyChecks(report *Report, cfg *config.Config, dup *duplicacy.Setup, revisions []duplicacy.RevisionInfo) {
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

	if localRepositoryRequiresSudo(cfg, h.rt) {
		report.CheckedRevisionCount = 0
		report.PassedRevisionCount = 0
		report.FailedRevisionCount = 0
		report.AddDisplayCheck("Revisions checked", "fail", "0")
		report.AddDisplayCheck("Revisions passed", "pass", "0")
		report.AddDisplayCheck("Revisions failed", "pass", "0")
		report.AddCheck("Integrity check", "fail", presentation.LocalRepositoryRequiresSudoMessage("verification"))
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

	reconciled := ReconcileVerifyResults(healthVerifyRevisions(revisions), healthVerifyResults(results))
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
		failedSummary = fmt.Sprintf("%d (%s)", report.FailedRevisionCount, SummariseRevisionIDs(report.FailedRevisions, 4))
	}
	report.AddDisplayCheck("Revisions failed", failedResult, failedSummary)

	if len(reconciled.MissingRevisions) > 0 {
		report.AddCheck("Integrity check", "fail", IntegrityCheckFailureMessage(report.FailedRevisions, reconciled.MissingRevisions))
		for _, check := range reconciled.DetailChecks {
			report.AddDisplayCheck(check.Name, check.Result, check.Message)
		}
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}
	if report.FailedRevisionCount > 0 {
		report.AddCheck("Integrity check", "fail", IntegrityCheckFailureMessage(report.FailedRevisions, nil))
		for _, check := range reconciled.DetailChecks {
			report.AddDisplayCheck(check.Name, check.Result, check.Message)
		}
		h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
		return
	}
	report.AddCheck("Integrity check", "pass", "All revisions validated")
	h.evaluateHealthRecency(report, cfg.Health, "verify", "Last verify run")
}

func healthVerifyRevisions(revisions []duplicacy.RevisionInfo) []VerifyRevision {
	converted := make([]VerifyRevision, 0, len(revisions))
	for _, revision := range revisions {
		converted = append(converted, VerifyRevision{
			Revision:  revision.Revision,
			CreatedAt: revision.CreatedAt,
		})
	}
	return converted
}

func healthVerifyResults(results []duplicacy.RevisionCheckResult) []VerifyResult {
	converted := make([]VerifyResult, 0, len(results))
	for _, result := range results {
		converted = append(converted, VerifyResult{
			Revision: result.Revision,
			Result:   result.Result,
			Message:  result.Message,
		})
	}
	return converted
}
