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
	"time"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("write failed")
}

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
	oldHandleConfigCommand := handleConfigCommand
	oldHandleUpdateCommand := handleUpdateCommand
	oldMaybeSendPreRunFailureNotification := maybeSendPreRunFailureNotification

	logDir = t.TempDir()
	geteuid = func() int { return 0 }
	lookPath = func(string) (string, error) { return "/usr/bin/true", nil }
	lockParent := t.TempDir()
	newLock = func(_, label string) *lock.Lock { return lock.New(lockParent, label) }
	newSourceLock = func(_, label string) *lock.Lock { return lock.NewSource(lockParent, label) }
	handleConfigCommand = workflow.HandleConfigCommand
	handleUpdateCommand = oldHandleUpdateCommand
	maybeSendPreRunFailureNotification = workflow.MaybeSendPreRunFailureNotification

	t.Cleanup(func() {
		logDir = oldLogDir
		geteuid = oldGeteuid
		lookPath = oldLookPath
		newLock = oldNewLock
		newSourceLock = oldNewSourceLock
		handleConfigCommand = oldHandleConfigCommand
		handleUpdateCommand = oldHandleUpdateCommand
		maybeSendPreRunFailureNotification = oldMaybeSendPreRunFailureNotification
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
	path := filepath.Join(dir, fmt.Sprintf("%s-backup.toml", label))
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeTargetConfig(t *testing.T, dir, label, target, body string) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("%s-backup.toml", label))
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func localConfigBody(label, destination, owner, group string, threads int, prune string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "label = %q\n", label)
	fmt.Fprintf(&b, "source_path = %q\n", "/volume1/"+label)
	if threads > 0 || prune != "" {
		b.WriteString("\n[common]\n")
	}
	if threads > 0 {
		fmt.Fprintf(&b, "threads = %d\n", threads)
	}
	if prune != "" {
		fmt.Fprintf(&b, "prune = %q\n", prune)
	}
	fmt.Fprintf(&b, "\n[targets.%s]\n", "onsite-usb")
	fmt.Fprintf(&b, "type = %q\nlocation = %q\n", "filesystem", "local")
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
	fmt.Fprintf(&b, "destination = %q\nrepository = %q\n", destination, label)
	return b.String()
}

func remoteConfigBody(label, destination string, threads int, prune string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "label = %q\n", label)
	fmt.Fprintf(&b, "source_path = %q\n", "/volume1/"+label)
	if threads > 0 || prune != "" {
		b.WriteString("\n[common]\n")
	}
	if threads > 0 {
		fmt.Fprintf(&b, "threads = %d\n", threads)
	}
	if prune != "" {
		fmt.Fprintf(&b, "prune = %q\n", prune)
	}
	fmt.Fprintf(&b, "\n[targets.%s]\n", "offsite-storj")
	fmt.Fprintf(&b, "type = %q\nlocation = %q\ndestination = %q\nrepository = %q\n", "object", "remote", destination, label)
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

