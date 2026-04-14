package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
)

type NotifyCommandError struct {
	Message string
	Output  string
}

func (e *NotifyCommandError) Error() string {
	return e.Message
}

func NotifyCommandOutput(err error) string {
	var notifyErr *NotifyCommandError
	if errors.As(err, &notifyErr) {
		return notifyErr.Output
	}
	return ""
}

type NotifyTestReport struct {
	Command     string                       `json:"command"`
	Label       string                       `json:"label"`
	Target      string                       `json:"target"`
	StorageType string                       `json:"storage_type,omitempty"`
	Location    string                       `json:"location,omitempty"`
	Provider    string                       `json:"provider"`
	Severity    string                       `json:"severity"`
	Category    string                       `json:"category"`
	Event       string                       `json:"event"`
	Summary     string                       `json:"summary"`
	Message     string                       `json:"message,omitempty"`
	DryRun      bool                         `json:"dry_run"`
	Result      string                       `json:"result"`
	Providers   []NotificationDeliveryResult `json:"providers,omitempty"`
}

func NewFailureNotifyTestReport(req *Request, message string) *NotifyTestReport {
	report := &NotifyTestReport{
		Command:  "test",
		Provider: "all",
		Severity: "warning",
		Category: "test",
		Event:    "notification_test",
		Summary:  "Notification test failed",
		Message:  message,
		DryRun:   req != nil && req.DryRun,
		Result:   "failed",
	}
	if req != nil {
		report.Command = fallbackNotifyValue(req.NotifyCommand, "test")
		report.Label = req.Source
		report.Target = req.Target()
		report.Provider = fallbackNotifyValue(req.NotifyProvider, "all")
		report.Severity = fallbackNotifyValue(req.NotifySeverity, "warning")
		if req.NotifySummary != "" {
			report.Summary = req.NotifySummary
		}
		if req.NotifyMessage != "" {
			report.Message = req.NotifyMessage
		}
	}
	return report
}

func WriteNotifyTestReport(w io.Writer, report *NotifyTestReport) error {
	if report == nil {
		return nil
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(report)
}

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

	destinations, err := configuredNotificationDestinations(cfg.Health.Notify, req.NotifyProvider)
	if err != nil {
		report := newNotifyTestReport(req, cfg, buildTestNotificationPayload(planner.rt, cfg.Label, cfg.Target, cfg.StorageType, cfg.Location, req), nil, "failed")
		return notifyOutput(report, req.JSONSummary), &NotifyCommandError{
			Message: fmt.Sprintf("Notification test failed for %s/%s; %s", cfg.Label, cfg.Target, OperatorMessage(err)),
			Output:  notifyOutput(report, req.JSONSummary),
		}
	}

	payload := buildTestNotificationPayload(planner.rt, cfg.Label, cfg.Target, cfg.StorageType, cfg.Location, req)
	report := newNotifyTestReport(req, cfg, payload, destinations, "")

	if req.DryRun {
		for i := range report.Providers {
			report.Providers[i].Result = "preview"
			report.Providers[i].Message = "Would send a simulated notification"
		}
		report.Result = "preview"
		return notifyOutput(report, req.JSONSummary), nil
	}

	results, sendErr := sendConfiguredNotificationsDetailedWithOptions(
		cfg.Health.Notify,
		plan.SecretsFile,
		cfg.Target,
		payload,
		req.NotifyProvider,
		notificationSendOptions{IgnoreOptionalAuthLoadErrors: true},
	)
	report.Providers = results
	if sendErr != nil {
		report.Result = "failed"
		output := notifyOutput(report, req.JSONSummary)
		message := fmt.Sprintf("Notification test failed for %s/%s", cfg.Label, cfg.Target)
		if failure := firstFailedNotificationResult(results); failure != "" {
			message = fmt.Sprintf("%s; %s", message, failure)
		}
		return output, &NotifyCommandError{
			Message: message,
			Output:  output,
		}
	}

	report.Result = "success"
	return notifyOutput(report, req.JSONSummary), nil
}

func newNotifyTestReport(req *Request, cfg *config.Config, payload *NotificationPayload, destinations []configuredNotificationDestination, result string) *NotifyTestReport {
	report := &NotifyTestReport{
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
		Result:      result,
	}
	for _, destination := range destinations {
		report.Providers = append(report.Providers, NotificationDeliveryResult{
			Provider:    destination.Provider,
			Destination: destination.Destination,
		})
	}
	return report
}

func notifyOutput(report *NotifyTestReport, jsonSummary bool) string {
	if jsonSummary {
		return formatNotifyTestJSON(report)
	}
	return formatNotifyTestOutput(report)
}

func formatNotifyTestJSON(report *NotifyTestReport) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(report)
	return b.String()
}

func formatNotifyTestOutput(report *NotifyTestReport) string {
	lines := []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Target", Value: report.Target},
		{Label: "Type", Value: report.StorageType},
		{Label: "Location", Value: report.Location},
		{Label: "Provider", Value: report.Provider},
		{Label: "Severity", Value: report.Severity},
		{Label: "Category", Value: report.Category},
		{Label: "Event", Value: report.Event},
		{Label: "Summary", Value: report.Summary},
	}
	if report.Message != "" {
		lines = append(lines, SummaryLine{Label: "Message", Value: report.Message})
	}
	lines = append(lines, SummaryLine{Label: "Dry Run", Value: fmt.Sprintf("%t", report.DryRun)})

	var providerLines []SummaryLine
	for _, provider := range report.Providers {
		label := strings.Title(provider.Provider)
		value := provider.Result
		if provider.Message != "" {
			value = fmt.Sprintf("%s (%s)", value, provider.Message)
		}
		if provider.Destination != "" {
			value = fmt.Sprintf("%s -> %s", value, provider.Destination)
		}
		providerLines = append(providerLines, SummaryLine{Label: label, Value: value})
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Notification test for %s/%s\n", report.Label, report.Target))
	for _, line := range lines {
		fmt.Fprintf(&b, "  %-20s : %s\n", line.Label, line.Value)
	}
	if len(providerLines) > 0 {
		b.WriteString("  Section: Providers\n")
		for _, line := range providerLines {
			fmt.Fprintf(&b, "    %-18s : %s\n", line.Label, line.Value)
		}
	}
	result := report.Result
	if result == "" {
		result = "unknown"
	}
	fmt.Fprintf(&b, "  %-20s : %s\n", "Result", strings.Title(result))
	return b.String()
}

func secretsFileForPlan(plan *Plan) string {
	if plan == nil {
		return ""
	}
	return plan.SecretsFile
}

func firstFailedNotificationResult(results []NotificationDeliveryResult) string {
	for _, result := range results {
		if result.Result == "failed" && strings.TrimSpace(result.Message) != "" {
			return result.Message
		}
	}
	return ""
}
