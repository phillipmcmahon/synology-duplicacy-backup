package restorepicker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type AppOptions struct {
	Title       string
	PathPrefix  string
	Primitive   PrimitiveOptions
	InspectOnly bool
}

var ErrPickerCancelled = errors.New("restore picker cancelled")

func NewApp(root *Node, opts AppOptions) *tview.Application {
	return newApp(root, opts, nil)
}

func RunPicker(root *Node, opts AppOptions) ([]string, error) {
	state := &pickerSession{cancelled: true}
	app := newApp(root, opts, state)
	if err := app.Run(); err != nil {
		return nil, err
	}
	if state.generated {
		return append([]string(nil), state.result...), nil
	}
	return nil, ErrPickerCancelled
}

func RunInspect(root *Node, opts AppOptions) error {
	opts.InspectOnly = true
	app := newApp(root, opts, nil)
	return app.Run()
}

type pickerSession struct {
	result    []string
	status    string
	generated bool
	cancelled bool
}

func newApp(root *Node, opts AppOptions, session *pickerSession) *tview.Application {
	app := tview.NewApplication()
	tree := tview.NewTreeView()
	tree.SetBorder(true).SetTitle(" Restore Tree Picker ")
	tree.SetGraphicsColor(tcell.ColorDarkSlateGray)
	tree.SetSelectedFunc(nil)

	detail := tview.NewTextView()
	detail.SetBorder(true).SetTitle(" Primitive Detail ")
	detail.SetDynamicColors(true)
	detail.SetScrollable(true)

	help := tview.NewTextView()
	help.SetBorder(true).SetTitle(" Controls ")
	help.SetDynamicColors(true)
	help.SetText(helpText(session != nil))
	rootNode := buildTreeNode(root)
	rootNode.SetExpanded(true)
	tree.SetRoot(rootNode).SetCurrentNode(rootNode)
	if prefixNode := findTreeNodeByPath(rootNode, opts.PathPrefix); prefixNode != nil {
		tree.SetCurrentNode(prefixNode)
	}

	updatePanels := func(node *tview.TreeNode) {
		updateDetail(detail, root, node, opts, sessionStatus(session))
	}
	setStatus := func(message string) {
		if session != nil {
			session.status = strings.TrimSpace(message)
		}
		updatePanels(tree.GetCurrentNode())
	}
	commitSelection := func() bool {
		if session == nil {
			return false
		}
		preview := CompileSelection(root, opts.Primitive)
		if len(preview.RestorePaths) == 0 {
			setStatus("Select at least one file or directory before continuing.")
			return false
		}
		session.result = append([]string(nil), preview.RestorePaths...)
		session.generated = true
		session.cancelled = false
		app.Stop()
		return true
	}
	handleGlobalKey := func(event *tcell.EventKey, allowNavigation bool) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			return nil
		}
		switch event.Rune() {
		case 'q', 'Q':
			if session != nil {
				session.cancelled = true
			}
			app.Stop()
			return nil
		case 'g', 'G':
			if opts.InspectOnly {
				return nil
			}
			if commitSelection() {
				return nil
			}
			return nil
		}
		if !allowNavigation {
			return event
		}
		node := tree.GetCurrentNode()
		switch event.Key() {
		case tcell.KeyRight:
			if handleExpandRight(tree, node) {
				updatePanels(tree.GetCurrentNode())
				return nil
			}
		case tcell.KeyLeft:
			if handleCollapseLeft(tree, node) {
				updatePanels(tree.GetCurrentNode())
				return nil
			}
		}
		switch event.Rune() {
		case ' ':
			if opts.InspectOnly {
				return nil
			}
			ref, _ := node.GetReference().(*Node)
			if ref == nil {
				return nil
			}
			ToggleSelection(ref)
			refreshTreeLabels(rootNode)
			tree.SetCurrentNode(node)
			setStatus("")
			return nil
		}
		return event
	}

	setPaneFocus := func(focusDetail bool) {
		if focusDetail {
			app.SetFocus(detail)
			tree.SetBorderColor(tcell.ColorDarkSlateGray)
			tree.SetBorderAttributes(tcell.AttrNone)
			tree.SetTitleColor(tcell.ColorDarkGray)
			detail.SetBorderColor(tcell.ColorYellow)
			detail.SetBorderAttributes(tcell.AttrBold)
			detail.SetTitleColor(tcell.ColorYellow)
			tree.SetTitle(" Restore Tree Picker ")
			detail.SetTitle(" [ACTIVE] PRIMITIVE DETAIL ")
			return
		}
		app.SetFocus(tree)
		tree.SetBorderColor(tcell.ColorYellow)
		tree.SetBorderAttributes(tcell.AttrBold)
		tree.SetTitleColor(tcell.ColorYellow)
		detail.SetBorderColor(tcell.ColorDarkSlateGray)
		detail.SetBorderAttributes(tcell.AttrNone)
		detail.SetTitleColor(tcell.ColorDarkGray)
		tree.SetTitle(" [ACTIVE] RESTORE TREE PICKER ")
		detail.SetTitle(" Primitive Detail ")
	}
	tree.SetChangedFunc(func(node *tview.TreeNode) {
		updatePanels(node)
	})
	tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			setPaneFocus(true)
			return nil
		}
		return handleGlobalKey(event, true)
	})
	detail.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyBacktab:
			setPaneFocus(false)
			return nil
		case tcell.KeyEscape:
			setPaneFocus(false)
			return nil
		}
		return handleGlobalKey(event, false)
	})
	detail.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyTab || key == tcell.KeyBacktab || key == tcell.KeyEscape {
			setPaneFocus(false)
		}
	})

	dirs, files := CountNodes(root)
	header := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf("[green]Restore Tree Picker[-]  Paths: %d  Directories: %d  Files: %d", dirs+files, dirs, files))

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(
			tview.NewFlex().
				AddItem(tree, 0, 2, true).
				AddItem(detail, 0, 1, false),
			0, 1, true,
		).
		AddItem(help, 10, 0, false)

	app.SetRoot(flex, true)
	updatePanels(rootNode)
	setPaneFocus(false)
	return app
}

