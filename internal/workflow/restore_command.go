package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

func HandleRestoreCommand(req *Request, meta Metadata, rt Runtime) (string, error) {
	switch req.RestoreCommand {
	case "plan":
		return handleRestorePlan(req, meta, rt)
	case "prepare":
		return handleRestorePrepare(req, meta, rt)
	default:
		return "", NewRequestError("unsupported restore command %q", req.RestoreCommand)
	}
}

func handleRestorePlan(req *Request, meta Metadata, rt Runtime) (string, error) {
	planner := NewPlanner(meta, rt, nil, nil)
	planReq := configValidationRequest(req, req.Target())
	plan := planner.derivePlan(planReq)
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.applyConfig(cfg, rt)

	report := newRestorePlanReport(req, meta, plan, cfg.Storage)
	report.applyState(loadRunState(meta, req.Source, req.Target()))
	return formatRestorePlan(report), nil
}

func handleRestorePrepare(req *Request, meta Metadata, rt Runtime) (string, error) {
	planner := NewPlanner(meta, rt, nil, nil)
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

	workspace := req.RestoreWorkspace
	if strings.TrimSpace(workspace) == "" {
		workspace = recommendedRestoreWorkspace(plan.SnapshotSource, req.Source, req.Target())
	}
	workspace = filepath.Clean(strings.TrimSpace(workspace))
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
	workspace := recommendedRestoreWorkspace(plan.SnapshotSource, req.Source, req.Target())
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
	writeRestoreSection(&b, "Resolved", []SummaryLine{
		{Label: "Config File", Value: report.ConfigFile},
		{Label: "Source Path", Value: report.SourcePath},
		{Label: "Storage", Value: report.Storage},
		{Label: "Secrets File", Value: report.SecretsFile},
	})
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
	writeRestoreSection(&b, "Safety", []SummaryLine{
		{Label: "Restore Execution", Value: "not performed by this command"},
		{Label: "Copy Back", Value: "inspect restored data first; use rsync --dry-run before live changes"},
		{Label: "Guide", Value: report.DocumentationPath},
	})
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
	writeRestoreSection(&b, "Resolved", []SummaryLine{
		{Label: "Config File", Value: report.ConfigFile},
		{Label: "Source Path", Value: report.SourcePath},
		{Label: "Storage", Value: report.Storage},
		{Label: "Secrets File", Value: report.SecretsFile},
	})
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
	writeRestoreSection(&b, "Safety", []SummaryLine{
		{Label: "Restore Execution", Value: "not performed by this command"},
		{Label: "Copy Back", Value: "manual only; inspect restored data and use rsync --dry-run first"},
		{Label: "Guide", Value: report.Guide},
	})
	return b.String()
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
