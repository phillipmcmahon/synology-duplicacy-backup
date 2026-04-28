package duplicacy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

// newTestSetup creates a Setup with a mock runner for tests.
func newTestSetup(t *testing.T, dryRun bool, results ...execpkg.MockResult) (*Setup, *execpkg.MockRunner) {
	t.Helper()
	mock := execpkg.NewMockRunner(results...)
	s := NewSetup(t.TempDir(), "/data/source", "/target", dryRun, mock)
	return s, mock
}

// ─── NewSetup tests ─────────────────────────────────────────────────────────

func TestNewSetup_PathConstruction(t *testing.T) {
	mock := execpkg.NewMockRunner()
	s := NewSetup("/tmp/work", "/data/source", "s3://bucket", false, mock)

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
	if s.Runner == nil {
		t.Error("Runner should not be nil")
	}
}

func TestNewSetup_DryRunFlag(t *testing.T) {
	mock := execpkg.NewMockRunner()
	s := NewSetup("/tmp/work", "/data/source", "s3://bucket", true, mock)
	if !s.DryRun {
		t.Error("DryRun should be true")
	}
}

// ─── CreateDirs tests ────────────────────────────────────────────────────────

func TestCreateDirs_ActualCreation(t *testing.T) {
	s, _ := newTestSetup(t, false)
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
	mock := execpkg.NewMockRunner()
	workRoot := filepath.Join(t.TempDir(), "nonexistent")
	s := NewSetup(workRoot, "/data/source", "/target", true, mock)

	if err := s.CreateDirs(); err != nil {
		t.Fatalf("CreateDirs (dry-run) failed: %v", err)
	}

	if _, err := os.Stat(s.DuplicacyDir); !os.IsNotExist(err) {
		t.Error("DuplicacyDir should not exist in dry-run mode")
	}
}

// ─── WritePreferences tests ─────────────────────────────────────────────────

func TestWritePreferences_WithSecrets(t *testing.T) {
	s, _ := newTestSetup(t, false)
	if err := s.CreateDirs(); err != nil {
		t.Fatalf("CreateDirs: %v", err)
	}

	sec := &secrets.Secrets{
		Keys: map[string]string{
			"s3_id":     "test-id-1234567890abcdefghij",
			"s3_secret": "test-secret-abcdefghijklmnopqrstuvwxyz0123456789abcdef",
		},
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
	if prefs[0]["storage"] != "/target" {
		t.Errorf("storage = %v", prefs[0]["storage"])
	}
	keys, ok := prefs[0]["keys"].(map[string]interface{})
	if !ok {
		t.Fatal("keys should be an object when secrets provided")
	}
	if keys["s3_id"] != sec.Keys["s3_id"] {
		t.Errorf("s3_id = %v", keys["s3_id"])
	}
}

func TestWritePreferences_WithoutSecrets(t *testing.T) {
	s, _ := newTestSetup(t, false)
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

func TestNewWorkspaceSetupWritesPreferencesInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	storage := "s3://bucket/homes"
	setup := NewWorkspaceSetup(workspace, storage, false, execpkg.NewMockRunner())

	if setup.DuplicacyRoot != workspace ||
		setup.DuplicacyDir != filepath.Join(workspace, ".duplicacy") ||
		setup.PrefsFile != filepath.Join(workspace, ".duplicacy", "preferences") ||
		setup.RepositoryPath != workspace ||
		setup.BackupTarget != storage {
		t.Fatalf("setup = %#v", setup)
	}
	if err := setup.CreateDirs(); err != nil {
		t.Fatalf("CreateDirs() error = %v", err)
	}
	if err := setup.WritePreferences(nil); err != nil {
		t.Fatalf("WritePreferences() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".duplicacy", "preferences")); err != nil {
		t.Fatalf("preferences not written in workspace: %v", err)
	}
}

func TestWritePreferences_DryRun(t *testing.T) {
	s, _ := newTestSetup(t, true)

	if err := s.WritePreferences(nil); err != nil {
		t.Fatalf("WritePreferences (dry-run): %v", err)
	}

	if _, err := os.Stat(s.PrefsFile); !os.IsNotExist(err) {
		t.Error("prefs file should not exist in dry-run mode")
	}
}

// ─── WriteFilters tests ─────────────────────────────────────────────────────

func TestWriteFilters_NonEmpty(t *testing.T) {
	s, _ := newTestSetup(t, false)
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
	s, _ := newTestSetup(t, false)
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
	s, _ := newTestSetup(t, true)

	if err := s.WriteFilters("some filter"); err != nil {
		t.Fatalf("WriteFilters (dry-run): %v", err)
	}

	if _, err := os.Stat(s.FilterFile); !os.IsNotExist(err) {
		t.Error("filter file should not exist in dry-run mode")
	}
}

// ─── SetPermissions tests ───────────────────────────────────────────────────

func TestSetPermissions_SetsCorrectPerms(t *testing.T) {
	s, _ := newTestSetup(t, false)
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
	s, _ := newTestSetup(t, true)

	// Should not fail even if dirs don't exist
	if err := s.SetPermissions(); err != nil {
		t.Fatalf("SetPermissions (dry-run): %v", err)
	}
}

// ─── RunBackup DryRun test ──────────────────────────────────────────────────

func TestRunBackup_DryRun(t *testing.T) {
	s, _ := newTestSetup(t, true)

	_, _, err := s.RunBackup(4)
	if err != nil {
		t.Fatalf("RunBackup (dry-run): %v", err)
	}
}

// ─── RunBackup with mock runner ─────────────────────────────────────────────

func TestRunBackup_UsesRunner(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{Stdout: "backup done\n"})

	stdout, _, err := s.RunBackup(4)
	if err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if stdout != "backup done\n" {
		t.Errorf("stdout = %q, want %q", stdout, "backup done\n")
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	inv := mock.Invocations[0]
	if inv.Cmd != "duplicacy" {
		t.Errorf("cmd = %q, want duplicacy", inv.Cmd)
	}
	if inv.Args[0] != "backup" {
		t.Errorf("args[0] = %q, want backup", inv.Args[0])
	}
}

func TestRunBackup_Error(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{Err: errors.New("backup failed")})

	_, _, err := s.RunBackup(4)
	if err == nil {
		t.Fatal("expected error from RunBackup")
	}
	var backupErr *apperrors.BackupError
	if !errors.As(err, &backupErr) {
		t.Errorf("expected *BackupError, got %T", err)
	}
}

// ─── ValidateRepo DryRun test ───────────────────────────────────────────────

func TestValidateRepo_DryRun(t *testing.T) {
	s, _ := newTestSetup(t, true)

	if err := s.ValidateRepo(); err != nil {
		t.Fatalf("ValidateRepo (dry-run): %v", err)
	}
}

func TestValidateRepo_UsesRunner(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{})

	if err := s.ValidateRepo(); err != nil {
		t.Fatalf("ValidateRepo: %v", err)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Cmd != "duplicacy" {
		t.Errorf("cmd = %q, want duplicacy", mock.Invocations[0].Cmd)
	}
}

func TestValidateRepo_Failure(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{Err: errors.New("list failed")})

	err := s.ValidateRepo()
	if err == nil {
		t.Fatal("expected error from ValidateRepo")
	}
	var pruneErr *apperrors.PruneError
	if !errors.As(err, &pruneErr) {
		t.Errorf("expected *PruneError, got %T", err)
	}
}

