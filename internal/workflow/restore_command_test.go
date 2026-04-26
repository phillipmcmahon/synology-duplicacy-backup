package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/restorepicker"
)

var (
	restorePromptOutput      io.Writer = os.Stdout
	runRestoreSelectPicker             = defaultRestoreDeps().RunSelectPicker
	runRestoreInspectPicker            = defaultRestoreDeps().RunInspectPicker
	restoreWorkspaceNow                = defaultRestoreDeps().Now
	testRestoreWorkspaceRoot           = defaultRestoreDeps().RestoreWorkspaceRoot
	newRestoreCommandRunner            = defaultRestoreDeps().NewRunner
	restoreProgress                    = defaultRestoreDeps().Progress
)

func restoreHandleCommand(req *Request, meta Metadata, rt Runtime) (string, error) {
	restoreReq := NewRestoreRequest(req)
	return handleRestoreCommand(&restoreReq, meta, rt, RestoreDeps{
		NewRunner:            newRestoreCommandRunner,
		PromptOutput:         restorePromptOutput,
		Now:                  restoreWorkspaceNow,
		RestoreWorkspaceRoot: testRestoreWorkspaceRoot,
		RunSelectPicker:      runRestoreSelectPicker,
		RunInspectPicker:     runRestoreInspectPicker,
		Progress:             restoreProgress,
	})
}

func restoreSelectRuntime(t *testing.T, input string) Runtime {
	t.Helper()
	rt := testRuntime()
	tempDir := t.TempDir()
	stdin, err := os.CreateTemp(tempDir, "stdin-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	if _, err := stdin.WriteString(input); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if _, err := stdin.Seek(0, 0); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}
	t.Cleanup(func() { _ = stdin.Close() })
	rt.Stdin = func() *os.File { return stdin }
	rt.StdinIsTTY = func() bool { return true }
	rt.TempDir = func() string { return tempDir }
	return rt
}

func suppressRestorePrompts(t *testing.T) {
	t.Helper()
	old := restorePromptOutput
	var output strings.Builder
	restorePromptOutput = &output
	t.Cleanup(func() { restorePromptOutput = old })
}

func captureRestorePrompts(t *testing.T) *strings.Builder {
	t.Helper()
	old := restorePromptOutput
	var output strings.Builder
	restorePromptOutput = &output
	t.Cleanup(func() { restorePromptOutput = old })
	return &output
}

func stubRestoreSelectPicker(t *testing.T, stub func([]string, restorepicker.AppOptions) ([]string, error)) {
	t.Helper()
	old := runRestoreSelectPicker
	runRestoreSelectPicker = stub
	t.Cleanup(func() { runRestoreSelectPicker = old })
}

func stubRestoreInspectPicker(t *testing.T, stub func([]string, restorepicker.AppOptions) error) {
	t.Helper()
	old := runRestoreInspectPicker
	runRestoreInspectPicker = stub
	t.Cleanup(func() { runRestoreInspectPicker = old })
}

func stubRestoreWorkspaceTime(t *testing.T, ts time.Time) {
	t.Helper()
	old := restoreWorkspaceNow
	restoreWorkspaceNow = func() time.Time { return ts }
	t.Cleanup(func() { restoreWorkspaceNow = old })
}

func setupRestoreWorkspaceRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "restore-drills")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", root, err)
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatalf("Chmod(%q) error = %v", root, err)
	}
	return root
}

func stubRestoreProgress(t *testing.T, progress RestoreProgress) {
	t.Helper()
	old := restoreProgress
	restoreProgress = progress
	t.Cleanup(func() { restoreProgress = old })
}

func TestHandleRestoreCommand_PlanLocalReadOnlyWithState(t *testing.T) {
	stubRestoreWorkspaceTime(t, time.Date(2026, 4, 24, 8, 15, 30, 0, time.Local))
	configDir := t.TempDir()
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", localTargetConfig("homes", "/volume1/homes", "/backups", "", "", 4, "-keep 0:365"))
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
		"duplicacy restore -r <revision> -stats",
		`duplicacy restore -r <revision> -stats -- "relative/path/from/snapshot"`,
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
	plan := &Plan{SnapshotSource: "/tmp/homes"}
	revision := duplicacy.RevisionInfo{
		Revision:  2403,
		CreatedAt: time.Date(2026, 4, 24, 7, 0, 0, 0, time.Local),
	}

	deps := defaultRestoreDeps()
	got := resolvedRestoreSelectWorkspace(req, plan, revision, deps)
	want := filepath.Join(root, "homes-onsite-usb-20260424-070000-rev2403")
	if got != want {
		t.Fatalf("resolvedRestoreSelectWorkspace() = %q, want %q", got, want)
	}
}

