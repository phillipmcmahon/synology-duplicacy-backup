package main

import (
	"fmt"
	"os"
	osexec "os/exec"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

var cliArgs = func() []string { return os.Args[1:] }
var geteuid = os.Geteuid
var lookPath = osexec.LookPath
var newLock = lock.New
var newSourceLock = lock.NewSource
var handleConfigCommand = workflow.HandleConfigCommand
var handleDiagnosticsCommand = workflow.HandleDiagnosticsCommand
var handleNotifyCommand = workflow.HandleNotifyCommand
var handleRestoreCommand = workflow.HandleRestoreCommand
var handleRollbackCommand = handleRollbackRequest
var handleUpdateCommand = handleUpdateRequest
var maybeSendPreRunFailureNotification = workflow.MaybeSendPreRunFailureNotification

const scriptName = "duplicacy-backup"

// logDir is test-only by default. Production uses Metadata.LogDir from the
// runtime user profile unless a test overrides this package variable.
var logDir string

func main() {
	os.Exit(run())
}

func run() int {
	return runWithArgs(cliArgs())
}

func runWithArgs(args []string) int {
	rt := workflow.DefaultRuntime()
	rt.Geteuid = geteuid
	rt.LookPath = lookPath
	rt.NewLock = newLock
	rt.NewSourceLock = newSourceLock
	meta := workflow.DefaultMetadataForRuntime(scriptName, version, buildTime, rt)
	if logDir != "" {
		meta.LogDir = logDir
	}
	rt = runtimeWithProfileOwner(rt, meta)

	result, code := buildRequest(args, meta, rt)
	if code != 0 {
		return code
	}
	if result.Handled {
		fmt.Print(result.Output)
		return 0
	}
	return dispatchRequest(result.Request, meta, rt)
}

func initLogger(meta workflow.Metadata) (*logger.Logger, error) {
	enableColour := logger.ColourEnabled(os.Stderr)
	if meta.HasProfileOwner {
		return logger.NewWithOwner(meta.LogDir, meta.ScriptName, enableColour, meta.ProfileOwnerUID, meta.ProfileOwnerGID)
	}
	return logger.New(meta.LogDir, meta.ScriptName, enableColour)
}

func runtimeWithProfileOwner(rt workflow.Runtime, meta workflow.Metadata) workflow.Runtime {
	if !meta.HasProfileOwner {
		return rt
	}
	rt.NewLock = func(lockParent, label string) *lock.Lock {
		return lock.NewWithOwner(lockParent, label, meta.ProfileOwnerUID, meta.ProfileOwnerGID)
	}
	rt.NewSourceLock = func(lockParent, label string) *lock.Lock {
		return lock.NewSourceWithOwner(lockParent, label, meta.ProfileOwnerUID, meta.ProfileOwnerGID)
	}
	return rt
}

func printFailureCompletion(meta workflow.Metadata, rt workflow.Runtime, log *logger.Logger, startedAt time.Time) {
	workflow.NewPresenter(meta, rt, log, false).PrintCompletion(1, startedAt)
}
