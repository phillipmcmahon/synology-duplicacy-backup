package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

type WebhookPayload struct {
	Version     string         `json:"version"`
	EventID     string         `json:"event_id"`
	Timestamp   string         `json:"timestamp"`
	Host        string         `json:"host"`
	Severity    string         `json:"severity"`
	Category    string         `json:"category"`
	Event       string         `json:"event"`
	Summary     string         `json:"summary"`
	Label       string         `json:"label"`
	Target      string         `json:"target"`
	StorageType string         `json:"storage_type,omitempty"`
	Location    string         `json:"location,omitempty"`
	Operation   string         `json:"operation,omitempty"`
	Check       string         `json:"check,omitempty"`
	Status      string         `json:"status"`
	Details     map[string]any `json:"details,omitempty"`
}

func shouldSendConfiguredWebhook(rt Runtime, interactive bool, cfg config.HealthNotifyConfig, sendFor string) bool {
	if cfg.WebhookURL == "" {
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

func sendWebhookPayload(cfg config.HealthNotifyConfig, secretsFile, target string, payload *WebhookPayload) error {
	if payload == nil {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode webhook payload: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token, err := loadOptionalHealthWebhookToken(secretsFile, target); err != nil {
		return err
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook delivery failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook delivery returned %s", resp.Status)
	}
	return nil
}

func buildRuntimeWebhookPayload(rt Runtime, plan *Plan, report *RunReport, err error, visibleRunStarted bool, preview *duplicacy.PrunePreview) *WebhookPayload {
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

	if !visibleRunStarted && plan.DoBackup {
		return newWebhookPayload(rt, "critical", "backup", "backup_could_not_start",
			fmt.Sprintf("Backup could not start for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"backup", "", "failed", details,
		)
	}

	switch lastFailedPhaseName(report) {
	case "Backup":
		return newWebhookPayload(rt, "critical", "backup", "backup_failed",
			fmt.Sprintf("Backup failed for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
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
				details["max_delete_percent"] = plan.SafePruneMaxDeletePercent
				details["max_delete_count"] = plan.SafePruneMaxDeleteCount
			}
			return newWebhookPayload(rt, "warning", "maintenance", "safe_prune_blocked",
				fmt.Sprintf("Safe prune blocked for %s/%s", report.Label, report.Target),
				report.Label, report.Target, report.StorageType, report.Location,
				"prune", "", "blocked", details,
			)
		}
		return newWebhookPayload(rt, "warning", "maintenance", "prune_failed",
			fmt.Sprintf("Prune failed for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"prune", "", "failed", details,
		)
	case "Storage cleanup":
		return newWebhookPayload(rt, "warning", "maintenance", "cleanup_failed",
			fmt.Sprintf("Storage cleanup failed for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"cleanup_storage", "", "failed", details,
		)
	default:
		return nil
	}
}

func buildHealthWebhookPayload(rt Runtime, report *HealthReport) *WebhookPayload {
	if report == nil {
		return nil
	}

	if report.CheckType == "verify" && report.FailedRevisionCount > 0 {
		return newWebhookPayload(rt, "critical", "health", "verify_failed_revisions",
			fmt.Sprintf("Verify found failed revisions for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"", report.CheckType, report.Status,
			map[string]any{
				"failed_revision_count": report.FailedRevisionCount,
				"failed_revisions":      append([]int(nil), report.FailedRevisions...),
				"message":               healthCheckMessage(report, "Integrity check"),
			},
		)
	}

	if result, message, ok := healthCheckResult(report, "Backup freshness"); ok && result == "fail" {
		return newWebhookPayload(rt, "critical", "health", "freshness_failed",
			fmt.Sprintf("Freshness failure for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"", report.CheckType, report.Status,
			map[string]any{
				"message":   message,
				"freshness": message,
			},
		)
	}

	switch report.Status {
	case "degraded":
		return newWebhookPayload(rt, "warning", "health", "health_degraded",
			fmt.Sprintf("Health degraded for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"", report.CheckType, report.Status,
			map[string]any{
				"message": firstHealthIssueMessage(report),
			},
		)
	case "unhealthy":
		return newWebhookPayload(rt, "critical", "health", "health_unhealthy",
			fmt.Sprintf("Health unhealthy for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"", report.CheckType, report.Status,
			map[string]any{
				"message": firstHealthIssueMessage(report),
			},
		)
	default:
		return nil
	}
}

func newWebhookPayload(rt Runtime, severity, category, event, summary, label, target, storageType, location, operation, check, status string, details map[string]any) *WebhookPayload {
	now := rt.Now().UTC()
	return &WebhookPayload{
		Version:     "1",
		EventID:     webhookEventID(rt, event, label, target),
		Timestamp:   formatReportTime(now),
		Host:        webhookHost(),
		Severity:    severity,
		Category:    category,
		Event:       event,
		Summary:     summary,
		Label:       label,
		Target:      target,
		StorageType: storageType,
		Location:    location,
		Operation:   operation,
		Check:       check,
		Status:      status,
		Details:     compactWebhookDetails(details),
	}
}

func webhookEventID(rt Runtime, event, label, target string) string {
	return fmt.Sprintf("%s-%d-%s-%s-%s",
		rt.Now().UTC().Format("20060102T150405.000000000Z"),
		rt.Getpid(),
		strings.ReplaceAll(event, "_", "-"),
		label,
		target,
	)
}

func webhookHost() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "unknown"
	}
	return host
}

func compactWebhookDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	trimmed := make(map[string]any, len(details))
	for key, value := range details {
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				continue
			}
		case []int:
			if len(v) == 0 {
				continue
			}
		}
		trimmed[key] = value
	}
	if len(trimmed) == 0 {
		return nil
	}
	return trimmed
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

func healthCheckResult(report *HealthReport, name string) (result string, message string, ok bool) {
	if report == nil {
		return "", "", false
	}
	for _, check := range report.Checks {
		if check.Name == name {
			return check.Result, check.Message, true
		}
	}
	return "", "", false
}

func healthCheckMessage(report *HealthReport, name string) string {
	_, message, _ := healthCheckResult(report, name)
	return message
}

func firstHealthIssueMessage(report *HealthReport) string {
	if report == nil {
		return ""
	}
	for _, issue := range report.Issues {
		if strings.TrimSpace(issue.Message) != "" {
			return issue.Message
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
