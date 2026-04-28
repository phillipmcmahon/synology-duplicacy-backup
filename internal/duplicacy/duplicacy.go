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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

var revisionRegex = regexp.MustCompile(`(?i)\brevision\s+(\d+)\b`)
var deleteRegex = regexp.MustCompile(`(?i)delet(?:ed?|ing)\s+.*?revision`)
var revisionCreatedAtRegex = regexp.MustCompile(`(?i)\brevision\s+(\d+)\b.*?\bcreated at\s+([0-9]{4}-[0-9]{2}-[0-9]{2}\s+[0-9]{2}:[0-9]{2}(?::[0-9]{2})?)`)
var checkRevisionPassRegex = regexp.MustCompile(`(?i)all chunks referenced by snapshot\s+.+?\s+at revision\s+(\d+)\s+exist`)
var checkRevisionFailRegex = regexp.MustCompile(`(?i)some chunks referenced by snapshot\s+.+?\s+at revision\s+(\d+)\s+are missing`)
var checkChunkMissingRegex = regexp.MustCompile(`(?i)chunk\s+[0-9a-f]+\s+referenced by snapshot\s+.+?\s+at revision\s+(\d+)\s+does not exist`)

const DefaultSnapshotID = "data"

type RepositoryProbeState string

const (
	RepositoryAccessible    RepositoryProbeState = "accessible"
	RepositoryUninitialized RepositoryProbeState = "uninitialized"
	RepositoryInaccessible  RepositoryProbeState = "inaccessible"
)

type RevisionInfo struct {
	Revision  int
	CreatedAt time.Time
}

type RevisionCheckResult struct {
	Revision  int
	CreatedAt time.Time
	Result    string
	Message   string
}

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
	IgnoreOwner    bool           // Restore without replaying stored uid/gid metadata.
	Runner         execpkg.Runner // Command runner for external process execution
}

