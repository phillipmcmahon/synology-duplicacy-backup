package workflow

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/restorepicker"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

var newRestoreCommandRunner = func() execpkg.Runner {
	runner := execpkg.NewCommandRunner(nil, false)
	runner.SetDebugCommands(false)
	return runner
}

var restorePromptOutput io.Writer = os.Stdout

var restoreWorkspaceNow = func() time.Time {
	return time.Now()
}

var runRestoreSelectPicker = func(paths []string, opts restorepicker.AppOptions) ([]string, error) {
	filteredPaths, err := restorepicker.FilterPaths(paths, opts.PathPrefix)
	if err != nil {
		return nil, err
	}
	root := restorepicker.BuildTree(filteredPaths)
	return restorepicker.RunPicker(root, opts)
}

func HandleRestoreCommand(req *Request, meta Metadata, rt Runtime) (string, error) {
	switch req.RestoreCommand {
	case "plan":
		return handleRestorePlan(req, meta, rt)
	case "prepare":
		return handleRestorePrepare(req, meta, rt)
	case "revisions":
		return handleRestoreRevisions(req, meta, rt)
	case "files":
		return handleRestoreFiles(req, meta, rt)
	case "run":
		return handleRestoreRun(req, meta, rt)
	case "select":
		return handleRestoreSelect(req, meta, rt)
	default:
		return "", NewRequestError("unsupported restore command %q", req.RestoreCommand)
	}
}

func handleRestorePlan(req *Request, meta Metadata, rt Runtime) (string, error) {
	planner := NewConfigPlanner(meta, rt)
	planReq := configValidationRequest(req, req.Target())
	plan := planner.derivePlan(planReq)
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.applyConfig(cfg, rt)

	report := newRestorePlanReport(req, meta, plan, cfg.Storage)
	report.loadAndApplyState(meta, req.Source, req.Target())
	return formatRestorePlan(report), nil
}

func handleRestorePrepare(req *Request, meta Metadata, rt Runtime) (string, error) {
	planner := NewConfigPlanner(meta, rt)
	planReq := configValidationRequest(req, req.Target())
	plan := planner.derivePlan(planReq)
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.applyConfig(cfg, rt)

	storageSpec := duplicacy.NewStorageSpec(cfg.Storage)
	var sec *secrets.Secrets
	if storageSpec.NeedsSecrets() {
		sec, err = planner.loadSecrets(plan)
		if err != nil {
			return "", err
		}
		if err := storageSpec.ValidateSecrets(sec); err != nil {
			return "", err
		}
	}

	workspace := resolvedRestoreWorkspace(req, plan)
	if err := validateRestoreWorkspace(workspace, plan.SnapshotSource); err != nil {
		return "", err
	}
	if err := ensureRestoreWorkspaceReady(workspace); err != nil {
		return "", err
	}

	dup := duplicacy.NewWorkspaceSetup(workspace, cfg.Storage, false, nil)
	if err := dup.CreateDirs(); err != nil {
		return "", err
	}
	if err := dup.WritePreferences(sec); err != nil {
		return "", err
	}
	if err := dup.SetPermissions(); err != nil {
		return "", err
	}

	return formatRestorePrepare(newRestorePrepareReport(req, plan, cfg.Storage, workspace, storageSpec.NeedsSecrets())), nil
}