func TestProbeRepository_DryRun(t *testing.T) {
	s, _ := newTestSetup(t, true)

	state, output, err := s.ProbeRepository()
	if err != nil {
		t.Fatalf("ProbeRepository (dry-run): %v", err)
	}
	if state != RepositoryAccessible {
		t.Fatalf("state = %q, want %q", state, RepositoryAccessible)
	}
	if output != "" {
		t.Fatalf("output = %q, want empty", output)
	}
}

func TestProbeRepository_Accessible(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{Stdout: "Listing all revisions for storage storj\n"})

	state, output, err := s.ProbeRepository()
	if err != nil {
		t.Fatalf("ProbeRepository: %v", err)
	}
	if state != RepositoryAccessible {
		t.Fatalf("state = %q, want %q", state, RepositoryAccessible)
	}
	if !strings.Contains(output, "Listing all revisions") {
		t.Fatalf("output = %q", output)
	}
}

func TestProbeRepository_Uninitialized(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{
		Stderr: "Storage has not been initialized yet; initialize the storage first\n",
		Err:    errors.New("list failed"),
	})

	state, output, err := s.ProbeRepository()
	if err != nil {
		t.Fatalf("ProbeRepository() unexpected error = %v", err)
	}
	if state != RepositoryUninitialized {
		t.Fatalf("state = %q, want %q", state, RepositoryUninitialized)
	}
	if !strings.Contains(strings.ToLower(output), "initialized") {
		t.Fatalf("output = %q", output)
	}
}

func TestProbeRepository_Inaccessible(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{
		Stderr: "Access denied\n",
		Err:    errors.New("list failed"),
	})

	state, _, err := s.ProbeRepository()
	if state != RepositoryInaccessible {
		t.Fatalf("state = %q, want %q", state, RepositoryInaccessible)
	}
	if err == nil {
		t.Fatal("expected error from ProbeRepository")
	}
	var pruneErr *apperrors.PruneError
	if !errors.As(err, &pruneErr) {
		t.Errorf("expected *PruneError, got %T", err)
	}
}

func TestLooksUninitializedRepositoryOutput(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "empty", output: "", want: false},
		{name: "initialized phrase", output: "Storage has not been initialized yet", want: true},
		{name: "repository phrase", output: "Repository has not been initialized", want: true},
		{name: "snapshots not found", output: "Failed to read snapshots/abc: not found", want: true},
		{name: "access denied", output: "Access denied", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksUninitializedRepositoryOutput(tc.output); got != tc.want {
				t.Fatalf("looksUninitializedRepositoryOutput(%q) = %t, want %t", tc.output, got, tc.want)
			}
		})
	}
}

// ─── GetTotalRevisionCount DryRun test ──────────────────────────────────────

func TestGetTotalRevisionCount_DryRun(t *testing.T) {
	s, _ := newTestSetup(t, true)

	count, _, err := s.GetTotalRevisionCount()
	if err != nil {
		t.Fatalf("GetTotalRevisionCount (dry-run): %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for dry-run, got %d", count)
	}
}

func TestGetTotalRevisionCount_UsesRunner(t *testing.T) {
	s, mock := newTestSetup(t, false,
		execpkg.MockResult{Stdout: "Listing all revisions for storage storj:\nrevision 1\nrevision 2\nrevision 3\n"},
	)

	count, _, err := s.GetTotalRevisionCount()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
}