type preferenceEntry struct {
	Name               string            `json:"name"`
	ID                 string            `json:"id"`
	Repository         string            `json:"repository"`
	Storage            string            `json:"storage"`
	Encrypted          bool              `json:"encrypted"`
	NoBackup           bool              `json:"no_backup"`
	NoRestore          bool              `json:"no_restore"`
	NoSavePassword     bool              `json:"no_save_password"`
	NobackupFile       string            `json:"nobackup_file"`
	Keys               map[string]string `json:"keys"`
	Filters            string            `json:"filters"`
	ExcludeByAttribute bool              `json:"exclude_by_attribute"`
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

// NewWorkspaceSetup creates a Duplicacy setup whose repository is the workspace
// itself. Restore drills use this shape so `duplicacy list` and future restore
// commands can be run directly from the drill workspace.
func NewWorkspaceSetup(workspace, storage string, dryRun bool, runner execpkg.Runner) *Setup {
	return &Setup{
		WorkRoot:       workspace,
		DuplicacyRoot:  workspace,
		DuplicacyDir:   filepath.Join(workspace, ".duplicacy"),
		PrefsFile:      filepath.Join(workspace, ".duplicacy", "preferences"),
		FilterFile:     filepath.Join(workspace, ".duplicacy", "filters"),
		RepositoryPath: workspace,
		BackupTarget:   storage,
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
	var keys map[string]string
	if sec != nil {
		keys = sec.Keys
	}

	prefs := []preferenceEntry{{
		Name:               "default",
		ID:                 DefaultSnapshotID,
		Repository:         s.RepositoryPath,
		Storage:            s.BackupTarget,
		Encrypted:          false,
		NoBackup:           false,
		NoRestore:          false,
		NoSavePassword:     false,
		NobackupFile:       "",
		Keys:               keys,
		Filters:            "",
		ExcludeByAttribute: false,
	}}
	data, err := json.MarshalIndent(prefs, "", "    ")
	if err != nil {
		return apperrors.NewBackupError("write-preferences", fmt.Errorf("failed to encode preferences file: %w", err), "path", s.PrefsFile)
	}
	data = append(data, '\n')

	if s.DryRun {
		return nil
	}

	if err := os.WriteFile(s.PrefsFile, data, 0660); err != nil {
		return apperrors.NewBackupError("write-preferences", fmt.Errorf("failed to write preferences file: %w", err), "path", s.PrefsFile)
	}
	return nil
}

// secretPattern matches JSON key-value pairs whose keys look credential-like
// and replaces the value with "REDACTED".
var secretPattern = regexp.MustCompile(`(?i)("[^"]*(?:secret|password|key|token|_id)[^"]*":\s*)"[^"]*"`)

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

// ProbeRepository runs the same read-only repository validation command as
// ValidateRepo, but returns a richer readiness classification for callers that
// need to distinguish an uninitialised repository from a broader access
// failure.
func (s *Setup) ProbeRepository() (RepositoryProbeState, string, error) {
	if s.DryRun {
		return RepositoryAccessible, "", nil
	}

	stdout, stderr, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", "list", "-files")
	combined := stdout + stderr
	if err == nil {
		return RepositoryAccessible, combined, nil
	}
	if looksUninitializedRepositoryOutput(combined) {
		return RepositoryUninitialized, combined, nil
	}
	return RepositoryInaccessible, combined, apperrors.NewPruneError("validate-repo", fmt.Errorf("repository validation failed"))
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

// GetLatestRevision returns the highest revision number reported by
// `duplicacy list`. The combined command output is returned for logging.
func (s *Setup) GetLatestRevision() (int, string, error) {
	if s.DryRun {
		return 0, "", nil
	}

	stdout, stderr, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", "list")
	combined := stdout + stderr
	if err != nil {
		return 0, combined, apperrors.NewPruneError("revision-latest", fmt.Errorf("failed to list revisions for latest revision inspection"))
	}

	latest := 0
	for _, match := range revisionRegex.FindAllStringSubmatch(combined, -1) {
		if len(match) > 1 {
			if n, convErr := strconv.Atoi(match[1]); convErr == nil && n > latest {
				latest = n
			}
		}
	}
	return latest, combined, nil
}

func (s *Setup) GetLatestRevisionInfo() (*RevisionInfo, string, error) {
	if s.DryRun {
		return nil, "", nil
	}

	revisions, combined, err := s.ListVisibleRevisions()
	if err != nil {
		return nil, combined, err
	}
	if len(revisions) == 0 {
		return nil, combined, nil
	}
	latest := revisions[0]
	return &latest, combined, nil
}

func (s *Setup) ListVisibleRevisions() ([]RevisionInfo, string, error) {
	if s.DryRun {
		return nil, "", nil
	}

	stdout, stderr, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", "list")
	combined := stdout + stderr
	if err != nil {
		return nil, combined, apperrors.NewPruneError("revision-list", fmt.Errorf("failed to list visible revisions for integrity inspection"))
	}
	return parseVisibleRevisions(combined), combined, nil
}

func (s *Setup) ListRevisionFiles(revision int) (string, error) {
	if s.DryRun {
		return "", nil
	}

	stdout, stderr, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", "list", "-files", "-r", strconv.Itoa(revision))
	combined := stdout + stderr
	if err != nil {
		return combined, apperrors.NewBackupError("restore-list-files", fmt.Errorf("failed to list files for revision %d", revision))
	}
	return combined, nil
}

func (s *Setup) RestoreRevision(revision int, relativePath string) (string, error) {
	return s.RestoreRevisionContext(context.Background(), revision, relativePath)
}

func (s *Setup) RestoreRevisionContext(ctx context.Context, revision int, relativePath string) (string, error) {
	if s.DryRun {
		return "", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	args := []string{"restore", "-r", strconv.Itoa(revision), "-stats"}
	if s.IgnoreOwner {
		args = append(args, "-ignore-owner")
	}
	if strings.TrimSpace(relativePath) != "" {
		args = append(args, "--", relativePath)
	}
	stdout, stderr, err := s.Runner.RunInDir(ctx, s.DuplicacyRoot, "duplicacy", args...)
	combined := stdout + stderr
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			cause := ctx.Err()
			if cause == nil {
				cause = err
			}
			return combined, apperrors.NewBackupError("restore-run", fmt.Errorf("restore interrupted: %w", cause))
		}
		return combined, apperrors.NewBackupError("restore-run", fmt.Errorf("failed to restore revision %d", revision))
	}
	return combined, nil
}

func (s *Setup) CheckVisibleRevisions() ([]RevisionCheckResult, string, error) {
	if s.DryRun {
		return nil, "", nil
	}

	stdout, stderr, err := s.Runner.RunInDir(context.Background(), s.DuplicacyRoot, "duplicacy", "check", "-persist")
	combined := stdout + stderr
	results := parseRevisionCheckResults(combined)
	if err != nil && len(results) == 0 {
		return nil, combined, apperrors.NewPruneError("revision-check", fmt.Errorf("failed to complete the storage integrity check"))
	}
	return results, combined, nil
}

func parseRevisionCreatedAt(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported revision timestamp %q", value)
}

func parseVisibleRevisions(output string) []RevisionInfo {
	if output == "" {
		return nil
	}

	seen := make(map[int]RevisionInfo)
	for _, match := range revisionCreatedAtRegex.FindAllStringSubmatch(output, -1) {
		if len(match) < 3 {
			continue
		}
		revision, convErr := strconv.Atoi(match[1])
		if convErr != nil {
			continue
		}
		createdAt, parseErr := parseRevisionCreatedAt(match[2])
		if parseErr != nil {
			continue
		}
		seen[revision] = RevisionInfo{Revision: revision, CreatedAt: createdAt}
	}
	for _, match := range revisionRegex.FindAllStringSubmatch(output, -1) {
		if len(match) < 2 {
			continue
		}
		revision, convErr := strconv.Atoi(match[1])
		if convErr != nil {
			continue
		}
		if _, ok := seen[revision]; !ok {
			seen[revision] = RevisionInfo{Revision: revision}
		}
	}

	revisions := make([]RevisionInfo, 0, len(seen))
	for _, revision := range seen {
		revisions = append(revisions, revision)
	}
	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].Revision > revisions[j].Revision
	})
	return revisions
}

