package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

const defaultRestoreWorkspaceRoot = "/volume1/restore-drills"

func resolvedRestoreWorkspace(req *RestoreRequest, plan *Plan, deps RestoreDeps) string {
	workspace := req.Workspace
	if strings.TrimSpace(workspace) == "" {
		workspace = recommendedRestoreWorkspace(req.Label, req.Target(), deps)
	}
	return filepath.Clean(strings.TrimSpace(workspace))
}

func resolvedRestoreRunWorkspace(req *RestoreRequest, rt Runtime, plan *Plan, storage string, sec *secrets.Secrets, deps RestoreDeps) (string, error) {
	if strings.TrimSpace(req.Workspace) != "" {
		return resolvedRestoreWorkspace(req, plan, deps), nil
	}
	revision, err := findRestoreRevision(req, rt, plan, storage, sec, deps)
	if err != nil {
		return "", err
	}
	return recommendedRestoreWorkspaceForRevision(req.Label, req.Target(), revision, deps), nil
}

func resolvedRestoreSelectWorkspace(req *RestoreRequest, plan *Plan, revision duplicacy.RevisionInfo, deps RestoreDeps) string {
	if strings.TrimSpace(req.Workspace) != "" {
		return resolvedRestoreWorkspace(req, plan, deps)
	}
	return recommendedRestoreWorkspaceForRevision(req.Label, req.Target(), revision, deps)
}

func recommendedRestoreWorkspace(label, target string, deps RestoreDeps) string {
	timestamp := deps.Now().Local().Format("20060102-150405")
	return filepath.Join(restoreWorkspaceRoot(deps), fmt.Sprintf("%s-%s-%s", label, target, timestamp))
}

func recommendedRestoreWorkspaceForRevision(label, target string, revision duplicacy.RevisionInfo, deps RestoreDeps) string {
	timestamp := deps.Now().Local().Format("20060102-150405")
	if !revision.CreatedAt.IsZero() {
		timestamp = revision.CreatedAt.Local().Format("20060102-150405")
	}
	return filepath.Join(restoreWorkspaceRoot(deps), fmt.Sprintf("%s-%s-%s-rev%d", label, target, timestamp, revision.Revision))
}

func restoreWorkspaceRoot(deps RestoreDeps) string {
	if strings.TrimSpace(deps.RestoreWorkspaceRoot) == "" {
		return defaultRestoreWorkspaceRoot
	}
	return filepath.Clean(strings.TrimSpace(deps.RestoreWorkspaceRoot))
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

func restoreWorkspaceForRead(req *RestoreRequest, plan *Plan, rt Runtime, allowTemporary bool, deps RestoreDeps) (string, string, func(), error) {
	if strings.TrimSpace(req.Workspace) != "" {
		workspace := resolvedRestoreWorkspace(req, plan, deps)
		if err := validateRestoreWorkspace(workspace, plan.SnapshotSource); err != nil {
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
	if err := validateRestoreWorkspace(workspace, plan.SnapshotSource); err != nil {
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
	return dup.SetPermissions()
}

func restoreWorkspacePrepared(workspace string) bool {
	info, err := os.Stat(filepath.Join(workspace, ".duplicacy", "preferences"))
	return err == nil && !info.IsDir()
}
