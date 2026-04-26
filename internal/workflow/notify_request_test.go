package workflow

import (
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
)

func TestNewNotifyRequestProjectsOnlyNotifyIntent(t *testing.T) {
	req := &Request{
		NotifyCommand:     "test",
		NotifyScope:       "update",
		Source:            "homes",
		RequestedTarget:   "onsite-usb",
		ConfigDir:         "/home/operator/.config/duplicacy-backup",
		SecretsDir:        "/home/operator/.config/duplicacy-backup/secrets",
		JSONSummary:       true,
		DryRun:            true,
		NotifyProvider:    "ntfy",
		NotifyEvent:       string(notify.EventUpdateInstallFailed),
		NotifySeverity:    "critical",
		NotifySummary:     "Synthetic summary",
		NotifyMessage:     "Synthetic message",
		RestoreCommand:    "run",
		RestoreRevision:   2403,
		UpdateCommand:     "update",
		UpdateForce:       true,
		RollbackCommand:   "rollback",
		RollbackCheckOnly: true,
		DoBackup:          true,
	}

	got := NewNotifyRequest(req)
	if got.Command != "test" || got.Scope != "update" || got.Label != "homes" || got.Target() != "onsite-usb" {
		t.Fatalf("notify identity projection failed: %#v", got)
	}
	if got.ConfigDir != "/home/operator/.config/duplicacy-backup" || got.SecretsDir != "/home/operator/.config/duplicacy-backup/secrets" {
		t.Fatalf("notify config projection failed: %#v", got)
	}
	if got.Provider != "ntfy" || got.Event != notify.EventUpdateInstallFailed || got.Severity != "critical" || got.Summary != "Synthetic summary" || got.Message != "Synthetic message" {
		t.Fatalf("notify option projection failed: %#v", got)
	}
	if !got.JSONSummary || !got.DryRun {
		t.Fatalf("notify flag projection failed: %#v", got)
	}
}

func TestNewNotifyRequestFromNilIsZeroValue(t *testing.T) {
	got := NewNotifyRequest(nil)
	if got.Command != "" || got.Scope != "" || got.Label != "" || got.Target() != "" || got.ConfigDir != "" || got.SecretsDir != "" || got.JSONSummary || got.DryRun || got.Provider != "" || got.Event != "" || got.Severity != "" || got.Summary != "" || got.Message != "" {
		t.Fatalf("NewNotifyRequest(nil) = %#v", got)
	}
}
