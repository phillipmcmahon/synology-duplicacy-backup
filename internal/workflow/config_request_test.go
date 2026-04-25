package workflow

import "testing"

func TestNewConfigRequestProjectsOnlyConfigIntent(t *testing.T) {
	req := &Request{
		ConfigCommand:   "validate",
		Source:          "homes",
		RequestedTarget: "onsite-usb",
		ConfigDir:       "/etc/duplicacy-backup",
		SecretsDir:      "/root/.secrets",
		NotifyCommand:   "test",
		RestoreCommand:  "run",
		UpdateCommand:   "update",
		DoBackup:        true,
		DryRun:          true,
		JSONSummary:     true,
	}

	got := NewConfigRequest(req)
	if got.Command != "validate" || got.Label != "homes" || got.Target() != "onsite-usb" {
		t.Fatalf("config identity projection failed: %#v", got)
	}
	if got.ConfigDir != "/etc/duplicacy-backup" || got.SecretsDir != "/root/.secrets" {
		t.Fatalf("config path projection failed: %#v", got)
	}
}

func TestNewConfigRequestFromNilIsZeroValue(t *testing.T) {
	got := NewConfigRequest(nil)
	if got != (ConfigRequest{}) {
		t.Fatalf("NewConfigRequest(nil) = %#v", got)
	}
	plan := NewConfigPlanRequest(nil)
	if plan != (ConfigPlanRequest{}) {
		t.Fatalf("NewConfigPlanRequest(nil) = %#v", plan)
	}
	if plan.Target() != "" {
		t.Fatalf("ConfigPlanRequest.Target() = %q", plan.Target())
	}
}

func TestConfigRequestPlanRequestCarriesPlannerInputsOnly(t *testing.T) {
	req := ConfigRequest{
		Command:    "explain",
		Label:      "homes",
		TargetName: "onsite-usb",
		ConfigDir:  "/etc/duplicacy-backup",
		SecretsDir: "/root/.secrets",
	}

	got := req.PlanRequest()
	if got.Label != "homes" || got.Target() != "onsite-usb" || got.ConfigDir != "/etc/duplicacy-backup" || got.SecretsDir != "/root/.secrets" {
		t.Fatalf("PlanRequest() = %#v", got)
	}
}
