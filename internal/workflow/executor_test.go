package workflow

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func testExecutorLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	return log
}

func readSingleLogFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("log file count = %d, want 1", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return string(data)
}

func setTestLoggerField[T any](t *testing.T, log *logger.Logger, name string, value T) {
	t.Helper()
	field := reflect.ValueOf(log).Elem().FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("logger field %q not found", name)
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func newTempInputFile(t *testing.T, content string) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	if _, err := io.WriteString(f, content); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}
	return f
}

func TestExecutor_EnforcePrunePreview_ThresholdExceededWithoutForce(t *testing.T) {
	plan := &Plan{
		Config: PlanConfig{
			SafePruneMaxDeleteCount:   1,
			SafePruneMaxDeletePercent: 10,
		},
	}
	executor := NewExecutor(MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir()), testRuntime(), testExecutorLogger(t), execpkg.NewMockRunner(), plan)

	preview := &duplicacy.PrunePreview{
		DeleteCount:     2,
		TotalRevisions:  10,
		PercentEnforced: true,
	}

	err := executor.enforcePrunePreview(preview)
	if err == nil {
		t.Fatal("expected prune threshold error")
	}
	if got := OperatorMessage(err); got != "Refusing to continue because safe prune thresholds were exceeded" {
		t.Fatalf("OperatorMessage() = %q", got)
	}
}

func TestExecutor_LogPrunePreviewOutput_SuppressesRevisionListing(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}

	executor := NewExecutor(
		MetadataForLogDir("duplicacy-backup", "1.0.0", "now", logDir),
		testRuntime(),
		log,
		execpkg.NewMockRunner(),
		&Plan{Request: PlanRequest{Verbose: true}},
	)

	preview := &duplicacy.PrunePreview{
		Output:         "Repository set to /volume1/homes\nNo snapshot to delete\n",
		RevisionOutput: "revision 1\nrevision 2\n",
	}

	executor.logPrunePreviewOutput(preview)
	log.Close()

	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("log file count = %d, want 1", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "Preview") || !strings.Contains(output, "Repository set to /volume1/homes") {
		t.Fatalf("expected safe prune preview output, got %q", output)
	}
	if strings.Contains(output, "[REVISION-LIST]") {
		t.Fatalf("expected revision listing to be suppressed, got %q", output)
	}
}

func TestExecutorRun_BackupCommandFailureStillPrintsFailureFooter(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	defer log.Close()

	lockParent := t.TempDir()
	rt := testRuntime()
	rt.NewLock = func(_, label string) *lock.Lock {
		return lock.New(lockParent, label)
	}
	rt.NewSourceLock = func(_, label string) *lock.Lock {
		return lock.NewSource(lockParent, label)
	}
	rt.SignalNotify = func(chan<- os.Signal, ...os.Signal) {}

	workRoot := filepath.Join(t.TempDir(), "work")
	plan := &Plan{
		Request: PlanRequest{
			DoBackup:            true,
			NeedsDuplicacySetup: true,
			OperationMode:       "Backup",
		},
		Config: PlanConfig{
			LogRetentionDays: 30,
			BackupLabel:      "homes",
			Threads:          4,
		},
		Paths: PlanPaths{
			WorkRoot:       workRoot,
			DuplicacyRoot:  filepath.Join(workRoot, "duplicacy"),
			RepositoryPath: "/volume1/homes-snap",
			BackupTarget:   "/backups/homes",
		},
	}
	runner := execpkg.NewMockRunner(execpkg.MockResult{
		Stdout: "Repository set to /volume1/homes-snap\n",
		Stderr: "storage write failed\n",
		Err:    errors.New("exit status 1"),
	})

	executor := NewExecutor(MetadataForLogDir("duplicacy-backup", "1.0.0", "now", logDir), rt, log, runner, plan)
	if code := executor.Run(); code != 1 {
		t.Fatalf("Run() = %d, want 1", code)
	}

	output := readSingleLogFile(t, logDir)
	required := []string{
		"Phase: Backup",
		"Backup failed while running duplicacy backup",
		"Result",
		"Failed",
		"Code",
		"Run completed -",
	}
	for _, token := range required {
		if !strings.Contains(output, token) {
			t.Fatalf("expected %q in log output, got %q", token, output)
		}
	}
}