func assertFailureScope(t *testing.T, stderr string, operation string, label string, target string, storageType string, location string) {
	t.Helper()
	if !strings.Contains(stderr, "Run could not start") {
		t.Fatalf("stderr = %q", stderr)
	}
	if operation != "" && (!strings.Contains(stderr, "Operation") || !strings.Contains(stderr, operation)) {
		t.Fatalf("stderr = %q", stderr)
	}
	if label != "" && (!strings.Contains(stderr, "Label") || !strings.Contains(stderr, label)) {
		t.Fatalf("stderr = %q", stderr)
	}
	if target != "" && (!strings.Contains(stderr, "Target") || !strings.Contains(stderr, target)) {
		t.Fatalf("stderr = %q", stderr)
	}
	if storageType != "" && (!strings.Contains(stderr, "Type") || !strings.Contains(stderr, storageType)) {
		t.Fatalf("stderr = %q", stderr)
	}
	if location != "" && (!strings.Contains(stderr, "Location") || !strings.Contains(stderr, location)) {
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
		!strings.Contains(stdout, "notify <test>") ||
		!strings.Contains(stdout, "update [OPTIONS]") ||
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
		!strings.Contains(stdout, "update [OPTIONS]") ||
		!strings.Contains(stdout, "notify <test>") ||
		!strings.Contains(stdout, "Use --help-full for the detailed reference.") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRun_UsesCLIArgs(t *testing.T) {
	oldCliArgs := cliArgs
	cliArgs = func() []string { return []string{"--help"} }
	t.Cleanup(func() { cliArgs = oldCliArgs })

	stdout, stderr := captureOutput(t, func() {
		if code := run(); code != 0 {
			t.Fatalf("run() = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stdout, "Use --help-full for the detailed reference.") {
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

func TestRunWithArgs_NotifyHelpReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"notify", "--help"}); code != 0 {
			t.Fatalf("runWithArgs(notify --help) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Notify commands:") || !strings.Contains(stdout, "Use --help-full for the detailed notify reference.") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_UpdateHelpReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"update", "--help"}); code != 0 {
			t.Fatalf("runWithArgs(update --help) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Update options:") || !strings.Contains(stdout, "Use --help-full for the detailed update reference.") {
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
	if !strings.Contains(stdout, "Use [targets.<name>] tables with:") ||
		!strings.Contains(stdout, "notify test             Send a clearly marked simulated notification through the configured providers") ||
		!strings.Contains(stdout, "update                  Check GitHub for a newer published release and install it through the packaged installer") ||
		!strings.Contains(stdout, "storj_s3_id") ||
		!strings.Contains(stdout, "storj_s3_secret") ||
		!strings.Contains(stdout, "health_webhook_bearer_token") ||
		!strings.Contains(stdout, "health_ntfy_token") ||
		!strings.Contains(stdout, "health status            Fast read-only health summary for operators and schedulers") ||
		!strings.Contains(stdout, "health verify            Read-only integrity check across revisions found for the current label") ||
		!strings.Contains(stdout, "DUPLICACY_BACKUP_CONFIG_DIR") ||
		!strings.Contains(stdout, "config explain --target offsite-storj homes") ||
		!strings.Contains(stdout, "--json-summary           Write a machine-readable run summary to stdout") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_NotifyHelpFullReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"notify", "--help-full"}); code != 0 {
			t.Fatalf("runWithArgs(notify --help-full) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "--provider <name>       One of all, webhook, or ntfy (default: all)") ||
		!strings.Contains(stdout, "notify test --target onsite-usb homes") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_UpdateHelpFullReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"update", "--help-full"}); code != 0 {
			t.Fatalf("runWithArgs(update --help-full) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "--check-only           Show the planned update without downloading or installing") ||
		!strings.Contains(stdout, "--force                Reinstall even when the selected release is already current") ||
		!strings.Contains(stdout, "/usr/local/bin/duplicacy-backup -> /usr/local/lib/duplicacy-backup/current") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_UpdateCheckOnlyReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		handleUpdateCommand = func(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
			if req.UpdateCommand != "update" || !req.UpdateCheckOnly || !req.UpdateForce || req.UpdateKeep != 2 {
				t.Fatalf("req = %+v", req)
			}
			return "update ok\n", nil
		}

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"update", "--check-only", "--force"}); code != 0 {
				t.Fatalf("runWithArgs(update --check-only) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if stdout != "update ok\n" {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_HealthStatusLoggerInitFailureJSONReturnsTwo(t *testing.T) {
	withTestGlobals(t, func() {
		logFilePath := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(logFilePath, []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		logDir = logFilePath

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"health", "status", "--target", "onsite-usb", "--json-summary", "homes"}); code != 2 {
				t.Fatalf("runWithArgs(health status logger init failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Failed to initialise logger") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, `"status": "unhealthy"`) ||
			!strings.Contains(stdout, `"check_type": "status"`) ||
			!strings.Contains(stdout, `"target": "onsite-usb"`) {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_HealthStatusNonRootJSONFailure(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"health", "status", "--target", "onsite-usb", "--json-summary", "homes"}); code != 2 {
				t.Fatalf("runWithArgs(health status non-root) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Health commands must be run as root") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Failed to initialise logger") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, `"check_type": "status"`) || !strings.Contains(stdout, `"status": "unhealthy"`) {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_RuntimeLoggerInitFailureJSONReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		logFilePath := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(logFilePath, []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		logDir = logFilePath

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--target", "onsite-usb", "--fix-perms", "--json-summary", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(runtime logger init failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Failed to initialise logger") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, `"result": "failed"`) ||
			!strings.Contains(stdout, `"target": "onsite-usb"`) {
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
	if !strings.Contains(stdout, "validate, explain, and paths operate on one selected target from a label config at a time.") ||
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

func TestRunWithArgs_InvalidFlagDoesNotRequireLogger(t *testing.T) {
	withTestGlobals(t, func() {
		logFilePath := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(logFilePath, []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		logDir = logFilePath

		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--nope"}); code != 1 {
				t.Fatalf("runWithArgs(--nope) = %d", code)
			}
		})

		if !strings.Contains(stderr, "unknown option --nope") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Failed to initialise logger") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_ExtraPositionalArgsReturnOne(t *testing.T) {
	withTestGlobals(t, func() {
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--target", "onsite-usb", "homes", "extra"}); code != 1 {
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
			if code := runWithArgs([]string{"--target", "onsite-usb", "--fix-perms", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(non-root) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Must be run as root") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Failed to initialise logger") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_NonRootPruneJSONFailureDoesNotRequireLogger(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--target", "onsite-usb", "--prune", "--json-summary", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(non-root prune json) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Must be run as root") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Failed to initialise logger") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, `"result": "failed"`) || !strings.Contains(stdout, `"failure_message": "Must be run as root"`) {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_ConfigValidateReturnsZeroWithoutRoot(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		handleConfigCommand = func(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
			return "Config validation succeeded for homes/onsite-usb\n  Section: Resolved\n    Label              : homes\n    Target             : onsite-usb\n    Config File        : /tmp/homes-backup.toml\n  Section: Validation\n    Config             : Valid\n    Source Path Access : Readable\n    Btrfs Source       : Valid\n    Required Settings  : Valid\n    Health Thresholds  : Valid\n    Destination Access : Writable\n    Repository Access  : Valid\n  Result               : Passed\n", nil
		}
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "validate", "--target", "onsite-usb", "--config-dir", t.TempDir(), "homes"}); code != 0 {
				t.Fatalf("runWithArgs(config validate) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "Config validation succeeded for homes/onsite-usb") || !strings.Contains(stdout, "Section: Resolved") || !strings.Contains(stdout, "Section: Validation") {
			t.Fatalf("stdout = %q", stdout)
		}
		for _, token := range []string{"Label", "homes", "Target", "onsite-usb", "Config File", "homes-backup.toml", "Result", "Passed"} {
			if !strings.Contains(stdout, token) {
				t.Fatalf("stdout missing %q:\n%s", token, stdout)
			}
		}
		if strings.Contains(stdout, "Not configured") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_ConfigValidateVerboseReturnsZeroWithoutRoot(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		handleConfigCommand = func(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
			if !req.Verbose {
				t.Fatalf("expected verbose request, got %+v", req)
			}
			return "Config validation succeeded for homes/onsite-usb\n  Result               : Passed\n", nil
		}
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "validate", "--verbose", "--target", "onsite-usb", "--config-dir", t.TempDir(), "homes"}); code != 0 {
				t.Fatalf("runWithArgs(config validate verbose) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "Config validation succeeded for homes/onsite-usb") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_ConfigValidateMissingTargetDoesNotRequireLogger(t *testing.T) {
	withTestGlobals(t, func() {
		logFilePath := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(logFilePath, []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		logDir = logFilePath

		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "validate", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config validate missing target) = %d", code)
			}
		})

		if !strings.Contains(stderr, "--target is required") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Failed to initialise logger") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_ConfigValidateFailurePrintsReportAndReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		handleConfigCommand = func(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
			return "", &workflow.ConfigCommandError{
				Message: "Config validation failed for homes/onsite-usb",
				Output:  "Config validation failed for homes/onsite-usb\n  Section: Resolved\n    Label              : homes\n    Target             : onsite-usb\n    Config File        : /tmp/homes-backup.toml\n  Section: Validation\n    Config             : Valid\n    Source Path Access : Invalid (source_path does not exist: /volume1/homes/nested)\n    Btrfs Source       : Not checked\n    Required Settings  : Valid\n    Destination Access : Writable\n    Repository Access  : Not checked\n  Result               : Failed\n",
			}
		}
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "validate", "--target", "onsite-usb", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config validate failure) = %d", code)
			}
		})
		if !strings.Contains(stdout, "Config validation failed for homes/onsite-usb") || !strings.Contains(stdout, "Section: Validation") || !strings.Contains(stdout, "Source Path Access") {
			t.Fatalf("stdout = %q", stdout)
		}
		for _, token := range []string{"Label", "homes", "Target", "onsite-usb", "Result", "Failed"} {
			if !strings.Contains(stdout, token) {
				t.Fatalf("stdout missing %q:\n%s", token, stdout)
			}
		}
		if !strings.Contains(stderr, "Config validation failed for homes/onsite-usb") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_ConfigValidatePermissionDeniedReportsAccessIssue(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		handleConfigCommand = func(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
			return "", apperrors.NewConfigError(
				"open",
				fmt.Errorf("cannot open config file /usr/local/lib/duplicacy-backup/.config/homes-backup.toml: %w", os.ErrPermission),
				"path", "/usr/local/lib/duplicacy-backup/.config/homes-backup.toml",
			)
		}
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "validate", "--target", "offsite-storj", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config validate permission denied) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Config file is not accessible: /usr/local/lib/duplicacy-backup/.config/homes-backup.toml") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "run as root or grant read and directory traverse access to the config path") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Config file not found") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_ConfigValidateUninitializedRepositoryPrintsHint(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		handleConfigCommand = func(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
			return "", &workflow.ConfigCommandError{
				Message: "Config validation failed for homes/offsite-storj; initialize the repository before running backups",
				Output:  "Config validation failed for homes/offsite-storj\n  Section: Resolved\n    Label              : homes\n    Target             : offsite-storj\n    Config File        : /tmp/homes-backup.toml\n  Section: Validation\n    Config             : Valid\n    Source Path Access : Readable\n    Btrfs Source       : Valid\n    Required Settings  : Valid\n    Health Thresholds  : Valid\n    Destination Access : Resolved\n    Secrets            : Valid\n    Repository Access  : Not initialized\n  Result               : Failed\n",
			}
		}
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "validate", "--target", "offsite-storj", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config validate uninitialized repository) = %d", code)
			}
		})
		if !strings.Contains(stdout, "Repository Access") || !strings.Contains(stdout, "Not initialized") {
			t.Fatalf("stdout = %q", stdout)
		}
		if !strings.Contains(stderr, "initialize the repository before running backups") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_ConfigValidateInaccessibleRepositoryDoesNotPrintInitHint(t *testing.T) {
	withTestGlobals(t, func() {
		oldHandle := handleConfigCommand
		handleConfigCommand = func(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
			return "", &workflow.ConfigCommandError{
				Message: "Config validation failed for homes/offsite-storj",
				Output:  "Config validation failed for homes/offsite-storj\n  Section: Resolved\n    Label              : homes\n    Target             : offsite-storj\n    Config File        : /tmp/homes-backup.toml\n  Section: Validation\n    Config             : Valid\n    Source Path Access : Readable\n    Btrfs Source       : Valid\n    Required Settings  : Valid\n    Health Thresholds  : Valid\n    Destination Access : Resolved\n    Secrets            : Valid\n    Repository Access  : Invalid (Repository is not ready)\n  Result               : Failed\n",
			}
		}
		t.Cleanup(func() { handleConfigCommand = oldHandle })

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "validate", "--target", "offsite-storj", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config validate) = %d", code)
			}
		})

		if !strings.Contains(stdout, "Repository Access") || !strings.Contains(stdout, "Invalid (Repository is not ready)") {
			t.Fatalf("stdout = %q", stdout)
		}
		if strings.Contains(stderr, "initialize the repository before running backups") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_ConfigExplainReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", owner, group, 4, "-keep 0:365"))
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "explain", "--target", "onsite-usb", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(config explain) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "Config explanation for homes/onsite-usb") || !strings.Contains(stdout, "Destination") || !strings.Contains(stdout, "Local Owner") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_ConfigPathsReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "paths", "--target", "onsite-usb", "homes"}); code != 0 {
				t.Fatalf("runWithArgs(config paths) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "Resolved paths for homes") || !strings.Contains(stdout, "Config Dir") || !strings.Contains(stdout, "Config File") {
			t.Fatalf("stdout = %q", stdout)
		}
		if strings.Contains(stdout, "Secrets File") || strings.Contains(stdout, "Secrets Dir") {
			t.Fatalf("stdout should omit secrets for local-only targets: %q", stdout)
		}
		if strings.Contains(stdout, "Work Dir") || strings.Contains(stdout, "Snapshot") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_NotifyTestDryRunReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", strings.Join([]string{
			`label = "homes"`,
			`source_path = "/volume1/homes"`,
			`[health.notify.ntfy]`,
			`url = "https://ntfy.sh"`,
			`topic = "duplicacy-alerts"`,
			`[targets.onsite-usb]`,
			`type = "filesystem"`,
			`location = "local"`,
			`destination = "/backups"`,
			`repository = "homes"`,
		}, "\n"))

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"notify", "test", "--dry-run", "--json-summary", "--target", "onsite-usb", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(notify test dry-run) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, `"command": "test"`) ||
			!strings.Contains(stdout, `"provider": "all"`) ||
			!strings.Contains(stdout, `"result": "preview"`) {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_NotifyTestFailurePrintsReportAndReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", strings.Join([]string{
			`label = "homes"`,
			`source_path = "/volume1/homes"`,
			`[targets.onsite-usb]`,
			`type = "filesystem"`,
			`location = "local"`,
			`destination = "/backups"`,
			`repository = "homes"`,
		}, "\n"))

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"notify", "test", "--target", "onsite-usb", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(notify test failure) = %d", code)
			}
		})
		if !strings.Contains(stdout, "Notification test for homes/onsite-usb") ||
			!strings.Contains(stdout, "Result") ||
			!strings.Contains(stdout, "Failed") {
			t.Fatalf("stdout = %q", stdout)
		}
		if !strings.Contains(stderr, "Notification test failed for homes/onsite-usb") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_ConfigLoadFailureReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--target", "onsite-usb", "--fix-perms", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Configuration file not found:") || !strings.Contains(stderr, "homes-backup.toml") {
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
			if code := runWithArgs([]string{"--target", "onsite-usb", "--fix-perms", "--dry-run", "--config-dir", configDir, "homes"}); code != 1 {
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
			if code := runWithArgs([]string{"--target", "onsite-usb", "--backup", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
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
			if code := runWithArgs([]string{"--target", "onsite-usb", "--backup", "--json-summary", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(json dry-run) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Run completed -") || !strings.Contains(stderr, "Backup phase completed (dry-run)") {
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
			if code := runWithArgs([]string{"--target", "onsite-usb", "--backup", "--json-summary", "--dry-run", "--verbose", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(json verbose dry-run) = %d", code)
			}
		})
		if strings.Contains(stderr, "Configuration parsed for") ||
			strings.Contains(stderr, "Verified '/volume1' is on a btrfs filesystem") ||
			strings.Contains(stderr, "Acquiring lock for label") ||
			strings.Contains(stderr, "Lock acquired:") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "  Script               :") ||
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
		if !strings.Contains(stderr, "Label") || !strings.Contains(stderr, "homes") || !strings.Contains(stderr, "Target") || !strings.Contains(stderr, "onsite-usb") {
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
			if code := runWithArgs([]string{"--target", "onsite-usb", "--json-summary", "--fix-perms", "--config-dir", configDir, "homes"}); code != 1 {
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

func TestRunWithArgs_PreRunBackupFailureSendsNotificationWhenConfigured(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", strings.Join([]string{
			`label = "homes"`,
			`source_path = "/volume1/homes"`,
			``,
			`[targets.onsite-usb]`,
			`type = "filesystem"`,
			`location = "local"`,
			`destination = "/backups"`,
			`repository = "homes"`,
			``,
			`[health.notify]`,
			`webhook_url = "http://127.0.0.1/unused"`,
			`notify_on = ["degraded", "unhealthy"]`,
			`send_for = ["backup"]`,
		}, "\n"))

		captured := make(chan *workflow.Request, 1)
		maybeSendPreRunFailureNotification = func(rt workflow.Runtime, interactive bool, plan *workflow.Plan, req *workflow.Request, startedAt, completedAt time.Time, err error) error {
			captured <- req
			return nil
		}

		lookPath = func(name string) (string, error) {
			if name == "duplicacy" {
				return "", fmt.Errorf("not found")
			}
			return "/usr/bin/true", nil
		}

		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--target", "onsite-usb", "--backup", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(pre-run notification failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Required command 'duplicacy' not found") {
			t.Fatalf("stderr = %q", stderr)
		}

		var req *workflow.Request
		select {
		case req = <-captured:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for pre-run notification call")
		}
		if req == nil || req.Source != "homes" || req.Target() != "onsite-usb" || !req.DoBackup {
			t.Fatalf("captured request = %#v", req)
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
		writeTargetConfig(t, configDir, "homes", "offsite-storj", remoteConfigBody("homes", "s3://bucket", 4, ""))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--target", "offsite-storj", "--backup", "--dry-run", "--config-dir", configDir, "--secrets-dir", secretsDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(remote missing secrets) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Secrets file not found:") || !strings.Contains(stderr, "homes-secrets.toml") {
			t.Fatalf("stderr = %q", stderr)
		}
		assertFailureScope(t, stderr, "Backup", "homes", "offsite-storj", "object", "remote")
		assertFailureFooter(t, stderr)
	})
}

