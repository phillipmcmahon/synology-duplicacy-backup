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
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/displaytime"
	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

var loadOptionalHealthWebhookToken = secrets.LoadOptionalHealthWebhookToken
var loadOptionalHealthNtfyToken = secrets.LoadOptionalHealthNtfyToken
var ntfyDisplayLocation = displaytime.Location

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
	Version   string         `json:"version"`
	EventID   string         `json:"event_id"`
	Timestamp string         `json:"timestamp"`
	Host      string         `json:"host"`
	Severity  string         `json:"severity"`
	Category  string         `json:"category"`
	Event     string         `json:"event"`
	Summary   string         `json:"summary"`
	Label     string         `json:"label"`
	Target    string         `json:"storage"`
	Location  string         `json:"location,omitempty"`
	Operation string         `json:"operation,omitempty"`
	Check     string         `json:"check,omitempty"`
	Status    string         `json:"status"`
	Details   map[string]any `json:"details,omitempty"`
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

type notificationProvider interface {
	Name() string
	Destination(config.HealthNotifyConfig) (string, bool)
	Send(config.HealthNotifyConfig, string, string, *Payload, SendOptions) error
}

type webhookProvider struct{}
type ntfyProvider struct{}

var notificationProviders = []notificationProvider{
	webhookProvider{},
	ntfyProvider{},
}

func (webhookProvider) Name() string {
	return ProviderWebhook
}

func (webhookProvider) Destination(cfg config.HealthNotifyConfig) (string, bool) {
	destination := strings.TrimSpace(cfg.WebhookURL)
	return destination, destination != ""
}

func (webhookProvider) Send(cfg config.HealthNotifyConfig, secretsFile, target string, payload *Payload, opts SendOptions) error {
	return sendWebhookPayload(cfg, secretsFile, target, payload, opts)
}

func (ntfyProvider) Name() string {
	return ProviderNtfy
}

func (ntfyProvider) Destination(cfg config.HealthNotifyConfig) (string, bool) {
	topic := strings.TrimSpace(cfg.Ntfy.Topic)
	if topic == "" {
		return "", false
	}
	url := strings.TrimRight(strings.TrimSpace(cfg.Ntfy.URL), "/")
	if url == "" {
		url = "https://ntfy.sh"
	}
	return url + "/" + topic, true
}

func (ntfyProvider) Send(cfg config.HealthNotifyConfig, secretsFile, target string, payload *Payload, opts SendOptions) error {
	return sendNtfyNotification(cfg, secretsFile, target, payload, opts)
}

func HasDestination(cfg config.HealthNotifyConfig) bool {
	for _, provider := range notificationProviders {
		if _, ok := provider.Destination(cfg); ok {
			return true
		}
	}
	return false
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
	return ConfiguredDestinationsForScope(cfg, provider, "selected storage")
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

	if provider == ProviderAll {
		destinations := configuredProviderDestinations(cfg, notificationProviders)
		if len(destinations) == 0 {
			return nil, fmt.Errorf("no notification destinations are configured for the %s", scope)
		}
		return destinations, nil
	}

	selected := providerByName(provider)
	if selected == nil {
		return nil, fmt.Errorf("unsupported notify provider %q", provider)
	}
	destination, ok := selected.Destination(cfg)
	if !ok {
		return nil, fmt.Errorf("no %s notification destination is configured for the %s", selected.Name(), scope)
	}
	return []Destination{{Provider: selected.Name(), Destination: destination}}, nil
}

func sendToProvider(provider string, cfg config.HealthNotifyConfig, secretsFile, target string, payload *Payload, opts SendOptions) error {
	selected := providerByName(provider)
	if selected == nil {
		return fmt.Errorf("unsupported notify provider %q", provider)
	}
	return selected.Send(cfg, secretsFile, target, payload, opts)
}

func configuredProviderDestinations(cfg config.HealthNotifyConfig, providers []notificationProvider) []Destination {
	var destinations []Destination
	for _, provider := range providers {
		if destination, ok := provider.Destination(cfg); ok {
			destinations = append(destinations, Destination{Provider: provider.Name(), Destination: destination})
		}
	}
	return destinations
}

