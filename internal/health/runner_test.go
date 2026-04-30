package health

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func captureHealthOutput(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(stderr) error = %v", err)
	}
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stderr = oldStderr
	output := <-done
	_ = r.Close()
	_ = w.Close()

	return output
}

func setHealthTestLoggerField[T any](t *testing.T, log *logger.Logger, name string, value T) {
	t.Helper()
	field := reflect.ValueOf(log).Elem().FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("logger field %q not found", name)
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local listener unavailable: %v", err)
	}

	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

func newHealthRuntime(now time.Time, tempDir string) Env {
	rt := workflow.DefaultEnv()
	rt.Geteuid = func() int { return 0 }
	rt.LookPath = func(string) (string, error) { return "/usr/bin/true", nil }
	rt.Now = func() time.Time { return now }
	rt.TempDir = func() string { return tempDir }
	rt.Getpid = func() int { return 1234 }
	rt.Getenv = func(string) string { return "" }
	return rt
}

func writeHealthConfig(t *testing.T, dir, label string, body string) {
	t.Helper()
	path := filepath.Join(dir, label+"-backup.toml")
	sourcePath := filepath.Join(dir, label+"-source")
	if err := os.MkdirAll(sourcePath, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	storage := filepath.Join(dir, label+"-storage")
	var config strings.Builder
	fmt.Fprintf(&config, "label = %q\n", label)
	fmt.Fprintf(&config, "source_path = %q\n", sourcePath)
	if !strings.Contains(body, "[storage.") {
		fmt.Fprintf(&config, "\n[storage.onsite-usb]\nlocation = %q\nstorage = %q\n", locationLocal, storage)
	}
	if strings.TrimSpace(body) != "" {
		config.WriteString("\n")
		config.WriteString(body)
	}
	if err := os.WriteFile(path, []byte(config.String()), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func assertOrderedTokens(t *testing.T, text string, tokens ...string) {
	t.Helper()
	last := -1
	for _, token := range tokens {
		idx := strings.Index(text, token)
		if idx < 0 {
			t.Fatalf("output missing %q:\n%s", token, text)
		}
		if idx < last {
			t.Fatalf("output order mismatch; %q appeared before expected sequence:\n%s", token, text)
		}
		last = idx
	}
}

func TestHealthRunner_StatusHealthy(t *testing.T) {
	now := time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	configDir := t.TempDir()
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n[health]\nfreshness_warn_hours = 12\nfreshness_fail_hours = 24\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	runner := execpkg.NewMockRunner(execpkg.MockResult{
		Stdout: "Snapshot homes revision 8 created at 2026-04-10 16:30\n",
	})
	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand:   "status",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
	})
	if code != 0 {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if report.Status != "healthy" || report.LatestRevision != 8 {
		t.Fatalf("report = %+v", report)
	}
	if report.RevisionCount != 1 {
		t.Fatalf("report = %+v", report)
	}
	if report.LatestRevisionAt == "" || report.LastSuccessAt == "" {
		t.Fatalf("report = %+v", report)
	}
}

func TestRootProfileConfigWarningDetectsMigratedOperatorConfig(t *testing.T) {
	operatorConfig := filepath.Join(t.TempDir(), "operator", ".config", "duplicacy-backup", "homes-backup.toml")
	if err := os.MkdirAll(filepath.Dir(operatorConfig), 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(operatorConfig, []byte("label = \"homes\"\n"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	rt := newHealthRuntime(time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC), t.TempDir())
	rt.Getenv = func(name string) string {
		switch name {
		case "HOME":
			return "/root"
		default:
			return ""
		}
	}

	got := rootProfileConfigWarning(&HealthRequest{Command: "doctor", Label: "homes"}, rt, []string{operatorConfig})
	if !strings.Contains(got, "/root/.config/duplicacy-backup") ||
		!strings.Contains(got, operatorConfig) ||
		!strings.Contains(got, "--config-dir") {
		t.Fatalf("rootProfileConfigWarning() = %q", got)
	}
}

func TestRootProfileConfigWarningIgnoresNonRootAndExplicitConfig(t *testing.T) {
	operatorConfig := filepath.Join(t.TempDir(), "operator", ".config", "duplicacy-backup", "homes-backup.toml")
	rt := newHealthRuntime(time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC), t.TempDir())
	rt.Getenv = func(name string) string {
		if name == "HOME" {
			return "/root"
		}
		return ""
	}

	nonRoot := rt
	nonRoot.Geteuid = func() int { return 1000 }
	if got := rootProfileConfigWarning(&HealthRequest{Command: "doctor", Label: "homes"}, nonRoot, []string{operatorConfig}); got != "" {
		t.Fatalf("non-root warning = %q, want empty", got)
	}

	req := &HealthRequest{Command: "doctor", Label: "homes", ConfigDir: filepath.Join(t.TempDir(), "config")}
	if got := rootProfileConfigWarning(req, rt, []string{operatorConfig}); got != "" {
		t.Fatalf("explicit config warning = %q, want empty", got)
	}
}

func TestHealthRunner_StatusAllowsLocalReadOnlyTarget(t *testing.T) {
	now := time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)

	configDir := t.TempDir()
	writeTargetTestConfig(t, configDir, "homes", "onsite-usb", buildTargetConfig("homes", "onsite-usb", locationLocal, "/volume1/homes", "/backups/homes", 0, ""))

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	runner := execpkg.NewMockRunner(execpkg.MockResult{
		Stdout: "Snapshot homes revision 8 created at 2026-04-10 16:30\n",
	})
	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand:   "status",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
	})
	if code != 0 || report.Status != "healthy" {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
}

func TestHealthRunner_StatusOutputShowsTargetAndDefersSecrets(t *testing.T) {
	now := time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	secretsDir := t.TempDir()
	sourcePath := filepath.Join(configDir, "homes-source")
	if err := os.MkdirAll(sourcePath, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	writeTargetTestConfig(t, configDir, "homes", "offsite-storj", remoteTargetConfig("homes", sourcePath, "s3://EU@gateway.storjshare.io/bucket", 4, "-keep 0:365"))
	writeTargetTestSecrets(t, secretsDir, "homes", "offsite-storj")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", "offsite-storj", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)

		runner := execpkg.NewMockRunner(execpkg.MockResult{
			Stdout: "Snapshot homes revision 8 created at 2026-04-10 16:30\n",
		})
		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "status",
			RequestedTarget: "offsite-storj",
			Source:          "homes",
			ConfigDir:       configDir,
			SecretsDir:      secretsDir,
		})
		if code != 0 {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
	})

	assertOrderedTokens(t, stderr,
		"Check                : Status",
		"Label                : homes",
		"Storage              : offsite-storj",
	)
	assertOrderedTokens(t, stderr,
		"Config File          :",
		"Revision Count       :",
		"Latest Revision      :",
		"Backup Freshness     :",
		"Secrets              :",
	)
}

