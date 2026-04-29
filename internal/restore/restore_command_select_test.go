package restore

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/restorepicker"
)

func TestHandleRestoreCommand_SelectRejectsNonInteractiveUse(t *testing.T) {
	rt := testRuntime()
	rt.StdinIsTTY = func() bool { return false }
	req := &Request{RestoreCommand: "select", Source: "homes", RequestedTarget: "onsite-usb"}
	_, err := restoreHandleCommand(req, MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt)
	if err == nil || !strings.Contains(err.Error(), "requires an interactive terminal") {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRestoreCommand_SelectShowsRestorePointPrompt(t *testing.T) {
	prompts := captureRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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

func TestHandleRestoreCommand_SelectReportsListFilesDiagnostics(t *testing.T) {
	suppressRestorePrompts(t)

	configDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source", "homes")
	storage := filepath.Join(t.TempDir(), "backups", "homes")
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot data revision 2403 created at 2026-04-20 02:30\n"},
		execpkg.MockResult{
			Stdout: "Storage set to /volumeUSB2/usbshare/duplicacy/homes\n",
			Stderr: "Failed to load snapshot: permission denied\n",
			Err:    errors.New("list files failed"),
		},
	)
	oldRunner := newRestoreCommandRunner
	newRestoreCommandRunner = func() execpkg.Runner { return mock }
	defer func() { newRestoreCommandRunner = oldRunner }()

	req := &Request{RestoreCommand: "select", Source: "homes", ConfigDir: configDir, RequestedTarget: "onsite-usb"}
	_, err := restoreHandleCommand(req, meta, restoreSelectRuntime(t, "1\n3\n"))
	if err == nil {
		t.Fatal("restoreHandleCommand() error = nil, want list-files failure")
	}
	message := err.Error()
	for _, want := range []string{
		"failed to list files for revision 2403",
		"Duplicacy command: duplicacy list -files -r 2403",
		"Duplicacy diagnostics:",
		"Failed to load snapshot: permission denied",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error missing %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "Storage set to") {
		t.Fatalf("error should prefer diagnostic lines over routine Duplicacy output:\n%s", message)
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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
		strings.Join(mock.Invocations[2].Args, " ") != "restore -r 2403 -stats -ignore-owner -- "+restorePath ||
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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
		strings.Join(mock.Invocations[1].Args, " ") != "restore -r 2403 -stats -ignore-owner" ||
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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
		strings.Join(mock.Invocations[2].Args, " ") != "restore -r 2403 -stats -ignore-owner -- docs/manual.pdf" ||
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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	meta := MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, sourcePath, storage, 4, "-keep 0:365"))

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
	_, err := restoreHandleCommand(&Request{RestoreCommand: "execute", Source: "homes"}, MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime())
	if err == nil || !strings.Contains(err.Error(), "unsupported restore command") {
		t.Fatalf("err = %v", err)
	}
}