func helpText(enableGenerate bool) string {
	if !enableGenerate {
		return `[yellow]Interactive tree inspection[-]
Arrow keys: navigate
Right: expand directory
Left: collapse directory
Tab: switch focus between tree and detail
When detail is focused: Up/Down/PgUp/PgDn scroll the right-hand panel
q: quit inspection

[green](x)[-] fully selected
[yellow](~)[-] partially selected
[gray]( )[-] clear
Focused pane uses a bright bold border and [ACTIVE] title markers.
Current row uses a high-contrast focus highlight.
Inspect mode does not generate restore commands.`
	}
	actionLine := "g: continue with the current selection and generate the restore commands"
	return `[yellow]Interactive tree picker[-]
Arrow keys: navigate
Right: expand directory
Left: collapse directory
Space: toggle full select / clear on current node
Tab: switch focus between tree and detail
When detail is focused: Up/Down/PgUp/PgDn scroll the right-hand panel
` + actionLine + `
q: cancel and leave restore select

[green](x)[-] fully selected
[yellow](~)[-] partially selected
[gray]( )[-] clear
Focused pane uses a bright bold border and [ACTIVE] title markers.
Current row uses a high-contrast focus highlight.
Selected rows should still keep their state marker and text cue.`
}

func buildTreeNode(node *Node) *tview.TreeNode {
	treeNode := tview.NewTreeNode(nodeLabel(node)).SetReference(node)
	applyNodeStyle(treeNode, node)
	if node.IsDir {
		for _, child := range node.Children {
			treeNode.AddChild(buildTreeNode(child))
		}
		return treeNode
	}
	return treeNode
}

func findTreeNodeByPath(node *tview.TreeNode, path string) *tview.TreeNode {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if node == nil || path == "" {
		return nil
	}
	ref, _ := node.GetReference().(*Node)
	if ref != nil && strings.Trim(ref.Path, "/") == path {
		return node
	}
	for _, child := range node.GetChildren() {
		if found := findTreeNodeByPath(child, path); found != nil {
			return found
		}
	}
	return nil
}

func displayEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func nodeLabel(node *Node) string {
	name := node.Name
	if node.Path == "" {
		name = node.Name
	} else if node.IsDir {
		name += "/"
	}
	suffix := ""
	switch node.Selection {
	case SelectionFull:
		if node.IsDir {
			suffix = "  <selected subtree>"
		} else {
			suffix = "  <selected>"
		}
	case SelectionPartial:
		suffix = "  <partial>"
	}
	return fmt.Sprintf(" %s  %s%s", SelectionPrefix(node.Selection), name, suffix)
}

func refreshTreeLabels(node *tview.TreeNode) {
	ref, _ := node.GetReference().(*Node)
	if ref != nil {
		node.SetText(nodeLabel(ref))
		applyNodeStyle(node, ref)
	}
	for _, child := range node.GetChildren() {
		refreshTreeLabels(child)
	}
}

func sessionStatus(session *pickerSession) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.status)
}