func TestHealthRunner_VerifyUnhealthyWhenStorageTooOld(t *testing.T) {
	now := time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n[health]\nfreshness_warn_hours = 1\nfreshness_fail_hours = 2\nverify_warn_after_hours = 24\n")

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 12:00\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "2026-04-10 12:10:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 8 exist\n"},
	)
	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand:   "verify",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
	})
	if code != 2 {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if report.Status != "unhealthy" {
		t.Fatalf("report = %+v", report)
	}
}

func TestHealthWebhookDelivery_WhenStdinIsNotTTY(t *testing.T) {
	var hits int
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	now := time.Date(2026, 4, 10, 21, 11, 7, 0, time.UTC)
	rt := newHealthRuntime(now, t.TempDir())
	rt.StdinIsTTY = func() bool { return false }

	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	setHealthTestLoggerField(t, log, "interactive", true)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n[health]\nfreshness_warn_hours = 1\nfreshness_fail_hours = 2\n[health.notify]\nwebhook_url = \""+server.URL+"\"\nnotify_on = [\"degraded\", \"unhealthy\"]\nsend_for = [\"doctor\", \"verify\"]\ninteractive = false\n")

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Err: errors.New("failed to list revisions for latest revision inspection")},
		execpkg.MockResult{Err: errors.New("repository invalid")},
	)
	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand:   "doctor",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
		SecretsDir:      t.TempDir(),
	})
	if code != 2 {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if !report.NotificationSent {
		t.Fatalf("report = %+v", report)
	}
	if hits != 1 {
		t.Fatalf("webhook hits = %d, want 1", hits)
	}
}

func TestHealthRunner_EarlyFailureSendsWebhookWhenConfigReadable(t *testing.T) {
	var hits int
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	now := time.Date(2026, 4, 10, 21, 11, 7, 0, time.UTC)
	rt := newHealthRuntime(now, t.TempDir())
	rt.LookPath = func(name string) (string, error) {
		if name == "duplicacy" {
			return "", errors.New("not found")
		}
		return "/usr/bin/true", nil
	}
	rt.StdinIsTTY = func() bool { return false }

	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	setHealthTestLoggerField(t, log, "interactive", true)
	configDir := t.TempDir()
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n[health.notify]\nwebhook_url = \""+server.URL+"\"\nnotify_on = [\"degraded\", \"unhealthy\"]\nsend_for = [\"doctor\", \"verify\"]\ninteractive = false\n")

	report, code := NewHealthRunner(meta, rt, log, execpkg.NewMockRunner()).Run(&Request{
		HealthCommand:   "doctor",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
		SecretsDir:      t.TempDir(),
	})
	if code != 2 {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if !report.NotificationSent {
		t.Fatalf("report = %+v", report)
	}
	if hits != 1 {
		t.Fatalf("webhook hits = %d, want 1", hits)
	}
}

