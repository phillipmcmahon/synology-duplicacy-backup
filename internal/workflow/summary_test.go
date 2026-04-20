package workflow

import (
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

func TestSummaryLines_FixPermsOnlyLayout(t *testing.T) {
	plan := &Plan{
		FixPermsOnly:  true,
		DryRun:        true,
		Target:        "onsite-usb",
		StorageType:   storageTypeFilesystem,
		Location:      locationLocal,
		LocalOwner:    "backupuser",
		LocalGroup:    "users",
		BackupTarget:  "/backups/homes",
		OperationMode: "Fix permissions",
	}

	lines := SummaryLines(plan)
	if len(lines) != 8 {
		t.Fatalf("len(lines) = %d, want 8", len(lines))
	}

	expected := []string{
		"Operation Mode",
		"Target",
		"Type",
		"Location",
		"Destination",
		"Local Owner",
		"Local Group",
		"Dry Run",
	}
	for i, label := range expected {
		if lines[i].Label != label {
			t.Fatalf("lines[%d].Label = %q, want %q", i, lines[i].Label, label)
		}
	}
}

func TestSummaryLines_DuplicacyStorageIncludesSecrets(t *testing.T) {
	plan := &Plan{
		Verbose:                     true,
		DoBackup:                    true,
		Threads:                     4,
		LogRetentionDays:            30,
		SafePruneMaxDeletePercent:   10,
		SafePruneMaxDeleteCount:     25,
		SafePruneMinTotalForPercent: 20,
		Secrets: &secrets.Secrets{
			Keys: map[string]string{
				"s3_id":     "1234567890123456789012345678",
				"s3_secret": "12345678901234567890123456789012345678901234567890123",
			},
		},
		BackupLabel:    "homes",
		Target:         "offsite-storj",
		StorageType:    storageTypeDuplicacy,
		Location:       locationRemote,
		SnapshotSource: "/volume1/homes",
		RepositoryPath: "/volume1/homes-snap",
		WorkRoot:       "/tmp/work",
		BackupTarget:   "/backups/homes",
		ConfigFile:     "/config/homes-backup.toml",
		SecretsDir:     "/root/.secrets",
		SecretsFile:    "/root/.secrets/homes-secrets.toml",
		ModeDisplay:    "offsite-storj",
		OperationMode:  "Backup",
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
		Verbose:                     true,
		DoBackup:                    true,
		Threads:                     4,
		LogRetentionDays:            30,
		SafePruneMaxDeletePercent:   10,
		SafePruneMaxDeleteCount:     25,
		SafePruneMinTotalForPercent: 20,
		Secrets: &secrets.Secrets{
			Keys: map[string]string{
				"s3_id":     "1234567890123456789012345678",
				"s3_secret": "12345678901234567890123456789012345678901234567890123",
			},
		},
		BackupLabel:    "homes",
		Target:         "onsite-rustfs",
		StorageType:    storageTypeDuplicacy,
		Location:       locationLocal,
		SnapshotSource: "/volume1/homes",
		RepositoryPath: "/volume1/homes-snap",
		WorkRoot:       "/tmp/work",
		BackupTarget:   "s3://rustfs.local/bucket/homes",
		ConfigFile:     "/config/homes-backup.toml",
		SecretsDir:     "/root/.secrets",
		SecretsFile:    "/root/.secrets/homes-secrets.toml",
		ModeDisplay:    "onsite-rustfs",
		OperationMode:  "Backup",
	}

	lines := SummaryLines(plan)
	labels := make(map[string]bool, len(lines))
	for _, line := range lines {
		labels[line.Label] = true
	}
	for _, want := range []string{"Type", "Location", "Storage Keys"} {
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
		DoBackup:       true,
		DoPrune:        true,
		FixPerms:       true,
		ForcePrune:     true,
		BackupLabel:    "homes",
		Target:         "onsite-usb",
		StorageType:    storageTypeFilesystem,
		Location:       locationLocal,
		SnapshotSource: "/volume1/homes",
		RepositoryPath: "/volume1/homes-snap",
		BackupTarget:   "/backups/homes",
		ConfigFile:     "/config/homes-backup.toml",
		ModeDisplay:    "onsite-usb",
		OperationMode:  "Backup + Forced prune + Fix permissions",
		WorkRoot:       "/tmp/work",
		Threads:        16,
		Filter:         "exclude",
		LocalOwner:     "phillip",
		LocalGroup:     "users",
	}

	lines := SummaryLines(plan)
	labels := make(map[string]bool, len(lines))
	for _, line := range lines {
		labels[line.Label] = true
	}

	if labels["Work Dir"] || labels["Threads"] || labels["Filter"] || labels["Prune Options"] {
		t.Fatalf("expected compact default summary, got %+v", lines)
	}
	if !labels["Operation Mode"] || !labels["Config File"] || !labels["Destination"] || !labels["Local Owner"] {
		t.Fatalf("expected essential summary fields, got %+v", lines)
	}
	wantOrder := []string{"Operation Mode", "Target", "Type", "Location", "Config File", "Source", "Snapshot", "Destination", "Force Prune", "Local Owner", "Local Group"}
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

func TestOperationMode_CombinedOperations(t *testing.T) {
	req := &Request{DoBackup: true, DoPrune: true, FixPerms: true}
	if got := OperationMode(req); got != "Backup + Safe prune + Fix permissions" {
		t.Fatalf("OperationMode() = %q", got)
	}
}

func TestOperationMode_ForcedPrune(t *testing.T) {
	req := &Request{DoBackup: true, DoPrune: true, ForcePrune: true, FixPerms: true}
	if got := OperationMode(req); got != "Backup + Forced prune + Fix permissions" {
		t.Fatalf("OperationMode() = %q", got)
	}
}

func TestOperationMode_BackupDeepPruneWithFixPerms(t *testing.T) {
	req := &Request{DoBackup: true, DoCleanupStore: true, FixPerms: true}
	if got := OperationMode(req); got != "Backup + Storage cleanup + Fix permissions" {
		t.Fatalf("OperationMode() = %q", got)
	}
}

func TestOperationMode_BackupForcedDeepPruneWithFixPerms(t *testing.T) {
	req := &Request{DoBackup: true, DoPrune: true, DoCleanupStore: true, ForcePrune: true, FixPerms: true}
	if got := OperationMode(req); got != "Backup + Forced prune + Storage cleanup + Fix permissions" {
		t.Fatalf("OperationMode() = %q", got)
	}
}
