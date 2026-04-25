package workflow

import "testing"

func TestNewHealthRequestProjectsOnlyHealthIntent(t *testing.T) {
	req := &Request{
		HealthCommand:   "verify",
		Source:          "homes",
		RequestedTarget: "onsite-usb",
		ConfigDir:       "/etc/duplicacy-backup",
		SecretsDir:      "/root/.secrets",
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
	if got.ConfigDir != "/etc/duplicacy-backup" || got.SecretsDir != "/root/.secrets" {
		t.Fatalf("health path projection failed: %#v", got)
	}
	if !got.JSONSummary || !got.Verbose {
		t.Fatalf("health flag projection failed: %#v", got)
	}
}

func TestHealthRequestPlanRequestCarriesPlannerInputsOnly(t *testing.T) {
	req := HealthRequest{
		Command:     "doctor",
		Label:       "homes",
		TargetName:  "onsite-usb",
		ConfigDir:   "/etc/duplicacy-backup",
		SecretsDir:  "/root/.secrets",
		JSONSummary: true,
		Verbose:     true,
	}

	got := req.PlanRequest()
	if got.Label != "homes" || got.Target() != "onsite-usb" || got.ConfigDir != "/etc/duplicacy-backup" || got.SecretsDir != "/root/.secrets" {
		t.Fatalf("PlanRequest() = %#v", got)
	}
}