func TestGetTotalRevisionCount_FailsClosedOnError(t *testing.T) {
	s, _ := newTestSetup(t, false,
		execpkg.MockResult{Err: errors.New("list failed")},
	)

	count, _, err := s.GetTotalRevisionCount()
	if err == nil {
		t.Fatal("expected error when duplicacy list fails, got nil")
	}
	if count != 0 {
		t.Errorf("expected count=0 on error, got %d", count)
	}
}

func TestGetLatestRevision(t *testing.T) {
	t.Run("dry run", func(t *testing.T) {
		s, _ := newTestSetup(t, true)
		revision, output, err := s.GetLatestRevision()
		if err != nil {
			t.Fatalf("GetLatestRevision() error = %v", err)
		}
		if revision != 0 || output != "" {
			t.Fatalf("revision=%d output=%q", revision, output)
		}
	})

	t.Run("selects highest revision", func(t *testing.T) {
		s, mock := newTestSetup(t, false, execpkg.MockResult{Stdout: "revision 4\nrevision 10\nrevision 7\n"})
		revision, output, err := s.GetLatestRevision()
		if err != nil {
			t.Fatalf("GetLatestRevision() error = %v", err)
		}
		if revision != 10 || !strings.Contains(output, "revision 10") {
			t.Fatalf("revision=%d output=%q", revision, output)
		}
		if len(mock.Invocations) != 1 || mock.Invocations[0].Dir != s.DuplicacyRoot {
			t.Fatalf("invocations = %#v", mock.Invocations)
		}
	})

	t.Run("errors on list failure", func(t *testing.T) {
		s, _ := newTestSetup(t, false, execpkg.MockResult{Stderr: "denied\n", Err: errors.New("list failed")})
		revision, output, err := s.GetLatestRevision()
		if err == nil {
			t.Fatal("expected error")
		}
		if revision != 0 || !strings.Contains(output, "denied") {
			t.Fatalf("revision=%d output=%q err=%v", revision, output, err)
		}
	})
}

func TestListVisibleRevisionsAndLatestInfo(t *testing.T) {
	output := strings.Join([]string{
		"Snapshot data at revision 1 created at 2026-04-23 02:30",
		"Snapshot data at revision 3 created at 2026-04-24 13:00:00",
		"Snapshot data at revision 2",
	}, "\n")
	s, _ := newTestSetup(t, false, execpkg.MockResult{Stdout: output}, execpkg.MockResult{Stdout: output})

	revisions, combined, err := s.ListVisibleRevisions()
	if err != nil {
		t.Fatalf("ListVisibleRevisions() error = %v", err)
	}
	if len(revisions) != 3 || revisions[0].Revision != 3 || revisions[1].Revision != 2 || revisions[2].Revision != 1 {
		t.Fatalf("revisions = %#v", revisions)
	}
	if revisions[0].CreatedAt.IsZero() || revisions[2].CreatedAt.IsZero() {
		t.Fatalf("expected created timestamps: %#v", revisions)
	}
	if !strings.Contains(combined, "revision 3") {
		t.Fatalf("combined = %q", combined)
	}

	info, _, err := s.GetLatestRevisionInfo()
	if err != nil {
		t.Fatalf("GetLatestRevisionInfo() error = %v", err)
	}
	if info == nil || info.Revision != 3 {
		t.Fatalf("latest = %#v", info)
	}
}

func TestListVisibleRevisionsErrorAndDryRun(t *testing.T) {
	s, _ := newTestSetup(t, true)
	revisions, output, err := s.ListVisibleRevisions()
	if err != nil || revisions != nil || output != "" {
		t.Fatalf("dry-run revisions=%#v output=%q err=%v", revisions, output, err)
	}
	info, output, err := s.GetLatestRevisionInfo()
	if err != nil || info != nil || output != "" {
		t.Fatalf("dry-run latest=%#v output=%q err=%v", info, output, err)
	}

	s, _ = newTestSetup(t, false, execpkg.MockResult{Stderr: "list failed\n", Err: errors.New("boom")})
	revisions, output, err = s.ListVisibleRevisions()
	if err == nil || revisions != nil || !strings.Contains(output, "list failed") {
		t.Fatalf("revisions=%#v output=%q err=%v", revisions, output, err)
	}
}

func TestListRevisionFilesAndRestoreRevision(t *testing.T) {
	s, mock := newTestSetup(t, false,
		execpkg.MockResult{Stdout: "files\n"},
		execpkg.MockResult{Stdout: "restored\n"},
		execpkg.MockResult{Stdout: "full restore\n"},
	)

	output, err := s.ListRevisionFiles(42)
	if err != nil || output != "files\n" {
		t.Fatalf("ListRevisionFiles() output=%q err=%v", output, err)
	}
	output, err = s.RestoreRevision(42, "path/with spaces.txt")
	if err != nil || output != "restored\n" {
		t.Fatalf("RestoreRevision() output=%q err=%v", output, err)
	}
	output, err = s.RestoreRevisionContext(context.TODO(), 42, "")
	if err != nil || output != "full restore\n" {
		t.Fatalf("RestoreRevisionContext(nil) output=%q err=%v", output, err)
	}
	if got := strings.Join(mock.Invocations[0].Args, " "); got != "list -files -r 42" {
		t.Fatalf("list args = %q", got)
	}
	if got := strings.Join(mock.Invocations[1].Args, " "); got != "restore -r 42 -stats -- path/with spaces.txt" {
		t.Fatalf("restore args = %q", got)
	}
	if got := strings.Join(mock.Invocations[2].Args, " "); got != "restore -r 42 -stats" {
		t.Fatalf("full restore args = %q", got)
	}
}

