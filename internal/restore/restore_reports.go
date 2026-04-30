package restore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/operator"
)

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

type restoreRevisionsReport struct {
	Label           string                `json:"label"`
	Target          string                `json:"storage_name"`
	Location        string                `json:"location"`
	ConfigFile      string                `json:"config_file"`
	SourcePath      string                `json:"source_path"`
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

type restoreRunReport struct {
	Label             string
	Target            string
	Location          string
	ConfigFile        string
	SourcePath        string
	Storage           string
	Workspace         string
	WorkspacePrepared bool
	Revision          int
	RestorePath       string
	Command           string
	DryRun            bool
	Result            string
	Output            string
	Guide             string
}

type restoreBatchRunReport struct {
	Label        string
	Target       string
	Location     string
	Workspace    string
	Revision     int
	RestorePaths []string
	Results      []restoreBatchPathResult
	Guide        string
}

type restoreBatchPathResult struct {
	Path   string
	Result string
	Output string
}

type restoreInspectReport struct {
	Label      string
	Target     string
	Location   string
	ConfigFile string
	SourcePath string
	Storage    string
	Revision   int
	PathPrefix string
	Guide      string
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
	RestoreCommands   []string
	Guide             string
}

func newRestorePlanReport(req *RestoreRequest, meta Metadata, plan *Plan, storage string, deps RestoreDeps) *restorePlanReport {
	workspace := resolvedRestoreWorkspace(req, plan, deps)
	secretsRequired := duplicacy.NewStorageSpec(storage).NeedsSecrets()
	report := &restorePlanReport{
		Label:             req.Label,
		Target:            req.Target(),
		Location:          plan.Config.Location,
		ConfigFile:        plan.Paths.ConfigFile,
		SourcePath:        plan.Paths.SnapshotSource,
		Storage:           storage,
		SecretsFile:       "Not required for this storage backend",
		SecretsRequired:   secretsRequired,
		StateFile:         StateFilePath(meta, req.Label, req.Target()),
		StateStatus:       "Not found",
		SnapshotID:        duplicacy.DefaultSnapshotID,
		Workspace:         workspace,
		ListCommand:       "duplicacy list",
		ListFilesCommand:  "duplicacy list -files -r <revision>",
		FullRestore:       "duplicacy restore -r <revision> -stats -ignore-owner",
		SelectiveRestore:  `duplicacy restore -r <revision> -stats -ignore-owner -- "relative/path/from/snapshot"`,
		DocumentationPath: "docs/restore-drills.md",
	}
	if secretsRequired {
		report.SecretsFile = plan.Paths.SecretsFile
	}
	report.InitCommand = fmt.Sprintf("duplicacy init %s %s", shellQuote(report.SnapshotID), shellQuote(storage))
	report.CopyBackPreview = "Unavailable until source_path is configured"
	if strings.TrimSpace(plan.Paths.SnapshotSource) != "" {
		report.CopyBackPreview = fmt.Sprintf("rsync -a --dry-run %s %s", shellQuote(filepath.Join(workspace, "relative/path")), shellQuote(filepath.Join(plan.Paths.SnapshotSource, "relative/path")))
	}
	return report
}

func newRestoreRevisionsReport(req *RestoreRequest, ctx *restoreExecutionContext, revisions []duplicacy.RevisionInfo) *restoreRevisionsReport {
	items := make([]restoreRevisionItem, 0, len(revisions))
	for _, revision := range revisions {
		items = append(items, restoreRevisionItem{
			Revision:  revision.Revision,
			CreatedAt: formatRevisionCreatedAt(revision),
		})
	}
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}
	secretsFile := "Not required for this storage backend"
	if ctx.secrets != nil {
		secretsFile = ctx.secretPath
	}
	return &restoreRevisionsReport{
		Label:           req.Label,
		Target:          req.Target(),
		Location:        ctx.plan.Config.Location,
		ConfigFile:      ctx.plan.Paths.ConfigFile,
		SourcePath:      ctx.plan.Paths.SnapshotSource,
		Storage:         ctx.cfg.Storage,
		Workspace:       ctx.workspace,
		WorkspaceMode:   ctx.mode,
		SecretsFile:     secretsFile,
		RevisionCount:   len(revisions),
		Showing:         len(items),
		Limit:           req.Limit,
		Revisions:       items,
		ExecutesRestore: false,
	}
}

func newRestoreRunReport(req *RestoreRequest, plan *Plan, storage, workspace string, revision int, restorePath string, dryRun bool) *restoreRunReport {
	command := fmt.Sprintf("duplicacy restore -r %d -stats -ignore-owner", revision)
	if restorePath != "" {
		command += " -- " + shellQuote(restorePath)
	}
	return &restoreRunReport{
		Label:             req.Label,
		Target:            req.Target(),
		Location:          plan.Config.Location,
		ConfigFile:        plan.Paths.ConfigFile,
		SourcePath:        plan.Paths.SnapshotSource,
		Storage:           storage,
		Workspace:         workspace,
		WorkspacePrepared: restoreWorkspacePrepared(workspace),
		Revision:          revision,
		RestorePath:       restorePath,
		Command:           command,
		DryRun:            dryRun,
		Result:            "Pending confirmation",
		Guide:             "docs/restore-drills.md",
	}
}

func newRestoreInspectReport(req *RestoreRequest, plan *Plan, storage string, revision int, pathPrefix string) *restoreInspectReport {
	pathPrefix = strings.TrimSpace(pathPrefix)
	if pathPrefix == "" {
		pathPrefix = "<entire revision>"
	}
	return &restoreInspectReport{
		Label:      req.Label,
		Target:     req.Target(),
		Location:   plan.Config.Location,
		ConfigFile: plan.Paths.ConfigFile,
		SourcePath: plan.Paths.SnapshotSource,
		Storage:    storage,
		Revision:   revision,
		PathPrefix: pathPrefix,
		Guide:      "docs/restore-drills.md",
	}
}

func newRestoreSelectReport(req *RestoreRequest, meta Metadata, plan *Plan, storage, workspace string, workspacePrepared bool, revision int, restorePaths []string) *restoreSelectReport {
	return &restoreSelectReport{
		Label:             req.Label,
		Target:            req.Target(),
		Location:          plan.Config.Location,
		ConfigFile:        plan.Paths.ConfigFile,
		SourcePath:        plan.Paths.SnapshotSource,
		Storage:           storage,
		Workspace:         workspace,
		WorkspacePrepared: workspacePrepared,
		Revision:          revision,
		RestorePaths:      normaliseRestoreSelection(restorePaths),
		RestoreCommands:   buildRestoreRunCommands(meta.ScriptName, req, revision, restorePaths, workspace, restoreStorageRequiresSudo(plan, storage)),
		Guide:             "docs/restore-drills.md",
	}
}

func (r *restorePlanReport) loadAndApplyState(meta Metadata, label, target string) {
	state, err := LoadRunState(meta, label, target)
	r.applyState(state, err)
}

func (r *restorePlanReport) applyState(state *RunState, err error) {
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.StateStatus = "Not found"
			return
		}
		r.StateStatus = fmt.Sprintf("Unreadable (%s)", operator.Message(err))
		return
	}
	r.StateStatus = "Available"
	if state.LastSuccessfulBackupRevision > 0 {
		r.LatestRevision = state.LastSuccessfulBackupRevision
	}
	r.LatestRevisionAt = state.LastSuccessfulBackupAt
}
