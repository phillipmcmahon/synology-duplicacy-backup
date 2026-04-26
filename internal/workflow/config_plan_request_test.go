package workflow

import "testing"

func TestNewConfigPlanRequestProjectsOnlyPlannerInputs(t *testing.T) {
	req := &Request{
		Source:          "homes",
		RequestedTarget: "onsite-usb",
		ConfigDir:       "/home/operator/.config/duplicacy-backup",
		SecretsDir:      "/home/operator/.config/duplicacy-backup/secrets",
		DoBackup:        true,
		DryRun:          true,
		JSONSummary:     true,
		NotifyCommand:   "test",
		RestoreCommand:  "run",
		UpdateCommand:   "update",
	}

	got := NewConfigPlanRequest(req)
	if got.Label != "homes" || got.Target() != "onsite-usb" || got.ConfigDir != "/home/operator/.config/duplicacy-backup" || got.SecretsDir != "/home/operator/.config/duplicacy-backup/secrets" {
		t.Fatalf("NewConfigPlanRequest() = %#v", got)
	}
}

func TestDerivePlanFromConfigPlanRequestDoesNotCarryRuntimeModes(t *testing.T) {
	planner := NewConfigPlanner(DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	plan := planner.derivePlan(ConfigPlanRequest{
		Label:      "homes",
		TargetName: "onsite-usb",
		ConfigDir:  "/home/operator/.config/duplicacy-backup",
		SecretsDir: "/home/operator/.config/duplicacy-backup/secrets",
	})

	if plan.BackupLabel != "homes" || plan.TargetName() != "onsite-usb" {
		t.Fatalf("plan identity = %#v", plan)
	}
	if plan.DoBackup || plan.DoPrune || plan.DoCleanupStore || plan.FixPerms || plan.DryRun || plan.JSONSummary {
		t.Fatalf("config plan leaked runtime flags: %#v", plan)
	}
}
