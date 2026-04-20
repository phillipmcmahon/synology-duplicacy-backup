package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
)

type CommandError struct {
	Message string
	Output  string
}

func (e *CommandError) Error() string {
	return e.Message
}

func CommandOutput(err error) string {
	var notifyErr *CommandError
	if errors.As(err, &notifyErr) {
		return notifyErr.Output
	}
	return ""
}

type TestReport struct {
	Command     string           `json:"command"`
	Scope       string           `json:"scope,omitempty"`
	Label       string           `json:"label"`
	Target      string           `json:"target"`
	StorageType string           `json:"storage_type,omitempty"`
	Location    string           `json:"location,omitempty"`
	Provider    string           `json:"provider"`
	Severity    string           `json:"severity"`
	Category    string           `json:"category"`
	Event       string           `json:"event"`
	Summary     string           `json:"summary"`
	Message     string           `json:"message,omitempty"`
	DryRun      bool             `json:"dry_run"`
	Result      string           `json:"result"`
	Providers   []DeliveryResult `json:"providers,omitempty"`
}

type TestReportInput struct {
	Command     string
	Scope       string
	Label       string
	Target      string
	StorageType string
	Location    string
	Provider    string
	Severity    string
	Category    string
	Event       string
	Summary     string
	Message     string
	DryRun      bool
}

func NewFailureTestReport(input TestReportInput) *TestReport {
	report := &TestReport{
		Command:  fallbackValue(input.Command, "test"),
		Scope:    input.Scope,
		Label:    input.Label,
		Target:   input.Target,
		Provider: fallbackValue(input.Provider, ProviderAll),
		Severity: fallbackValue(input.Severity, "warning"),
		Category: fallbackValue(input.Category, "test"),
		Event:    fallbackValue(input.Event, "notification_test"),
		Summary:  fallbackValue(input.Summary, "Notification test failed"),
		Message:  input.Message,
		DryRun:   input.DryRun,
		Result:   "failed",
	}
	return report
}

func NewTestReport(input TestReportInput, destinations []Destination, result string) *TestReport {
	report := &TestReport{
		Command:     fallbackValue(input.Command, "test"),
		Scope:       input.Scope,
		Label:       input.Label,
		Target:      input.Target,
		StorageType: input.StorageType,
		Location:    input.Location,
		Provider:    fallbackValue(input.Provider, ProviderAll),
		Severity:    input.Severity,
		Category:    input.Category,
		Event:       input.Event,
		Summary:     input.Summary,
		Message:     input.Message,
		DryRun:      input.DryRun,
		Result:      result,
	}
	for _, destination := range destinations {
		report.Providers = append(report.Providers, DeliveryResult{
			Provider:    destination.Provider,
			Destination: destination.Destination,
		})
	}
	return report
}

func WriteTestReport(w io.Writer, report *TestReport) error {
	if report == nil {
		return nil
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(report)
}

func FormatTestOutput(report *TestReport, jsonSummary bool) string {
	if jsonSummary {
		return formatTestJSON(report)
	}
	return formatTestText(report)
}

func FirstFailedResult(results []DeliveryResult) string {
	for _, result := range results {
		if result.Result == "failed" && strings.TrimSpace(result.Message) != "" {
			return result.Message
		}
	}
	return ""
}

func formatTestJSON(report *TestReport) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(report)
	return b.String()
}

func formatTestText(report *TestReport) string {
	lines := []reportLine{
		{Label: "Scope", Value: report.Scope},
		{Label: "Label", Value: report.Label},
		{Label: "Target", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Provider", Value: report.Provider},
		{Label: "Severity", Value: report.Severity},
		{Label: "Category", Value: report.Category},
		{Label: "Event", Value: report.Event},
		{Label: "Summary", Value: report.Summary},
	}
	if report.Message != "" {
		lines = append(lines, reportLine{Label: "Message", Value: report.Message})
	}
	lines = append(lines, reportLine{Label: "Dry Run", Value: fmt.Sprintf("%t", report.DryRun)})

	var providerLines []reportLine
	for _, provider := range report.Providers {
		label := presentation.Title(provider.Provider)
		value := provider.Result
		if provider.Message != "" {
			value = fmt.Sprintf("%s (%s)", value, provider.Message)
		}
		if provider.Destination != "" {
			value = fmt.Sprintf("%s -> %s", value, provider.Destination)
		}
		providerLines = append(providerLines, reportLine{Label: label, Value: value})
	}

	var b strings.Builder
	title := strings.TrimSpace(report.Scope)
	if title == "" {
		title = strings.Trim(strings.TrimSpace(report.Label)+"/"+strings.TrimSpace(report.Target), "/")
	}
	if title == "" {
		title = "notification"
	}
	b.WriteString(fmt.Sprintf("Notification test for %s\n", title))
	for _, line := range lines {
		if strings.TrimSpace(line.Value) == "" {
			continue
		}
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
	fmt.Fprintf(&b, "  %-20s : %s\n", "Result", presentation.Title(result))
	return b.String()
}

type reportLine struct {
	Label string
	Value string
}
