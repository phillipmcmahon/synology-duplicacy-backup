package restore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
)

func TestHandleRestoreCommand_RunDryRunDerivesWorkspaceWithoutSourcePath(t *testing.T) {
	configDir := t.TempDir()
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", `
label = "homes"

[common]
threads = 4
prune = "-keep 0:365"

[storage.onsite-usb]
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
		"Dry Run",
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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	if len(mock.Invocations) != 1 || mock.Invocations[0].Dir != workspace || strings.Join(mock.Invocations[0].Args, " ") != "restore -r 2403 -stats -ignore-owner -- docs/readme.md" {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_RunSudoOperatorRepairsWorkspaceOwnership(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	rt := testRuntime()
	rt.Geteuid = func() int { return 0 }
	rt.Getenv = func(name string) string {
		switch name {
		case "SUDO_USER":
			return "operator"
		case "SUDO_UID":
			return "1026"
		case "SUDO_GID":
			return "100"
		case "HOME":
			return "/home/operator"
		default:
			return ""
		}
	}
	meta := DefaultMetadataForEnv("duplicacy-backup", "1.0.0", "now", rt)
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

	type chownCall struct {
		path string
		uid  int
		gid  int
	}
	var calls []chownCall
	oldChown := profileChown
	profileChown = func(path string, uid, gid int) error {
		calls = append(calls, chownCall{path: path, uid: uid, gid: gid})
		return nil
	}
	t.Cleanup(func() { profileChown = oldChown })

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "Restored docs/readme.md\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	t.Cleanup(func() { newRestoreCommandRunner = oldRunner })

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreRevision: 2403, RestorePath: "docs/readme.md", RestoreYes: true}
	if _, err := restoreHandleCommand(req, meta, rt); err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	if len(calls) == 0 {
		t.Fatal("profile ownership repair was not attempted")
	}
	wantPaths := map[string]bool{
		workspace: false,
		filepath.Join(workspace, ".duplicacy", "preferences"): false,
	}
	for _, call := range calls {
		if call.uid != 1026 || call.gid != 100 {
			t.Fatalf("chown call = %+v, want uid:gid 1026:100", call)
		}
		if _, ok := wantPaths[call.path]; ok {
			wantPaths[call.path] = true
		}
	}
	for path, seen := range wantPaths {
		if !seen {
			t.Fatalf("ownership repair did not include %s; calls = %#v", path, calls)
		}
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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
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
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, filepath.Join(t.TempDir(), "backups", "homes"), 4, "-keep 0:365"))

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
			_, err := restoreHandleCommand(req, MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
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
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, "/volume1/homes", filepath.Join(t.TempDir(), "backups", "homes"), 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreRevision: 2403, RestoreYes: true}
	_, err := restoreHandleCommand(req, MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "must be empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRestoreCommand_RevisionsListsVisibleRevisionsReadOnly(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
		"Source Path",
		sourcePath,
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

func TestHandleRestoreCommand_RevisionsWithWorkspaceRequiresPreparedWorkspace(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	if len(mock.Invocations) != 1 || mock.Invocations[0].Dir != workspace || strings.Join(mock.Invocations[0].Args, " ") != "restore -r 2403 -stats -ignore-owner -- docs/readme.md" {
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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
		strings.Join(mock.Invocations[1].Args, " ") != "restore -r 2403 -stats -ignore-owner -- docs/readme.md" ||
		mock.Invocations[1].Dir != wantWorkspace {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_RunUsesConfiguredWorkspaceTemplate(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := "/tmp/homes-run-config-template-test"
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	root := setupRestoreWorkspaceRoot(t)
	wantWorkspace := filepath.Join(root, "homes-onsite-usb-rev2403-20260424-070000")
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	restoreSection := fmt.Sprintf("[restore]\nworkspace_root = %q\nworkspace_template = \"{label}-{storage}-rev{revision}-{snapshot_timestamp}\"\n", root)
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365", restoreSection))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-24 07:00\n"},
		execpkg.MockResult{Stdout: "Restored docs/readme.md\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreRevision: 2403, RestorePath: "docs/readme.md", RestoreYes: true}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	if !strings.Contains(out, wantWorkspace) {
		t.Fatalf("output missing configured workspace %q:\n%s", wantWorkspace, out)
	}
	if len(mock.Invocations) != 2 || mock.Invocations[1].Dir != wantWorkspace {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_RunCLIWorkspaceTemplateOverridesConfig(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := "/tmp/homes-run-cli-template-test"
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	root := setupRestoreWorkspaceRoot(t)
	wantWorkspace := filepath.Join(root, "manual-onsite-usb-2403-20260424-070000")
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	restoreSection := fmt.Sprintf("[restore]\nworkspace_root = %q\nworkspace_template = \"config-{label}-{storage}-{revision}\"\n", root)
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365", restoreSection))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-24 07:00\n"},
		execpkg.MockResult{Stdout: "Restored docs/readme.md\n"},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspaceTemplate: "manual-{storage}-{revision}-{snapshot_timestamp}", RestoreRevision: 2403, RestorePath: "docs/readme.md", RestoreYes: true}
	out, err := restoreHandleCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("restoreHandleCommand() error = %v", err)
	}
	if !strings.Contains(out, wantWorkspace) || strings.Contains(out, "config-homes-onsite-usb-2403") {
		t.Fatalf("output should use CLI workspace template over config:\n%s", out)
	}
	if len(mock.Invocations) != 2 || mock.Invocations[1].Dir != wantWorkspace {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestHandleRestoreCommand_RunPreservesExistingWorkspaceRootPermissions(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := "/tmp/homes-run-root-permissions-test"
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	root := setupRestoreWorkspaceRoot(t)
	wantWorkspace := filepath.Join(root, "homes-onsite-usb-20260424-070000-rev2403")
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
