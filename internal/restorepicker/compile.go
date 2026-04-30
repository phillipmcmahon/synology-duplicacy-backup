package restorepicker

import (
	"fmt"
	"sort"
	"strings"
)

type PrimitiveOptions struct {
	ScriptName   string
	Source       string
	Target       string
	Revision     string
	Workspace    string
	RootPath     string
	RootIsDir    bool
	RequiresSudo bool
}

type PrimitivePreview struct {
	FullRestore  bool
	RestorePaths []string
	Commands     []string
	Notes        []string
}

func DefaultPrimitiveOptions() PrimitiveOptions {
	return PrimitiveOptions{
		ScriptName: "duplicacy-backup",
		Source:     "<label>",
		Target:     "<target>",
		Revision:   "<revision>",
		Workspace:  "<workspace>",
	}
}

func CompileSelection(root *Node, opts PrimitiveOptions) PrimitivePreview {
	opts = normalisePrimitiveOptions(opts)
	paths := compileSelectionPaths(root, opts)
	preview := PrimitivePreview{
		FullRestore:  len(paths) == 1 && paths[0] == "",
		RestorePaths: append([]string(nil), paths...),
	}
	if len(paths) == 0 {
		return preview
	}
	for _, restorePath := range paths {
		preview.Commands = append(preview.Commands, buildPrimitiveCommand(opts, restorePath))
	}
	if len(preview.Commands) > 1 {
		preview.Notes = append(preview.Notes, fmt.Sprintf("Current selection expands to %d explicit restore run commands.", len(preview.Commands)))
	}
	if len(preview.Commands) > 10 {
		preview.Notes = append(preview.Notes, "Complex partial selections are expressible, but the current contract can become verbose.")
	}
	return preview
}

func normalisePrimitiveOptions(opts PrimitiveOptions) PrimitiveOptions {
	defaults := DefaultPrimitiveOptions()
	if strings.TrimSpace(opts.ScriptName) == "" {
		opts.ScriptName = defaults.ScriptName
	}
	if strings.TrimSpace(opts.Source) == "" {
		opts.Source = defaults.Source
	}
	if strings.TrimSpace(opts.Target) == "" {
		opts.Target = defaults.Target
	}
	if strings.TrimSpace(opts.Revision) == "" {
		opts.Revision = defaults.Revision
	}
	if strings.TrimSpace(opts.Workspace) == "" {
		opts.Workspace = defaults.Workspace
	}
	return opts
}

func compileSelectionPaths(root *Node, opts PrimitiveOptions) []string {
	if root == nil || root.Selection == SelectionNone {
		return nil
	}
	if root.Selection == SelectionFull {
		if rooted := rootedSelectionPath(opts); rooted != "" {
			return []string{rooted}
		}
		return []string{""}
	}
	var paths []string
	for _, child := range root.Children {
		paths = append(paths, compileSelectionPathsFromNode(child)...)
	}
	return uniqueSortedStrings(paths)
}

func rootedSelectionPath(opts PrimitiveOptions) string {
	rootPath := strings.Trim(strings.TrimSpace(opts.RootPath), "/")
	if rootPath == "" {
		return ""
	}
	if opts.RootIsDir {
		return directoryPattern(rootPath)
	}
	return rootPath
}

func compileSelectionPathsFromNode(node *Node) []string {
	if node == nil || node.Selection == SelectionNone {
		return nil
	}
	if node.Selection == SelectionFull {
		if node.IsDir {
			return []string{directoryPattern(node.Path)}
		}
		return []string{node.Path}
	}
	var paths []string
	for _, child := range node.Children {
		paths = append(paths, compileSelectionPathsFromNode(child)...)
	}
	return uniqueSortedStrings(paths)
}

func directoryPattern(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	// Duplicacy restore accepts this path glob as the subtree selection form
	// used by the existing NAS restore smoke tests.
	return path + "/*"
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func buildPrimitiveCommand(opts PrimitiveOptions, restorePath string) string {
	args := []string{}
	if opts.RequiresSudo {
		args = append(args, "sudo")
	}
	args = append(args,
		shellQuote(opts.ScriptName),
		"restore",
		"run",
		"--storage",
		shellQuote(opts.Target),
		"--revision",
		opts.Revision,
		"--workspace",
		shellQuote(opts.Workspace),
		"--yes",
	)
	if strings.TrimSpace(restorePath) != "" {
		args = append(args, "--path", shellQuote(restorePath))
	}
	args = append(args, shellQuote(opts.Source))
	return strings.Join(args, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
