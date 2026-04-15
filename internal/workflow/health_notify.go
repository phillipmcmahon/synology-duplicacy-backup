package workflow

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
)

func (h *HealthRunner) shouldSendNotification(req *Request, cfg config.HealthConfig, status string) bool {
	if req == nil {
		return false
	}
	if !shouldSendConfiguredNotification(h.rt, h.log.Interactive(), cfg.Notify, req.HealthCommand) {
		return false
	}
	if !containsString(cfg.Notify.NotifyOn, status) {
		return false
	}
	return true
}

func (h *HealthRunner) maybeSendEarlyNotification(req *Request, report *HealthReport) {
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
		report.AddCheck("Notification", "warn", OperatorMessage(err))
		return
	}
	report.NotificationSent = true
	report.AddCheck("Notification", "pass", "Delivered")
}

func (h *HealthRunner) loadHealthNotifyConfig(req *Request) (config.HealthConfig, string, bool) {
	if req == nil || req.Source == "" {
		return config.HealthConfig{}, "", false
	}

	planner := NewPlanner(h.meta, h.rt, h.log, h.runner)
	plan := planner.derivePlan(configValidationRequest(req, req.Target()))
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return config.HealthConfig{}, "", false
	}
	if err := cfg.Health.Validate(); err != nil {
		return config.HealthConfig{}, "", false
	}
	return cfg.Health, plan.SecretsFile, true
}
