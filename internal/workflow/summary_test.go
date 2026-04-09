package workflow

import (
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

func TestSummaryLines_FixPermsOnlyLayout(t *testing.T) {
	plan := &Plan{
		FixPermsOnly:  true,
		DryRun:        true,
		LocalOwner:    "backupuser",
		LocalGroup:    "users",
		BackupTarget:  "/backups/homes",
		OperationMode: "Fix permissions only",
	}

	lines := SummaryLines(plan)
	if len(lines) != 5 {
		t.Fatalf("len(lines) = %d, want 5", len(lines))
	}

	expected := []string{
		"Operation Mode",
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

func TestSummaryLines_RemoteIncludesSecrets(t *testing.T) {
	plan := &Plan{
		DoBackup:                    true,
		RemoteMode:                  true,
		Threads:                     4,
		LogRetentionDays:            30,
		SafePruneMaxDeletePercent:   10,
		SafePruneMaxDeleteCount:     25,
		SafePruneMinTotalForPercent: 20,
		Secrets: &secrets.Secrets{
			StorjS3ID:     "1234567890123456789012345678",
			StorjS3Secret: "12345678901234567890123456789012345678901234567890123",
		},
		BackupLabel:    "homes",
		SnapshotSource: "/volume1/homes",
		RepositoryPath: "/volume1/homes-snap",
		WorkRoot:       "/tmp/work",
		BackupTarget:   "/backups/homes",
		ConfigFile:     "/config/homes-backup.toml",
		SecretsDir:     "/root/.secrets",
		SecretsFile:    "/root/.secrets/duplicacy-homes.toml",
		ModeDisplay:    "REMOTE",
		OperationMode:  "Backup only",
	}

	lines := SummaryLines(plan)
	foundSecretsFile := false
	foundSecretsDir := false
	for _, line := range lines {
		if line.Label == "Secrets Dir" {
			foundSecretsDir = true
		}
		if line.Label == "Secrets File" {
			foundSecretsFile = true
		}
	}
	if !foundSecretsDir || !foundSecretsFile {
		t.Fatalf("expected secrets lines in summary, got %+v", lines)
	}
}
