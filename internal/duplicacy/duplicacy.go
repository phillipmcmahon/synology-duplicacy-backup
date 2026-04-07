// Package duplicacy wraps the duplicacy CLI for backup, prune, and list operations.
// It manages preferences file generation, filter files, and command execution.
package duplicacy

import (
        "bytes"
        "fmt"
        "os"
        "os/exec"
        "path/filepath"
        "regexp"
        "strconv"
        "strings"

        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

var revisionRegex = regexp.MustCompile(`(?i)\brevision\s+(\d+)\b`)
var deleteRegex = regexp.MustCompile(`(?i)delet(?:ed?|ing)\s+revision`)

// Setup represents a duplicacy working environment.
type Setup struct {
        WorkRoot       string // Top-level temp work dir
        DuplicacyRoot  string // Where duplicacy runs from (contains .duplicacy/)
        DuplicacyDir   string // .duplicacy directory
        PrefsFile      string // .duplicacy/preferences
        FilterFile     string // .duplicacy/filters
        RepositoryPath string // Snapshot source or target
        BackupTarget   string // Storage destination
        Log            *logger.Logger
        DryRun         bool
}

// NewSetup creates a new duplicacy working environment.
func NewSetup(workRoot, repositoryPath, backupTarget string, log *logger.Logger, dryRun bool) *Setup {
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
                Log:            log,
                DryRun:         dryRun,
        }
}

// CreateDirs creates the duplicacy working directories.
func (s *Setup) CreateDirs() error {
        if s.DryRun {
                s.Log.DryRun("mkdir -p %s", s.DuplicacyDir)
                return nil
        }

        if err := os.MkdirAll(s.DuplicacyDir, 0770); err != nil {
                return fmt.Errorf("failed to create duplicacy directories: %w", err)
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
                s.Log.DryRun("write JSON preferences to %s", s.PrefsFile)
                s.Log.Info("%s", json)
                return nil
        }

        return os.WriteFile(s.PrefsFile, []byte(json), 0660)
}

// WriteFilters writes the duplicacy filter file.
func (s *Setup) WriteFilters(filter string) error {
        if filter == "" {
                return nil
        }

        s.Log.Info("Creating filter definitions")

        if s.DryRun {
                s.Log.DryRun("Write filters to %s", s.FilterFile)
                for _, line := range strings.Split(filter, "\n") {
                        s.Log.Info("  %s", line)
                }
                return nil
        }

        if err := os.WriteFile(s.FilterFile, []byte(filter+"\n"), 0660); err != nil {
                return fmt.Errorf("failed to write filter file: %w", err)
        }

        s.Log.Info("Active filters:")
        for _, line := range strings.Split(filter, "\n") {
                s.Log.Info("  %s", line)
        }
        return nil
}

// SetPermissions sets directory (770) and file (660) permissions on the work directory.
func (s *Setup) SetPermissions() error {
        if s.DryRun {
                s.Log.DryRun("find %s -type d -exec chmod 770 {} +", s.DuplicacyRoot)
                s.Log.DryRun("find %s -type f -exec chmod 660 {} +", s.DuplicacyRoot)
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
                return fmt.Errorf("failed to set permissions in %s: %w", s.DuplicacyRoot, err)
        }
        return nil
}

// RunBackup executes `duplicacy backup -stats -threads N`.
func (s *Setup) RunBackup(threads int) error {
        args := []string{"backup", "-stats", "-threads", strconv.Itoa(threads)}

        if s.DryRun {
                s.Log.DryRun("duplicacy %s", strings.Join(args, " "))
                return nil
        }

        s.Log.Info("Starting backup")
        return s.runDuplicacy(args)
}

// ValidateRepo runs `duplicacy list -files` to verify the repo is valid.
func (s *Setup) ValidateRepo() error {
        if s.DryRun {
                s.Log.DryRun("duplicacy list -files")
                return nil
        }

        cmd := exec.Command("duplicacy", "list", "-files")
        cmd.Dir = s.DuplicacyRoot
        if err := cmd.Run(); err != nil {
                return fmt.Errorf("duplicacy repository validation failed - may need initialization")
        }

        s.Log.Info("Duplicacy repository validated")
        return nil
}

