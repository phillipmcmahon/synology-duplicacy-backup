package notify

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewTestReportFormatsTextAndJSON(t *testing.T) {
	report := NewTestReport(TestReportInput{
		Command:     "notify test",
		Scope:       "homes/offsite-storj",
		Label:       "homes",
		Target:      "offsite-storj",
		StorageType: "duplicacy",
		Location:    "remote",
		Provider:    ProviderAll,
		Severity:    "warning",
		Category:    "test",
		Event:       "notification_test",
		Summary:     "Notification test for homes/offsite-storj",
		Message:     "Operator initiated smoke test.",
		DryRun:      true,
	}, []Destination{
		{Provider: ProviderWebhook, Destination: "https://example.invalid/hook"},
		{Provider: ProviderNtfy, Destination: "https://ntfy.sh/test-topic"},
	}, "delivered")
	report.Providers[0].Result = "delivered"
	report.Providers[1].Result = "failed"
	report.Providers[1].Message = "ntfy delivery returned 500"

	text := FormatTestOutput(report, false)
	for _, token := range []string{
		"Notification test for homes/offsite-storj",
		"Provider", "all",
		"Dry Run", "true",
		"Section: Providers",
		"Webhook", "delivered -> https://example.invalid/hook",
		"Ntfy", "failed (ntfy delivery returned 500) -> https://ntfy.sh/test-topic",
		"Result", "Delivered",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("text output missing %q:\n%s", token, text)
		}
	}

	var payload TestReport
	if err := json.Unmarshal([]byte(FormatTestOutput(report, true)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Command != "notify test" || payload.Result != "delivered" || len(payload.Providers) != 2 {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestFailureReportAndCommandOutput(t *testing.T) {
	report := NewFailureTestReport(TestReportInput{
		Label:   "homes",
		Target:  "offsite-storj",
		Message: "No notification destination is configured.",
	})
	if report.Command != "test" || report.Provider != ProviderAll || report.Severity != "warning" || report.Result != "failed" {
		t.Fatalf("failure report defaults = %+v", report)
	}

	var buf bytes.Buffer
	if err := WriteTestReport(&buf, report); err != nil {
		t.Fatalf("WriteTestReport() error = %v", err)
	}
	if !strings.Contains(buf.String(), `"result": "failed"`) {
		t.Fatalf("encoded report = %s", buf.String())
	}
	if err := WriteTestReport(&buf, nil); err != nil {
		t.Fatalf("WriteTestReport(nil) error = %v", err)
	}

	err := &CommandError{Message: "delivery failed", Output: "provider output"}
	if err.Error() != "delivery failed" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if got := CommandOutput(err); got != "provider output" {
		t.Fatalf("CommandOutput(CommandError) = %q", got)
	}
	if got := CommandOutput(assertionError("plain error")); got != "" {
		t.Fatalf("CommandOutput(non-command error) = %q", got)
	}
}

func TestFirstFailedResultSkipsEmptyMessages(t *testing.T) {
	results := []DeliveryResult{
		{Provider: ProviderWebhook, Result: "delivered"},
		{Provider: ProviderNtfy, Result: "failed"},
		{Provider: ProviderNtfy, Result: "failed", Message: "ntfy delivery returned 500"},
	}
	if got := FirstFailedResult(results); got != "ntfy delivery returned 500" {
		t.Fatalf("FirstFailedResult() = %q", got)
	}
}

func TestBuildTestPayloadAppliesOperatorDefaults(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 30, 0, 0, time.UTC)
	payload := BuildTestPayload(now, 4321, "homes", "onsite-usb1", "filesystem", "local", "", "", "")

	if payload.Severity != "warning" || payload.Category != "test" || payload.Event != "notification_test" {
		t.Fatalf("payload classification = %+v", payload)
	}
	if payload.Summary != "Notification test for homes/onsite-usb1" {
		t.Fatalf("Summary = %q", payload.Summary)
	}
	if got := DetailsMessage(payload.Details); got != "This is a simulated operator-initiated test notification." {
		t.Fatalf("DetailsMessage() = %q", got)
	}
	if !strings.Contains(payload.EventID, "notification-test-homes-onsite-usb1") {
		t.Fatalf("EventID = %q", payload.EventID)
	}
}

type assertionError string

func (e assertionError) Error() string {
	return string(e)
}
