package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

var profileChown = os.Chown

type RunState = workflowcore.RunState

func stateFilePath(meta Metadata, label string, target string) string {
	return workflowcore.StateFilePath(meta, label, target)
}

// StateFilePath returns the profile state file path for report-only command
// subsystems.
func StateFilePath(meta Metadata, label string, target string) string {
	return stateFilePath(meta, label, target)
}

func loadRunState(meta Metadata, label, target string) (*RunState, error) {
	return workflowcore.LoadRunState(meta, label, target)
}

// LoadRunState reads prior run state for report-only command subsystems.
func LoadRunState(meta Metadata, label, target string) (*RunState, error) {
	return loadRunState(meta, label, target)
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
	if err := chownProfilePath(meta, meta.StateDir); err != nil {
		return err
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
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("failed to set state file permissions on %s: %w", path, err)
	}
	if err := chownProfilePath(meta, path); err != nil {
		return err
	}
	return nil
}

func chownProfilePath(meta Metadata, path string) error {
	if !meta.HasProfileOwner {
		return nil
	}
	if err := profileChown(path, meta.ProfileOwnerUID, meta.ProfileOwnerGID); err != nil {
		return fmt.Errorf("failed to set profile ownership on %s to %d:%d: %w", path, meta.ProfileOwnerUID, meta.ProfileOwnerGID, err)
	}
	return nil
}

func updateRunState(meta Metadata, plan *Plan, report *RunReport, backupRevision int) error {
	if plan == nil || report == nil || plan.Config.BackupLabel == "" {
		return nil
	}

	return mutateRunState(meta, plan.Config.BackupLabel, plan.TargetName(), func(state *RunState) error {
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
		return nil
	})
}

func updateHealthCheckState(meta Metadata, label, target string, checkType string, checkedAt time.Time) error {
	if label == "" {
		return nil
	}
	return mutateRunState(meta, label, target, func(state *RunState) error {
		formatted := formatReportTime(checkedAt)
		switch checkType {
		case "status":
			state.LastStatusAt = formatted
		case "doctor":
			state.LastDoctorAt = formatted
		case "verify":
			state.LastVerifyAt = formatted
		}
		return nil
	})
}

func mutateRunState(meta Metadata, label, target string, mutate func(*RunState) error) error {
	state := &RunState{}
	if existing, err := loadRunState(meta, label, target); err == nil {
		state = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if mutate != nil {
		if err := mutate(state); err != nil {
			return err
		}
	}
	return saveRunState(meta, label, target, state)
}
