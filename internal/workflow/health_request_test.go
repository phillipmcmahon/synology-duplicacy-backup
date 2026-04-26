package workflow

import "testing"

func TestNewHealthRequestProjectsOnlyHealthIntent(t *testing.T) {
	req := &Request{
		HealthCommand:   "verify",
		Source:          "homes",
		RequestedTarget: "onsite-usb",
		ConfigDir:       "/home/operator/.config/duplicacy-backup",
		SecretsDir:      "/home/operator/.config/duplicacy-backup/secrets",
		JSONSummary:     true,
		Verbose:         true,
		NotifyCommand:   "test",
		RestoreCommand:  "run",
		UpdateCommand:   "update",
		DoBackup:        true,
		DryRun:          true,
	}

	got := NewHealthRequest(req)
	if got.Command != "verify" || got.Label != "homes" || got.Target() != "onsite-usb" {
		t.Fatalf("health identity projection failed: %#v", got)
	}
	if got.ConfigDir != "/home/operator/.config/duplicacy-backup" || got.SecretsDir != "/home/operator/.config/duplicacy-backup/secrets" || !got.JSONSummary || !got.Verbose {
		t.Fatalf("health option projection failed: %#v", got)
	}
	plan := got.PlanRequest()
	if plan.Label != "homes" || plan.Target() != "onsite-usb" || plan.ConfigDir != "/home/operator/.config/duplicacy-backup" || plan.SecretsDir != "/home/operator/.config/duplicacy-backup/secrets" {
		t.Fatalf("PlanRequest() = %#v", plan)
	}
}

func TestNewHealthRequestFromNilIsZeroValue(t *testing.T) {
	got := NewHealthRequest(nil)
	if got != (HealthRequest{}) {
		t.Fatalf("NewHealthRequest(nil) = %#v", got)
	}
}
