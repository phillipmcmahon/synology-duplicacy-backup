package main

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/update"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func handleUpdateRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (update.Result, error) {
	updater := update.New(meta.ScriptName, meta.Version, update.Runtime{
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		Stdin:        rt.Stdin,
		StdinIsTTY:   rt.StdinIsTTY,
		CommandPath:  func() string { return os.Args[0] },
		LookPath:     exec.LookPath,
		Executable:   rt.Executable,
		EvalSymlinks: rt.EvalSymlinks,
		TempDir:      rt.TempDir,
		MkdirTemp:    os.MkdirTemp,
		RemoveAll:    os.RemoveAll,
	})
	return updater.RunResult(updateOptionsFromRequest(req))
}

func updateOptionsFromRequest(req *workflow.Request) update.Options {
	if req == nil {
		return update.Options{Keep: update.DefaultKeep}
	}
	return update.Options{
		RequestedVersion: req.UpdateVersion,
		CheckOnly:        req.UpdateCheckOnly,
		Force:            req.UpdateForce,
		Yes:              req.UpdateYes,
		Keep:             req.UpdateKeep,
	}
}

func updateStatusForWorkflow(status update.Status) workflow.UpdateStatus {
	switch status {
	case update.StatusInstalled:
		return workflow.UpdateStatusInstalled
	case update.StatusCurrent:
		return workflow.UpdateStatusCurrent
	case update.StatusAvailable:
		return workflow.UpdateStatusAvailable
	case update.StatusReinstallRequested:
		return workflow.UpdateStatusReinstallRequested
	case update.StatusFailed:
		return workflow.UpdateStatusFailed
	case update.StatusCancelled:
		return workflow.UpdateStatusCancelled
	default:
		return workflow.UpdateStatusUnknown
	}
}
