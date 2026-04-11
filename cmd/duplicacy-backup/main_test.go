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
	oldNewSourceLock := newSourceLock

	logDir = t.TempDir()
	geteuid = func() int { return 0 }
	lookPath = func(string) (string, error) { return "/usr/bin/true", nil }
	lockParent := t.TempDir()
	newLock = func(_, label string) *lock.Lock { return lock.New(lockParent, label) }
	newSourceLock = func(_, label string) *lock.Lock { return lock.NewSource(lockParent, label) }

	t.Cleanup(func() {
		logDir = oldLogDir
		geteuid = oldGeteuid
		lookPath = oldLookPath
		newLock = oldNewLock
		newSourceLock = oldNewSourceLock
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
	if u.Username != "root" && g.Name != "root" {
		return u.Username, g.Name
	}

	for _, name := range []string{"nobody", "daemon"} {
		if _, err := user.Lookup(name); err == nil {
			u.Username = name
			break
		}
	}
	for _, name := range []string{"nogroup", "nobody", "daemon", "staff", "users"} {
		if _, err := user.LookupGroup(name); err == nil && name != "root" {
			g.Name = name
			break
		}
	}
	if u.Username == "root" || g.Name == "root" {
		t.Skip("no non-root owner/group available on this system")
	}
	return u.Username, g.Name
}

func writeConfig(t *testing.T, dir, label, body string) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("%s-local-backup.toml", label))
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeTargetConfig(t *testing.T, dir, label, target, body string) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("%s-%s-backup.toml", label, target))
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func localConfigBody(label, destination, owner, group string, threads int, prune string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "label = %q\n", label)
	fmt.Fprintf(&b, "source_path = %q\n\n", "/volume1/"+label)
	fmt.Fprintf(&b, "[target]\nname = %q\ntype = %q\n", "local", "local")
	if owner != "" || group != "" {
		b.WriteString("allow_local_accounts = true\n")
	} else {
		b.WriteString("allow_local_accounts = false\n")
	}
	if owner != "" {
		fmt.Fprintf(&b, "local_owner = %q\n", owner)
	}
	if group != "" {
		fmt.Fprintf(&b, "local_group = %q\n", group)
	}
	fmt.Fprintf(&b, "\n[storage]\ndestination = %q\nrepository = %q\n", destination, label)
	if threads > 0 {
		fmt.Fprintf(&b, "\n[capture]\nthreads = %d\n", threads)
	}
	if prune != "" {
		fmt.Fprintf(&b, "\n[retention]\nprune = %q\n", prune)
	}
	return b.String()
}

func remoteConfigBody(label, destination string, threads int, prune string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "label = %q\n", label)
	fmt.Fprintf(&b, "source_path = %q\n\n", "/volume1/"+label)
	fmt.Fprintf(&b, "[target]\nname = %q\ntype = %q\nrequires_network = true\n", "remote", "remote")
	fmt.Fprintf(&b, "\n[storage]\ndestination = %q\nrepository = %q\n", destination, label)
	if threads > 0 {
		fmt.Fprintf(&b, "\n[capture]\nthreads = %d\n", threads)
	}
	if prune != "" {
		fmt.Fprintf(&b, "\n[retention]\nprune = %q\n", prune)
	}
	return b.String()
}

