package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ─── StripColour tests ──────────────────────────────────────────────────────

func TestStripColour(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"bold red", "\033[1;31mERROR\033[0m", "ERROR"},
		{"cyan info", "\033[0;36m[INFO] message\033[0m", "[INFO] message"},
		{"dim gray", "\033[2;37mDEBUG\033[0m", "DEBUG"},
		{"multiple codes", "\033[1;33mWARN\033[0m: \033[1;31mfailed\033[0m", "WARN: failed"},
		{"empty string", "", ""},
		{"no codes", "just text", "just text"},
		{"K escape code", "\033[2Koverwritten", "overwritten"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripColour(tt.input)
			if got != tt.want {
				t.Errorf("StripColour(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ─── IsTerminal tests ───────────────────────────────────────────────────────

func TestIsTerminal(t *testing.T) {
	// A pipe is definitely not a terminal.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if IsTerminal(r) {
		t.Error("IsTerminal(pipe) = true, want false")
	}

	// nil file should return false
	if IsTerminal(nil) {
		t.Error("IsTerminal(nil) = true, want false")
	}
}

func TestIsTerminal_RegularFile(t *testing.T) {
	// A regular file should not be a terminal
	f, err := os.CreateTemp(t.TempDir(), "term-test-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer f.Close()

	if IsTerminal(f) {
		t.Error("IsTerminal(regular file) = true, want false")
	}
}

// ─── Level.String tests ─────────────────────────────────────────────────────

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{SUCCESS, "SUCCESS"},
		{WARNING, "WARN"},
		{ERROR, "ERRO"},
		{Level(99), "UNKNOWN"},
		{Level(-1), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("Level(%d).String() = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

// ─── New tests ──────────────────────────────────────────────────────────────

func TestNew_Success(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir, "testscript", false)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer log.Close()

	// Check that log file was created
	timestamp := time.Now().Format("20060102")
	logPath := filepath.Join(dir, "testscript-"+timestamp+".log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("expected log file at %s", logPath)
	}

	// Check logger fields
	if log.scriptName != "testscript" {
		t.Errorf("scriptName = %q, want %q", log.scriptName, "testscript")
	}
	if log.logDir != dir {
		t.Errorf("logDir = %q, want %q", log.logDir, dir)
	}
	if log.enableColour != false {
		t.Error("enableColour should be false")
	}
}

func TestStartActivity_WritesOnlyToInteractiveStderr(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir, "testscript", false)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer log.Close()

	var stderr bytes.Buffer
	log.stderr = &stderr
	log.interactive = true

	stop := log.StartActivity("Inspecting repository revisions")
	stop()

	output := stderr.String()
	if !strings.Contains(output, "Status") || !strings.Contains(output, "Inspecting repository revisions") {
		t.Fatalf("stderr output = %q", output)
	}
	if !strings.Contains(output, "[INFO]") {
		t.Fatalf("expected activity output to include log level prefix, got %q", output)
	}
	if !strings.Contains(output, "[20") {
		t.Fatalf("expected activity output to include timestamp prefix, got %q", output)
	}

	timestamp := time.Now().Format("20060102")
	logPath := filepath.Join(dir, "testscript-"+timestamp+".log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}
	if strings.Contains(string(data), "Inspecting repository revisions") {
		t.Fatalf("expected activity output to stay out of log file, got %q", string(data))
	}
}

func TestNew_WithColour(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir, "test", true)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer log.Close()

	if !log.enableColour {
		t.Error("enableColour should be true")
	}
}

func TestNew_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "logdir")
	log, err := New(dir, "test", false)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer log.Close()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected New() to create the log directory")
	}
}

func TestNew_InvalidDirectory(t *testing.T) {
	// /dev/null is not a directory
	_, err := New("/dev/null/invalid", "test", false)
	if err == nil {
		t.Fatal("expected error for invalid directory")
	}
	if !strings.Contains(err.Error(), "failed to create log directory") {
		t.Errorf("error should mention directory creation, got: %v", err)
	}
}

// ─── Close tests ────────────────────────────────────────────────────────────

func TestClose(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir, "test", false)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	log.Close()
	// Calling Close() again should not panic
	log.Close()
}

func TestClose_NilLogFile(t *testing.T) {
	log := &Logger{logFile: nil}
	// Should not panic
	log.Close()
}

// ─── colourForLevel tests ───────────────────────────────────────────────────