func TestListRevisionFilesAndRestoreErrors(t *testing.T) {
	s, _ := newTestSetup(t, true)
	if output, err := s.ListRevisionFiles(1); err != nil || output != "" {
		t.Fatalf("dry-run ListRevisionFiles() output=%q err=%v", output, err)
	}
	if output, err := s.RestoreRevisionContext(context.Background(), 1, "path"); err != nil || output != "" {
		t.Fatalf("dry-run RestoreRevisionContext() output=%q err=%v", output, err)
	}

	s, _ = newTestSetup(t, false,
		execpkg.MockResult{Stderr: "list failed\n", Err: errors.New("boom")},
		execpkg.MockResult{Stderr: "restore failed\n", Err: errors.New("boom")},
	)
	if output, err := s.ListRevisionFiles(7); err == nil || !strings.Contains(output, "list failed") {
		t.Fatalf("ListRevisionFiles() output=%q err=%v", output, err)
	}
	if output, err := s.RestoreRevisionContext(context.Background(), 7, "path"); err == nil || !strings.Contains(output, "restore failed") {
		t.Fatalf("RestoreRevisionContext() output=%q err=%v", output, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, _ = newTestSetup(t, false, execpkg.MockResult{Stderr: "interrupted\n", Err: context.Canceled})
	if output, err := s.RestoreRevisionContext(ctx, 7, "path"); err == nil || !strings.Contains(output, "interrupted") || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("RestoreRevisionContext(cancelled) output=%q err=%v", output, err)
	}
}

func TestParseVisibleRevisionsAndCheckResults(t *testing.T) {
	created, err := parseRevisionCreatedAt("2026-04-24 13:00")
	if err != nil {
		t.Fatalf("parseRevisionCreatedAt(short) error = %v", err)
	}
	if created.IsZero() {
		t.Fatal("created timestamp should be populated")
	}
	if _, err := parseRevisionCreatedAt("not a time"); err == nil {
		t.Fatal("expected parseRevisionCreatedAt to reject unsupported timestamp")
	}

	revisions := parseVisibleRevisions(strings.Join([]string{
		"Snapshot data at revision 5 created at 2026-04-25 13:00:00",
		"Snapshot data at revision 4 created at 2026-04-25 07:00",
		"Snapshot data at revision 3",
		"revision not-a-number",
	}, "\n"))
	if len(revisions) != 3 || revisions[0].Revision != 5 || revisions[1].Revision != 4 || revisions[2].Revision != 3 {
		t.Fatalf("revisions = %#v", revisions)
	}
	if got := parseVisibleRevisions(""); got != nil {
		t.Fatalf("empty parseVisibleRevisions() = %#v", got)
	}

	checks := parseRevisionCheckResults(strings.Join([]string{
		"All chunks referenced by snapshot data at revision 5 exist",
		"Some chunks referenced by snapshot data at revision 4 are missing",
		"Chunk abcdef referenced by snapshot data at revision 3 does not exist",
		"All chunks referenced by snapshot data at revision 3 exist",
	}, "\n"))
	if len(checks) != 3 || checks[0].Revision != 5 || checks[0].Result != "pass" || checks[1].Result != "fail" || checks[2].Result != "fail" {
		t.Fatalf("checks = %#v", checks)
	}
	if got := parseRevisionCheckResults(""); got != nil {
		t.Fatalf("empty parseRevisionCheckResults() = %#v", got)
	}
}

// ─── SafePrunePreview DryRun test ───────────────────────────────────────────

func TestSafePrunePreview_DryRun(t *testing.T) {
	s, _ := newTestSetup(t, true)

	preview, err := s.SafePrunePreview([]string{"-keep", "0:365"}, 20)
	if err != nil {
		t.Fatalf("SafePrunePreview (dry-run): %v", err)
	}
	if preview == nil {
		t.Fatal("preview should not be nil")
	}
}

func TestSafePrunePreview_UsesRunner(t *testing.T) {
	s, mock := newTestSetup(t, false,
		// First call: prune -dry-run
		execpkg.MockResult{Stdout: "Deleting snapshot data at revision 1\nDeleting snapshot data at revision 2\n"},
		// Second call: list (for GetTotalRevisionCount)
		execpkg.MockResult{Stdout: "revision 1\nrevision 2\nrevision 3\nrevision 4\nrevision 5\nrevision 6\nrevision 7\nrevision 8\nrevision 9\nrevision 10\nrevision 11\nrevision 12\nrevision 13\nrevision 14\nrevision 15\nrevision 16\nrevision 17\nrevision 18\nrevision 19\nrevision 20\n"},
	)

	preview, err := s.SafePrunePreview([]string{"-keep", "0:365"}, 20)
	if err != nil {
		t.Fatalf("SafePrunePreview: %v", err)
	}
	if preview.DeleteCount != 2 {
		t.Errorf("DeleteCount = %d, want 2", preview.DeleteCount)
	}
	if preview.TotalRevisions != 20 {
		t.Errorf("TotalRevisions = %d, want 20", preview.TotalRevisions)
	}
	if !preview.PercentEnforced {
		t.Error("PercentEnforced should be true")
	}
	if len(mock.Invocations) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(mock.Invocations))
	}
}

