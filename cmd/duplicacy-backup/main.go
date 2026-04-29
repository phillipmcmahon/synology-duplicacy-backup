package main

import (
	"fmt"
	"os"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/restore"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

var cliArgs = func() []string { return os.Args[1:] }
var defaultEnv = workflow.DefaultEnv
var handleConfigCommand = workflow.HandleConfigCommand
var handleDiagnosticsCommand = workflow.HandleDiagnosticsCommand
var handleNotifyCommand = workflow.HandleNotifyCommand
var handleRestoreCommand = restore.HandleRestoreCommand
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
	rt := defaultEnv()
	meta := workflow.DefaultMetadataForEnv(scriptName, version, buildTime, rt)
	if logDir != "" {
		meta.LogDir = logDir
	}
	rt = envWithProfileOwner(rt, meta)

	result, code := buildRequest(args, meta, rt)
	if code != 0 {
		return code
	}
	if result.Handled {
		fmt.Print(result.Output)
		return 0
	}
	return dispatchCommand(result.Command, meta, rt)
}

func initLogger(meta workflow.Metadata) (*logger.Logger, error) {
	enableColour := logger.ColourEnabled(os.Stderr)
	if meta.HasProfileOwner {
		return logger.NewWithOwner(meta.LogDir, meta.ScriptName, enableColour, meta.ProfileOwnerUID, meta.ProfileOwnerGID)
	}
	return logger.New(meta.LogDir, meta.ScriptName, enableColour)
}

func envWithProfileOwner(rt workflow.Env, meta workflow.Metadata) workflow.Env {
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

func printFailureCompletion(meta workflow.Metadata, rt workflow.Env, log *logger.Logger, startedAt time.Time) {
	workflow.NewPresenter(meta, rt, log, false).PrintCompletion(1, startedAt)
}
