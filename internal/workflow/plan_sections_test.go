package workflow

import "testing"

func TestPlanSectionsAndFallbacks(t *testing.T) {
	if got := (*Plan)(nil).Sections(); got.Request.OperationMode != "" || got.Config.Target != "" || got.Paths.WorkRoot != "" || got.Display.BackupCommand != "" {
		t.Fatalf("nil Sections() = %#v", got)
	}
	if (*Plan)(nil).TargetName() != "" {
		t.Fatal("nil TargetName should be empty")
	}

	plan := &Plan{
		DoBackup:                true,
		DoPrune:                 true,
		DoCleanupStore:          true,
		ForcePrune:              true,
		DryRun:                  true,
		Verbose:                 true,
		JSONSummary:             true,
		NeedsDuplicacySetup:     true,
		NeedsSnapshot:           true,
		DefaultNotice:           "notice",
		OperationMode:           "Backup",
		Target:                  "onsite-usb",
		Location:                locationLocal,
		BackupLabel:             "homes",
		Threads:                 16,
		FilterLines:             []string{"-e *.tmp"},
		PruneArgs:               []string{"-keep", "1:7"},
		WorkRoot:                "/tmp/work",
		DuplicacyRoot:           "/tmp/work/duplicacy",
		BackupTarget:            "/backups/homes",
		SnapshotCreateCommand:   "btrfs subvolume snapshot",
		BackupCommand:           "duplicacy backup",
		CleanupStorageCommand:   "duplicacy prune -exclusive",
		WorkDirRemoveCommand:    "rm -rf /tmp/work",
		ModeDisplay:             "local",
		LogRetentionDays:        28,
		SafePruneMaxDeleteCount: 25,
	}

	sections := plan.Sections()
	if !sections.Request.DoBackup || !sections.Request.DoPrune || !sections.Request.DoCleanupStore ||
		!sections.Request.ForcePrune || !sections.Request.DryRun ||
		!sections.Request.Verbose || !sections.Request.JSONSummary || !sections.Request.NeedsDuplicacySetup ||
		!sections.Request.NeedsSnapshot || sections.Request.DefaultNotice != "notice" || sections.Request.OperationMode != "Backup" {
		t.Fatalf("request section = %#v", sections.Request)
	}
	if sections.Config.Target != "onsite-usb" || sections.Config.Location != locationLocal || sections.Config.BackupLabel != "homes" ||
		sections.Config.Threads != 16 || sections.Config.LogRetentionDays != 28 || sections.Config.SafePruneMaxDeleteCount != 25 {
		t.Fatalf("config section = %#v", sections.Config)
	}
	if sections.Paths.WorkRoot != "/tmp/work" || sections.Paths.DuplicacyRoot != "/tmp/work/duplicacy" || sections.Paths.BackupTarget != "/backups/homes" {
		t.Fatalf("paths section = %#v", sections.Paths)
	}
	if sections.Display.BackupCommand != "duplicacy backup" || sections.Display.CleanupStorageCommand == "" {
		t.Fatalf("display section = %#v", sections.Display)
	}

	sections.Config.FilterLines[0] = "mutated"
	sections.Config.PruneArgs[0] = "mutated"
	if plan.FilterLines[0] == "mutated" || plan.PruneArgs[0] == "mutated" {
		t.Fatal("Sections should copy slices")
	}
	if plan.TargetName() != "onsite-usb" || plan.WorkDir() != "/tmp/work/duplicacy" || plan.IsRemoteLocation() {
		t.Fatalf("plan helpers failed")
	}
	plan.DuplicacyRoot = ""
	if plan.WorkDir() != "/tmp/work/duplicacy" {
		t.Fatalf("WorkDir fallback = %q", plan.WorkDir())
	}
}