func TestColourForLevel_ColourEnabled(t *testing.T) {
	log := &Logger{enableColour: true}

	tests := []struct {
		level    Level
		expected string
	}{
		{DEBUG, colourDebug},
		{INFO, colourInfo},
		{SUCCESS, colourSuccess},
		{WARNING, colourWarn},
		{ERROR, colourError},
		{Level(99), ""},
	}

	for _, tt := range tests {
		got := log.colourForLevel(tt.level)
		if got != tt.expected {
			t.Errorf("colourForLevel(%v) with colour = %q, want %q", tt.level, got, tt.expected)
		}
	}
}

func TestColourForLevel_ColourDisabled(t *testing.T) {
	log := &Logger{enableColour: false}

	for _, level := range []Level{DEBUG, INFO, SUCCESS, WARNING, ERROR, Level(99)} {
		got := log.colourForLevel(level)
		if got != "" {
			t.Errorf("colourForLevel(%v) without colour = %q, want empty", level, got)
		}
	}
}

// ─── reset tests ────────────────────────────────────────────────────────────

func TestReset_ColourEnabled(t *testing.T) {
	log := &Logger{enableColour: true}
	if got := log.reset(); got != colourReset {
		t.Errorf("reset() with colour = %q, want %q", got, colourReset)
	}
}

func TestReset_ColourDisabled(t *testing.T) {
	log := &Logger{enableColour: false}
	if got := log.reset(); got != "" {
		t.Errorf("reset() without colour = %q, want empty", got)
	}
}

// ─── Log and convenience method tests ───────────────────────────────────────

func newTestLogger(t *testing.T, enableColour bool) *Logger {
	t.Helper()
	dir := t.TempDir()
	log, err := New(dir, "test", enableColour)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return log
}

