// Package duplicacy wraps the duplicacy CLI for backup, prune, and list operations.
// It manages preferences file generation, filter files, and command execution.
//
// All external command execution is delegated to an [exec.Runner] so that
// tests can substitute a [exec.MockRunner] and verify behaviour without
// requiring the real duplicacy binary.
//
// Functions return structured error types from [errors] with rich context
// instead of logging directly.  The coordinator is responsible for all
// operator-facing output.
package duplicacy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

var revisionRegex = regexp.MustCompile(`(?i)\brevision\s+(\d+)\b`)
var deleteRegex = regexp.MustCompile(`(?i)delet(?:ed?|ing)\s+.*?revision`)

// Setup represents a duplicacy working environment.
type Setup struct {
	WorkRoot       string // Top-level temp work dir
	DuplicacyRoot  string // Where duplicacy runs from (contains .duplicacy/)
	DuplicacyDir   string // .duplicacy directory
	PrefsFile      string // .duplicacy/preferences
	FilterFile     string // .duplicacy/filters
	RepositoryPath string // Snapshot source or target
	BackupTarget   string // Storage destination
	DryRun         bool
	Runner         execpkg.Runner // Command runner for external process execution
}

// NewSetup creates a new duplicacy working environment.
// The runner parameter is used for all external command execution (duplicacy CLI).
func NewSetup(workRoot, repositoryPath, backupTarget string, dryRun bool, runner execpkg.Runner) *Setup {
	duplicacyRoot := filepath.Join(workRoot, "duplicacy")
	duplicacyDir := filepath.Join(duplicacyRoot, ".duplicacy")

	return &Setup{
		WorkRoot:       workRoot,
		DuplicacyRoot:  duplicacyRoot,
		DuplicacyDir:   duplicacyDir,
		PrefsFile:      filepath.Join(duplicacyDir, "preferences"),
		FilterFile:     filepath.Join(duplicacyDir, "filters"),
		RepositoryPath: repositoryPath,
		BackupTarget:   backupTarget,
		DryRun:         dryRun,
		Runner:         runner,
	}
}

// CreateDirs creates the duplicacy working directories.
func (s *Setup) CreateDirs() error {
	if s.DryRun {
		return nil
	}

	if err := os.MkdirAll(s.DuplicacyDir, 0770); err != nil {
		return apperrors.NewBackupError("create-dirs", fmt.Errorf("failed to create duplicacy directories: %w", err), "path", s.DuplicacyDir)
	}
	return nil
}

// WritePreferences writes the duplicacy preferences JSON file.
func (s *Setup) WritePreferences(sec *secrets.Secrets) error {
	keysContent := "null"
	if sec != nil {
		keysContent = fmt.Sprintf(`{
            "storj_s3_id": "%s",
            "storj_s3_secret": "%s"
}`, sec.StorjS3ID, sec.StorjS3Secret)
	}

	json := fmt.Sprintf(`[
    {
        "name": "storj",
        "id": "data",
        "repository": "%s",
        "storage": "%s",
        "encrypted": false,
        "no_backup": false,
        "no_restore": false,
        "no_save_password": false,
        "nobackup_file": "",
        "keys": %s,
        "filters": "",
        "exclude_by_attribute": false
    }
]
`, s.RepositoryPath, s.BackupTarget, keysContent)

	if s.DryRun {
		return nil
	}

	if err := os.WriteFile(s.PrefsFile, []byte(json), 0660); err != nil {
		return apperrors.NewBackupError("write-preferences", fmt.Errorf("failed to write preferences file: %w", err), "path", s.PrefsFile)
	}
	return nil
}

// secretPattern matches JSON key-value pairs whose keys contain "secret", "id",
// "password", or "key" (case-insensitive) and replaces the value with "REDACTED".
var secretPattern = regexp.MustCompile(`(?i)("(?:storj_s3_id|storj_s3_secret|password|key)":\s*)"[^"]*"`)

// redactSecrets replaces sensitive credential values in a JSON-like string
// with "REDACTED" so they are not leaked in log output.
func redactSecrets(s string) string {
	return secretPattern.ReplaceAllString(s, `${1}"REDACTED"`)
}

// WriteFilters writes the duplicacy filter file.
// Returns nil if filter is empty (no file is written).
func (s *Setup) WriteFilters(filter string) error {
	if filter == "" {
		return nil
	}

	if s.DryRun {
		return nil
	}

	if err := os.WriteFile(s.FilterFile, []byte(filter+"\n"), 0660); err != nil {
		return apperrors.NewBackupError("write-filters", fmt.Errorf("failed to write filter file: %w", err), "path", s.FilterFile)
	}

	return nil
}

// SetPermissions sets directory (770) and file (660) permissions on the work directory.
func (s *Setup) SetPermissions() error {
	if s.DryRun {
		return nil
	}

	err := filepath.Walk(s.DuplicacyRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.Chmod(path, 0770)
		}
		return os.Chmod(path, 0660)
	})
	if err != nil {
		return apperrors.NewBackupError("set-permissions", fmt.Errorf("failed to set permissions: %w", err), "path", s.DuplicacyRoot)
	}
	return nil
}

// RunBackup executes `duplicacy backup -stats -threads N`.
// Stdout and stderr from the duplicacy command are returned for the
// coordinator to display.
func (s *Setup) RunBackup(threads int) (string, string, error) {
	args := []string{"backup", "-stats", "-threads", strconv.Itoa(threads)}

	if s.DryRun {
		return "", "", nil
	}

	stdout, stderr, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", args...)
	if err != nil {
		return stdout, stderr, apperrors.NewBackupError("run", fmt.Errorf("backup command failed: %w", err), "threads", strconv.Itoa(threads))
	}
	return stdout, stderr, nil
}

