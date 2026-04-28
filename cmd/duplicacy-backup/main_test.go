package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/command"
	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/update"
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
	oldIsSynologyDSM := isSynologyDSM
	oldNewLock := newLock
	oldNewSourceLock := newSourceLock
	oldHandleConfigCommand := handleConfigCommand
	oldHandleDiagnosticsCommand := handleDiagnosticsCommand
	oldHandleRestoreCommand := handleRestoreCommand
	oldHandleRollbackCommand := handleRollbackCommand
	oldHandleUpdateCommand := handleUpdateCommand
	oldMaybeSendPreRunFailureNotification := maybeSendPreRunFailureNotification

	logDir = t.TempDir()
	geteuid = func() int { return 0 }
	t.Setenv("SUDO_USER", "operator")
	t.Setenv("SUDO_UID", "1000")
	lookPath = func(string) (string, error) { return "/usr/bin/true", nil }
	isSynologyDSM = func() bool { return true }
	lockParent := t.TempDir()
	newLock = func(_, label string) *lock.Lock { return lock.New(lockParent, label) }
	newSourceLock = func(_, label string) *lock.Lock { return lock.NewSource(lockParent, label) }
	handleConfigCommand = workflow.HandleConfigCommand
	handleDiagnosticsCommand = workflow.HandleDiagnosticsCommand
	handleRestoreCommand = workflow.HandleRestoreCommand
	handleRollbackCommand = oldHandleRollbackCommand
	handleUpdateCommand = oldHandleUpdateCommand
	maybeSendPreRunFailureNotification = workflow.MaybeSendPreRunFailureNotification

	t.Cleanup(func() {
		logDir = oldLogDir
		geteuid = oldGeteuid
		lookPath = oldLookPath
		isSynologyDSM = oldIsSynologyDSM
		newLock = oldNewLock
		newSourceLock = oldNewSourceLock
		handleConfigCommand = oldHandleConfigCommand
		handleDiagnosticsCommand = oldHandleDiagnosticsCommand
		handleRestoreCommand = oldHandleRestoreCommand
		handleRollbackCommand = oldHandleRollbackCommand
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

func writeUpdateNotifyAppConfig(t *testing.T, dir, ntfyURL, notifyOn string) string {
	t.Helper()
	path := filepath.Join(dir, "duplicacy-backup.toml")
	body := strings.Join([]string{
		`[update.notify]`,
		fmt.Sprintf(`notify_on = [%q]`, notifyOn),
		`[update.notify.ntfy]`,
		fmt.Sprintf(`url = %q`, ntfyURL),
		`topic = "duplicacy-updates"`,
	}, "\n")
	if err := os.WriteFile(path, []byte(body+"\n"), 0644); err != nil {
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
	fmt.Fprintf(&b, "location = %q\n", "local")
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
	fmt.Fprintf(&b, "storage = %q\n", filepath.Join(destination, label))
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
	fmt.Fprintf(&b, "location = %q\nstorage = %q\n", "remote", destination)
	return b.String()
}

func localDuplicacyConfigBody(label, storage string, threads int, prune string) string {
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
	fmt.Fprintf(&b, "\n[targets.%s]\n", "onsite-rustfs")
	fmt.Fprintf(&b, "location = %q\nstorage = %q\n", "local", storage)
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
		!strings.Contains(stdout, "diagnostics [OPTIONS] <source>") ||
		!strings.Contains(stdout, "notify <test>") ||
		!strings.Contains(stdout, "rollback [OPTIONS]") ||
		!strings.Contains(stdout, "restore <plan|list-revisions|run|select>") ||
		!strings.Contains(stdout, "update [OPTIONS]") ||
		!strings.Contains(stdout, "health <status|doctor|verify>") ||
		!strings.Contains(stdout, "Commands:") ||
		!strings.Contains(stdout, "Config and inspection  config, diagnostics, health") ||
		!strings.Contains(stdout, "Managed install        update, rollback") ||
		!strings.Contains(stdout, "Use --help-full for the detailed reference.") ||
		!strings.Contains(stdout, "cleanup-storage") ||
		!strings.Contains(stdout, "--json-summary") ||
		!strings.Contains(stdout, "--check-only") {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout, "storj_s3_id") ||
		strings.Contains(stdout, "storj_s3_secret") ||
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
		!strings.Contains(stdout, "rollback [OPTIONS]") ||
		!strings.Contains(stdout, "diagnostics [OPTIONS] <source>") ||
		!strings.Contains(stdout, "notify <test>") ||
		!strings.Contains(stdout, "restore <plan|list-revisions|run|select>") ||
		!strings.Contains(stdout, "Use --help-full for the detailed reference.") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunRestoreRequestNonProgressOutcomes(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "test", "now", t.TempDir())
	rt := workflow.DefaultRuntime()

	cases := []struct {
		name       string
		output     string
		err        error
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "success", output: "restore plan\n", wantCode: 0, wantStdout: "restore plan\n"},
		{name: "cancelled", err: workflow.ErrRestoreCancelled, wantCode: 0, wantStderr: "Restore cancelled by operator"},
		{name: "interrupted", output: "partial report\n", err: workflow.ErrRestoreInterrupted, wantCode: exitCodeGeneralFailure, wantStdout: "partial report\n", wantStderr: "Restore interrupted by operator"},
		{name: "failed", output: "failure report\n", err: workflow.NewRequestError("restore failed"), wantCode: exitCodeGeneralFailure, wantStdout: "failure report\n", wantStderr: "[ERRO] restore failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTestGlobals(t, func() {
				handleRestoreCommand = func(*workflow.Request, workflow.Metadata, workflow.Runtime) (string, error) {
					return tc.output, tc.err
				}
				stdout, stderr := captureOutput(t, func() {
					code := runRestoreRequest(&workflow.Request{RestoreCommand: "plan", Source: "homes", RequestedTarget: "onsite-usb"}, meta, rt)
					if code != tc.wantCode {
						t.Fatalf("code = %d, want %d", code, tc.wantCode)
					}
				})
				if !strings.Contains(stdout, tc.wantStdout) {
					t.Fatalf("stdout = %q, want contains %q", stdout, tc.wantStdout)
				}
				if !strings.Contains(stderr, tc.wantStderr) {
					t.Fatalf("stderr = %q, want contains %q", stderr, tc.wantStderr)
				}
			})
		})
	}
}

