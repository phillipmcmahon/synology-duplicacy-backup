package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
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

func withWebhookTokenLoader(t *testing.T, loader func(string) (string, error)) {
	t.Helper()
	old := loadOptionalHealthWebhookToken
	loadOptionalHealthWebhookToken = loader
	t.Cleanup(func() {
		loadOptionalHealthWebhookToken = old
	})
}

func healthOwnerGroup(t *testing.T) (string, string) {
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

func newHealthRuntime(now time.Time, tempDir string) Runtime {
	rt := DefaultRuntime()
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
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func TestHealthRunner_StatusHealthy(t *testing.T) {
	now := time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC)
	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)

	owner, group := healthOwnerGroup(t)
	configDir := t.TempDir()
	writeHealthConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n[health]\nfreshness_warn_hours = 12\nfreshness_fail_hours = 24\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	runner := execpkg.NewMockRunner(execpkg.MockResult{
		Stdout: "Snapshot homes revision 8 created at 2026-04-10 16:30\n",
	})
	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand: "status",
		Source:        "homes",
		ConfigDir:     configDir,
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
	if report.LatestRevisionAt == "" || report.LocalLastSuccessAt == "" {
		t.Fatalf("report = %+v", report)
	}
}

func TestHealthRunner_VerifyUnhealthyWhenStorageTooOld(t *testing.T) {
	now := time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC)
	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)

	owner, group := healthOwnerGroup(t)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n[health]\nfreshness_warn_hours = 1\nfreshness_fail_hours = 2\nverify_warn_after_hours = 24\n")

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 12:00\n"},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "2026-04-10 12:10:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 8 exist\n"},
	)
	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand: "verify",
		Source:        "homes",
		ConfigDir:     configDir,
	})
	if code != 2 {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if report.Status != "unhealthy" {
		t.Fatalf("report = %+v", report)
	}
}

func TestHealthWebhookDelivery(t *testing.T) {
	var gotAuth string
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC), t.TempDir())
	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)

	withWebhookTokenLoader(t, func(string) (string, error) {
		return "hook-token", nil
	})

	report := NewFailureHealthReport(&Request{HealthCommand: "verify", Source: "homes"}, "verify", "boom", rt.Now())
	cfg := config.HealthNotifyConfig{WebhookURL: server.URL}
	if err := NewHealthRunner(meta, rt, log, execpkg.NewMockRunner()).sendWebhook(cfg, "", report); err != nil {
		t.Fatalf("sendWebhook() error = %v", err)
	}
	if gotAuth != "Bearer hook-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}

func TestHealthWebhookDelivery_WhenStdinIsNotTTY(t *testing.T) {
	var hits int
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
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

	owner, group := healthOwnerGroup(t)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n[health]\nfreshness_warn_hours = 1\nfreshness_fail_hours = 2\n[health.notify]\nwebhook_url = \""+server.URL+"\"\nnotify_on = [\"degraded\", \"unhealthy\"]\nsend_for = [\"doctor\", \"verify\"]\ninteractive = false\n")

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Err: errors.New("failed to list revisions for latest revision inspection")},
		execpkg.MockResult{Err: errors.New("repository invalid")},
	)
	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand: "doctor",
		Source:        "homes",
		ConfigDir:     configDir,
		SecretsDir:    t.TempDir(),
	})
	if code != 2 {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if !report.WebhookSent {
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

	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
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

	owner, group := healthOwnerGroup(t)
	configDir := t.TempDir()
	writeHealthConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n[health.notify]\nwebhook_url = \""+server.URL+"\"\nnotify_on = [\"degraded\", \"unhealthy\"]\nsend_for = [\"doctor\", \"verify\"]\ninteractive = false\n")

	report, code := NewHealthRunner(meta, rt, log, execpkg.NewMockRunner()).Run(&Request{
		HealthCommand: "doctor",
		Source:        "homes",
		ConfigDir:     configDir,
		SecretsDir:    t.TempDir(),
	})
	if code != 2 {
		t.Fatalf("code = %d, report = %+v", code, report)
	}
	if !report.WebhookSent {
		t.Fatalf("report = %+v", report)
	}
	if hits != 1 {
		t.Fatalf("webhook hits = %d, want 1", hits)
	}
}

