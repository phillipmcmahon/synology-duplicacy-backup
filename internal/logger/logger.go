// Package logger provides structured logging with file and console output,
// colour support with automatic TTY detection, and log rotation.
package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Level represents a log severity level.
type Level int

const (
	DEBUG Level = iota
	INFO
	SUCCESS
	WARNING
	ERROR
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case SUCCESS:
		return "SUCCESS"
	case WARNING:
		return "WARN"
	case ERROR:
		return "ERRO"
	default:
		return "UNKNOWN"
	}
}

// Logger handles logging to both stderr and a log file.
type Logger struct {
	logFile      *os.File
	stderr       io.Writer
	enableColour bool
	interactive  bool
	verbose      bool
	scriptName   string
	logDir       string
	mu           sync.Mutex
	activity     *activityState
}

type activityState struct {
	message string
	start   time.Time
	stop    chan struct{}
}

// ANSI colour codes — matched to the original bash script.
const (
	colourReset   = "\033[0m"
	colourDebug   = "\033[2;37m" // Dim gray – subdued, won't distract
	colourInfo    = "\033[1;32m" // Bold green – matches bash COLOUR_INFO
	colourSuccess = "\033[1;32m" // Bold green – matches bash success/result styling
	colourWarn    = "\033[1;33m" // Bold yellow – matches bash COLOUR_WARN
	colourError   = "\033[1;31m" // Bold red – matches bash COLOUR_ERROR
	colourLabel   = "\033[1;36m" // Bold cyan – matches bash COLOUR_LABEL
	colourValue   = "\033[1;37m" // Bold white – matches bash COLOUR_VALUE
)

var ansiRegex = regexp.MustCompile(`\x1B\[[0-9;]*[mK]`)

// StripColour removes all ANSI escape sequences from a string.
func StripColour(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// IsTerminal reports whether the given file descriptor is connected to a
// terminal (TTY). This is used to decide whether colour output is appropriate
// and whether interactive safety prompts should appear.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	return isTerminalFD(f.Fd())
}

// New creates a new Logger instance.
// enableColour controls whether ANSI colour codes are emitted to stderr.
// Callers should typically pass IsTerminal(os.Stderr) to auto-detect.
func New(logDir, scriptName string, enableColour bool) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}

	timestamp := time.Now().Format("20060102")
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", scriptName, timestamp))

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logPath, err)
	}

	return &Logger{
		logFile:      f,
		stderr:       os.Stderr,
		enableColour: enableColour,
		interactive:  IsTerminal(os.Stderr),
		verbose:      false,
		scriptName:   scriptName,
		logDir:       logDir,
	}, nil
}

// Close closes the log file.
func (l *Logger) Close() {
	l.stopActivity(nil)
	if l.logFile != nil {
		l.logFile.Close()
	}
}

func (l *Logger) colourForLevel(level Level) string {
	if !l.enableColour {
		return ""
	}
	switch level {
	case DEBUG:
		return colourDebug
	case INFO:
		return colourInfo
	case SUCCESS:
		return colourSuccess
	case WARNING:
		return colourWarn
	case ERROR:
		return colourError
	default:
		return ""
	}
}

func (l *Logger) reset() string {
	if !l.enableColour {
		return ""
	}
	return colourReset
}

// ColourizeForLevel applies the standard logger colour for a level to an
// arbitrary value without requiring a Logger instance.
func ColourizeForLevel(level Level, value string, enableColour bool) string {
	if !enableColour {
		return value
	}
	var colour string
	switch level {
	case DEBUG:
		colour = colourDebug
	case INFO:
		colour = colourInfo
	case SUCCESS:
		colour = colourSuccess
	case WARNING:
		colour = colourWarn
	case ERROR:
		colour = colourError
	default:
		return value
	}
	return fmt.Sprintf("%s%s%s", colour, value, colourReset)
}

