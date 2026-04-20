package workflow

import (
	"fmt"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
)

func shouldSendConfiguredNotification(rt Runtime, interactive bool, cfg config.HealthNotifyConfig, sendFor string) bool {
	if !notify.HasDestination(cfg) {
		return false
	}
	if interactive && rt.StdinIsTTY() && !cfg.Interactive {
		return false
	}
	if sendFor == "" {
		return true
	}
	return containsString(cfg.SendFor, sendFor)
}

func buildTestNotificationPayload(rt Runtime, label, target, location string, req *Request) *notify.Payload {
	return notify.BuildTestPayload(
		rt.Now(),
		rt.Getpid(),
		label,
		target,
		location,
		req.NotifySeverity,
		req.NotifySummary,
		req.NotifyMessage,
	)
}

func buildRuntimeNotificationPayload(rt Runtime, plan *Plan, report *RunReport, err error, visibleRunStarted bool, preview *duplicacy.PrunePreview) *notify.Payload {
	if plan == nil || report == nil || err == nil {
		return nil
	}
	if isInteractiveCancellation(err) {
		return nil
	}

	details := map[string]any{
		"code":    report.ExitCode,
		"message": OperatorMessage(err),
	}
	if report.DurationSecond > 0 {
		details["duration_seconds"] = report.DurationSecond
	}

	if !visibleRunStarted && plan.DoBackup {
		return notify.NewPayload(rt.Now(), rt.Getpid(), "critical", "backup", "backup_could_not_start",
			fmt.Sprintf("Backup could not start for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"backup", "", "failed", details,
		)
	}

	switch lastFailedPhaseName(report) {
	case "Backup":
		return notify.NewPayload(rt.Now(), rt.Getpid(), "critical", "backup", "backup_failed",
			fmt.Sprintf("Backup failed for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"backup", "", "failed", details,
		)
	case "Prune":
		if isSafePruneBlocked(err) {
			if preview != nil {
				details["preview_deletes"] = preview.DeleteCount
				details["preview_total_revisions"] = preview.TotalRevisions
				if preview.TotalRevisions > 0 {
					details["delete_percent"] = preview.DeletePercent
				}
				details["max_delete_percent"] = plan.SafePruneMaxDeletePercent
				details["max_delete_count"] = plan.SafePruneMaxDeleteCount
			}
			return notify.NewPayload(rt.Now(), rt.Getpid(), "warning", "maintenance", "safe_prune_blocked",
				fmt.Sprintf("Safe prune blocked for %s/%s", report.Label, report.Target),
				report.Label, report.Target, report.Location,
				"prune", "", "blocked", details,
			)
		}
		return notify.NewPayload(rt.Now(), rt.Getpid(), "warning", "maintenance", "prune_failed",
			fmt.Sprintf("Prune failed for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"prune", "", "failed", details,
		)
	case "Storage cleanup":
		return notify.NewPayload(rt.Now(), rt.Getpid(), "warning", "maintenance", "cleanup_failed",
			fmt.Sprintf("Storage cleanup failed for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"cleanup_storage", "", "failed", details,
		)
	default:
		return nil
	}
}

func buildHealthNotificationPayload(rt Runtime, report *HealthReport) *notify.Payload {
	if report == nil {
		return nil
	}

	if report.CheckType == "verify" && report.FailedRevisionCount > 0 {
		return notify.NewPayload(rt.Now(), rt.Getpid(), "critical", "health", "verify_failed_revisions",
			fmt.Sprintf("Verify found failed revisions for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"", report.CheckType, report.Status,
			healthNotificationDetails(report, map[string]any{
				"failed_revision_count": report.FailedRevisionCount,
				"failed_revisions":      append([]int(nil), report.FailedRevisions...),
				"message":               healthCheckMessage(report, "Integrity check"),
			}),
		)
	}

	if result, message, ok := healthCheckResult(report, "Backup freshness"); ok && result == "fail" {
		return notify.NewPayload(rt.Now(), rt.Getpid(), "critical", "health", "freshness_failed",
			fmt.Sprintf("Freshness failure for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"", report.CheckType, report.Status,
			healthNotificationDetails(report, map[string]any{
				"message":   message,
				"freshness": message,
			}),
		)
	}

	switch report.Status {
	case "degraded":
		return notify.NewPayload(rt.Now(), rt.Getpid(), "warning", "health", "health_degraded",
			fmt.Sprintf("Health degraded for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"", report.CheckType, report.Status,
			healthNotificationDetails(report, map[string]any{
				"message": firstHealthIssueMessage(report),
			}),
		)
	case "unhealthy":
		return notify.NewPayload(rt.Now(), rt.Getpid(), "critical", "health", "health_unhealthy",
			fmt.Sprintf("Health unhealthy for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"", report.CheckType, report.Status,
			healthNotificationDetails(report, map[string]any{
				"message": firstHealthIssueMessage(report),
			}),
		)
	default:
		return nil
	}
}

func healthNotificationDetails(report *HealthReport, details map[string]any) map[string]any {
	if details == nil {
		details = make(map[string]any)
	}
	if report == nil {
		return details
	}
	if report.FailureCode != "" {
		details["failure_code"] = report.FailureCode
	}
	if len(report.FailureCodes) > 0 {
		details["failure_codes"] = append([]string(nil), report.FailureCodes...)
	}
	if len(report.RecommendedActions) > 0 {
		details["recommended_action_codes"] = append([]string(nil), report.RecommendedActions...)
	}
	return details
}

func lastFailedPhaseName(report *RunReport) string {
	if report == nil {
		return ""
	}
	for i := len(report.Phases) - 1; i >= 0; i-- {
		if report.Phases[i].Result == "failed" {
			return report.Phases[i].Name
		}
	}
	return ""
}

func healthCheckResult(report *HealthReport, name string) (result string, message string, ok bool) {
	if report == nil {
		return "", "", false
	}
	for _, check := range report.Checks {
		if check.Name == name {
			return check.Result, check.Message, true
		}
	}
	return "", "", false
}

func healthCheckMessage(report *HealthReport, name string) string {
	_, message, _ := healthCheckResult(report, name)
	return message
}

func firstHealthIssueMessage(report *HealthReport) string {
	if report == nil {
		return ""
	}
	for _, issue := range report.Issues {
		if strings.TrimSpace(issue.Message) != "" {
			return issue.Message
		}
	}
	return ""
}

func isSafePruneBlocked(err error) bool {
	return OperatorMessage(err) == "Refusing to continue because safe prune thresholds were exceeded"
}

func isInteractiveCancellation(err error) bool {
	return OperatorMessage(err) == "Operation cancelled at the interactive safety prompt"
}

func firstFailedNotificationResult(results []notify.DeliveryResult) string {
	return notify.FirstFailedResult(results)
}

func notifyDetailsMessage(details map[string]any) string {
	return notify.DetailsMessage(details)
}
