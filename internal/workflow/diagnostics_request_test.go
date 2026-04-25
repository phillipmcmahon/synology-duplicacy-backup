package workflow

import "testing"

func TestNewDiagnosticsRequestProjectsOnlyDiagnosticsIntent(t *testing.T) {
	req := &Request{
		DiagnosticsCommand: "diagnostics",
		Source:             "homes",
		RequestedTarget:    "onsite-usb",
		ConfigDir:          "/etc/duplicacy-backup",
		SecretsDir:         "/root/.secrets",
		JSONSummary:        true,
		NotifyCommand:      "test",
		NotifyProvider:     "ntfy",
		RestoreCommand:     "run",
		RestoreRevision:    2403,
		UpdateCommand:      "update",
		UpdateForce:        true,
		DoBackup:           true,
		DryRun:             true,
	}

	got := NewDiagnosticsRequest(req)
	if got.Command != "diagnostics" || got.Label != "homes" || got.Target() != "onsite-usb" {
		t.Fatalf("diagnostics identity projection failed: %#v", got)
	}
	if got.ConfigDir != "/etc/duplicacy-backup" || got.SecretsDir != "/root/.secrets" {
		t.Fatalf("diagnostics config projection failed: %#v", got)
	}
	if !got.JSONSummary {
		t.Fatalf("diagnostics JSON projection failed: %#v", got)
	}
}

func TestNewDiagnosticsRequestFromNilIsZeroValue(t *testing.T) {
	got := NewDiagnosticsRequest(nil)
	if got.Command != "" || got.Label != "" || got.Target() != "" || got.ConfigDir != "" || got.SecretsDir != "" || got.JSONSummary {
		t.Fatalf("NewDiagnosticsRequest(nil) = %#v", got)
	}
}
