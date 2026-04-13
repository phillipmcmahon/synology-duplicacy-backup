package workflow

import "time"

func (e *Executor) maybeSendFailureWebhook() {
	if e == nil || e.plan == nil || e.report == nil || e.lastErr == nil {
		return
	}
	if e.plan.DryRun {
		return
	}

	sendFor := runtimeWebhookSendFor(e.report, e.visibleRunStarted)
	if !shouldSendConfiguredWebhook(e.rt, e.log.Interactive(), e.plan.Notify, sendFor) {
		return
	}

	payload := buildRuntimeWebhookPayload(e.rt, e.plan, e.report, e.lastErr, e.visibleRunStarted, e.lastPrunePreview)
	if payload == nil {
		return
	}
	if err := sendWebhookPayload(e.plan.Notify, e.plan.SecretsFile, e.report.Target, payload); err != nil {
		e.log.Warn("%s", statusLinef("Failed to deliver webhook notification: %v", err))
	}
}

func runtimeWebhookSendFor(report *RunReport, visibleRunStarted bool) string {
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

func MaybeSendPreRunFailureWebhook(rt Runtime, interactive bool, plan *Plan, req *Request, startedAt, completedAt time.Time, err error) error {
	if plan == nil || req == nil || err == nil || !req.DoBackup {
		return nil
	}
	if plan.DryRun {
		return nil
	}

	report := NewFailureRunReport(req, plan, startedAt, completedAt, 1, OperatorMessage(err))
	sendFor := runtimeWebhookSendFor(report, false)
	if !shouldSendConfiguredWebhook(rt, interactive, plan.Notify, sendFor) {
		return nil
	}

	payload := buildRuntimeWebhookPayload(rt, plan, report, err, false, nil)
	return sendWebhookPayload(plan.Notify, plan.SecretsFile, report.Target, payload)
}
