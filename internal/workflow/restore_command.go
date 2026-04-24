package workflow

import (
	"bufio"
	"os"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

type restoreSelectIntent string

const (
	restoreSelectIntentInspect   restoreSelectIntent = "inspect"
	restoreSelectIntentFull      restoreSelectIntent = "full"
	restoreSelectIntentSelective restoreSelectIntent = "selective"
)

func HandleRestoreCommand(req *Request, meta Metadata, rt Runtime) (string, error) {
	return handleRestoreCommand(req, meta, rt, defaultRestoreDeps())
}

func handleRestoreCommand(req *Request, meta Metadata, rt Runtime, deps RestoreDeps) (string, error) {
	deps = deps.withDefaults()
	switch req.RestoreCommand {
	case "plan":
		return handleRestorePlan(req, meta, rt, deps)
	case "list-revisions":
		return handleRestoreRevisions(req, meta, rt, deps)
	case "run":
		return handleRestoreRun(req, meta, rt, deps)
	case "select":
		return handleRestoreSelect(req, meta, rt, deps)
	default:
		return "", NewRequestError("unsupported restore command %q", req.RestoreCommand)
	}
}

func handleRestorePlan(req *Request, meta Metadata, rt Runtime, deps RestoreDeps) (string, error) {
	planner := NewConfigPlanner(meta, rt)
	planReq := configValidationRequest(req, req.Target())
	plan := planner.derivePlan(planReq)
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.applyConfig(cfg, rt)

	report := newRestorePlanReport(req, meta, plan, cfg.Storage, deps)
	report.loadAndApplyState(meta, req.Source, req.Target())
	return formatRestorePlan(report), nil
}

func handleRestoreRevisions(req *Request, meta Metadata, rt Runtime, deps RestoreDeps) (string, error) {
	ctx, err := newRestoreExecutionContext(req, meta, rt, true, deps)
	if err != nil {
		return "", err
	}
	defer ctx.cleanup()

	revisions, _, err := ctx.dup.ListVisibleRevisions()
	if err != nil {
		return "", err
	}
	report := newRestoreRevisionsReport(req, ctx, revisions)
	if req.JSONSummary {
		return marshalRestoreJSON(report)
	}
	return formatRestoreRevisions(report), nil
}

func handleRestoreRun(req *Request, meta Metadata, rt Runtime, deps RestoreDeps) (string, error) {
	restorePath, err := cleanRestorePath(req.RestorePath)
	if err != nil {
		return "", err
	}
	ctx, err := newRestoreRunContext(req, meta, rt, deps)
	if err != nil {
		return "", err
	}

	report := newRestoreRunReport(req, ctx.plan, ctx.storage, ctx.workspace, restorePath)
	if req.DryRun {
		report.Result = "Dry run"
		report.Output = "workspace preparation and restore command were not executed"
		return formatRestoreRun(report), nil
	}
	if !req.RestoreYes {
		confirmed, err := confirmRestoreRun(rt, report)
		if err != nil {
			return "", err
		}
		if !confirmed {
			return "", NewRequestError("restore cancelled")
		}
	}

	if err := prepareRestoreWorkspace(ctx.workspace, ctx.storage, ctx.secrets); err != nil {
		return "", err
	}
	report.WorkspacePrepared = true

	dup := duplicacy.NewWorkspaceSetup(ctx.workspace, ctx.storage, false, deps.NewRunner())
	output, err := dup.RestoreRevision(req.RestoreRevision, restorePath)
	report.Output = strings.TrimSpace(output)
	if err != nil {
		report.Result = "Failed"
		return formatRestoreRun(report), err
	}
	report.Result = "Restored into workspace"
	return formatRestoreRun(report), nil
}

func handleRestoreSelect(req *Request, meta Metadata, rt Runtime, deps RestoreDeps) (string, error) {
	if rt.StdinIsTTY == nil || !rt.StdinIsTTY() {
		return "", NewRequestError("restore select requires an interactive terminal; use restore list-revisions and restore run for scripts or scheduled jobs")
	}
	stdin := os.Stdin
	if rt.Stdin != nil && rt.Stdin() != nil {
		stdin = rt.Stdin()
	}
	reader := bufio.NewReader(stdin)

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
		return "", NewRequestError("restore select found no visible revisions; run restore list-revisions --target %s %s to inspect the target directly", req.Target(), req.Source)
	}
	revision, err := promptRestoreRevision(reader, revisions, 50, deps)
	if err != nil {
		return "", err
	}
	intent, err := promptRestoreSelectIntent(reader, req.RestorePathPrefix, deps)
	if err != nil {
		return "", err
	}

	if intent == restoreSelectIntentInspect {
		if err := promptRestoreInspect(ctx, req, meta, revision.Revision, deps); err != nil {
			return "", err
		}
		return formatRestoreInspect(newRestoreInspectReport(req, ctx.plan, ctx.cfg.Storage, revision.Revision, req.RestorePathPrefix)), nil
	}

	restorePaths := []string{""}
	commandWorkspace := resolvedRestoreSelectWorkspace(req, ctx.plan, revision, deps)
	workspacePrepared := restoreWorkspacePrepared(commandWorkspace)
	if err := validateRestoreWorkspace(commandWorkspace, ctx.plan.SnapshotSource); err != nil {
		return "", err
	}
	if intent == restoreSelectIntentFull {
		pathPrefix, err := cleanRestorePath(req.RestorePathPrefix)
		if err != nil {
			return "", err
		}
		if pathPrefix != "" {
			restorePaths = []string{restoreScopedDirectoryPattern(pathPrefix)}
		}
	}
	if intent == restoreSelectIntentSelective {
		restorePaths, err = promptRestorePath(ctx, req, meta, revision.Revision, deps)
		if err != nil {
			return "", err
		}
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
	outputs := []string{selectOutput}
	for _, restorePath := range restorePaths {
		runReq := *req
		runReq.RestoreCommand = "run"
		runReq.RestoreRevision = revision.Revision
		runReq.RestorePath = restorePath
		runReq.RestoreWorkspace = commandWorkspace
		runReq.RestoreYes = true
		runOutput, err := handleRestoreRun(&runReq, meta, rt, deps)
		outputs = append(outputs, runOutput)
		if err != nil {
			return strings.Join(outputs, "\n"), err
		}
	}
	return strings.Join(outputs, "\n"), nil
}