func readLogFile(t *testing.T, log *Logger) string {
	t.Helper()
	timestamp := time.Now().Format("20060102")
	logPath := filepath.Join(log.logDir, "test-"+timestamp+".log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	return string(data)
}

func TestLog_WritesToFile(t *testing.T) {
	log := newTestLogger(t, false)
	log.Log(INFO, "test message %d", 42)

	content := readLogFile(t, log)
	if !strings.Contains(content, "[INFO] test message 42") {
		t.Errorf("log file should contain message, got: %s", content)
	}
}

func TestLog_FileHasNoColour(t *testing.T) {
	log := newTestLogger(t, true) // colour enabled
	log.Log(ERROR, "error msg")

	content := readLogFile(t, log)
	if strings.Contains(content, "\033[") {
		t.Error("log file should not contain ANSI colour codes")
	}
	if !strings.Contains(content, "[ERRO] error msg") {
		t.Errorf("log file should contain plain message, got: %s", content)
	}
}

func TestLog_TimestampFormat(t *testing.T) {
	log := newTestLogger(t, false)
	log.Log(INFO, "timestamp test")

	content := readLogFile(t, log)
	// Should contain YYYY-MM-DD HH:MM:SS format
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(content, today) {
		t.Errorf("log should contain today's date %q, got: %s", today, content)
	}
}

func TestDebug(t *testing.T) {
	log := newTestLogger(t, false)
	log.SetVerbose(true)
	log.Debug("debug msg %s", "test")

	content := readLogFile(t, log)
	if !strings.Contains(content, "[DEBUG] debug msg test") {
		t.Errorf("expected DEBUG level in log, got: %s", content)
	}
}

func TestDebug_SuppressedWhenNotVerbose(t *testing.T) {
	log := newTestLogger(t, false)
	log.Debug("debug msg %s", "test")

	content := readLogFile(t, log)
	if strings.Contains(content, "[DEBUG] debug msg test") {
		t.Errorf("expected DEBUG level to be suppressed, got: %s", content)
	}
}

func TestInfo(t *testing.T) {
	log := newTestLogger(t, false)
	log.Info("info msg")

	content := readLogFile(t, log)
	if !strings.Contains(content, "[INFO] info msg") {
		t.Errorf("expected INFO level in log, got: %s", content)
	}
}

func TestSuccess(t *testing.T) {
	log := newTestLogger(t, false)
	log.Success("ok!")

	content := readLogFile(t, log)
	if !strings.Contains(content, "[SUCCESS] ok!") {
		t.Errorf("expected SUCCESS level in log, got: %s", content)
	}
}

func TestWarn(t *testing.T) {
	log := newTestLogger(t, false)
	log.Warn("warning %d", 1)

	content := readLogFile(t, log)
	if !strings.Contains(content, "[WARN] warning 1") {
		t.Errorf("expected WARN level in log, got: %s", content)
	}
}

func TestError(t *testing.T) {
	log := newTestLogger(t, false)
	log.Error("err: %v", "bad")

	content := readLogFile(t, log)
	if !strings.Contains(content, "[ERRO] err: bad") {
		t.Errorf("expected ERRO level in log, got: %s", content)
	}
}

func TestDryRun(t *testing.T) {
	log := newTestLogger(t, false)
	log.DryRun("some-command --flag")

	content := readLogFile(t, log)
	if !strings.Contains(content, "Dry run") || !strings.Contains(content, "some-command --flag") {
		t.Errorf("expected dry-run message, got: %s", content)
	}
}

// ─── Format method tests ────────────────────────────────────────────────────

func TestFormatLabel_NoColour(t *testing.T) {
	log := &Logger{enableColour: false}
	got := log.FormatLabel("TestLabel")
	// %-20s pads to 20 characters total
	if len(got) != 20 {
		t.Errorf("FormatLabel length = %d, want 20 (got %q)", len(got), got)
	}
	if got[:9] != "TestLabel" {
		t.Errorf("FormatLabel should start with 'TestLabel', got %q", got)
	}
}

func TestFormatLabel_WithColour(t *testing.T) {
	log := &Logger{enableColour: true}
	got := log.FormatLabel("TestLabel")
	// Should contain cyan colour code
	if !strings.Contains(got, colourLabel) {
		t.Error("FormatLabel with colour should contain cyan code")
	}
	if !strings.Contains(got, colourReset) {
		t.Error("FormatLabel with colour should contain reset code")
	}
	stripped := StripColour(got)
	if !strings.HasPrefix(stripped, "TestLabel") {
		t.Errorf("stripped FormatLabel = %q, want prefix 'TestLabel'", stripped)
	}
}

func TestFormatValue_NoColour(t *testing.T) {
	log := &Logger{enableColour: false}
	got := log.FormatValue("myvalue")
	if got != "myvalue" {
		t.Errorf("FormatValue = %q, want %q", got, "myvalue")
	}
}

func TestFormatValue_WithColour(t *testing.T) {
	log := &Logger{enableColour: true}
	got := log.FormatValue("myvalue")
	if !strings.Contains(got, colourValue) {
		t.Error("FormatValue with colour should contain value colour code")
	}
	if StripColour(got) != "myvalue" {
		t.Errorf("stripped FormatValue = %q, want %q", StripColour(got), "myvalue")
	}
}

func TestFormatResult_NoColour(t *testing.T) {
	log := &Logger{enableColour: false}
	if got := log.FormatResult("Success"); got != "Success" {
		t.Errorf("FormatResult(Success) = %q", got)
	}
	if got := log.FormatResult("Failed"); got != "Failed" {
		t.Errorf("FormatResult(Failed) = %q", got)
	}
}

func TestFormatResult_WithColour_Success(t *testing.T) {
	log := &Logger{enableColour: true}
	got := log.FormatResult("Success")
	if !strings.Contains(got, colourSuccess) {
		t.Error("FormatResult(Success) should use green colour")
	}
}

func TestFormatResult_WithColour_Failure(t *testing.T) {
	log := &Logger{enableColour: true}
	got := log.FormatResult("Failed")
	if !strings.Contains(got, colourError) {
		t.Error("FormatResult(Failed) should use red colour")
	}
}

func TestFormatResult_WithColour_Other(t *testing.T) {
	log := &Logger{enableColour: true}
	got := log.FormatResult("UNKNOWN_STATUS")
	if !strings.Contains(got, colourError) {
		t.Error("FormatResult(non-Success) should use red colour")
	}
}

// ─── PrintLine and PrintSeparator tests ─────────────────────────────────────

func TestPrintLine(t *testing.T) {
	log := newTestLogger(t, false)
	log.PrintLine("Label", "Value")

	content := readLogFile(t, log)
	if !strings.Contains(content, "Label") || !strings.Contains(content, "Value") {
		t.Errorf("PrintLine should contain label and value, got: %s", content)
	}
}

func TestPrintSeparator(t *testing.T) {
	log := newTestLogger(t, false)
	log.PrintSeparator()

	content := readLogFile(t, log)
	if !strings.Contains(content, "============================================================") {
		t.Errorf("PrintSeparator should contain equals signs, got: %s", content)
	}
}

// ─── CleanupOldLogs tests ───────────────────────────────────────────────────

func TestCleanupOldLogs_DisabledWhenZero(t *testing.T) {
	log := newTestLogger(t, false)
	log.CleanupOldLogs(0, false)

	content := readLogFile(t, log)
	if !strings.Contains(content, "Log retention disabled") {
		t.Errorf("expected retention disabled message, got: %s", content)
	}
}

func TestCleanupOldLogs_DisabledWhenNegative(t *testing.T) {
	log := newTestLogger(t, false)
	log.CleanupOldLogs(-5, false)

	content := readLogFile(t, log)
	if !strings.Contains(content, "Log retention disabled") {
		t.Errorf("expected retention disabled message, got: %s", content)
	}
}

func TestCleanupOldLogs_DryRun(t *testing.T) {
	log := newTestLogger(t, false)
	log.CleanupOldLogs(30, true)

	content := readLogFile(t, log)
	if !strings.Contains(content, "Dry run") {
		t.Errorf("expected dry-run message, got: %s", content)
	}
}

func TestCleanupOldLogs_RemovesOldFiles(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir, "test", false)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer log.Close()

	// Create an old log file and set its modification time to 60 days ago
	oldFile := filepath.Join(dir, "test-20250101.log")
	if err := os.WriteFile(oldFile, []byte("old log"), 0644); err != nil {
		t.Fatalf("failed to create old log file: %v", err)
	}
	oldTime := time.Now().AddDate(0, 0, -60)
	os.Chtimes(oldFile, oldTime, oldTime)

	// Create a recent log file
	recentFile := filepath.Join(dir, "test-20260409.log")
	if err := os.WriteFile(recentFile, []byte("recent log"), 0644); err != nil {
		t.Fatalf("failed to create recent log file: %v", err)
	}

	log.CleanupOldLogs(30, false)

	// Old file should be removed
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old log file should have been removed")
	}

	// Recent file should still exist
	if _, err := os.Stat(recentFile); os.IsNotExist(err) {
		t.Error("recent log file should not have been removed")
	}
}