func providerByName(name string) notificationProvider {
	for _, provider := range notificationProviders {
		if provider.Name() == name {
			return provider
		}
	}
	return nil
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

func BuildTestPayload(now time.Time, pid int, label, target, location, severity, summary, message string) *Payload {
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
	return NewPayload(now, pid, severity, "test", string(EventNotificationTest), summary, label, target, location, "", "", "test", map[string]any{
		"message": message,
	})
}

func NewPayload(now time.Time, pid int, severity, category, event, summary, label, target, location, operation, check, status string, details map[string]any) *Payload {
	now = now.UTC()
	return &Payload{
		Version:   "1",
		EventID:   notificationEventID(now, pid, event, label, target),
		Timestamp: formatReportTime(now),
		Host:      notificationHost(),
		Severity:  severity,
		Category:  category,
		Event:     event,
		Summary:   summary,
		Label:     label,
		Target:    target,
		Location:  location,
		Operation: operation,
		Check:     check,
		Status:    status,
		Details:   compactNotificationDetails(details),
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
	return strings.Join(compactTags(
		"duplicacy",
		payload.Category,
		ntfyStateTag(payload),
	), ",")
}

func ntfyStateTag(payload *Payload) string {
	if payload == nil {
		return ""
	}
	if ntfyNeedsSudo(payload) {
		return "needs-sudo"
	}
	switch payload.Event {
	case string(EventNotificationTest):
		return ""
	case string(EventSafePruneBlocked):
		return "prune-blocked"
	case string(EventVerifyFailedRevisions):
		return "verify-failed"
	case string(EventFreshnessFailed):
		return "freshness-failed"
	case string(EventHealthDegraded):
		return "degraded"
	case string(EventHealthUnhealthy):
		return "unhealthy"
	case string(EventUpdateInstallSucceeded):
		return "update-installed"
	case string(EventUpdateAlreadyCurrent):
		return "current"
	case string(EventUpdateReinstallRequested):
		return "reinstall-requested"
	case string(EventUpdateCheckFailed), string(EventUpdateDownloadFailed), string(EventUpdateChecksumFailed),
		string(EventUpdateAttestationFailed), string(EventUpdateInstallFailed):
		return "update-failed"
	}
	return payload.Status
}

func compactTags(values ...string) []string {
	seen := make(map[string]bool)
	tags := make([]string, 0, len(values))
	for _, value := range values {
		tag := sanitizeTag(value)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}

func sanitizeTag(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return strings.NewReplacer(" ", "-", "_", "-", "/", "-").Replace(value)
}

func ntfyMessageBody(payload *Payload) string {
	lines := []string{fmt.Sprintf("What: %s", payload.Summary)}
	if affected := ntfyAffectedLine(payload); affected != "" {
		lines = append(lines, "Affected: "+affected)
	}
	if reason := ntfyReason(payload); reason != "" {
		lines = append(lines, "Why: "+reason)
	}
	if action := ntfyAction(payload); action != "" {
		lines = append(lines, "Action: "+action)
	}
	if details := ntfyOperatorDetailLines(payload.Details); len(details) > 0 {
		lines = append(lines, details...)
	}
	lines = append(lines, "Context: "+ntfyContextLine(payload))
	return strings.Join(lines, "\n")
}

func ntfyAffectedLine(payload *Payload) string {
	switch {
	case payload.Label != "" && payload.Target != "" && payload.Location != "":
		return fmt.Sprintf("%s / %s (%s)", payload.Label, payload.Target, payload.Location)
	case payload.Label != "" && payload.Target != "":
		return fmt.Sprintf("%s / %s", payload.Label, payload.Target)
	case payload.Operation != "":
		return payload.Operation
	default:
		return ""
	}
}

func ntfyReason(payload *Payload) string {
	message := DetailsMessage(payload.Details)
	switch {
	case ntfyNeedsSudo(payload):
		return "Local repository is root-protected."
	case isManagedPathFailure(message):
		return "Update must be run through the managed command."
	default:
		return message
	}
}

func ntfyAction(payload *Payload) string {
	if action := stringDetail(payload.Details, "action"); action != "" {
		return action
	}
	if ntfyNeedsSudo(payload) {
		return "Run this check with sudo or review storage permissions."
	}
	if isManagedPathFailure(DetailsMessage(payload.Details)) {
		return "Run update using the managed duplicacy-backup command."
	}
	if actions := ntfyRecommendedActions(payload.Details); actions != "" {
		return actions
	}
	if payload.Event == string(EventSafePruneBlocked) {
		return "Review the prune preview before changing policy or using force."
	}
	if payload.Event == string(EventNotificationTest) {
		return "No action needed; this is a notification test."
	}
	return ""
}

func ntfyContextLine(payload *Payload) string {
	var parts []string
	if payload.Host != "" {
		parts = append(parts, payload.Host)
	}
	if payload.Check != "" {
		parts = append(parts, "check "+payload.Check)
	}
	if payload.Operation != "" {
		parts = append(parts, "operation "+payload.Operation)
	}
	if timestamp := ntfyLocalTimestamp(payload.Timestamp); timestamp != "" {
		parts = append(parts, timestamp)
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, ", ")
}

func ntfyLocalTimestamp(timestamp string) string {
	timestamp = strings.TrimSpace(timestamp)
	if timestamp == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}
	return parsed.In(ntfyDisplayLocation()).Format("2006-01-02 15:04:05 MST")
}

func ntfyNeedsSudo(payload *Payload) bool {
	if payload == nil {
		return false
	}
	message := strings.ToLower(DetailsMessage(payload.Details))
	return strings.Contains(message, "requires sudo") ||
		strings.Contains(message, "root-protected") ||
		stringDetail(payload.Details, "failure_code") == "verify_access_failed"
}

func isManagedPathFailure(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "requires the managed stable command path") &&
		strings.Contains(message, "current executable is")
}

func ntfyOperatorDetailLines(details map[string]any) []string {
	if len(details) == 0 {
		return nil
	}
	var lines []string
	if value := failedRevisionsDetail(details); value != "" {
		lines = append(lines, "Failed revisions: "+value)
	}
	if value := prunePreviewDetail(details); value != "" {
		lines = append(lines, "Prune preview: "+value)
	}
	if value := pruneLimitsDetail(details); value != "" {
		lines = append(lines, "Prune limits: "+value)
	}
	return lines
}

func ntfyRecommendedActions(details map[string]any) string {
	codes := stringSliceDetails(details, "recommended_action_codes")
	if len(codes) == 0 {
		return ""
	}
	actions := make([]string, 0, len(codes))
	for _, code := range codes {
		switch code {
		case "check_storage_access":
			actions = append(actions, "check storage access")
		case "recheck_repository_state":
			actions = append(actions, "recheck repository state")
		case "rerun_verify":
			actions = append(actions, "rerun verify")
		case "run_backup":
			actions = append(actions, "run a backup")
		default:
			actions = append(actions, strings.ReplaceAll(code, "_", " "))
		}
	}
	return strings.Join(compactStrings(actions...), "; ")
}

func compactStrings(values ...string) []string {
	seen := make(map[string]bool)
	compacted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		compacted = append(compacted, value)
	}
	return compacted
}

