package notify

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
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
		"homes", "offsite-storj", "object", "remote", "", "verify", "unhealthy", map[string]any{"message": "boom"},
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
		"homes", "offsite-storj", "object", "remote", "prune", "", "blocked", map[string]any{"message": "Safe prune blocked because deletion threshold would be exceeded"},
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
	if !strings.Contains(gotBody, "Type: object") || !strings.Contains(gotBody, "Safe prune blocked because deletion threshold would be exceeded") {
		t.Fatalf("Body = %q", gotBody)
	}
}