func handleRestoreRevisions(req *Request, meta Metadata, rt Runtime) (string, error) {
	ctx, err := newRestoreExecutionContext(req, meta, rt, true)
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

func handleRestoreFiles(req *Request, meta Metadata, rt Runtime) (string, error) {
	restorePath, err := cleanRestorePath(req.RestorePath)
	if err != nil {
		return "", err
	}
	ctx, err := newRestoreExecutionContext(req, meta, rt, true)
	if err != nil {
		return "", err
	}
	defer ctx.cleanup()

	output, err := ctx.dup.ListRevisionFiles(req.RestoreRevision)
	if err != nil {
		return "", err
	}
	paths, totalMatches := extractRestoreFileLines(output, restorePath, req.RestoreLimit)
	report := newRestoreFilesReport(req, ctx, restorePath, paths, totalMatches)
	if req.JSONSummary {
		return marshalRestoreJSON(report)
	}
	return formatRestoreFiles(report), nil
}

func handleRestoreRun(req *Request, meta Metadata, rt Runtime) (string, error) {
	restorePath, err := cleanRestorePath(req.RestorePath)
	if err != nil {
		return "", err
	}
	planner := NewConfigPlanner(meta, rt)
	planReq := configValidationRequest(req, req.Target())
	plan := planner.derivePlan(planReq)
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.applyConfig(cfg, rt)

	workspace := resolvedPreparedRestoreWorkspace(req, plan)
	if err := validateRestoreWorkspace(workspace, plan.SnapshotSource); err != nil {
		return "", err
	}
	if !restoreWorkspacePrepared(workspace) {
		return "", NewRequestError("restore run requires a prepared workspace; run restore prepare --target %s --workspace %s %s first", req.Target(), shellQuote(workspace), req.Source)
	}

	report := newRestoreRunReport(req, plan, cfg.Storage, workspace, restorePath)
	if req.DryRun {
		report.Result = "Dry run"
		report.Output = "restore command was not executed"
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

	dup := duplicacy.NewWorkspaceSetup(workspace, cfg.Storage, false, newRestoreCommandRunner())
	output, err := dup.RestoreRevision(req.RestoreRevision, restorePath)
	report.Output = strings.TrimSpace(output)
	if err != nil {
		report.Result = "Failed"
		return formatRestoreRun(report), err
	}
	report.Result = "Restored into workspace"
	return formatRestoreRun(report), nil
}

func handleRestoreSelect(req *Request, meta Metadata, rt Runtime) (string, error) {
	if rt.StdinIsTTY == nil || !rt.StdinIsTTY() {
		return "", NewRequestError("restore select requires an interactive terminal; use restore revisions, restore files, and restore run for scripts or scheduled jobs")
	}
	stdin := os.Stdin
	if rt.Stdin != nil && rt.Stdin() != nil {
		stdin = rt.Stdin()
	}
	reader := bufio.NewReader(stdin)

	ctx, commandWorkspace, workspacePrepared, cleanup, err := newRestoreSelectContext(req, meta, rt)
	if err != nil {
		return "", err
	}
	defer cleanup()
	if req.RestoreExecute && !workspacePrepared {
		return "", NewRequestError("restore select --execute requires a prepared workspace; run restore prepare --target %s --workspace %s %s first", req.Target(), shellQuote(commandWorkspace), req.Source)
	}

	revisions, _, err := ctx.dup.ListVisibleRevisions()
	if err != nil {
		return "", err
	}
	if len(revisions) == 0 {
		return "", NewRequestError("restore select found no visible revisions; run restore revisions --target %s %s to inspect the target directly", req.Target(), req.Source)
	}
	revision, err := promptRestoreRevision(reader, revisions, 50)
	if err != nil {
		return "", err
	}

	restorePaths := []string{""}
	selective, err := promptRestoreYesNo(reader, "Restore a specific path instead of the full revision? [y/N]: ")
	if err != nil {
		return "", err
	}
	if selective {
		restorePaths, err = promptRestorePath(ctx, req, meta, revision)
		if err != nil {
			return "", err
		}
	}

	report := newRestoreSelectReport(req, meta, ctx.plan, ctx.cfg.Storage, commandWorkspace, workspacePrepared, revision, restorePaths)
	selectOutput := formatRestoreSelect(report)
	if !req.RestoreExecute {
		return selectOutput, nil
	}
	confirmed, err := confirmRestoreSelectExecution(reader, report)
	if err != nil {
		return "", err
	}
	if !confirmed {
		return "", NewRequestError("restore select execution cancelled")
	}
	outputs := []string{selectOutput}
	for _, restorePath := range restorePaths {
		runReq := *req
		runReq.RestoreCommand = "run"
		runReq.RestoreRevision = revision
		runReq.RestorePath = restorePath
		runReq.RestoreWorkspace = commandWorkspace
		runReq.RestoreYes = true
		runReq.RestoreExecute = false
		runOutput, err := handleRestoreRun(&runReq, meta, rt)
		outputs = append(outputs, runOutput)
		if err != nil {
			return strings.Join(outputs, "\n"), err
		}
	}
	return strings.Join(outputs, "\n"), nil
}

type restoreExecutionContext struct {
	plan       *Plan
	cfg        *configForRestore
	workspace  string
	mode       string
	secrets    *secrets.Secrets
	dup        *duplicacy.Setup
	cleanup    func()
	secretPath string
}

type configForRestore struct {
	Storage string
}

func newRestoreExecutionContext(req *Request, meta Metadata, rt Runtime, allowTemporary bool) (*restoreExecutionContext, error) {
	planner := NewConfigPlanner(meta, rt)
	planReq := configValidationRequest(req, req.Target())
	plan := planner.derivePlan(planReq)
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return nil, err
	}
	plan.applyConfig(cfg, rt)

	storageSpec := duplicacy.NewStorageSpec(cfg.Storage)
	var sec *secrets.Secrets
	if storageSpec.NeedsSecrets() {
		sec, err = planner.loadSecrets(plan)
		if err != nil {
			return nil, err
		}
		if err := storageSpec.ValidateSecrets(sec); err != nil {
			return nil, err
		}
	}

	workspace, mode, cleanup, err := restoreWorkspaceForRead(req, plan, rt, allowTemporary)
	if err != nil {
		return nil, err
	}
	if mode == "temporary" {
		if err := writeRestoreWorkspacePreferences(workspace, cfg.Storage, sec); err != nil {
			cleanup()
			return nil, err
		}
	}

	return &restoreExecutionContext{
		plan:       plan,
		cfg:        &configForRestore{Storage: cfg.Storage},
		workspace:  workspace,
		mode:       mode,
		secrets:    sec,
		dup:        duplicacy.NewWorkspaceSetup(workspace, cfg.Storage, false, newRestoreCommandRunner()),
		cleanup:    cleanup,
		secretPath: plan.SecretsFile,
	}, nil
}

type restorePlanReport struct {
	Label             string
	Target            string
	Location          string
	ConfigFile        string
	SourcePath        string
	Storage           string
	SecretsFile       string
	SecretsRequired   bool
	StateFile         string
	StateStatus       string
	LatestRevision    int
	LatestRevisionAt  string
	SnapshotID        string
	Workspace         string
	InitCommand       string
	ListCommand       string
	ListFilesCommand  string
	FullRestore       string
	SelectiveRestore  string
	CopyBackPreview   string
	DocumentationPath string
}

type restorePrepareReport struct {
	Label            string
	Target           string
	Location         string
	ConfigFile       string
	SourcePath       string
	Storage          string
	SecretsFile      string
	Workspace        string
	PreferencesFile  string
	SnapshotID       string
	ListCommand      string
	ListFilesCommand string
	Guide            string
}

type restoreRevisionsReport struct {
	Label           string                `json:"label"`
	Target          string                `json:"target"`
	Location        string                `json:"location"`
	ConfigFile      string                `json:"config_file"`
	Storage         string                `json:"storage"`
	Workspace       string                `json:"workspace"`
	WorkspaceMode   string                `json:"workspace_mode"`
	SecretsFile     string                `json:"secrets_file"`
	RevisionCount   int                   `json:"revision_count"`
	Showing         int                   `json:"showing"`
	Limit           int                   `json:"limit"`
	Revisions       []restoreRevisionItem `json:"revisions"`
	ExecutesRestore bool                  `json:"executes_restore"`
}

type restoreRevisionItem struct {
	Revision  int    `json:"revision"`
	CreatedAt string `json:"created_at,omitempty"`
}

type restoreFilesReport struct {
	Label           string   `json:"label"`
	Target          string   `json:"target"`
	Location        string   `json:"location"`
	ConfigFile      string   `json:"config_file"`
	Storage         string   `json:"storage"`
	Workspace       string   `json:"workspace"`
	WorkspaceMode   string   `json:"workspace_mode"`
	Revision        int      `json:"revision"`
	PathFilter      string   `json:"path_filter,omitempty"`
	TotalMatches    int      `json:"total_matches"`
	Showing         int      `json:"showing"`
	Limit           int      `json:"limit"`
	Paths           []string `json:"paths"`
	ExecutesRestore bool     `json:"executes_restore"`
}

type restoreRunReport struct {
	Label       string
	Target      string
	Location    string
	ConfigFile  string
	SourcePath  string
	Storage     string
	Workspace   string
	Revision    int
	RestorePath string
	Command     string
	DryRun      bool
	Result      string
	Output      string
	Guide       string
}

type restoreSelectReport struct {
	Label             string
	Target            string
	Location          string
	ConfigFile        string
	SourcePath        string
	Storage           string
	Workspace         string
	WorkspacePrepared bool
	Revision          int
	RestorePaths      []string
	PrepareCommand    string
	RestoreCommands   []string
	Execute           bool
	Guide             string
}

func newRestorePrepareReport(req *Request, plan *Plan, storage, workspace string, secretsRequired bool) *restorePrepareReport {
	secretsFile := "Not required for this storage backend"
	if secretsRequired {
		secretsFile = plan.SecretsFile
	}
	return &restorePrepareReport{
		Label:            req.Source,
		Target:           req.Target(),
		Location:         plan.Location,
		ConfigFile:       plan.ConfigFile,
		SourcePath:       plan.SnapshotSource,
		Storage:          storage,
		SecretsFile:      secretsFile,
		Workspace:        workspace,
		PreferencesFile:  filepath.Join(workspace, ".duplicacy", "preferences"),
		SnapshotID:       duplicacy.DefaultSnapshotID,
		ListCommand:      "duplicacy list",
		ListFilesCommand: "duplicacy list -files -r <revision>",
		Guide:            "docs/restore-drills.md",
	}
}

func newRestorePlanReport(req *Request, meta Metadata, plan *Plan, storage string) *restorePlanReport {
	workspace := resolvedRestoreWorkspace(req, plan)
	secretsRequired := duplicacy.NewStorageSpec(storage).NeedsSecrets()
	report := &restorePlanReport{
		Label:             req.Source,
		Target:            req.Target(),
		Location:          plan.Location,
		ConfigFile:        plan.ConfigFile,
		SourcePath:        plan.SnapshotSource,
		Storage:           storage,
		SecretsFile:       "Not required for this storage backend",
		SecretsRequired:   secretsRequired,
		StateFile:         stateFilePath(meta, req.Source, req.Target()),
		StateStatus:       "Not found",
		SnapshotID:        duplicacy.DefaultSnapshotID,
		Workspace:         workspace,
		ListCommand:       "duplicacy list",
		ListFilesCommand:  "duplicacy list -files -r <revision>",
		FullRestore:       "duplicacy restore -r <revision> -stats",
		SelectiveRestore:  `duplicacy restore -r <revision> -stats -- "relative/path/from/snapshot"`,
		DocumentationPath: "docs/restore-drills.md",
	}
	if secretsRequired {
		report.SecretsFile = plan.SecretsFile
	}
	report.InitCommand = fmt.Sprintf("duplicacy init %s %s", shellQuote(report.SnapshotID), shellQuote(storage))
	report.CopyBackPreview = fmt.Sprintf("rsync -a --dry-run %s %s", shellQuote(filepath.Join(workspace, "relative/path")), shellQuote(filepath.Join(plan.SnapshotSource, "relative/path")))
	return report
}

func newRestoreRevisionsReport(req *Request, ctx *restoreExecutionContext, revisions []duplicacy.RevisionInfo) *restoreRevisionsReport {
	items := make([]restoreRevisionItem, 0, len(revisions))
	for _, revision := range revisions {
		items = append(items, restoreRevisionItem{
			Revision:  revision.Revision,
			CreatedAt: formatRevisionCreatedAt(revision),
		})
	}
	if req.RestoreLimit > 0 && len(items) > req.RestoreLimit {
		items = items[:req.RestoreLimit]
	}
	secretsFile := "Not required for this storage backend"
	if ctx.secrets != nil {
		secretsFile = ctx.secretPath
	}
	return &restoreRevisionsReport{
		Label:           req.Source,
		Target:          req.Target(),
		Location:        ctx.plan.Location,
		ConfigFile:      ctx.plan.ConfigFile,
		Storage:         ctx.cfg.Storage,
		Workspace:       ctx.workspace,
		WorkspaceMode:   ctx.mode,
		SecretsFile:     secretsFile,
		RevisionCount:   len(revisions),
		Showing:         len(items),
		Limit:           req.RestoreLimit,
		Revisions:       items,
		ExecutesRestore: false,
	}
}

func newRestoreFilesReport(req *Request, ctx *restoreExecutionContext, restorePath string, paths []string, totalMatches int) *restoreFilesReport {
	return &restoreFilesReport{
		Label:           req.Source,
		Target:          req.Target(),
		Location:        ctx.plan.Location,
		ConfigFile:      ctx.plan.ConfigFile,
		Storage:         ctx.cfg.Storage,
		Workspace:       ctx.workspace,
		WorkspaceMode:   ctx.mode,
		Revision:        req.RestoreRevision,
		PathFilter:      restorePath,
		TotalMatches:    totalMatches,
		Showing:         len(paths),
		Limit:           req.RestoreLimit,
		Paths:           paths,
		ExecutesRestore: false,
	}
}

func newRestoreRunReport(req *Request, plan *Plan, storage, workspace, restorePath string) *restoreRunReport {
	command := fmt.Sprintf("duplicacy restore -r %d -stats", req.RestoreRevision)
	if restorePath != "" {
		command += " -- " + shellQuote(restorePath)
	}
	return &restoreRunReport{
		Label:       req.Source,
		Target:      req.Target(),
		Location:    plan.Location,
		ConfigFile:  plan.ConfigFile,
		SourcePath:  plan.SnapshotSource,
		Storage:     storage,
		Workspace:   workspace,
		Revision:    req.RestoreRevision,
		RestorePath: restorePath,
		Command:     command,
		DryRun:      req.DryRun,
		Result:      "Pending confirmation",
		Guide:       "docs/restore-drills.md",
	}
}

func newRestoreSelectReport(req *Request, meta Metadata, plan *Plan, storage, workspace string, workspacePrepared bool, revision int, restorePaths []string) *restoreSelectReport {
	return &restoreSelectReport{
		Label:             req.Source,
		Target:            req.Target(),
		Location:          plan.Location,
		ConfigFile:        plan.ConfigFile,
		SourcePath:        plan.SnapshotSource,
		Storage:           storage,
		Workspace:         workspace,
		WorkspacePrepared: workspacePrepared,
		Revision:          revision,
		RestorePaths:      normaliseRestoreSelection(restorePaths),
		PrepareCommand:    buildRestorePrepareCommand(meta.ScriptName, req, workspace),
		RestoreCommands:   buildRestoreRunCommands(meta.ScriptName, req, revision, restorePaths, workspace),
		Execute:           req.RestoreExecute,
		Guide:             "docs/restore-drills.md",
	}
}

func (r *restorePlanReport) loadAndApplyState(meta Metadata, label, target string) {
	state, err := loadRunState(meta, label, target)
	r.applyState(state, err)
}

func (r *restorePlanReport) applyState(state *RunState, err error) {
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.StateStatus = "Not found"
			return
		}
		r.StateStatus = fmt.Sprintf("Unreadable (%s)", OperatorMessage(err))
		return
	}
	r.StateStatus = "Available"
	if state.LastSuccessfulBackupRevision > 0 {
		r.LatestRevision = state.LastSuccessfulBackupRevision
	}
	r.LatestRevisionAt = state.LastSuccessfulBackupAt
}