func TestWriteHealthReport_DoesNotIncludeSummaryField(t *testing.T) {
	report := NewFailureHealthReport(&HealthRequest{Command: "doctor", Label: "homes", TargetName: "onsite-usb"}, "doctor", "boom", time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC))

	var buf bytes.Buffer
	if err := WriteHealthReport(&buf, report); err != nil {
		t.Fatalf("WriteHealthReport() error = %v", err)
	}

	text := buf.String()
	if strings.Contains(text, `"summary"`) {
		t.Fatalf("health JSON should not include summary field: %s", text)
	}
	if strings.Contains(text, `"checks"`) {
		t.Fatalf("health JSON should not include rendered checks: %s", text)
	}
	if !strings.Contains(text, `"status": "unhealthy"`) || !strings.Contains(text, `"check_type": "doctor"`) {
		t.Fatalf("unexpected JSON: %s", text)
	}
}

func TestWriteHealthReport_VerifyAlwaysIncludesStableFailureFields(t *testing.T) {
	report := &Report{
		Status:               "healthy",
		CheckType:            "verify",
		Label:                "homes",
		Mode:                 "onsite-usb",
		CheckedAt:            "2026-04-10T22:25:20Z",
		RevisionCount:        79,
		LatestRevision:       2338,
		LatestRevisionAt:     "2026-04-10T22:15:20Z",
		LastVerifyRunAt:      "2026-04-10T22:25:20Z",
		CheckedRevisionCount: 79,
		PassedRevisionCount:  79,
		Checks: []Check{
			{Name: "Revisions checked", Result: "pass", Message: "79"},
			{Name: "Revisions failed", Result: "pass", Message: "0"},
			{Name: "Last verify run", Result: "pass", Message: "<1m ago"},
		},
	}

	var buf bytes.Buffer
	if err := WriteHealthReport(&buf, report); err != nil {
		t.Fatalf("WriteHealthReport() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if got, ok := payload["failed_revision_count"].(float64); !ok || got != 0 {
		t.Fatalf("failed_revision_count = %#v, want 0", payload["failed_revision_count"])
	}
	if got, ok := payload["revision_count"].(float64); !ok || got != 79 {
		t.Fatalf("revision_count = %#v, want 79", payload["revision_count"])
	}
	if got, ok := payload["checked_revision_count"].(float64); !ok || got != 79 {
		t.Fatalf("checked_revision_count = %#v, want 79", payload["checked_revision_count"])
	}
	if got, ok := payload["passed_revision_count"].(float64); !ok || got != 79 {
		t.Fatalf("passed_revision_count = %#v, want 79", payload["passed_revision_count"])
	}
	failed, ok := payload["failed_revisions"].([]any)
	if !ok || len(failed) != 0 {
		t.Fatalf("failed_revisions = %#v, want []", payload["failed_revisions"])
	}
	if got := payload["last_verify_run_at"]; got != "2026-04-10T22:25:20Z" {
		t.Fatalf("last_verify_run_at = %#v", got)
	}
	if _, ok := payload["checks"]; ok {
		t.Fatalf("checks should not appear in JSON: %#v", payload["checks"])
	}
	if strings.Contains(buf.String(), `\u003c1m ago`) || strings.Contains(buf.String(), `"Last verify run"`) {
		t.Fatalf("cadence check should not appear in JSON: %s", buf.String())
	}
	if strings.Contains(buf.String(), `"message": "0"`) {
		t.Fatalf("counter values should not appear as JSON issues: %s", buf.String())
	}
}

func TestHealthRunner_VerifyHealthyWhenAllVisibleRevisionsValidate(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())

	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n[health]\nfreshness_warn_hours = 24\nfreshness_fail_hours = 48\nverify_warn_after_hours = 24\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastDoctorAt:                 formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 17:30\nSnapshot homes revision 7 created at 2026-04-10 16:30\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 8 exist\n2026-04-10 20:00:01.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 7 exist\n"},
	)

	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand:   "verify",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
	})
	if code != 0 {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if report.RevisionCount != 2 || report.CheckedRevisionCount != 2 || report.PassedRevisionCount != 2 || report.FailedRevisionCount != 0 {
		t.Fatalf("report = %+v", report)
	}
	if len(report.RevisionResults) != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestHealthRunner_VerifySkipsBtrfsReadinessChecks(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())

	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n[health]\nfreshness_warn_hours = 24\nfreshness_fail_hours = 48\nverify_warn_after_hours = 24\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 17:30\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 8 exist\n"},
	)

	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand:   "verify",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
	})
	if code != 0 || report.Status != "healthy" {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if report.CheckedRevisionCount != 1 || report.PassedRevisionCount != 1 || report.FailedRevisionCount != 0 {
		t.Fatalf("report = %+v", report)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("backup-readiness checks should not create verify issues: %+v", report.Issues)
	}
	if result, message, ok := CheckResult(report, "Btrfs"); !ok || result != "info" || message != "Not checked; backup-readiness validation is not required for storage integrity verification" {
		t.Fatalf("Btrfs check = (%q, %q), present=%t, report=%+v", result, message, ok, report)
	}
	if _, _, ok := CheckResult(report, "Btrfs root"); ok {
		t.Fatalf("verify should not run root subvolume metadata readiness check: %+v", report)
	}
	if _, _, ok := CheckResult(report, "Btrfs source"); ok {
		t.Fatalf("verify should not run source subvolume metadata readiness check: %+v", report)
	}
	if _, _, ok := CheckResult(report, "Last doctor run"); ok {
		t.Fatalf("verify should not report doctor recency: %+v", report)
	}
}

