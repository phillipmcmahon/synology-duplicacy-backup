package restorepicker

import (
	"path/filepath"
	"slices"
	"strings"
)

type Node struct {
	Name      string
	Path      string
	IsDir     bool
	Children  []*Node
	Parent    *Node
	Selection SelectionState
	index     map[string]*Node
}

type SelectionState int

const (
	SelectionNone SelectionState = iota
	SelectionPartial
	SelectionFull
)

func BuildTree(paths []string) *Node {
	root := &Node{
		Name:  "<snapshot root>",
		Path:  "",
		IsDir: true,
		index: map[string]*Node{},
	}
	for _, path := range paths {
		current := root
		parts := strings.Split(filepath.ToSlash(path), "/")
		for i, part := range parts {
			if part == "" {
				continue
			}
			childPath := part
			if current.Path != "" {
				childPath = current.Path + "/" + part
			}
			child := current.index[part]
			if child == nil {
				child = &Node{
					Name:      part,
					Path:      childPath,
					IsDir:     i < len(parts)-1,
					Parent:    current,
					Selection: SelectionNone,
					index:     map[string]*Node{},
				}
				current.index[part] = child
				current.Children = append(current.Children, child)
			}
			if i < len(parts)-1 {
				child.IsDir = true
			}
			current = child
		}
	}
	sortNode(root)
	return root
}

func sortNode(node *Node) {
	slices.SortFunc(node.Children, func(a, b *Node) int {
		if a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	for _, child := range node.Children {
		sortNode(child)
	}
}

func CountNodes(node *Node) (dirs, files int) {
	for _, child := range node.Children {
		if child.IsDir {
			dirs++
			subDirs, subFiles := CountNodes(child)
			dirs += subDirs
			files += subFiles
			continue
		}
		files++
	}
	return dirs, files
}

func ToggleSelection(node *Node) {
	if node == nil {
		return
	}
	target := SelectionFull
	if node.Selection == SelectionFull {
		target = SelectionNone
	}
	setSelectionRecursive(node, target)
	updateAncestorSelection(node.Parent)
}

func setSelectionRecursive(node *Node, state SelectionState) {
	if node == nil {
		return
	}
	node.Selection = state
	for _, child := range node.Children {
		setSelectionRecursive(child, state)
	}
}

func updateAncestorSelection(node *Node) {
	for node != nil {
		node.Selection = deriveSelection(node)
		node = node.Parent
	}
}

func deriveSelection(node *Node) SelectionState {
	if len(node.Children) == 0 {
		return node.Selection
	}
	allFull := true
	allNone := true
	for _, child := range node.Children {
		switch child.Selection {
		case SelectionFull:
			allNone = false
		case SelectionNone:
			allFull = false
		default:
			allFull = false
			allNone = false
		}
	}
	switch {
	case allFull:
		return SelectionFull
	case allNone:
		return SelectionNone
	default:
		return SelectionPartial
	}
}

func SelectionPrefix(state SelectionState) string {
	switch state {
	case SelectionFull:
		return "(x)"
	case SelectionPartial:
		return "(~)"
	default:
		return "( )"
	}
}

type SelectionSummary struct {
	FullDirectories    int
	PartialDirectories int
	SelectedFiles      int
}

func SummariseSelection(node *Node) SelectionSummary {
	var summary SelectionSummary
	summariseSelection(node, &summary)
	return summary
}

func summariseSelection(node *Node, summary *SelectionSummary) {
	for _, child := range node.Children {
		if child.IsDir {
			switch child.Selection {
			case SelectionFull:
				summary.FullDirectories++
			case SelectionPartial:
				summary.PartialDirectories++
			}
			summariseSelection(child, summary)
			continue
		}
		if child.Selection == SelectionFull {
			summary.SelectedFiles++
		}
	}
}