func TestWriteRestoreLoggerFailureWrapsLoggerError(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		if code := writeRestoreLoggerFailure(os.ErrPermission); code != exitCodeGeneralFailure {
			t.Fatalf("writeRestoreLoggerFailure() code = %d, want %d", code, exitCodeGeneralFailure)
		}
	})
	if !strings.Contains(stderr, "failed to initialise restore progress logger") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestWriteCommandFailurePreservesMultilineOperatorDiagnostics(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		code := writeCommandFailure("", workflow.NewMessageError("backup/restore-list-files: failed to list files for revision 10\nDuplicacy command: duplicacy list -files -r 10\nDuplicacy diagnostics:\nFailed to load snapshot: permission denied"))
		if code != exitCodeGeneralFailure {
			t.Fatalf("code = %d, want %d", code, exitCodeGeneralFailure)
		}
	})
	for _, want := range []string{
		"[ERRO] backup/restore-list-files: failed to list files for revision 10",
		"Duplicacy command: duplicacy list -files -r 10",
		"Duplicacy diagnostics:",
		"Failed to load snapshot: permission denied",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr)
		}
	}
}

func TestWriteCommandFailureHonoursForcedColour(t *testing.T) {
	t.Setenv("DUPLICACY_BACKUP_FORCE_COLOUR", "1")

	_, stderr := captureOutput(t, func() {
		code := writeCommandFailure("", workflow.NewRequestError("restore failed"))
		if code != exitCodeGeneralFailure {
			t.Fatalf("code = %d, want %d", code, exitCodeGeneralFailure)
		}
	})
	if !strings.Contains(stderr, "\x1b[1;31m[ERRO] restore failed\x1b[0m") {
		t.Fatalf("stderr = %q, want coloured direct error", stderr)
	}
}

func TestRunRollbackRequestPrivilegeAndSuccess(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "test", "now", t.TempDir())
	rt := workflow.DefaultRuntime()

	withTestGlobals(t, func() {
		rt.Geteuid = func() int { return 1000 }
		_, stderr := captureOutput(t, func() {
			code := runRollbackRequest(&workflow.Request{RollbackCommand: "rollback"}, meta, rt)
			if code != exitCodeGeneralFailure {
				t.Fatalf("code = %d", code)
			}
		})
		if !strings.Contains(stderr, "rollback activation must be run as root") {
			t.Fatalf("stderr = %q", stderr)
		}
	})

	withTestGlobals(t, func() {
		rt.Geteuid = func() int { return 0 }
		handleRollbackCommand = func(*workflow.RollbackRequest, workflow.Metadata, workflow.Runtime) (update.RollbackResult, error) {
			return update.RollbackResult{Output: "Rollback\n  Result               : Ready to rollback\n"}, nil
		}
		stdout, stderr := captureOutput(t, func() {
			code := runRollbackRequest(&workflow.Request{RollbackCommand: "rollback", RollbackCheckOnly: true}, meta, rt)
			if code != 0 {
				t.Fatalf("code = %d", code)
			}
		})
		if stderr != "" || !strings.Contains(stdout, "Ready to rollback") {
			t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
		}
	})
}