func TestHealthRunner_VerifyUnhealthyWhenNoRevisionsFound(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastDoctorAt:                 formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)

		runner := execpkg.NewMockRunner(
			execpkg.MockResult{Stdout: ""},
			execpkg.MockResult{},
		)

		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "verify",
			RequestedTarget: "onsite-usb",
			Source:          "homes",
			ConfigDir:       configDir,
		})
		if code != 2 || report.Status != "unhealthy" {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
		if report.RevisionCount != 0 || report.CheckedRevisionCount != 0 || report.PassedRevisionCount != 0 || report.FailedRevisionCount != 0 {
			t.Fatalf("report = %+v", report)
		}
		found := false
		for _, issue := range report.Issues {
			if strings.Contains(issue.Message, "No revisions were found for this backup") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("report = %+v", report)
		}
	})

	if !strings.Contains(stderr, "Revision Count") || !strings.Contains(stderr, "0") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Integrity Check") || !strings.Contains(stderr, "No revisions were found for this backup") {
		t.Fatalf("stderr = %q", stderr)
	}

	var buf bytes.Buffer
	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: ""},
		execpkg.MockResult{},
	)
	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand:   "verify",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
	})
	if code != 2 {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if err := WriteHealthReport(&buf, report); err != nil {
		t.Fatalf("WriteHealthReport() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := payload["failure_code"]; got != verifyFailureNoRevisionsFound {
		t.Fatalf("failure_code = %#v, want %q", got, verifyFailureNoRevisionsFound)
	}
	if got, ok := payload["revision_count"].(float64); !ok || got != 0 {
		t.Fatalf("revision_count = %#v, want 0", payload["revision_count"])
	}
	if got, ok := payload["checked_revision_count"].(float64); !ok || got != 0 {
		t.Fatalf("checked_revision_count = %#v, want 0", payload["checked_revision_count"])
	}
	if got, ok := payload["passed_revision_count"].(float64); !ok || got != 0 {
		t.Fatalf("passed_revision_count = %#v, want 0", payload["passed_revision_count"])
	}
	actions, ok := payload["recommended_action_codes"].([]any)
	if !ok || len(actions) != 1 || actions[0].(string) != verifyActionRunBackup {
		t.Fatalf("recommended_action_codes = %#v, want [%q]", payload["recommended_action_codes"], verifyActionRunBackup)
	}
}

func TestHealthRunner_VerifyUnhealthyWhenResultsDoNotCoverAllVisibleRevisions(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())

	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastDoctorAt:                 formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 17:30\nSnapshot homes revision 7 created at 2026-04-10 16:30\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 8 exist\n"},
	)

	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand:   "verify",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
	})
	if code != 2 || report.Status != "unhealthy" {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if report.CheckedRevisionCount != 1 || report.PassedRevisionCount != 1 {
		t.Fatalf("report = %+v", report)
	}
	if len(report.RevisionResults) != 1 || report.RevisionResults[0].Message != "No integrity result returned" {
		t.Fatalf("report = %+v", report)
	}
	found := false
	for _, issue := range report.Issues {
		if strings.Contains(issue.Message, "returned no integrity result") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("report = %+v", report)
	}
	if !reflect.DeepEqual(report.FailureCodes, []string{verifyFailureResultMissing}) {
		t.Fatalf("FailureCodes = %#v, want [%q]", report.FailureCodes, verifyFailureResultMissing)
	}
}

