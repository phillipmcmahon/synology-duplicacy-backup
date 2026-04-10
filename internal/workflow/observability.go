package workflow

import (
	"encoding/json"
	"io"
	"time"
)

type RunReport struct {
	Label          string        `json:"label,omitempty"`
	Operation      string        `json:"operation,omitempty"`
	Mode           string        `json:"mode,omitempty"`
	Result         string        `json:"result"`
	ExitCode       int           `json:"exit_code"`
	DryRun         bool          `json:"dry_run"`
	Remote         bool          `json:"remote"`
	StartedAt      string        `json:"started_at"`
	CompletedAt    string        `json:"completed_at"`
	DurationSecond int           `json:"duration_seconds"`
	FailureMessage string        `json:"failure_message,omitempty"`
	Phases         []PhaseReport `json:"phases,omitempty"`
}

type PhaseReport struct {
	Name           string `json:"name"`
	Result         string `json:"result"`
	StartedAt      string `json:"started_at"`
	CompletedAt    string `json:"completed_at"`
	DurationSecond int    `json:"duration_seconds"`
}

func NewRunReport(plan *Plan, startedAt time.Time) *RunReport {
	report := &RunReport{
		Result:    "success",
		ExitCode:  0,
		StartedAt: formatReportTime(startedAt),
	}
	if plan == nil {
		return report
	}

	report.Label = plan.BackupLabel
	report.Operation = plan.OperationMode
	report.Mode = plan.ModeDisplay
	report.DryRun = plan.DryRun
	report.Remote = plan.RemoteMode
	return report
}

func NewFailureRunReport(req *Request, startedAt time.Time, completedAt time.Time, exitCode int, message string) *RunReport {
	report := &RunReport{
		Result:         "failed",
		ExitCode:       exitCode,
		DryRun:         req != nil && req.DryRun,
		Remote:         req != nil && req.RemoteMode,
		StartedAt:      formatReportTime(startedAt),
		CompletedAt:    formatReportTime(completedAt),
		DurationSecond: int(durationSeconds(completedAt.Sub(startedAt))),
		FailureMessage: message,
	}
	if req != nil {
		report.Label = req.Source
		report.Operation = OperationMode(req)
		if req.RemoteMode {
			report.Mode = "Remote"
		} else {
			report.Mode = "Local"
		}
	}
	return report
}

func (r *RunReport) StartPhase(name string, startedAt time.Time) int {
	r.Phases = append(r.Phases, PhaseReport{
		Name:      name,
		Result:    "running",
		StartedAt: formatReportTime(startedAt),
	})
	return len(r.Phases) - 1
}

func (r *RunReport) CompletePhase(index int, result string, completedAt time.Time) {
	if index < 0 || index >= len(r.Phases) {
		return
	}
	phase := &r.Phases[index]
	phase.Result = result
	phase.CompletedAt = formatReportTime(completedAt)
	if started, err := time.Parse(time.RFC3339, phase.StartedAt); err == nil {
		phase.DurationSecond = int(durationSeconds(completedAt.Sub(started)))
	}
}

func (r *RunReport) CompleteRun(exitCode int, message string, completedAt time.Time) {
	r.ExitCode = exitCode
	if exitCode == 0 {
		r.Result = "success"
	} else {
		r.Result = "failed"
		r.FailureMessage = message
	}
	r.CompletedAt = formatReportTime(completedAt)
	if started, err := time.Parse(time.RFC3339, r.StartedAt); err == nil {
		r.DurationSecond = int(durationSeconds(completedAt.Sub(started)))
	}
}

func WriteRunReport(w io.Writer, report *RunReport) error {
	if report == nil {
		return nil
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func formatReportTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func durationSeconds(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	return d.Truncate(time.Second) / time.Second
}
