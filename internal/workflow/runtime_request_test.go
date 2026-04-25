package workflow

import "testing"

func TestNewRuntimeRequestProjectsOnlyRuntimeIntent(t *testing.T) {
	req := &Request{
		Source:             "homes",
		RequestedTarget:    "onsite-usb",
		ConfigDir:          "/etc/duplicacy-backup",
		SecretsDir:         "/root/.secrets",
		DoPrune:            true,
		ForcePrune:         true,
		DryRun:             true,
		Verbose:            true,
		JSONSummary:        true,
		DefaultNotice:      "notice",
		NotifyCommand:      "test",
		NotifyProvider:     "ntfy",
		RestoreCommand:     "run",
		RestoreRevision:    2403,
		UpdateCommand:      "update",
		RollbackCommand:    "rollback",
		DiagnosticsCommand: "diagnostics",
	}

	got := NewRuntimeRequest(req)
	if got.Mode != RuntimeModePrune || got.Label != "homes" || got.Target() != "onsite-usb" {
		t.Fatalf("runtime identity projection failed: %#v", got)
	}
	if got.ConfigDir != "/etc/duplicacy-backup" || got.SecretsDir != "/root/.secrets" {
		t.Fatalf("runtime path projection failed: %#v", got)
	}
	if !got.ForcePrune || !got.DryRun || !got.Verbose || !got.JSONSummary || got.DefaultNotice != "notice" {
		t.Fatalf("runtime flag projection failed: %#v", got)
	}
}

func TestRuntimeRequestModeHelpers(t *testing.T) {
	tests := []struct {
		name       string
		mode       RuntimeMode
		backup     bool
		prune      bool
		cleanup    bool
		fixPerms   bool
		fixOnly    bool
		operation  string
		forcePrune bool
	}{
		{name: "backup", mode: RuntimeModeBackup, backup: true, operation: "Backup"},
		{name: "safe prune", mode: RuntimeModePrune, prune: true, operation: "Safe prune"},
		{name: "forced prune", mode: RuntimeModePrune, prune: true, operation: "Forced prune", forcePrune: true},
		{name: "cleanup", mode: RuntimeModeCleanupStorage, cleanup: true, operation: "Storage cleanup"},
		{name: "fix perms", mode: RuntimeModeFixPerms, fixPerms: true, fixOnly: true, operation: "Fix permissions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &RuntimeRequest{Mode: tt.mode, ForcePrune: tt.forcePrune}
			if req.DoBackup() != tt.backup || req.DoPrune() != tt.prune || req.DoCleanupStore() != tt.cleanup || req.FixPerms() != tt.fixPerms || req.FixPermsOnly() != tt.fixOnly {
				t.Fatalf("mode helpers for %s = backup:%t prune:%t cleanup:%t fix:%t fixOnly:%t", tt.mode, req.DoBackup(), req.DoPrune(), req.DoCleanupStore(), req.FixPerms(), req.FixPermsOnly())
			}
			if got := OperationMode(req); got != tt.operation {
				t.Fatalf("OperationMode() = %q, want %q", got, tt.operation)
			}
		})
	}
}