// Log writes a log message at the given level.
func (l *Logger) Log(level Level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	message := l.formatMessage(level, msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.activity != nil {
		l.clearActivityLocked()
	}

	colour := l.colourForLevel(level)
	fmt.Fprintf(l.stderr, "%s%s%s\n", colour, message, l.reset())

	plain := StripColour(message)
	fmt.Fprintln(l.logFile, plain)

	if l.activity != nil {
		l.renderActivityLocked(l.activity)
	}
}

// Record writes a log message to the log file without echoing it to stderr.
// It is used for live terminal activity that should remain append-only in the
// persistent log without adding repeated lines to the operator's screen.
func (l *Logger) Record(level Level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	message := l.formatMessage(level, msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	plain := StripColour(message)
	fmt.Fprintln(l.logFile, plain)
}

// Debug logs at DEBUG level.
func (l *Logger) Debug(format string, args ...interface{}) {
	if !l.verbose {
		return
	}
	l.Log(DEBUG, format, args...)
}

// Info logs at INFO level.
func (l *Logger) Info(format string, args ...interface{}) {
	l.Log(INFO, format, args...)
}

// Success logs at SUCCESS level.
func (l *Logger) Success(format string, args ...interface{}) {
	l.Log(SUCCESS, format, args...)
}

// Warn logs at WARNING level.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.Log(WARNING, format, args...)
}

// Error logs at ERROR level.
func (l *Logger) Error(format string, args ...interface{}) {
	l.Log(ERROR, format, args...)
}

// DryRun logs a dry-run message.
func (l *Logger) DryRun(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.PrintLine("Dry run", msg)
}

// FormatLabel returns a formatted label string for config summaries.
func (l *Logger) FormatLabel(label string) string {
	if l.enableColour {
		return fmt.Sprintf("%s%-20s%s", colourLabel, label, colourReset)
	}
	return fmt.Sprintf("%-20s", label)
}

// FormatValue returns a formatted value string for config summaries.
func (l *Logger) FormatValue(value string) string {
	if l.enableColour {
		return fmt.Sprintf("%s%s%s", colourValue, value, colourReset)
	}
	return value
}

// FormatResult returns a formatted result string using the standard semantic
// colours for run and health summaries.
func (l *Logger) FormatResult(result string) string {
	if !l.enableColour {
		return result
	}
	switch result {
	case "Success", "Healthy":
		return fmt.Sprintf("%s%s%s", colourSuccess, result, colourReset)
	case "Degraded":
		return fmt.Sprintf("%s%s%s", colourWarn, result, colourReset)
	default:
		return fmt.Sprintf("%s%s%s", colourError, result, colourReset)
	}
}

// PrintLine prints a label-value pair.
func (l *Logger) PrintLine(label, value string) {
	l.Info("  %s : %s", l.FormatLabel(label), l.FormatValue(value))
}

// PrintSeparator prints a visual separator.
func (l *Logger) PrintSeparator() {
	l.Info("============================================================")
}

func (l *Logger) Interactive() bool {
	return l.interactive
}

func (l *Logger) StartActivity(message string) func() {
	if !l.interactive {
		return func() {}
	}

	act := &activityState{
		message: message,
		start:   time.Now(),
		stop:    make(chan struct{}),
	}

	l.stopActivity(nil)

	l.mu.Lock()
	l.activity = act
	l.renderActivityLocked(act)
	l.mu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.mu.Lock()
				if l.activity == act {
					l.renderActivityLocked(act)
				}
				l.mu.Unlock()
			case <-act.stop:
				return
			}
		}
	}()

	return func() {
		l.stopActivity(act)
	}
}

// CleanupOldLogs removes log files older than retentionDays.
func (l *Logger) CleanupOldLogs(retentionDays int, dryRun bool) {
	if retentionDays <= 0 {
		return
	}

	pattern := filepath.Join(l.logDir, l.scriptName+"-*.log")
	if dryRun {
		return
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		l.Warn("Failed to glob log files: %v", err)
		return
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	for _, f := range matches {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(f); err != nil {
				l.Warn("Failed to remove old log file %s: %v", f, err)
			}
		}
	}
}

func (l *Logger) stopActivity(act *activityState) {
	l.mu.Lock()
	defer l.mu.Unlock()

	current := l.activity
	if current == nil {
		return
	}
	if act != nil && current != act {
		return
	}

	close(current.stop)
	l.clearActivityLocked()
	l.activity = nil
}

func (l *Logger) clearActivityLocked() {
	if !l.interactive {
		return
	}
	fmt.Fprint(l.stderr, "\r\033[2K")
}

func (l *Logger) renderActivityLocked(act *activityState) {
	if !l.interactive {
		return
	}

	elapsed := time.Since(act.start)
	if elapsed < 0 {
		elapsed = 0
	}
	seconds := int(elapsed.Round(time.Second) / time.Second)
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	value := fmt.Sprintf("%s (%02d:%02d:%02d)", act.message, hours, minutes, secs)
	message := l.formatMessage(INFO, fmt.Sprintf("  %s : %s", l.FormatLabel("Status"), l.FormatValue(value)))
	fmt.Fprintf(l.stderr, "\r\033[2K%s%s%s", l.colourForLevel(INFO), message, l.reset())
}

func (l *Logger) formatMessage(level Level, msg string) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	return fmt.Sprintf("[%s] [%s] %s", timestamp, level, msg)
}

// Writer returns an io.Writer that logs each line at the given level.
func (l *Logger) Writer(level Level, prefix string) io.Writer {
	return &logWriter{logger: l, level: level, prefix: prefix}
}

type logWriter struct {
	logger *Logger
	level  Level
	prefix string
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	lines := strings.Split(strings.TrimRight(string(p), "\n"), "\n")
	for _, line := range lines {
		if line != "" {
			w.logger.Log(w.level, "%s%s", w.prefix, line)
		}
	}
	return len(p), nil
}
func (l *Logger) SetVerbose(verbose bool) {
	l.verbose = verbose
}

func (l *Logger) Verbose() bool {
	return l.verbose
}

func (l *Logger) Confirm(prompt string, input io.Reader) (bool, error) {
	reader := bufio.NewReader(input)

	l.mu.Lock()
	if l.activity != nil {
		l.clearActivityLocked()
	}
	message := l.formatMessage(WARNING, prompt)
	fmt.Fprintf(l.stderr, "%s%s%s ", l.colourForLevel(WARNING), message, l.reset())
	l.mu.Unlock()

	response, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(response)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
