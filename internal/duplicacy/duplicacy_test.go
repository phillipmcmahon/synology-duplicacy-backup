package duplicacy

import (
        "encoding/json"
        "os"
        "path/filepath"
        "strings"
        "testing"

        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

// newTestLogger creates a logger in a temp dir for tests.
func newTestLogger(t *testing.T) *logger.Logger {
        t.Helper()
        dir := t.TempDir()
        log, err := logger.New(dir, "test", false)
        if err != nil {
                t.Fatalf("failed to create test logger: %v", err)
        }
        t.Cleanup(func() { log.Close() })
        return log
}

// ─── NewSetup tests ─────────────────────────────────────────────────────────

func TestNewSetup_PathConstruction(t *testing.T) {
        log := newTestLogger(t)
        s := NewSetup("/tmp/work", "/data/source", "s3://bucket", log, false)

        if s.WorkRoot != "/tmp/work" {
                t.Errorf("WorkRoot = %q", s.WorkRoot)
        }
        if s.DuplicacyRoot != "/tmp/work/duplicacy" {
                t.Errorf("DuplicacyRoot = %q", s.DuplicacyRoot)
        }
        if s.DuplicacyDir != "/tmp/work/duplicacy/.duplicacy" {
                t.Errorf("DuplicacyDir = %q", s.DuplicacyDir)
        }
        if s.PrefsFile != "/tmp/work/duplicacy/.duplicacy/preferences" {
                t.Errorf("PrefsFile = %q", s.PrefsFile)
        }
        if s.FilterFile != "/tmp/work/duplicacy/.duplicacy/filters" {
                t.Errorf("FilterFile = %q", s.FilterFile)
        }
        if s.RepositoryPath != "/data/source" {
                t.Errorf("RepositoryPath = %q", s.RepositoryPath)
        }
        if s.BackupTarget != "s3://bucket" {
                t.Errorf("BackupTarget = %q", s.BackupTarget)
        }
        if s.DryRun {
                t.Error("DryRun should be false")
        }
}

func TestNewSetup_DryRunFlag(t *testing.T) {
        log := newTestLogger(t)
        s := NewSetup("/tmp/work", "/data/source", "s3://bucket", log, true)
        if !s.DryRun {
                t.Error("DryRun should be true")
        }
}

// ─── CreateDirs tests ────────────────────────────────────────────────────────

func TestCreateDirs_ActualCreation(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        s := NewSetup(workRoot, "/data/source", "/target", log, false)

        if err := s.CreateDirs(); err != nil {
                t.Fatalf("CreateDirs failed: %v", err)
        }

        info, err := os.Stat(s.DuplicacyDir)
        if err != nil {
                t.Fatalf("DuplicacyDir not created: %v", err)
        }
        if !info.IsDir() {
                t.Error("DuplicacyDir is not a directory")
        }
}

func TestCreateDirs_DryRun(t *testing.T) {
        log := newTestLogger(t)
        workRoot := filepath.Join(t.TempDir(), "nonexistent")
        s := NewSetup(workRoot, "/data/source", "/target", log, true)

        if err := s.CreateDirs(); err != nil {
                t.Fatalf("CreateDirs (dry-run) failed: %v", err)
        }

        if _, err := os.Stat(s.DuplicacyDir); !os.IsNotExist(err) {
                t.Error("DuplicacyDir should not exist in dry-run mode")
        }
}

// ─── WritePreferences tests ─────────────────────────────────────────────────

func TestWritePreferences_WithSecrets(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        s := NewSetup(workRoot, "/data/source", "s3://bucket", log, false)
        if err := s.CreateDirs(); err != nil {
                t.Fatalf("CreateDirs: %v", err)
        }

        sec := &secrets.Secrets{
                StorjS3ID:     "test-id-1234567890abcdefghij",
                StorjS3Secret: "test-secret-abcdefghijklmnopqrstuvwxyz0123456789abcdef",
        }

        if err := s.WritePreferences(sec); err != nil {
                t.Fatalf("WritePreferences: %v", err)
        }

        data, err := os.ReadFile(s.PrefsFile)
        if err != nil {
                t.Fatalf("failed to read prefs file: %v", err)
        }

        // Validate JSON structure
        var prefs []map[string]interface{}
        if err := json.Unmarshal(data, &prefs); err != nil {
                t.Fatalf("invalid JSON: %v", err)
        }
        if len(prefs) != 1 {
                t.Fatalf("expected 1 preference entry, got %d", len(prefs))
        }
        if prefs[0]["repository"] != "/data/source" {
                t.Errorf("repository = %v", prefs[0]["repository"])
        }
        if prefs[0]["storage"] != "s3://bucket" {
                t.Errorf("storage = %v", prefs[0]["storage"])
        }
        keys, ok := prefs[0]["keys"].(map[string]interface{})
        if !ok {
                t.Fatal("keys should be an object when secrets provided")
        }
        if keys["storj_s3_id"] != sec.StorjS3ID {
                t.Errorf("storj_s3_id = %v", keys["storj_s3_id"])
        }
}

func TestWritePreferences_WithoutSecrets(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        s := NewSetup(workRoot, "/data/source", "/local/target", log, false)
        if err := s.CreateDirs(); err != nil {
                t.Fatalf("CreateDirs: %v", err)
        }

        if err := s.WritePreferences(nil); err != nil {
                t.Fatalf("WritePreferences: %v", err)
        }

        data, err := os.ReadFile(s.PrefsFile)
        if err != nil {
                t.Fatalf("failed to read prefs file: %v", err)
        }

        // When nil secrets, keys should be null in JSON
        content := string(data)
        if !strings.Contains(content, `"keys": null`) {
                t.Error("keys should be null when no secrets provided")
        }
}

func TestWritePreferences_DryRun(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        s := NewSetup(workRoot, "/data/source", "/target", log, true)

        if err := s.WritePreferences(nil); err != nil {
                t.Fatalf("WritePreferences (dry-run): %v", err)
        }

        if _, err := os.Stat(s.PrefsFile); !os.IsNotExist(err) {
                t.Error("prefs file should not exist in dry-run mode")
        }
}

// ─── WriteFilters tests ─────────────────────────────────────────────────────

func TestWriteFilters_NonEmpty(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        s := NewSetup(workRoot, "/data/source", "/target", log, false)
        if err := s.CreateDirs(); err != nil {
                t.Fatalf("CreateDirs: %v", err)
        }

        filter := "-e *.tmp\n-e *.log"
        if err := s.WriteFilters(filter); err != nil {
                t.Fatalf("WriteFilters: %v", err)
        }

        data, err := os.ReadFile(s.FilterFile)
        if err != nil {
                t.Fatalf("failed to read filter file: %v", err)
        }
        if string(data) != filter+"\n" {
                t.Errorf("filter content = %q, want %q", string(data), filter+"\n")
        }
}

func TestWriteFilters_Empty(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        s := NewSetup(workRoot, "/data/source", "/target", log, false)
        if err := s.CreateDirs(); err != nil {
                t.Fatalf("CreateDirs: %v", err)
        }

        if err := s.WriteFilters(""); err != nil {
                t.Fatalf("WriteFilters (empty): %v", err)
        }

        if _, err := os.Stat(s.FilterFile); !os.IsNotExist(err) {
                t.Error("filter file should not be created for empty filter")
        }
}

func TestWriteFilters_DryRun(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        s := NewSetup(workRoot, "/data/source", "/target", log, true)

        if err := s.WriteFilters("some filter"); err != nil {
                t.Fatalf("WriteFilters (dry-run): %v", err)
        }

        if _, err := os.Stat(s.FilterFile); !os.IsNotExist(err) {
                t.Error("filter file should not exist in dry-run mode")
        }
}

// ─── SetPermissions tests ───────────────────────────────────────────────────

func TestSetPermissions_SetsCorrectPerms(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        s := NewSetup(workRoot, "/data/source", "/target", log, false)
        if err := s.CreateDirs(); err != nil {
                t.Fatalf("CreateDirs: %v", err)
        }

        // Create a test file inside DuplicacyRoot
        testFile := filepath.Join(s.DuplicacyDir, "testfile")
        if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
                t.Fatalf("failed to create test file: %v", err)
        }

        if err := s.SetPermissions(); err != nil {
                t.Fatalf("SetPermissions: %v", err)
        }

        // Check directory permissions
        info, err := os.Stat(s.DuplicacyRoot)
        if err != nil {
                t.Fatalf("stat DuplicacyRoot: %v", err)
        }
        if info.Mode().Perm() != 0770 {
                t.Errorf("DuplicacyRoot perm = %04o, want 0770", info.Mode().Perm())
        }

        // Check file permissions
        finfo, err := os.Stat(testFile)
        if err != nil {
                t.Fatalf("stat test file: %v", err)
        }
        if finfo.Mode().Perm() != 0660 {
                t.Errorf("test file perm = %04o, want 0660", finfo.Mode().Perm())
        }
}