// ─── RunPrune DryRun test ───────────────────────────────────────────────────

func TestRunPrune_DryRun(t *testing.T) {
	s, _ := newTestSetup(t, true)

	_, _, err := s.RunPrune([]string{"-keep", "0:365"})
	if err != nil {
		t.Fatalf("RunPrune (dry-run): %v", err)
	}
}

func TestRunPrune_UsesRunner(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{})

	_, _, err := s.RunPrune([]string{"-keep", "0:365"})
	if err != nil {
		t.Fatalf("RunPrune: %v", err)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Cmd != "duplicacy" {
		t.Errorf("cmd = %q, want duplicacy", mock.Invocations[0].Cmd)
	}
}

// ─── RunCleanupStorage tests ────────────────────────────────────────────────

func TestRunCleanupStorage_DryRun(t *testing.T) {
	s, _ := newTestSetup(t, true)

	_, _, err := s.RunCleanupStorage()
	if err != nil {
		t.Fatalf("RunCleanupStorage (dry-run): %v", err)
	}
}

func TestRunCleanupStorage_UsesRunner(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{})

	_, _, err := s.RunCleanupStorage()
	if err != nil {
		t.Fatalf("RunCleanupStorage: %v", err)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	inv := mock.Invocations[0]
	if inv.Args[0] != "prune" || inv.Args[1] != "-exhaustive" {
		t.Errorf("unexpected args: %v", inv.Args)
	}
}

// ─── Cleanup tests ──────────────────────────────────────────────────────────

func TestCleanup_RemovesWorkRoot(t *testing.T) {
	s, _ := newTestSetup(t, false)
	workRoot := s.WorkRoot
	// Create a subdirectory to ensure it exists
	subdir := filepath.Join(workRoot, "sub")
	os.MkdirAll(subdir, 0755)

	if err := s.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if _, err := os.Stat(workRoot); !os.IsNotExist(err) {
		t.Error("WorkRoot should be removed after Cleanup")
	}
}

func TestCleanup_DryRun(t *testing.T) {
	s, _ := newTestSetup(t, true)
	if err := s.Cleanup(); err != nil {
		t.Fatalf("Cleanup (dry-run) failed: %v", err)
	}

	if _, err := os.Stat(s.WorkRoot); err != nil {
		t.Error("WorkRoot should NOT be removed in dry-run mode")
	}
}

func TestCleanup_EmptyWorkRoot(t *testing.T) {
	s := &Setup{WorkRoot: ""}
	// Should be a no-op, not panic
	if err := s.Cleanup(); err != nil {
		t.Fatalf("Cleanup with empty WorkRoot should not error: %v", err)
	}
}

// ─── RunInDir directory context tests ────────────────────────────────────────
// These tests verify that every duplicacy operation calls RunInDir with the
// correct DuplicacyRoot directory.  This is the key regression test for the
// "wrong directory" bug where duplicacy commands ran without a working
// directory and could not find .duplicacy/preferences.

func TestRunBackup_CallsRunInDir_WithDuplicacyRoot(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{Stdout: "ok"})

	_, _, err := s.RunBackup(4)
	if err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	inv := mock.Invocations[0]
	if inv.Dir != s.DuplicacyRoot {
		t.Errorf("RunBackup Dir = %q, want DuplicacyRoot %q", inv.Dir, s.DuplicacyRoot)
	}
}

func TestValidateRepo_CallsRunInDir_WithDuplicacyRoot(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{})

	_ = s.ValidateRepo()
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Dir != s.DuplicacyRoot {
		t.Errorf("ValidateRepo Dir = %q, want DuplicacyRoot %q", mock.Invocations[0].Dir, s.DuplicacyRoot)
	}
}

func TestGetTotalRevisionCount_CallsRunInDir_WithDuplicacyRoot(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{Stdout: "revision 1\n"})

	_, _, err := s.GetTotalRevisionCount()
	if err != nil {
		t.Fatalf("GetTotalRevisionCount: %v", err)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Dir != s.DuplicacyRoot {
		t.Errorf("GetTotalRevisionCount Dir = %q, want DuplicacyRoot %q", mock.Invocations[0].Dir, s.DuplicacyRoot)
	}
}

func TestRunPrune_CallsRunInDir_WithDuplicacyRoot(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{})

	_, _, err := s.RunPrune([]string{"-keep", "0:365"})
	if err != nil {
		t.Fatalf("RunPrune: %v", err)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Dir != s.DuplicacyRoot {
		t.Errorf("RunPrune Dir = %q, want DuplicacyRoot %q", mock.Invocations[0].Dir, s.DuplicacyRoot)
	}
}

func TestRunCleanupStorage_CallsRunInDir_WithDuplicacyRoot(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{})

	_, _, err := s.RunCleanupStorage()
	if err != nil {
		t.Fatalf("RunCleanupStorage: %v", err)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	if mock.Invocations[0].Dir != s.DuplicacyRoot {
		t.Errorf("RunCleanupStorage Dir = %q, want DuplicacyRoot %q", mock.Invocations[0].Dir, s.DuplicacyRoot)
	}
}

