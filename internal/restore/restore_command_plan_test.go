package restore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
)

func TestHandleRestoreCommand_PlanLocalReadOnlyWithState(t *testing.T) {
	stubRestoreWorkspaceTime(t, time.Date(2026, 4, 24, 8, 15, 30, 0, time.Local))
	configDir := t.TempDir()
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", "/volume1/homes", "/backups", 4, "-keep 0:365"))
	if err := saveRunState(meta, "homes", "onsite-usb", &RunState{
		LastSuccessfulBackupRevision: 2403,
		LastSuccessfulBackupAt:       "2026-04-20T02:30:00Z",
	}); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	req := &Request{RestoreCommand: "plan", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}

	for _, token := range []string{
		"Restore plan for homes/onsite-usb",
		"Read Only",
		"true",
		"Executes Restore",
		"false",
		"Section: Resolved",
		"Source Path",
		"/volume1/homes",
		"Storage",
		"/backups/homes",
		"Section: Safe Workspace",
		"/volume1/restore-drills/homes-onsite-usb-20260424-081530",
		"Snapshot ID",
		"data",
		"Section: Revision Signal",
		"Latest Revision",
		"2403 (2026-04-20T02:30:00Z)",
		"Section: Suggested Commands",
		"duplicacy init 'data' '/backups/homes'",
		"duplicacy list -files -r <revision>",
		"duplicacy restore -r <revision> -stats -ignore-owner",
		`duplicacy restore -r <revision> -stats -ignore-owner -- "relative/path/from/snapshot"`,
		"rsync -a --dry-run",
		"Section: Safety",
		"not performed by this command",
		"docs/restore-drills.md",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}

	if strings.Contains(out, "mkdir -p /volume1/restore-drills") {
		t.Fatalf("workspace command should shell-quote paths:\n%s", out)
	}
	if _, err := os.Stat("/volume1/restore-drills/homes-onsite-usb-20260424-081530"); err == nil {
		t.Fatalf("restore plan command must not create the suggested workspace")
	}
}

func TestResolvedRestoreSelectWorkspace_UsesRevisionTimestampAndID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "restore-drills")
	req := &RestoreRequest{Label: "homes", TargetName: "onsite-usb", WorkspaceRoot: root}
	plan := &Plan{Paths: PlanPaths{SnapshotSource: "/tmp/homes"}}
	revision := duplicacy.RevisionInfo{
		Revision:  2403,
		CreatedAt: time.Date(2026, 4, 24, 7, 0, 0, 0, time.Local),
	}

	deps := defaultRestoreDeps()
	got, err := resolvedRestoreSelectWorkspace(req, plan, revision, deps)
	if err != nil {
		t.Fatalf("resolvedRestoreSelectWorkspace() error = %v", err)
	}
	want := filepath.Join(root, "homes-onsite-usb-20260424-070000-rev2403")
	if got != want {
		t.Fatalf("resolvedRestoreSelectWorkspace() = %q, want %q", got, want)
	}
}

func TestResolvedRestoreSelectWorkspace_UsesExplicitWorkspace(t *testing.T) {
	req := &RestoreRequest{Label: "homes", TargetName: "onsite-usb", Workspace: "/restore/exact"}
	plan := &Plan{Paths: PlanPaths{SnapshotSource: "/tmp/homes"}}
	revision := duplicacy.RevisionInfo{
		Revision:  2403,
		CreatedAt: time.Date(2026, 4, 24, 7, 0, 0, 0, time.Local),
	}

	got, err := resolvedRestoreSelectWorkspace(req, plan, revision, defaultRestoreDeps())
	if err != nil {
		t.Fatalf("resolvedRestoreSelectWorkspace() error = %v", err)
	}
	want := "/restore/exact"
	if got != want {
		t.Fatalf("resolvedRestoreSelectWorkspace() = %q, want %q", got, want)
	}
}