// looksUninitializedRepositoryOutput is a best-effort heuristic for Duplicacy's
// human-readable CLI output. Duplicacy does not expose a structured readiness
// code here, so this list should be updated if future Duplicacy versions change
// their uninitialised-storage wording.
func looksUninitializedRepositoryOutput(output string) bool {
	if output == "" {
		return false
	}
	lower := strings.ToLower(output)
	patterns := []string{
		"not initialized",
		"not been initialized",
		"initialize the storage",
		"initialize this storage",
		"storage has not been initialized",
		"repository has not been initialized",
		"snapshot directory not found",
		"snapshots directory not found",
		"snapshots/ does not exist",
		"snapshots directory does not exist",
	}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	if strings.Contains(lower, "snapshots/") && strings.Contains(lower, "not found") {
		return true
	}
	return false
}

func parseRevisionCheckResults(output string) []RevisionCheckResult {
	if output == "" {
		return nil
	}

	results := make(map[int]RevisionCheckResult)
	for _, match := range checkRevisionPassRegex.FindAllStringSubmatch(output, -1) {
		if len(match) < 2 {
			continue
		}
		revision, convErr := strconv.Atoi(match[1])
		if convErr != nil {
			continue
		}
		if existing, ok := results[revision]; ok && existing.Result == "fail" {
			continue
		}
		results[revision] = RevisionCheckResult{
			Revision: revision,
			Result:   "pass",
			Message:  "Validated",
		}
	}
	for _, match := range checkRevisionFailRegex.FindAllStringSubmatch(output, -1) {
		if len(match) < 2 {
			continue
		}
		revision, convErr := strconv.Atoi(match[1])
		if convErr != nil {
			continue
		}
		results[revision] = RevisionCheckResult{
			Revision: revision,
			Result:   "fail",
			Message:  "Missing chunks",
		}
	}
	for _, match := range checkChunkMissingRegex.FindAllStringSubmatch(output, -1) {
		if len(match) < 2 {
			continue
		}
		revision, convErr := strconv.Atoi(match[1])
		if convErr != nil {
			continue
		}
		if existing, ok := results[revision]; ok && existing.Result == "fail" {
			continue
		}
		results[revision] = RevisionCheckResult{
			Revision: revision,
			Result:   "fail",
			Message:  "Missing chunks",
		}
	}

	parsed := make([]RevisionCheckResult, 0, len(results))
	for _, result := range results {
		parsed = append(parsed, result)
	}
	sort.Slice(parsed, func(i, j int) bool {
		return parsed[i].Revision > parsed[j].Revision
	})
	return parsed
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
