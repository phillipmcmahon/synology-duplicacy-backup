package workflowcore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// StateFilePath returns the profile state file path for report-only command
// subsystems.
func StateFilePath(meta Metadata, label string, target string) string {
	return filepath.Join(meta.StateDir, fmt.Sprintf("%s.%s.json", label, target))
}

// LoadRunState reads prior run state for report-only command subsystems.
func LoadRunState(meta Metadata, label, target string) (*RunState, error) {
	path := StateFilePath(meta, label, target)
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
