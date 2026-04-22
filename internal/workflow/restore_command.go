package workflow

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

var newRestoreCommandRunner = func() execpkg.Runner {
	runner := execpkg.NewCommandRunner(nil, false)
	runner.SetDebugCommands(false)
	return runner
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

	workspace := resolvedRestoreWorkspace(req, plan)
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

func recommendedRestoreWorkspace(sourcePath, label, target string) string {
	base := rootVolumeForSource(sourcePath)
	return filepath.Join(base, "restore-drills", fmt.Sprintf("%s-%s", label, target))
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

func extractRestoreFileLines(output, restorePath string, limit int) ([]string, int) {
	var paths []string
	totalMatches := 0
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if restorePath != "" && !strings.Contains(filepath.ToSlash(line), restorePath) {
			continue
		}
		totalMatches++
		if limit <= 0 || len(paths) < limit {
			paths = append(paths, line)
		}
	}
	return paths, totalMatches
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