func TestSafePrunePreview_CallsRunInDir_WithDuplicacyRoot(t *testing.T) {
	s, mock := newTestSetup(t, false,
		execpkg.MockResult{Stdout: "Deleting snapshot data at revision 1\n"}, // prune -dry-run
		execpkg.MockResult{Stdout: "revision 1\nrevision 2\n"},               // list
	)

	_, err := s.SafePrunePreview([]string{"-keep", "0:365"}, 20)
	if err != nil {
		t.Fatalf("SafePrunePreview: %v", err)
	}
	// Both invocations (prune preview + revision count) should use DuplicacyRoot
	for i, inv := range mock.Invocations {
		if inv.Dir != s.DuplicacyRoot {
			t.Errorf("Invocation[%d] Dir = %q, want DuplicacyRoot %q", i, inv.Dir, s.DuplicacyRoot)
		}
	}
}

// TestAllDuplicacyOps_NeverUseRun_AlwaysUseRunInDir is an integration-style
// test that runs every non-dry-run operation and asserts that ALL mock
// invocations have a non-empty Dir field set to DuplicacyRoot.  If someone
// accidentally changes a RunInDir call back to Run, this test will catch it.
func TestAllDuplicacyOps_NeverUseRun_AlwaysUseRunInDir(t *testing.T) {
	// Queue enough results for: RunBackup + ValidateRepo + GetTotalRevisionCount + RunPrune + RunCleanupStorage
	mock := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "backup ok\n"},  // RunBackup
		execpkg.MockResult{},                       // ValidateRepo
		execpkg.MockResult{Stdout: "revision 1\n"}, // GetTotalRevisionCount
		execpkg.MockResult{},                       // RunPrune
		execpkg.MockResult{},                       // RunCleanupStorage
	)
	s := NewSetup(t.TempDir(), "/data/source", "/target", false, mock)

	s.RunBackup(4)
	s.ValidateRepo()
	s.GetTotalRevisionCount()
	s.RunPrune([]string{"-keep", "0:365"})
	s.RunCleanupStorage()

	for i, inv := range mock.Invocations {
		if inv.Dir == "" {
			t.Errorf("Invocation[%d] (%s %v) used Run instead of RunInDir — Dir is empty",
				i, inv.Cmd, inv.Args)
		}
		if inv.Dir != s.DuplicacyRoot {
			t.Errorf("Invocation[%d] (%s %v) Dir = %q, want DuplicacyRoot %q",
				i, inv.Cmd, inv.Args, inv.Dir, s.DuplicacyRoot)
		}
	}
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
		{"Deleting snapshot data at revision 2211", 1},
		{"Deleting snapshot data at revision 2211\nDeleting snapshot data at revision 2212", 2},
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

// ─── redactSecrets tests ────────────────────────────────────────────────────

func TestRedactSecrets_RedactsCredentials(t *testing.T) {
	input := `{
    "keys": {
        "s3_id": "AKIAIOSFODNN7EXAMPLE",
        "s3_secret": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
    }
}`
	result := redactSecrets(input)

	if strings.Contains(result, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("s3_id should be redacted")
	}
	if strings.Contains(result, "wJalrXUtnFEMI") {
		t.Error("s3_secret should be redacted")
	}
	if !strings.Contains(result, `"s3_id": "REDACTED"`) {
		t.Errorf("expected redacted s3_id placeholder, got:\n%s", result)
	}
	if !strings.Contains(result, `"s3_secret": "REDACTED"`) {
		t.Errorf("expected redacted s3_secret placeholder, got:\n%s", result)
	}
}

func TestRedactSecrets_PreservesNonSecretFields(t *testing.T) {
	input := `{
    "repository": "/data/source",
    "storage": "s3://bucket",
    "encrypted": false,
    "keys": {
        "s3_id": "my-id",
        "s3_secret": "my-secret"
    }
}`
	result := redactSecrets(input)

	if !strings.Contains(result, `"repository": "/data/source"`) {
		t.Error("repository field should be preserved")
	}
	if !strings.Contains(result, `"storage": "s3://bucket"`) {
		t.Error("storage field should be preserved")
	}
}

func TestRedactSecrets_NullKeysUnchanged(t *testing.T) {
	input := `{"keys": null, "name": "storj"}`
	result := redactSecrets(input)
	if result != input {
		t.Errorf("null keys should be unchanged, got: %s", result)
	}
}

func TestWritePreferences_DryRun_DoesNotWriteFile(t *testing.T) {
	s, _ := newTestSetup(t, true)

	sec := &secrets.Secrets{
		Keys: map[string]string{
			"s3_id":     "AKIAIOSFODNN7EXAMPLE",
			"s3_secret": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
	}

	if err := s.WritePreferences(sec); err != nil {
		t.Fatalf("WritePreferences (dry-run): %v", err)
	}

	// Verify no prefs file written in dry-run mode
	if _, err := os.Stat(s.PrefsFile); !os.IsNotExist(err) {
		t.Error("prefs file should not exist in dry-run mode")
	}
}

// ─── Structured error type tests ────────────────────────────────────────────

func TestRunBackup_Error_StructuredType(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{Err: errors.New("backup failed")})

	_, _, err := s.RunBackup(4)
	var backupErr *apperrors.BackupError
	if !errors.As(err, &backupErr) {
		t.Fatalf("expected *BackupError, got %T", err)
	}
	if backupErr.Phase != "run" {
		t.Errorf("phase = %q, want run", backupErr.Phase)
	}
	if backupErr.Context["threads"] != "4" {
		t.Errorf("context threads = %q, want 4", backupErr.Context["threads"])
	}
}