func TestSetPermissions_DryRun(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        s := NewSetup(workRoot, "/data/source", "/target", log, true)

        // Should not fail even if dirs don't exist
        if err := s.SetPermissions(); err != nil {
                t.Fatalf("SetPermissions (dry-run): %v", err)
        }
}

// ─── RunBackup DryRun test ──────────────────────────────────────────────────

func TestRunBackup_DryRun(t *testing.T) {
        log := newTestLogger(t)
        s := NewSetup(t.TempDir(), "/data/source", "/target", log, true)

        if err := s.RunBackup(4); err != nil {
                t.Fatalf("RunBackup (dry-run): %v", err)
        }
}

// ─── ValidateRepo DryRun test ───────────────────────────────────────────────

func TestValidateRepo_DryRun(t *testing.T) {
        log := newTestLogger(t)
        s := NewSetup(t.TempDir(), "/data/source", "/target", log, true)

        if err := s.ValidateRepo(); err != nil {
                t.Fatalf("ValidateRepo (dry-run): %v", err)
        }
}

// ─── GetTotalRevisionCount DryRun test ──────────────────────────────────────

func TestGetTotalRevisionCount_DryRun(t *testing.T) {
        log := newTestLogger(t)
        s := NewSetup(t.TempDir(), "/data/source", "/target", log, true)

        count, err := s.GetTotalRevisionCount()
        if err != nil {
                t.Fatalf("GetTotalRevisionCount (dry-run): %v", err)
        }
        if count != 0 {
                t.Errorf("expected 0 for dry-run, got %d", count)
        }
}