func TestExecutorStartVisibleRunResetsOverallStartTime(t *testing.T) {
	log := testExecutorLogger(t)
	defer log.Close()

	base := time.Date(2026, 4, 10, 16, 47, 47, 900_000_000, time.UTC)
	header := time.Date(2026, 4, 10, 16, 47, 50, 100_000_000, time.UTC)

	rt := testRuntime()
	rt.Now = func() time.Time { return header }

	plan := &Plan{
		Request: PlanRequest{OperationMode: "Storage cleanup"},
		Config:  PlanConfig{BackupLabel: "homes"},
	}

	executor := &Executor{
		meta:      MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir()),
		rt:        rt,
		log:       log,
		plan:      plan,
		view:      NewPresenter(MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir()), rt, log, false),
		startedAt: base,
		report:    NewRunReport(plan, base),
	}

	executor.startVisibleRun()

	if !executor.startedAt.Equal(header) {
		t.Fatalf("startedAt = %v, want %v", executor.startedAt, header)
	}
	if executor.report.StartedAt != formatReportTime(header) {
		t.Fatalf("report.StartedAt = %q, want %q", executor.report.StartedAt, formatReportTime(header))
	}
}

func TestPresenterPrintDurationTruncates(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	defer log.Close()

	end := time.Date(2026, 4, 10, 16, 47, 54, 100_000_000, time.UTC)
	start := time.Date(2026, 4, 10, 16, 47, 50, 900_000_000, time.UTC)

	rt := testRuntime()
	rt.Now = func() time.Time { return end }

	presenter := NewPresenter(MetadataForLogDir("duplicacy-backup", "1.0.0", "now", logDir), rt, log, false)
	presenter.PrintDuration(start)
	log.Close()

	output := readSingleLogFile(t, logDir)
	if !strings.Contains(output, "Duration") || !strings.Contains(output, "00:00:03") {
		t.Fatalf("output = %q", output)
	}
}