func TestHealthRunner_VerifyMixedFailedAndMissingResultsShapeJSONAndOutput(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 10,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastDoctorAt:                 formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	var report *Report
	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)

		runner := execpkg.NewMockRunner(
			execpkg.MockResult{Stdout: "Snapshot homes revision 10 created at 2026-04-10 17:45\nSnapshot homes revision 9 created at 2026-04-10 17:15\nSnapshot homes revision 8 created at 2026-04-10 16:45\n"},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 10 exist\n2026-04-10 20:00:01.000 WARN SNAPSHOT_CHECK Some chunks referenced by snapshot homes at revision 9 are missing\n"},
		)

		var code int
		report, code = NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "verify",
			RequestedTarget: "onsite-usb",
			Source:          "homes",
			ConfigDir:       configDir,
		})
		if code != 2 || report.Status != "unhealthy" {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
	})

	if report.RevisionCount != 3 || report.CheckedRevisionCount != 2 || report.PassedRevisionCount != 1 || report.FailedRevisionCount != 1 {
		t.Fatalf("report = %+v", report)
	}
	if !reflect.DeepEqual(report.FailedRevisions, []int{9}) {
		t.Fatalf("FailedRevisions = %#v, want []int{9}", report.FailedRevisions)
	}
	if len(report.RevisionResults) != 2 {
		t.Fatalf("report = %+v", report)
	}
	if report.RevisionResults[0].Revision != 9 || report.RevisionResults[0].Result != "fail" || report.RevisionResults[0].Message != "Missing chunks" {
		t.Fatalf("report.RevisionResults[0] = %+v", report.RevisionResults[0])
	}
	if report.RevisionResults[1].Revision != 8 || report.RevisionResults[1].Result != "fail" || report.RevisionResults[1].Message != "No integrity result returned" {
		t.Fatalf("report.RevisionResults[1] = %+v", report.RevisionResults[1])
	}
	if !strings.Contains(stderr, "Revisions Failed") || !strings.Contains(stderr, "1 (9)") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Integrity Check") || !strings.Contains(stderr, "1 failed; 1 returned no result") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Revision 9") || !strings.Contains(stderr, "Missing chunks") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Revision 8") || !strings.Contains(stderr, "No integrity result returned") {
		t.Fatalf("stderr = %q", stderr)
	}

	var buf bytes.Buffer
	if err := WriteHealthReport(&buf, report); err != nil {
		t.Fatalf("WriteHealthReport() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, legacyKey := range []string{"storage_visible_revision_count", "storage_latest_revision", "storage_latest_revision_at", "verified_revision_count", "checks"} {
		if _, ok := payload[legacyKey]; ok {
			t.Fatalf("payload unexpectedly included %q: %#v", legacyKey, payload[legacyKey])
		}
	}
	if got, ok := payload["revision_count"].(float64); !ok || got != 3 {
		t.Fatalf("revision_count = %#v, want 3", payload["revision_count"])
	}
	if got, ok := payload["checked_revision_count"].(float64); !ok || got != 2 {
		t.Fatalf("checked_revision_count = %#v, want 2", payload["checked_revision_count"])
	}
	if got, ok := payload["passed_revision_count"].(float64); !ok || got != 1 {
		t.Fatalf("passed_revision_count = %#v, want 1", payload["passed_revision_count"])
	}
	if got, ok := payload["failed_revision_count"].(float64); !ok || got != 1 {
		t.Fatalf("failed_revision_count = %#v, want 1", payload["failed_revision_count"])
	}
	if got := payload["failure_code"]; got != verifyFailureIntegrityFailed {
		t.Fatalf("failure_code = %#v, want %q", got, verifyFailureIntegrityFailed)
	}
	codes, ok := payload["failure_codes"].([]any)
	if !ok || len(codes) != 2 || codes[0].(string) != verifyFailureIntegrityFailed || codes[1].(string) != verifyFailureResultMissing {
		t.Fatalf("failure_codes = %#v, want [%q, %q]", payload["failure_codes"], verifyFailureIntegrityFailed, verifyFailureResultMissing)
	}
	actions, ok := payload["recommended_action_codes"].([]any)
	if !ok || len(actions) != 3 {
		t.Fatalf("recommended_action_codes = %#v", payload["recommended_action_codes"])
	}
	failed, ok := payload["failed_revisions"].([]any)
	if !ok || len(failed) != 1 || failed[0].(float64) != 9 {
		t.Fatalf("failed_revisions = %#v, want [9]", payload["failed_revisions"])
	}
	results, ok := payload["revision_results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("revision_results = %#v, want 2 entries", payload["revision_results"])
	}
}

