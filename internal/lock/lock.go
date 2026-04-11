// Package lock provides directory-based PID locking for concurrency control.
// It uses mkdir atomicity to prevent concurrent backup runs for the same label.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

// Lock represents a directory-based process lock.
type Lock struct {
	Path    string
	PIDFile string
	pid     int
}

type Status struct {
	Path    string
	PID     int
	Present bool
	Active  bool
	Stale   bool
}

// New creates a new Lock for the given backup label.
func New(lockParent, label string) *Lock {
	return newLock(lockParent, fmt.Sprintf("backup-%s.lock.d", label))
}

// NewTarget creates a target-scoped repository lock for the given label/target pair.
func NewTarget(lockParent, label, target string) *Lock {
	return newLock(lockParent, fmt.Sprintf("backup-%s-%s.lock.d", label, target))
}

// NewSource creates a short-lived source lock for snapshot operations on a label.
func NewSource(lockParent, label string) *Lock {
	return newLock(lockParent, fmt.Sprintf("source-%s.lock.d", label))
}

func newLock(lockParent, lockName string) *Lock {
	lockPath := filepath.Join(lockParent, lockName)
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
		return apperrors.NewLockError("create-parent", fmt.Errorf("failed to create lock parent directory: %w", err), "path", filepath.Dir(l.Path))
	}

	// Try to create the lock directory atomically
	if err := os.Mkdir(l.Path, 0755); err == nil {
		return l.writePID()
	}

	// Lock exists - check if the holder is still running
	existingPID, err := l.readPID()
	if err == nil && existingPID > 0 {
		if processExists(existingPID) {
			return apperrors.NewLockError("held", fmt.Errorf("another backup is already running (PID: %d)", existingPID), "pid", strconv.Itoa(existingPID))
		}
	}

	// Stale lock - remove and retry
	os.RemoveAll(l.Path)

	if err := os.Mkdir(l.Path, 0755); err != nil {
		return apperrors.NewLockError("stale-retry", fmt.Errorf("failed to acquire lock after removing stale lock"), "path", l.Path)
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

func Inspect(lockParent, label string) (*Status, error) {
	l := New(lockParent, label)
	return inspectLock(l)
}

func InspectTarget(lockParent, label, target string) (*Status, error) {
	return inspectLock(NewTarget(lockParent, label, target))
}

func InspectSource(lockParent, label string) (*Status, error) {
	return inspectLock(NewSource(lockParent, label))
}

func inspectLock(l *Lock) (*Status, error) {
	status := &Status{Path: l.Path}

	if _, err := os.Stat(l.Path); err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return nil, apperrors.NewLockError("stat", fmt.Errorf("failed to inspect lock path: %w", err), "path", l.Path)
	}

	status.Present = true
	pid, err := l.readPID()
	if err != nil {
		status.Stale = true
		return status, nil
	}
	status.PID = pid
	status.Active = processExists(pid)
	status.Stale = !status.Active
	return status, nil
}