// GetTotalRevisionCount returns the number of unique revisions via `duplicacy list`.
func (s *Setup) GetTotalRevisionCount() (int, error) {
        if s.DryRun {
                return 0, nil
        }

        var buf bytes.Buffer
        cmd := exec.Command("duplicacy", "list")
        cmd.Dir = s.DuplicacyRoot
        cmd.Stdout = &buf
        cmd.Stderr = &buf

        if err := cmd.Run(); err != nil {
                for _, line := range strings.Split(buf.String(), "\n") {
                        if line != "" {
                                s.Log.Warn("%s", line)
                        }
                }
                return 0, fmt.Errorf("failed to list revisions: %w", err)
        }

        output := buf.String()
        for _, line := range strings.Split(output, "\n") {
                if line != "" {
                        s.Log.Info("[REVISION-LIST] %s", line)
                }
        }

        // Count unique revision numbers
        seen := make(map[int]bool)
        for _, match := range revisionRegex.FindAllStringSubmatch(output, -1) {
                if len(match) > 1 {
                        if n, err := strconv.Atoi(match[1]); err == nil {
                                seen[n] = true
                        }
                }
        }

        return len(seen), nil
}

// PrunePreview holds the results of a safe prune dry-run preview.
type PrunePreview struct {
        DeleteCount          int
        TotalRevisions       int
        DeletePercent        int  // Approximate (truncated) – for display only
        PercentEnforced      bool
        RevisionCountFailed  bool // True when revision listing failed
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
func (s *Setup) SafePrunePreview(pruneArgs []string, minTotalForPercent int) (*PrunePreview, error) {
        if s.DryRun {
                s.Log.DryRun("duplicacy prune %s -dry-run", strings.Join(pruneArgs, " "))
                return &PrunePreview{}, nil
        }

        s.Log.Info("Running safe prune preview")

        args := append([]string{"prune"}, pruneArgs...)
        args = append(args, "-dry-run")

        var buf bytes.Buffer
        cmd := exec.Command("duplicacy", args...)
        cmd.Dir = s.DuplicacyRoot
        cmd.Stdout = &buf
        cmd.Stderr = &buf

        if err := cmd.Run(); err != nil {
                output := buf.String()
                for _, line := range strings.Split(output, "\n") {
                        if line != "" {
                                s.Log.Error("%s", line)
                        }
                }
                return nil, fmt.Errorf("safe prune preview failed")
        }

        output := buf.String()
        for _, line := range strings.Split(output, "\n") {
                if line != "" {
                        s.Log.Info("[SAFE-PRUNE-PREVIEW] %s", line)
                }
        }

        // Count deletion lines
        deleteCount := len(deleteRegex.FindAllString(output, -1))

        // Get total revision count
        totalCount, err := s.GetTotalRevisionCount()

        preview := &PrunePreview{
                DeleteCount: deleteCount,
        }

        if err != nil {
                s.Log.Warn("Unable to determine revision count: %v", err)
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
func (s *Setup) RunPrune(pruneArgs []string) error {
        args := append([]string{"prune"}, pruneArgs...)

        if s.DryRun {
                s.Log.DryRun("duplicacy %s", strings.Join(args, " "))
                return nil
        }

        s.Log.Info("Starting policy prune")
        return s.runDuplicacy(args)
}

// RunDeepPrune executes `duplicacy prune -exhaustive -exclusive`.
func (s *Setup) RunDeepPrune() error {
        args := []string{"prune", "-exhaustive", "-exclusive"}

        if s.DryRun {
                s.Log.DryRun("duplicacy %s", strings.Join(args, " "))
                return nil
        }

        s.Log.Warn("Starting deep prune maintenance step: duplicacy prune -exhaustive -exclusive")
        return s.runDuplicacy(args)
}

func (s *Setup) runDuplicacy(args []string) error {
        cmd := exec.Command("duplicacy", args...)
        cmd.Dir = s.DuplicacyRoot
        cmd.Stdout = s.Log.Writer(logger.INFO, "[DUPLICACY] ")
        cmd.Stderr = s.Log.Writer(logger.ERROR, "[DUPLICACY] ")

        if err := cmd.Run(); err != nil {
                return fmt.Errorf("duplicacy %s failed: %w", args[0], err)
        }
        return nil
}

// Cleanup removes the work root directory.
func (s *Setup) Cleanup() {
        if s.WorkRoot != "" {
                s.Log.Info("Removing duplicacy work directory... %s", s.WorkRoot)
                if s.DryRun {
                        s.Log.DryRun("rm -rf %s", s.WorkRoot)
                        return
                }
                if err := os.RemoveAll(s.WorkRoot); err != nil {
                        s.Log.Warn("Failed to remove work directory %s: %v", s.WorkRoot, err)
                }
        }
}
