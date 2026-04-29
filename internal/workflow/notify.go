package workflow

import (
	"fmt"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
)

func shouldSendConfiguredNotification(rt Env, interactive bool, cfg config.HealthNotifyConfig, sendFor string) bool {
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

func buildTestNotificationPayload(rt Env, label, target, location string, req *NotifyRequest) *notify.Payload {
	return notify.BuildTestPayload(
		rt.Now(),
		rt.Getpid(),
		label,
		target,
		location,
		req.Severity,
		req.Summary,
		req.Message,
	)
}

func buildRuntimeNotificationPayload(rt Env, plan *Plan, report *RunReport, err error, visibleRunStarted bool, preview *duplicacy.PrunePreview) *notify.Payload {
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

	if !visibleRunStarted && plan.Request.DoBackup {
		return notify.NewPayload(rt.Now(), rt.Getpid(), "critical", "backup", string(notify.EventBackupCouldNotStart),
			fmt.Sprintf("Backup could not start for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"backup", "", "failed", details,
		)
	}

	switch lastFailedPhaseName(report) {
	case "Backup":
		return notify.NewPayload(rt.Now(), rt.Getpid(), "critical", "backup", string(notify.EventBackupFailed),
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
				details["max_delete_percent"] = plan.Config.SafePruneMaxDeletePercent
				details["max_delete_count"] = plan.Config.SafePruneMaxDeleteCount
			}
			return notify.NewPayload(rt.Now(), rt.Getpid(), "warning", "maintenance", string(notify.EventSafePruneBlocked),
				fmt.Sprintf("Safe prune blocked for %s/%s", report.Label, report.Target),
				report.Label, report.Target, report.Location,
				"prune", "", "blocked", details,
			)
		}
		return notify.NewPayload(rt.Now(), rt.Getpid(), "warning", "maintenance", string(notify.EventPruneFailed),
			fmt.Sprintf("Prune failed for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"prune", "", "failed", details,
		)
	case "Storage cleanup":
		return notify.NewPayload(rt.Now(), rt.Getpid(), "warning", "maintenance", string(notify.EventCleanupFailed),
			fmt.Sprintf("Storage cleanup failed for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"cleanup_storage", "", "failed", details,
		)
	default:
		return nil
	}
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

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
