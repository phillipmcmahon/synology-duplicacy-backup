package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type RunState struct {
	Label                        string `json:"label,omitempty"`
	Target                       string `json:"target,omitempty"`
	LastRunStartedAt             string `json:"last_run_started_at,omitempty"`
	LastRunCompletedAt           string `json:"last_run_completed_at,omitempty"`
	LastRunResult                string `json:"last_run_result,omitempty"`
	LastSuccessfulRunAt          string `json:"last_successful_run_at,omitempty"`
	LastSuccessfulOperation      string `json:"last_successful_operation,omitempty"`
	LastSuccessfulBackupRevision int    `json:"last_successful_backup_revision,omitempty"`
	LastSuccessfulBackupAt       string `json:"last_successful_backup_at,omitempty"`
	LastFailureSummary           string `json:"last_failure_summary,omitempty"`
	LastStatusAt                 string `json:"last_status_at,omitempty"`
	LastDoctorAt                 string `json:"last_doctor_at,omitempty"`
	LastVerifyAt                 string `json:"last_verify_at,omitempty"`
}

func stateFilePath(meta Metadata, label string, target string) string {
	return filepath.Join(meta.StateDir, fmt.Sprintf("%s.%s.json", label, target))
}

func loadRunState(meta Metadata, label, target string) (*RunState, error) {
	path := stateFilePath(meta, label, target)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state RunState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("invalid state file %s: %w", path, err)
	}
	if state.Label == "" {
		state.Label = label
	}
	if state.Target == "" {
		state.Target = target
	}
	return &state, nil
}

func saveRunState(meta Metadata, label, target string, state *RunState) error {
	if state == nil {
		return nil
	}
	if err := os.MkdirAll(meta.StateDir, 0700); err != nil {
		return fmt.Errorf("failed to create state directory %s: %w", meta.StateDir, err)
	}
	if err := os.Chmod(meta.StateDir, 0700); err != nil {
		return fmt.Errorf("failed to set state directory permissions on %s: %w", meta.StateDir, err)
	}
	state.Label = label
	state.Target = target
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode state file: %w", err)
	}
	body = append(body, '\n')
	path := stateFilePath(meta, label, target)
	if err := os.WriteFile(path, body, 0600); err != nil {
		return fmt.Errorf("failed to write state file %s: %w", path, err)
	}
	return nil
}

func updateRunState(meta Metadata, plan *Plan, report *RunReport, backupRevision int) error {
	if plan == nil || report == nil || plan.BackupLabel == "" {
		return nil
	}

	state := &RunState{}
	if existing, err := loadRunState(meta, plan.BackupLabel, plan.TargetName()); err == nil {
		state = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	state.LastRunStartedAt = report.StartedAt
	state.LastRunCompletedAt = report.CompletedAt
	state.LastRunResult = report.Result
	if report.Result == "success" {
		state.LastSuccessfulRunAt = report.CompletedAt
		state.LastSuccessfulOperation = report.Operation
		state.LastFailureSummary = ""
		if backupRevision > 0 {
			state.LastSuccessfulBackupRevision = backupRevision
			state.LastSuccessfulBackupAt = report.CompletedAt
		}
	} else if report.FailureMessage != "" {
		state.LastFailureSummary = report.FailureMessage
	}

	return saveRunState(meta, plan.BackupLabel, plan.TargetName(), state)
}

func updateHealthCheckState(meta Metadata, label, target string, checkType string, checkedAt time.Time) error {
	if label == "" {
		return nil
	}
	state := &RunState{}
	if existing, err := loadRunState(meta, label, target); err == nil {
		state = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	formatted := formatReportTime(checkedAt)
	switch checkType {
	case "status":
		state.LastStatusAt = formatted
	case "doctor":
		state.LastDoctorAt = formatted
	case "verify":
		state.LastVerifyAt = formatted
	}
	return saveRunState(meta, label, target, state)
}
