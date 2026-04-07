// Package lock provides directory-based PID locking for concurrency control.
// It uses mkdir atomicity to prevent concurrent backup runs for the same label.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Lock represents a directory-based process lock.
type Lock struct {
	Path    string
	PIDFile string
	pid     int
}

// New creates a new Lock for the given backup label.
func New(lockParent, label string) *Lock {
	lockPath := filepath.Join(lockParent, fmt.Sprintf("backup-%s.lock.d", label))
	return &Lock{
		Path:    lockPath,
		PIDFile: filepath.Join(lockPath, "pid"),
		pid:     os.Getpid(),
	}
}

// Acquire attempts to acquire the lock. Returns an error if another process
// holds it, or removes stale locks and retries.
func (l *Lock) Acquire() error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(l.Path), 0755); err != nil {
		return fmt.Errorf("failed to create lock parent directory: %w", err)
	}

	// Try to create the lock directory atomically
	if err := os.Mkdir(l.Path, 0755); err == nil {
		return l.writePID()
	}

	// Lock exists - check if the holder is still running
	existingPID, err := l.readPID()
	if err == nil && existingPID > 0 {
		if processExists(existingPID) {
			return fmt.Errorf("another backup is already running (PID: %d)", existingPID)
		}
	}

	// Stale lock - remove and retry
	os.RemoveAll(l.Path)

	if err := os.Mkdir(l.Path, 0755); err != nil {
		return fmt.Errorf("failed to acquire lock after removing stale lock")
	}

	return l.writePID()
}

// Release removes the lock directory if owned by this process.
func (l *Lock) Release() {
	if l.Path == "" {
		return
	}

	existingPID, err := l.readPID()
	if err == nil && existingPID != l.pid {
		return // Not our lock
	}

	os.RemoveAll(l.Path)
}

func (l *Lock) writePID() error {
	return os.WriteFile(l.PIDFile, []byte(strconv.Itoa(l.pid)), 0644)
}

func (l *Lock) readPID() (int, error) {
	data, err := os.ReadFile(l.PIDFile)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func processExists(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}