// ─── SafePrunePreview DryRun test ───────────────────────────────────────────

func TestSafePrunePreview_DryRun(t *testing.T) {
        log := newTestLogger(t)
        s := NewSetup(t.TempDir(), "/data/source", "/target", log, true)

        preview, err := s.SafePrunePreview([]string{"-keep", "0:365"}, 20)
        if err != nil {
                t.Fatalf("SafePrunePreview (dry-run): %v", err)
        }
        if preview == nil {
                t.Fatal("preview should not be nil")
        }
}

// ─── RunPrune DryRun test ───────────────────────────────────────────────────

func TestRunPrune_DryRun(t *testing.T) {
        log := newTestLogger(t)
        s := NewSetup(t.TempDir(), "/data/source", "/target", log, true)

        if err := s.RunPrune([]string{"-keep", "0:365"}); err != nil {
                t.Fatalf("RunPrune (dry-run): %v", err)
        }
}

// ─── RunDeepPrune DryRun test ───────────────────────────────────────────────

func TestRunDeepPrune_DryRun(t *testing.T) {
        log := newTestLogger(t)
        s := NewSetup(t.TempDir(), "/data/source", "/target", log, true)

        if err := s.RunDeepPrune(); err != nil {
                t.Fatalf("RunDeepPrune (dry-run): %v", err)
        }
}

// ─── Cleanup tests ──────────────────────────────────────────────────────────

func TestCleanup_RemovesWorkRoot(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        // Create a subdirectory to ensure it exists
        subdir := filepath.Join(workRoot, "sub")
        os.MkdirAll(subdir, 0755)

        s := NewSetup(workRoot, "/data/source", "/target", log, false)
        s.Cleanup()

        if _, err := os.Stat(workRoot); !os.IsNotExist(err) {
                t.Error("WorkRoot should be removed after Cleanup")
        }
}

func TestCleanup_DryRun(t *testing.T) {
        log := newTestLogger(t)
        workRoot := t.TempDir()
        s := NewSetup(workRoot, "/data/source", "/target", log, true)
        s.Cleanup()

        if _, err := os.Stat(workRoot); err != nil {
                t.Error("WorkRoot should NOT be removed in dry-run mode")
        }
}

func TestCleanup_EmptyWorkRoot(t *testing.T) {
        log := newTestLogger(t)
        s := &Setup{WorkRoot: "", Log: log}
        // Should be a no-op, not panic
        s.Cleanup()
}

// ─── Regex tests ────────────────────────────────────────────────────────────

func TestRevisionRegex(t *testing.T) {
        tests := []struct {
                input   string
                matches []string
        }{
                {"revision 1", []string{"1"}},
                {"Revision 42 at", []string{"42"}},
                {"revision 1\nrevision 2\nrevision 3", []string{"1", "2", "3"}},
                {"no match here", nil},
                {"REVISION 100", []string{"100"}},
        }
        for _, tt := range tests {
                matches := revisionRegex.FindAllStringSubmatch(tt.input, -1)
                var got []string
                for _, m := range matches {
                        if len(m) > 1 {
                                got = append(got, m[1])
                        }
                }
                if len(got) != len(tt.matches) {
                        t.Errorf("revisionRegex(%q): got %v, want %v", tt.input, got, tt.matches)
                        continue
                }
                for i := range got {
                        if got[i] != tt.matches[i] {
                                t.Errorf("revisionRegex(%q)[%d]: got %q, want %q", tt.input, i, got[i], tt.matches[i])
                        }
                }
        }
}

