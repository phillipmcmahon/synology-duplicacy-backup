package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/restorepicker"
)

func main() {
	var inputFile string
	var pathPrefix string
	var title string
	var dumpOnly bool
	var scriptName string
	var source string
	var target string
	var revision string
	var workspace string

	flag.StringVar(&inputFile, "input-file", "", "Path to a plain path list or exported duplicacy list -files output (default: stdin)")
	flag.StringVar(&pathPrefix, "path-prefix", "", "Optional snapshot-relative prefix to load beneath")
	flag.StringVar(&title, "title", "Restore Tree Picker POC", "Window title text")
	flag.BoolVar(&dumpOnly, "dump-only", false, "Print parsed paths and exit without launching the TUI")
	flag.StringVar(&scriptName, "script-name", "duplicacy-backup", "Script name used in primitive restore command previews")
	flag.StringVar(&source, "source", "homes", "Source label placeholder used in primitive restore command previews")
	flag.StringVar(&target, "target", "onsite-usb", "Target placeholder used in primitive restore command previews")
	flag.StringVar(&revision, "revision", "2403", "Revision placeholder used in primitive restore command previews")
	flag.StringVar(&workspace, "workspace", "/volume1/restore-drills/homes-onsite-usb", "Workspace placeholder used in primitive restore command previews")
	flag.Parse()

	input, closeFn, err := openInput(inputFile)
	if err != nil {
		fatal(err)
	}
	defer closeFn()

	paths, err := restorepicker.LoadPaths(input, pathPrefix)
	if err != nil {
		fatal(err)
	}
	if len(paths) == 0 {
		fatal(fmt.Errorf("no snapshot-relative paths were loaded"))
	}
	if dumpOnly {
		for _, path := range paths {
			fmt.Println(path)
		}
		return
	}

	root := restorepicker.BuildTree(paths)
	app := restorepicker.NewApp(root, restorepicker.AppOptions{
		Title:      title,
		PathPrefix: pathPrefix,
		Primitive: restorepicker.PrimitiveOptions{
			ScriptName: scriptName,
			Source:     source,
			Target:     target,
			Revision:   revision,
			Workspace:  workspace,
		},
	})
	if err := app.Run(); err != nil {
		fatal(err)
	}
}

func openInput(path string) (io.Reader, func(), error) {
	if path == "" || path == "-" {
		return os.Stdin, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open input file: %w", err)
	}
	return file, func() { _ = file.Close() }, nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "restore-picker-poc: %v\n", err)
	os.Exit(1)
}