func TestRunWithArgs_InvalidTomlConfigReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", "label = \"homes\"\nsource_path = \"/volume1/homes\"\n\n[targets.onsite-usb]\ntype = \"local\"\nallow_local_accounts = false\ndestination = \"/backups\"\nrepository = \"homes\"\nthreads =\n")
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--target", "onsite-usb", "--backup", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(invalid toml) = %d", code)
			}
		})
		if !strings.Contains(stderr, "contains invalid TOML") {
			t.Fatalf("stderr = %q", stderr)
		}
		assertFailureScope(t, stderr, "Backup", "homes", "onsite-usb", "", "")
		assertFailureFooter(t, stderr)
	})
}

func TestRunWithArgs_ObjectFixPermsFailureIncludesStorageIdentity(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeTargetConfig(t, configDir, "homes", "offsite-storj", remoteConfigBody("homes", "s3://bucket", 4, ""))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--target", "offsite-storj", "--fix-perms", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(object fix-perms failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "fix-perms is only supported for filesystem targets") {
			t.Fatalf("stderr = %q", stderr)
		}
		assertFailureScope(t, stderr, "Fix permissions", "homes", "offsite-storj", "object", "remote")
		assertFailureFooter(t, stderr)
	})
}

func TestRunWithArgs_FixPermsOnlyDryRunReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", owner, group, 0, ""))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--target", "onsite-usb", "--fix-perms", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
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
			if code := runWithArgs([]string{"--target", "onsite-usb", "--cleanup-storage", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(cleanup-storage dry-run) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Phase: Storage cleanup") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Phase: Setup") || !strings.Contains(stderr, "Setup phase completed (dry-run)") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Status") || !strings.Contains(stderr, "Scanning storage for unreferenced chunks") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "Storage cleanup phase completed (dry-run)") {
			t.Fatalf("stderr = %q", stderr)
		}
		setupIdx := strings.Index(stderr, "Phase: Setup")
		cleanupIdx := strings.Index(stderr, "Phase: Storage cleanup")
		if setupIdx < 0 || cleanupIdx < 0 || setupIdx >= cleanupIdx {
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
			if code := runWithArgs([]string{"--target", "onsite-usb", "--prune", "--backup", "--fix-perms", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
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
			if code := runWithArgs([]string{"--target", "onsite-usb", "--cleanup-storage", "--prune", "--backup", "--fix-perms", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
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

func TestInferHealthFailureRequest(t *testing.T) {
	req := inferHealthFailureRequest([]string{
		"health", "verify", "--target", "offsite-storj", "--json-summary", "--verbose",
		"--config-dir", "/cfg", "--secrets-dir", "/sec", "homes", "ignored",
	})
	if req.HealthCommand != "verify" {
		t.Fatalf("HealthCommand = %q", req.HealthCommand)
	}
	if req.RequestedTarget != "offsite-storj" {
		t.Fatalf("RequestedTarget = %q", req.RequestedTarget)
	}
	if !req.JSONSummary || !req.Verbose {
		t.Fatalf("req = %+v", req)
	}
	if req.Source != "homes" {
		t.Fatalf("Source = %q", req.Source)
	}
}

func TestInferHealthFailureRequest_NonHealthCommandReturnsEmptyRequest(t *testing.T) {
	req := inferHealthFailureRequest([]string{"--target", "onsite-usb", "homes"})
	if req.HealthCommand != "" || req.RequestedTarget != "" || req.Source != "" || req.JSONSummary || req.Verbose {
		t.Fatalf("req = %+v", req)
	}
}

func TestEmitJSONFailureSummary(t *testing.T) {
	startedAt := time.Unix(100, 0).UTC()
	completedAt := time.Unix(130, 0).UTC()

	emitJSONFailureSummary(nil, nil, nil, startedAt, completedAt, "ignored")

	var buf bytes.Buffer
	emitJSONFailureSummary(&buf, &workflow.Request{Source: "homes", RequestedTarget: "onsite-usb"}, &workflow.Plan{StorageType: "filesystem", Location: "local"}, startedAt, completedAt, "boom")
	if !strings.Contains(buf.String(), `"result": "failed"`) || !strings.Contains(buf.String(), `"target": "onsite-usb"`) || !strings.Contains(buf.String(), `"storage_type": "filesystem"`) || !strings.Contains(buf.String(), `"location": "local"`) {
		t.Fatalf("summary = %q", buf.String())
	}
}

func TestEmitJSONFailureSummary_WriteFailureReportsError(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		emitJSONFailureSummary(errWriter{}, nil, nil, time.Unix(100, 0).UTC(), time.Unix(130, 0).UTC(), "boom")
	})
	if !strings.Contains(stderr, "Failed to write JSON summary") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestBuildRequest_JSONSummaryHealthFailureInfersRequest(t *testing.T) {
	meta := workflow.DefaultMetadata(scriptName, version, buildTime, t.TempDir())
	rt := workflow.DefaultRuntime()

	stdout, stderr := captureOutput(t, func() {
		result, code := buildRequest([]string{"health", "verify", "--json-summary", "--target", "offsite-storj"}, meta, rt)
		if code != 1 {
			t.Fatalf("buildRequest() code = %d", code)
		}
		if result != nil {
			t.Fatalf("buildRequest() result = %#v, want nil", result)
		}
	})
	if !strings.Contains(stdout, `"check_type": "verify"`) || !strings.Contains(stdout, `"target": "offsite-storj"`) {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "source directory required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestBuildRequest_JSONSummaryNotifyFailureInfersRequest(t *testing.T) {
	meta := workflow.DefaultMetadata(scriptName, version, buildTime, t.TempDir())
	rt := workflow.DefaultRuntime()

	stdout, stderr := captureOutput(t, func() {
		result, code := buildRequest([]string{"notify", "test", "--json-summary", "--target", "offsite-storj"}, meta, rt)
		if code != 1 {
			t.Fatalf("buildRequest() code = %d", code)
		}
		if result != nil {
			t.Fatalf("buildRequest() result = %#v, want nil", result)
		}
	})
	if !strings.Contains(stdout, `"command": "test"`) ||
		!strings.Contains(stdout, `"provider": "all"`) ||
		!strings.Contains(stdout, `"target": "offsite-storj"`) ||
		!strings.Contains(stdout, `"result": "failed"`) {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "source directory required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestInferNotifyFailureRequest(t *testing.T) {
	req := inferNotifyFailureRequest([]string{
		"notify", "test",
		"--target", "offsite-storj",
		"--provider", "ntfy",
		"--severity", "critical",
		"--summary", "Synthetic summary",
		"--message", "Synthetic message",
		"--dry-run",
		"--json-summary",
		"--config-dir", "/cfg",
		"--secrets-dir", "/sec",
		"homes", "ignored",
	})
	if req.NotifyCommand != "test" {
		t.Fatalf("NotifyCommand = %q", req.NotifyCommand)
	}
	if req.RequestedTarget != "offsite-storj" {
		t.Fatalf("RequestedTarget = %q", req.RequestedTarget)
	}
	if req.NotifyProvider != "ntfy" || req.NotifySeverity != "critical" {
		t.Fatalf("req = %+v", req)
	}
	if req.NotifySummary != "Synthetic summary" || req.NotifyMessage != "Synthetic message" {
		t.Fatalf("req = %+v", req)
	}
	if !req.DryRun || !req.JSONSummary {
		t.Fatalf("req = %+v", req)
	}
	if req.Source != "homes" {
		t.Fatalf("Source = %q", req.Source)
	}
}

func TestInferNotifyFailureRequest_NonNotifyCommandReturnsDefaults(t *testing.T) {
	req := inferNotifyFailureRequest([]string{"--target", "onsite-usb1", "homes"})
	if req.NotifyCommand != "" || req.RequestedTarget != "" || req.Source != "" || req.DryRun || req.JSONSummary {
		t.Fatalf("req = %+v", req)
	}
	if req.NotifyProvider != "all" || req.NotifySeverity != "warning" {
		t.Fatalf("req = %+v", req)
	}
}

func TestBuildRequest_JSONSummaryParseFailureDoesNotRequireLogger(t *testing.T) {
	logFilePath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(logFilePath, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	meta := workflow.DefaultMetadata(scriptName, version, buildTime, logFilePath)
	rt := workflow.DefaultRuntime()

	stdout, stderr := captureOutput(t, func() {
		result, code := buildRequest([]string{"--json-summary", "--nope"}, meta, rt)
		if code != 1 {
			t.Fatalf("buildRequest() code = %d", code)
		}
		if result != nil {
			t.Fatalf("buildRequest() result = %#v, want nil", result)
		}
	})
	if strings.Contains(stderr, "Failed to initialise logger") || !strings.Contains(stderr, "--nope") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stdout, `"result": "failed"`) {
		t.Fatalf("stdout = %q", stdout)
	}
}