func TestRunPrune_Error_StructuredType(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{Err: errors.New("prune failed")})

	_, _, err := s.RunPrune([]string{"-keep", "0:365"})
	var pruneErr *apperrors.PruneError
	if !errors.As(err, &pruneErr) {
		t.Fatalf("expected *PruneError, got %T", err)
	}
	if pruneErr.Phase != "run" {
		t.Errorf("phase = %q, want run", pruneErr.Phase)
	}
}

func TestRunCleanupStorage_Error_StructuredType(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{Err: errors.New("cleanup failed")})

	_, _, err := s.RunCleanupStorage()
	var pruneErr *apperrors.PruneError
	if !errors.As(err, &pruneErr) {
		t.Fatalf("expected *PruneError, got %T", err)
	}
	if pruneErr.Phase != "cleanup-storage" {
		t.Errorf("phase = %q, want cleanup-storage", pruneErr.Phase)
	}
}

// ─── Output return tests ────────────────────────────────────────────────────

func TestRunBackup_ReturnsOutput(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{Stdout: "backup output\n", Stderr: "backup warnings\n"})

	stdout, stderr, err := s.RunBackup(4)
	if err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if stdout != "backup output\n" {
		t.Errorf("stdout = %q", stdout)
	}
	if stderr != "backup warnings\n" {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunPrune_ReturnsOutput(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{Stdout: "prune output\n"})

	stdout, _, err := s.RunPrune([]string{"-keep", "0:365"})
	if err != nil {
		t.Fatalf("RunPrune: %v", err)
	}
	if stdout != "prune output\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunCleanupStorage_ReturnsOutput(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{Stdout: "cleanup output\n"})

	stdout, _, err := s.RunCleanupStorage()
	if err != nil {
		t.Fatalf("RunCleanupStorage: %v", err)
	}
	if stdout != "cleanup output\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestGetLatestRevisionInfo_ParsesCreatedAt(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{
		Stdout: "Snapshot homes revision 7 created at 2026-04-10 17:10\nSnapshot homes revision 8 created at 2026-04-10 17:25\n",
	})

	info, output, err := s.GetLatestRevisionInfo()
	if err != nil {
		t.Fatalf("GetLatestRevisionInfo() error = %v", err)
	}
	if info == nil || info.Revision != 8 {
		t.Fatalf("info = %+v", info)
	}
	if info.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be parsed")
	}
	if !strings.Contains(output, "revision 8") {
		t.Fatalf("output = %q", output)
	}
}

func TestListVisibleRevisions_ParsesAndSortsAllVisibleRevisions(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{
		Stdout: "Snapshot homes revision 7 created at 2026-04-10 17:10\nSnapshot homes revision 8 created at 2026-04-10 17:25\nSnapshot homes revision 6\n",
	})

	revisions, output, err := s.ListVisibleRevisions()
	if err != nil {
		t.Fatalf("ListVisibleRevisions() error = %v", err)
	}
	if len(revisions) != 3 {
		t.Fatalf("len(revisions) = %d, want 3", len(revisions))
	}
	if revisions[0].Revision != 8 || revisions[1].Revision != 7 || revisions[2].Revision != 6 {
		t.Fatalf("revisions = %+v", revisions)
	}
	if revisions[0].CreatedAt.IsZero() || revisions[1].CreatedAt.IsZero() {
		t.Fatalf("expected created_at timestamps to be parsed: %+v", revisions)
	}
	if !revisions[2].CreatedAt.IsZero() {
		t.Fatalf("expected revision without created at to stay zero: %+v", revisions[2])
	}
	if !strings.Contains(output, "revision 8") {
		t.Fatalf("output = %q", output)
	}
}

func TestListRevisionFiles_RunsDuplicacyListFilesForRevision(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{
		Stdout: "docs/readme.md\nmusic/song.flac\n",
	})

	output, err := s.ListRevisionFiles(2403)
	if err != nil {
		t.Fatalf("ListRevisionFiles() error = %v", err)
	}
	if output != "docs/readme.md\nmusic/song.flac\n" {
		t.Fatalf("output = %q", output)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
	invocation := mock.Invocations[0]
	if invocation.Cmd != "duplicacy" || invocation.Dir != s.DuplicacyRoot || strings.Join(invocation.Args, " ") != "list -files -r 2403" {
		t.Fatalf("invocation = %#v", invocation)
	}
}

func TestListRevisionFiles_ReturnsCombinedOutputOnFailure(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{
		Stdout: "stdout\n",
		Stderr: "stderr\n",
		Err:    errors.New("list failed"),
	})

	output, err := s.ListRevisionFiles(2403)
	if err == nil || !strings.Contains(err.Error(), "failed to list files for revision 2403") {
		t.Fatalf("error = %v", err)
	}
	if output != "stdout\nstderr\n" {
		t.Fatalf("output = %q", output)
	}
}

