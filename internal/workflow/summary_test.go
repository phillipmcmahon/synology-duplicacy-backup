package workflow

import (
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

func TestSummaryLines_DuplicacyStorageIncludesSecrets(t *testing.T) {
	plan := &Plan{
		Request: PlanRequest{
			Verbose:       true,
			DoBackup:      true,
			OperationMode: "Backup",
		},
		Config: PlanConfig{
			Threads:                     4,
			LogRetentionDays:            30,
			SafePruneMaxDeletePercent:   10,
			SafePruneMaxDeleteCount:     25,
			SafePruneMinTotalForPercent: 20,
			BackupLabel:                 "homes",
			Target:                      "offsite-storj",
			Location:                    locationRemote,
		},
		Secrets: &secrets.Secrets{
			Keys: map[string]string{
				"s3_id":     "1234567890123456789012345678",
				"s3_secret": "12345678901234567890123456789012345678901234567890123",
			},
		},
		Paths: PlanPaths{
			SnapshotSource: "/volume1/homes",
			RepositoryPath: "/volume1/homes-snap",
			WorkRoot:       "/tmp/work",
			BackupTarget:   "/backups/homes",
			ConfigFile:     "/config/homes-backup.toml",
			SecretsDir:     "/home/operator/.config/duplicacy-backup/secrets",
			SecretsFile:    "/home/operator/.config/duplicacy-backup/secrets/homes-secrets.toml",
		},
		Display: PlanDisplay{ModeDisplay: "offsite-storj"},
	}

	lines := SummaryLines(plan)
	foundSecretsFile := false
	foundSecretsDir := false
	foundStorageKeys := false
	for _, line := range lines {
		if line.Label == "Secrets Dir" {
			foundSecretsDir = true
		}
		if line.Label == "Secrets File" {
			foundSecretsFile = true
		}
		if line.Label == "Storage Keys" {
			foundStorageKeys = true
		}
	}
	if !foundSecretsDir || !foundSecretsFile || !foundStorageKeys {
		t.Fatalf("expected secrets lines in summary, got %+v", lines)
	}
}

func TestSummaryLines_LocalDuplicacyStorageIncludesNeutralSecretLabels(t *testing.T) {
	plan := &Plan{
		Request: PlanRequest{
			Verbose:       true,
			DoBackup:      true,
			OperationMode: "Backup",
		},
		Config: PlanConfig{
			Threads:                     4,
			LogRetentionDays:            30,
			SafePruneMaxDeletePercent:   10,
			SafePruneMaxDeleteCount:     25,
			SafePruneMinTotalForPercent: 20,
			BackupLabel:                 "homes",
			Target:                      "onsite-rustfs",
			Location:                    locationLocal,
		},
		Secrets: &secrets.Secrets{
			Keys: map[string]string{
				"s3_id":     "1234567890123456789012345678",
				"s3_secret": "12345678901234567890123456789012345678901234567890123",
			},
		},
		Paths: PlanPaths{
			SnapshotSource: "/volume1/homes",
			RepositoryPath: "/volume1/homes-snap",
			WorkRoot:       "/tmp/work",
			BackupTarget:   "s3://rustfs.local/bucket/homes",
			ConfigFile:     "/config/homes-backup.toml",
			SecretsDir:     "/home/operator/.config/duplicacy-backup/secrets",
			SecretsFile:    "/home/operator/.config/duplicacy-backup/secrets/homes-secrets.toml",
		},
		Display: PlanDisplay{ModeDisplay: "onsite-rustfs"},
	}

	lines := SummaryLines(plan)
	labels := make(map[string]bool, len(lines))
	for _, line := range lines {
		labels[line.Label] = true
	}
	for _, want := range []string{"Location", "Storage Keys"} {
		if !labels[want] {
			t.Fatalf("missing %q in summary lines: %+v", want, lines)
		}
	}
	for _, old := range []string{"Remote Access Key", "Remote Secret Key", "Storage Access Key", "Storage Secret Key"} {
		if labels[old] {
			t.Fatalf("summary should not use remote-specific label %q: %+v", old, lines)
		}
	}
}

func TestSummaryLines_DefaultOutputIsCompact(t *testing.T) {
	plan := &Plan{
		Request: PlanRequest{
			DoBackup:      true,
			DoPrune:       true,
			ForcePrune:    true,
			OperationMode: "Backup + Forced prune",
		},
		Config: PlanConfig{
			BackupLabel: "homes",
			Target:      "onsite-usb",
			Location:    locationLocal,
			Threads:     16,
			Filter:      "exclude",
		},
		Paths: PlanPaths{
			SnapshotSource: "/volume1/homes",
			RepositoryPath: "/volume1/homes-snap",
			BackupTarget:   "/backups/homes",
			ConfigFile:     "/config/homes-backup.toml",
			WorkRoot:       "/tmp/work",
		},
		Display: PlanDisplay{ModeDisplay: "onsite-usb"},
	}

	lines := SummaryLines(plan)
	labels := make(map[string]bool, len(lines))
	for _, line := range lines {
		labels[line.Label] = true
	}

	if labels["Work Dir"] || labels["Threads"] || labels["Filter"] || labels["Prune Options"] {
		t.Fatalf("expected compact default summary, got %+v", lines)
	}
	if !labels["Operation Mode"] || !labels["Config File"] || !labels["Storage"] || !labels["Force Prune"] {
		t.Fatalf("expected essential summary fields, got %+v", lines)
	}
	wantOrder := []string{"Operation Mode", "Target", "Location", "Config File", "Source Path", "Snapshot", "Storage", "Force Prune"}
	got := make([]string, 0, len(lines))
	for _, line := range lines {
		got = append(got, line.Label)
	}
	for i, label := range wantOrder {
		if got[i] != label {
			t.Fatalf("summary label order = %v, want prefix %v", got, wantOrder)
		}
	}
}

func TestOperationMode_Backup(t *testing.T) {
	req := &RuntimeRequest{Mode: RuntimeModeBackup}
	if got := OperationMode(req); got != "Backup" {
		t.Fatalf("OperationMode() = %q", got)
	}
}

func TestOperationMode_ForcedPrune(t *testing.T) {
	req := &RuntimeRequest{Mode: RuntimeModePrune, ForcePrune: true}
	if got := OperationMode(req); got != "Forced prune" {
		t.Fatalf("OperationMode() = %q", got)
	}
}

func TestOperationMode_CleanupStorage(t *testing.T) {
	req := &RuntimeRequest{Mode: RuntimeModeCleanupStorage}
	if got := OperationMode(req); got != "Storage cleanup" {
		t.Fatalf("OperationMode() = %q", got)
	}
}
