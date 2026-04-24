package restorepicker

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
)

func TestLoadPathsSupportsPlainAndDuplicacyRows(t *testing.T) {
	input := strings.NewReader(`
docs/readme.md
5585354 2026-04-20 19:29:38 45fcaf55f07a698bd608e892802bd3f7275a8688374de79acbc5ebb078ebdc06 phillipmcmahon/code/archive/file one.tar.gz
Files: 2471
Snapshot data revision 1 created at 2026-04-23 02:30 -hash
Total size: 287254112235, file chunks: 6658, metadata chunks: 4
phillipmcmahon/music/song.flac
`)

	paths, err := LoadPaths(input, "")
	if err != nil {
		t.Fatalf("LoadPaths() error = %v", err)
	}
	want := []string{
		"docs/readme.md",
		"phillipmcmahon/code/archive/file one.tar.gz",
		"phillipmcmahon/music/song.flac",
	}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d (%v)", len(paths), len(want), paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestLoadPathsFiltersByPrefix(t *testing.T) {
	input := strings.NewReader("docs/readme.md\nphillipmcmahon/code/main.go\nphillipmcmahon/music/song.flac\n")

	paths, err := LoadPaths(input, "phillipmcmahon/code")
	if err != nil {
		t.Fatalf("LoadPaths() error = %v", err)
	}
	want := []string{"phillipmcmahon/code/main.go"}
	if len(paths) != len(want) || paths[0] != want[0] {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestFilterPathsFiltersByPrefix(t *testing.T) {
	paths, err := FilterPaths([]string{
		"docs/readme.md",
		"phillipmcmahon/code/main.go",
		"phillipmcmahon/code/internal/readme.md",
		"phillipmcmahon/music/song.flac",
	}, "phillipmcmahon/code")
	if err != nil {
		t.Fatalf("FilterPaths() error = %v", err)
	}
	want := []string{
		"phillipmcmahon/code/internal/readme.md",
		"phillipmcmahon/code/main.go",
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestBuildTreeCountsDirectoriesAndFiles(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"docs/reference/api.md",
		"music/live/song.flac",
	})

	dirs, files := CountNodes(root)
	if dirs != 4 {
		t.Fatalf("dirs = %d, want 4", dirs)
	}
	if files != 3 {
		t.Fatalf("files = %d, want 3", files)
	}
	if got := root.Children[0].Name; got != "docs" {
		t.Fatalf("first child = %q, want docs", got)
	}
	if got := root.Children[1].Name; got != "music" {
		t.Fatalf("second child = %q, want music", got)
	}
}

func TestToggleSelectionDirectoryToPartial(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"docs/reference/api.md",
		"music/live/song.flac",
	})

	docs := root.Children[0]
	if docs.Name != "docs" {
		t.Fatalf("docs node = %q, want docs", docs.Name)
	}
	ToggleSelection(docs)
	if docs.Selection != SelectionFull {
		t.Fatalf("docs selection = %v, want full", docs.Selection)
	}
	reference := docs.Children[0]
	if reference.Name != "reference" {
		t.Fatalf("reference node = %q, want reference", reference.Name)
	}
	ToggleSelection(reference)
	if reference.Selection != SelectionNone {
		t.Fatalf("reference selection = %v, want none", reference.Selection)
	}
	if docs.Selection != SelectionPartial {
		t.Fatalf("docs selection = %v, want partial", docs.Selection)
	}
	if root.Selection != SelectionPartial {
		t.Fatalf("root selection = %v, want partial", root.Selection)
	}
}

func TestToggleSelectionAcrossBranchesSummarisesMixedState(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"docs/reference/api.md",
		"music/live/song.flac",
		"music/studio/song.flac",
	})

	ToggleSelection(root.Children[0])
	ToggleSelection(root.Children[1].Children[0])

	summary := SummariseSelection(root)
	if summary.FullDirectories != 3 {
		t.Fatalf("full directories = %d, want 3", summary.FullDirectories)
	}
	if summary.PartialDirectories != 1 {
		t.Fatalf("partial directories = %d, want 1", summary.PartialDirectories)
	}
	if summary.SelectedFiles != 3 {
		t.Fatalf("selected files = %d, want 3", summary.SelectedFiles)
	}
}