func TestHealthRunner_VerifyFailureSummaryIsOperatorFriendly(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastDoctorAt:                 formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)

		runner := execpkg.NewMockRunner(
			execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 17:30\nSnapshot homes revision 7 created at 2026-04-10 16:30\n"},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 WARN SNAPSHOT_CHECK Some chunks referenced by snapshot homes at revision 8 are missing\n2026-04-10 20:00:01.000 WARN SNAPSHOT_CHECK Some chunks referenced by snapshot homes at revision 7 are missing\n"},
		)

		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "verify",
			RequestedTarget: "onsite-usb",
			Source:          "homes",
			ConfigDir:       configDir,
		})
		if code != 2 || report.Status != "unhealthy" {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
	})

	if !strings.Contains(stderr, "Revisions Failed") || !strings.Contains(stderr, "2 (8, 7)") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Integrity Check") || !strings.Contains(stderr, "2 revision(s) failed integrity checks: 8, 7") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Revision 8") || !strings.Contains(stderr, "Revision 7") || !strings.Contains(stderr, "Missing chunks") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHealthRunner_VerifyCheckFailureBeforeAttributionSetsAccessCodes(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 17:30\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "fatal failure\n", Err: errors.New("check failed")},
	)

	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand:   "verify",
		RequestedTarget: "onsite-usb",
		Source:          "homes",
		ConfigDir:       configDir,
	})
	if code != 2 || report.Status != "unhealthy" {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if report.CheckedRevisionCount != 0 || report.PassedRevisionCount != 0 || report.FailedRevisionCount != 0 {
		t.Fatalf("report = %+v", report)
	}
	if !reflect.DeepEqual(report.FailureCodes, []string{verifyFailureAccessFailed}) {
		t.Fatalf("FailureCodes = %#v, want [%q]", report.FailureCodes, verifyFailureAccessFailed)
	}
	var buf bytes.Buffer
	if err := WriteHealthReport(&buf, report); err != nil {
		t.Fatalf("WriteHealthReport() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := payload["failure_code"]; got != verifyFailureAccessFailed {
		t.Fatalf("failure_code = %#v, want %q", got, verifyFailureAccessFailed)
	}
}

func TestHealthRunner_VerifyListingFailureSetsListingCodesAndZeroCounts(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)

		runner := execpkg.NewMockRunner(execpkg.MockResult{Err: errors.New("list failed")})
		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "verify",
			RequestedTarget: "onsite-usb",
			Source:          "homes",
			ConfigDir:       configDir,
		})
		if code != 2 || report.Status != "unhealthy" {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
		if !reflect.DeepEqual(report.FailureCodes, []string{verifyFailureListingFailed}) {
			t.Fatalf("FailureCodes = %#v, want [%q]", report.FailureCodes, verifyFailureListingFailed)
		}
		var buf bytes.Buffer
		if err := WriteHealthReport(&buf, report); err != nil {
			t.Fatalf("WriteHealthReport() error = %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if got := payload["failure_code"]; got != verifyFailureListingFailed {
			t.Fatalf("failure_code = %#v, want %q", got, verifyFailureListingFailed)
		}
		if got, ok := payload["revision_count"].(float64); !ok || got != 0 {
			t.Fatalf("revision_count = %#v, want 0", payload["revision_count"])
		}
		if got, ok := payload["last_verify_run_at"]; ok && got == "" {
			t.Fatalf("last_verify_run_at = %#v", got)
		}
	})

	if !strings.Contains(stderr, "Latest Revision") || !strings.Contains(stderr, "Could not inspect storage revisions") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Integrity Check") || !strings.Contains(stderr, "Revision inspection failed") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Last Verify Run") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHealthRunner_VerifyRepositoryAccessFailureRemainsDistinctFromIntegrityFailure(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)

		runner := execpkg.NewMockRunner(
			execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 17:30\n"},
			execpkg.MockResult{Err: errors.New("repository invalid")},
			execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 8 exist\n"},
		)

		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "verify",
			RequestedTarget: "onsite-usb",
			Source:          "homes",
			ConfigDir:       configDir,
		})
		if code != 2 || report.Status != "unhealthy" {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
		if !reflect.DeepEqual(report.FailureCodes, []string{verifyFailureAccessFailed}) {
			t.Fatalf("FailureCodes = %#v, want [%q]", report.FailureCodes, verifyFailureAccessFailed)
		}
		if report.PassedRevisionCount != 1 || report.FailedRevisionCount != 0 {
			t.Fatalf("report = %+v", report)
		}
	})

	if !strings.Contains(stderr, "Repository Access") || !strings.Contains(stderr, "Repository is not ready") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Integrity Check") || !strings.Contains(stderr, "All revisions validated") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHealthRunner_VerifyLocalRepositoryRequiresSudoBeforeIntegrityCheck(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	rt.Geteuid = func() int { return 1000 }
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	var runner *execpkg.MockRunner
	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)

		runner = execpkg.NewMockRunner()

		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "verify",
			RequestedTarget: "onsite-usb",
			Source:          "homes",
			ConfigDir:       configDir,
		})
		if code != 2 || report.Status != "unhealthy" {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
		if !reflect.DeepEqual(report.FailureCodes, []string{verifyFailureAccessFailed}) {
			t.Fatalf("FailureCodes = %#v, want [%q]", report.FailureCodes, verifyFailureAccessFailed)
		}
		if report.CheckedRevisionCount != 0 || report.PassedRevisionCount != 0 || report.FailedRevisionCount != 0 {
			t.Fatalf("report = %+v", report)
		}
	})

	if got := len(runner.Invocations); got != 0 {
		t.Fatalf("runner invocations = %d, want no Duplicacy repository calls before sudo-required stop: %#v", got, runner.Invocations)
	}
	for _, token := range []string{
		"Repository Access",
		"Requires sudo",
		"Integrity Check",
		"local filesystem repository is root-protected",
	} {
		if !strings.Contains(stderr, token) {
			t.Fatalf("stderr missing %q: %q", token, stderr)
		}
	}
	if strings.Contains(stderr, "Repository is not ready") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Checking stored revisions") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHealthRunner_StatusLocalRepositoryRequiresSudoBeforeRevisionListing(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	rt.Geteuid = func() int { return 1000 }
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	var runner *execpkg.MockRunner
	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)

		runner = execpkg.NewMockRunner()

		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "status",
			RequestedTarget: "onsite-usb",
			Source:          "homes",
			ConfigDir:       configDir,
		})
		if code != 2 || report.Status != "unhealthy" {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
		if report.RevisionCount != 0 || report.LatestRevision != 0 {
			t.Fatalf("report = %+v", report)
		}
	})

	if got := len(runner.Invocations); got != 0 {
		t.Fatalf("runner invocations = %d, want no Duplicacy repository calls before sudo-required stop: %#v", got, runner.Invocations)
	}
	for _, token := range []string{
		"Repository Access",
		"Requires sudo",
	} {
		if !strings.Contains(stderr, token) {
			t.Fatalf("stderr missing %q: %q", token, stderr)
		}
	}
	if strings.Contains(stderr, "Checking stored revisions") || strings.Contains(stderr, "Repository is not ready") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHealthRunner_AddVerifyFailureCodeDeduplicatesActions(t *testing.T) {
	runner := &HealthRunner{}
	report := &Report{CheckType: "verify"}

	runner.addVerifyFailureCode(report, verifyFailureIntegrityFailed)
	runner.addVerifyFailureCode(report, verifyFailureResultMissing)
	runner.addVerifyFailureCode(report, verifyFailureIntegrityFailed)

	if report.FailureCode != verifyFailureIntegrityFailed {
		t.Fatalf("FailureCode = %q, want %q", report.FailureCode, verifyFailureIntegrityFailed)
	}
	if !reflect.DeepEqual(report.FailureCodes, []string{verifyFailureIntegrityFailed, verifyFailureResultMissing}) {
		t.Fatalf("FailureCodes = %#v", report.FailureCodes)
	}
	if !runner.hasVerifyFailureCode(report, verifyFailureIntegrityFailed) {
		t.Fatal("expected integrity failure code to be present")
	}
	if !runner.hasVerifyFailureCode(report, verifyFailureResultMissing) {
		t.Fatal("expected missing-result failure code to be present")
	}
	if reflect.DeepEqual(report.RecommendedActions, []string{}) || len(report.RecommendedActions) != 3 {
		t.Fatalf("RecommendedActions = %#v", report.RecommendedActions)
	}
}