// ValidateRepo runs `duplicacy list -files` to verify the repo is valid.
func (s *Setup) ValidateRepo() error {
	if s.DryRun {
		return nil
	}

	_, _, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", "list", "-files")
	if err != nil {
		return apperrors.NewPruneError("validate-repo", fmt.Errorf("repository validation failed - may need initialization"))
	}
	return nil
}

// GetTotalRevisionCount returns the number of unique revisions via `duplicacy list`.
// On error it returns 0 and a structured error; the combined output is returned
// for the coordinator to log if needed.
func (s *Setup) GetTotalRevisionCount() (int, string, error) {
	if s.DryRun {
		return 0, "", nil
	}

	stdout, stderr, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", "list")
	combined := stdout + stderr
	if err != nil {
		return 0, combined, apperrors.NewPruneError("revision-count", fmt.Errorf("failed to list revisions for percentage calculation (fail-closed)"))
	}

	// Count unique revision numbers
	seen := make(map[int]bool)
	for _, match := range revisionRegex.FindAllStringSubmatch(combined, -1) {
		if len(match) > 1 {
			if n, err := strconv.Atoi(match[1]); err == nil {
				seen[n] = true
			}
		}
	}

	return len(seen), combined, nil
}

// PrunePreview holds the results of a safe prune dry-run preview.
type PrunePreview struct {
	DeleteCount         int
	TotalRevisions      int
	DeletePercent       int // Approximate (truncated) – for display only
	PercentEnforced     bool
	RevisionCountFailed bool   // True when revision listing failed
	Output              string // Combined stdout+stderr from the preview command
	RevisionOutput      string // Combined stdout+stderr from revision listing
}

// ExceedsPercent reports whether the deletion ratio exceeds maxPercent
// using cross-multiplication to avoid integer-division truncation.
// Returns false when percentage enforcement is not active.
func (p *PrunePreview) ExceedsPercent(maxPercent int) bool {
	if !p.PercentEnforced || p.TotalRevisions <= 0 {
		return false
	}
	// deleteCount/totalRevisions > maxPercent/100
	// ⟺ deleteCount*100 > maxPercent*totalRevisions
	return p.DeleteCount*100 > maxPercent*p.TotalRevisions
}

// SafePrunePreview runs a prune dry-run and counts deletions.
// The combined output from both the prune preview and revision listing
// is included in the returned PrunePreview for the coordinator to display.
func (s *Setup) SafePrunePreview(pruneArgs []string, minTotalForPercent int) (*PrunePreview, error) {
	if s.DryRun {
		return &PrunePreview{}, nil
	}

	args := append([]string{"prune"}, pruneArgs...)
	args = append(args, "-dry-run")

	stdout, stderr, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", args...)
	combined := stdout + stderr
	if err != nil {
		return nil, apperrors.NewPruneError("safe-preview", fmt.Errorf("safe prune preview failed"), "args", strings.Join(args, " "))
	}

	// Count deletion lines
	deleteCount := len(deleteRegex.FindAllString(combined, -1))

	// Get total revision count
	totalCount, revOutput, revErr := s.GetTotalRevisionCount()

	preview := &PrunePreview{
		DeleteCount:    deleteCount,
		Output:         combined,
		RevisionOutput: revOutput,
	}

	if revErr != nil {
		preview.RevisionCountFailed = true
	} else {
		preview.TotalRevisions = totalCount
		if totalCount >= minTotalForPercent && totalCount > 0 {
			preview.DeletePercent = deleteCount * 100 / totalCount // truncated; display only
			preview.PercentEnforced = true
		}
	}

	return preview, nil
}

// RunPrune executes `duplicacy prune` with the given arguments.
// Stdout and stderr are returned for the coordinator to display.
func (s *Setup) RunPrune(pruneArgs []string) (string, string, error) {
	args := append([]string{"prune"}, pruneArgs...)

	if s.DryRun {
		return "", "", nil
	}

	stdout, stderr, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", args...)
	if err != nil {
		return stdout, stderr, apperrors.NewPruneError("run", fmt.Errorf("prune command failed: %w", err), "args", strings.Join(args, " "))
	}
	return stdout, stderr, nil
}

// RunCleanupStorage executes `duplicacy prune -exhaustive -exclusive`.
// Stdout and stderr are returned for the coordinator to display.
func (s *Setup) RunCleanupStorage() (string, string, error) {
	args := []string{"prune", "-exhaustive", "-exclusive"}

	if s.DryRun {
		return "", "", nil
	}

	stdout, stderr, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", args...)
	if err != nil {
		return stdout, stderr, apperrors.NewPruneError("cleanup-storage", fmt.Errorf("storage cleanup command failed: %w", err))
	}
	return stdout, stderr, nil
}

// Cleanup removes the work root directory.  Returns an error if removal
// fails; returns nil on success or if WorkRoot is empty.
func (s *Setup) Cleanup() error {
	if s.WorkRoot == "" {
		return nil
	}
	if s.DryRun {
		return nil
	}
	if err := os.RemoveAll(s.WorkRoot); err != nil {
		return fmt.Errorf("failed to remove work directory %s: %w", s.WorkRoot, err)
	}
	return nil
}