func resolvedRestoreWorkspace(req *Request, plan *Plan) string {
	workspace := req.RestoreWorkspace
	if strings.TrimSpace(workspace) == "" {
		workspace = recommendedRestoreWorkspace(plan.SnapshotSource, req.Source, req.Target())
	}
	return filepath.Clean(strings.TrimSpace(workspace))
}

func resolvedPreparedRestoreWorkspace(req *Request, plan *Plan) string {
	if strings.TrimSpace(req.RestoreWorkspace) != "" {
		return resolvedRestoreWorkspace(req, plan)
	}
	if workspace, ok := latestPreparedRestoreWorkspace(plan.SnapshotSource, req.Source, req.Target()); ok {
		return workspace
	}
	return recommendedRestoreWorkspace(plan.SnapshotSource, req.Source, req.Target())
}

func recommendedRestoreWorkspace(sourcePath, label, target string) string {
	base := rootVolumeForSource(sourcePath)
	timestamp := restoreWorkspaceNow().Local().Format("20060102-150405")
	return filepath.Join(base, "restore-drills", fmt.Sprintf("%s-%s-%s", label, target, timestamp))
}

func latestPreparedRestoreWorkspace(sourcePath, label, target string) (string, bool) {
	base := filepath.Join(rootVolumeForSource(sourcePath), "restore-drills")
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", false
	}
	prefix := fmt.Sprintf("%s-%s", label, target)
	bestName := ""
	bestPath := ""
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name != prefix && !strings.HasPrefix(name, prefix+"-") {
			continue
		}
		path := filepath.Join(base, name)
		if !restoreWorkspacePrepared(path) {
			continue
		}
		if bestName == "" || name > bestName {
			bestName = name
			bestPath = path
		}
	}
	if bestPath == "" {
		return "", false
	}
	return bestPath, true
}