func TestWriteHealthReport_DoesNotIncludeSummaryField(t *testing.T) {
	report := NewFailureHealthReport(&Request{HealthCommand: "doctor", Source: "homes"}, "doctor", "boom", time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC))

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
	report := &HealthReport{
		Status:               "healthy",
		CheckType:            "verify",
		Label:                "homes",
		Mode:                 "Local",
		CheckedAt:            "2026-04-10T22:25:20Z",
		LastVerifyRunAt:      "2026-04-10T22:25:20Z",
		CheckedRevisionCount: 79,
		PassedRevisionCount:  79,
		Checks: []HealthCheck{
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
}

func TestHealthRunner_VerifyHealthyWhenAllVisibleRevisionsValidate(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())

	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)

	owner, group := healthOwnerGroup(t)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n[health]\nfreshness_warn_hours = 24\nfreshness_fail_hours = 48\nverify_warn_after_hours = 24\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastDoctorAt:                 formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 17:30\nSnapshot homes revision 7 created at 2026-04-10 16:30\n"},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 8 exist\n2026-04-10 20:00:01.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 7 exist\n"},
	)

	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand: "verify",
		Source:        "homes",
		ConfigDir:     configDir,
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

func TestHealthRunner_VerifyUnhealthyWhenResultsDoNotCoverAllVisibleRevisions(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())

	log, err := logger.New(t.TempDir(), "duplicacy-backup", false)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	t.Cleanup(log.Close)

	owner, group := healthOwnerGroup(t)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastDoctorAt:                 formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", state); err != nil {
		t.Fatalf("saveRunState() error = %v", err)
	}

	runner := execpkg.NewMockRunner(
		execpkg.MockResult{Stdout: "Snapshot homes revision 8 created at 2026-04-10 17:30\nSnapshot homes revision 7 created at 2026-04-10 16:30\n"},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "btrfs\n"},
		execpkg.MockResult{},
		execpkg.MockResult{},
		execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 8 exist\n"},
	)

	report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
		HealthCommand: "verify",
		Source:        "homes",
		ConfigDir:     configDir,
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
}