func stringDetail(details map[string]any, key string) string {
	value, _ := details[key].(string)
	return strings.TrimSpace(value)
}

func stringSliceDetails(details map[string]any, key string) []string {
	switch values := details[key].(type) {
	case []string:
		return values
	case []any:
		parts := make([]string, 0, len(values))
		for _, value := range values {
			parts = append(parts, fmt.Sprint(value))
		}
		return parts
	default:
		return nil
	}
}

func intDetail(details map[string]any, key string) (int, bool) {
	switch value := details[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func intSliceDetail(details map[string]any, key string) []int {
	switch values := details[key].(type) {
	case []int:
		return values
	case []any:
		converted := make([]int, 0, len(values))
		for _, value := range values {
			switch v := value.(type) {
			case int:
				converted = append(converted, v)
			case float64:
				converted = append(converted, int(v))
			}
		}
		return converted
	default:
		return nil
	}
}

func failedRevisionsDetail(details map[string]any) string {
	count, ok := intDetail(details, "failed_revision_count")
	if !ok || count <= 0 {
		return ""
	}
	revisions := intSliceDetail(details, "failed_revisions")
	if len(revisions) == 0 {
		return fmt.Sprintf("%d", count)
	}
	parts := make([]string, 0, len(revisions))
	for _, revision := range revisions {
		parts = append(parts, fmt.Sprintf("%d", revision))
	}
	return fmt.Sprintf("%d (%s)", count, strings.Join(parts, ", "))
}

func prunePreviewDetail(details map[string]any) string {
	deletes, ok := intDetail(details, "preview_deletes")
	if !ok {
		return ""
	}
	total, hasTotal := intDetail(details, "preview_total_revisions")
	percent, hasPercent := intDetail(details, "delete_percent")
	switch {
	case hasTotal && hasPercent:
		return fmt.Sprintf("would delete %d of %d revisions (%d%%)", deletes, total, percent)
	case hasTotal:
		return fmt.Sprintf("would delete %d of %d revisions", deletes, total)
	default:
		return fmt.Sprintf("would delete %d revisions", deletes)
	}
}

func pruneLimitsDetail(details map[string]any) string {
	maxPercent, hasPercent := intDetail(details, "max_delete_percent")
	maxCount, hasCount := intDetail(details, "max_delete_count")
	switch {
	case hasPercent && hasCount:
		return fmt.Sprintf("max %d%% or %d revisions", maxPercent, maxCount)
	case hasPercent:
		return fmt.Sprintf("max %d%%", maxPercent)
	case hasCount:
		return fmt.Sprintf("max %d revisions", maxCount)
	default:
		return ""
	}
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
