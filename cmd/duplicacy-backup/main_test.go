package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
)

func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(stdout) error = %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(stderr) error = %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	doneOut := make(chan string, 1)
	doneErr := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, stdoutR)
		doneOut <- buf.String()
	}()
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, stderrR)
		doneErr <- buf.String()
	}()

	fn()

	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	return <-doneOut, <-doneErr
}

func withTestGlobals(t *testing.T, fn func()) {
	t.Helper()
	oldLogDir := logDir
	oldGeteuid := geteuid
	oldLookPath := lookPath
	oldNewLock := newLock

	logDir = t.TempDir()
	geteuid = func() int { return 0 }
	lookPath = func(string) (string, error) { return "/usr/bin/true", nil }
	lockParent := t.TempDir()
	newLock = func(_, label string) *lock.Lock { return lock.New(lockParent, label) }

	t.Cleanup(func() {
		logDir = oldLogDir
		geteuid = oldGeteuid
		lookPath = oldLookPath
		newLock = oldNewLock
	})

	fn()
}

func currentUserGroup(t *testing.T) (string, string) {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() error = %v", err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Fatalf("user.LookupGroupId() error = %v", err)
	}
	return u.Username, g.Name
}

func writeConfig(t *testing.T, dir, label, body string) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("%s-backup.toml", label))
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func TestRunWithArgs_HelpReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"--help"}); code != 0 {
			t.Fatalf("runWithArgs(--help) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "<source>-backup.toml") || !strings.Contains(stdout, "duplicacy-<label>.toml") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_VersionReturnsZero(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		if code := runWithArgs([]string{"--version"}); code != 0 {
			t.Fatalf("runWithArgs(--version) = %d", code)
		}
	})
	if !strings.Contains(stdout, scriptName) {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_InvalidFlagReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--nope"}); code != 1 {
				t.Fatalf("runWithArgs(--nope) = %d", code)
			}
		})
		if !strings.Contains(stderr, "unknown option --nope.") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_NonRootReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--fix-perms", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(non-root) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Must be run as root.") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_ConfigLoadFailureReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--fix-perms", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Configuration file not found:") || !strings.Contains(stderr, "homes-backup.toml.") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_LockAcquisitionFailureReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n")

		blocker := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		newLock = func(_, label string) *lock.Lock { return lock.New(blocker, label) }

		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--fix-perms", "--dry-run", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(lock failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Lock acquisition failed:") || !strings.Contains(stderr, "not a directory.") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_BackupDryRunReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nthreads = 4\n[local]\n")
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--backup", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(backup dry-run) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Backup phase completed (dry-run).") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "All operations completed.") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_RemoteMissingSecretsReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		secretsDir := t.TempDir()
		writeConfig(t, configDir, "homes", "[common]\ndestination = \"s3://bucket\"\nthreads = 4\n[remote]\n")
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--remote", "--dry-run", "--config-dir", configDir, "--secrets-dir", secretsDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(remote missing secrets) = %d", code)
			}
		})
		if !strings.Contains(stderr, "secrets file not found:") || !strings.Contains(stderr, "duplicacy-homes.toml.") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_FixPermsOnlyDryRunReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n")
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--fix-perms", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(fix-perms dry-run) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Permission normalisation completed (dry-run).") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "All operations completed.") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}