func TestHealthRunner_VerifyFailureSummaryIsOperatorFriendly(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())

	owner, group := healthOwnerGroup(t)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastDoctorAt:                 formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", state); err != nil {
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
			execpkg.MockResult{Stdout: "btrfs\n"},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "btrfs\n"},
			execpkg.MockResult{},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 WARN SNAPSHOT_CHECK Some chunks referenced by snapshot homes at revision 8 are missing\n2026-04-10 20:00:01.000 WARN SNAPSHOT_CHECK Some chunks referenced by snapshot homes at revision 7 are missing\n"},
		)

		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand: "verify",
			Source:        "homes",
			ConfigDir:     configDir,
		})
		if code != 2 || report.Status != "unhealthy" {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
	})

	if !strings.Contains(stderr, "Revisions failed") || !strings.Contains(stderr, "2 (8, 7)") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Integrity check") || !strings.Contains(stderr, "2 revision(s) failed integrity checks: 8, 7") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Revision 8") || !strings.Contains(stderr, "Revision 7") || !strings.Contains(stderr, "Missing chunks") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHealthRunner_VerifyMissingResultsAreShownPerRevision(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 0, 0, 0, time.UTC)
	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())

	owner, group := healthOwnerGroup(t)
	configDir := t.TempDir()
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-2 * time.Hour)),
		LastSuccessfulBackupRevision: 8,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-2 * time.Hour)),
		LastDoctorAt:                 formatReportTime(now.Add(-2 * time.Hour)),
		LastVerifyAt:                 formatReportTime(now.Add(-2 * time.Hour)),
	}
	if err := saveRunState(meta, "homes", state); err != nil {
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
			execpkg.MockResult{Stdout: "btrfs\n"},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "btrfs\n"},
			execpkg.MockResult{},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "2026-04-10 20:00:00.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 8 exist\n"},
		)

		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand: "verify",
			Source:        "homes",
			ConfigDir:     configDir,
		})
		if code != 2 || report.Status != "unhealthy" {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
	})

	if !strings.Contains(stderr, "Revisions checked") || !strings.Contains(stderr, "1") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Integrity check") || !strings.Contains(stderr, "1 revision(s) returned no integrity result: 7") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Revision 7") || !strings.Contains(stderr, "No integrity result returned") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHealthRunner_VerifyOutputUsesAlignedFooter(t *testing.T) {
	now := time.Date(2026, 4, 10, 20, 22, 23, 0, time.UTC)
	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	owner, group := healthOwnerGroup(t)
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-90 * time.Minute)),
		LastSuccessfulBackupRevision: 2338,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-90 * time.Minute)),
		LastDoctorAt:                 formatReportTime(now.Add(-30 * time.Minute)),
		LastVerifyAt:                 formatReportTime(now.Add(-30 * time.Minute)),
	}
	if err := saveRunState(meta, "homes", state); err != nil {
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
			execpkg.MockResult{Stdout: "btrfs\n"},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "btrfs\n"},
			execpkg.MockResult{},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "2026-04-10 20:22:24.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 2338 exist\n2026-04-10 20:22:24.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 2337 exist\n"},
		)
		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand: "verify",
			Source:        "homes",
			ConfigDir:     configDir,
		})
		if code != 0 {
			t.Fatalf("code = %d, report = %+v", code, report)
		}
	})

	if !strings.Contains(stderr, "Revision count") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Latest revision") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Last doctor run") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Last verify run") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Status") ||
		!strings.Contains(stderr, "Inspecting visible storage revisions") ||
		!strings.Contains(stderr, "Validating repository access") ||
		!strings.Contains(stderr, "Checking revisions for this backup (2 total)") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Section: Status") ||
		!strings.Contains(stderr, "Section: Doctor") ||
		!strings.Contains(stderr, "Section: Verify") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Btrfs root") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Btrfs") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Yes") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "Btrfs source         : /") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Revisions checked") ||
		!strings.Contains(stderr, "Revisions passed") ||
		!strings.Contains(stderr, "Revisions failed") ||
		!strings.Contains(stderr, "Integrity check") {
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
		"Local state",
		"Lock",
		"Duplicacy setup",
		"Health state",
		"Webhook",
		"Config",
		"Remote secrets",
		"Revision count",
		"Latest revision",
		"Storage freshness",
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

func TestHealthCheckSection_WebhookUsesAlerts(t *testing.T) {
	if got := healthCheckSection("Webhook"); got != "Alerts" {
		t.Fatalf("healthCheckSection(Webhook) = %q, want Alerts", got)
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
	meta := DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir())
	meta.StateDir = t.TempDir()
	rt := newHealthRuntime(now, t.TempDir())
	configDir := t.TempDir()
	owner, group := healthOwnerGroup(t)
	sourceRoot := t.TempDir()
	meta.RootVolume = sourceRoot
	if err := os.MkdirAll(filepath.Join(sourceRoot, "homes"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeHealthConfig(t, configDir, "homes", "[common]\ndestination = \"/backups\"\nprune = \"-keep 0:365\"\nthreads = 4\n[local]\nlocal_owner = \""+owner+"\"\nlocal_group = \""+group+"\"\n")

	state := &RunState{
		LastSuccessfulRunAt:          formatReportTime(now.Add(-90 * time.Minute)),
		LastSuccessfulBackupRevision: 2338,
		LastSuccessfulBackupAt:       formatReportTime(now.Add(-90 * time.Minute)),
		LastDoctorAt:                 formatReportTime(now.Add(-30 * time.Second)),
		LastVerifyAt:                 formatReportTime(now.Add(-30 * time.Second)),
	}
	if err := saveRunState(meta, "homes", state); err != nil {
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
			execpkg.MockResult{Stdout: "btrfs\n"},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "btrfs\n"},
			execpkg.MockResult{},
			execpkg.MockResult{},
			execpkg.MockResult{Stdout: "2026-04-10 20:22:24.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 2338 exist\n2026-04-10 20:22:24.000 INFO SNAPSHOT_CHECK All chunks referenced by snapshot homes at revision 2337 exist\n"},
		)
		report, code := NewHealthRunner(meta, rt, log, runner).Run(&Request{
			HealthCommand: "verify",
			Source:        "homes",
			ConfigDir:     configDir,
			Verbose:       true,
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
		!strings.Contains(stderr, "Section: Verify") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "<1m ago") {
		t.Fatalf("stderr = %q", stderr)
	}
}
