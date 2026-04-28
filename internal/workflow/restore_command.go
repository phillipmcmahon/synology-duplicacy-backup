package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

// ErrRestoreCancelled means the operator deliberately exited before restore
// execution, for example by typing q or answering no. It is a clean exit and
// dispatch maps it to exit code 0.
var ErrRestoreCancelled = errors.New("restore cancelled")

// ErrRestoreInterrupted means an active restore process was interrupted after
// execution started. Dispatch maps it to exit code 1 because the drill
// workspace may contain a partial restore that needs inspection.
var ErrRestoreInterrupted = errors.New("restore interrupted")

type restoreSelectIntent string

const (
	restoreSelectIntentInspect   restoreSelectIntent = "inspect"
	restoreSelectIntentFull      restoreSelectIntent = "full"
	restoreSelectIntentSelective restoreSelectIntent = "selective"
)

type restoreRunInputs struct {
	Revision                int
	RestorePath             string
	Workspace               string
	AssumeYes               bool
	DryRun                  bool
	SuppressPrepareStatus   bool
	SuppressRestoreActivity bool
	DeferOwnershipRepair    bool
	Context                 context.Context
}

func HandleRestoreCommand(req *Request, meta Metadata, rt Runtime) (string, error) {
	restoreReq := NewRestoreRequest(req)
	return handleRestoreCommand(&restoreReq, meta, rt, defaultRestoreDeps())
}

func HandleRestoreCommandWithLogger(req *Request, meta Metadata, rt Runtime, log *logger.Logger) (string, error) {
	restoreReq := NewRestoreRequest(req)
	deps := defaultRestoreDeps()
	deps.Progress = NewRestoreProgress(meta, rt, log)
	return handleRestoreCommand(&restoreReq, meta, rt, deps)
}

func handleRestoreCommand(req *RestoreRequest, meta Metadata, rt Runtime, deps RestoreDeps) (string, error) {
	deps = deps.withDefaults()
	if err := validateRestoreWorkspaceSelection(req); err != nil {
		return "", err
	}
	switch req.Command {
	case "plan":
		return handleRestorePlan(req, meta, rt, deps)
	case "list-revisions":
		return handleRestoreRevisions(req, meta, rt, deps)
	case "run":
		return handleRestoreRun(req, meta, rt, deps)
	case "select":
		return handleRestoreSelect(req, meta, rt, deps)
	default:
		return "", NewRequestError("unsupported restore command %q", req.Command)
	}
}

func handleRestorePlan(req *RestoreRequest, meta Metadata, rt Runtime, deps RestoreDeps) (string, error) {
	planner := NewConfigPlanner(meta, rt)
	plan := planner.derivePlan(req.PlanRequest())
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.applyConfig(cfg, rt)

	report := newRestorePlanReport(req, meta, plan, cfg.Storage, deps)
	report.loadAndApplyState(meta, req.Label, req.Target())
	return formatRestorePlan(report), nil
}

func handleRestoreRevisions(req *RestoreRequest, meta Metadata, rt Runtime, deps RestoreDeps) (string, error) {
	ctx, err := newRestoreExecutionContext(req, meta, rt, true, deps)
	if err != nil {
		return "", err
	}
	defer ctx.cleanup()

	stopActivity := deps.Progress.StartActivity("Loading restore points")
	revisions, _, err := ctx.dup.ListVisibleRevisions()
	stopActivity()
	if err != nil {
		return "", err
	}
	report := newRestoreRevisionsReport(req, ctx, revisions)
	if req.JSONSummary {
		return marshalRestoreJSON(report)
	}
	return formatRestoreRevisions(report), nil
}

func handleRestoreRun(req *RestoreRequest, meta Metadata, rt Runtime, deps RestoreDeps) (string, error) {
	restorePath, err := cleanRestorePath(req.Path)
	if err != nil {
		return "", err
	}
	ctx, err := newRestoreRunContext(req, meta, rt, deps)
	if err != nil {
		return "", err
	}
	return executeRestoreRun(req, rt, deps, ctx, restoreRunInputs{
		Revision:    req.Revision,
		RestorePath: restorePath,
		Workspace:   ctx.workspace,
		AssumeYes:   req.Yes,
		DryRun:      req.DryRun,
	})
}

