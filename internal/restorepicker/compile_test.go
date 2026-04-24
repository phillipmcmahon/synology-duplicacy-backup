package restorepicker

import (
	"strings"
	"testing"
)

func TestCompileSelectionFullRootUsesFullRestoreCommand(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"music/live/song.flac",
	})
	ToggleSelection(root)

	preview := CompileSelection(root, PrimitiveOptions{
		ScriptName: "duplicacy-backup",
		Source:     "homes",
		Target:     "onsite-usb",
		Revision:   "2403",
		Workspace:  "/volume1/restore-drills/homes-onsite-usb",
	})

	if !preview.FullRestore {
		t.Fatalf("FullRestore = false, want true")
	}
	if len(preview.RestorePaths) != 1 || preview.RestorePaths[0] != "" {
		t.Fatalf("RestorePaths = %#v, want full revision marker", preview.RestorePaths)
	}
	if len(preview.Commands) != 1 {
		t.Fatalf("len(Commands) = %d, want 1", len(preview.Commands))
	}
	if strings.Contains(preview.Commands[0], "--path") {
		t.Fatalf("full restore command should not include --path: %s", preview.Commands[0])
	}
}

func TestCompileSelectionPartialDirectoryExpandsToChildPatterns(t *testing.T) {
	root := BuildTree([]string{
		"phillipmcmahon/code/archive/v5.0.0/a.tar.gz",
		"phillipmcmahon/code/archive/v5.1.0/b.tar.gz",
		"phillipmcmahon/code/docs/readme.md",
	})

	code := root.Children[0].Children[0]
	ToggleSelection(code)
	docs := code.Children[1]
	ToggleSelection(docs)

	preview := CompileSelection(root, PrimitiveOptions{})
	wantPaths := []string{
		"phillipmcmahon/code/archive/*",
	}
	if len(preview.RestorePaths) != len(wantPaths) {
		t.Fatalf("RestorePaths = %#v, want %#v", preview.RestorePaths, wantPaths)
	}
	for i := range wantPaths {
		if preview.RestorePaths[i] != wantPaths[i] {
			t.Fatalf("RestorePaths[%d] = %q, want %q", i, preview.RestorePaths[i], wantPaths[i])
		}
	}
}

func TestCompileSelectionAcrossBranchesProducesMultipleCommands(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"music/live/song.flac",
		"music/studio/song.flac",
	})

	ToggleSelection(root.Children[0].Children[0])
	ToggleSelection(root.Children[1].Children[0])

	preview := CompileSelection(root, PrimitiveOptions{
		ScriptName: "duplicacy-backup",
		Source:     "homes",
		Target:     "onsite-usb",
		Revision:   "2403",
		Workspace:  "/volume1/restore-drills/homes-onsite-usb",
	})

	if len(preview.RestorePaths) != 2 {
		t.Fatalf("len(RestorePaths) = %d, want 2 (%#v)", len(preview.RestorePaths), preview.RestorePaths)
	}
	if len(preview.Commands) != 2 {
		t.Fatalf("len(Commands) = %d, want 2", len(preview.Commands))
	}
	if len(preview.Notes) == 0 || !strings.Contains(preview.Notes[0], "2 explicit restore run commands") {
		t.Fatalf("Notes = %#v, want multi-command note", preview.Notes)
	}
}
