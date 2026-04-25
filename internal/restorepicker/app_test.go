package restorepicker

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
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

func TestHelpTextDistinguishesInspectAndGenerateModes(t *testing.T) {
	inspect := helpText(false)
	if !strings.Contains(inspect, "Interactive tree inspection") ||
		!strings.Contains(inspect, "q: quit inspection") ||
		strings.Contains(inspect, "g: continue") {
		t.Fatalf("inspect help text = %q", inspect)
	}

	generate := helpText(true)
	if !strings.Contains(generate, "Interactive tree picker") ||
		!strings.Contains(generate, "g: continue with the current selection and generate the restore commands") ||
		!strings.Contains(generate, "q: cancel") {
		t.Fatalf("generate help text = %q", generate)
	}
}

func TestNodeLabelShowsSelectionStateAndKind(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"docs/reference/api.md",
	})
	docs := root.Children[0]
	readme := docs.Children[1]

	if got := nodeLabel(docs); !strings.Contains(got, "( )") || !strings.Contains(got, "docs/") {
		t.Fatalf("nodeLabel(docs) = %q", got)
	}

	ToggleSelection(readme)
	if got := nodeLabel(readme); !strings.Contains(got, "(x)") || !strings.Contains(got, "<selected>") {
		t.Fatalf("nodeLabel(readme selected) = %q", got)
	}

	if got := nodeLabel(docs); !strings.Contains(got, "(~)") || !strings.Contains(got, "<partial>") {
		t.Fatalf("nodeLabel(docs partial) = %q", got)
	}

	ToggleSelection(docs)
	if got := nodeLabel(docs); !strings.Contains(got, "(x)") || !strings.Contains(got, "<selected subtree>") {
		t.Fatalf("nodeLabel(docs full) = %q", got)
	}
}

func TestRefreshTreeLabelsReflectsSelectionChanges(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"docs/reference/api.md",
	})
	rootNode := buildTreeNode(root)
	docs := root.Children[0]
	docsTreeNode := rootNode.GetChildren()[0]

	ToggleSelection(docs)
	refreshTreeLabels(rootNode)

	if got := docsTreeNode.GetText(); !strings.Contains(got, "(x)") || !strings.Contains(got, "<selected subtree>") {
		t.Fatalf("refreshed docs label = %q", got)
	}
}

func TestFormatPrimitivePreviewIncludesPathsCommandsAndNotes(t *testing.T) {
	preview := PrimitivePreview{
		RestorePaths: []string{"docs/*", "music/song.flac"},
		Commands: []string{
			"duplicacy-backup restore run --path 'docs/*'",
			"duplicacy-backup restore run --path music/song.flac",
		},
		Notes: []string{"2 explicit restore run commands will be generated."},
	}

	text := formatPrimitivePreview(preview)
	for _, token := range []string{
		"Mode              : Selective restore",
		"Restore paths     : 2",
		"- docs/*",
		"1. duplicacy-backup restore run --path 'docs/*'",
		"2 explicit restore run commands will be generated.",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("preview missing %q:\n%s", token, text)
		}
	}
}

func TestFormatPrimitivePreviewFullRestoreMarker(t *testing.T) {
	preview := PrimitivePreview{
		FullRestore:  true,
		RestorePaths: []string{""},
		Commands:     []string{"duplicacy-backup restore run --revision 2403"},
	}

	text := formatPrimitivePreview(preview)
	if !strings.Contains(text, "Mode              : Full restore") ||
		!strings.Contains(text, "- <full revision>") {
		t.Fatalf("preview = %q", text)
	}
}

func TestDisplayBreadcrumbBuildsPathFromParents(t *testing.T) {
	root := BuildTree([]string{
		"phillipmcmahon/code/duplicacy-backup/README.md",
	})
	readme := root.Children[0].Children[0].Children[0].Children[0]

	if got := displayBreadcrumb(readme); got != "phillipmcmahon/code/duplicacy-backup/README.md" {
		t.Fatalf("displayBreadcrumb() = %q", got)
	}
	if got := displayBreadcrumb(root); got != "<snapshot root>" {
		t.Fatalf("displayBreadcrumb(root) = %q", got)
	}
}