func TestResolvedRestoreSelectWorkspace_UsesExplicitWorkspace(t *testing.T) {
	req := &RestoreRequest{Label: "homes", TargetName: "onsite-usb", Workspace: "/restore/exact"}
	plan := &Plan{SnapshotSource: "/tmp/homes"}
	revision := duplicacy.RevisionInfo{
		Revision:  2403,
		CreatedAt: time.Date(2026, 4, 24, 7, 0, 0, 0, time.Local),
	}

	got := resolvedRestoreSelectWorkspace(req, plan, revision, defaultRestoreDeps())
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

func TestValidateRestoreWorkspaceRoot_RejectsMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-restore-root")
	err := validateRestoreWorkspaceRoot(&RestoreRequest{WorkspaceRoot: root})
	if err == nil || !strings.Contains(err.Error(), "--workspace-root does not exist") {
		t.Fatalf("validateRestoreWorkspaceRoot() err = %v", err)
	}
}

func TestHandleRestoreCommand_RejectsWorkspaceAndRoot(t *testing.T) {
	req := &Request{RestoreCommand: "run", Source: "homes", RequestedTarget: "onsite-usb", RestoreWorkspace: "/restore/exact", RestoreWorkspaceRoot: "/restore/root", RestoreRevision: 2403, RestoreYes: true}
	_, err := restoreHandleCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "--workspace and --workspace-root cannot be used together") {
		t.Fatalf("restoreHandleCommand() err = %v", err)
	}
}

func TestResolvedRestoreSelectWorkspace_DefaultRootIgnoresSourcePath(t *testing.T) {
	req := &RestoreRequest{Label: "homes", TargetName: "onsite-usb"}
	plan := &Plan{SnapshotSource: "/volumeUSB2/historical-source"}
	revision := duplicacy.RevisionInfo{
		Revision:  2403,
		CreatedAt: time.Date(2026, 4, 24, 7, 0, 0, 0, time.Local),
	}

	got := resolvedRestoreSelectWorkspace(req, plan, revision, defaultRestoreDeps())
	want := "/volume1/restore-drills/homes-onsite-usb-20260424-070000-rev2403"
	if got != want {
		t.Fatalf("resolvedRestoreSelectWorkspace() = %q, want %q", got, want)
	}
}

