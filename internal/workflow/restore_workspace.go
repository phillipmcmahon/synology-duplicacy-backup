package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

const defaultRestoreWorkspaceRoot = "/volume1/restore-drills"
const defaultRestoreWorkspaceTemplate = "{label}-{target}-{snapshot_timestamp}-rev{revision}"
const defaultRestorePlanWorkspaceTemplate = "{label}-{target}-{run_timestamp}"

var unsafeRestoreWorkspaceSegmentPattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func resolvedRestoreWorkspace(req *RestoreRequest, plan *Plan, deps RestoreDeps) string {
	workspace := req.Workspace
	if strings.TrimSpace(workspace) == "" {
		workspace = recommendedRestoreWorkspaceRoot(req.Label, req.Target(), restoreWorkspaceRootForRequest(req, deps), req.WorkspaceTemplate, deps)
	}
	return filepath.Clean(strings.TrimSpace(workspace))
}

func resolvedRestoreRunWorkspace(req *RestoreRequest, rt Runtime, plan *Plan, storage string, sec *secrets.Secrets, deps RestoreDeps) (string, error) {
	// Defence in depth: restore run normally enters through handleRestoreCommand,
	// but select-action handoffs also resolve a run workspace directly.
	if err := validateRestoreWorkspaceSelection(req); err != nil {
		return "", err
	}
	// Keep pure request-shape validation before filesystem checks so malformed
	// requests fail before we inspect operator-managed roots.
	if err := validateRestoreWorkspaceRoot(req); err != nil {
		return "", err
	}
	if strings.TrimSpace(req.Workspace) != "" {
		return resolvedRestoreWorkspace(req, plan, deps), nil
	}
	revision, err := findRestoreRevision(req, rt, plan, storage, sec, deps)
	if err != nil {
		return "", err
	}
	workspace, err := recommendedRestoreWorkspaceForRevisionRoot(req.Label, req.Target(), revision, restoreWorkspaceRootForRequest(req, deps), req.WorkspaceTemplate, deps)
	if err != nil {
		return "", err
	}
	return workspace, nil
}

func resolvedRestoreSelectWorkspace(req *RestoreRequest, plan *Plan, revision duplicacy.RevisionInfo, deps RestoreDeps) (string, error) {
	if strings.TrimSpace(req.Workspace) != "" {
		return resolvedRestoreWorkspace(req, plan, deps), nil
	}
	return recommendedRestoreWorkspaceForRevisionRoot(req.Label, req.Target(), revision, restoreWorkspaceRootForRequest(req, deps), req.WorkspaceTemplate, deps)
}

func recommendedRestoreWorkspaceRoot(label, target string, root string, workspaceTemplate string, deps RestoreDeps) string {
	now := deps.Now()
	template := strings.TrimSpace(workspaceTemplate)
	if template == "" {
		template = defaultRestorePlanWorkspaceTemplate
	}
	name, err := renderRestoreWorkspaceTemplate(template, restoreWorkspaceTemplateValues{
		Label:             label,
		Target:            target,
		SnapshotTimestamp: "<restore-point-timestamp>",
		Revision:          "<revision>",
		RunTimestamp:      restoreWorkspaceTimestamp(now),
	})
	if err != nil {
		// Plan output should remain advisory even if a configured template is invalid;
		// execution paths validate templates and return the actionable error.
		name = fmt.Sprintf("%s-%s-%s", safeRestoreWorkspaceSegment(label), safeRestoreWorkspaceSegment(target), restoreWorkspaceTimestamp(now))
	}
	return filepath.Join(root, name)
}

