package health

import (
	"fmt"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func (h *HealthRunner) shouldSendNotification(req *HealthRequest, cfg config.HealthConfig, status string) bool {
	if req == nil {
		return false
	}
	if !shouldSendConfiguredNotification(h.rt, h.log.Interactive(), cfg.Notify, req.Command) {
		return false
	}
	if !containsString(cfg.Notify.NotifyOn, status) {
		return false
	}
	return true
}

func (h *HealthRunner) maybeSendEarlyNotification(req *HealthRequest, report *Report) {
	if req == nil || report == nil {
		return
	}
	cfg, secretsFile, ok := h.loadHealthNotifyConfig(req)
	if !ok || !h.shouldSendNotification(req, cfg, report.Status) {
		return
	}
	payload := buildHealthNotificationPayload(h.rt, report)
	if payload == nil {
		return
	}
	if err := notify.SendConfigured(cfg.Notify, secretsFile, report.Target, payload); err != nil {
		report.AddCheck("Notification", "warn", workflow.OperatorMessage(err))
		return
	}
	report.NotificationSent = true
	report.AddCheck("Notification", "pass", "Delivered")
}

func (h *HealthRunner) loadHealthNotifyConfig(req *HealthRequest) (config.HealthConfig, string, bool) {
	if req == nil || req.Label == "" {
		return config.HealthConfig{}, "", false
	}

	planner := workflow.NewPlanner(h.meta, h.rt, h.log, h.runner)
	plan := planner.DeriveConfigPlan(req.PlanRequest())
	cfg, err := planner.LoadConfig(plan)
	if err != nil {
		return config.HealthConfig{}, "", false
	}
	if err := cfg.Health.Validate(); err != nil {
		return config.HealthConfig{}, "", false
	}
	return cfg.Health, plan.Paths.SecretsFile, true
}

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

func buildHealthNotificationPayload(rt Env, report *Report) *notify.Payload {
	if report == nil {
		return nil
	}

	if report.CheckType == "verify" && report.FailedRevisionCount > 0 {
		return notify.NewPayload(rt.Now(), rt.Getpid(), "critical", "health", string(notify.EventVerifyFailedRevisions),
			fmt.Sprintf("Verify found failed revisions for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"", report.CheckType, report.Status,
			healthNotificationDetails(report, map[string]any{
				"failed_revision_count": report.FailedRevisionCount,
				"failed_revisions":      append([]int(nil), report.FailedRevisions...),
				"message":               CheckMessage(report, "Integrity check"),
			}),
		)
	}

	if result, message, ok := CheckResult(report, "Backup freshness"); ok && result == "fail" {
		return notify.NewPayload(rt.Now(), rt.Getpid(), "critical", "health", string(notify.EventFreshnessFailed),
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
		return notify.NewPayload(rt.Now(), rt.Getpid(), "warning", "health", string(notify.EventHealthDegraded),
			fmt.Sprintf("Health degraded for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"", report.CheckType, report.Status,
			healthNotificationDetails(report, map[string]any{
				"message": FirstIssueMessage(report),
			}),
		)
	case "unhealthy":
		return notify.NewPayload(rt.Now(), rt.Getpid(), "critical", "health", string(notify.EventHealthUnhealthy),
			fmt.Sprintf("Health unhealthy for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.Location,
			"", report.CheckType, report.Status,
			healthNotificationDetails(report, map[string]any{
				"message": FirstIssueMessage(report),
			}),
		)
	default:
		return nil
	}
}

func healthNotificationDetails(report *Report, details map[string]any) map[string]any {
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