func TestRunHealthNonRootReachesRealDependencyFailure(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "test", "now", t.TempDir())
	rt := workflow.DefaultRuntime()
	rt.Geteuid = func() int { return 1000 }
	rt.Now = func() time.Time { return time.Date(2026, 4, 25, 18, 0, 0, 0, time.UTC) }

	stdout, stderr := captureOutput(t, func() {
		code := runHealthRequest(&workflow.Request{HealthCommand: "status", Source: "homes", RequestedTarget: "onsite-usb", JSONSummary: true}, meta, rt)
		if code != exitCodeHealthUnhealthy {
			t.Fatalf("code = %d", code)
		}
	})
	if !strings.Contains(stderr, "Required command 'duplicacy' not found") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stdout, `"status": "unhealthy"`) || !strings.Contains(stdout, `"label": "homes"`) {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestUpdateAndRollbackOptionAdapters(t *testing.T) {
	if got := updateOptionsFromRequest(nil); got.Keep != update.DefaultKeep {
		t.Fatalf("nil update options = %#v", got)
	}
	updateReq := workflow.UpdateRequest{Version: "v7.2.1", CheckOnly: true, Force: true, Yes: true, Keep: 5, Attestations: "required"}
	if got := updateOptionsFromRequest(&updateReq); got.RequestedVersion != "v7.2.1" || !got.CheckOnly || !got.Force || !got.Yes || got.Keep != 5 || got.Attestations != "required" {
		t.Fatalf("update options = %#v", got)
	}
	if got := rollbackOptionsFromRequest(nil); got != (update.RollbackOptions{}) {
		t.Fatalf("nil rollback options = %#v", got)
	}
	rollbackReq := workflow.RollbackRequest{Version: "v7.1.1", CheckOnly: true, Yes: true}
	if got := rollbackOptionsFromRequest(&rollbackReq); got.RequestedVersion != "v7.1.1" || !got.CheckOnly || !got.Yes {
		t.Fatalf("rollback options = %#v", got)
	}
}

func TestRunUpdateRequestFailureNotificationWarning(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "test", "now", t.TempDir())
	rt := workflow.DefaultRuntime()
	rt.Geteuid = func() int { return 0 }

	withTestGlobals(t, func() {
		handleUpdateCommand = func(*workflow.UpdateRequest, workflow.Metadata, workflow.Runtime) (update.Result, error) {
			return update.Result{Status: update.StatusFailed}, errors.New("install failed")
		}
		_, stderr := captureOutput(t, func() {
			code := runUpdateRequest(&workflow.Request{UpdateCommand: "update", UpdateYes: true}, meta, rt)
			if code != exitCodeGeneralFailure {
				t.Fatalf("code = %d", code)
			}
		})
		if !strings.Contains(stderr, "[ERRO] install failed") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
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
	if !strings.Contains(stdout, "Config commands:") ||
		!strings.Contains(stdout, "--config-dir <path>     (default: $HOME/.config/duplicacy-backup)") ||
		!strings.Contains(stdout, ".config/duplicacy-backup/secrets") ||
		!strings.Contains(stdout, "Use --help-full for the detailed config reference.") {
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
	if !strings.Contains(stdout, "Notify commands:") ||
		!strings.Contains(stdout, "--provider <all|webhook|ntfy>        (default: all)") ||
		!strings.Contains(stdout, "--severity <warning|critical|info>   (default: warning)") ||
		!strings.Contains(stdout, "--config-dir <path>                  (default: $HOME/.config/duplicacy-backup)") ||
		!strings.Contains(stdout, ".config/duplicacy-backup/secrets") ||
		!strings.Contains(stdout, "Use --help-full for the detailed notify reference.") {
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
	if !strings.Contains(stdout, "Update options:") ||
		!strings.Contains(stdout, "--keep <count>                       (default: 2)") ||
		!strings.Contains(stdout, "--version <tag>                      (default: latest)") ||
		!strings.Contains(stdout, "--attestations <off|auto|required>   (default: off)") ||
		!strings.Contains(stdout, "--config-dir <path>                  (default: $HOME/.config/duplicacy-backup)") ||
		!strings.Contains(stdout, "Use --help-full for the detailed update reference.") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_DiagnosticsHelpReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"diagnostics", "--help"}); code != 0 {
			t.Fatalf("runWithArgs(diagnostics --help) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Diagnostics options:") ||
		!strings.Contains(stdout, "--json-summary") ||
		!strings.Contains(stdout, "Use --help-full for the detailed diagnostics reference.") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_RollbackHelpReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"rollback", "--help"}); code != 0 {
			t.Fatalf("runWithArgs(rollback --help) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Rollback options:") ||
		!strings.Contains(stdout, "--version <tag>") ||
		!strings.Contains(stdout, "Use --help-full for the detailed rollback reference.") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_RestoreHelpReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"restore", "--help"}); code != 0 {
			t.Fatalf("runWithArgs(restore --help) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Restore commands:") ||
		!strings.Contains(stdout, "--target <name>") ||
		!strings.Contains(stdout, "--workspace <path>") ||
		!strings.Contains(stdout, "--workspace-root <path>") ||
		!strings.Contains(stdout, "--config-dir <path>") ||
		!strings.Contains(stdout, "--secrets-dir <path>") ||
		!strings.Contains(stdout, "restore run --target onsite-usb --revision 2403 --path docs/readme.md --yes homes") ||
		!strings.Contains(stdout, "Use --help-full for the detailed restore reference.") {
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
	if !strings.Contains(stdout, "Use [targets.<name>.keys] tables with Duplicacy key names such as:") ||
		!strings.Contains(stdout, "COMMAND OVERVIEW:") ||
		!strings.Contains(stdout, "Runtime operations      Run or maintain one configured label target") ||
		!strings.Contains(stdout, "Config and inspection   Read, explain, validate, or diagnose configured targets") ||
		!strings.Contains(stdout, "Notifications           Send explicit synthetic notification checks") ||
		!strings.Contains(stdout, "Managed install         Manage the installed application binary") ||
		!strings.Contains(stdout, "notify test           Send a simulated notification through configured providers") ||
		!strings.Contains(stdout, "diagnostics           Print a redacted support bundle for one label and target") ||
		!strings.Contains(stdout, "restore plan          Print a read-only Duplicacy restore-drill plan without executing a restore") ||
		!strings.Contains(stdout, "update                Check GitHub for a newer published release and install it through the packaged installer") ||
		!strings.Contains(stdout, "rollback              Inspect or activate a retained managed-install version") ||
		!strings.Contains(stdout, "s3_id") ||
		!strings.Contains(stdout, "s3_secret") ||
		!strings.Contains(stdout, "health_webhook_bearer_token") ||
		!strings.Contains(stdout, "health_ntfy_token") ||
		!strings.Contains(stdout, "health status         Fast read-only health summary for operators and schedulers") ||
		!strings.Contains(stdout, "health verify         Read-only integrity check across revisions found for the current label") ||
		!strings.Contains(stdout, "DUPLICACY_BACKUP_CONFIG_DIR") ||
		!strings.Contains(stdout, "config explain --target offsite-storj homes") ||
		!strings.Contains(stdout, "--json-summary           Write a machine-readable command summary to stdout") ||
		!strings.Contains(stdout, "--keep <count>           Update retention count (default: 2)") ||
		!strings.Contains(stdout, "--attestations <mode>    Update release attestation mode") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_RestoreHelpFullReturnsZero(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := runWithArgs([]string{"restore", "--help-full"}); code != 0 {
			t.Fatalf("runWithArgs(restore --help-full) = %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "restore plan:") ||
		!strings.Contains(stdout, "restore run:") ||
		!strings.Contains(stdout, "does not create directories, write Duplicacy preferences, execute duplicacy restore") ||
		!strings.Contains(stdout, "creates that workspace and writes .duplicacy/preferences when needed") ||
		!strings.Contains(stdout, "docs/restore-drills.md") {
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
		!strings.Contains(stdout, "--keep <count>         Keep this many newest installed binaries after activation (default: 2)") ||
		!strings.Contains(stdout, "/usr/local/bin/duplicacy-backup -> /usr/local/lib/duplicacy-backup/current") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWithArgs_UpdateCheckOnlyReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		handleUpdateCommand = func(req *workflow.UpdateRequest, meta workflow.Metadata, rt workflow.Runtime) (update.Result, error) {
			if req.Command != "update" || !req.CheckOnly || !req.Force || req.Keep != 2 {
				t.Fatalf("req = %+v", req)
			}
			return update.Result{Output: "update ok\n", Status: update.StatusAvailable}, nil
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

func TestRunWithArgs_DiagnosticsDispatchReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		handleDiagnosticsCommand = func(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
			if req.DiagnosticsCommand != "diagnostics" || req.Target() != "onsite-usb" || req.Source != "homes" || !req.JSONSummary {
				t.Fatalf("req = %+v", req)
			}
			return "diagnostics ok\n", nil
		}

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"diagnostics", "--target", "onsite-usb", "--json-summary", "homes"}); code != 0 {
				t.Fatalf("runWithArgs(diagnostics) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if stdout != "diagnostics ok\n" {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_RollbackCheckOnlyNonRootReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		handleRollbackCommand = func(req *workflow.RollbackRequest, meta workflow.Metadata, rt workflow.Runtime) (update.RollbackResult, error) {
			if req.Command != "rollback" || !req.CheckOnly {
				t.Fatalf("req = %+v", req)
			}
			return update.RollbackResult{Output: "rollback check ok\n"}, nil
		}

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"rollback", "--check-only"}); code != 0 {
				t.Fatalf("runWithArgs(rollback --check-only) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if stdout != "rollback check ok\n" {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_RollbackActivationNonRootFailsBeforeHandler(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		called := false
		handleRollbackCommand = func(req *workflow.RollbackRequest, meta workflow.Metadata, rt workflow.Runtime) (update.RollbackResult, error) {
			called = true
			return update.RollbackResult{}, nil
		}

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"rollback"}); code != 1 {
				t.Fatalf("runWithArgs(rollback non-root) = %d", code)
			}
		})
		if called {
			t.Fatal("rollback handler should not be called for non-root activation attempts")
		}
		if stdout != "" {
			t.Fatalf("stdout = %q", stdout)
		}
		if !strings.Contains(stderr, "rollback activation must be run as root") ||
			!strings.Contains(stderr, "--check-only") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_UpdateInstallNonRootFailsBeforeHandler(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		called := false
		handleUpdateCommand = func(req *workflow.UpdateRequest, meta workflow.Metadata, rt workflow.Runtime) (update.Result, error) {
			called = true
			return update.Result{}, nil
		}

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"update", "--attestations", "required"}); code != 1 {
				t.Fatalf("runWithArgs(update non-root) = %d", code)
			}
		})
		if called {
			t.Fatal("update handler should not be called for non-root install attempts")
		}
		if stdout != "" {
			t.Fatalf("stdout = %q", stdout)
		}
		if !strings.Contains(stderr, "update install must be run as root") ||
			!strings.Contains(stderr, "sudo") ||
			!strings.Contains(stderr, "--check-only") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "gh auth") || strings.Contains(stderr, "GitHub CLI") {
			t.Fatalf("stderr should not expose secondary attestation errors: %q", stderr)
		}
	})
}

