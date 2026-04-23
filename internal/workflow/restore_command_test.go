package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
)

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

func TestHandleRestoreCommand_PlanLocalReadOnlyWithState(t *testing.T) {
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
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
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
		"/volume1/restore-drills/homes-onsite-usb",
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
	if _, err := os.Stat("/volume1/restore-drills/homes-onsite-usb"); err == nil {
		t.Fatalf("restore plan command must not create the suggested workspace")
	}
}

func TestHandleRestoreCommand_PlanRemoteDoesNotLoadSecrets(t *testing.T) {
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://gateway.example.invalid/bucket/homes", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "plan", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj"}
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
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

func TestHandleRestoreCommand_PrepareLocalWritesWorkspacePreferences(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "prepare", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}

	preferences := filepath.Join(workspace, ".duplicacy", "preferences")
	for _, token := range []string{
		"Restore workspace prepared for homes/onsite-usb",
		"Executes Restore",
		"false",
		"Copies Back",
		"false",
		"Section: Workspace",
		workspace,
		preferences,
		"duplicacy list",
		"duplicacy list -files -r <revision>",
		"not performed by this command",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "duplicacy restore") {
		t.Fatalf("prepare output should not suggest restore execution:\n%s", out)
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
		t.Fatalf("local prepare should not write storage keys: %#v", prefs[0])
	}
}