func TestHealthRunner_VerifyMissingResultsAreShownPerRevision(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastDoctorAt:                 formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)

		runner := execpkg.NewMockRunner(
			execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 17:30\nSnapshot homes revision 7 created at 2026-04-10 16:30\n"},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 8 exist\n"},
		)

		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "verify",
			RequestedTarget: "onsite-usb",
			Source:          "homes",
			ConfigDir:       configDir,
		})
		if code != 2 || report.Status != "unhealthy" {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
	})

	if !strings.Contains(stderr, "Revisions Checked") || !strings.Contains(stderr, "1") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Integrity Check") || !strings.Contains(stderr, "1 revision(s) returned no integrity result: 7") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Revision 7") || !strings.Contains(stderr, "No integrity result returned") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHealthRunner_VerifyOutputUsesAlignedFooter(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 22, 23, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-90 * time.Minute)),
		LastSuccessfulBackupRevision: 2338,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-90 * time.Minute)),
		LastDoctorAt:                 formatReportTime(now.Add(-30 * time.Minute)),
		LastVerifyAt:                 formatReportTime(now.Add(-30 * time.Minute)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)

		runner := execpkg.NewMockRunner(
			execpkg.MockResult{Stdout: "Snapshot homes revision 2338 created at 2026-04-10 18:59\nSnapshot homes revision 2337 created at 2026-04-10 18:30\n"},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "2026-04-10 20:22:24.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 2338 exist\n2026-04-10 20:22:24.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 2337 exist\n"},
		)
		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "verify",
			RequestedTarget: "onsite-usb",
			Source:          "homes",
			ConfigDir:       configDir,
		})
		if code != 0 {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
	})

	if !strings.Contains(stderr, "Revision Count") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Latest Revision") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Last Verify Run") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Status") ||
		!strings.Contains(stderr, "Checking stored revisions") ||
		!strings.Contains(stderr, "Validating repository access") ||
		!strings.Contains(stderr, "Checking revision integrity for this backup (2 total)") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Section: Status") ||
		!strings.Contains(stderr, "Section: Doctor") ||
		!strings.Contains(stderr, "Section: Repository") ||
		!strings.Contains(stderr, "Section: Verify") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Btrfs Root") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Btrfs") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Not checked; backup-readiness validation is not required for storage integrity verification") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Btrfs Source         : /") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Revisions Checked") ||
		!strings.Contains(stderr, "Revisions Passed") ||
		!strings.Contains(stderr, "Revisions Failed") ||
		!strings.Contains(stderr, "Integrity Check") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Verification storage listing") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Verification freshness") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Verify metadata") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "revision-latest") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Doctor freshness") || strings.Contains(stderr, "Verify freshness") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "30m0s ago") || strings.Contains(stderr, "90m0s") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Summary") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Code") || !strings.Contains(stderr, "Duration") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHealthCheckLabelsFitColumnWidth(t *testing.T) {
	names := []string{
		"Environment",
		"Backup state",
		"Lock",
		"Duplicacy setup",
		"Health state",
		"Notification",
		"Config file",
		"Secrets",
		"Revision count",
		"Latest revision",
		"Backup freshness",
		"Source path",
		"Btrfs",
		"Btrfs root",
		"Btrfs source",
		"Repository access",
		"Last doctor run",
		"Revisions checked",
		"Revisions passed",
		"Revisions failed",
		"Integrity check",
		"Last verify run",
	}

	for _, name := range names {
		label := healthCheckLabel(name)
		if len(label) > 20 {
			t.Fatalf("health display label %q exceeds 20 characters (%d)", label, len(label))
		}
	}
}