func TestRunWithArgs_UpdateCheckOnlyNonRootReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		handleUpdateCommand = func(req *workflow.UpdateRequest, meta workflow.Metadata, rt workflow.Runtime) (update.Result, error) {
			if !req.CheckOnly {
				t.Fatalf("CheckOnly = false")
			}
			return update.Result{Output: "check only ok\n", Status: update.StatusAvailable}, nil
		}

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"update", "--check-only"}); code != 0 {
				t.Fatalf("runWithArgs(update --check-only non-root) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if stdout != "check only ok\n" {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_UpdateFailureSendsConfiguredNotification(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		var gotTitle string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotTitle = r.Header.Get("Title")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		writeUpdateNotifyAppConfig(t, configDir, server.URL, "failed")

		handleUpdateCommand = func(req *workflow.UpdateRequest, meta workflow.Metadata, rt workflow.Runtime) (update.Result, error) {
			return update.Result{Status: update.StatusFailed}, fmt.Errorf("update install failed: exit status 1")
		}

		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"update", "--yes", "--config-dir", configDir}); code != 1 {
				t.Fatalf("runWithArgs(update --yes) = %d", code)
			}
		})
		if !strings.Contains(stderr, "[ERRO] update install failed") {
			t.Fatalf("stderr = %q", stderr)
		}
		if gotTitle != "WARNING: Duplicacy Backup update install failed" {
			t.Fatalf("Title = %q", gotTitle)
		}
	})
}