func validateRestoreWorkspace(workspace, sourcePath string) error {
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	if workspace == "" || workspace == "." {
		return NewRequestError("--workspace must be an absolute path")
	}
	if !filepath.IsAbs(workspace) {
		return NewRequestError("--workspace must be an absolute path: %s", workspace)
	}
	source := filepath.Clean(sourcePath)
	if workspace == source {
		return NewRequestError("restore workspace must not be the live source path: %s", workspace)
	}
	if isPathWithin(source, workspace) {
		return NewRequestError("restore workspace must not be inside the live source path: %s", workspace)
	}
	return nil
}

func isPathWithin(parent, child string) bool {
	if parent == "" || child == "" || !filepath.IsAbs(parent) || !filepath.IsAbs(child) {
		return false
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func ensureRestoreWorkspaceReady(workspace string) error {
	info, err := os.Stat(workspace)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(workspace, 0770)
		}
		return fmt.Errorf("restore workspace is not accessible: %w", err)
	}
	if !info.IsDir() {
		return NewRequestError("restore workspace must be a directory: %s", workspace)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return fmt.Errorf("restore workspace cannot be read: %w", err)
	}
	for _, entry := range entries {
		if entry.Name() == ".duplicacy" && entry.IsDir() {
			continue
		}
		return NewRequestError("restore workspace must be empty before preparation: %s", workspace)
	}
	return nil
}

