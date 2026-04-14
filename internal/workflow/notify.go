package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

var loadOptionalHealthWebhookToken = secrets.LoadOptionalHealthWebhookToken
var loadOptionalHealthNtfyToken = secrets.LoadOptionalHealthNtfyToken

type NotificationPayload struct {
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

type NotificationDeliveryResult struct {
	Provider    string `json:"provider"`
	Destination string `json:"destination,omitempty"`
	Result      string `json:"result"`
	Message     string `json:"message,omitempty"`
}

type configuredNotificationDestination struct {
	Provider    string
	Destination string
}

type notificationSendOptions struct {
	IgnoreOptionalAuthLoadErrors bool
}

const (
	notifyProviderAll     = "all"
	notifyProviderWebhook = "webhook"
	notifyProviderNtfy    = "ntfy"
)

func shouldSendConfiguredNotification(rt Runtime, interactive bool, cfg config.HealthNotifyConfig, sendFor string) bool {
	if !hasNotifyDestination(cfg) {
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

func hasNotifyDestination(cfg config.HealthNotifyConfig) bool {
	return strings.TrimSpace(cfg.WebhookURL) != "" || strings.TrimSpace(cfg.Ntfy.Topic) != ""
}

func sendConfiguredNotifications(cfg config.HealthNotifyConfig, secretsFile, target string, payload *NotificationPayload) error {
	results, err := sendConfiguredNotificationsDetailedWithOptions(cfg, secretsFile, target, payload, notifyProviderAll, notificationSendOptions{})
	if err == nil {
		return nil
	}
	var errs []error
	for _, result := range results {
		if result.Result == "failed" && result.Message != "" {
			errs = append(errs, errors.New(result.Message))
		}
	}
	if len(errs) == 0 {
		return err
	}
	return errors.Join(errs...)
}

func sendConfiguredNotificationsDetailed(cfg config.HealthNotifyConfig, secretsFile, target string, payload *NotificationPayload, provider string) ([]NotificationDeliveryResult, error) {
	return sendConfiguredNotificationsDetailedWithOptions(cfg, secretsFile, target, payload, provider, notificationSendOptions{})
}

func sendConfiguredNotificationsDetailedWithOptions(cfg config.HealthNotifyConfig, secretsFile, target string, payload *NotificationPayload, provider string, opts notificationSendOptions) ([]NotificationDeliveryResult, error) {
	if payload == nil {
		return nil, nil
	}
	destinations, err := configuredNotificationDestinations(cfg, provider)
	if err != nil {
		return nil, err
	}
	results := make([]NotificationDeliveryResult, 0, len(destinations))
	var errs []error
	for _, destination := range destinations {
		sendErr := sendNotificationToProvider(destination.Provider, cfg, secretsFile, target, payload, opts)
		result := NotificationDeliveryResult{
			Provider:    destination.Provider,
			Destination: destination.Destination,
		}
		if sendErr != nil {
			result.Result = "failed"
			result.Message = OperatorMessage(sendErr)
			errs = append(errs, sendErr)
		} else {
			result.Result = "delivered"
		}
		results = append(results, result)
	}
	return results, errors.Join(errs...)
}

func sendWebhookPayload(cfg config.HealthNotifyConfig, secretsFile, target string, payload *NotificationPayload, opts notificationSendOptions) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode webhook payload: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token, err := loadOptionalNotificationToken(secretsFile, target, loadOptionalHealthWebhookToken, opts); err != nil {
		return err
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doNotifyRequest(req, "webhook delivery")
}

func sendNtfyNotification(cfg config.HealthNotifyConfig, secretsFile, target string, payload *NotificationPayload, opts notificationSendOptions) error {
	url := strings.TrimRight(strings.TrimSpace(cfg.Ntfy.URL), "/")
	if url == "" {
		url = "https://ntfy.sh"
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url+"/"+strings.TrimSpace(cfg.Ntfy.Topic), bytes.NewBufferString(ntfyMessageBody(payload)))
	if err != nil {
		return fmt.Errorf("failed to build ntfy request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", ntfyTitle(payload))
	req.Header.Set("Priority", ntfyPriority(payload.Severity))
	req.Header.Set("Tags", ntfyTags(payload))
	if token, err := loadOptionalNotificationToken(secretsFile, target, loadOptionalHealthNtfyToken, opts); err != nil {
		return err
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doNotifyRequest(req, "ntfy delivery")
}

func doNotifyRequest(req *http.Request, label string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s failed: %w", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", label, resp.Status)
	}
	return nil
}

func configuredNotificationDestinations(cfg config.HealthNotifyConfig, provider string) ([]configuredNotificationDestination, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = notifyProviderAll
	}

	webhookConfigured := strings.TrimSpace(cfg.WebhookURL) != ""
	ntfyConfigured := strings.TrimSpace(cfg.Ntfy.Topic) != ""
	ntfyURL := strings.TrimRight(strings.TrimSpace(cfg.Ntfy.URL), "/")
	if ntfyURL == "" {
		ntfyURL = "https://ntfy.sh"
	}

	switch provider {
	case notifyProviderAll:
		var destinations []configuredNotificationDestination
		if webhookConfigured {
			destinations = append(destinations, configuredNotificationDestination{Provider: notifyProviderWebhook, Destination: cfg.WebhookURL})
		}
		if ntfyConfigured {
			destinations = append(destinations, configuredNotificationDestination{Provider: notifyProviderNtfy, Destination: ntfyURL + "/" + strings.TrimSpace(cfg.Ntfy.Topic)})
		}
		if len(destinations) == 0 {
			return nil, NewMessageError("No notification destinations are configured for the selected target")
		}
		return destinations, nil
	case notifyProviderWebhook:
		if !webhookConfigured {
			return nil, NewMessageError("No webhook notification destination is configured for the selected target")
		}
		return []configuredNotificationDestination{{Provider: notifyProviderWebhook, Destination: cfg.WebhookURL}}, nil
	case notifyProviderNtfy:
		if !ntfyConfigured {
			return nil, NewMessageError("No ntfy notification destination is configured for the selected target")
		}
		return []configuredNotificationDestination{{Provider: notifyProviderNtfy, Destination: ntfyURL + "/" + strings.TrimSpace(cfg.Ntfy.Topic)}}, nil
	default:
		return nil, NewMessageError("Unsupported notify provider %q", provider)
	}
}

func sendNotificationToProvider(provider string, cfg config.HealthNotifyConfig, secretsFile, target string, payload *NotificationPayload, opts notificationSendOptions) error {
	switch provider {
	case notifyProviderWebhook:
		return sendWebhookPayload(cfg, secretsFile, target, payload, opts)
	case notifyProviderNtfy:
		return sendNtfyNotification(cfg, secretsFile, target, payload, opts)
	default:
		return NewMessageError("Unsupported notify provider %q", provider)
	}
}

func loadOptionalNotificationToken(secretsFile, target string, loader func(string, string) (string, error), opts notificationSendOptions) (string, error) {
	if strings.TrimSpace(secretsFile) == "" {
		return "", nil
	}
	token, err := loader(secretsFile, target)
	if err == nil || !opts.IgnoreOptionalAuthLoadErrors {
		return token, err
	}

	var secErr *apperrors.SecretsError
	if !errors.As(err, &secErr) {
		return "", err
	}

	switch secErr.Phase {
	case "stat", "open", "permissions", "ownership":
		return "", nil
	default:
		return "", err
	}
}

func ntfyTitle(payload *NotificationPayload) string {
	severity := strings.ToUpper(strings.TrimSpace(payload.Severity))
	if severity == "" {
		return payload.Summary
	}
	return severity + ": " + payload.Summary
}

func ntfyPriority(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return "5"
	case "warning":
		return "3"
	case "info":
		return "2"
	default:
		return "3"
	}
}

func ntfyTags(payload *NotificationPayload) string {
	tags := []string{"duplicacy"}
	for _, value := range []string{payload.Severity, payload.Category, payload.Event, payload.Status} {
		tag := sanitizeNotifyTag(value)
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	return strings.Join(tags, ",")
}

func sanitizeNotifyTag(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return strings.NewReplacer(" ", "-", "_", "-", "/", "-").Replace(value)
}

func ntfyMessageBody(payload *NotificationPayload) string {
	lines := []string{
		fmt.Sprintf("Host: %s", fallbackNotifyValue(payload.Host, "unknown")),
		fmt.Sprintf("Severity: %s", payload.Severity),
		fmt.Sprintf("Category: %s", payload.Category),
		fmt.Sprintf("Event: %s", payload.Event),
	}
	if payload.Label != "" {
		lines = append(lines, fmt.Sprintf("Label: %s", payload.Label))
	}
	if payload.Target != "" {
		lines = append(lines, fmt.Sprintf("Target: %s", payload.Target))
	}
	if payload.StorageType != "" {
		lines = append(lines, fmt.Sprintf("Type: %s", payload.StorageType))
	}
	if payload.Location != "" {
		lines = append(lines, fmt.Sprintf("Location: %s", payload.Location))
	}
	if payload.Operation != "" {
		lines = append(lines, fmt.Sprintf("Operation: %s", payload.Operation))
	}
	if payload.Check != "" {
		lines = append(lines, fmt.Sprintf("Check: %s", payload.Check))
	}
	if payload.Status != "" {
		lines = append(lines, fmt.Sprintf("Status: %s", payload.Status))
	}
	if payload.Timestamp != "" {
		lines = append(lines, fmt.Sprintf("Timestamp: %s", payload.Timestamp))
	}
	if message := notifyDetailsMessage(payload.Details); message != "" {
		lines = append(lines, "", message)
	}
	return strings.Join(lines, "\n")
}

func buildTestNotificationPayload(rt Runtime, label, target, storageType, location string, req *Request) *NotificationPayload {
	severity := strings.TrimSpace(req.NotifySeverity)
	if severity == "" {
		severity = "warning"
	}
	summary := strings.TrimSpace(req.NotifySummary)
	if summary == "" {
		summary = fmt.Sprintf("Notification test for %s/%s", label, target)
	}
	message := strings.TrimSpace(req.NotifyMessage)
	if message == "" {
		message = "This is a simulated operator-initiated test notification."
	}
	return newNotificationPayload(rt, severity, "test", "notification_test", summary, label, target, storageType, location, "", "", "test", map[string]any{
		"message": message,
	})
}

func notifyDetailsMessage(details map[string]any) string {
	if len(details) == 0 {
		return ""
	}
	message, _ := details["message"].(string)
	return strings.TrimSpace(message)
}

func fallbackNotifyValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func buildRuntimeNotificationPayload(rt Runtime, plan *Plan, report *RunReport, err error, visibleRunStarted bool, preview *duplicacy.PrunePreview) *NotificationPayload {
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
		return newNotificationPayload(rt, "critical", "backup", "backup_could_not_start",
			fmt.Sprintf("Backup could not start for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"backup", "", "failed", details,
		)
	}

	switch lastFailedPhaseName(report) {
	case "Backup":
		return newNotificationPayload(rt, "critical", "backup", "backup_failed",
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
			return newNotificationPayload(rt, "warning", "maintenance", "safe_prune_blocked",
				fmt.Sprintf("Safe prune blocked for %s/%s", report.Label, report.Target),
				report.Label, report.Target, report.StorageType, report.Location,
				"prune", "", "blocked", details,
			)
		}
		return newNotificationPayload(rt, "warning", "maintenance", "prune_failed",
			fmt.Sprintf("Prune failed for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"prune", "", "failed", details,
		)
	case "Storage cleanup":
		return newNotificationPayload(rt, "warning", "maintenance", "cleanup_failed",
			fmt.Sprintf("Storage cleanup failed for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"cleanup_storage", "", "failed", details,
		)
	default:
		return nil
	}
}

func buildHealthNotificationPayload(rt Runtime, report *HealthReport) *NotificationPayload {
	if report == nil {
		return nil
	}

	if report.CheckType == "verify" && report.FailedRevisionCount > 0 {
		return newNotificationPayload(rt, "critical", "health", "verify_failed_revisions",
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
		return newNotificationPayload(rt, "critical", "health", "freshness_failed",
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
		return newNotificationPayload(rt, "warning", "health", "health_degraded",
			fmt.Sprintf("Health degraded for %s/%s", report.Label, report.Target),
			report.Label, report.Target, report.StorageType, report.Location,
			"", report.CheckType, report.Status,
			map[string]any{
				"message": firstHealthIssueMessage(report),
			},
		)
	case "unhealthy":
		return newNotificationPayload(rt, "critical", "health", "health_unhealthy",
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

func newNotificationPayload(rt Runtime, severity, category, event, summary, label, target, storageType, location, operation, check, status string, details map[string]any) *NotificationPayload {
	now := rt.Now().UTC()
	return &NotificationPayload{
		Version:     "1",
		EventID:     notificationEventID(rt, event, label, target),
		Timestamp:   formatReportTime(now),
		Host:        notificationHost(),
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
		Details:     compactNotificationDetails(details),
	}
}

func notificationEventID(rt Runtime, event, label, target string) string {
	return fmt.Sprintf("%s-%d-%s-%s-%s",
		rt.Now().UTC().Format("20060102T150405.000000000Z"),
		rt.Getpid(),
		strings.ReplaceAll(event, "_", "-"),
		label,
		target,
	)
}

func notificationHost() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "unknown"
	}
	return host
}

func compactNotificationDetails(details map[string]any) map[string]any {
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
