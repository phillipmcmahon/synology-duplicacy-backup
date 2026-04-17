package main

import (
	"fmt"
	"io"
	"os"
	"time"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

const (
	exitCodeOK              = 0
	exitCodeGeneralFailure  = 1
	exitCodeHealthUnhealthy = 2
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
	result, err := handleUpdateCommand(req, meta, rt)
	updateStatus := updateStatusForWorkflow(result.Status)
	if err != nil {
		if notifyErr := workflow.MaybeSendUpdateFailureNotification(req, meta, rt, updateStatus, err); notifyErr != nil {
			fmt.Fprintf(os.Stderr, "[WARN] Failed to send update failure notification: %s\n", workflow.OperatorMessage(notifyErr))
		}
		return writeCommandFailure("", err)
	}
	fmt.Print(result.Output)
	if notifyErr := workflow.MaybeSendUpdateSuccessNotification(req, meta, rt, updateStatus); notifyErr != nil {
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
		code = writeHealthJSONSummary(os.Stdout, report, code)
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
		code = writeRuntimeJSONSummary(os.Stdout, executor.Report(), code)
	}
	return code
}

func writeCommandFailure(report string, err error) int {
	if report != "" {
		fmt.Print(report)
	}
	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", workflow.OperatorMessage(err))
	return exitCodeGeneralFailure
}

func writeRuntimeJSONSummary(w io.Writer, report *workflow.RunReport, code int) int {
	if err := workflow.WriteRunReport(w, report); err != nil {
		writeJSONSummaryFailure(err)
		if code == exitCodeOK {
			return exitCodeGeneralFailure
		}
	}
	return code
}

func writeHealthJSONSummary(w io.Writer, report *workflow.HealthReport, code int) int {
	if err := workflow.WriteHealthReport(w, report); err != nil {
		writeJSONSummaryFailure(err)
		if code == exitCodeOK {
			return exitCodeHealthUnhealthy
		}
	}
	return code
}

func writeJSONSummaryFailure(err error) {
	fmt.Fprintf(os.Stderr, "[ERRO] Failed to write JSON summary: %v\n", err)
}

func writeHealthLoggerFailure(req *workflow.Request, rt workflow.Runtime, err error) int {
	fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", err)
	if req.JSONSummary {
		report := workflow.NewFailureHealthReport(req, req.HealthCommand, fmt.Sprintf("Failed to initialise logger: %v", err), rt.Now())
		_ = workflow.WriteHealthReport(os.Stdout, report)
	}
	return exitCodeHealthUnhealthy
}

func writeHealthPrivilegeFailure(req *workflow.Request, rt workflow.Runtime) int {
	message := "health commands must be run as root"
	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", message)
	if req.JSONSummary {
		report := workflow.NewFailureHealthReport(req, req.HealthCommand, message, rt.Now())
		_ = workflow.WriteHealthReport(os.Stdout, report)
	}
	return exitCodeHealthUnhealthy
}

func writeRuntimeLoggerFailure(req *workflow.Request, rt workflow.Runtime, err error) int {
	fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", err)
	if req.JSONSummary {
		now := rt.Now()
		emitJSONFailureSummary(os.Stdout, req, nil, now, now, fmt.Sprintf("Failed to initialise logger: %v", err))
	}
	return exitCodeGeneralFailure
}

func writeRuntimePrivilegeFailure(req *workflow.Request, rt workflow.Runtime) int {
	message := "must be run as root"
	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", message)
	if req.JSONSummary {
		now := rt.Now()
		emitJSONFailureSummary(os.Stdout, req, nil, now, now, message)
	}
	return exitCodeGeneralFailure
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
	return exitCodeGeneralFailure
}