func TestRunWithArgs_UpdateFailureNotificationFailureDoesNotMaskUpdateError(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		writeUpdateNotifyAppConfig(t, configDir, server.URL, "failed")

		handleUpdateCommand = func(req *workflow.UpdateRequest, meta workflow.Metadata, rt workflow.Runtime) (update.Result, error) {
			return update.Result{Status: update.StatusFailed}, fmt.Errorf("update install failed: exit status 1")
		}

		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"update", "--yes", "--config-dir", configDir}); code != 1 {
				t.Fatalf("runWithArgs(update --yes) = %d", code)
			}
		})
		if !strings.Contains(stderr, "[WARN] Failed to send update failure notification") ||
			!strings.Contains(stderr, "[ERRO] update install failed") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_UpdateSuccessNotificationFailureDoesNotFailCommand(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		writeUpdateNotifyAppConfig(t, configDir, server.URL, "succeeded")

		handleUpdateCommand = func(req *workflow.UpdateRequest, meta workflow.Metadata, rt workflow.Runtime) (update.Result, error) {
			return update.Result{
				Output: "Update\n  Human text changed   : yes\n",
				Status: update.StatusInstalled,
			}, nil
		}

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"update", "--yes", "--config-dir", configDir}); code != 0 {
				t.Fatalf("runWithArgs(update --yes) = %d", code)
			}
		})
		if !strings.Contains(stdout, "Human text changed   : yes") {
			t.Fatalf("stdout = %q", stdout)
		}
		if !strings.Contains(stderr, "[WARN] Failed to send update notification") {
			t.Fatalf("stderr = %q", stderr)
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
		if !strings.Contains(stderr, "Configuration file not found") {
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
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "--json-summary", "homes"}); code != 1 {
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

func TestRunWithArgs_HelpDoesNotRequireSynologyDSM(t *testing.T) {
	withTestGlobals(t, func() {
		isSynologyDSM = func() bool { return false }

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--help"}); code != 0 {
				t.Fatalf("runWithArgs(--help) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("expected empty stderr, got %q", stderr)
		}
		if !strings.Contains(stdout, "Usage: duplicacy-backup") {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_OperationalCommandRequiresSynologyDSM(t *testing.T) {
	withTestGlobals(t, func() {
		isSynologyDSM = func() bool { return false }
		called := false
		handleConfigCommand = func(*workflow.Request, workflow.Metadata, workflow.Runtime) (string, error) {
			called = true
			return "", nil
		}

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "explain", "--target", "onsite-usb", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config explain) = %d", code)
			}
		})
		if called {
			t.Fatalf("config handler was called before DSM platform validation")
		}
		if stdout != "" {
			t.Fatalf("expected empty stdout, got %q", stdout)
		}
		if !strings.Contains(stderr, "requires Synology DSM with btrfs-backed /volume* storage") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_DirectRootConfigRequiresExplicitRuntimeProfile(t *testing.T) {
	withTestGlobals(t, func() {
		t.Setenv("SUDO_USER", "")
		t.Setenv("SUDO_UID", "")
		called := false
		handleConfigCommand = func(*workflow.Request, workflow.Metadata, workflow.Runtime) (string, error) {
			called = true
			return "config ok\n", nil
		}

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "explain", "--target", "onsite-usb", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config explain direct root) = %d", code)
			}
		})
		if called {
			t.Fatal("config handler should not be called before direct-root validation")
		}
		if stdout != "" {
			t.Fatalf("stdout = %q", stdout)
		}
		for _, token := range []string{
			directRootProfileErrorLead + " config explain",
			"run as the operator user",
			"--config-dir and --secrets-dir",
			"XDG_STATE_HOME",
			"/root",
		} {
			if !strings.Contains(stderr, token) {
				t.Fatalf("stderr missing %q:\n%s", token, stderr)
			}
		}
	})
}

func TestDirectRootProfilePolicyForRequestCoversCommandSurface(t *testing.T) {
	type policyCase struct {
		name           string
		req            *workflow.Request
		command        string
		usesProfile    bool
		requiresSecret bool
	}
	cases := []policyCase{
		{name: "nil request", req: nil},
		{name: "empty request", req: &workflow.Request{}},
		{name: "backup", req: &workflow.Request{DoBackup: true}, command: "backup", usesProfile: true, requiresSecret: true},
		{name: "prune", req: &workflow.Request{DoPrune: true}, command: "prune", usesProfile: true, requiresSecret: true},
		{name: "cleanup-storage", req: &workflow.Request{DoCleanupStore: true}, command: "cleanup-storage", usesProfile: true, requiresSecret: true},
		{name: "diagnostics", req: &workflow.Request{DiagnosticsCommand: "diagnostics"}, command: "diagnostics", usesProfile: true, requiresSecret: true},
		{name: "update", req: &workflow.Request{UpdateCommand: "update"}, command: "update", usesProfile: true},
		{name: "rollback", req: &workflow.Request{RollbackCommand: "rollback"}, command: "", usesProfile: false},
	}

	for _, command := range []string{"validate", "explain", "paths"} {
		cases = append(cases, policyCase{
			name:           "config " + command,
			req:            &workflow.Request{ConfigCommand: command},
			command:        "config " + command,
			usesProfile:    true,
			requiresSecret: true,
		})
	}
	for _, command := range []string{"status", "doctor", "verify"} {
		cases = append(cases, policyCase{
			name:           "health " + command,
			req:            &workflow.Request{HealthCommand: command},
			command:        "health " + command,
			usesProfile:    true,
			requiresSecret: true,
		})
	}
	for _, command := range []string{"plan", "list-revisions", "run", "select"} {
		cases = append(cases, policyCase{
			name:           "restore " + command,
			req:            &workflow.Request{RestoreCommand: command},
			command:        "restore " + command,
			usesProfile:    true,
			requiresSecret: true,
		})
	}
	cases = append(cases,
		policyCase{
			name:           "notify test label",
			req:            &workflow.Request{NotifyCommand: "test"},
			command:        "notify test",
			usesProfile:    true,
			requiresSecret: true,
		},
		policyCase{
			name:           "notify test update",
			req:            &workflow.Request{NotifyCommand: "test", NotifyScope: "update"},
			command:        "notify test update",
			usesProfile:    true,
			requiresSecret: true,
		},
	)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := directRootProfilePolicyForRequest(tc.req)
			if got.Command != tc.command || got.UsesProfile != tc.usesProfile || got.RequiresSecrets != tc.requiresSecret {
				t.Fatalf("policy = %+v, want command=%q usesProfile=%v requiresSecrets=%v", got, tc.command, tc.usesProfile, tc.requiresSecret)
			}
		})
	}
}

func TestRunWithArgs_DirectRootBackupRequiresSudoOperator(t *testing.T) {
	withTestGlobals(t, func() {
		t.Setenv("SUDO_USER", "")
		t.Setenv("SUDO_UID", "")

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(backup direct root) = %d", code)
			}
		})
		if stdout != "" {
			t.Fatalf("stdout = %q", stdout)
		}
		if !strings.Contains(stderr, directRootProfileErrorLead+" backup") ||
			!strings.Contains(stderr, "run with sudo from that operator account") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "configuration file not found") {
			t.Fatalf("direct-root validation should fail before config loading: %q", stderr)
		}
	})
}