func TestSelectionPrefix(t *testing.T) {
	cases := []struct {
		state SelectionState
		want  string
	}{
		{SelectionNone, "( )"},
		{SelectionPartial, "(~)"},
		{SelectionFull, "(x)"},
	}
	for _, tc := range cases {
		if got := SelectionPrefix(tc.state); got != tc.want {
			t.Fatalf("SelectionPrefix(%v) = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestToggleSelectionAllowsSnapshotRootSelection(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"docs/reference/api.md",
		"music/live/song.flac",
	})

	ToggleSelection(root)
	if root.Selection != SelectionFull {
		t.Fatalf("root selection = %v, want full", root.Selection)
	}
	summary := SummariseSelection(root)
	if summary.FullDirectories != 4 {
		t.Fatalf("full directories = %d, want 4", summary.FullDirectories)
	}
	if summary.SelectedFiles != 3 {
		t.Fatalf("selected files = %d, want 3", summary.SelectedFiles)
	}

	ToggleSelection(root)
	if root.Selection != SelectionNone {
		t.Fatalf("root selection after second toggle = %v, want none", root.Selection)
	}
	summary = SummariseSelection(root)
	if summary.FullDirectories != 0 || summary.PartialDirectories != 0 || summary.SelectedFiles != 0 {
		t.Fatalf("summary after clear = %#v, want all zero", summary)
	}
}

func TestNewAppStartsAtPathPrefixNode(t *testing.T) {
	root := BuildTree([]string{
		"phillipmcmahon/code/archive/v5.0.0/a.tar.gz",
		"phillipmcmahon/code/docs/readme.md",
		"phillipmcmahon/music/song.flac",
	})

	app := NewApp(root, AppOptions{PathPrefix: "phillipmcmahon/code"})
	tree, ok := app.GetFocus().(*tview.TreeView)
	if !ok {
		t.Fatalf("focus = %T, want *tview.TreeView", app.GetFocus())
	}
	current := tree.GetCurrentNode()
	ref, _ := current.GetReference().(*Node)
	if ref == nil {
		t.Fatalf("current node reference is nil")
	}
	if ref.Path != "phillipmcmahon/code" {
		t.Fatalf("current path = %q, want phillipmcmahon/code", ref.Path)
	}
}

func TestHandleExpandRightExpandsCollapsedDirectory(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"docs/reference/api.md",
	})

	rootNode := buildTreeNode(root)
	rootNode.SetExpanded(true)
	docsNode := rootNode.GetChildren()[0]
	docsNode.SetExpanded(false)

	tree := tview.NewTreeView().SetRoot(rootNode).SetCurrentNode(docsNode)

	if !handleExpandRight(tree, docsNode) {
		t.Fatalf("handleExpandRight() = false, want true")
	}
	if !docsNode.IsExpanded() {
		t.Fatalf("docs node expanded = false, want true")
	}
	if tree.GetCurrentNode() != docsNode {
		t.Fatalf("current node changed on expand, want docs node")
	}
}

func TestHandleExpandRightMovesIntoFirstChildWhenExpanded(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"docs/reference/api.md",
	})

	rootNode := buildTreeNode(root)
	rootNode.SetExpanded(true)
	docsNode := rootNode.GetChildren()[0]
	docsNode.SetExpanded(true)

	tree := tview.NewTreeView().SetRoot(rootNode).SetCurrentNode(docsNode)

	if !handleExpandRight(tree, docsNode) {
		t.Fatalf("handleExpandRight() = false, want true")
	}
	if tree.GetCurrentNode() != docsNode.GetChildren()[0] {
		t.Fatalf("current node = %v, want first child", tree.GetCurrentNode())
	}
}

func TestHandleCollapseLeftCollapsesExpandedDirectory(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"docs/reference/api.md",
	})

	rootNode := buildTreeNode(root)
	rootNode.SetExpanded(true)
	docsNode := rootNode.GetChildren()[0]
	docsNode.SetExpanded(true)

	tree := tview.NewTreeView().SetRoot(rootNode).SetCurrentNode(docsNode)

	if !handleCollapseLeft(tree, docsNode) {
		t.Fatalf("handleCollapseLeft() = false, want true")
	}
	if docsNode.IsExpanded() {
		t.Fatalf("docs node expanded = true, want false")
	}
	if tree.GetCurrentNode() != docsNode {
		t.Fatalf("current node changed on collapse, want docs node")
	}
}

func TestHandleCollapseLeftMovesToParentWhenAlreadyCollapsed(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"docs/reference/api.md",
	})

	rootNode := buildTreeNode(root)
	rootNode.SetExpanded(true)
	docsNode := rootNode.GetChildren()[0]
	docsNode.SetExpanded(false)
	readmeNode := docsNode.GetChildren()[1]

	tree := tview.NewTreeView().SetRoot(rootNode).SetCurrentNode(readmeNode)

	if !handleCollapseLeft(tree, readmeNode) {
		t.Fatalf("handleCollapseLeft() = false, want true")
	}
	if tree.GetCurrentNode() != docsNode {
		t.Fatalf("current node = %v, want docs node", tree.GetCurrentNode())
	}
}
