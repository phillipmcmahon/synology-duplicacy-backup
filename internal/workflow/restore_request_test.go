package workflow

import "testing"

func TestNewRestoreRequestProjectsOnlyRestoreIntent(t *testing.T) {
	req := &Request{
		RestoreCommand:    "run",
		Source:            "homes",
		RequestedTarget:   "onsite-usb",
		ConfigDir:         "/etc/duplicacy-backup",
		SecretsDir:        "/root/.secrets",
		JSONSummary:       true,
		DryRun:            true,
		RestoreWorkspace:  "/volume1/restore-drills/homes",
		RestoreRevision:   2403,
		RestorePath:       "phillipmcmahon/code",
		RestorePathPrefix: "phillipmcmahon",
		RestoreLimit:      25,
		RestoreYes:        true,
		DoBackup:          true,
		NotifyProvider:    "ntfy",
		UpdateVersion:     "v9.9.9",
	}

	got := NewRestoreRequest(req)
	if got.Command != "run" || got.Label != "homes" || got.Target() != "onsite-usb" {
		t.Fatalf("restore identity projection failed: %#v", got)
	}
	if got.ConfigDir != "/etc/duplicacy-backup" || got.SecretsDir != "/root/.secrets" {
		t.Fatalf("restore config projection failed: %#v", got)
	}
	if got.Workspace != "/volume1/restore-drills/homes" || got.Revision != 2403 || got.Path != "phillipmcmahon/code" || got.PathPrefix != "phillipmcmahon" {
		t.Fatalf("restore operation projection failed: %#v", got)
	}
	if !got.JSONSummary || !got.DryRun || !got.Yes || got.Limit != 25 {
		t.Fatalf("restore option projection failed: %#v", got)
	}
}

func TestRestoreRequestConfigRequestKeepsPlannerInputsOnly(t *testing.T) {
	restoreReq := RestoreRequest{
		Label:       "homes",
		TargetName:  "onsite-usb",
		ConfigDir:   "/etc/duplicacy-backup",
		SecretsDir:  "/root/.secrets",
		DryRun:      true,
		Workspace:   "/volume1/restore-drills/homes",
		Revision:    2403,
		Path:        "docs/readme.md",
		JSONSummary: true,
	}

	got := restoreReq.ConfigRequest()
	if got.Source != "homes" || got.RequestedTarget != "onsite-usb" {
		t.Fatalf("config request identity = %#v", got)
	}
	if got.ConfigDir != "/etc/duplicacy-backup" || got.SecretsDir != "/root/.secrets" {
		t.Fatalf("config request paths = %#v", got)
	}
	if got.DryRun || got.JSONSummary || got.RestoreRevision != 0 || got.RestorePath != "" {
		t.Fatalf("config request leaked restore-only fields: %#v", got)
	}
	if got.DoBackup || got.DoPrune || got.DoCleanupStore || got.FixPerms {
		t.Fatalf("config request should not carry runtime modes: %#v", got)
	}
}