func TestValidateRestoreWorkspaceSelection_RejectsWorkspaceAndRoot(t *testing.T) {
	err := validateRestoreWorkspaceSelection(&RestoreRequest{Workspace: "/restore/exact", WorkspaceRoot: "/restore/root"})
	if err == nil || !strings.Contains(err.Error(), "--workspace and --workspace-root cannot be used together") {
		t.Fatalf("validateRestoreWorkspaceSelection() err = %v", err)
	}
}

func TestValidateRestoreWorkspaceSelection_RejectsRelativeRoot(t *testing.T) {
	err := validateRestoreWorkspaceSelection(&RestoreRequest{WorkspaceRoot: "restore-drills"})
	if err == nil || !strings.Contains(err.Error(), "--workspace-root must be an absolute path") {
		t.Fatalf("validateRestoreWorkspaceSelection() err = %v", err)
	}
}

func TestValidateRestoreWorkspaceSelection_RejectsWorkspaceAndTemplate(t *testing.T) {
	err := validateRestoreWorkspaceSelection(&RestoreRequest{Workspace: "/restore/exact", WorkspaceTemplate: "{label}-{storage}"})
	if err == nil || !strings.Contains(err.Error(), "--workspace and --workspace-template cannot be used together") {
		t.Fatalf("validateRestoreWorkspaceSelection() err = %v", err)
	}
}

func TestApplyRestoreConfigDefaults_ExactWorkspaceOverridesConfigDefaults(t *testing.T) {
	req := &RestoreRequest{Workspace: "/restore/exact"}
	applyRestoreConfigDefaults(req, &config.Config{
		RestoreWorkspaceRoot:     "/restore/root",
		RestoreWorkspaceTemplate: "{label}-{storage}",
	})

	if req.WorkspaceRoot != "" || req.WorkspaceTemplate != "" {
		t.Fatalf("exact workspace should ignore config defaults: %+v", req)
	}
	if err := validateRestoreWorkspaceSelection(req); err != nil {
		t.Fatalf("validateRestoreWorkspaceSelection() error = %v", err)
	}
}

func TestValidateRestoreWorkspaceSelection_RejectsUnsupportedTemplateVariable(t *testing.T) {
	err := validateRestoreWorkspaceSelection(&RestoreRequest{WorkspaceTemplate: "{label}-{unknown}"})
	if err == nil || !strings.Contains(err.Error(), "unsupported variable {unknown}") {
		t.Fatalf("validateRestoreWorkspaceSelection() err = %v", err)
	}
}

func TestValidateRestoreWorkspaceSelection_RejectsTemplatePath(t *testing.T) {
	err := validateRestoreWorkspaceSelection(&RestoreRequest{WorkspaceTemplate: "{label}/{storage}"})
	if err == nil || !strings.Contains(err.Error(), "must produce one folder name") {
		t.Fatalf("validateRestoreWorkspaceSelection() err = %v", err)
	}
}

func TestValidateRestoreWorkspaceRoot_RejectsMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-restore-root")
	err := validateRestoreWorkspaceRoot(&RestoreRequest{WorkspaceRoot: root})
	if err == nil || !strings.Contains(err.Error(), "--workspace-root does not exist") {
		t.Fatalf("validateRestoreWorkspaceRoot() err = %v", err)
	}
}

func TestHandleRestoreCommand_RejectsWorkspaceAndRoot(t *testing.T) {
	req := &Request{RestoreCommand: "run", Source: "homes", RequestedTarget: "onsite-usb", RestoreWorkspace: "/restore/exact", RestoreWorkspaceRoot: "/restore/root", RestoreRevision: 2403, RestoreYes: true}
	_, err := restoreHandleCommand(req, MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "--workspace and --workspace-root cannot be used together") {
		t.Fatalf("restoreHandleCommand() err = %v", err)
	}
}

