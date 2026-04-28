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

func TestCompileSelectionFullRootUnderPathPrefixUsesSubtreePattern(t *testing.T) {
	root := BuildTree([]string{
		"phillipmcmahon/code/archive/v5.0.0/a.tar.gz",
		"phillipmcmahon/code/archive/v5.1.0/b.tar.gz",
		"phillipmcmahon/code/docs/readme.md",
	})
	ToggleSelection(root)

	preview := CompileSelection(root, PrimitiveOptions{
		RootPath:  "phillipmcmahon/code",
		RootIsDir: true,
	})

	if preview.FullRestore {
		t.Fatalf("FullRestore = true, want false for prefixed subtree")
	}
	want := []string{"phillipmcmahon/code/*"}
	if len(preview.RestorePaths) != 1 || preview.RestorePaths[0] != want[0] {
		t.Fatalf("RestorePaths = %#v, want %#v", preview.RestorePaths, want)
	}
}

func TestCompileSelectionDirectoryCommandUsesDuplicacySubtreePattern(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"phillipmcmahon/code/archive/v5.0.0/a.tar.gz",
		"phillipmcmahon/code/archive/v5.1.0/b.tar.gz",
		"phillipmcmahon/music/song.flac",
	})
	ToggleSelection(root.Children[1].Children[0])

	preview := CompileSelection(root, PrimitiveOptions{
		ScriptName: "duplicacy-backup",
		Source:     "homes",
		Target:     "onsite-usb",
		Revision:   "2403",
		Workspace:  "/volume1/restore-drills/homes-onsite-usb",
	})

	wantPath := "phillipmcmahon/code/*"
	wantCommand := "'duplicacy-backup' restore run --target 'onsite-usb' --revision 2403 --workspace '/volume1/restore-drills/homes-onsite-usb' --yes --path 'phillipmcmahon/code/*' 'homes'"
	if len(preview.RestorePaths) != 1 || preview.RestorePaths[0] != wantPath {
		t.Fatalf("RestorePaths = %#v, want [%q]", preview.RestorePaths, wantPath)
	}
	if len(preview.Commands) != 1 || preview.Commands[0] != wantCommand {
		t.Fatalf("Commands = %#v, want [%q]", preview.Commands, wantCommand)
	}
}

func TestCompileSelectionCommandCanRequireSudo(t *testing.T) {
	root := BuildTree([]string{"phillipmcmahon/code/readme.md"})
	ToggleSelection(root.Children[0].Children[0])

	preview := CompileSelection(root, PrimitiveOptions{
		ScriptName:   "duplicacy-backup",
		Source:       "homes",
		Target:       "onsite-usb",
		Revision:     "2403",
		Workspace:    "/volume1/restore-drills/homes-onsite-usb",
		RequiresSudo: true,
	})

	if len(preview.Commands) != 1 || !strings.HasPrefix(preview.Commands[0], "sudo ") {
		t.Fatalf("Commands = %#v, want sudo-prefixed restore command", preview.Commands)
	}
}

func TestCompileSelectionNoSelectionProducesNoCommands(t *testing.T) {
	root := BuildTree([]string{"docs/readme.md"})

	preview := CompileSelection(root, PrimitiveOptions{})
	if preview.FullRestore {
		t.Fatalf("FullRestore = true, want false")
	}
	if len(preview.RestorePaths) != 0 || len(preview.Commands) != 0 || len(preview.Notes) != 0 {
		t.Fatalf("preview = %#v, want empty preview", preview)
	}
}

func TestCompileSelectionRootPathFileUsesExactPath(t *testing.T) {
	root := BuildTree([]string{"phillipmcmahon/code/readme.md"})
	ToggleSelection(root)

	preview := CompileSelection(root, PrimitiveOptions{
		RootPath:  "phillipmcmahon/code/readme.md",
		RootIsDir: false,
	})

	if preview.FullRestore {
		t.Fatalf("FullRestore = true, want false for prefixed file")
	}
	want := "phillipmcmahon/code/readme.md"
	if len(preview.RestorePaths) != 1 || preview.RestorePaths[0] != want {
		t.Fatalf("RestorePaths = %#v, want [%q]", preview.RestorePaths, want)
	}
}

func TestCompileSelectionEscapesShellArgumentsAndAddsComplexSelectionNote(t *testing.T) {
	paths := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		paths = append(paths, "dir"+string(rune('a'+i))+"/file.txt")
	}
	root := BuildTree(paths)
	for _, child := range root.Children[:11] {
		ToggleSelection(child.Children[0])
	}

	preview := CompileSelection(root, PrimitiveOptions{
		ScriptName: "duplicacy-backup",
		Source:     "home's",
		Target:     "onsite-usb",
		Revision:   "2403",
		Workspace:  "/volume1/restore drill",
	})

	if len(preview.Commands) != 11 {
		t.Fatalf("len(Commands) = %d, want 11", len(preview.Commands))
	}
	if len(preview.Notes) != 2 || !strings.Contains(preview.Notes[1], "current contract can become verbose") {
		t.Fatalf("Notes = %#v, want complex selection note", preview.Notes)
	}
	if !strings.Contains(preview.Commands[0], "'/volume1/restore drill'") {
		t.Fatalf("command did not quote workspace with space: %s", preview.Commands[0])
	}
	if !strings.Contains(preview.Commands[0], "'home'\\''s'") {
		t.Fatalf("command did not escape source quote: %s", preview.Commands[0])
	}
}
