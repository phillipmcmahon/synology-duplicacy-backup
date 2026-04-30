package workflow

import "testing"

func TestNewConfigRequestProjectsOnlyConfigIntent(t *testing.T) {
	req := &Request{
		ConfigCommand:        "validate",
		Source:               "homes",
		RequestedStorageName: "onsite-usb",
		ConfigDir:            "/home/operator/.config/duplicacy-backup",
		SecretsDir:           "/home/operator/.config/duplicacy-backup/secrets",
		NotifyCommand:        "test",
		RestoreCommand:       "run",
		UpdateCommand:        "update",
		DoBackup:             true,
		DryRun:               true,
		JSONSummary:          true,
	}

	got := NewConfigRequest(req)
	if got.Command != "validate" || got.Label != "homes" || got.Target() != "onsite-usb" {
		t.Fatalf("config identity projection failed: %#v", got)
	}
	if got.ConfigDir != "/home/operator/.config/duplicacy-backup" || got.SecretsDir != "/home/operator/.config/duplicacy-backup/secrets" {
		t.Fatalf("config path projection failed: %#v", got)
	}
}

func TestNewConfigRequestFromNilIsZeroValue(t *testing.T) {
	got := NewConfigRequest(nil)
	if got != (ConfigRequest{}) {
		t.Fatalf("NewConfigRequest(nil) = %#v", got)
	}
	projected := NewConfigPlanRequest(nil)
	if projected != (ConfigPlanRequest{}) {
		t.Fatalf("NewConfigPlanRequest(nil) = %#v", projected)
	}
	if projected.Target() != "" {
		t.Fatalf("ConfigPlanRequest.Target() = %q", projected.Target())
	}
}

func TestConfigRequestPlanRequestCarriesPlannerInputsOnly(t *testing.T) {
	req := ConfigRequest{
		Command:    "explain",
		Label:      "homes",
		TargetName: "onsite-usb",
		ConfigDir:  "/home/operator/.config/duplicacy-backup",
		SecretsDir: "/home/operator/.config/duplicacy-backup/secrets",
	}

	got := req.PlanRequest()
	if got.Label != "homes" || got.Target() != "onsite-usb" || got.ConfigDir != "/home/operator/.config/duplicacy-backup" || got.SecretsDir != "/home/operator/.config/duplicacy-backup/secrets" {
		t.Fatalf("PlanRequest() = %#v", got)
	}
}