func TestRestoreRevision_RunsFullAndSelectiveRestoreCommands(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{Stdout: "full\n"}, execpkg.MockResult{Stdout: "selective\n"})

	if output, err := s.RestoreRevision(2403, ""); err != nil || output != "full\n" {
		t.Fatalf("RestoreRevision(full) output = %q err = %v", output, err)
	}
	if output, err := s.RestoreRevision(2404, "docs/readme.md"); err != nil || output != "selective\n" {
		t.Fatalf("RestoreRevision(selective) output = %q err = %v", output, err)
	}
	if len(mock.Invocations) != 2 {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
	if strings.Join(mock.Invocations[0].Args, " ") != "restore -r 2403 -stats" {
		t.Fatalf("full restore args = %#v", mock.Invocations[0].Args)
	}
	if strings.Join(mock.Invocations[1].Args, " ") != "restore -r 2404 -stats -- docs/readme.md" {
		t.Fatalf("selective restore args = %#v", mock.Invocations[1].Args)
	}
}

func TestRestoreRevision_IgnoreOwnerAddsDuplicacyFlag(t *testing.T) {
	s, mock := newTestSetup(t, false, execpkg.MockResult{Stdout: "selective\n"})
	s.IgnoreOwner = true

	if output, err := s.RestoreRevision(2404, "docs/readme.md"); err != nil || output != "selective\n" {
		t.Fatalf("RestoreRevision(selective) output = %q err = %v", output, err)
	}
	if len(mock.Invocations) != 1 {
		t.Fatalf("invocations = %#v", mock.Invocations)
	}
	if strings.Join(mock.Invocations[0].Args, " ") != "restore -r 2404 -stats -ignore-owner -- docs/readme.md" {
		t.Fatalf("selective restore args = %#v", mock.Invocations[0].Args)
	}
}

func TestRestoreRevision_ReturnsCombinedOutputOnFailure(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{
		Stdout: "stdout\n",
		Stderr: "stderr\n",
		Err:    errors.New("restore failed"),
	})

	output, err := s.RestoreRevision(2403, "")
	if err == nil || !strings.Contains(err.Error(), "failed to restore revision 2403") {
		t.Fatalf("error = %v", err)
	}
	if output != "stdout\nstderr\n" {
		t.Fatalf("output = %q", output)
	}
}

func TestCheckVisibleRevisions_ParsesPassAndFailResults(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{
		Stdout: "2026-04-10 00:31:11.326 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 235 exist\n" +
			"2026-04-10 00:31:12.326 INFO SNAPSHOT_CHECK Some chunks referenced by snapshot homes at revision 234 are missing\n",
		Err: errors.New("check failed"),
	})

	results, output, err := s.CheckVisibleRevisions()
	if err != nil {
		t.Fatalf("CheckVisibleRevisions() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Revision != 235 || results[0].Result != "pass" {
		t.Fatalf("results[0] = %+v", results[0])
	}
	if results[1].Revision != 234 || results[1].Result != "fail" || results[1].Message != "Missing chunks" {
		t.Fatalf("results[1] = %+v", results[1])
	}
	if !strings.Contains(output, "revision 234") {
		t.Fatalf("output = %q", output)
	}
}

func TestCheckVisibleRevisions_FailsWhenCommandCannotBeParsed(t *testing.T) {
	s, _ := newTestSetup(t, false, execpkg.MockResult{
		Stdout: "fatal failure\n",
		Err:    errors.New("check failed"),
	})

	_, _, err := s.CheckVisibleRevisions()
	if err == nil {
		t.Fatal("expected error when no revision results can be parsed")
	}
	var pruneErr *apperrors.PruneError
	if !errors.As(err, &pruneErr) {
		t.Fatalf("expected *PruneError, got %T", err)
	}
	if pruneErr.Phase != "revision-check" {
		t.Fatalf("phase = %q, want revision-check", pruneErr.Phase)
	}
}

func TestParseVisibleRevisions_DeduplicatesByRevision(t *testing.T) {
	revisions := parseVisibleRevisions("Snapshot homes revision 8 created at 2026-04-10 17:25\nSnapshot homes revision 8\nSnapshot homes revision 7\n")
	if len(revisions) != 2 {
		t.Fatalf("len(revisions) = %d, want 2", len(revisions))
	}
	if revisions[0].Revision != 8 || revisions[1].Revision != 7 {
		t.Fatalf("revisions = %+v", revisions)
	}
	if revisions[0].CreatedAt.IsZero() {
		t.Fatalf("expected revision 8 created_at to be preserved: %+v", revisions[0])
	}
}

func TestParseRevisionCheckResults_PrefersFailWhenChunkMissingLinesAppear(t *testing.T) {
	results := parseRevisionCheckResults("Chunk abc123 referenced by snapshot homes at revision 2337 does not exist\nAll chunks referenced by snapshot homes at revision 2338 exist\n")
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Revision != 2338 || results[0].Result != "pass" {
		t.Fatalf("results[0] = %+v", results[0])
	}
	if results[1].Revision != 2337 || results[1].Result != "fail" {
		t.Fatalf("results[1] = %+v", results[1])
	}
}

func TestGetTotalRevisionCount_ReturnsOutput(t *testing.T) {
	s, _ := newTestSetup(t, false,
		execpkg.MockResult{Stdout: "revision 1\nrevision 2\n"},
	)

	count, output, err := s.GetTotalRevisionCount()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if !strings.Contains(output, "revision 1") {
		t.Errorf("output should contain revision listing: %s", output)
	}
}
