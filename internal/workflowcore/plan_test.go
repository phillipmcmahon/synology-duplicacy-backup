package workflowcore

import (
	"path/filepath"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
)

func TestPlanSectionsReturnsDefensiveCopies(t *testing.T) {
	plan := &Plan{
		Request: PlanRequest{DoBackup: true, OperationMode: "Backup"},
		Config: PlanConfig{
			StorageName:  "onsite-usb",
			FilterLines:  []string{"- *.tmp"},
			PruneArgs:    []string{"-keep", "1:1"},
			BackupLabel:  "homes",
			PruneOptions: "-keep 1:1",
		},
		Paths: PlanPaths{RunTimestamp: "20260430-100000"},
	}

	sections := plan.Sections()
	sections.Config.FilterLines[0] = "- changed"
	sections.Config.PruneArgs[0] = "-changed"

	if plan.Config.FilterLines[0] != "- *.tmp" {
		t.Fatalf("FilterLines was not copied defensively")
	}
	if plan.Config.PruneArgs[0] != "-keep" {
		t.Fatalf("PruneArgs was not copied defensively")
	}

	if got := (*Plan)(nil).Sections(); got.Request != (PlanRequest{}) || got.Config.StorageName != "" || got.Paths.RunTimestamp != "" {
		t.Fatalf("nil Sections = %+v", got)
	}
}

func TestPlanIdentityHelpers(t *testing.T) {
	if (&Plan{Config: PlanConfig{Location: LocationRemote}}).IsRemoteLocation() != true {
		t.Fatalf("remote plan not detected")
	}
	if (&Plan{Config: PlanConfig{Location: LocationLocal}}).IsRemoteLocation() {
		t.Fatalf("local plan detected as remote")
	}
	if (*Plan)(nil).IsRemoteLocation() {
		t.Fatalf("nil plan detected as remote")
	}

	if got := (&Plan{Config: PlanConfig{StorageName: "onsite-usb"}}).ModeLabel(); got != "onsite-usb" {
		t.Fatalf("ModeLabel = %q", got)
	}
	if got := (&Plan{}).ModeLabel(); got != "not supplied" {
		t.Fatalf("empty ModeLabel = %q", got)
	}
	if got := (*Plan)(nil).ModeLabel(); got != "" {
		t.Fatalf("nil ModeLabel = %q", got)
	}

	if got := (&Plan{Paths: PlanPaths{WorkRoot: "/tmp/work"}}).WorkDir(); got != "/tmp/work/duplicacy" {
		t.Fatalf("WorkDir fallback = %q", got)
	}
	if got := (&Plan{Paths: PlanPaths{DuplicacyRoot: "/tmp/duplicacy"}}).WorkDir(); got != "/tmp/duplicacy" {
		t.Fatalf("WorkDir explicit = %q", got)
	}
	if got := (*Plan)(nil).TargetName(); got != "" {
		t.Fatalf("nil TargetName = %q", got)
	}
}

func TestPlanApplyConfig(t *testing.T) {
	cfg := &config.Config{
		Label:                       "homes",
		StorageName:                 "onsite-usb",
		Location:                    LocationLocal,
		SourcePath:                  "/volume1/homes",
		Storage:                     "/volumeUSB1/duplicacy/homes",
		Threads:                     8,
		Filter:                      "- *.tmp\n\n- cache",
		Prune:                       "-keep 7:30",
		PruneArgs:                   []string{"-keep", "7:30"},
		LogRetentionDays:            14,
		SafePruneMaxDeletePercent:   10,
		SafePruneMaxDeleteCount:     20,
		SafePruneMinTotalForPercent: 30,
	}
	plan := &Plan{
		Request: PlanRequest{DoBackup: true},
		Config:  PlanConfig{BackupLabel: "homes"},
		Paths:   PlanPaths{RunTimestamp: "20260430-100000"},
	}
	rt := Env{Getpid: func() int { return 1234 }}

	plan.ApplyConfig(cfg, rt)

	if plan.Config.StorageName != "onsite-usb" || plan.Config.Location != LocationLocal {
		t.Fatalf("config identity not applied: %+v", plan.Config)
	}
	if plan.Paths.SnapshotSource != "/volume1/homes" {
		t.Fatalf("SnapshotSource = %q", plan.Paths.SnapshotSource)
	}
	wantSnapshot := filepath.Join("/volume1", "homes-onsite-usb-20260430-100000-1234")
	if plan.Paths.SnapshotTarget != wantSnapshot {
		t.Fatalf("SnapshotTarget = %q, want %q", plan.Paths.SnapshotTarget, wantSnapshot)
	}
	if plan.Paths.RepositoryPath != wantSnapshot {
		t.Fatalf("RepositoryPath = %q, want snapshot target for backup", plan.Paths.RepositoryPath)
	}
	if plan.Paths.BackupTarget != "/volumeUSB1/duplicacy/homes" {
		t.Fatalf("BackupTarget = %q", plan.Paths.BackupTarget)
	}
	if len(plan.Config.FilterLines) != 2 || plan.Config.FilterLines[1] != "- cache" {
		t.Fatalf("FilterLines = %#v", plan.Config.FilterLines)
	}
	if plan.Config.PruneArgsDisplay != "-keep 7:30" {
		t.Fatalf("PruneArgsDisplay = %q", plan.Config.PruneArgsDisplay)
	}
	if plan.Config.SafePruneMinTotalForPercent != 30 {
		t.Fatalf("safe prune config not applied: %+v", plan.Config)
	}
}

func TestPlanApplyConfigForMaintenanceUsesSourceRepository(t *testing.T) {
	cfg := &config.Config{StorageName: "remote", Location: LocationRemote, SourcePath: "relative/source", Storage: "s3://bucket/path"}
	plan := &Plan{Config: PlanConfig{BackupLabel: "homes"}, Paths: PlanPaths{RunTimestamp: "ts"}}

	plan.ApplyConfig(cfg, Env{Getpid: func() int { return 99 }})

	if plan.Paths.RepositoryPath != "relative/source" {
		t.Fatalf("RepositoryPath = %q, want source path for non-backup", plan.Paths.RepositoryPath)
	}
	if plan.Paths.SnapshotTarget != "relative/source/homes-remote-ts-99" {
		t.Fatalf("SnapshotTarget = %q", plan.Paths.SnapshotTarget)
	}
}

func TestPlanApplyConfigNilSafe(t *testing.T) {
	var plan *Plan
	plan.ApplyConfig(&config.Config{}, Env{})
	plan.ApplyConfigIdentity(&config.Config{})

	plan = &Plan{}
	plan.ApplyConfig(nil, Env{})
	plan.ApplyConfigIdentity(nil)

	if plan.Config.StorageName != "" {
		t.Fatalf("nil config changed plan: %+v", plan)
	}
}