func TestDeleteRegex(t *testing.T) {
        tests := []struct {
                input string
                count int
        }{
                {"deleting revision 1", 1},
                {"deleted revision 2", 1},
                {"delete revision 3", 1},
                {"Deleting revision 4\nDeleted revision 5", 2},
                {"no deletions here", 0},
        }
        for _, tt := range tests {
                matches := deleteRegex.FindAllString(tt.input, -1)
                if len(matches) != tt.count {
                        t.Errorf("deleteRegex(%q): found %d, want %d", tt.input, len(matches), tt.count)
                }
        }
}

// ─── PrunePreview struct test ───────────────────────────────────────────────

func TestPrunePreview_Fields(t *testing.T) {
        p := &PrunePreview{
                DeleteCount:     5,
                TotalRevisions:  100,
                DeletePercent:   5,
                PercentEnforced: true,
        }
        if p.DeleteCount != 5 {
                t.Errorf("DeleteCount = %d", p.DeleteCount)
        }
        if p.TotalRevisions != 100 {
                t.Errorf("TotalRevisions = %d", p.TotalRevisions)
        }
        if p.DeletePercent != 5 {
                t.Errorf("DeletePercent = %d", p.DeletePercent)
        }
        if !p.PercentEnforced {
                t.Error("PercentEnforced should be true")
        }
}

// ─── ExceedsPercent cross-multiplication tests ──────────────────────────────

func TestExceedsPercent_CrossMultiplication(t *testing.T) {
        tests := []struct {
                name       string
                delete     int
                total      int
                enforced   bool
                maxPercent int
                want       bool
        }{
                // 3/29 ≈ 10.34%, should exceed 10% (integer division would truncate to 10 and miss this)
                {"truncation edge case 3 of 29 at 10%", 3, 29, true, 10, true},
                // exact boundary: 10/100 = 10%, should NOT exceed 10%
                {"exact boundary 10 of 100 at 10%", 10, 100, true, 10, false},
                // clearly over
                {"clearly over 11 of 100 at 10%", 11, 100, true, 10, true},
                // clearly under
                {"clearly under 1 of 100 at 10%", 1, 100, true, 10, false},
                // not enforced returns false regardless
                {"not enforced", 50, 100, false, 10, false},
                // zero total returns false
                {"zero total", 5, 0, true, 10, false},
                // 1/10 = 10%, should NOT exceed 10%
                {"exact 1 of 10 at 10%", 1, 10, true, 10, false},
                // 2/19 ≈ 10.53%, should exceed 10%
                {"truncation edge 2 of 19 at 10%", 2, 19, true, 10, true},
                // 0 deletes never exceeds
                {"zero deletes", 0, 100, true, 10, false},
        }

        for _, tt := range tests {
                t.Run(tt.name, func(t *testing.T) {
                        p := &PrunePreview{
                                DeleteCount:     tt.delete,
                                TotalRevisions:  tt.total,
                                PercentEnforced: tt.enforced,
                        }
                        got := p.ExceedsPercent(tt.maxPercent)
                        if got != tt.want {
                                t.Errorf("ExceedsPercent(%d): delete=%d total=%d => %v, want %v",
                                        tt.maxPercent, tt.delete, tt.total, got, tt.want)
                        }
                })
        }
}

// ─── RevisionCountFailed flag test ──────────────────────────────────────────

func TestPrunePreview_RevisionCountFailed(t *testing.T) {
        // When RevisionCountFailed is set, ExceedsPercent should return false
        // (percentage enforcement is not active)
        p := &PrunePreview{
                DeleteCount:         10,
                TotalRevisions:      0,
                PercentEnforced:     false,
                RevisionCountFailed: true,
        }
        if !p.RevisionCountFailed {
                t.Error("RevisionCountFailed should be true")
        }
        if p.ExceedsPercent(5) {
                t.Error("ExceedsPercent should return false when percent not enforced")
        }
}

// ─── GetTotalRevisionCount failure test ─────────────────────────────────────

func TestGetTotalRevisionCount_FailsClosedOnError(t *testing.T) {
        // When duplicacy is not installed / list fails, GetTotalRevisionCount
        // must return an error (fail closed) rather than (0, nil).
        log := newTestLogger(t)
        s := NewSetup(t.TempDir(), "/data/source", "/target", log, false)

        // duplicacy is not installed in test env, so cmd.Run will fail
        count, err := s.GetTotalRevisionCount()
        if err == nil {
                t.Fatal("expected error when duplicacy list fails, got nil")
        }
        if count != 0 {
                t.Errorf("expected count=0 on error, got %d", count)
        }
}