func TestRunWithArgs_DirectRootAllowsExplicitRuntimeProfile(t *testing.T) {
	withTestGlobals(t, func() {
		t.Setenv("SUDO_USER", "")
		t.Setenv("SUDO_UID", "")
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		logDir = ""
		handleConfigCommand = func(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
			if req.ConfigDir == "" || req.SecretsDir == "" {
				t.Fatalf("expected explicit profile request, got %+v", req)
			}
			if !strings.HasPrefix(meta.StateDir, os.Getenv("XDG_STATE_HOME")) ||
				!strings.HasPrefix(meta.LogDir, os.Getenv("XDG_STATE_HOME")) ||
				!strings.HasPrefix(meta.LockParent, os.Getenv("XDG_STATE_HOME")) {
				t.Fatalf("metadata did not use explicit XDG_STATE_HOME: %+v", meta)
			}
			return "config ok\n", nil
		}

		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{
				"config", "explain",
				"--config-dir", t.TempDir(),
				"--secrets-dir", t.TempDir(),
				"--target", "onsite-usb",
				"homes",
			}); code != 0 {
				t.Fatalf("runWithArgs(config explain explicit direct root) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		if stdout != "config ok\n" {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestDefaultVersionFallbackIsDev(t *testing.T) {
	if version != "dev" {
		t.Fatalf("version fallback = %q, want dev; release builds must inject main.version via ldflags", version)
	}
}

func TestRunWithArgs_InvalidFlagReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"--nope"}); code != 1 {
				t.Fatalf("runWithArgs(--nope) = %d", code)
			}
		})
		if !strings.Contains(stderr, "unknown top-level option --nope") {
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

		if !strings.Contains(stderr, "unknown top-level option --nope") {
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
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "homes", "extra"}); code != 1 {
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
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(non-root) = %d", code)
			}
		})
		if !strings.Contains(stderr, "backup must be run as root") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Failed to initialise logger") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_NonRootPruneJSONReachesConfigFailure(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"prune", "--target", "onsite-usb", "--json-summary", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(non-root prune json) = %d", code)
			}
		})
		if !strings.Contains(stderr, "configuration file not found") {
			t.Fatalf("stderr = %q", stderr)
		}
		if strings.Contains(stderr, "Failed to initialise logger") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, `"result": "failed"`) || !strings.Contains(stdout, `"failure_message": "configuration file not found`) {
			t.Fatalf("stdout = %q", stdout)
		}
	})
}

func TestRunWithArgs_ConfigValidateReturnsZeroWithoutRoot(t *testing.T) {
	withTestGlobals(t, func() {
		geteuid = func() int { return 1000 }
		handleConfigCommand = func(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
			return "Config validation succeeded for homes/onsite-usb\n  Section: Resolved\n    Label              : homes\n    Target             : onsite-usb\n    Config File        : /tmp/homes-backup.toml\n  Section: Validation\n    Config             : Valid\n    Source Path Access : Readable\n    Btrfs Source       : Valid\n    Required Settings  : Valid\n    Health Thresholds  : Valid\n    Storage Access     : Writable\n    Repository Access  : Valid\n  Result               : Passed\n", nil
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
				Output:  "Config validation failed for homes/onsite-usb\n  Section: Resolved\n    Label              : homes\n    Target             : onsite-usb\n    Config File        : /tmp/homes-backup.toml\n  Section: Validation\n    Config             : Valid\n    Source Path Access : Invalid (source_path does not exist: /volume1/homes/nested)\n    Btrfs Source       : Not checked\n    Required Settings  : Valid\n    Storage Access     : Writable\n    Repository Access  : Not checked\n  Result               : Failed\n",
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
				fmt.Errorf("cannot open config file /home/operator/.config/duplicacy-backup/homes-backup.toml: %w", os.ErrPermission),
				"path", "/home/operator/.config/duplicacy-backup/homes-backup.toml",
			)
		}
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"config", "validate", "--target", "offsite-storj", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config validate permission denied) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Config file is not accessible: /home/operator/.config/duplicacy-backup/homes-backup.toml") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stderr, "grant read and directory traverse access to the config path, or pass the correct --config-dir for the operator profile") {
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
				Output:  "Config validation failed for homes/offsite-storj\n  Section: Resolved\n    Label              : homes\n    Target             : offsite-storj\n    Config File        : /tmp/homes-backup.toml\n  Section: Validation\n    Config             : Valid\n    Source Path Access : Readable\n    Btrfs Source       : Valid\n    Required Settings  : Valid\n    Health Thresholds  : Valid\n    Storage Access     : Resolved\n    Secrets            : Valid\n    Repository Access  : Not initialized\n  Result               : Failed\n",
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
				Output:  "Config validation failed for homes/offsite-storj\n  Section: Resolved\n    Label              : homes\n    Target             : offsite-storj\n    Config File        : /tmp/homes-backup.toml\n  Section: Validation\n    Config             : Valid\n    Source Path Access : Readable\n    Btrfs Source       : Valid\n    Required Settings  : Valid\n    Health Thresholds  : Valid\n    Storage Access     : Resolved\n    Secrets            : Valid\n    Repository Access  : Invalid (Repository is not ready)\n  Result               : Failed\n",
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
		if !strings.Contains(stdout, "Config explanation for homes/onsite-usb") || !strings.Contains(stdout, "Storage") || !strings.Contains(stdout, "Local Owner") {
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

func TestRunWithArgs_RestorePlanReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", "", "", 4, "-keep 0:365"))
		stdout, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"restore", "plan", "--target", "onsite-usb", "--config-dir", configDir, "homes"}); code != 0 {
				t.Fatalf("runWithArgs(restore plan) = %d", code)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q", stderr)
		}
		for _, token := range []string{
			"Restore plan for homes/onsite-usb",
			"Read Only",
			"Executes Restore",
			"Section: Suggested Commands",
			"duplicacy init",
			"duplicacy restore -r <revision> -stats",
			"docs/restore-drills.md",
		} {
			if !strings.Contains(stdout, token) {
				t.Fatalf("stdout missing %q:\n%s", token, stdout)
			}
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
			`location = "local"`,
			`storage = "/backups/homes"`,
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
			`location = "local"`,
			`storage = "/backups/homes"`,
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
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(config failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "configuration file not found:") || !strings.Contains(stderr, "homes-backup.toml") {
			t.Fatalf("stderr = %q", stderr)
		}
		assertFailureFooter(t, stderr)
	})
}