func executeRestoreRun(req *RestoreRequest, rt Runtime, deps RestoreDeps, ctx *restoreRunContext, inputs restoreRunInputs) (string, error) {
	report := newRestoreRunReport(req, ctx.plan, ctx.storage, inputs.Workspace, inputs.Revision, inputs.RestorePath, inputs.DryRun)
	if inputs.DryRun {
		report.Result = "Dry Run"
		report.Output = "workspace preparation and restore command were not executed"
		return formatRestoreRun(report), nil
	}
	if !inputs.AssumeYes {
		confirmed, err := confirmRestoreRun(rt, report)
		if err != nil {
			return "", err
		}
		if !confirmed {
			return "", NewRequestError("restore cancelled")
		}
	}

	runCtx, cleanup, interrupt := newRestoreInterruptContext(rt, deps.Progress, inputs.Workspace)
	defer cleanup()
	interrupt.setCurrent(1, 1, inputs.RestorePath)
	inputs.Context = runCtx
	output, err := executeRestoreRunConfirmed(req, rt, deps, ctx, inputs, true)
	if err == nil {
		interrupt.setCompleted(1, 1)
	}
	return output, err
}

func executeRestoreRunConfirmed(req *RestoreRequest, rt Runtime, deps RestoreDeps, ctx *restoreRunContext, inputs restoreRunInputs, printRunProgress bool) (string, error) {
	report := newRestoreRunReport(req, ctx.plan, ctx.storage, inputs.Workspace, inputs.Revision, inputs.RestorePath, inputs.DryRun)
	startedAt := rt.Now()
	if printRunProgress {
		deps.Progress.PrintRunStart(req, ctx.plan, inputs, startedAt)
	}
	success := false
	defer func() {
		if printRunProgress {
			deps.Progress.PrintRunCompletion(success, startedAt)
		}
	}()

	if !inputs.SuppressPrepareStatus {
		deps.Progress.PrintStatus("Preparing drill workspace")
	}
	if err := prepareRestoreWorkspace(inputs.Workspace, ctx.storage, ctx.secrets); err != nil {
		return "", err
	}
	report.WorkspacePrepared = true
	if !inputs.DeferOwnershipRepair {
		if err := restoreWorkspaceProfileOwnership(ctx.meta, inputs.Workspace); err != nil {
			return formatRestoreRun(report), err
		}
	}

	dup := duplicacy.NewWorkspaceSetup(inputs.Workspace, ctx.storage, false, deps.NewRunner())
	stopActivity := func() {}
	if !inputs.SuppressRestoreActivity {
		stopActivity = deps.Progress.StartActivity(restoreProgressActivity(inputs))
	}
	runContext := inputs.Context
	if runContext == nil {
		runContext = context.Background()
	}
	output, err := dup.RestoreRevisionContext(runContext, inputs.Revision, inputs.RestorePath)
	stopActivity()
	if err != nil {
		ownershipErr := error(nil)
		if !inputs.DeferOwnershipRepair {
			ownershipErr = restoreWorkspaceProfileOwnership(ctx.meta, inputs.Workspace)
		}
		report.Result = "Failed"
		if errors.Is(err, context.Canceled) {
			report.Result = "Interrupted"
			err = ErrRestoreInterrupted
		}
		report.Output = restoreOutputForReport(output, false)
		if ownershipErr != nil {
			err = fmt.Errorf("%w; additionally failed to set restore workspace ownership: %v", err, ownershipErr)
		}
		return formatRestoreRun(report), err
	}
	if !inputs.DeferOwnershipRepair {
		if err := restoreWorkspaceProfileOwnership(ctx.meta, inputs.Workspace); err != nil {
			report.Result = "Ownership repair failed"
			report.Output = restoreOutputForReport(output, true)
			return formatRestoreRun(report), err
		}
	}
	report.Result = "Restored into workspace"
	report.Output = restoreOutputForReport(output, true)
	success = true
	return formatRestoreRun(report), nil
}