func TestResolvedRestoreSelectWorkspace_DefaultRootIgnoresSourcePath(t *testing.T) {
	req := &RestoreRequest{Label: "homes", TargetName: "onsite-usb"}
	plan := &Plan{Paths: PlanPaths{SnapshotSource: "/volumeUSB2/historical-source"}}
	revision := duplicacy.RevisionInfo{
		Revision:  2403,
		CreatedAt: time.Date(2026, 4, 24, 7, 0, 0, 0, time.Local),
	}

	got, err := resolvedRestoreSelectWorkspace(req, plan, revision, defaultRestoreDeps())
	if err != nil {
		t.Fatalf("resolvedRestoreSelectWorkspace() error = %v", err)
	}
	want := "/volume1/restore-drills/homes-onsite-usb-20260424-070000-rev2403"
	if got != want {
		t.Fatalf("resolvedRestoreSelectWorkspace() = %q, want %q", got, want)
	}
}

func TestHandleRestoreCommand_PlanRemoteDoesNotLoadSecrets(t *testing.T) {
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://gateway.example.invalid/bucket/homes", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "plan", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}

	secretsFile := filepath.Join(secretsDir, "homes-secrets.toml")
	for _, token := range []string{
		"Restore plan for homes/offsite-storj",
		"Location",
		"remote",
		"Secrets File",
		secretsFile,
		"State",
		"Not found",
		"duplicacy init 'data' 's3://gateway.example.invalid/bucket/homes'",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "s3_id") || strings.Contains(out, "s3_secret") {
		t.Fatalf("restore plan should not expose or require secret key values:\n%s", out)
	}
}

func TestHandleRestoreCommand_PlanAllowsMissingSourcePathForDR(t *testing.T) {
	stubRestoreWorkspaceTime(t, time.Date(2026, 4, 24, 8, 15, 30, 0, time.Local))
	configDir := t.TempDir()
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", `
label = "homes"

[common]
threads = 4
prune = "-keep 0:365"

[storage.offsite-storj]
location = "remote"
storage = "s3://gateway.example.invalid/bucket/homes"
`)

	req := &Request{RestoreCommand: "plan", Source: "homes", ConfigDir: configDir, RequestedTarget: "offsite-storj"}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore plan for homes/offsite-storj",
		"Source Path",
		"Not configured (restore-only access is allowed; copy-back context unavailable)",
		"/volume1/restore-drills/homes-offsite-storj-20260424-081530",
		"Copy Back Preview",
		"Unavailable until source_path is configured",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleRestoreCommand_LocalRepositoryRequiresSudoForMetadataCommands(t *testing.T) {
	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, "/volume1/homes", "/backups/homes", 4, "-keep 0:365"))

	tests := []struct {
		name string
		req  *Request
		rt   Env
	}{
		{
			name: "list revisions",
			req:  &Request{RestoreCommand: "list-revisions", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"},
			rt:   nonRootRestoreRuntime(),
		},
		{
			name: "run",
			req:  &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: filepath.Join(t.TempDir(), "workspace"), RestoreRevision: 2403, RestoreYes: true},
			rt:   nonRootRestoreRuntime(),
		},
		{
			name: "select",
			req:  &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"},
			rt: func() Env {
				rt := restoreSelectRuntime(t, "1\n")
				rt.Geteuid = func() int { return 1000 }
				return rt
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := execpkg.NewMockRunner()
			oldRunner := newRestoreCommandRunner
			newRestoreCommandRunner = func() execpkg.Runner { return mock }
			t.Cleanup(func() { newRestoreCommandRunner = oldRunner })

			_, err := restoreHandleCommand(tt.req, MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir()), tt.rt)
			if err == nil || !strings.Contains(err.Error(), "requires sudo: local filesystem repository is root-protected") {
				t.Fatalf("restoreHandleCommand() err = %v", err)
			}
			if len(mock.Invocations) != 0 {
				t.Fatalf("invocations = %#v, want no Duplicacy repository calls before sudo-required stop", mock.Invocations)
			}
		})
	}
}