func TestShouldPromptForSafety(t *testing.T) {
	cases := []struct {
		name              string
		plan              *Plan
		stderrInteractive bool
		stdinTTY          bool
		want              bool
	}{
		{
			name:              "force prune interactive tty",
			plan:              &Plan{Request: PlanRequest{ForcePrune: true}},
			stderrInteractive: true,
			stdinTTY:          true,
			want:              true,
		},
		{
			name:              "cleanup storage interactive tty",
			plan:              &Plan{Request: PlanRequest{DoCleanupStore: true}},
			stderrInteractive: true,
			stdinTTY:          true,
			want:              true,
		},
		{
			name: "dry run does not prompt",
			plan: &Plan{Request: PlanRequest{
				ForcePrune: true,
				DryRun:     true,
			}},
			stderrInteractive: true,
			stdinTTY:          true,
			want:              false,
		},
		{
			name:              "non interactive does not prompt",
			plan:              &Plan{Request: PlanRequest{ForcePrune: true}},
			stderrInteractive: false,
			stdinTTY:          true,
			want:              false,
		},
		{
			name:              "non tty stdin does not prompt",
			plan:              &Plan{Request: PlanRequest{DoCleanupStore: true}},
			stderrInteractive: true,
			stdinTTY:          false,
			want:              false,
		},
		{
			name:              "safe prune does not prompt",
			plan:              &Plan{Request: PlanRequest{DoPrune: true}},
			stderrInteractive: true,
			stdinTTY:          true,
			want:              false,
		},
		{
			name:              "nil plan does not prompt",
			plan:              nil,
			stderrInteractive: true,
			stdinTTY:          true,
			want:              false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldPromptForSafety(tt.plan, tt.stderrInteractive, tt.stdinTTY); got != tt.want {
				t.Fatalf("shouldPromptForSafety() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSafetyWarnings(t *testing.T) {
	plan := &Plan{Request: PlanRequest{ForcePrune: true, DoCleanupStore: true}}
	warnings := safetyWarnings(plan)
	if len(warnings) != 2 {
		t.Fatalf("len(warnings) = %d, want 2", len(warnings))
	}
	if !strings.Contains(warnings[0], "Forced prune") {
		t.Fatalf("warnings[0] = %q", warnings[0])
	}
	if !strings.Contains(warnings[1], "Storage cleanup") {
		t.Fatalf("warnings[1] = %q", warnings[1])
	}
	if got := safetyWarnings(nil); got != nil {
		t.Fatalf("safetyWarnings(nil) = %#v", got)
	}
}

func TestExecutorConfirmSafetyRails(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		log := testExecutorLogger(t)
		defer log.Close()
		setTestLoggerField(t, log, "interactive", true)
		setTestLoggerField(t, log, "stderr", io.Discard)

		input := newTempInputFile(t, "y\n")
		executor := &Executor{
			log:  log,
			rt:   Env{Stdin: func() *os.File { return input }, StdinIsTTY: func() bool { return true }},
			plan: &Plan{Request: PlanRequest{ForcePrune: true}},
		}

		if err := executor.confirmSafetyRails(); err != nil {
			t.Fatalf("confirmSafetyRails() error = %v", err)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		log := testExecutorLogger(t)
		defer log.Close()
		setTestLoggerField(t, log, "interactive", true)
		setTestLoggerField(t, log, "stderr", io.Discard)

		input := newTempInputFile(t, "n\n")
		executor := &Executor{
			log:  log,
			rt:   Env{Stdin: func() *os.File { return input }, StdinIsTTY: func() bool { return true }},
			plan: &Plan{Request: PlanRequest{DoCleanupStore: true}},
		}

		err := executor.confirmSafetyRails()
		if err == nil || !strings.Contains(err.Error(), "interactive safety prompt") {
			t.Fatalf("confirmSafetyRails() error = %v", err)
		}
	})

	t.Run("read error", func(t *testing.T) {
		log := testExecutorLogger(t)
		defer log.Close()
		setTestLoggerField(t, log, "interactive", true)
		setTestLoggerField(t, log, "stderr", &bytes.Buffer{})

		input := newTempInputFile(t, "y\n")
		if err := input.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		executor := &Executor{
			log:  log,
			rt:   Env{Stdin: func() *os.File { return input }, StdinIsTTY: func() bool { return true }},
			plan: &Plan{Request: PlanRequest{ForcePrune: true}},
		}

		err := executor.confirmSafetyRails()
		if err == nil || !strings.Contains(err.Error(), "Could not read confirmation") {
			t.Fatalf("confirmSafetyRails() error = %v", err)
		}
	})
}

func TestExecutorRun_SafetyPromptCancellationFailsCleanly(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	defer log.Close()
	setTestLoggerField(t, log, "interactive", true)
	setTestLoggerField(t, log, "stderr", io.Discard)

	input := newTempInputFile(t, "n\n")
	rt := testRuntime()
	rt.Stdin = func() *os.File { return input }
	rt.StdinIsTTY = func() bool { return true }
	rt.SignalNotify = func(chan<- os.Signal, ...os.Signal) {}

	executor := NewExecutor(MetadataForLogDir("duplicacy-backup", "1.0.0", "now", logDir), rt, log, execpkg.NewMockRunner(), &Plan{
		Request: PlanRequest{
			ForcePrune:    true,
			OperationMode: "Prune",
		},
		Config: PlanConfig{
			BackupLabel:      "homes",
			StorageName:      "onsite-usb",
			Location:         locationLocal,
			LogRetentionDays: 30,
		},
	})

	if code := executor.Run(); code != 1 {
		t.Fatalf("Run() = %d, want 1", code)
	}

	output := readSingleLogFile(t, logDir)
	for _, token := range []string{"Run could not start", "Operation", "Prune", "Label", "homes", "Storage", "onsite-usb", "Location", locationLocal} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}
	if strings.Contains(output, "Run started -") {
		t.Fatalf("unexpected visible run header for pre-run failure:\n%s", output)
	}
	if !strings.Contains(output, "Operation cancelled at the interactive safety prompt") {
		t.Fatalf("output = %q", output)
	}
	if !strings.Contains(output, "Result") || !strings.Contains(output, "Failed") {
		t.Fatalf("output missing failure footer: %q", output)
	}
}

func TestExecutorRun_PruneOnlyStillPreparesDuplicacySetup(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	defer log.Close()

	lockParent := t.TempDir()
	workRoot := t.TempDir()
	rt := testRuntime()
	rt.NewLock = func(_, label string) *lock.Lock { return lock.New(lockParent, label) }
	rt.NewSourceLock = func(_, label string) *lock.Lock { return lock.NewSource(lockParent, label) }
	rt.SignalNotify = func(chan<- os.Signal, ...os.Signal) {}

	plan := &Plan{
		Request: PlanRequest{
			DoPrune:             true,
			DryRun:              true,
			Verbose:             true,
			NeedsDuplicacySetup: true,
			OperationMode:       "Prune",
		},
		Config: PlanConfig{
			LogRetentionDays:            30,
			BackupLabel:                 "homes",
			SafePruneMaxDeleteCount:     25,
			SafePruneMaxDeletePercent:   10,
			SafePruneMinTotalForPercent: 20,
		},
		Paths: PlanPaths{
			WorkRoot:       workRoot,
			DuplicacyRoot:  filepath.Join(workRoot, "duplicacy"),
			RepositoryPath: "/volume1/homes",
			BackupTarget:   "/backups/homes",
		},
	}

	executor := NewExecutor(MetadataForLogDir("duplicacy-backup", "1.0.0", "now", logDir), rt, log, execpkg.NewMockRunner(), plan)
	if code := executor.Run(); code != 0 {
		t.Fatalf("Run() = %d, want 0", code)
	}
	if executor.Report() == nil {
		t.Fatal("Report() = nil")
	}

	output := readSingleLogFile(t, logDir)
	for _, token := range []string{"Run started -", "Phase: Setup", "write JSON preferences", "Setup phase completed (dry-run)", "Phase: Prune", "Prune phase completed (dry-run)"} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}
	setupIdx := strings.Index(output, "Phase: Setup")
	pruneIdx := strings.Index(output, "Phase: Prune")
	if setupIdx < 0 || pruneIdx < 0 || setupIdx >= pruneIdx {
		t.Fatalf("expected setup phase before prune phase:\n%s", output)
	}
}

func TestExecutorRun_LockAcquireFailure(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	defer log.Close()

	badParent := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(badParent, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rt := testRuntime()
	rt.NewLock = func(_, label string) *lock.Lock { return lock.New(badParent, label) }
	rt.NewSourceLock = func(_, label string) *lock.Lock { return lock.NewSource(badParent, label) }
	rt.SignalNotify = func(chan<- os.Signal, ...os.Signal) {}

	executor := NewExecutor(MetadataForLogDir("duplicacy-backup", "1.0.0", "now", logDir), rt, log, execpkg.NewMockRunner(), &Plan{
		Request: PlanRequest{
			DoBackup:      true,
			DryRun:        true,
			OperationMode: "Backup",
		},
		Config: PlanConfig{
			BackupLabel:      "homes",
			StorageName:      "onsite-usb",
			Location:         locationLocal,
			LogRetentionDays: 30,
		},
	})
	if code := executor.Run(); code != 1 {
		t.Fatalf("Run() = %d, want 1", code)
	}

	output := readSingleLogFile(t, logDir)
	for _, token := range []string{"Run could not start", "Operation", "Backup", "Label", "homes", "Storage", "onsite-usb", "Location", locationLocal} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}
	if strings.Contains(output, "Run started -") {
		t.Fatalf("unexpected visible run header for pre-run failure:\n%s", output)
	}
	if !strings.Contains(output, "Cannot create the lock directory parent") {
		t.Fatalf("output = %q", output)
	}
}

func TestExecutorRun_AllOperationsDryRun(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.New(logDir, "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	defer log.Close()

	lockParent := t.TempDir()
	workRoot := t.TempDir()
	rt := testRuntime()
	rt.NewLock = func(_, label string) *lock.Lock { return lock.New(lockParent, label) }
	rt.NewSourceLock = func(_, label string) *lock.Lock { return lock.NewSource(lockParent, label) }
	rt.SignalNotify = func(chan<- os.Signal, ...os.Signal) {}

	plan := &Plan{
		Request: PlanRequest{
			DoBackup:            true,
			DoPrune:             true,
			DoCleanupStore:      true,
			DryRun:              true,
			Verbose:             true,
			NeedsDuplicacySetup: true,
			NeedsSnapshot:       true,
			OperationMode:       "Backup + Safe prune + Storage cleanup",
		},
		Config: PlanConfig{
			LogRetentionDays:            30,
			BackupLabel:                 "homes",
			SafePruneMaxDeleteCount:     25,
			SafePruneMaxDeletePercent:   10,
			SafePruneMinTotalForPercent: 20,
		},
		Paths: PlanPaths{
			WorkRoot:       workRoot,
			DuplicacyRoot:  filepath.Join(workRoot, "duplicacy"),
			RepositoryPath: "/volume1/homes-snap",
			BackupTarget:   "/backups/homes",
			SnapshotSource: "/volume1/homes",
			SnapshotTarget: "/volume1/homes-snap",
		},
	}

	executor := NewExecutor(MetadataForLogDir("duplicacy-backup", "1.0.0", "now", logDir), rt, log, execpkg.NewMockRunner(), plan)
	if code := executor.Run(); code != 0 {
		t.Fatalf("Run() = %d, want 0", code)
	}

	output := readSingleLogFile(t, logDir)
	for _, token := range []string{
		"Phase: Backup",
		"Backup phase completed (dry-run)",
		"Phase: Prune",
		"Prune phase completed (dry-run)",
		"Phase: Storage cleanup",
		"Storage cleanup phase completed (dry-run)",
	} {
		if !strings.Contains(output, token) {
			t.Fatalf("output missing %q:\n%s", token, output)
		}
	}
}