func handleRestoreSelect(req *RestoreRequest, meta Metadata, rt Runtime, deps RestoreDeps) (string, error) {
	reader, interactive := runtimeStdinReader(rt)
	if !interactive {
		return "", NewRequestError("restore select requires an interactive terminal; use restore list-revisions and restore run for scripts or scheduled jobs")
	}
	ctx, cleanup, err := newRestoreSelectContext(req, meta, rt, deps)
	if err != nil {
		return "", err
	}
	defer cleanup()

	revisions, _, err := ctx.dup.ListVisibleRevisions()
	if err != nil {
		return "", err
	}
	if len(revisions) == 0 {
		return "", NewRequestError("restore select found no visible revisions; run restore list-revisions --target %s %s to inspect the target directly", req.Target(), req.Label)
	}
	revision, err := promptRestoreRevision(reader, revisions, 50, deps)
	if err != nil {
		return "", err
	}
	intent, err := promptRestoreSelectIntent(reader, req.PathPrefix, deps)
	if err != nil {
		return "", err
	}

	switch intent {
	case restoreSelectIntentInspect:
		return runRestoreSelectInspect(ctx, req, meta, revision, deps)
	case restoreSelectIntentFull:
		return runRestoreSelectFull(ctx, req, meta, rt, reader, revision, deps)
	case restoreSelectIntentSelective:
		return runRestoreSelectSelective(ctx, req, meta, rt, reader, revision, deps)
	default:
		return "", NewRequestError("unsupported restore select intent %q", intent)
	}
}

func runRestoreSelectInspect(ctx *restoreExecutionContext, req *RestoreRequest, meta Metadata, revision duplicacy.RevisionInfo, deps RestoreDeps) (string, error) {
	if err := promptRestoreInspect(ctx, req, meta, revision.Revision, deps); err != nil {
		return "", err
	}
	return formatRestoreInspect(newRestoreInspectReport(req, ctx.plan, ctx.cfg.Storage, revision.Revision, req.PathPrefix)), nil
}

func runRestoreSelectFull(ctx *restoreExecutionContext, req *RestoreRequest, meta Metadata, rt Runtime, reader restoreLineReader, revision duplicacy.RevisionInfo, deps RestoreDeps) (string, error) {
	restorePaths := []string{""}
	pathPrefix, err := cleanRestorePath(req.PathPrefix)
	if err != nil {
		return "", err
	}
	if pathPrefix != "" {
		restorePaths = []string{restoreScopedDirectoryPattern(pathPrefix)}
	}
	return runRestoreSelectExecution(ctx, req, meta, rt, reader, revision, restorePaths, deps)
}

func runRestoreSelectSelective(ctx *restoreExecutionContext, req *RestoreRequest, meta Metadata, rt Runtime, reader restoreLineReader, revision duplicacy.RevisionInfo, deps RestoreDeps) (string, error) {
	restorePaths, err := promptRestorePath(ctx, req, meta, revision.Revision, deps)
	if err != nil {
		return "", err
	}
	return runRestoreSelectExecution(ctx, req, meta, rt, reader, revision, restorePaths, deps)
}

