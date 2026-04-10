package main

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

var (
	version   = "2.0.0"
	buildTime = "unknown"
)

var cliArgs = func() []string { return os.Args[1:] }
var geteuid = os.Geteuid
var lookPath = osexec.LookPath
var newLock = lock.New

const scriptName = "duplicacy-backup"

var logDir = "/var/log"

func main() {
	os.Exit(run())
}

func run() int {
	return runWithArgs(cliArgs())
}

func runWithArgs(args []string) int {
	meta := workflow.DefaultMetadata(scriptName, version, buildTime, logDir)
	rt := workflow.DefaultRuntime()
	rt.Geteuid = geteuid
	rt.LookPath = lookPath
	rt.NewLock = newLock

	result, code := buildRequest(args, meta, rt)
	if code != 0 {
		return code
	}
	if result.Handled {
		fmt.Print(result.Output)
		return 0
	}

	log, err := initLogger(meta)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", err)
		return 1
	}
	log.SetVerbose(result.Request.Verbose)

	runner := execpkg.NewCommandRunner(log, result.Request.DryRun)
	planner := workflow.NewPlanner(meta, rt, log, runner)
	plan, err := planner.Build(result.Request)
	if err != nil {
		log.Error("%s", workflow.OperatorMessage(err))
		log.Close()
		return 1
	}

	executor := workflow.NewExecutor(meta, rt, log, runner, plan)
	return executor.Run()
}

func buildRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*workflow.ParseResult, int) {
	result, err := workflow.ParseRequest(args, meta, rt)
	if err == nil {
		return result, 0
	}

	log, logErr := initLogger(meta)
	if logErr != nil {
		fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", logErr)
		fmt.Fprintf(os.Stderr, "[ERRO] %v\n", err)
		return nil, 1
	}
	defer log.Close()

	log.Error("%s", workflow.OperatorMessage(err))
	var requestErr *workflow.RequestError
	if errors.As(err, &requestErr) && requestErr.ShowUsage {
		fmt.Fprintln(os.Stderr)
		fmt.Print(workflow.UsageText(meta, rt))
	}
	return nil, 1
}

func initLogger(meta workflow.Metadata) (*logger.Logger, error) {
	enableColour := logger.IsTerminal(os.Stderr)
	return logger.New(meta.LogDir, meta.ScriptName, enableColour)
}