func TestHandleRestoreCommand_PrepareRemoteLoadsSecretsWithoutPrintingValues(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("remote restore prepare requires root-owned secrets file")
	}

	configDir := t.TempDir()
	secretsDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", "/volume1/homes", "s3://bucket/homes", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	req := &Request{RestoreCommand: "prepare", Source: "homes", ConfigDir: configDir, SecretsDir: secretsDir, RequestedTarget: "offsite-storj", RestoreWorkspace: workspace}
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}
	if !strings.Contains(out, "Secrets File") || !strings.Contains(out, filepath.Join(secretsDir, "homes-secrets.toml")) {
		t.Fatalf("output missing secrets file:\n%s", out)
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

func TestHandleRestoreCommand_PrepareRejectsUnsafeWorkspaces(t *testing.T) {
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
			req := &Request{RestoreCommand: "prepare", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: tt.workspace}
			_, err := HandleRestoreCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestHandleRestoreCommand_PrepareRejectsNonEmptyWorkspace(t *testing.T) {
	configDir := t.TempDir()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "existing.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, "/volume1/homes", filepath.Join(t.TempDir(), "backups", "homes"), "", "", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "prepare", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	_, err := HandleRestoreCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
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

	req := &Request{RestoreCommand: "revisions", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreLimit: 1}
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore revisions for homes/onsite-usb",
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

func TestHandleRestoreCommand_FilesListsRevisionPaths(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(execpkg.MockResult{Stdout: "docs/readme.md\nmusic/song.flac\ndocs/manual.pdf\n"})
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "files", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreRevision: 2403, RestorePath: "docs", RestoreLimit: 1}
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore files for homes/onsite-usb revision 2403",
		"Path Filter",
		"docs",
		"Total Matches",
		"2",
		"docs/readme.md",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "music/song.flac") || strings.Contains(out, "docs/manual.pdf") {
		t.Fatalf("output should be filtered and limited:\n%s", out)
	}
	if len(mock.Invocations) != 1 || strings.Join(mock.Invocations[0].Args, " ") != "list -files -r 2403" {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
}

func TestExtractRestoreFileLinesParsesDuplicacyRows(t *testing.T) {
	output := strings.Join([]string{
		"5585354 2026-04-20 19:29:38 45fcaf55f07a698bd608e892802bd3f7275a8688374de79acbc5ebb078ebdc06 phillipmcmahon/code/archive.tar.gz",
		"1024 2026-04-21 08:10:11 1234567890abcdef Documents/Folder With Spaces/report final.pdf",
		"plain/path/from/test-fixture.txt",
	}, "\n")

	paths, total := extractRestoreFileLines(output, "", 0)
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	want := []string{
		"phillipmcmahon/code/archive.tar.gz",
		"Documents/Folder With Spaces/report final.pdf",
		"plain/path/from/test-fixture.txt",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}

	filtered, total := extractRestoreFileLines(output, "Folder With Spaces", 1)
	if total != 1 || !reflect.DeepEqual(filtered, []string{"Documents/Folder With Spaces/report final.pdf"}) {
		t.Fatalf("filtered = %#v total = %d", filtered, total)
	}
}

func TestHandleRestoreCommand_RevisionsWithWorkspaceRequiresPreparedWorkspace(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "revisions", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	_, err := HandleRestoreCommand(req, meta, testRuntime())
	if err == nil || !strings.Contains(err.Error(), "requires a prepared workspace") {
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
	out, err := HandleRestoreCommand(req, meta, testRuntime())
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
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

func TestHandleRestoreCommand_RunRequiresPreparedWorkspace(t *testing.T) {
	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	workspace := filepath.Join(t.TempDir(), "restore-workspace")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "run", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreRevision: 2403, RestoreYes: true}
	_, err := HandleRestoreCommand(req, meta, testRuntime())
	if err == nil || !strings.Contains(err.Error(), "requires a prepared workspace") {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRestoreCommand_SelectRejectsNonInteractiveUse(t *testing.T) {
	rt := testRuntime()
	rt.StdinIsTTY = func() bool { return false }
	req := &Request{RestoreCommand: "select", Source: "homes", RequestedTarget: "onsite-usb"}
	_, err := HandleRestoreCommand(req, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt)
	if err == nil || !strings.Contains(err.Error(), "requires an interactive terminal") {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRestoreCommand_SelectGeneratesFullRestoreCommand(t *testing.T) {
	suppressRestorePrompts(t)

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
	out, err := HandleRestoreCommand(req, meta, restoreSelectRuntime(t, "1\nn\n"))
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore selection for homes/onsite-usb",
		"Executes Restore",
		"false",
		"Prepared",
		"false",
		"Prepare Command",
		"restore prepare",
		"Restore Command",
		"restore run",
		"--revision 2403",
		"<full revision>",
		"restore select only generates primitive commands",
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

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace}
	out, err := HandleRestoreCommand(req, meta, restoreSelectRuntime(t, "2403\ny\ndocs\n2\n"))
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
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
	if strings.Contains(out, "Prepare Command") {
		t.Fatalf("prepared workspace should not print a prepare command:\n%s", out)
	}
	if len(mock.Invocations) != 2 ||
		strings.Join(mock.Invocations[0].Args, " ") != "list" ||
		strings.Join(mock.Invocations[1].Args, " ") != "list -files -r 2403" {
		t.Fatalf("invocations = %#v", mock.Invocations)
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

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreExecute: true}
	out, err := HandleRestoreCommand(req, meta, restoreSelectRuntime(t, "1\ny\narchive\n1\ny\n"))
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}
	restorePath := "phillipmcmahon/code/duplicacy-backup/archive/v5.0.0/duplicacy-backup_5.0.0_linux_armv7.tar.gz"
	for _, token := range []string{
		"Path",
		restorePath,
		"--path '" + restorePath + "'",
		"Restored into workspace",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
		}
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

func TestHandleRestoreCommand_SelectExecuteRequiresPreparedWorkspace(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, "", "", 4, "-keep 0:365"))

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreExecute: true}
	_, err := HandleRestoreCommand(req, meta, restoreSelectRuntime(t, "1\nn\ny\n"))
	if err == nil || !strings.Contains(err.Error(), "--execute requires a prepared workspace") {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRestoreCommand_SelectExecuteCancellation(t *testing.T) {
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

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreExecute: true}
	_, err := HandleRestoreCommand(req, meta, restoreSelectRuntime(t, "1\nn\nn\n"))
	if err == nil || !strings.Contains(err.Error(), "execution cancelled") {
		t.Fatalf("error = %v", err)
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

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb", RestoreWorkspace: workspace, RestoreExecute: true}
	out, err := HandleRestoreCommand(req, meta, restoreSelectRuntime(t, "1\ny\ndocs\n2\ny\n"))
	if err != nil {
		t.Fatalf("HandleRestoreCommand() error = %v", err)
	}
	for _, token := range []string{
		"Restore selection for homes/onsite-usb",
		"Executes Restore",
		"true",
		"restore select delegates to restore run after confirmation",
		"delegated to restore run after confirmation",
		"Restore run for homes/onsite-usb revision 2403",
		"Restored into workspace",
		"Restored docs/manual.pdf",
	} {
		if !strings.Contains(out, token) {
			t.Fatalf("output missing %q:\n%s", token, out)
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

func TestHandleRestoreCommand_Unsupported(t *testing.T) {
	_, err := HandleRestoreCommand(&Request{RestoreCommand: "execute", Source: "homes"}, DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "unsupported restore command") {
		t.Fatalf("err = %v", err)
	}
}
