package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/command"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	healthpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/health"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/restore"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

const (
	exitCodeOK              = 0
	exitCodeGeneralFailure  = 1
	exitCodeHealthUnhealthy = 2
)

func dispatchRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Env) int {
	spec, ok := dispatchSpecForRequest(req)
	if !ok {
		commandName := ""
		if req != nil {
			commandName = req.Command
		}
		return writeCommandFailure("", workflow.NewRequestError("no dispatch handler registered for command %q", commandName))
	}
	if command.RequiresDSMForRequest(req) {
		if err := requireSynologyDSM(); err != nil {
			return writeCommandFailure("", err)
		}
	}
	if err := validateRootExecution(req, rt); err != nil {
		return writeCommandFailure("", err)
	}

	return spec.handle(req, meta, rt)
}

type directRootProfilePolicy struct {
	Command         string
	UsesProfile     bool
	RequiresSecrets bool
}

const directRootProfileErrorLead = "direct root execution is ambiguous for"

func validateRootExecution(req *workflow.Request, rt workflow.Env) error {
	if workflow.EnvEUID(rt) != 0 || workflow.HasSudoOperator(rt) {
		return nil
	}
	policy := directRootProfilePolicyForRequest(req)
	if !policy.UsesProfile {
		return nil
	}
	if hasExplicitDirectRootProfile(req, rt, policy) {
		return nil
	}
	return directRootProfileError(policy)
}

func directRootProfilePolicyForRequest(req *workflow.Request) directRootProfilePolicy {
	if req == nil {
		return directRootProfilePolicy{}
	}
	commandName, policy := command.ProfilePolicyForRequest(req)
	if !policy.UsesProfile {
		return directRootProfilePolicy{}
	}
	return directRootProfilePolicy{
		Command:         commandName,
		UsesProfile:     policy.UsesProfile,
		RequiresSecrets: policy.RequiresSecrets,
	}
}

func hasExplicitDirectRootProfile(req *workflow.Request, rt workflow.Env, policy directRootProfilePolicy) bool {
	if req == nil || req.ConfigDir == "" {
		return false
	}
	if policy.RequiresSecrets && req.SecretsDir == "" {
		return false
	}
	getenv := rt.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	return getenv("XDG_STATE_HOME") != ""
}

func directRootProfileError(policy directRootProfilePolicy) error {
	profileFlags := "--config-dir"
	if policy.RequiresSecrets {
		profileFlags += " and --secrets-dir"
	}
	return workflow.NewRequestError(
		"%s %s; run as the operator user, or for root-required operations run with sudo from that operator account. Expert direct-root use must pass explicit %s and set XDG_STATE_HOME so config, secrets, logs, state, and locks do not fall back to /root",
		directRootProfileErrorLead,
		policy.Command,
		profileFlags,
	)
}

func runConfigRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Env) int {
	output, err := handleConfigCommand(req, meta, rt)
	if err != nil {
		return writeCommandFailure(workflow.ConfigCommandOutput(err), err)
	}
	fmt.Print(output)
	return 0
}

func runDiagnosticsRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Env) int {
	output, err := handleDiagnosticsCommand(req, meta, rt)
	if err != nil {
		return writeCommandFailure("", err)
	}
	fmt.Print(output)
	return 0
}

func runNotifyRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Env) int {
	output, err := handleNotifyCommand(req, meta, rt)
	if err != nil {
		return writeCommandFailure(notify.CommandOutput(err), err)
	}
	fmt.Print(output)
	return 0
}

func runRestoreRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Env) int {
	restoreReq := restore.NewRestoreRequest(req)
	if restoreReq.UsesProgress() {
		log, err := initLogger(meta)
		if err != nil {
			return writeRestoreLoggerFailure(err)
		}
		defer log.Close()
		output, err := restore.HandleRestoreCommandWithLogger(req, meta, rt, log)
		if errors.Is(err, workflow.ErrRestoreCancelled) {
			writeDirectInfo("Restore cancelled by operator")
			return 0
		}
		if errors.Is(err, workflow.ErrRestoreInterrupted) {
			if output != "" {
				fmt.Print(output)
			}
			writeDirectWarn("Restore interrupted by operator; drill workspace was retained")
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
		writeDirectInfo("Restore cancelled by operator")
		return 0
	}
	if errors.Is(err, workflow.ErrRestoreInterrupted) {
		if output != "" {
			fmt.Print(output)
		}
		writeDirectWarn("Restore interrupted by operator; drill workspace was retained")
		return exitCodeGeneralFailure
	}
	if err != nil {
		return writeCommandFailure(output, err)
	}
	fmt.Print(output)
	return 0
}

func runRollbackRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Env) int {
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

func runUpdateRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Env) int {
	updateReq := workflow.NewUpdateRequest(req)
	if !updateReq.CheckOnly && rt.Geteuid() != 0 {
		return writeUpdatePrivilegeFailure()
	}
	result, err := handleUpdateCommand(&updateReq, meta, rt)
	updateStatus := updateStatusForWorkflow(result.Status)
	if err != nil {
		if notifyErr := workflow.MaybeSendUpdateFailureNotification(&updateReq, meta, rt, updateStatus, err); notifyErr != nil {
			writeDirectWarn("Failed to send update failure notification: %s", workflow.OperatorMessage(notifyErr))
		}
		return writeCommandFailure("", err)
	}
	fmt.Print(result.Output)
	if notifyErr := workflow.MaybeSendUpdateSuccessNotification(&updateReq, meta, rt, updateStatus); notifyErr != nil {
		writeDirectWarn("Failed to send update notification: %s", workflow.OperatorMessage(notifyErr))
	}
	return 0
}

func writeUpdatePrivilegeFailure() int {
	message := "update install must be run as root; re-run with sudo or use --check-only to inspect the update plan"
	writeDirectError("%s", message)
	return exitCodeGeneralFailure
}

func writeRollbackPrivilegeFailure() int {
	message := "rollback activation must be run as root; re-run with sudo or use --check-only to inspect rollback candidates"
	writeDirectError("%s", message)
	return exitCodeGeneralFailure
}

func runHealthRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Env) int {
	healthReq := healthpkg.NewHealthRequest(req)
	log, err := initLogger(meta)
	if err != nil {
		return writeHealthLoggerFailure(&healthReq, rt, err)
	}
	log.SetVerbose(healthReq.Verbose)

	runner := execpkg.NewCommandRunner(log, false)
	report, code := healthpkg.NewHealthRunner(meta, rt, log, runner).Run(req)
	if req.JSONSummary {
		code = writeHealthJSONSummary(os.Stdout, report, code)
	}
	log.Close()
	return code
}

func runRuntimeRequest(req *workflow.Request, meta workflow.Metadata, rt workflow.Env) int {
	runtimeReq := workflow.NewRuntimeRequest(req)
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
	if plan.Request.JSONSummary {
		code = writeRuntimeJSONSummary(os.Stdout, executor.Report(), code)
	}
	return code
}

func writeCommandFailure(report string, err error) int {
	if report != "" {
		fmt.Print(report)
	}
	writeDirectError("%s", workflow.OperatorMessage(err))
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

func writeHealthJSONSummary(w io.Writer, report *healthpkg.Report, code int) int {
	if err := healthpkg.WriteHealthReport(w, report); err != nil {
		writeJSONSummaryFailure(err)
		if code == exitCodeOK {
			return exitCodeHealthUnhealthy
		}
	}
	return code
}

func writeJSONSummaryFailure(err error) {
	writeDirectError("Failed to write JSON summary: %v", err)
}

func writeHealthLoggerFailure(req *healthpkg.HealthRequest, rt workflow.Env, err error) int {
	writeDirectError("Failed to initialise logger: %v", err)
	if req.JSONSummary {
		report := healthpkg.NewFailureHealthReport(req, req.Command, fmt.Sprintf("Failed to initialise logger: %v", err), rt.Now())
		_ = healthpkg.WriteHealthReport(os.Stdout, report)
	}
	return exitCodeHealthUnhealthy
}

func writeRuntimeLoggerFailure(req *workflow.RuntimeRequest, rt workflow.Env, err error) int {
	writeDirectError("Failed to initialise logger: %v", err)
	if req.JSONSummary {
		now := rt.Now()
		emitJSONFailureSummary(os.Stdout, req, nil, now, now, fmt.Sprintf("Failed to initialise logger: %v", err))
	}
	return exitCodeGeneralFailure
}

func writeRestoreLoggerFailure(err error) int {
	return writeCommandFailure("", fmt.Errorf("failed to initialise restore progress logger: %w", err))
}

func handlePlannerFailure(req *workflow.RuntimeRequest, failurePlan *workflow.Plan, meta workflow.Metadata, rt workflow.Env, log *logger.Logger, startedAt time.Time, err error) int {
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
