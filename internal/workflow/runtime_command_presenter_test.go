package workflow

import "testing"

func TestRuntimeCommandPresenterFormatsDryRunCommandsFromPlanData(t *testing.T) {
	plan := &Plan{
		Config: PlanConfig{
			Threads:          4,
			PruneArgsDisplay: "-keep 7:30",
		},
		Paths: PlanPaths{
			SnapshotSource: "/volume1/homes",
			SnapshotTarget: "/volume1/homes-snap",
			DuplicacyRoot:  "/tmp/work/duplicacy",
			WorkRoot:       "/tmp/work",
		},
	}
	commands := NewRuntimeCommandPresenter(plan)

	cases := map[string]string{
		"snapshot create":   commands.SnapshotCreate(),
		"snapshot delete":   commands.SnapshotDelete(),
		"work dir create":   commands.WorkDirCreate(),
		"preferences write": commands.PreferencesWrite(),
		"filters write":     commands.FiltersWrite(),
		"dir perms":         commands.WorkDirDirPerms(),
		"file perms":        commands.WorkDirFilePerms(),
		"backup":            commands.Backup(),
		"validate repo":     commands.ValidateRepo(),
		"prune preview":     commands.PrunePreview(),
		"policy prune":      commands.PolicyPrune(),
		"cleanup storage":   commands.CleanupStorage(),
		"work dir remove":   commands.WorkDirRemove(""),
	}

	want := map[string]string{
		"snapshot create":   "btrfs subvolume snapshot -r /volume1/homes /volume1/homes-snap",
		"snapshot delete":   "btrfs subvolume delete /volume1/homes-snap",
		"work dir create":   "mkdir -p /tmp/work/duplicacy/.duplicacy",
		"preferences write": "write JSON preferences to /tmp/work/duplicacy/.duplicacy/preferences",
		"filters write":     "write filters to /tmp/work/duplicacy/.duplicacy/filters",
		"dir perms":         "find /tmp/work/duplicacy -type d -exec chmod 770 {} +",
		"file perms":        "find /tmp/work/duplicacy -type f -exec chmod 660 {} +",
		"backup":            "duplicacy backup -stats -threads 4",
		"validate repo":     "duplicacy list -files",
		"prune preview":     "duplicacy prune -keep 7:30 -dry-run",
		"policy prune":      "duplicacy prune -keep 7:30",
		"cleanup storage":   "duplicacy prune -exhaustive -exclusive",
		"work dir remove":   "rm -rf /tmp/work",
	}

	for name, got := range cases {
		if got != want[name] {
			t.Fatalf("%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestRuntimeCommandPresenterFormatsPruneWithoutPolicyArgs(t *testing.T) {
	commands := NewRuntimeCommandPresenter(&Plan{})
	if got := commands.PrunePreview(); got != "duplicacy prune -dry-run" {
		t.Fatalf("PrunePreview() = %q", got)
	}
	if got := commands.PolicyPrune(); got != "duplicacy prune" {
		t.Fatalf("PolicyPrune() = %q", got)
	}
	if got := commands.WorkDirRemove("/tmp/actual-work"); got != "rm -rf /tmp/actual-work" {
		t.Fatalf("WorkDirRemove() = %q", got)
	}
}
