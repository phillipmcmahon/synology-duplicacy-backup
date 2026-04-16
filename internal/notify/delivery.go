package notify

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
	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

var loadOptionalHealthWebhookToken = secrets.LoadOptionalHealthWebhookToken
var loadOptionalHealthNtfyToken = secrets.LoadOptionalHealthNtfyToken

func SetTokenLoadersForTesting(webhookLoader, ntfyLoader func(string, string) (string, error)) func() {
	oldWebhook := loadOptionalHealthWebhookToken
	oldNtfy := loadOptionalHealthNtfyToken
	if webhookLoader != nil {
		loadOptionalHealthWebhookToken = webhookLoader
	}
	if ntfyLoader != nil {
		loadOptionalHealthNtfyToken = ntfyLoader
	}
	return func() {
		loadOptionalHealthWebhookToken = oldWebhook
		loadOptionalHealthNtfyToken = oldNtfy
	}
}

type Payload struct {
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

type DeliveryResult struct {
	Provider    string `json:"provider"`
	Destination string `json:"destination,omitempty"`
	Result      string `json:"result"`
	Message     string `json:"message,omitempty"`
}

type Destination struct {
	Provider    string
	Destination string
}

type SendOptions struct {
	IgnoreOptionalAuthLoadErrors bool
}

const (
	ProviderAll     = "all"
	ProviderWebhook = "webhook"
	ProviderNtfy    = "ntfy"
)

func HasDestination(cfg config.HealthNotifyConfig) bool {
	return strings.TrimSpace(cfg.WebhookURL) != "" || strings.TrimSpace(cfg.Ntfy.Topic) != ""
}

func SendConfigured(cfg config.HealthNotifyConfig, secretsFile, target string, payload *Payload) error {
	results, err := SendConfiguredDetailedWithOptions(cfg, secretsFile, target, payload, ProviderAll, SendOptions{})
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

func SendConfiguredDetailed(cfg config.HealthNotifyConfig, secretsFile, target string, payload *Payload, provider string) ([]DeliveryResult, error) {
	return SendConfiguredDetailedWithOptions(cfg, secretsFile, target, payload, provider, SendOptions{})
}

func SendConfiguredDetailedWithOptions(cfg config.HealthNotifyConfig, secretsFile, target string, payload *Payload, provider string, opts SendOptions) ([]DeliveryResult, error) {
	if payload == nil {
		return nil, nil
	}
	destinations, err := ConfiguredDestinations(cfg, provider)
	if err != nil {
		return nil, err
	}
	results := make([]DeliveryResult, 0, len(destinations))
	var errs []error
	for _, destination := range destinations {
		sendErr := sendToProvider(destination.Provider, cfg, secretsFile, target, payload, opts)
		result := DeliveryResult{
			Provider:    destination.Provider,
			Destination: destination.Destination,
		}
		if sendErr != nil {
			result.Result = "failed"
			result.Message = sendErr.Error()
			errs = append(errs, sendErr)
		} else {
			result.Result = "delivered"
		}
		results = append(results, result)
	}
	return results, errors.Join(errs...)
}

func ConfiguredDestinations(cfg config.HealthNotifyConfig, provider string) ([]Destination, error) {
	return ConfiguredDestinationsForScope(cfg, provider, "selected target")
}

func ConfiguredDestinationsForScope(cfg config.HealthNotifyConfig, provider string, scope string) ([]Destination, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = ProviderAll
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "selected notification scope"
	}

	webhookConfigured := strings.TrimSpace(cfg.WebhookURL) != ""
	ntfyConfigured := strings.TrimSpace(cfg.Ntfy.Topic) != ""
	ntfyURL := strings.TrimRight(strings.TrimSpace(cfg.Ntfy.URL), "/")
	if ntfyURL == "" {
		ntfyURL = "https://ntfy.sh"
	}

	switch provider {
	case ProviderAll:
		var destinations []Destination
		if webhookConfigured {
			destinations = append(destinations, Destination{Provider: ProviderWebhook, Destination: cfg.WebhookURL})
		}
		if ntfyConfigured {
			destinations = append(destinations, Destination{Provider: ProviderNtfy, Destination: ntfyURL + "/" + strings.TrimSpace(cfg.Ntfy.Topic)})
		}
		if len(destinations) == 0 {
			return nil, fmt.Errorf("No notification destinations are configured for the %s", scope)
		}
		return destinations, nil
	case ProviderWebhook:
		if !webhookConfigured {
			return nil, fmt.Errorf("No webhook notification destination is configured for the %s", scope)
		}
		return []Destination{{Provider: ProviderWebhook, Destination: cfg.WebhookURL}}, nil
	case ProviderNtfy:
		if !ntfyConfigured {
			return nil, fmt.Errorf("No ntfy notification destination is configured for the %s", scope)
		}
		return []Destination{{Provider: ProviderNtfy, Destination: ntfyURL + "/" + strings.TrimSpace(cfg.Ntfy.Topic)}}, nil
	default:
		return nil, fmt.Errorf("Unsupported notify provider %q", provider)
	}
}

func sendToProvider(provider string, cfg config.HealthNotifyConfig, secretsFile, target string, payload *Payload, opts SendOptions) error {
	switch provider {
	case ProviderWebhook:
		return sendWebhookPayload(cfg, secretsFile, target, payload, opts)
	case ProviderNtfy:
		return sendNtfyNotification(cfg, secretsFile, target, payload, opts)
	default:
		return fmt.Errorf("Unsupported notify provider %q", provider)
	}
}

func sendWebhookPayload(cfg config.HealthNotifyConfig, secretsFile, target string, payload *Payload, opts SendOptions) error {
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
	return doRequest(req, "webhook delivery")
}

func sendNtfyNotification(cfg config.HealthNotifyConfig, secretsFile, target string, payload *Payload, opts SendOptions) error {
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
	return doRequest(req, "ntfy delivery")
}

func doRequest(req *http.Request, label string) error {
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

func loadOptionalNotificationToken(secretsFile, target string, loader func(string, string) (string, error), opts SendOptions) (string, error) {
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

func BuildTestPayload(now time.Time, pid int, label, target, storageType, location, severity, summary, message string) *Payload {
	severity = strings.TrimSpace(severity)
	if severity == "" {
		severity = "warning"
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = fmt.Sprintf("Notification test for %s/%s", label, target)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "This is a simulated operator-initiated test notification."
	}
	return NewPayload(now, pid, severity, "test", "notification_test", summary, label, target, storageType, location, "", "", "test", map[string]any{
		"message": message,
	})
}

func NewPayload(now time.Time, pid int, severity, category, event, summary, label, target, storageType, location, operation, check, status string, details map[string]any) *Payload {
	now = now.UTC()
	return &Payload{
		Version:     "1",
		EventID:     notificationEventID(now, pid, event, label, target),
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

func DetailsMessage(details map[string]any) string {
	if len(details) == 0 {
		return ""
	}
	message, _ := details["message"].(string)
	return strings.TrimSpace(message)
}

func notificationEventID(now time.Time, pid int, event, label, target string) string {
	return fmt.Sprintf("%s-%d-%s-%s-%s",
		now.Format("20060102T150405.000000000Z"),
		pid,
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

func ntfyTitle(payload *Payload) string {
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

func ntfyTags(payload *Payload) string {
	tags := []string{"duplicacy"}
	for _, value := range []string{payload.Severity, payload.Category, payload.Event, payload.Status} {
		tag := sanitizeTag(value)
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	return strings.Join(tags, ",")
}

func sanitizeTag(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return strings.NewReplacer(" ", "-", "_", "-", "/", "-").Replace(value)
}

func ntfyMessageBody(payload *Payload) string {
	lines := []string{
		fmt.Sprintf("Host: %s", fallbackValue(payload.Host, "unknown")),
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
	if message := DetailsMessage(payload.Details); message != "" {
		lines = append(lines, "", message)
	}
	return strings.Join(lines, "\n")
}

func fallbackValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatReportTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
