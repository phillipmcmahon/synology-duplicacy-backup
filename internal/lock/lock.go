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

var (
	lockMkdirAll  = os.MkdirAll
	lockMkdir     = os.Mkdir
	lockRemoveAll = os.RemoveAll
	lockReadFile  = os.ReadFile
	lockWriteFile = os.WriteFile
	lockStat      = os.Stat
	processExists = linuxProcProcessExists
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
	if err := lockMkdirAll(filepath.Dir(l.Path), 0755); err != nil {
		return apperrors.NewLockError("create-parent", fmt.Errorf("failed to create lock parent directory: %w", err), "path", filepath.Dir(l.Path))
	}

	// Try to create the lock directory atomically
	if err := lockMkdir(l.Path, 0755); err == nil {
		return l.writePID()
	} else if !os.IsExist(err) {
		return apperrors.NewLockError("create", fmt.Errorf("failed to create lock directory: %w", err), "path", l.Path)
	}

	// Lock exists - check if the holder is still running
	existingPID, err := l.readPID()
	if err == nil && existingPID > 0 {
		if processExists(existingPID) {
			return apperrors.NewLockError("held", fmt.Errorf("another backup is already running (PID: %d)", existingPID), "pid", strconv.Itoa(existingPID))
		}
	}

	// Stale lock - remove and retry
	if err := lockRemoveAll(l.Path); err != nil {
		return apperrors.NewLockError("stale-remove", fmt.Errorf("failed to remove stale lock: %w", err), "path", l.Path)
	}

	if err := lockMkdir(l.Path, 0755); err != nil {
		if os.IsExist(err) {
			existingPID, readErr := l.readPID()
			if readErr == nil && existingPID > 0 && processExists(existingPID) {
				return apperrors.NewLockError("held", fmt.Errorf("another backup is already running (PID: %d)", existingPID), "pid", strconv.Itoa(existingPID))
			}
		}
		return apperrors.NewLockError("stale-retry", fmt.Errorf("failed to acquire lock after removing stale lock: %w", err), "path", l.Path)
	}

	return l.writePID()
}

// Release removes the lock directory if owned by this process.
// It returns true when cleanup succeeds for this process's lock or an already
// absent lock, and false when the lock belongs to another PID.
func (l *Lock) Release() bool {
	if l.Path == "" {
		return false
	}

	existingPID, err := l.readPID()
	if err == nil && existingPID != l.pid {
		return false // Not our lock
	}

	return lockRemoveAll(l.Path) == nil
}

func (l *Lock) writePID() error {
	return lockWriteFile(l.PIDFile, []byte(strconv.Itoa(l.pid)), 0644)
}

func (l *Lock) readPID() (int, error) {
	data, err := lockReadFile(l.PIDFile)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// linuxProcProcessExists checks whether a PID exists through Linux /proc.
// Synology DSM runs on Linux, which is the production target for runtime
// locking. On non-Linux development hosts without /proc, arbitrary PIDs are
// treated as absent and stale-lock tests skip active-process assertions.
func linuxProcProcessExists(pid int) bool {
	_, err := lockStat(fmt.Sprintf("/proc/%d", pid))
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

	if _, err := lockStat(l.Path); err != nil {
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