func TestHealthCheckSection_NotificationUsesAlerts(t *testing.T) {
	if got := healthCheckSection("Notification"); got != "Alerts" {
		t.Fatalf("healthCheckSection(Notification) = %q, want Alerts", got)
	}
}

func TestHealthCheckSection_StatusRevisionFieldsStayInStatus(t *testing.T) {
	for _, name := range []string{"Revision count", "Latest revision"} {
		if got := healthCheckSection(name); got != "Status" {
			t.Fatalf("healthCheckSection(%q) = %q, want Status", name, got)
		}
	}
}

func TestHumanAgeFormatting(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{name: "sub-minute", in: 30 * time.Second, want: "less than 1m"},
		{name: "minutes", in: 14 * time.Minute, want: "14m"},
		{name: "hours-minutes", in: 2*time.Hour + 49*time.Minute, want: "2h49m"},
		{name: "days-hours", in: 26 * time.Hour, want: "1d2h"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := humanAge(tc.in); got != tc.want {
				t.Fatalf("humanAge(%s) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHumanAgoFormatting(t *testing.T) {
	if got := humanAgo(45 * time.Second); got != "<1m ago" {
		t.Fatalf("humanAgo(sub-minute) = %q", got)
	}
	if got := humanAgo(25 * time.Minute); got != "25m ago" {
		t.Fatalf("humanAgo(25m) = %q", got)
	}
}

func TestHealthRunner_VerboseOutputStaysStructured(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 22, 23, 0, time.UTC)
	meta := workflow.MetadataForLogDir("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\nprune = \"-keep 0:365\"\nthreads = 4\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-90 * time.Minute)),
		LastSuccessfulBackupRevision: 2338,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-90 * time.Minute)),
		LastDoctorAt:                 formatReportTime(now.Add(-30 * time.Second)),
		LastVerifyAt:                 formatReportTime(now.Add(-30 * time.Second)),
	}
	if err := saveRunState(meta, "homes", "onsite-usb", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	stderr := captureHealthOutput(t, func() {
		log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
		if err != nil {
			t.Fatalf("logger.New() error = %v", err)
		}
		t.Cleanup(log.Close)
		log.SetVerbose(true)

		runner := execpkg.NewMockRunner(
			execpkg.MockResult{Stdout: "Snapshot homes revision 2338 created at 2026-04-10 18:59\nSnapshot homes revision 2337 created at 2026-04-10 18:30\n"},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "2026-04-10 20:22:24.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 2338 exist\n2026-04-10 20:22:24.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 2337 exist\n"},
		)
		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand:   "verify",
			RequestedTarget: "onsite-usb",
			Source:          "homes",
			ConfigDir:       configDir,
			Verbose:         true,
		})
		if code != 0 {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
	})

	if strings.Contains(stderr, "exec: ") {
		t.Fatalf("stderr should not contain raw exec debug lines: %q", stderr)
	}
	if !strings.Contains(stderr, "Section: Status") ||
		!strings.Contains(stderr, "Section: Doctor") ||
		!strings.Contains(stderr, "Section: Repository") ||
		!strings.Contains(stderr, "Section: Verify") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "<1m ago") {
		t.Fatalf("stderr = %q", stderr)
	}
}
