package workflow

import (
	"fmt"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
)

func HandleNotifyCommand(req *Request, meta Metadata, rt Runtime) (string, error) {
	planner := NewPlanner(meta, rt, nil, newConfigCommandRunner())

	switch req.NotifyCommand {
	case "test":
		return handleNotifyTest(req, planner)
	default:
		return "", NewRequestError("unsupported notify command %q", req.NotifyCommand)
	}
}

func handleNotifyTest(req *Request, planner *Planner) (string, error) {
	plan := planner.derivePlan(configValidationRequest(req, req.Target()))
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}

	plan.Target = cfg.Target
	plan.StorageType = cfg.StorageType
	plan.Location = cfg.Location
	plan.Notify = cfg.Health.Notify
	plan.SecretsFile = secretsFileForPlan(plan)

	payload := buildTestNotificationPayload(planner.rt, cfg.Label, cfg.Target, cfg.StorageType, cfg.Location, req)
	destinations, err := notify.ConfiguredDestinations(cfg.Health.Notify, req.NotifyProvider)
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
		plan.SecretsFile,
		cfg.Target,
		payload,
		req.NotifyProvider,
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

func newNotifyTestReport(req *Request, cfg *config.Config, payload *notify.Payload, destinations []notify.Destination, result string) *notify.TestReport {
	return notify.NewTestReport(notify.TestReportInput{
		Command:     "test",
		Label:       cfg.Label,
		Target:      cfg.Target,
		StorageType: cfg.StorageType,
		Location:    cfg.Location,
		Provider:    req.NotifyProvider,
		Severity:    payload.Severity,
		Category:    payload.Category,
		Event:       payload.Event,
		Summary:     payload.Summary,
		Message:     notifyDetailsMessage(payload.Details),
		DryRun:      req.DryRun,
	}, destinations, result)
}

func secretsFileForPlan(plan *Plan) string {
	if plan == nil {
		return ""
	}
	return plan.SecretsFile
}
