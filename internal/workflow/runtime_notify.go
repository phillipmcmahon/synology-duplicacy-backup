package workflow

import (
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
)

func (e *Executor) maybeSendFailureNotification() {
	if e == nil || e.plan == nil || e.report == nil || e.lastErr == nil {
		return
	}
	if e.plan.DryRun {
		return
	}

	sendFor := runtimeNotificationSendFor(e.report, e.visibleRunStarted)
	if !shouldSendConfiguredNotification(e.rt, e.log.Interactive(), e.plan.Notify, sendFor) {
		return
	}

	payload := buildRuntimeNotificationPayload(e.rt, e.plan, e.report, e.lastErr, e.visibleRunStarted, e.lastPrunePreview)
	if payload == nil {
		return
	}
	if err := notify.SendConfigured(e.plan.Notify, e.plan.SecretsFile, e.report.Target, payload); err != nil {
		e.log.Warn("%s", statusLinef("Failed to deliver notification: %v", OperatorMessage(err)))
	}
}

func runtimeNotificationSendFor(report *RunReport, visibleRunStarted bool) string {
	if report == nil {
		return ""
	}
	if !visibleRunStarted && report.Operation == "Backup" {
		return "backup"
	}
	switch lastFailedPhaseName(report) {
	case "Backup":
		return "backup"
	case "Prune":
		return "prune"
	case "Storage cleanup":
		return "cleanup-storage"
	default:
		return ""
	}
}

func MaybeSendPreRunFailureNotification(rt Runtime, interactive bool, plan *Plan, req *RuntimeRequest, startedAt, completedAt time.Time, err error) error {
	if plan == nil || req == nil || err == nil || !req.DoBackup() {
		return nil
	}
	if plan.DryRun {
		return nil
	}

	report := NewFailureRunReport(req, plan, startedAt, completedAt, 1, OperatorMessage(err))
	sendFor := runtimeNotificationSendFor(report, false)
	if !shouldSendConfiguredNotification(rt, interactive, plan.Notify, sendFor) {
		return nil
	}

	payload := buildRuntimeNotificationPayload(rt, plan, report, err, false, nil)
	return notify.SendConfigured(plan.Notify, plan.SecretsFile, report.Target, payload)
}