func assertFailureFooter(t *testing.T, stderr string) {
	t.Helper()
	if !strings.Contains(stderr, "Result") || !strings.Contains(stderr, "Failed") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Code") || !strings.Contains(stderr, "1") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Duration") || !strings.Contains(stderr, "Run completed -") {
		t.Fatalf("stderr = %q", stderr)
	}
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
	if !strings.Contains(stdout, "config <validate|explain|paths>") ||
		!strings.Contains(stdout, "health <status|doctor|verify>") ||
		!strings.Contains(stdout, "Use --help-full for the detailed reference.") ||
		!strings.Contains(stdout, "--cleanup-storage") ||
		!strings.Contains(stdout, "--json-summary") {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout, "Current TOML keys: storj_s3_id and storj_s3_secret") ||
		strings.Contains(stdout, "DUPLICACY_BACKUP_CONFIG_DIR") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_NoArgsReturnsHelp(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs(nil); code != 0 {
			t.Fatalf("runWithArgs() = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "health <status|doctor|verify>") ||
		!strings.Contains(stdout, "Use --help-full for the detailed reference.") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_ConfigHelpReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"config", "--help"}); code != 0 {
			t.Fatalf("runWithArgs(config --help) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Config commands:") || !strings.Contains(stdout, "Use --help-full for the detailed config reference.") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_HelpFullReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"--help-full"}); code != 0 {
			t.Fatalf("runWithArgs(--help-full) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Current TOML keys: storj_s3_id, storj_s3_secret, and optional health_webhook_bearer_token") ||
		!strings.Contains(stdout, "health status            Fast read-only health summary for operators and schedulers") ||
		!strings.Contains(stdout, "health verify            Read-only integrity check across revisions found for the current label") ||
		!strings.Contains(stdout, "DUPLICACY_BACKUP_CONFIG_DIR") ||
		!strings.Contains(stdout, "config explain --target remote homes") ||
		!strings.Contains(stdout, "--json-summary           Write a machine-readable run summary to stdout") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_HealthStatusNonRootJSONFailure(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"health", "status", "--json-summary", "homes"}); code != 2 {
				t.Fatalf("runWithArgs(health status non-root) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Health commands must be run as root") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, `"check_type": "status"`) || !strings.Contains(stdout, `"status": "unhealthy"`) {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_ConfigHelpFullReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"config", "--help-full"}); code != 0 {
			t.Fatalf("runWithArgs(config --help-full) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "validate, explain, and paths operate on one label-target pair at a time.") ||
		!strings.Contains(stdout, "--help-full             Show the detailed config help message") {
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
		if !strings.Contains(stderr, "unknown option --nope") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_ExtraPositionalArgsReturnOne(t *testing.T) {
	withTestGlobals(t, func() {
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"homes", "extra"}); code != 1 {
				t.Fatalf("runWithArgs(extra args) = %d", code)
			}
		})
		if !strings.Contains(stderr, "unexpected extra arguments: extra") {
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
		if !strings.Contains(stderr, "Must be run as root") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_ConfigValidateReturnsZeroWithoutRoot(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", owner, group, 4, "-keep 0:365"))
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "validate", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(config validate) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "Config validation succeeded for homes/local") || !strings.Contains(stdout, "Target") {
			t.Fatalf("stdout = %q", stdout)
		}
		if !strings.Contains(stdout, "homes-local-backup.toml") || strings.Contains(stdout, "Not configured") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_ConfigExplainReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", owner, group, 4, "-keep 0:365"))
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "explain", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(config explain) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "Config explanation for homes/local") || !strings.Contains(stdout, "Destination") || !strings.Contains(stdout, "Local Owner") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_ConfigPathsReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "paths", "homes"}); code != 0 {
				t.Fatalf("runWithArgs(config paths) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "Resolved paths for homes") || !strings.Contains(stdout, "Config Dir") || !strings.Contains(stdout, "Secrets File") {
			t.Fatalf("stdout = %q", stdout)
		}
		if strings.Contains(stdout, "Work Dir") || strings.Contains(stdout, "Snapshot") {
			t.Fatalf("stdout = %q", stdout)
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
		if !strings.Contains(stderr, "Configuration file not found:") || !strings.Contains(stderr, "homes-local-backup.toml") {
			t.Fatalf("stderr = %q", stderr)
		}
		assertFailureFooter(t, stderr)
	})
}

func TestRunWithArgs_LockAcquisitionFailureReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", owner, group, 0, ""))

		blocker := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		newLock = func(_, label string) *lock.Lock { return lock.New(blocker, label) }
		newSourceLock = func(_, label string) *lock.Lock { return lock.NewSource(blocker, label) }

		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--fix-perms", "--dry-run", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(lock failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Cannot create the lock directory parent at") || !strings.Contains(stderr, "check that the lock parent path exists and is writable by root") {
			t.Fatalf("stderr = %q", stderr)
		}
		assertFailureFooter(t, stderr)
	})
}

func TestRunWithArgs_BackupDryRunReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", "", "", 4, ""))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--backup", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(backup dry-run) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Backup phase completed (dry-run)") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Status") || !strings.Contains(stderr, "Backing up snapshot") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Duration") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_JSONSummaryDryRunReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", "", "", 4, ""))
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--json-summary", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(json dry-run) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Run completed -") || !strings.Contains(stderr, "Backup phase completed (dry-run)") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "No primary operation specified: defaulting to backup only.") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "\"label\": \"homes\"") ||
			!strings.Contains(stdout, "\"result\": \"success\"") ||
			!strings.Contains(stdout, "\"phases\"") ||
			!strings.Contains(stdout, "\"name\": \"Backup\"") ||
			!strings.Contains(stdout, "\"duration_seconds\": 0") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_JSONSummaryVerboseDryRunKeepsStartBlockFirst(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", "", "", 4, ""))
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--json-summary", "--dry-run", "--verbose", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(json verbose dry-run) = %d", code)
			}
		})
		if strings.Contains(stderr, "Configuration parsed for") ||
			strings.Contains(stderr, "Verified '/volume1' is on a btrfs filesystem") ||
			strings.Contains(stderr, "Acquiring lock for label") ||
			strings.Contains(stderr, "Lock acquired:") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "  Label                :") ||
			strings.Contains(stderr, "  Script               :") ||
			strings.Contains(stderr, "  PID                  :") ||
			strings.Contains(stderr, "  Lock Path            :") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Phase: Cleanup") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Run started -") || !strings.Contains(stderr, "Run Summary:") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "  Notice               : No primary operation specified: defaulting to backup only") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "\"operation\": \"Backup\"") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_JSONSummaryFailureReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--json-summary", "--fix-perms", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(json failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Configuration file not found:") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "\"result\": \"failed\"") ||
			!strings.Contains(stdout, "\"failure_message\": \"Configuration file not found:") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_JSONSummaryRequestFailureReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--json-summary", "--nope"}); code != 1 {
				t.Fatalf("runWithArgs(json request failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "unknown option --nope") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "\"result\": \"failed\"") ||
			!strings.Contains(stdout, "\"failure_message\": \"unknown option --nope\"") {
			t.Fatalf("stdout = %q", stdout)
		}
		if strings.Contains(stdout, "Usage:") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_RemoteMissingSecretsReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		secretsDir := t.TempDir()
		writeTargetConfig(t, configDir, "homes", "remote", remoteConfigBody("homes", "s3://bucket", 4, ""))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--remote", "--dry-run", "--config-dir", configDir, "--secrets-dir", secretsDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(remote missing secrets) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Remote secrets file not found:") || !strings.Contains(stderr, "duplicacy-homes-remote.toml") {
			t.Fatalf("stderr = %q", stderr)
		}
		assertFailureFooter(t, stderr)
	})
}

