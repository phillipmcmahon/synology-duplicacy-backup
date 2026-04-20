package notify

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

func TestSendConfigured_WebhookAddsBearerToken(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := SetTokenLoadersForTesting(func(string, string) (string, error) {
		return "hook-token", nil
	}, nil)
	defer restore()

	payload := NewPayload(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), 1234, "critical", "health", "health_unhealthy",
		"Health unhealthy for homes/offsite-storj",
		"homes", "offsite-storj", "duplicacy", "remote", "", "verify", "unhealthy", map[string]any{"message": "boom"},
	)
	cfg := config.HealthNotifyConfig{WebhookURL: server.URL}
	if err := SendConfigured(cfg, "/root/.secrets/homes-secrets.toml", "offsite-storj", payload); err != nil {
		t.Fatalf("SendConfigured() error = %v", err)
	}
	if gotAuth != "Bearer hook-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}

func TestSendConfigured_Ntfy(t *testing.T) {
	var gotTitle, gotPriority, gotTags, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		gotPriority = r.Header.Get("Priority")
		gotTags = r.Header.Get("Tags")
		gotAuth = r.Header.Get("Authorization")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		gotBody = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := SetTokenLoadersForTesting(func(string, string) (string, error) { return "", nil }, func(string, string) (string, error) {
		return "ntfy-token", nil
	})
	defer restore()

	payload := NewPayload(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), 1234, "warning", "maintenance", "safe_prune_blocked",
		"Safe prune blocked for homes/offsite-storj",
		"homes", "offsite-storj", "duplicacy", "remote", "prune", "", "blocked", map[string]any{"message": "Safe prune blocked because deletion threshold would be exceeded"},
	)
	cfg := config.HealthNotifyConfig{
		Ntfy: config.HealthNotifyNtfyConfig{
			URL:   server.URL,
			Topic: "duplicacy-alerts",
		},
	}
	if err := SendConfigured(cfg, "/root/.secrets/homes-secrets.toml", "offsite-storj", payload); err != nil {
		t.Fatalf("SendConfigured() error = %v", err)
	}
	if gotTitle != "WARNING: Safe prune blocked for homes/offsite-storj" {
		t.Fatalf("Title = %q", gotTitle)
	}
	if gotPriority != "3" {
		t.Fatalf("Priority = %q", gotPriority)
	}
	if gotTags != "duplicacy,warning,maintenance,safe-prune-blocked,blocked" {
		t.Fatalf("Tags = %q", gotTags)
	}
	if gotAuth != "Bearer ntfy-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !strings.Contains(gotBody, "Location: remote") || !strings.Contains(gotBody, "Safe prune blocked because deletion threshold would be exceeded") {
		t.Fatalf("Body = %q", gotBody)
	}
}

func TestConfiguredDestinationsAndHasDestination(t *testing.T) {
	empty := config.HealthNotifyConfig{}
	if HasDestination(empty) {
		t.Fatal("HasDestination(empty) = true, want false")
	}

	cfg := config.HealthNotifyConfig{
		WebhookURL: "https://example.invalid/hook",
		Ntfy: config.HealthNotifyNtfyConfig{
			URL:   "https://ntfy.example.invalid/",
			Topic: "homes-alerts",
		},
	}
	if !HasDestination(cfg) {
		t.Fatal("HasDestination(configured) = false, want true")
	}

	destinations, err := ConfiguredDestinationsForScope(cfg, "", "homes label")
	if err != nil {
		t.Fatalf("ConfiguredDestinationsForScope(all) error = %v", err)
	}
	if len(destinations) != 2 || destinations[0].Provider != ProviderWebhook || destinations[1].Destination != "https://ntfy.example.invalid/homes-alerts" {
		t.Fatalf("destinations = %+v", destinations)
	}

	if _, err := ConfiguredDestinationsForScope(empty, ProviderNtfy, "homes label"); err == nil || !strings.Contains(err.Error(), "no ntfy notification destination") {
		t.Fatalf("ConfiguredDestinationsForScope(missing ntfy) err = %v", err)
	}
	if _, err := ConfiguredDestinationsForScope(empty, ProviderWebhook, "homes label"); err == nil || !strings.Contains(err.Error(), "no webhook notification destination") {
		t.Fatalf("ConfiguredDestinationsForScope(missing webhook) err = %v", err)
	}
	if _, err := ConfiguredDestinationsForScope(empty, "discord", "homes label"); err == nil || !strings.Contains(err.Error(), "unsupported notify provider") {
		t.Fatalf("ConfiguredDestinationsForScope(unsupported) err = %v", err)
	}
}

func TestSendConfiguredDetailedWrapperReportsProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	payload := BuildTestPayload(time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC), 1234, "homes", "offsite-storj", "duplicacy", "remote", "", "", "")
	results, err := SendConfiguredDetailed(config.HealthNotifyConfig{WebhookURL: server.URL}, "", "offsite-storj", payload, ProviderWebhook)
	if err == nil {
		t.Fatal("SendConfiguredDetailed() err = nil, want provider failure")
	}
	if len(results) != 1 || results[0].Result != "failed" || !strings.Contains(results[0].Message, "webhook delivery returned 500") {
		t.Fatalf("results = %+v, err = %v", results, err)
	}
}

func TestSendConfiguredAggregatesFailedProviderMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	payload := BuildTestPayload(time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC), 1234, "homes", "offsite-storj", "duplicacy", "remote", "", "", "")
	err := SendConfigured(config.HealthNotifyConfig{WebhookURL: server.URL}, "", "offsite-storj", payload)
	if err == nil || !strings.Contains(err.Error(), "webhook delivery returned 502") {
		t.Fatalf("SendConfigured() err = %v", err)
	}
}

func TestLoadOptionalNotificationTokenIgnoresOnlyOptionalAccessFailures(t *testing.T) {
	ignored := func(string, string) (string, error) {
		return "", apperrors.NewSecretsError("stat", errors.New("missing secrets file"))
	}
	token, err := loadOptionalNotificationToken("/root/.secrets/homes-secrets.toml", "offsite-storj", ignored, SendOptions{IgnoreOptionalAuthLoadErrors: true})
	if err != nil || token != "" {
		t.Fatalf("ignored optional token load = %q, %v", token, err)
	}

	parseFailure := func(string, string) (string, error) {
		return "", apperrors.NewSecretsError("parse", errors.New("invalid TOML"))
	}
	if _, err := loadOptionalNotificationToken("/root/.secrets/homes-secrets.toml", "offsite-storj", parseFailure, SendOptions{IgnoreOptionalAuthLoadErrors: true}); err == nil {
		t.Fatal("parse failure err = nil, want error")
	}

	plainFailure := func(string, string) (string, error) {
		return "", errors.New("networked secret backend failed")
	}
	if _, err := loadOptionalNotificationToken("/root/.secrets/homes-secrets.toml", "offsite-storj", plainFailure, SendOptions{IgnoreOptionalAuthLoadErrors: true}); err == nil {
		t.Fatal("plain failure err = nil, want error")
	}
}

func TestNtfyPriorityMapping(t *testing.T) {
	tests := map[string]string{
		"critical": "5",
		"info":     "2",
		"warning":  "3",
		"unknown":  "3",
	}
	for severity, want := range tests {
		if got := ntfyPriority(severity); got != want {
			t.Fatalf("ntfyPriority(%q) = %q, want %q", severity, got, want)
		}
	}
}
