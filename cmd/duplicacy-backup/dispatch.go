package main

import (
	"fmt"
	"os"
	"time"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func dispatchRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	switch {
	case req.ConfigCommand != "":
		return runConfigRequest(req, meta, rt)
	case req.NotifyCommand != "":
		return runNotifyRequest(req, meta, rt)
	case req.UpdateCommand != "":
		return runUpdateRequest(req, meta, rt)
	case req.HealthCommand != "":
		return runHealthRequest(req, meta, rt)
	default:
		return runRuntimeRequest(req, meta, rt)
	}
}

func runConfigRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	output, err := handleConfigCommand(req, meta, rt)
	if err != nil {
		return writeCommandFailure(workflow.ConfigCommandOutput(err), err)
	}
	fmt.Print(output)
	return 0
}

func runNotifyRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	output, err := handleNotifyCommand(req, meta, rt)
	if err != nil {
		return writeCommandFailure(notify.CommandOutput(err), err)
	}
	fmt.Print(output)
	return 0
}

func runUpdateRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	output, err := handleUpdateCommand(req, meta, rt)
	if err != nil {
		if notifyErr := workflow.MaybeSendUpdateFailureNotification(req, meta, rt, err); notifyErr != nil {
			fmt.Fprintf(os.Stderr, "[WARN] Failed to send update failure notification: %s\n", workflow.OperatorMessage(notifyErr))
		}
		return writeCommandFailure("", err)
	}
	fmt.Print(output)
	if notifyErr := workflow.MaybeSendUpdateSuccessNotification(req, meta, rt, output); notifyErr != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to send update notification: %s\n", workflow.OperatorMessage(notifyErr))
	}
	return 0
}

func runHealthRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	if rt.Geteuid() != 0 {
		return writeHealthPrivilegeFailure(req, rt)
	}
	log, err := initLogger(meta)
	if err != nil {
		return writeHealthLoggerFailure(req, rt, err)
	}
	log.SetVerbose(req.Verbose)

	runner := execpkg.NewCommandRunner(log, false)
	report, code := workflow.NewHealthRunner(meta, rt, log, runner).Run(req)
	if req.JSONSummary {
		if err := workflow.WriteHealthReport(os.Stdout, report); err != nil {
			fmt.Fprintf(os.Stderr, "[ERRO] Failed to write JSON summary: %v\n", err)
			if code == 0 {
				code = 2
			}
		}
	}
	log.Close()
	return code
}

func runRuntimeRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	if rt.Geteuid() != 0 {
		return writeRuntimePrivilegeFailure(req, rt)
	}
	log, err := initLogger(meta)
	if err != nil {
		return writeRuntimeLoggerFailure(req, rt, err)
	}
	log.SetVerbose(req.Verbose)
	startedAt := rt.Now()

	runner := execpkg.NewCommandRunner(log, req.DryRun)
	planner := workflow.NewPlanner(meta, rt, log, runner)
	plan, err := planner.Build(req)
	if err != nil {
		return handlePlannerFailure(req, planner.FailureContext(req), meta, rt, log, startedAt, err)
	}

	executor := workflow.NewExecutor(meta, rt, log, runner, plan)
	code := executor.Run()
	if plan.JSONSummary {
		if err := workflow.WriteRunReport(os.Stdout, executor.Report()); err != nil {
			fmt.Fprintf(os.Stderr, "[ERRO] Failed to write JSON summary: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	return code
}

func writeCommandFailure(report string, err error) int {
	if report != "" {
		fmt.Print(report)
	}
	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", workflow.OperatorMessage(err))
	return 1
}

func writeHealthLoggerFailure(req *workflow.Request, rt workflow.Runtime, err error) int {
	fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", err)
	if req.JSONSummary {
		report := workflow.NewFailureHealthReport(req, req.HealthCommand, fmt.Sprintf("Failed to initialise logger: %v", err), rt.Now())
		_ = workflow.WriteHealthReport(os.Stdout, report)
	}
	return 2
}

func writeHealthPrivilegeFailure(req *workflow.Request, rt workflow.Runtime) int {
	message := "Health commands must be run as root"
	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", message)
	if req.JSONSummary {
		report := workflow.NewFailureHealthReport(req, req.HealthCommand, message, rt.Now())
		_ = workflow.WriteHealthReport(os.Stdout, report)
	}
	return 2
}

func writeRuntimeLoggerFailure(req *workflow.Request, rt workflow.Runtime, err error) int {
	fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", err)
	if req.JSONSummary {
		now := rt.Now()
		emitJSONFailureSummary(os.Stdout, req, nil, now, now, fmt.Sprintf("Failed to initialise logger: %v", err))
	}
	return 1
}

func writeRuntimePrivilegeFailure(req *workflow.Request, rt workflow.Runtime) int {
	message := "Must be run as root"
	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", message)
	if req.JSONSummary {
		now := rt.Now()
		emitJSONFailureSummary(os.Stdout, req, nil, now, now, message)
	}
	return 1
}

func handlePlannerFailure(req *workflow.Request, failurePlan *workflow.Plan, meta workflow.Metadata, rt workflow.Runtime, log *logger.Logger, startedAt time.Time, err error) int {
	presenter := workflow.NewPresenter(meta, rt, log, false)
	if failurePlan != nil {
		presenter.PrintPreRunFailurePlan(failurePlan)
	} else {
		presenter.PrintPreRunFailureContext(req)
	}
	log.Error("%s", workflow.OperatorMessage(err))
	if failurePlan != nil {
		if notifyErr := maybeSendPreRunFailureNotification(rt, log.Interactive(), failurePlan, req, startedAt, rt.Now(), err); notifyErr != nil {
			log.Warn("%s", workflow.OperatorMessage(notifyErr))
		}
	}
	printFailureCompletion(meta, rt, log, startedAt)
	if req.JSONSummary {
		emitJSONFailureSummary(os.Stdout, req, failurePlan, startedAt, rt.Now(), workflow.OperatorMessage(err))
	}
	log.Close()
	return 1
}