func TestRunWithArgs_InvalidTomlConfigReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", "label = \"homes\"\nsource_path = \"/volume1/homes\"\n\n[target]\nname = \"local\"\ntype = \"local\"\nallow_local_accounts = false\n\n[storage]\ndestination = \"/backups\"\nrepository = \"homes\"\n\n[capture]\nthreads =\n")
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--backup", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(invalid toml) = %d", code)
			}
		})
		if !strings.Contains(stderr, "contains invalid TOML") {
			t.Fatalf("stderr = %q", stderr)
		}
		assertFailureFooter(t, stderr)
	})
}

func TestRunWithArgs_FixPermsOnlyDryRunReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", owner, group, 0, ""))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--fix-perms", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(fix-perms dry-run) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Fix permissions phase completed (dry-run)") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Status") || !strings.Contains(stderr, "Applying ownership and permissions") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_CleanupStorageOnlyDryRunReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", "", "", 4, ""))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--cleanup-storage", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(cleanup-storage dry-run) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Phase: Storage cleanup") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Status") || !strings.Contains(stderr, "Scanning storage for unreferenced chunks") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Storage cleanup phase completed (dry-run)") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Phase: Prune") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Storage cleanup") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_CombinedOperationsUseFixedExecutionOrder(t *testing.T) {
	withTestGlobals(t, func() {
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", owner, group, 4, "-keep 0:365"))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--prune", "--backup", "--fix-perms", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(combined dry-run) = %d", code)
			}
		})

		backupIdx := strings.Index(stderr, "Phase: Backup")
		pruneIdx := strings.Index(stderr, "Phase: Prune")
		fixPermsIdx := strings.Index(stderr, "Phase: Fix permissions")
		if backupIdx < 0 || pruneIdx < 0 || fixPermsIdx < 0 {
			t.Fatalf("stderr = %q", stderr)
		}
		if !(backupIdx < pruneIdx && pruneIdx < fixPermsIdx) {
			t.Fatalf("expected fixed phase order backup -> prune -> fix-perms, stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Backup + Safe prune + Fix permissions") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Inspecting repository revisions") || !strings.Contains(stderr, "Applying ownership and permissions") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Run Summary:") || strings.Contains(stderr, "Acquiring lock for label") || strings.Contains(stderr, "Lock acquired:") {
			t.Fatalf("expected default output to suppress technical startup details, stderr = %q", stderr)
		}
		if strings.Contains(stderr, "exec: ") {
			t.Fatalf("expected default output to suppress raw exec lines, stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_CleanupStorageUsesFixedExecutionOrder(t *testing.T) {
	withTestGlobals(t, func() {
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", owner, group, 4, "-keep 0:365"))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--cleanup-storage", "--prune", "--backup", "--fix-perms", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(cleanup-storage dry-run) = %d", code)
			}
		})

		backupIdx := strings.Index(stderr, "Phase: Backup")
		pruneIdx := strings.Index(stderr, "Phase: Prune")
		cleanupIdx := strings.Index(stderr, "Phase: Storage cleanup")
		fixPermsIdx := strings.Index(stderr, "Phase: Fix permissions")
		if backupIdx < 0 || pruneIdx < 0 || cleanupIdx < 0 || fixPermsIdx < 0 {
			t.Fatalf("stderr = %q", stderr)
		}
		if !(backupIdx < pruneIdx && pruneIdx < cleanupIdx && cleanupIdx < fixPermsIdx) {
			t.Fatalf("expected fixed phase order backup -> prune -> cleanup-storage -> fix-perms, stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Backup + Safe prune + Storage cleanup + Fix permissions") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}