func TestRunWithArgs_LockAcquisitionFailureReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		owner, group := currentUserGroup(t)
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", owner, group, 0, "-keep 0:365"))

		blocker := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		newLock = func(_, label string) *lock.Lock { return lock.New(blocker, label) }
		newSourceLock = func(_, label string) *lock.Lock { return lock.NewSource(blocker, label) }

		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"prune", "--target", "onsite-usb", "--dry-run", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(lock failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Cannot create the lock directory parent at") || !strings.Contains(stderr, "check that the lock parent path exists and is writable by the user running this command") {
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
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
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
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "--json-summary", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
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
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "--json-summary", "--dry-run", "--verbose", "--config-dir", configDir, "homes"}); code != 0 {
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
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "--json-summary", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(json failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "configuration file not found:") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "\"result\": \"failed\"") ||
			!strings.Contains(stdout, "\"failure_message\": \"configuration file not found:") {
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
			`location = "local"`,
			`storage = "/backups/homes"`,
			``,
			`[health.notify]`,
			`webhook_url = "http://127.0.0.1/unused"`,
			`notify_on = ["degraded", "unhealthy"]`,
			`send_for = ["backup"]`,
		}, "\n"))

		captured := make(chan *workflow.RuntimeRequest, 1)
		maybeSendPreRunFailureNotification = func(rt workflow.Runtime, interactive bool, plan *workflow.Plan, req *workflow.RuntimeRequest, startedAt, completedAt time.Time, err error) error {
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
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "--config-dir", configDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(pre-run notification failure) = %d", code)
			}
		})
		if !strings.Contains(stderr, "required command 'duplicacy' not found") {
			t.Fatalf("stderr = %q", stderr)
		}

		var req *workflow.RuntimeRequest
		select {
		case req = <-captured:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for pre-run notification call")
		}
		if req == nil || req.Label != "homes" || req.Target() != "onsite-usb" || !req.DoBackup() {
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
		if !strings.Contains(stderr, "unknown top-level option --json-summary") {
			t.Fatalf("stderr = %q", stderr)
		}
		if !strings.Contains(stdout, "\"result\": \"failed\"") ||
			!strings.Contains(stdout, "\"failure_message\": \"unknown top-level option --json-summary") {
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
			if code := runWithArgs([]string{"backup", "--target", "offsite-storj", "--dry-run", "--config-dir", configDir, "--secrets-dir", secretsDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(remote missing secrets) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Secrets file not found:") || !strings.Contains(stderr, "homes-secrets.toml") {
			t.Fatalf("stderr = %q", stderr)
		}
		assertFailureScope(t, stderr, "Backup", "homes", "offsite-storj", "duplicacy", "remote")
		assertFailureFooter(t, stderr)
	})
}

func TestRunWithArgs_LocalDuplicacyMissingSecretsReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		secretsDir := t.TempDir()
		writeTargetConfig(t, configDir, "homes", "onsite-rustfs", localDuplicacyConfigBody("homes", "s3://rustfs.local/bucket", 4, ""))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"backup", "--target", "onsite-rustfs", "--dry-run", "--config-dir", configDir, "--secrets-dir", secretsDir, "homes"}); code != 1 {
				t.Fatalf("runWithArgs(local duplicacy missing secrets) = %d", code)
			}
		})
		if !strings.Contains(stderr, "Secrets file not found:") || !strings.Contains(stderr, "homes-secrets.toml") {
			t.Fatalf("stderr = %q", stderr)
		}
		assertFailureScope(t, stderr, "Backup", "homes", "onsite-rustfs", "duplicacy", "local")
		assertFailureFooter(t, stderr)
	})
}