func runRestoreSelectExecution(ctx *restoreExecutionContext, req *RestoreRequest, meta Metadata, rt Runtime, reader restoreLineReader, revision duplicacy.RevisionInfo, restorePaths []string, deps RestoreDeps) (string, error) {
	if err := validateRestoreWorkspaceRoot(req); err != nil {
		return "", err
	}
	commandWorkspace := resolvedRestoreSelectWorkspace(req, ctx.plan, revision, deps)
	workspacePrepared := restoreWorkspacePrepared(commandWorkspace)
	if err := validateRestoreWorkspace(commandWorkspace, ctx.plan.SnapshotSource); err != nil {
		return "", err
	}
	report := newRestoreSelectReport(req, meta, ctx.plan, ctx.cfg.Storage, commandWorkspace, workspacePrepared, revision.Revision, restorePaths)
	selectOutput := formatRestoreSelect(report)
	confirmed, err := confirmRestoreSelectExecution(reader, report, deps)
	if err != nil {
		return "", err
	}
	if !confirmed {
		return selectOutput, nil
	}
	batchReport := &restoreBatchRunReport{
		Label:        req.Label,
		Target:       req.Target(),
		Location:     ctx.plan.Location,
		Workspace:    commandWorkspace,
		Revision:     revision.Revision,
		RestorePaths: normaliseRestoreSelection(restorePaths),
		Guide:        "docs/restore-drills.md",
	}
	runCtx := &restoreRunContext{
		plan:      ctx.plan,
		storage:   ctx.cfg.Storage,
		secrets:   ctx.secrets,
		workspace: commandWorkspace,
		meta:      meta,
	}
	startedAt := rt.Now()
	execCtx, cleanup, interrupt := newRestoreInterruptContext(rt, deps.Progress, commandWorkspace)
	defer cleanup()
	deps.Progress.PrintSelectionStart(req, ctx.plan, revision.Revision, commandWorkspace, len(restorePaths), startedAt)
	success := false
	defer func() {
		deps.Progress.PrintRunCompletion(success, startedAt)
	}()
	for i, restorePath := range restorePaths {
		interrupt.setCurrent(i+1, len(restorePaths), restorePath)
		stopActivity := func() {}
		suppressPerPathProgress := len(restorePaths) > 1
		if suppressPerPathProgress {
			stopActivity = deps.Progress.StartSelectionActivity(i+1, len(restorePaths), restorePath)
		}
		runOutput, err := executeRestoreRunConfirmed(req, rt, deps, runCtx, restoreRunInputs{
			Revision:                revision.Revision,
			RestorePath:             restorePath,
			Workspace:               commandWorkspace,
			AssumeYes:               true,
			SuppressPrepareStatus:   suppressPerPathProgress,
			SuppressRestoreActivity: suppressPerPathProgress,
			DeferOwnershipRepair:    true,
			Context:                 execCtx,
		}, false)
		stopActivity()
		pathResult := restoreBatchPathResult{
			Path:   restorePath,
			Result: restoreResultFromOutput(runOutput),
		}
		if err != nil {
			if errors.Is(err, ErrRestoreInterrupted) || errors.Is(err, context.Canceled) {
				pathResult.Result = "Interrupted"
				err = ErrRestoreInterrupted
			}
			if ownershipErr := restoreWorkspaceProfileOwnership(runCtx.meta, commandWorkspace); ownershipErr != nil {
				err = fmt.Errorf("%w; additionally failed to set restore workspace ownership: %v", err, ownershipErr)
			}
			pathResult.Output = restoreDuplicacyOutputFromRestoreRun(runOutput)
			batchReport.Results = append(batchReport.Results, pathResult)
			return formatRestoreBatchRun(batchReport), err
		}
		batchReport.Results = append(batchReport.Results, pathResult)
		interrupt.setCompleted(i+1, len(restorePaths))
	}
	if err := restoreWorkspaceProfileOwnership(runCtx.meta, commandWorkspace); err != nil {
		return formatRestoreBatchRun(batchReport), err
	}
	success = true
	return formatRestoreBatchRun(batchReport), nil
}

func restoreDuplicacyOutputFromRestoreRun(output string) string {
	const section = "Section: Duplicacy Summary"
	index := strings.Index(output, section)
	if index == -1 {
		return ""
	}
	lines := strings.Split(output[index+len(section):], "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Section: ") {
			break
		}
		if strings.HasPrefix(line, "Output") {
			_, value, _ := strings.Cut(line, ":")
			values = append(values, strings.TrimSpace(value))
			continue
		}
		values = append(values, line)
	}
	return strings.Join(values, "\n")
}

func restoreResultFromOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Result") && strings.Contains(line, ":") {
			_, value, _ := strings.Cut(line, ":")
			return strings.TrimSpace(value)
		}
	}
	return "Completed"
}