func updateDetail(detail *tview.TextView, root *Node, node *tview.TreeNode, opts AppOptions, status string) {
	if node == nil {
		return
	}
	ref, _ := node.GetReference().(*Node)
	if ref == nil {
		return
	}
	kind := "file"
	children := 0
	if ref.IsDir {
		kind = "directory"
		children = len(ref.Children)
	}
	summary := SummariseSelection(root)
	preview := CompileSelection(root, opts.Primitive)
	statusBlock := ""
	if status != "" {
		statusBlock = fmt.Sprintf("\n[yellow]Status:[-] %s\n", status)
	}
	previewBlock := formatPrimitivePreview(preview)
	if opts.InspectOnly {
		previewBlock = "  Inspection mode only. Browse the revision contents and press q when you are done.\n  No restore commands are generated in this mode."
	}
	detail.SetText(fmt.Sprintf(
		"[yellow]Title:[-] %s\n[yellow]Prefix:[-] %s\n[yellow]Kind:[-] %s\n[yellow]Path:[-] %s\n[yellow]State:[-] %s\n[yellow]Visible Children:[-] %d\n\n[yellow]Selection Summary:[-]\n  Full directories : %d\n  Partial dirs     : %d\n  Selected files   : %d\n\n[yellow]Current breadcrumb:[-] %s%s\n[yellow]Primitive Preview:[-]\n%s",
		displayEmpty(opts.Title, "Restore Tree Picker POC"),
		displayEmpty(opts.PathPrefix, "<snapshot root>"),
		kind,
		displayEmpty(ref.Path, "<snapshot root>"),
		selectionName(ref.Selection),
		children,
		summary.FullDirectories,
		summary.PartialDirectories,
		summary.SelectedFiles,
		displayBreadcrumb(ref),
		statusBlock,
		previewBlock,
	))
	detail.ScrollToBeginning()
}

func formatPrimitivePreview(preview PrimitivePreview) string {
	if len(preview.Commands) == 0 {
		return "  No restore primitives yet. Use Space to build a selection."
	}
	lines := make([]string, 0, 3+len(preview.RestorePaths)+len(preview.Commands)+len(preview.Notes))
	mode := "Selective restore"
	if preview.FullRestore {
		mode = "Full restore"
	}
	lines = append(lines, "  Mode              : "+mode)
	lines = append(lines, fmt.Sprintf("  Restore paths     : %d", len(preview.RestorePaths)))
	for _, restorePath := range preview.RestorePaths {
		display := restorePath
		if display == "" {
			display = "<full revision>"
		}
		lines = append(lines, "    - "+display)
	}
	lines = append(lines, "  Commands:")
	for i, command := range preview.Commands {
		lines = append(lines, fmt.Sprintf("    %d. %s", i+1, command))
	}
	if len(preview.Notes) > 0 {
		lines = append(lines, "  Notes:")
		for _, note := range preview.Notes {
			lines = append(lines, "    - "+note)
		}
	}
	return strings.Join(lines, "\n")
}

func selectionName(state SelectionState) string {
	switch state {
	case SelectionFull:
		return "full"
	case SelectionPartial:
		return "partial"
	default:
		return "none"
	}
}

func displayBreadcrumb(node *Node) string {
	if node == nil || node.Path == "" {
		return "<snapshot root>"
	}
	parts := []string{}
	for current := node; current != nil && current.Path != ""; current = current.Parent {
		parts = append(parts, current.Name)
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	return strings.Join(parts, "/")
}

func applyNodeStyle(treeNode *tview.TreeNode, node *Node) {
	var base tcell.Style
	var selected tcell.Style

	switch node.Selection {
	case SelectionFull:
		base = tcell.StyleDefault.Foreground(tcell.ColorLime).Background(tview.Styles.PrimitiveBackgroundColor).Bold(true)
		selected = tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkGreen).Bold(true)
	case SelectionPartial:
		base = tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tview.Styles.PrimitiveBackgroundColor).Bold(true)
		selected = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorGoldenrod).Bold(true)
	default:
		if node.IsDir {
			base = tcell.StyleDefault.Foreground(tcell.ColorLightSteelBlue).Background(tview.Styles.PrimitiveBackgroundColor).Bold(true)
			selected = tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkSlateBlue).Bold(true)
		} else {
			base = tcell.StyleDefault.Foreground(tcell.ColorSilver).Background(tview.Styles.PrimitiveBackgroundColor)
			selected = tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDimGray).Bold(true)
		}
	}

	treeNode.SetTextStyle(base)
	treeNode.SetSelectedTextStyle(selected)
}

func handleExpandRight(tree *tview.TreeView, node *tview.TreeNode) bool {
	if tree == nil || node == nil {
		return false
	}
	ref, _ := node.GetReference().(*Node)
	if ref == nil || !ref.IsDir {
		return false
	}
	if !node.IsExpanded() {
		node.SetExpanded(true)
		return true
	}
	children := node.GetChildren()
	if len(children) == 0 {
		return true
	}
	tree.SetCurrentNode(children[0])
	return true
}

func handleCollapseLeft(tree *tview.TreeView, node *tview.TreeNode) bool {
	if tree == nil || node == nil {
		return false
	}
	ref, _ := node.GetReference().(*Node)
	if ref == nil {
		return false
	}
	if ref.IsDir && node.IsExpanded() && len(node.GetChildren()) > 0 {
		node.SetExpanded(false)
		return true
	}
	parent := findParentTreeNode(tree.GetRoot(), node)
	if parent != nil {
		tree.SetCurrentNode(parent)
		return true
	}
	return false
}

func findParentTreeNode(root, target *tview.TreeNode) *tview.TreeNode {
	if root == nil || target == nil {
		return nil
	}
	for _, child := range root.GetChildren() {
		if child == target {
			return root
		}
		if parent := findParentTreeNode(child, target); parent != nil {
			return parent
		}
	}
	return nil
}
