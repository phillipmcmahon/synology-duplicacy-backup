package main

import (
	"errors"
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
	case req.DiagnosticsCommand != "":
		return runDiagnosticsRequest(req, meta, rt)
	case req.NotifyCommand != "":
		return runNotifyRequest(req, meta, rt)
	case req.RestoreCommand != "":
		return runRestoreRequest(req, meta, rt)
	case req.RollbackCommand != "":
		return runRollbackRequest(req, meta, rt)
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

func runDiagnosticsRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	output, err := handleDiagnosticsCommand(req, meta, rt)
	if err != nil {
		return writeCommandFailure("", err)
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

func runRestoreRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	if restoreCommandUsesProgress(req) {
		log, err := initLogger(meta)
		if err != nil {
			return writeCommandFailure("", err)
		}
		defer log.Close()
		output, err := workflow.HandleRestoreCommandWithLogger(req, meta, rt, log)
		if errors.Is(err, workflow.ErrRestoreCancelled) {
			fmt.Fprintln(os.Stderr, "[INFO] Restore cancelled by operator")
			return 0
		}
		if errors.Is(err, workflow.ErrRestoreInterrupted) {
			if output != "" {
				fmt.Print(output)
			}
			fmt.Fprintln(os.Stderr, "[WARN] Restore interrupted by operator; drill workspace was retained")
			return exitCodeGeneralFailure
		}
		if err != nil {
			return writeCommandFailure(output, err)
		}
		fmt.Print(output)
		return 0
	}

	output, err := handleRestoreCommand(req, meta, rt)
	if errors.Is(err, workflow.ErrRestoreCancelled) {
		fmt.Fprintln(os.Stderr, "[INFO] Restore cancelled by operator")
		return 0
	}
	if errors.Is(err, workflow.ErrRestoreInterrupted) {
		if output != "" {
			fmt.Print(output)
		}
		fmt.Fprintln(os.Stderr, "[WARN] Restore interrupted by operator; drill workspace was retained")
		return exitCodeGeneralFailure
	}
	if err != nil {
		return writeCommandFailure(output, err)
	}
	fmt.Print(output)
	return 0
}

func restoreCommandUsesProgress(req *workflow.Request) bool {
	if req == nil {
		return false
	}
	return req.RestoreCommand == "run" || req.RestoreCommand == "select"
}

func runRollbackRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	rollbackReq := workflow.NewRollbackRequest(req)
	if !rollbackReq.CheckOnly && rt.Geteuid() != 0 {
		return writeRollbackPrivilegeFailure()
	}
	result, err := handleRollbackCommand(&rollbackReq, meta, rt)
	if err != nil {
		return writeCommandFailure("", err)
	}
	fmt.Print(result.Output)
	return 0
}

func runUpdateRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	updateReq := workflow.NewUpdateRequest(req)
	if !updateReq.CheckOnly && rt.Geteuid() != 0 {
		return writeUpdatePrivilegeFailure()
	}
	result, err := handleUpdateCommand(&updateReq, meta, rt)
	updateStatus := updateStatusForWorkflow(result.Status)
	if err != nil {
		if notifyErr := workflow.MaybeSendUpdateFailureNotification(&updateReq, meta, rt, updateStatus, err); notifyErr != nil {
			fmt.Fprintf(os.Stderr, "[WARN] Failed to send update failure notification: %s\n", workflow.OperatorMessage(notifyErr))
		}
		return writeCommandFailure("", err)
	}
	fmt.Print(result.Output)
	if notifyErr := workflow.MaybeSendUpdateSuccessNotification(&updateReq, meta, rt, updateStatus); notifyErr != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to send update notification: %s\n", workflow.OperatorMessage(notifyErr))
	}
	return 0
}

func writeUpdatePrivilegeFailure() int {
	message := "update install must be run as root; re-run with sudo or use --check-only to inspect the update plan"
	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", message)
	return exitCodeGeneralFailure
}

func writeRollbackPrivilegeFailure() int {
	message := "rollback activation must be run as root; re-run with sudo or use --check-only to inspect rollback candidates"
	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", message)
	return exitCodeGeneralFailure
}

func runHealthRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	healthReq := workflow.NewHealthRequest(req)
	if rt.Geteuid() != 0 {
		return writeHealthPrivilegeFailure(&healthReq, rt)
	}
	log, err := initLogger(meta)
	if err != nil {
		return writeHealthLoggerFailure(&healthReq, rt, err)
	}
	log.SetVerbose(healthReq.Verbose)

	runner := execpkg.NewCommandRunner(log, false)
	report, code := workflow.NewHealthRunner(meta, rt, log, runner).Run(req)
	if req.JSONSummary {
		code = writeHealthJSONSummary(os.Stdout, report, code)
	}
	log.Close()
	return code
}

func runRuntimeRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) int {
	runtimeReq := workflow.NewRuntimeRequest(req)
	if rt.Geteuid() != 0 {
		return writeRuntimePrivilegeFailure(&runtimeReq, rt)
	}
	log, err := initLogger(meta)
	if err != nil {
		return writeRuntimeLoggerFailure(&runtimeReq, rt, err)
	}
	log.SetVerbose(runtimeReq.Verbose)
	startedAt := rt.Now()

	runner := execpkg.NewCommandRunner(log, runtimeReq.DryRun)
	planner := workflow.NewPlanner(meta, rt, log, runner)
	plan, err := planner.Build(&runtimeReq)
	if err != nil {
		return handlePlannerFailure(&runtimeReq, planner.FailureContext(&runtimeReq), meta, rt, log, startedAt, err)
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

func writeHealthLoggerFailure(req *workflow.HealthRequest, rt workflow.Runtime, err error) int {
	fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", err)
	if req.JSONSummary {
		report := workflow.NewFailureHealthReport(req, req.Command, fmt.Sprintf("Failed to initialise logger: %v", err), rt.Now())
		_ = workflow.WriteHealthReport(os.Stdout, report)
	}
	return exitCodeHealthUnhealthy
}

func writeHealthPrivilegeFailure(req *workflow.HealthRequest, rt workflow.Runtime) int {
	message := "health commands must be run as root"
	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", message)
	if req.JSONSummary {
		report := workflow.NewFailureHealthReport(req, req.Command, message, rt.Now())
		_ = workflow.WriteHealthReport(os.Stdout, report)
	}
	return exitCodeHealthUnhealthy
}

func writeRuntimeLoggerFailure(req *workflow.RuntimeRequest, rt workflow.Runtime, err error) int {
	fmt.Fprintf(os.Stderr, "[ERRO] Failed to initialise logger: %v\n", err)
	if req.JSONSummary {
		now := rt.Now()
		emitJSONFailureSummary(os.Stdout, req, nil, now, now, fmt.Sprintf("Failed to initialise logger: %v", err))
	}
	return exitCodeGeneralFailure
}

func writeRuntimePrivilegeFailure(req *workflow.RuntimeRequest, rt workflow.Runtime) int {
	message := "must be run as root"
	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", message)
	if req.JSONSummary {
		now := rt.Now()
		emitJSONFailureSummary(os.Stdout, req, nil, now, now, message)
	}
	return exitCodeGeneralFailure
}

func handlePlannerFailure(req *workflow.RuntimeRequest, failurePlan *workflow.Plan, meta workflow.Metadata, rt workflow.Runtime, log *logger.Logger, startedAt time.Time, err error) int {
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
