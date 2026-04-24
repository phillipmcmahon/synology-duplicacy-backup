package workflow

import (
	"io"
	"os"
	"time"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/restorepicker"
)

type RestoreDeps struct {
	NewRunner        func() execpkg.Runner
	PromptOutput     io.Writer
	Now              func() time.Time
	RunSelectPicker  func(paths []string, opts restorepicker.AppOptions) ([]string, error)
	RunInspectPicker func(paths []string, opts restorepicker.AppOptions) error
}

func defaultRestoreDeps() RestoreDeps {
	return RestoreDeps{
		NewRunner: func() execpkg.Runner {
			runner := execpkg.NewCommandRunner(nil, false)
			runner.SetDebugCommands(false)
			return runner
		},
		PromptOutput: os.Stdout,
		Now:          time.Now,
		RunSelectPicker: func(paths []string, opts restorepicker.AppOptions) ([]string, error) {
			filteredPaths, err := restorepicker.FilterPaths(paths, opts.PathPrefix)
			if err != nil {
				return nil, err
			}
			root := restorepicker.BuildTree(filteredPaths)
			return restorepicker.RunPicker(root, opts)
		},
		RunInspectPicker: func(paths []string, opts restorepicker.AppOptions) error {
			filteredPaths, err := restorepicker.FilterPaths(paths, opts.PathPrefix)
			if err != nil {
				return err
			}
			root := restorepicker.BuildTree(filteredPaths)
			return restorepicker.RunInspect(root, opts)
		},
	}
}

func (deps RestoreDeps) withDefaults() RestoreDeps {
	defaults := defaultRestoreDeps()
	if deps.NewRunner == nil {
		deps.NewRunner = defaults.NewRunner
	}
	if deps.PromptOutput == nil {
		deps.PromptOutput = defaults.PromptOutput
	}
	if deps.Now == nil {
		deps.Now = defaults.Now
	}
	if deps.RunSelectPicker == nil {
		deps.RunSelectPicker = defaults.RunSelectPicker
	}
	if deps.RunInspectPicker == nil {
		deps.RunInspectPicker = defaults.RunInspectPicker
	}
	return deps
}