func TestHandleRestoreCommand_PlanRemoteDoesNotLoadSecrets(t *testing.T) {
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
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
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", `
label = "homes"

[common]
threads = 4
prune = "-keep 0:365"

[targets.offsite-storj]
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

func TestHandleRestoreCommand_RunDryRunDerivesWorkspaceWithoutSourcePath(t *testing.T) {
	configDir := t.TempDir()
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", `
label = "homes"

[common]
threads = 4
prune = "-keep 0:365"

[targets.onsite-usb]
location = "local"
storage = "/backups/homes"
`)

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-24 07:00\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreRevision: 2403, RestorePath: "docs/readme.md", RestoreYes: true, DryRun: true}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore run for homes/onsite-usb revision 2403",
		"Source Path",
		"Not configured (restore-only access is allowed; copy-back context unavailable)",
		"/volume1/restore-drills/homes-onsite-usb-20260424-070000-rev2403",
		"Dry run",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if len(mock.Invocations) != 1 || strings.Join(mock.Invocations[0].Args, " ") != "list" {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_RunPreparesExplicitWorkspace(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Restored docs/readme.md\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreRevision: 2403, RestorePath: "docs/readme.md", RestoreYes: true}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}

	preferences := filepath.Join(workspace, ".duplicacy", "preferences")
	for _, token := range []string{
		"Restore run for homes/onsite-usb revision 2403",
		"Executes Restore",
		"true",
		"Copies Back",
		"false",
		"Section: Workspace",
		workspace,
		"Restored into workspace",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}

	body, err := os.ReadFile(preferences)
	if err != nil {
		t.Fatalf("ReadFile(preferences) error = %v", err)
	}
	var prefs []map[string]any
	if err := json.Unmarshal(body, &prefs); err != nil {
		t.Fatalf("preferences JSON error = %v\n%s", err, body)
	}
	if len(prefs) != 1 || prefs[0]["id"] != "data" || prefs[0]["repository"] != workspace || prefs[0]["storage"] != storage {
		t.Fatalf("preferences = %#v", prefs)
	}
	if _, ok := prefs[0]["keys"].(map[string]any); ok {
		t.Fatalf("local restore run should not write storage keys: %#v", prefs[0])
	}
	if len(mock.Invocations) != 1 || mock.Invocations[0].Dir != workspace || strings.Join(mock.Invocations[0].Args, " ") != "restore -r 2403 -stats -- docs/readme.md" {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

type recordingRestoreProgress struct {
	events []string
}

func (p *recordingRestoreProgress) PrintRunStart(_ *RestoreRequest, _ *Plan, inputs restoreRunInputs, _ time.Time) {
	p.events = append(p.events, "start:"+restoreProgressPath(inputs.RestorePath))
}

func (p *recordingRestoreProgress) PrintSelectionStart(_ *RestoreRequest, _ *Plan, revision int, _ string, total int, _ time.Time) {
	p.events = append(p.events, "selection:"+strconv.Itoa(revision)+":"+strconv.Itoa(total))
}

func (p *recordingRestoreProgress) PrintStatus(status string) {
	p.events = append(p.events, "status:"+status)
}

func (p *recordingRestoreProgress) StartActivity(status string) func() {
	p.events = append(p.events, "activity:"+status)
	return func() {
		p.events = append(p.events, "activity:stop")
	}
}

func (p *recordingRestoreProgress) StartSelectionActivity(current, total int, path string) func() {
	p.events = append(p.events, "selection-activity:"+restoreSelectionProgressActivity(current, total, path))
	return func() {
		p.events = append(p.events, "selection-activity:stop")
	}
}

func (p *recordingRestoreProgress) PrintInterrupted(info restoreInterruptInfo) {
	p.events = append(p.events, "interrupted:"+restoreInterruptProgress(info)+":"+info.CurrentPath)
}

func (p *recordingRestoreProgress) PrintRunCompletion(success bool, _ time.Time) {
	p.events = append(p.events, "complete:"+strconv.FormatBool(success))
}

func TestHandleRestoreCommand_RunEmitsProgress(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Restored docs/readme.md\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	progress := &recordingRestoreProgress{}
	stubRestoreProgress(t, progress)

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreRevision: 2403, RestorePath: "docs/readme.md", RestoreYes: true}
	if _, err := restoreHandleCommand(req, meta, testRuntime()); err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}

	for _, token := range []string{
		"start:docs/readme.md",
		"status:Preparing drill workspace",
		"activity:Restoring selected path from revision 2403 into drill workspace",
		"activity:stop",
		"complete:true",
	} {
		if !slices.Contains(progress.events, token) {
			t.Fatalf("progress missing %q in %#v", token, progress.events)
		}
	}
}

func TestHandleRestoreCommand_RunReportsInterruptedRestore(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{
		Stdout: "Restoring /workspace to revision 2403\nDownloaded chunk 1 size 123\n",
		Err:    context.Canceled,
	})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	progress := &recordingRestoreProgress{}
	stubRestoreProgress(t, progress)

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreRevision: 2403, RestorePath: "docs/readme.md", RestoreYes: true}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if !errors.Is(err, ErrRestoreInterrupted) {
		t.Fatalf("restoreHandleCommand() error = %v, want ErrRestoreInterrupted", err)
	}
	for _, token := range []string{
		"Restore run for homes/onsite-usb revision 2403",
		"Result",
		"Interrupted",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if !slices.Contains(progress.events, "complete:false") {
		t.Fatalf("progress events = %#v, want failed completion", progress.events)
	}
	if len(mock.Invocations) != 1 || mock.Invocations[0].Ctx == nil {
		t.Fatalf("invocations = %#v, want one context-aware restore invocation", mock.Invocations)
	}
}

func TestHandleRestoreCommand_RunRemoteLoadsSecretsWithoutPrintingValues(t *testing.T) {
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket/homes", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Restored full revision\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj", RestoreWorkspace: workspace, RestoreRevision: 2403, RestoreYes: true}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	if strings.Contains(out, "ABCDEFGHIJKLMNOPQRSTUVWXYZ01") || strings.Contains(out, "abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR") {
		t.Fatalf("output leaked secret values:\n%s", out)
	}
	body, err := os.ReadFile(filepath.Join(workspace, ".duplicacy", "preferences"))
	if err != nil {
		t.Fatalf("ReadFile(preferences) error = %v", err)
	}
	if !strings.Contains(string(body), `"s3_id"`) || !strings.Contains(string(body), `"s3_secret"`) {
		t.Fatalf("preferences missing required storage keys:\n%s", body)
	}
}

func TestHandleRestoreCommand_RunRejectsUnsafeWorkspaces(t *testing.T) {
	configDir := t.TempDir()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source")
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, filepath.Join(t.TempDir(), "backups", "homes"), "", "", 4, "-keep 0:365"))

	tests := []struct {
		name      string
		workspace string
		want      string
	}{
		{name: "relative", workspace: "relative/path", want: "absolute path"},
		{name: "source", workspace: sourcePath, want: "live source path"},
		{name: "source child", workspace: filepath.Join(sourcePath, "restore"), want: "inside the live source path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: tt.workspace, RestoreRevision: 2403, RestoreYes: true}
			_, err := restoreHandleCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestHandleRestoreCommand_RunRejectsNonEmptyUnpreparedWorkspace(t *testing.T) {
	configDir := t.TempDir()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "existing.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, "/volume1/homes", filepath.Join(t.TempDir(), "backups", "homes"), "", "", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreRevision: 2403, RestoreYes: true}
	_, err := restoreHandleCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "must be empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRestoreCommand_RevisionsListsVisibleRevisionsReadOnly(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\nSnapshot data revision 2402 created at 2026-04-19 02:30\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "list-revisions", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreLimit: 1}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore revision list for homes/onsite-usb",
		"Read Only",
		"Executes Restore",
		"false",
		"Workspace",
		"temporary",
		"Revision Count",
		"2",
		"2403 (2026-04-20 02:30:00)",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if len(mock.Invocations) != 1 || mock.Invocations[0].Cmd != "duplicacy" || strings.Join(mock.Invocations[0].Args, " ") != "list" {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestExtractRestoreFileLinesParsesDuplicacyRows(t *testing.T) {
	output := strings.Join([]string{
		"5585354 2026-04-20 19:29:38 45fcaf55f07a698bd608e892802bd3f7275a8688374de79acbc5ebb078ebdc06 phillipmcmahon/code/archive.tar.gz",
		"1024 2026-04-21 08:10:11 1234567890abcdef Documents/Folder With Spaces/report final.pdf",
		"Files: 2471",
		"Snapshot data revision 1 created at 2026-04-23 02:30 -hash",
		"Total size: 287254112235, file chunks: 6658, metadata chunks: 4",
		"plain/path/from/test-fixture.txt",
	}, "\n")

	paths := extractRestoreFilePaths(output)
	want := []string{
		"phillipmcmahon/code/archive.tar.gz",
		"Documents/Folder With Spaces/report final.pdf",
		"plain/path/from/test-fixture.txt",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestHandleRestoreCommand_RevisionsWithWorkspaceRequiresPreparedWorkspace(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "list-revisions", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	_, err := restoreHandleCommand(req, meta, testRuntime())
	if err == nil || !strings.Contains(err.Error(), "requires a workspace containing .duplicacy/preferences") {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRestoreCommand_RunRestoresOnlyIntoPreparedWorkspace(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".duplicacy"), 0770); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".duplicacy", "preferences"), []byte("[]\n"), 0660); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Restored docs/readme.md\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreRevision: 2403, RestorePath: "docs/readme.md", RestoreYes: true}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore run for homes/onsite-usb revision 2403",
		"Restored into workspace",
		"Live Source",
		"not modified",
		workspace,
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if len(mock.Invocations) != 1 || mock.Invocations[0].Dir != workspace || strings.Join(mock.Invocations[0].Args, " ") != "restore -r 2403 -stats -- docs/readme.md" {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_RunDerivesWorkspaceFromRevision(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := "/tmp/homes-run-self-prepare-test"
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	root := setupRestoreWorkspaceRoot(t)
	wantWorkspace := filepath.Join(root, "homes-onsite-usb-20260424-070000-rev2403")
	_ = os.RemoveAll(wantWorkspace)
	t.Cleanup(func() { _ = os.RemoveAll(wantWorkspace) })
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-24 07:00\n"},
		execpkg.MockResult{Stdout: "Restored docs/readme.md\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspaceRoot: root, RestoreRevision: 2403, RestorePath: "docs/readme.md", RestoreYes: true}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		wantWorkspace,
		"Restored into workspace",
		"Prepared",
		"true",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if _, err := os.Stat(filepath.Join(wantWorkspace, ".duplicacy", "preferences")); err != nil {
		t.Fatalf("preferences not written: %v", err)
	}
	if len(mock.Invocations) != 2 ||
		strings.Join(mock.Invocations[0].Args, " ") != "list" ||
		strings.Join(mock.Invocations[1].Args, " ") != "restore -r 2403 -stats -- docs/readme.md" ||
		mock.Invocations[1].Dir != wantWorkspace {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_RunPreservesExistingWorkspaceRootPermissions(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := "/tmp/homes-run-root-permissions-test"
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	root := setupRestoreWorkspaceRoot(t)
	wantWorkspace := filepath.Join(root, "homes-onsite-usb-20260424-070000-rev2403")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-24 07:00\n"},
		execpkg.MockResult{Stdout: "Restored docs/readme.md\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspaceRoot: root, RestoreRevision: 2403, RestorePath: "docs/readme.md", RestoreYes: true}
	if _, err := restoreHandleCommand(req, meta, testRuntime()); err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", root, err)
	}
	if got := rootInfo.Mode().Perm(); got != 0755 {
		t.Fatalf("workspace root mode = %v, want 0755", got)
	}
	workspaceInfo, err := os.Stat(wantWorkspace)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", wantWorkspace, err)
	}
	if got := workspaceInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("workspace mode = %v, want 0700", got)
	}
}

func TestHandleRestoreCommand_SelectRejectsNonInteractiveUse(t *testing.T) {
	rt := testRuntime()
	rt.StdinIsTTY = func() bool { return false }
	req := &Request{RestoreCommand: "select", Source: "homes", RequestedTarget: "onsite-usb"}
	_, err := restoreHandleCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt)
	if err == nil || !strings.Contains(err.Error(), "requires an interactive terminal") {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRestoreCommand_SelectShowsRestorePointPrompt(t *testing.T) {
	prompts := captureRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\nSnapshot data revision 2402 created at 2026-04-19 02:30\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n2\nn\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	output := prompts.String()
	for _, token := range []string{
		"Available restore points:",
		"2026-04-20 02:30:00 | rev 2403",
		"Select restore point by list number or revision id (q to cancel):",
		"Choose what you want to do next:",
		"Inspect revision contents only",
		"q. Cancel and exit without restoring",
	} {
		if !strings.Contains(output, token) {
			t.Fatalf("prompt output missing %q:\n%s", token, output)
		}
	}
}

func TestHandleRestoreCommand_SelectInspectsRevisionWithoutWorkspace(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"},
		execpkg.MockResult{Stdout: "docs/readme.md\ndocs/manual.pdf\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	inspected := false
	stubRestoreInspectPicker(t, func(paths []string, opts restorepicker.AppOptions) error {
		inspected = true
		if opts.PathPrefix != "" {
			t.Fatalf("PathPrefix = %q, want empty", opts.PathPrefix)
		}
		if len(paths) != 2 {
			t.Fatalf("paths = %#v", paths)
		}
		return nil
	})

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n1\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	if !inspected {
		t.Fatalf("inspect picker was not invoked")
	}
	for _, token := range []string{
		"Restore inspection for homes/onsite-usb revision 2403",
		"Generated Commands",
		"none; inspect mode does not generate restore commands",
		"Restore Execution",
		"not performed by this command",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if len(mock.Invocations) != 2 ||
		strings.Join(mock.Invocations[0].Args, " ") != "list" ||
		strings.Join(mock.Invocations[1].Args, " ") != "list -files -r 2403" {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_SelectGeneratesFullRestoreCommand(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	root := setupRestoreWorkspaceRoot(t)
	wantWorkspace := filepath.Join(root, "homes-onsite-usb-20260420-023000-rev2403")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\nSnapshot data revision 2402 created at 2026-04-19 02:30\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspaceRoot: root}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n2\nn\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore selection for homes/onsite-usb",
		"Executes Restore",
		"after confirmation",
		"Prepared",
		"false",
		"Restore Command",
		"restore run",
		"--revision 2403",
		wantWorkspace,
		"<full revision>",
		"restore select previews explicit restore run commands; restore run prepares the workspace and restores only there",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "--path") {
		t.Fatalf("full restore command should not include --path:\n%s", out)
	}
	if len(mock.Invocations) != 1 || strings.Join(mock.Invocations[0].Args, " ") != "list" {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_SelectOptionTwoWithPathPrefixUsesScopedSubtree(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-24 07:00\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{
		RestoreCommand:    "select",
		Source:            "homes",
		ConfigDir:         configDir,
		RequestedTarget:   "onsite-usb",
		RestorePathPrefix: "phillipmcmahon/code",
	}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n2\nn\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	if !strings.Contains(out, "--path 'phillipmcmahon/code/*'") {
		t.Fatalf("output missing scoped subtree path:\n%s", out)
	}
	if strings.Contains(out, "<full revision>") {
		t.Fatalf("output should not fall back to full revision when path-prefix is set:\n%s", out)
	}
}

func TestHandleRestoreCommand_SelectGeneratesSelectiveRestoreCommand(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".duplicacy"), 0770); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".duplicacy", "preferences"), []byte("[]\n"), 0660); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\nSnapshot data revision 2402 created at 2026-04-19 02:30\n"},
		execpkg.MockResult{Stdout: "docs/readme.md\ndocs/manual.pdf\nmusic/song.flac\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()
	stubRestoreSelectPicker(t, func(paths []string, opts restorepicker.AppOptions) ([]string, error) {
		if opts.PathPrefix != "" {
			t.Fatalf("PathPrefix = %q, want empty", opts.PathPrefix)
		}
		return []string{"docs/manual.pdf"}, nil
	})

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "2403\n3\nn\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore selection for homes/onsite-usb",
		"Prepared",
		"true",
		"--revision 2403",
		"--path 'docs/manual.pdf'",
		workspace,
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "Prepare Command") || strings.Contains(out, "restore prepare") {
		t.Fatalf("select output should not print a prepare command:\n%s", out)
	}
	if len(mock.Invocations) != 2 ||
		strings.Join(mock.Invocations[0].Args, " ") != "list" ||
		strings.Join(mock.Invocations[1].Args, " ") != "list -files -r 2403" {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_SelectBuildsMultipleRestoreCommands(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".duplicacy"), 0770); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".duplicacy", "preferences"), []byte("[]\n"), 0660); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"},
		execpkg.MockResult{Stdout: "docs/readme.md\nmusic/song.flac\nphotos/family.jpg\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()
	stubRestoreSelectPicker(t, func(paths []string, opts restorepicker.AppOptions) ([]string, error) {
		return []string{"docs/*", "music/*"}, nil
	})

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n3\nn\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"--path 'docs/*'",
		"--path 'music/*'",
		"docs/*",
		"music/*",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleRestoreCommand_SelectParsesDuplicacyFileListRows(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".duplicacy"), 0770); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".duplicacy", "preferences"), []byte("[]\n"), 0660); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"},
		execpkg.MockResult{Stdout: "5585354 2026-04-20 19:29:38 45fcaf55f07a698bd608e892802bd3f7275a8688374de79acbc5ebb078ebdc06 phillipmcmahon/code/duplicacy-backup/archive/v5.0.0/duplicacy-backup_5.0.0_linux_armv7.tar.gz\n"},
		execpkg.MockResult{Stdout: "Restored phillipmcmahon/code/duplicacy-backup/archive/v5.0.0/duplicacy-backup_5.0.0_linux_armv7.tar.gz\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()
	restorePath := "phillipmcmahon/code/duplicacy-backup/archive/v5.0.0/duplicacy-backup_5.0.0_linux_armv7.tar.gz"
	stubRestoreSelectPicker(t, func(paths []string, opts restorepicker.AppOptions) ([]string, error) {
		if len(paths) != 1 || paths[0] != restorePath {
			t.Fatalf("paths = %#v, want [%q]", paths, restorePath)
		}
		if opts.PathPrefix != "phillipmcmahon/code/duplicacy-backup/archive/v5.0.0" {
			t.Fatalf("PathPrefix = %q", opts.PathPrefix)
		}
		return []string{restorePath}, nil
	})

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestorePathPrefix: "phillipmcmahon/code/duplicacy-backup/archive/v5.0.0"}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n3\ny\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"Path",
		restorePath,
		"Restored into workspace",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "--path '"+restorePath+"'") {
		t.Fatalf("confirmed restore output should not repeat the generated command preview:\n%s", out)
	}
	if strings.Contains(out, "5585354 2026-04-20 19:29:38") {
		t.Fatalf("output should not include Duplicacy metadata columns as the restore path:\n%s", out)
	}
	if len(mock.Invocations) != 3 ||
		strings.Join(mock.Invocations[0].Args, " ") != "list" ||
		strings.Join(mock.Invocations[1].Args, " ") != "list -files -r 2403" ||
		strings.Join(mock.Invocations[2].Args, " ") != "restore -r 2403 -stats -- "+restorePath ||
		mock.Invocations[2].Dir != workspace {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_SelectAutoPreparesWorkspaceBeforeExecution(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"},
		execpkg.MockResult{Stdout: "Restored full revision\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n2\ny\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore run for homes/onsite-usb revision 2403",
		"Restored into workspace",
		workspace,
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if len(mock.Invocations) != 2 ||
		strings.Join(mock.Invocations[0].Args, " ") != "list" ||
		strings.Join(mock.Invocations[1].Args, " ") != "restore -r 2403 -stats" ||
		mock.Invocations[1].Dir != workspace {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_SelectStopsAfterPreviewWhenExecutionNotConfirmed(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".duplicacy"), 0770); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".duplicacy", "preferences"), []byte("[]\n"), 0660); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n2\nn\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	if !strings.Contains(out, "Restore selection for homes/onsite-usb") {
		t.Fatalf("output missing preview:\n%s", out)
	}
	if strings.Contains(out, "Restore run for homes/onsite-usb revision 2403") {
		t.Fatalf("restore should not have executed after declining confirmation:\n%s", out)
	}
	if len(mock.Invocations) != 1 || strings.Join(mock.Invocations[0].Args, " ") != "list" {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_SelectExecuteDelegatesToRestoreRun(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".duplicacy"), 0770); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".duplicacy", "preferences"), []byte("[]\n"), 0660); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"},
		execpkg.MockResult{Stdout: "docs/readme.md\ndocs/manual.pdf\n"},
		execpkg.MockResult{Stdout: "Restored docs/manual.pdf\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()
	stubRestoreSelectPicker(t, func(paths []string, opts restorepicker.AppOptions) ([]string, error) {
		return []string{"docs/manual.pdf"}, nil
	})

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n3\ny\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore run for homes/onsite-usb revision 2403",
		"Executes Restore",
		"true",
		"Restored into workspace",
		"docs/manual.pdf - Restored into workspace",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	for _, token := range []string{
		"Restore selection for homes/onsite-usb",
		"Generated Commands",
		"not performed unless you confirm after reviewing the commands",
	} {
		if strings.Contains(out, token) {
			t.Fatalf("confirmed restore select output should not repeat preview token %q:\n%s", token, out)
		}
	}
	if len(mock.Invocations) != 3 ||
		strings.Join(mock.Invocations[0].Args, " ") != "list" ||
		strings.Join(mock.Invocations[1].Args, " ") != "list -files -r 2403" ||
		strings.Join(mock.Invocations[2].Args, " ") != "restore -r 2403 -stats -- docs/manual.pdf" ||
		mock.Invocations[2].Dir != workspace {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_SelectExecuteMultiplePathsUsesBatchProgress(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".duplicacy"), 0770); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".duplicacy", "preferences"), []byte("[]\n"), 0660); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"},
		execpkg.MockResult{Stdout: "docs/readme.md\nmusic/song.flac\n"},
		execpkg.MockResult{Stdout: "Restored docs\n"},
		execpkg.MockResult{Stdout: "Restored music\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()
	stubRestoreSelectPicker(t, func(paths []string, opts restorepicker.AppOptions) ([]string, error) {
		return []string{"docs/*", "music/*"}, nil
	})
	progress := &recordingRestoreProgress{}
	stubRestoreProgress(t, progress)

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	if _, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n3\ny\n")); err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}

	for _, token := range []string{
		"selection:2403:2",
		"selection-activity:Restoring selection 1 of 2: docs/*",
		"selection-activity:Restoring selection 2 of 2: music/*",
		"complete:true",
	} {
		if !slices.Contains(progress.events, token) {
			t.Fatalf("progress missing %q in %#v", token, progress.events)
		}
	}
	for _, token := range []string{
		"status:Preparing drill workspace",
		"activity:Restoring selected path from revision 2403 into drill workspace",
	} {
		if slices.Contains(progress.events, token) {
			t.Fatalf("multi-selection progress should not include %q in %#v", token, progress.events)
		}
	}
}

func TestHandleRestoreCommand_SelectGeneratesDirectoryPattern(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"},
		execpkg.MockResult{Stdout: "docs/readme.md\ndocs/reference/api.md\nmusic/song.flac\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()
	stubRestoreSelectPicker(t, func(paths []string, opts restorepicker.AppOptions) ([]string, error) {
		return []string{"docs/*"}, nil
	})

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n3\nn\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"--path 'docs/*'",
		"Path",
		"docs/*",
		"restore run",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleRestoreCommand_SelectPassesPathPrefixToPicker(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"},
		execpkg.MockResult{Stdout: "phillipmcmahon/code/app/main.go\nphillipmcmahon/code/app/internal/readme.md\nphillipmcmahon/music/song.flac\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()
	stubRestoreSelectPicker(t, func(paths []string, opts restorepicker.AppOptions) ([]string, error) {
		if opts.PathPrefix != "phillipmcmahon/code" {
			t.Fatalf("PathPrefix = %q", opts.PathPrefix)
		}
		return []string{"phillipmcmahon/code/app/*"}, nil
	})

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestorePathPrefix: "phillipmcmahon/code"}
	out, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n3\nn\n"))
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	for _, token := range []string{
		"--path 'phillipmcmahon/code/app/*'",
		"phillipmcmahon/code/app/*",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
}

func TestHandleRestoreCommand_SelectCancellationFromPicker(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"},
		execpkg.MockResult{Stdout: "docs/readme.md\nmusic/live/song.flac\nmusic/studio/song.flac\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()
	stubRestoreSelectPicker(t, func(paths []string, opts restorepicker.AppOptions) ([]string, error) {
		return nil, restorepicker.ErrPickerCancelled
	})

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n3\n"))
	if !errors.Is(err, ErrRestoreCancelled) {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRestoreCommand_SelectCancellationAtRevisionPrompt(t *testing.T) {
	prompts := captureRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "q\n"))
	if !errors.Is(err, ErrRestoreCancelled) {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(prompts.String(), "q to cancel") {
		t.Fatalf("prompt output missing cancel guidance:\n%s", prompts.String())
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("restore should not continue after cancellation: %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_Unsupported(t *testing.T) {
	_, err := restoreHandleCommand(&Request{RestoreCommand: "execute", Source: "homes"}, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "unsupported restore command") {
		t.Fatalf("err = %v", err)
	}
}
