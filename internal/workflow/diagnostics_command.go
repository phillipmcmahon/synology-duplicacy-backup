package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"syscall"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type DiagnosticsReport struct {
	Label             string                   `json:"label"`
	Target            string                   `json:"target"`
	Location          string                   `json:"location"`
	ConfigDir         string                   `json:"config_dir"`
	ConfigFile        string                   `json:"config_file"`
	SecretsDir        string                   `json:"secrets_dir"`
	SecretsFile       string                   `json:"secrets_file,omitempty"`
	SecretsStatus     string                   `json:"secrets_status"`
	SourcePath        string                   `json:"source_path"`
	Storage           string                   `json:"storage"`
	StorageScheme     string                   `json:"storage_scheme"`
	StateFile         string                   `json:"state_file"`
	StateStatus       string                   `json:"state_status"`
	LastRunResult     string                   `json:"last_run_result,omitempty"`
	LastRunCompleted  string                   `json:"last_run_completed_at,omitempty"`
	LastSuccessfulRun string                   `json:"last_successful_run_at,omitempty"`
	LastBackup        string                   `json:"last_successful_backup_at,omitempty"`
	LastBackupRev     *int                     `json:"last_successful_backup_revision,omitempty"`
	LastFailure       string                   `json:"last_failure_summary,omitempty"`
	LastStatus        string                   `json:"last_status_at,omitempty"`
	LastDoctor        string                   `json:"last_doctor_at,omitempty"`
	LastVerify        string                   `json:"last_verify_at,omitempty"`
	Paths             []DiagnosticsPathSummary `json:"paths"`
}

type DiagnosticsPathSummary struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

func HandleDiagnosticsCommand(req *Request, meta Metadata, rt Runtime) (string, error) {
	diagnosticsReq := NewDiagnosticsRequest(req)
	planner := NewConfigPlanner(meta, rt)
	plan := planner.derivePlan(diagnosticsReq.PlanRequest())
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.applyConfig(cfg, rt)

	report := newDiagnosticsReport(&diagnosticsReq, meta, plan)
	if diagnosticsReq.JSONSummary {
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to encode diagnostics report: %w", err)
		}
		return string(append(body, '\n')), nil
	}
	return formatDiagnosticsReport(report), nil
}

func newDiagnosticsReport(req *DiagnosticsRequest, meta Metadata, plan *Plan) *DiagnosticsReport {
	storage := duplicacy.NewStorageSpec(plan.BackupTarget)
	report := &DiagnosticsReport{
		Label:         req.Label,
		Target:        req.Target(),
		Location:      plan.Location,
		ConfigDir:     plan.ConfigDir,
		ConfigFile:    plan.ConfigFile,
		SecretsDir:    plan.SecretsDir,
		SourcePath:    plan.SnapshotSource,
		Storage:       redactStorageValue(plan.BackupTarget),
		StorageScheme: storage.Scheme(),
		StateFile:     stateFilePath(meta, req.Label, req.Target()),
		StateStatus:   "Not found",
		Paths: []DiagnosticsPathSummary{
			pathSummary("Config Dir", plan.ConfigDir),
			pathSummary("Config File", plan.ConfigFile),
			pathSummary("Source Path", plan.SnapshotSource),
			pathSummary("State Dir", meta.StateDir),
			pathSummary("State File", stateFilePath(meta, req.Label, req.Target())),
			pathSummary("Log Dir", meta.LogDir),
		},
	}
	if storage.IsLocalPath() {
		report.Paths = append(report.Paths, pathSummary("Storage Path", plan.BackupTarget))
	} else {
		report.Paths = append(report.Paths, DiagnosticsPathSummary{
			Name:   "Storage Path",
			Path:   report.Storage,
			Status: "Not locally inspectable",
		})
	}
	report.applySecretsStatus(storage)
	report.applyState(meta, req.Label, req.Target())
	return report
}

func (r *DiagnosticsReport) applySecretsStatus(storage duplicacy.StorageSpec) {
	if !storage.NeedsSecrets() {
		r.SecretsStatus = "Not required"
		return
	}
	r.SecretsFile = secrets.GetSecretsFilePath(r.SecretsDir, r.Label)
	r.Paths = append(r.Paths, pathSummary("Secrets File", r.SecretsFile))
	file, err := os.Open(r.SecretsFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.SecretsStatus = "Required (file not found)"
			return
		}
		r.SecretsStatus = fmt.Sprintf("Required (not readable: %s)", OperatorMessage(err))
		return
	}
	defer file.Close()
	sec, err := secrets.ParseSecrets(file, r.SecretsFile, r.Target)
	if err != nil {
		r.SecretsStatus = fmt.Sprintf("Required (invalid: %s)", OperatorMessage(err))
		return
	}
	if err := storage.ValidateSecrets(sec); err != nil {
		r.SecretsStatus = fmt.Sprintf("Required (missing keys: %s)", OperatorMessage(err))
		return
	}
	r.SecretsStatus = fmt.Sprintf("Available (%s)", sec.MaskedKeys())
}