func restoreWorkspaceForRead(req *Request, plan *Plan, rt Runtime, allowTemporary bool) (string, string, func(), error) {
	if strings.TrimSpace(req.RestoreWorkspace) != "" {
		workspace := resolvedRestoreWorkspace(req, plan)
		if err := validateRestoreWorkspace(workspace, plan.SnapshotSource); err != nil {
			return "", "", func() {}, err
		}
		if !restoreWorkspacePrepared(workspace) {
			return "", "", func() {}, NewRequestError("restore listing with --workspace requires a prepared workspace; run restore prepare --target %s --workspace %s %s first", req.Target(), shellQuote(workspace), req.Source)
		}
		return workspace, "prepared", func() {}, nil
	}
	if !allowTemporary {
		return "", "", func() {}, NewRequestError("--workspace is required")
	}
	base := rt.TempDir
	tempParent := os.TempDir()
	if base != nil && strings.TrimSpace(base()) != "" {
		tempParent = base()
	}
	workspace, err := os.MkdirTemp(tempParent, "duplicacy-restore-*")
	if err != nil {
		return "", "", func() {}, fmt.Errorf("restore temporary workspace cannot be created: %w", err)
	}
	if err := validateRestoreWorkspace(workspace, plan.SnapshotSource); err != nil {
		_ = os.RemoveAll(workspace)
		return "", "", func() {}, err
	}
	return workspace, "temporary", func() { _ = os.RemoveAll(workspace) }, nil
}

func writeRestoreWorkspacePreferences(workspace, storage string, sec *secrets.Secrets) error {
	dup := duplicacy.NewWorkspaceSetup(workspace, storage, false, nil)
	if err := dup.CreateDirs(); err != nil {
		return err
	}
	if err := dup.WritePreferences(sec); err != nil {
		return err
	}
	return dup.SetPermissions()
}

func restoreWorkspacePrepared(workspace string) bool {
	info, err := os.Stat(filepath.Join(workspace, ".duplicacy", "preferences"))
	return err == nil && !info.IsDir()
}

func newRestoreSelectContext(req *Request, meta Metadata, rt Runtime) (*restoreExecutionContext, string, bool, func(), error) {
	planner := NewConfigPlanner(meta, rt)
	planReq := configValidationRequest(req, req.Target())
	plan := planner.derivePlan(planReq)
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return nil, "", false, func() {}, err
	}
	plan.applyConfig(cfg, rt)

	commandWorkspace := resolvedPreparedRestoreWorkspace(req, plan)
	if err := validateRestoreWorkspace(commandWorkspace, plan.SnapshotSource); err != nil {
		return nil, "", false, func() {}, err
	}
	workspacePrepared := restoreWorkspacePrepared(commandWorkspace)

	listingReq := *req
	if workspacePrepared {
		listingReq.RestoreWorkspace = commandWorkspace
	} else {
		listingReq.RestoreWorkspace = ""
	}
	ctx, err := newRestoreExecutionContext(&listingReq, meta, rt, true)
	if err != nil {
		return nil, "", false, func() {}, err
	}
	return ctx, commandWorkspace, workspacePrepared, ctx.cleanup, nil
}

func cleanRestorePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.ContainsRune(value, 0) {
		return "", NewRequestError("--path must not contain NUL characters")
	}
	if filepath.IsAbs(value) {
		return "", NewRequestError("--path must be relative to the backup snapshot: %s", value)
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", NewRequestError("--path must stay inside the backup snapshot: %s", value)
	}
	return filepath.ToSlash(cleaned), nil
}

func promptRestoreRevision(reader *bufio.Reader, revisions []duplicacy.RevisionInfo, limit int) (int, error) {
	shown := revisions
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}
	fmt.Fprintln(restorePromptOutput, "Available revisions:")
	for i, revision := range shown {
		value := strconv.Itoa(revision.Revision)
		if createdAt := formatRevisionCreatedAt(revision); createdAt != "" {
			value += " (" + createdAt + ")"
		}
		fmt.Fprintf(restorePromptOutput, "  %d. %s\n", i+1, value)
	}
	answer, err := promptRestoreLine(reader, "Select revision by list number or revision id: ")
	if err != nil {
		return 0, err
	}
	choice, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || choice <= 0 {
		return 0, NewRequestError("restore select requires a positive revision selection")
	}
	if choice <= len(shown) {
		return shown[choice-1].Revision, nil
	}
	for _, revision := range revisions {
		if revision.Revision == choice {
			return revision.Revision, nil
		}
	}
	return 0, NewRequestError("revision %d was not found in the visible revision list", choice)
}

func promptRestorePath(ctx *restoreExecutionContext, req *Request, meta Metadata, revision int) ([]string, error) {
	pathPrefix, err := cleanRestorePath(req.RestorePathPrefix)
	if err != nil {
		return nil, err
	}
	output, err := ctx.dup.ListRevisionFiles(revision)
	if err != nil {
		return nil, err
	}
	paths := extractRestoreFilePaths(output)
	if len(paths) == 0 {
		return nil, NewRequestError("restore select found no restorable paths in revision %d", revision)
	}
	if pathPrefix != "" && !restorePathPrefixHasMatches(paths, pathPrefix) {
		return nil, NewRequestError("restore select found no paths under prefix %q in revision %d", pathPrefix, revision)
	}
	restorePaths, err := runRestoreSelectPicker(paths, restorepicker.AppOptions{
		Title:      fmt.Sprintf("Restore selection for %s/%s", req.Source, req.Target()),
		PathPrefix: pathPrefix,
		Primitive: restorepicker.PrimitiveOptions{
			ScriptName: meta.ScriptName,
			Source:     req.Source,
			Target:     req.Target(),
			Revision:   strconv.Itoa(revision),
			Workspace:  ctx.workspace,
		},
	})
	if err != nil {
		if errors.Is(err, restorepicker.ErrPickerCancelled) {
			return nil, NewRequestError("restore select cancelled")
		}
		return nil, err
	}
	if len(restorePaths) == 0 {
		return nil, NewRequestError("restore select requires at least one restore path")
	}
	return restorePaths, nil
}

