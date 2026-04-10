package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"time"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

var (
	version   = "2.1.4"
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
	if result.Request.ConfigCommand != "" {
		output, err := workflow.HandleConfigCommand(result.Request, meta, rt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERRO] %s\n", workflow.OperatorMessage(err))
			return 1
		}
		fmt.Print(output)
		return 0
	}
	if result.Request.HealthCommand != "" {
		log, err := initLogger(meta)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", err)
			if result.Request.JSONSummary {
				report := workflow.NewFailureHealthReport(result.Request, result.Request.HealthCommand, fmt.Sprintf("Failed to initialise logger: %v", err), rt.Now())
				_ = workflow.WriteHealthReport(os.Stdout, report)
			}
			return 2
		}
		log.SetVerbose(result.Request.Verbose)
		runner := execpkg.NewCommandRunner(log, false)
		report, code := workflow.NewHealthRunner(meta, rt, log, runner).Run(result.Request)
		if result.Request.JSONSummary {
			if err := workflow.WriteHealthReport(os.Stdout, report); err != nil {
				fmt.Fprintf(os.Stderr, "[ERRO] Failed to write JSON summary: %v\n", err)
				if code == 0 {
					return 2
				}
			}
		}
		log.Close()
		return code
	}

	log, err := initLogger(meta)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", err)
		if result.Request.JSONSummary {
			now := rt.Now()
			emitJSONFailureSummary(os.Stdout, result.Request, now, now, fmt.Sprintf("Failed to initialise logger: %v", err))
		}
		return 1
	}
	log.SetVerbose(result.Request.Verbose)
	startedAt := rt.Now()

	runner := execpkg.NewCommandRunner(log, result.Request.DryRun)
	planner := workflow.NewPlanner(meta, rt, log, runner)
	plan, err := planner.Build(result.Request)
	if err != nil {
		log.Error("%s", workflow.OperatorMessage(err))
		printFailureCompletion(meta, rt, log, startedAt)
		if result.Request.JSONSummary {
			emitJSONFailureSummary(os.Stdout, result.Request, startedAt, rt.Now(), workflow.OperatorMessage(err))
		}
		log.Close()
		return 1
	}

	executor := workflow.NewExecutor(meta, rt, log, runner, plan)
	code = executor.Run()
	if plan.JSONSummary {
		if err := workflow.WriteRunReport(os.Stdout, executor.Report()); err != nil {
			fmt.Fprintf(os.Stderr, "[ERRO] Failed to write JSON summary: %v\n", err)
			if code == 0 {
				return 1
			}
		}
	}
	return code
}

func buildRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*workflow.ParseResult, int) {
	startedAt := rt.Now()
	result, err := workflow.ParseRequest(args, meta, rt)
	if err == nil {
		return result, 0
	}

	log, logErr := initLogger(meta)
	if logErr != nil {
		fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", logErr)
		fmt.Fprintf(os.Stderr, "[ERRO] %v\n", err)
		if wantsJSONSummary(args) {
			emitJSONFailureSummary(os.Stdout, nil, startedAt, rt.Now(), err.Error())
		}
		return nil, 1
	}
	defer log.Close()

	log.Error("%s", workflow.OperatorMessage(err))
	completedAt := rt.Now()
	printFailureCompletion(meta, rt, log, startedAt)
	if wantsJSONSummary(args) {
		if looksLikeHealthCommand(args) {
			req := inferHealthFailureRequest(args)
			_ = workflow.WriteHealthReport(os.Stdout, workflow.NewFailureHealthReport(req, req.HealthCommand, workflow.OperatorMessage(err), completedAt))
		} else {
			emitJSONFailureSummary(os.Stdout, nil, startedAt, completedAt, workflow.OperatorMessage(err))
		}
	}
	var requestErr *workflow.RequestError
	if errors.As(err, &requestErr) && requestErr.ShowUsage && !wantsJSONSummary(args) {
		fmt.Fprintln(os.Stderr)
		fmt.Print(workflow.UsageText(meta, rt))
	}
	return nil, 1
}

func wantsJSONSummary(args []string) bool {
	for _, arg := range args {
		if arg == "--json-summary" {
			return true
		}
	}
	return false
}

func looksLikeHealthCommand(args []string) bool {
	return len(args) > 0 && args[0] == "health"
}

func inferHealthFailureRequest(args []string) *workflow.Request {
	req := &workflow.Request{}
	if len(args) == 0 || args[0] != "health" {
		return req
	}
	if len(args) > 1 && args[1] != "" && args[1][0] != '-' {
		req.HealthCommand = args[1]
	}
	var positional []string
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--remote":
			req.RemoteMode = true
		case "--json-summary":
			req.JSONSummary = true
		case "--verbose":
			req.Verbose = true
		case "--config-dir", "--secrets-dir":
			i++
		default:
			if len(args[i]) > 0 && args[i][0] != '-' {
				positional = append(positional, args[i])
			}
		}
	}
	if len(positional) > 0 {
		req.Source = positional[0]
	}
	return req
}

func emitJSONFailureSummary(w io.Writer, req *workflow.Request, startedAt, completedAt time.Time, message string) {
	if w == nil {
		return
	}
	if err := workflow.WriteRunReport(w, workflow.NewFailureRunReport(req, startedAt, completedAt, 1, message)); err != nil {
		fmt.Fprintf(os.Stderr, "[ERRO] Failed to write JSON summary: %v\n", err)
	}
}

func initLogger(meta workflow.Metadata) (*logger.Logger, error) {
	enableColour := logger.IsTerminal(os.Stderr)
	return logger.New(meta.LogDir, meta.ScriptName, enableColour)
}

func printFailureCompletion(meta workflow.Metadata, rt workflow.Runtime, log *logger.Logger, startedAt time.Time) {
	workflow.NewPresenter(meta, rt, log, false).PrintCompletion(1, startedAt)
}