func TestCleanupOldLogs_KeepsFilesWithinRetention(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir, "test", false)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer log.Close()

	// Create a file from 5 days ago (within 30-day retention)
	recentFile := filepath.Join(dir, "test-20260404.log")
	if err := os.WriteFile(recentFile, []byte("recent"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	recentTime := time.Now().AddDate(0, 0, -5)
	os.Chtimes(recentFile, recentTime, recentTime)

	log.CleanupOldLogs(30, false)

	if _, err := os.Stat(recentFile); os.IsNotExist(err) {
		t.Error("file within retention should not have been removed")
	}
}

// ─── Writer / logWriter tests ───────────────────────────────────────────────

func TestWriter_SingleLine(t *testing.T) {
	log := newTestLogger(t, false)
	w := log.Writer(INFO, "")

	n, err := w.Write([]byte("single line\n"))
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len("single line\n") {
		t.Errorf("Write() returned %d, want %d", n, len("single line\n"))
	}

	content := readLogFile(t, log)
	if !strings.Contains(content, "single line") {
		t.Errorf("log should contain 'single line', got: %s", content)
	}
}

func TestWriter_MultipleLines(t *testing.T) {
	log := newTestLogger(t, false)
	w := log.Writer(WARNING, "")

	w.Write([]byte("line1\nline2\nline3\n"))

	content := readLogFile(t, log)
	if !strings.Contains(content, "line1") || !strings.Contains(content, "line2") || !strings.Contains(content, "line3") {
		t.Errorf("log should contain all lines, got: %s", content)
	}
}

func TestWriter_WithPrefix(t *testing.T) {
	log := newTestLogger(t, false)
	w := log.Writer(INFO, "[PREFIX] ")

	w.Write([]byte("message\n"))

	content := readLogFile(t, log)
	if !strings.Contains(content, "[PREFIX] message") {
		t.Errorf("log should contain prefix, got: %s", content)
	}
}

func TestWriter_EmptyLinesSkipped(t *testing.T) {
	log := newTestLogger(t, false)
	w := log.Writer(INFO, "")

	w.Write([]byte("\n\n\n"))

	// Read log file - should only have the log file entry from the logger itself
	content := readLogFile(t, log)
	// Count INFO entries - should be zero from our write
	lines := strings.Split(strings.TrimSpace(content), "\n")
	// Filter out only lines that contain our empty write
	emptyWrites := 0
	for _, line := range lines {
		if strings.Contains(line, "[INFO]") && !strings.Contains(line, "single") {
			emptyWrites++
		}
	}
	// Empty lines should be skipped, so no INFO entries from our write
}

func TestWriter_AlwaysReturnsFullLength(t *testing.T) {
	log := newTestLogger(t, false)
	w := log.Writer(INFO, "")

	input := []byte("data with no newline")
	n, err := w.Write(input)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write() returned %d, want %d", n, len(input))
	}
}