func recommendedRestoreWorkspaceForRevisionRoot(label, target string, revision duplicacy.RevisionInfo, root string, workspaceTemplate string, deps RestoreDeps) (string, error) {
	now := deps.Now()
	snapshotTimestamp := restoreWorkspaceTimestamp(now)
	if !revision.CreatedAt.IsZero() {
		snapshotTimestamp = restoreWorkspaceTimestamp(revision.CreatedAt)
	}
	template := strings.TrimSpace(workspaceTemplate)
	if template == "" {
		template = defaultRestoreWorkspaceTemplate
	}
	name, err := renderRestoreWorkspaceTemplate(template, restoreWorkspaceTemplateValues{
		Label:             label,
		Target:            target,
		SnapshotTimestamp: snapshotTimestamp,
		Revision:          fmt.Sprintf("%d", revision.Revision),
		RunTimestamp:      restoreWorkspaceTimestamp(now),
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name), nil
}

func restoreWorkspaceRoot(deps RestoreDeps) string {
	// This is the deps fallback half of restoreWorkspaceRootForRequest; request
	// values win when the operator supplies --workspace-root.
	if strings.TrimSpace(deps.RestoreWorkspaceRoot) == "" {
		return defaultRestoreWorkspaceRoot
	}
	return filepath.Clean(strings.TrimSpace(deps.RestoreWorkspaceRoot))
}

func restoreWorkspaceRootForRequest(req *RestoreRequest, deps RestoreDeps) string {
	if req != nil && strings.TrimSpace(req.WorkspaceRoot) != "" {
		return filepath.Clean(strings.TrimSpace(req.WorkspaceRoot))
	}
	return restoreWorkspaceRoot(deps)
}

func validateRestoreWorkspaceSelection(req *RestoreRequest) error {
	if req == nil {
		return nil
	}
	if strings.TrimSpace(req.Workspace) != "" {
		if strings.TrimSpace(req.WorkspaceRoot) != "" {
			return NewRequestError("--workspace and --workspace-root cannot be used together")
		}
		if strings.TrimSpace(req.WorkspaceTemplate) != "" {
			return NewRequestError("--workspace and --workspace-template cannot be used together")
		}
	}
	if strings.TrimSpace(req.WorkspaceRoot) != "" && !filepath.IsAbs(filepath.Clean(strings.TrimSpace(req.WorkspaceRoot))) {
		return NewRequestError("--workspace-root must be an absolute path: %s", req.WorkspaceRoot)
	}
	if err := validateRestoreWorkspaceTemplate(req.WorkspaceTemplate); err != nil {
		return err
	}
	return nil
}

func applyRestoreConfigDefaults(req *RestoreRequest, cfg *config.Config) {
	if req == nil || cfg == nil {
		return
	}
	if strings.TrimSpace(req.Workspace) != "" {
		return
	}
	if strings.TrimSpace(req.WorkspaceRoot) == "" {
		req.WorkspaceRoot = cfg.RestoreWorkspaceRoot
	}
	if strings.TrimSpace(req.WorkspaceTemplate) == "" {
		req.WorkspaceTemplate = cfg.RestoreWorkspaceTemplate
	}
}

type restoreWorkspaceTemplateValues struct {
	Label             string
	Target            string
	SnapshotTimestamp string
	Revision          string
	RunTimestamp      string
}

func restoreWorkspaceTimestamp(value time.Time) string {
	return value.Local().Format("20060102-150405")
}

func validateRestoreWorkspaceTemplate(template string) error {
	template = strings.TrimSpace(template)
	if template == "" {
		return nil
	}
	_, err := renderRestoreWorkspaceTemplate(template, restoreWorkspaceTemplateValues{
		Label:             "label",
		Target:            "target",
		SnapshotTimestamp: "20260424-070000",
		Revision:          "2403",
		RunTimestamp:      "20260428-120000",
	})
	return err
}

func renderRestoreWorkspaceTemplate(template string, values restoreWorkspaceTemplateValues) (string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", NewRequestError("restore workspace template must not be empty")
	}
	replacements := map[string]string{
		"label":              safeRestoreWorkspaceSegment(values.Label),
		"target":             safeRestoreWorkspaceSegment(values.Target),
		"snapshot_timestamp": safeRestoreWorkspaceSegment(values.SnapshotTimestamp),
		"revision":           safeRestoreWorkspaceSegment(values.Revision),
		"run_timestamp":      safeRestoreWorkspaceSegment(values.RunTimestamp),
	}

	var out strings.Builder
	for i := 0; i < len(template); {
		if template[i] != '{' {
			out.WriteByte(template[i])
			i++
			continue
		}
		end := strings.IndexByte(template[i+1:], '}')
		if end < 0 {
			return "", NewRequestError("restore workspace template has an unclosed variable: %s", template)
		}
		name := template[i+1 : i+1+end]
		value, ok := replacements[name]
		if !ok {
			return "", NewRequestError("restore workspace template uses unsupported variable {%s}; supported variables are {label}, {target}, {snapshot_timestamp}, {revision}, and {run_timestamp}", name)
		}
		out.WriteString(value)
		i += end + 2
	}

	name := strings.TrimSpace(out.String())
	if name == "" || name == "." || name == ".." {
		return "", NewRequestError("restore workspace template must produce a folder name")
	}
	if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return "", NewRequestError("restore workspace template must produce one folder name, not a path: %s", template)
	}
	return name, nil
}

func safeRestoreWorkspaceSegment(value string) string {
	value = strings.TrimSpace(value)
	value = unsafeRestoreWorkspaceSegmentPattern.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "unknown"
	}
	return value
}

func validateRestoreWorkspaceRoot(req *RestoreRequest) error {
	if req == nil || strings.TrimSpace(req.WorkspaceRoot) == "" {
		return nil
	}
	root := filepath.Clean(strings.TrimSpace(req.WorkspaceRoot))
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return NewRequestError("--workspace-root does not exist: %s\nhint: create it via DSM as a shared folder or with mkdir -p", root)
		}
		return fmt.Errorf("restore workspace root is not accessible: %w", err)
	}
	if !info.IsDir() {
		return NewRequestError("--workspace-root must be a directory: %s", root)
	}
	return nil
}

