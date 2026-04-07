// Package logger provides structured logging with file and console output,
// colour support, and log rotation.
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Level represents a log severity level.
type Level int

const (
	INFO Level = iota
	WARNING
	ERROR
)

func (l Level) String() string {
	switch l {
	case INFO:
		return "INFO"
	case WARNING:
		return "WARNING"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger handles logging to both stderr and a log file.
type Logger struct {
	logFile      *os.File
	enableColour bool
	scriptName   string
	logDir       string
}

// ANSI colour codes
const (
	colourReset = "\033[0m"
	colourInfo  = "\033[1;32m"
	colourWarn  = "\033[1;33m"
	colourError = "\033[1;31m"
	colourLabel = "\033[1;36m"
	colourValue = "\033[1;37m"
)

var ansiRegex = regexp.MustCompile(`\x1B\[[0-9;]*[mK]`)

// New creates a new Logger instance.
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
		enableColour: enableColour,
		scriptName:   scriptName,
		logDir:       logDir,
	}, nil
}

// Close closes the log file.
func (l *Logger) Close() {
	if l.logFile != nil {
		l.logFile.Close()
	}
}

func (l *Logger) colourForLevel(level Level) string {
	if !l.enableColour {
		return ""
	}
	switch level {
	case INFO:
		return colourInfo
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

// Log writes a log message at the given level.
func (l *Logger) Log(level Level, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	message := fmt.Sprintf("[%s] [%s] %s", timestamp, level, msg)

	// Write to stderr with colour
	colour := l.colourForLevel(level)
	fmt.Fprintf(os.Stderr, "%s%s%s\n", colour, message, l.reset())

	// Write to file without colour
	plain := ansiRegex.ReplaceAllString(message, "")
	fmt.Fprintln(l.logFile, plain)
}

// Info logs at INFO level.
func (l *Logger) Info(format string, args ...interface{}) {
	l.Log(INFO, format, args...)
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
	l.Info("[DRY-RUN] Would run: %s", msg)
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

// FormatResult returns a formatted result string (SUCCESS=green, else red).
func (l *Logger) FormatResult(result string) string {
	if !l.enableColour {
		return result
	}
	if result == "SUCCESS" {
		return fmt.Sprintf("%s%s%s", colourInfo, result, colourReset)
	}
	return fmt.Sprintf("%s%s%s", colourError, result, colourReset)
}

// PrintLine prints a label-value pair.
func (l *Logger) PrintLine(label, value string) {
	l.Info("  %s : %s", l.FormatLabel(label), l.FormatValue(value))
}

// PrintSeparator prints a visual separator.
func (l *Logger) PrintSeparator() {
	if l.enableColour {
		l.Info("%s====%s", colourInfo, colourReset)
	} else {
		l.Info("====")
	}
}

// CleanupOldLogs removes log files older than retentionDays.
func (l *Logger) CleanupOldLogs(retentionDays int, dryRun bool) {
	if retentionDays <= 0 {
		l.Info("Log retention disabled")
		return
	}

	pattern := filepath.Join(l.logDir, l.scriptName+"-*.log")
	if dryRun {
		l.DryRun("find %s -maxdepth 1 -type f -name %s-*.log -mtime +%d -delete", l.logDir, l.scriptName, retentionDays)
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