func restorePathPrefixHasMatches(paths []string, prefix string) bool {
	prefix = strings.Trim(prefix, "/")
	for _, path := range paths {
		path = strings.Trim(filepath.ToSlash(path), "/")
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func promptRestoreYesNo(reader *bufio.Reader, prompt string) (bool, error) {
	answer, err := promptRestoreLine(reader, prompt)
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func confirmRestoreSelectExecution(reader *bufio.Reader, report *restoreSelectReport) (bool, error) {
	fmt.Fprintln(restorePromptOutput, "Ready to execute restore command(s):")
	fmt.Fprintf(restorePromptOutput, "  Revision : %d\n", report.Revision)
	fmt.Fprintf(restorePromptOutput, "  Workspace: %s\n", report.Workspace)
	for _, path := range restoreDisplayPaths(report.RestorePaths) {
		fmt.Fprintf(restorePromptOutput, "  Path     : %s\n", path)
	}
	for _, command := range report.RestoreCommands {
		fmt.Fprintf(restorePromptOutput, "  Command  : %s\n", command)
	}
	return promptRestoreYesNo(reader, "Execute restore into the prepared workspace? [y/N]: ")
}

func promptRestoreLine(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Fprint(restorePromptOutput, prompt)
	answer, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(answer) == "" {
		return "", fmt.Errorf("failed to read restore selection: %w", err)
	}
	return strings.TrimSpace(answer), nil
}

func buildRestorePrepareCommand(scriptName string, req *Request, workspace string) string {
	args := []string{
		"sudo",
		shellQuote(scriptName),
		"restore",
		"prepare",
		"--target",
		shellQuote(req.Target()),
		"--workspace",
		shellQuote(workspace),
	}
	args = appendRestoreConfigFlags(args, req)
	args = append(args, shellQuote(req.Source))
	return strings.Join(args, " ")
}

func buildRestoreRunCommand(scriptName string, req *Request, revision int, restorePath string, workspace string) string {
	args := []string{
		"sudo",
		shellQuote(scriptName),
		"restore",
		"run",
		"--target",
		shellQuote(req.Target()),
		"--revision",
		strconv.Itoa(revision),
		"--workspace",
		shellQuote(workspace),
		"--yes",
	}
	if restorePath != "" {
		args = append(args, "--path", shellQuote(restorePath))
	}
	args = appendRestoreConfigFlags(args, req)
	args = append(args, shellQuote(req.Source))
	return strings.Join(args, " ")
}

func buildRestoreRunCommands(scriptName string, req *Request, revision int, restorePaths []string, workspace string) []string {
	restorePaths = normaliseRestoreSelection(restorePaths)
	commands := make([]string, 0, len(restorePaths))
	for _, restorePath := range restorePaths {
		commands = append(commands, buildRestoreRunCommand(scriptName, req, revision, restorePath, workspace))
	}
	return commands
}

func normaliseRestoreSelection(restorePaths []string) []string {
	if len(restorePaths) == 0 {
		return []string{""}
	}
	normalised := make([]string, 0, len(restorePaths))
	for _, restorePath := range restorePaths {
		normalised = append(normalised, strings.TrimSpace(restorePath))
	}
	return normalised
}

func appendRestoreConfigFlags(args []string, req *Request) []string {
	if strings.TrimSpace(req.ConfigDir) != "" {
		args = append(args, "--config-dir", shellQuote(req.ConfigDir))
	}
	if strings.TrimSpace(req.SecretsDir) != "" {
		args = append(args, "--secrets-dir", shellQuote(req.SecretsDir))
	}
	return args
}

func extractRestoreFileLines(output, restorePath string, limit int) ([]string, int) {
	allPaths := extractRestoreFilePaths(output)
	var paths []string
	totalMatches := 0
	for _, path := range allPaths {
		if restorePath != "" && !strings.Contains(filepath.ToSlash(path), restorePath) {
			continue
		}
		totalMatches++
		if limit <= 0 || len(paths) < limit {
			paths = append(paths, path)
		}
	}
	return paths, totalMatches
}

func extractRestoreFilePaths(output string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isIgnoredRestoreListLine(line) {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(extractRestoreFilePath(line)))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func extractRestoreFilePath(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || isIgnoredRestoreListLine(line) {
		return ""
	}
	fields, remainder := splitLeadingFields(line, 4)
	if len(fields) < 4 || strings.TrimSpace(remainder) == "" {
		return line
	}
	if _, err := strconv.ParseInt(fields[0], 10, 64); err != nil {
		return line
	}
	if !looksLikeDate(fields[1]) || !looksLikeTime(fields[2]) || !looksLikeHexDigest(fields[3]) {
		return line
	}
	return strings.TrimSpace(remainder)
}

func isIgnoredRestoreListLine(line string) bool {
	line = strings.TrimSpace(line)
	switch {
	case line == "":
		return true
	case strings.HasPrefix(line, "Files: "):
		return true
	case strings.HasPrefix(line, "Total size: "):
		return true
	case strings.HasPrefix(line, "Repository set to "):
		return true
	case strings.HasPrefix(line, "Storage set to "):
		return true
	case strings.HasPrefix(line, "Loaded "):
		return true
	case strings.HasPrefix(line, "Parsing "):
		return true
	case strings.HasPrefix(line, "Restoring "):
		return true
	case strings.HasPrefix(line, "Restored "):
		return true
	case strings.HasPrefix(line, "Skipped "):
		return true
	case strings.HasPrefix(line, "Downloaded "):
		return true
	case strings.HasPrefix(line, "Total running time: "):
		return true
	case strings.HasPrefix(line, "Snapshot ") && strings.Contains(line, " created at "):
		return true
	default:
		return false
	}
}

func splitLeadingFields(value string, count int) ([]string, string) {
	remainder := strings.TrimLeftFunc(value, unicode.IsSpace)
	fields := make([]string, 0, count)
	for len(fields) < count && remainder != "" {
		fieldEnd := strings.IndexFunc(remainder, unicode.IsSpace)
		if fieldEnd < 0 {
			fields = append(fields, remainder)
			return fields, ""
		}
		fields = append(fields, remainder[:fieldEnd])
		remainder = strings.TrimLeftFunc(remainder[fieldEnd:], unicode.IsSpace)
	}
	return fields, remainder
}

func looksLikeDate(value string) bool {
	if len(value) != len("2006-01-02") {
		return false
	}
	for i, r := range value {
		switch i {
		case 4, 7:
			if r != '-' {
				return false
			}
		default:
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func looksLikeTime(value string) bool {
	if len(value) != len("15:04:05") {
		return false
	}
	for i, r := range value {
		switch i {
		case 2, 5:
			if r != ':' {
				return false
			}
		default:
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func looksLikeHexDigest(value string) bool {
	if len(value) < 8 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func formatRevisionCreatedAt(revision duplicacy.RevisionInfo) string {
	if revision.CreatedAt.IsZero() {
		return ""
	}
	return revision.CreatedAt.Format("2006-01-02 15:04:05")
}

func marshalRestoreJSON(value interface{}) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func confirmRestoreRun(rt Runtime, report *restoreRunReport) (bool, error) {
	if rt.StdinIsTTY == nil || !rt.StdinIsTTY() {
		return false, NewRequestError("restore run requires --yes when not running interactively")
	}
	stdin := os.Stdin
	if rt.Stdin != nil && rt.Stdin() != nil {
		stdin = rt.Stdin()
	}
	fmt.Fprintf(os.Stdout, "Restore revision %d into %s? [y/N]: ", report.Revision, report.Workspace)
	answer, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(answer) == "" {
		return false, fmt.Errorf("failed to read restore confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func formatRestorePlan(report *restorePlanReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore plan for %s/%s\n", report.Label, report.Target)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Target", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Read Only", Value: "true"},
		{Label: "Executes Restore", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, report.SourcePath, report.Storage, report.SecretsFile)
	writeRestoreSection(&b, "Safe Workspace", []SummaryLine{
		{Label: "Workspace", Value: report.Workspace},
		{Label: "Snapshot ID", Value: report.SnapshotID},
		{Label: "Rule", Value: "restore here first, never directly over the live source path"},
	})

	revision := "Not known from state"
	if report.LatestRevision > 0 {
		revision = strconv.Itoa(report.LatestRevision)
		if report.LatestRevisionAt != "" {
			revision = fmt.Sprintf("%s (%s)", revision, report.LatestRevisionAt)
		}
	}
	writeRestoreSection(&b, "Revision Signal", []SummaryLine{
		{Label: "State File", Value: report.StateFile},
		{Label: "State", Value: report.StateStatus},
		{Label: "Latest Revision", Value: revision},
		{Label: "Live Listing", Value: "run duplicacy list from the drill workspace"},
	})
	writeRestoreSection(&b, "Suggested Commands", []SummaryLine{
		{Label: "Create Workspace", Value: "sudo mkdir -p " + shellQuote(report.Workspace)},
		{Label: "Enter Workspace", Value: "cd " + shellQuote(report.Workspace)},
		{Label: "Init Workspace", Value: report.InitCommand},
		{Label: "List Revisions", Value: report.ListCommand},
		{Label: "List Files", Value: report.ListFilesCommand},
		{Label: "Full Restore", Value: report.FullRestore},
		{Label: "Selective Restore", Value: report.SelectiveRestore},
		{Label: "Copy Back Preview", Value: report.CopyBackPreview},
	})
	writeRestoreSafetySection(&b, "inspect restored data first; use rsync --dry-run before live changes", report.DocumentationPath)
	return b.String()
}

func formatRestorePrepare(report *restorePrepareReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore workspace prepared for %s/%s\n", report.Label, report.Target)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Target", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Executes Restore", Value: "false"},
		{Label: "Copies Back", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, report.SourcePath, report.Storage, report.SecretsFile)
	writeRestoreSection(&b, "Workspace", []SummaryLine{
		{Label: "Path", Value: report.Workspace},
		{Label: "Snapshot ID", Value: report.SnapshotID},
		{Label: "Preferences", Value: report.PreferencesFile},
		{Label: "Rule", Value: "restore here first, never directly over the live source path"},
	})
	writeRestoreSection(&b, "Next Commands", []SummaryLine{
		{Label: "Enter Workspace", Value: "cd " + shellQuote(report.Workspace)},
		{Label: "List Revisions", Value: report.ListCommand},
		{Label: "List Files", Value: report.ListFilesCommand},
	})
	writeRestoreSafetySection(&b, "manual only; inspect restored data and use rsync --dry-run first", report.Guide)
	return b.String()
}

func formatRestoreRevisions(report *restoreRevisionsReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore revisions for %s/%s\n", report.Label, report.Target)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Target", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Read Only", Value: "true"},
		{Label: "Executes Restore", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, "", report.Storage, report.SecretsFile)
	writeRestoreSection(&b, "Workspace", []SummaryLine{
		{Label: "Mode", Value: report.WorkspaceMode},
		{Label: "Path", Value: report.Workspace},
	})
	writeRestoreSection(&b, "Revisions", []SummaryLine{
		{Label: "Revision Count", Value: strconv.Itoa(report.RevisionCount)},
		{Label: "Showing", Value: fmt.Sprintf("%d of %d", report.Showing, report.RevisionCount)},
		{Label: "Limit", Value: strconv.Itoa(report.Limit)},
	})
	for _, revision := range report.Revisions {
		value := strconv.Itoa(revision.Revision)
		if revision.CreatedAt != "" {
			value += " (" + revision.CreatedAt + ")"
		}
		writeRestoreLines(&b, []SummaryLine{{Label: "Revision", Value: value}})
	}
	writeRestoreSafetySection(&b, "not applicable; this command only lists revisions", "docs/restore-drills.md")
	return b.String()
}

func formatRestoreFiles(report *restoreFilesReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore files for %s/%s revision %d\n", report.Label, report.Target, report.Revision)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Target", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Read Only", Value: "true"},
		{Label: "Executes Restore", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, "", report.Storage, "")
	writeRestoreSection(&b, "Workspace", []SummaryLine{
		{Label: "Mode", Value: report.WorkspaceMode},
		{Label: "Path", Value: report.Workspace},
	})
	writeRestoreSection(&b, "File Listing", []SummaryLine{
		{Label: "Revision", Value: strconv.Itoa(report.Revision)},
		{Label: "Path Filter", Value: report.PathFilter},
		{Label: "Total Matches", Value: strconv.Itoa(report.TotalMatches)},
		{Label: "Showing", Value: fmt.Sprintf("%d of %d", report.Showing, report.TotalMatches)},
		{Label: "Limit", Value: strconv.Itoa(report.Limit)},
	})
	for _, path := range report.Paths {
		writeRestoreLines(&b, []SummaryLine{{Label: "Path", Value: path}})
	}
	writeRestoreSafetySection(&b, "not applicable; this command only lists revision contents", "docs/restore-drills.md")
	return b.String()
}

func formatRestoreRun(report *restoreRunReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore run for %s/%s revision %d\n", report.Label, report.Target, report.Revision)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Target", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Executes Restore", Value: fmt.Sprintf("%t", !report.DryRun)},
		{Label: "Copies Back", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, report.SourcePath, report.Storage, "")
	writeRestoreSection(&b, "Workspace", []SummaryLine{
		{Label: "Path", Value: report.Workspace},
		{Label: "Rule", Value: "restored files stay in the workspace until an operator manually copies them back"},
	})
	writeRestoreSection(&b, "Restore", []SummaryLine{
		{Label: "Revision", Value: strconv.Itoa(report.Revision)},
		{Label: "Path", Value: report.RestorePath},
		{Label: "Command", Value: report.Command},
		{Label: "Dry Run", Value: fmt.Sprintf("%t", report.DryRun)},
		{Label: "Result", Value: report.Result},
	})
	if report.Output != "" {
		writeRestoreSection(&b, "Duplicacy Output", []SummaryLine{{Label: "Output", Value: report.Output}})
	}
	writeRestoreSection(&b, "Safety", []SummaryLine{
		{Label: "Live Source", Value: "not modified"},
		{Label: "Copy Back", Value: "manual only; inspect restored data and use rsync --dry-run first"},
		{Label: "Guide", Value: report.Guide},
	})
	return b.String()
}

func formatRestoreSelect(report *restoreSelectReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore selection for %s/%s\n", report.Label, report.Target)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Target", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Executes Restore", Value: fmt.Sprintf("%t", report.Execute)},
		{Label: "Copies Back", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, report.SourcePath, report.Storage, "")
	writeRestoreSection(&b, "Workspace", []SummaryLine{
		{Label: "Path", Value: report.Workspace},
		{Label: "Prepared", Value: fmt.Sprintf("%t", report.WorkspacePrepared)},
		{Label: "Prepare Command", Value: selectValue(!report.WorkspacePrepared, report.PrepareCommand, "")},
	})
	selectionLines := []SummaryLine{{Label: "Revision", Value: strconv.Itoa(report.Revision)}}
	for _, restorePath := range restoreDisplayPaths(report.RestorePaths) {
		selectionLines = append(selectionLines, SummaryLine{Label: "Path", Value: restorePath})
	}
	writeRestoreSection(&b, "Selection", selectionLines)
	commandLines := make([]SummaryLine, 0, len(report.RestoreCommands))
	for _, command := range report.RestoreCommands {
		commandLines = append(commandLines, SummaryLine{Label: "Restore Command", Value: command})
	}
	writeRestoreSection(&b, "Generated Commands", commandLines)
	writeRestoreSection(&b, "Safety", []SummaryLine{
		{Label: "Command Model", Value: selectValue(report.Execute, "restore select delegates to restore run after confirmation", "restore select only generates primitive commands")},
		{Label: "Restore Execution", Value: selectValue(report.Execute, "delegated to restore run after confirmation", "not performed by this command")},
		{Label: "Copy Back", Value: "manual only; inspect restored data and use rsync --dry-run first"},
		{Label: "Guide", Value: report.Guide},
	})
	return b.String()
}

func restoreDisplayPaths(restorePaths []string) []string {
	restorePaths = normaliseRestoreSelection(restorePaths)
	display := make([]string, 0, len(restorePaths))
	for _, restorePath := range restorePaths {
		if restorePath == "" {
			display = append(display, "<full revision>")
			continue
		}
		display = append(display, restorePath)
	}
	return display
}

func selectValue(condition bool, whenTrue string, whenFalse string) string {
	if condition {
		return whenTrue
	}
	return whenFalse
}

func writeRestoreResolvedSection(b *strings.Builder, configFile, sourcePath, storage, secretsFile string) {
	writeRestoreSection(b, "Resolved", []SummaryLine{
		{Label: "Config File", Value: configFile},
		{Label: "Source Path", Value: sourcePath},
		{Label: "Storage", Value: storage},
		{Label: "Secrets File", Value: secretsFile},
	})
}

func writeRestoreSafetySection(b *strings.Builder, copyBack, guide string) {
	writeRestoreSection(b, "Safety", []SummaryLine{
		{Label: "Restore Execution", Value: "not performed by this command"},
		{Label: "Copy Back", Value: copyBack},
		{Label: "Guide", Value: guide},
	})
}

func writeRestoreSection(b *strings.Builder, name string, lines []SummaryLine) {
	fmt.Fprintf(b, "  Section: %s\n", name)
	for _, line := range lines {
		if strings.TrimSpace(line.Value) == "" {
			continue
		}
		fmt.Fprintf(b, "    %-18s : %s\n", line.Label, line.Value)
	}
}

func writeRestoreLines(b *strings.Builder, lines []SummaryLine) {
	for _, line := range lines {
		if strings.TrimSpace(line.Value) == "" {
			continue
		}
		fmt.Fprintf(b, "  %-20s : %s\n", line.Label, line.Value)
	}
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