func validateRestoreWorkspace(workspace, sourcePath string) error {
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	if workspace == "" || workspace == "." {
		return NewRequestError("--workspace must be an absolute path")
	}
	if !filepath.IsAbs(workspace) {
		return NewRequestError("--workspace must be an absolute path: %s", workspace)
	}
	if strings.TrimSpace(sourcePath) == "" {
		return nil
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
			return os.MkdirAll(workspace, 0700)
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

func restoreWorkspaceForRead(req *RestoreRequest, plan *Plan, rt Runtime, allowTemporary bool, deps RestoreDeps) (string, string, func(), error) {
	if strings.TrimSpace(req.Workspace) != "" {
		workspace := resolvedRestoreWorkspace(req, plan, deps)
		if err := validateRestoreWorkspace(workspace, plan.Paths.SnapshotSource); err != nil {
			return "", "", func() {}, err
		}
		if !restoreWorkspacePrepared(workspace) {
			return "", "", func() {}, NewRequestError("restore listing with --workspace requires a workspace containing .duplicacy/preferences; omit --workspace to use a temporary read-only listing workspace")
		}
		return workspace, "prepared", func() {}, nil
	}
	if !allowTemporary {
		return "", "", func() {}, NewRequestError("--workspace is required")
	}
	workspace, cleanup, err := temporaryRestoreWorkspace(plan, rt)
	if err != nil {
		return "", "", func() {}, err
	}
	return workspace, "temporary", cleanup, nil
}

func temporaryRestoreWorkspace(plan *Plan, rt Runtime) (string, func(), error) {
	base := rt.TempDir
	tempParent := os.TempDir()
	if base != nil && strings.TrimSpace(base()) != "" {
		tempParent = base()
	}
	workspace, err := os.MkdirTemp(tempParent, "duplicacy-restore-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("restore temporary workspace cannot be created: %w", err)
	}
	if err := validateRestoreWorkspace(workspace, plan.Paths.SnapshotSource); err != nil {
		_ = os.RemoveAll(workspace)
		return "", func() {}, err
	}
	return workspace, func() { _ = os.RemoveAll(workspace) }, nil
}

func findRestoreRevision(req *RestoreRequest, rt Runtime, plan *Plan, storage string, sec *secrets.Secrets, deps RestoreDeps) (duplicacy.RevisionInfo, error) {
	workspace, cleanup, err := temporaryRestoreWorkspace(plan, rt)
	if err != nil {
		return duplicacy.RevisionInfo{}, err
	}
	defer cleanup()
	if err := writeRestoreWorkspacePreferences(workspace, storage, sec); err != nil {
		return duplicacy.RevisionInfo{}, err
	}
	dup := duplicacy.NewWorkspaceSetup(workspace, storage, false, deps.NewRunner())
	revisions, _, err := dup.ListVisibleRevisions()
	if err != nil {
		return duplicacy.RevisionInfo{}, err
	}
	for _, revision := range revisions {
		if revision.Revision == req.Revision {
			return revision, nil
		}
	}
	return duplicacy.RevisionInfo{}, NewRequestError("revision %d was not found in the visible revision list", req.Revision)
}

func prepareRestoreWorkspace(workspace, storage string, sec *secrets.Secrets) error {
	if restoreWorkspacePrepared(workspace) {
		return nil
	}
	if err := ensureRestoreWorkspaceReady(workspace); err != nil {
		return err
	}
	return writeRestoreWorkspacePreferences(workspace, storage, sec)
}

func writeRestoreWorkspacePreferences(workspace, storage string, sec *secrets.Secrets) error {
	dup := duplicacy.NewWorkspaceSetup(workspace, storage, false, nil)
	if err := dup.CreateDirs(); err != nil {
		return err
	}
	if err := dup.WritePreferences(sec); err != nil {
		return err
	}
	if err := dup.SetPermissions(); err != nil {
		return err
	}
	return os.Chmod(workspace, 0700)
}

func restoreWorkspacePrepared(workspace string) bool {
	info, err := os.Stat(filepath.Join(workspace, ".duplicacy", "preferences"))
	if err != nil {
		// Missing or unreadable preferences both mean "not prepared" here. The
		// subsequent prepare step surfaces any concrete permission/filesystem
		// error at the point where it can describe the attempted action.
		return false
	}
	return !info.IsDir()
}

func restoreWorkspaceProfileOwnership(meta Metadata, workspace string) error {
	if !meta.HasProfileOwner || strings.TrimSpace(workspace) == "" {
		return nil
	}
	return filepath.WalkDir(workspace, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("restore workspace ownership walk failed at %s: %w", path, err)
		}
		return chownProfilePath(meta, path)
	})
}