func TestRunWithArgs_InvalidTomlConfigReturnsOne(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", "label = \"homes\"\nsource_path = \"/volume1/homes\"\n\n[targets.onsite-usb]\nlocation = \"local\"\nstorage = \"/backups/homes\"\nthreads =\n")
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"backup", "--target", "onsite-usb", "--config-dir", configDir, "homes"}); code != 1 {
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

func TestRunWithArgs_FixPermsCommandIsRemoved(t *testing.T) {
	withTestGlobals(t, func() {
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"fix-perms", "--target", "onsite-usb", "homes"}); code != 1 {
				t.Fatalf("runWithArgs(fix-perms removed) = %d", code)
			}
		})
		if !strings.Contains(stderr, "unknown command fix-perms") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestRunWithArgs_CleanupStorageOnlyDryRunReturnsZero(t *testing.T) {
	withTestGlobals(t, func() {
		configDir := t.TempDir()
		writeConfig(t, configDir, "homes", localConfigBody("homes", "/backups", "", "", 4, ""))
		_, stderr := captureOutput(t, func() {
			if code := runWithArgs([]string{"cleanup-storage", "--target", "onsite-usb", "--dry-run", "--config-dir", configDir, "homes"}); code != 0 {
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

func TestParseFailureContext_HealthRequest(t *testing.T) {
	ctx := command.ParseFailureContext([]string{
		"health", "verify", "--target", "offsite-storj", "--json-summary", "--verbose",
		"--config-dir", "/cfg", "--secrets-dir", "/sec", "homes", "ignored",
	})
	if ctx.Kind != command.FailureRequestHealth || !ctx.JSONSummary {
		t.Fatalf("ctx = %+v", ctx)
	}
	req := ctx.Request
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

func TestParseFailureContext_NonSpecialCommandReturnsEmptyRequest(t *testing.T) {
	ctx := command.ParseFailureContext([]string{"--target", "onsite-usb", "homes"})
	if ctx.Kind != command.FailureRequestNone || ctx.JSONSummary || ctx.Request.HealthCommand != "" ||
		ctx.Request.RequestedTarget != "" || ctx.Request.Source != "" || ctx.Request.Verbose {
		t.Fatalf("ctx = %+v", ctx)
	}
}

func TestEmitJSONFailureSummary(t *testing.T) {
	startedAt := time.Unix(100, 0).UTC()
	completedAt := time.Unix(130, 0).UTC()

	emitJSONFailureSummary(nil, nil, nil, startedAt, completedAt, "ignored")

	var buf bytes.Buffer
	emitJSONFailureSummary(&buf, &workflow.RuntimeRequest{Label: "homes", TargetName: "onsite-usb"}, &workflow.Plan{
		Config: workflow.PlanConfig{Location: "local"},
	}, startedAt, completedAt, "boom")
	if !strings.Contains(buf.String(), `"result": "failed"`) || !strings.Contains(buf.String(), `"target": "onsite-usb"`) || !strings.Contains(buf.String(), `"location": "local"`) || strings.Contains(buf.String(), `"storage_type"`) {
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

func TestWriteRuntimeJSONSummaryFailureOnlyUpgradesSuccess(t *testing.T) {
	for _, tt := range []struct {
		name string
		code int
		want int
	}{
		{name: "success becomes general failure", code: 0, want: 1},
		{name: "existing failure is preserved", code: 1, want: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr := captureOutput(t, func() {
				got := writeRuntimeJSONSummary(errWriter{}, &workflow.RunReport{}, tt.code)
				if got != tt.want {
					t.Fatalf("writeRuntimeJSONSummary() = %d, want %d", got, tt.want)
				}
			})
			if !strings.Contains(stderr, "Failed to write JSON summary") {
				t.Fatalf("stderr = %q", stderr)
			}
		})
	}
}

func TestWriteHealthJSONSummaryFailureOnlyUpgradesHealthySuccess(t *testing.T) {
	for _, tt := range []struct {
		name string
		code int
		want int
	}{
		{name: "healthy success becomes unhealthy", code: 0, want: 2},
		{name: "degraded result is preserved", code: 1, want: 1},
		{name: "unhealthy result is preserved", code: 2, want: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr := captureOutput(t, func() {
				got := writeHealthJSONSummary(errWriter{}, &workflow.HealthReport{}, tt.code)
				if got != tt.want {
					t.Fatalf("writeHealthJSONSummary() = %d, want %d", got, tt.want)
				}
			})
			if !strings.Contains(stderr, "Failed to write JSON summary") {
				t.Fatalf("stderr = %q", stderr)
			}
		})
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

func TestBuildRequest_JSONSummaryNotifyUpdateFailureInfersScope(t *testing.T) {
	meta := workflow.DefaultMetadata(scriptName, version, buildTime, t.TempDir())
	rt := workflow.DefaultRuntime()

	stdout, stderr := captureOutput(t, func() {
		result, code := buildRequest([]string{"notify", "test", "update", "--json-summary", "--event", "unknown"}, meta, rt)
		if code != 1 {
			t.Fatalf("buildRequest() code = %d", code)
		}
		if result != nil {
			t.Fatalf("buildRequest() result = %#v, want nil", result)
		}
	})
	if !strings.Contains(stdout, `"scope": "update"`) ||
		!strings.Contains(stdout, `"result": "failed"`) {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "unsupported notify event") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestParseFailureContext_NotifyRequest(t *testing.T) {
	ctx := command.ParseFailureContext([]string{
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
	if ctx.Kind != command.FailureRequestNotify || !ctx.JSONSummary {
		t.Fatalf("ctx = %+v", ctx)
	}
	req := ctx.Request
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

func TestParseFailureContext_NotifyUpdateScope(t *testing.T) {
	ctx := command.ParseFailureContext([]string{
		"notify", "test", "update",
		"--provider", "ntfy",
		"--event", "update_install_failed",
		"--dry-run",
		"--json-summary",
	})
	if ctx.Kind != command.FailureRequestNotify || !ctx.JSONSummary {
		t.Fatalf("ctx = %+v", ctx)
	}
	req := ctx.Request
	if req.NotifyCommand != "test" || req.NotifyScope != "update" || req.Source != "" || req.Target() != "" {
		t.Fatalf("req = %+v", req)
	}
	if req.NotifyProvider != "ntfy" || req.NotifyEvent != "update_install_failed" {
		t.Fatalf("req = %+v", req)
	}
	if !req.DryRun || !req.JSONSummary {
		t.Fatalf("req = %+v", req)
	}
}

func TestParseFailureContext_NonNotifyCommandDoesNotApplyNotifyDefaults(t *testing.T) {
	ctx := command.ParseFailureContext([]string{"--target", "onsite-usb1", "homes"})
	if ctx.Kind != command.FailureRequestNone || ctx.Request.NotifyCommand != "" ||
		ctx.Request.RequestedTarget != "" || ctx.Request.Source != "" || ctx.Request.DryRun ||
		ctx.Request.JSONSummary || ctx.Request.NotifyProvider != "" || ctx.Request.NotifySeverity != "" {
		t.Fatalf("ctx = %+v", ctx)
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
	if strings.Contains(stderr, "Failed to initialise logger") || !strings.Contains(stderr, "--json-summary") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stdout, `"result": "failed"`) {
		t.Fatalf("stdout = %q", stdout)
	}
}