func (r *DiagnosticsReport) applyState(meta Metadata, label, target string) {
	state, err := loadRunState(meta, label, target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.StateStatus = "Not found"
			return
		}
		r.StateStatus = fmt.Sprintf("Unreadable (%s)", OperatorMessage(err))
		return
	}
	r.StateStatus = "Available"
	r.LastRunResult = state.LastRunResult
	r.LastRunCompleted = state.LastRunCompletedAt
	r.LastSuccessfulRun = state.LastSuccessfulRunAt
	r.LastBackup = state.LastSuccessfulBackupAt
	lastBackupRev := state.LastSuccessfulBackupRevision
	r.LastBackupRev = &lastBackupRev
	r.LastFailure = state.LastFailureSummary
	r.LastStatus = state.LastStatusAt
	r.LastDoctor = state.LastDoctorAt
	r.LastVerify = state.LastVerifyAt
}

func pathSummary(name, path string) DiagnosticsPathSummary {
	summary := DiagnosticsPathSummary{Name: name, Path: path}
	if strings.TrimSpace(path) == "" {
		summary.Status = "Not configured"
		return summary
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			summary.Status = "Missing"
			return summary
		}
		summary.Status = fmt.Sprintf("Not accessible (%s)", OperatorMessage(err))
		return summary
	}
	kind := "file"
	if info.IsDir() {
		kind = "dir"
	}
	summary.Status = fmt.Sprintf("Exists (%s, mode %04o%s)", kind, info.Mode().Perm(), ownerSuffix(info))
	return summary
}

func ownerSuffix(info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return fmt.Sprintf(", owner %d:%d", stat.Uid, stat.Gid)
}

func redactStorageValue(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return value
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword("****", "****")
	}
	query := parsed.Query()
	for key := range query {
		if isSensitiveQueryKey(key) {
			query.Set(key, "****")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func isSensitiveQueryKey(key string) bool {
	key = strings.ToLower(key)
	for _, token := range []string{"token", "secret", "password", "passwd", "pass", "key"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func formatDiagnosticsReport(report *DiagnosticsReport) string {
	var b strings.Builder
	b.WriteString(presentation.FormatLines(fmt.Sprintf("Diagnostics for %s/%s", report.Label, report.Target), []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Target", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Config File", Value: report.ConfigFile},
		{Label: "Source Path", Value: report.SourcePath},
		{Label: "Storage", Value: report.Storage},
		{Label: "Storage Scheme", Value: report.StorageScheme},
	}))
	writeDiagnosticsSection(&b, "Secrets", []SummaryLine{
		{Label: "Secrets File", Value: presentation.DisplayEmpty(report.SecretsFile, "Not required")},
		{Label: "Secrets Status", Value: report.SecretsStatus},
	})
	writeDiagnosticsSection(&b, "State", diagnosticsStateLines(report))
	writeDiagnosticsSection(&b, "Permissions", diagnosticsPathLines(report.Paths))
	return b.String()
}

func diagnosticsStateLines(report *DiagnosticsReport) []SummaryLine {
	lines := []SummaryLine{
		{Label: "State File", Value: report.StateFile},
		{Label: "State Status", Value: report.StateStatus},
	}
	if report.LastRunResult != "" {
		lines = append(lines, SummaryLine{Label: "Last Run Result", Value: report.LastRunResult})
	}
	if report.LastRunCompleted != "" {
		lines = append(lines, SummaryLine{Label: "Last Run Completed", Value: report.LastRunCompleted})
	}
	if report.LastSuccessfulRun != "" {
		lines = append(lines, SummaryLine{Label: "Last Successful Run", Value: report.LastSuccessfulRun})
	}
	if report.LastBackupRev != nil {
		lines = append(lines, SummaryLine{Label: "Last Backup Rev", Value: fmt.Sprintf("%d", *report.LastBackupRev)})
	}
	if report.LastBackup != "" {
		lines = append(lines, SummaryLine{Label: "Last Backup", Value: report.LastBackup})
	}
	if report.LastFailure != "" {
		lines = append(lines, SummaryLine{Label: "Last Failure", Value: report.LastFailure})
	}
	if report.LastStatus != "" {
		lines = append(lines, SummaryLine{Label: "Last Status", Value: report.LastStatus})
	}
	if report.LastDoctor != "" {
		lines = append(lines, SummaryLine{Label: "Last Doctor", Value: report.LastDoctor})
	}
	if report.LastVerify != "" {
		lines = append(lines, SummaryLine{Label: "Last Verify", Value: report.LastVerify})
	}
	return lines
}

func diagnosticsPathLines(paths []DiagnosticsPathSummary) []SummaryLine {
	lines := make([]SummaryLine, 0, len(paths))
	for _, path := range paths {
		lines = append(lines, SummaryLine{Label: path.Name, Value: fmt.Sprintf("%s (%s)", path.Path, path.Status)})
	}
	return lines
}

func writeDiagnosticsSection(b *strings.Builder, name string, lines []SummaryLine) {
	fmt.Fprintf(b, "  Section: %s\n", name)
	for _, line := range lines {
		fmt.Fprintf(b, "    %-18s : %s\n", line.Label, line.Value)
	}
}