func TestSelectionNameReturnsOperatorWords(t *testing.T) {
	cases := []struct {
		state SelectionState
		want  string
	}{
		{SelectionNone, "none"},
		{SelectionPartial, "partial"},
		{SelectionFull, "full"},
	}
	for _, tc := range cases {
		if got := selectionName(tc.state); got != tc.want {
			t.Fatalf("selectionName(%v) = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestPickerInputCaptureTogglesSelectionAndGeneratesPrimitives(t *testing.T) {
	root := BuildTree([]string{
		"docs/readme.md",
		"music/song.flac",
	})
	session := &pickerSession{cancelled: true}
	app := newApp(root, AppOptions{
		Primitive: PrimitiveOptions{
			ScriptName: "duplicacy-backup",
			Source:     "homes",
			Target:     "onsite-usb",
			Revision:   "2403",
			Workspace:  "/volume1/restore-drills/homes",
		},
	}, session)

	tree, ok := app.GetFocus().(*tview.TreeView)
	if !ok {
		t.Fatalf("focus = %T, want *tview.TreeView", app.GetFocus())
	}
	capture := tree.GetInputCapture()
	if capture == nil {
		t.Fatal("tree input capture is nil")
	}

	if event := capture(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone)); event != nil {
		t.Fatalf("space event returned %v, want consumed", event)
	}
	if root.Selection != SelectionFull {
		t.Fatalf("root selection = %v, want full", root.Selection)
	}
	if event := capture(tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModNone)); event != nil {
		t.Fatalf("generate event returned %v, want consumed", event)
	}
	if !session.generated || session.cancelled {
		t.Fatalf("session = %#v, want generated and not cancelled", session)
	}
	if len(session.result) != 1 || session.result[0] != "" {
		t.Fatalf("session result = %#v, want full revision marker", session.result)
	}
}

func TestPickerInputCaptureReportsEmptySelectionAndCancel(t *testing.T) {
	root := BuildTree([]string{"docs/readme.md"})
	session := &pickerSession{cancelled: true}
	app := newApp(root, AppOptions{}, session)
	tree := app.GetFocus().(*tview.TreeView)
	capture := tree.GetInputCapture()

	if event := capture(tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModNone)); event != nil {
		t.Fatalf("generate event returned %v, want consumed", event)
	}
	if session.generated {
		t.Fatalf("session generated = true, want false")
	}
	if !strings.Contains(session.status, "Select at least one") {
		t.Fatalf("session status = %q, want empty-selection guidance", session.status)
	}

	if event := capture(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone)); event != nil {
		t.Fatalf("cancel event returned %v, want consumed", event)
	}
	if !session.cancelled {
		t.Fatalf("session cancelled = false, want true")
	}
}

func TestPickerInputCaptureSwitchesFocusAndInspectModeIgnoresSelection(t *testing.T) {
	root := BuildTree([]string{"docs/readme.md"})
	session := &pickerSession{cancelled: true}
	app := newApp(root, AppOptions{InspectOnly: true}, session)
	tree := app.GetFocus().(*tview.TreeView)
	capture := tree.GetInputCapture()

	if event := capture(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone)); event != nil {
		t.Fatalf("inspect space event returned %v, want consumed", event)
	}
	if root.Selection != SelectionNone {
		t.Fatalf("root selection = %v, want unchanged none", root.Selection)
	}
	if event := capture(tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModNone)); event != nil {
		t.Fatalf("inspect generate event returned %v, want consumed", event)
	}
	if session.generated {
		t.Fatalf("session generated = true, want false in inspect mode")
	}

	if event := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); event != nil {
		t.Fatalf("tab event returned %v, want consumed", event)
	}
	detail, ok := app.GetFocus().(*tview.TextView)
	if !ok {
		t.Fatalf("focus = %T, want *tview.TextView", app.GetFocus())
	}
	detailCapture := detail.GetInputCapture()
	if event := detailCapture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); event != nil {
		t.Fatalf("escape event returned %v, want consumed", event)
	}
	if _, ok := app.GetFocus().(*tview.TreeView); !ok {
		t.Fatalf("focus = %T, want tree after escape", app.GetFocus())
	}
}

func TestSessionStatusAndUpdateDetailHandleEmptyInputs(t *testing.T) {
	if got := sessionStatus(nil); got != "" {
		t.Fatalf("sessionStatus(nil) = %q, want empty", got)
	}
	if got := sessionStatus(&pickerSession{status: "  ready  "}); got != "ready" {
		t.Fatalf("sessionStatus(trim) = %q, want ready", got)
	}

	detail := tview.NewTextView()
	updateDetail(detail, BuildTree([]string{"docs/readme.md"}), nil, AppOptions{}, "ignored")
	if got := detail.GetText(false); got != "" {
		t.Fatalf("detail text after nil node = %q, want empty", got)
	}
	updateDetail(detail, BuildTree([]string{"docs/readme.md"}), tview.NewTreeNode("empty"), AppOptions{}, "ignored")
	if got := detail.GetText(false); got != "" {
		t.Fatalf("detail text after nil ref = %q, want empty", got)
	}
}

func TestTreeNavigationHelpersHandleMissingOrLeafNodes(t *testing.T) {
	tree := tview.NewTreeView()
	if handleExpandRight(nil, nil) || handleExpandRight(tree, nil) {
		t.Fatalf("handleExpandRight nil cases returned true")
	}
	leaf := buildTreeNode(BuildTree([]string{"docs/readme.md"}).Children[0].Children[0])
	tree.SetRoot(leaf).SetCurrentNode(leaf)
	if handleExpandRight(tree, leaf) {
		t.Fatalf("handleExpandRight leaf = true, want false")
	}
	if handleCollapseLeft(nil, nil) || handleCollapseLeft(tree, nil) {
		t.Fatalf("handleCollapseLeft nil cases returned true")
	}
	if handleCollapseLeft(tree, leaf) {
		t.Fatalf("handleCollapseLeft root leaf = true, want false")
	}
}
