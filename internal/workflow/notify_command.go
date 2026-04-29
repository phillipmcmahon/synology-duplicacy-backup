package workflow

import (
	"fmt"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
)

func HandleNotifyCommand(req *Request, meta Metadata, rt Env) (string, error) {
	planner := NewPlanner(meta, rt, nil, newConfigCommandRunner())
	notifyReq := NewNotifyRequest(req)

	switch notifyReq.Command {
	case "test":
		return handleNotifyTest(&notifyReq, planner)
	default:
		return "", NewRequestError("unsupported notify command %q", notifyReq.Command)
	}
}

func handleNotifyTest(req *NotifyRequest, planner *Planner) (string, error) {
	if req.Scope == "update" {
		return handleUpdateNotifyTest(req, planner.meta, planner.rt)
	}

	plan := planner.derivePlan(req.PlanRequest())
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}

	plan.Config.Target = cfg.Target
	plan.Config.Location = cfg.Location
	plan.Config.Notify = cfg.Health.Notify
	plan.Paths.SecretsFile = secretsFileForPlan(plan)

	payload := buildTestNotificationPayload(planner.rt, cfg.Label, cfg.Target, cfg.Location, req)
	destinations, err := notify.ConfiguredDestinations(cfg.Health.Notify, req.Provider)
	if err != nil {
		report := newNotifyTestReport(req, cfg, payload, nil, "failed")
		output := notify.FormatTestOutput(report, req.JSONSummary)
		return output, &notify.CommandError{
			Message: fmt.Sprintf("Notification test failed for %s/%s; %s", cfg.Label, cfg.Target, OperatorMessage(err)),
			Output:  output,
		}
	}

	report := newNotifyTestReport(req, cfg, payload, destinations, "")
	if req.DryRun {
		for i := range report.Providers {
			report.Providers[i].Result = "preview"
			report.Providers[i].Message = "Would send a simulated notification"
		}
		report.Result = "preview"
		return notify.FormatTestOutput(report, req.JSONSummary), nil
	}

	results, sendErr := notify.SendConfiguredDetailedWithOptions(
		cfg.Health.Notify,
		plan.Paths.SecretsFile,
		cfg.Target,
		payload,
		req.Provider,
		notify.SendOptions{IgnoreOptionalAuthLoadErrors: true},
	)
	report.Providers = results
	if sendErr != nil {
		report.Result = "failed"
		output := notify.FormatTestOutput(report, req.JSONSummary)
		message := fmt.Sprintf("Notification test failed for %s/%s", cfg.Label, cfg.Target)
		if failure := firstFailedNotificationResult(results); failure != "" {
			message = fmt.Sprintf("%s; %s", message, OperatorMessage(fmt.Errorf("%s", failure)))
		}
		return output, &notify.CommandError{
			Message: message,
			Output:  output,
		}
	}

	report.Result = "success"
	return notify.FormatTestOutput(report, req.JSONSummary), nil
}

func handleUpdateNotifyTest(req *NotifyRequest, meta Metadata, rt Env) (string, error) {
	cfg, configPath, ok, err := loadUpdateNotifyConfig(req.ConfigDir, rt)
	if err != nil {
		return "", err
	}
	payload := BuildUpdateTestNotificationPayload(req, meta, rt)
	if !ok {
		report := newUpdateNotifyTestReport(req, payload, nil, "failed")
		output := notify.FormatTestOutput(report, req.JSONSummary)
		return output, &notify.CommandError{
			Message: fmt.Sprintf("Notification test failed for update; update notification config not found: %s", configPath),
			Output:  output,
		}
	}

	destinations, err := notify.ConfiguredDestinationsForScope(cfg, req.Provider, updateNotifyScope)
	if err != nil {
		report := newUpdateNotifyTestReport(req, payload, nil, "failed")
		output := notify.FormatTestOutput(report, req.JSONSummary)
		return output, &notify.CommandError{
			Message: fmt.Sprintf("Notification test failed for update; %s", OperatorMessage(err)),
			Output:  output,
		}
	}

	report := newUpdateNotifyTestReport(req, payload, destinations, "")
	if req.DryRun {
		for i := range report.Providers {
			report.Providers[i].Result = "preview"
			report.Providers[i].Message = "Would send a simulated update notification"
		}
		report.Result = "preview"
		return notify.FormatTestOutput(report, req.JSONSummary), nil
	}

	results, sendErr := notify.SendConfiguredDetailedWithOptions(
		cfg,
		"",
		"",
		payload,
		req.Provider,
		notify.SendOptions{IgnoreOptionalAuthLoadErrors: true},
	)
	report.Providers = results
	if sendErr != nil {
		report.Result = "failed"
		output := notify.FormatTestOutput(report, req.JSONSummary)
		message := "Notification test failed for update"
		if failure := firstFailedNotificationResult(results); failure != "" {
			message = fmt.Sprintf("%s; %s", message, OperatorMessage(fmt.Errorf("%s", failure)))
		}
		return output, &notify.CommandError{
			Message: message,
			Output:  output,
		}
	}

	report.Result = "success"
	return notify.FormatTestOutput(report, req.JSONSummary), nil
}

func newNotifyTestReport(req *NotifyRequest, cfg *config.Config, payload *notify.Payload, destinations []notify.Destination, result string) *notify.TestReport {
	return notify.NewTestReport(notify.TestReportInput{
		Command:  "test",
		Label:    cfg.Label,
		Target:   cfg.Target,
		Location: cfg.Location,
		Provider: req.Provider,
		Severity: payload.Severity,
		Category: payload.Category,
		Event:    payload.Event,
		Summary:  payload.Summary,
		Message:  notifyDetailsMessage(payload.Details),
		DryRun:   req.DryRun,
	}, destinations, result)
}

func newUpdateNotifyTestReport(req *NotifyRequest, payload *notify.Payload, destinations []notify.Destination, result string) *notify.TestReport {
	return notify.NewTestReport(notify.TestReportInput{
		Command:  "test",
		Scope:    "update",
		Provider: req.Provider,
		Severity: payload.Severity,
		Category: payload.Category,
		Event:    payload.Event,
		Summary:  payload.Summary,
		Message:  notifyDetailsMessage(payload.Details),
		DryRun:   req.DryRun,
	}, destinations, result)
}

func secretsFileForPlan(plan *Plan) string {
	if plan == nil {
		return ""
	}
	return plan.Paths.SecretsFile
}
