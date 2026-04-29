package main

import (
	"os"
	"strings"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/update"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func TestUpdateOptionsFromRequestMapsCommandRequest(t *testing.T) {
	got := updateOptionsFromRequest(&workflow.UpdateRequest{
		Version:      "v4.1.8",
		CheckOnly:    true,
		Force:        true,
		Yes:          true,
		Keep:         7,
		Attestations: "required",
	})

	if got.RequestedVersion != "v4.1.8" || !got.CheckOnly || !got.Force || !got.Yes || got.Keep != 7 || got.Attestations != "required" {
		t.Fatalf("updateOptionsFromRequest() = %+v", got)
	}
}

func TestRollbackOptionsFromRequestMapsCommandRequest(t *testing.T) {
	got := rollbackOptionsFromRequest(&workflow.RollbackRequest{
		Version:   "v5.1.1",
		CheckOnly: true,
		Yes:       true,
	})

	if got.RequestedVersion != "v5.1.1" || !got.CheckOnly || !got.Yes {
		t.Fatalf("rollbackOptionsFromRequest() = %+v", got)
	}
}

func TestUpdateStatusForWorkflowMappingContract(t *testing.T) {
	tests := []struct {
		name string
		in   update.Status
		want workflow.UpdateStatus
	}{
		{name: "installed", in: update.StatusInstalled, want: workflow.UpdateStatusInstalled},
		{name: "current", in: update.StatusCurrent, want: workflow.UpdateStatusCurrent},
		{name: "available", in: update.StatusAvailable, want: workflow.UpdateStatusAvailable},
		{name: "reinstall requested", in: update.StatusReinstallRequested, want: workflow.UpdateStatusReinstallRequested},
		{name: "failed", in: update.StatusFailed, want: workflow.UpdateStatusFailed},
		{name: "cancelled", in: update.StatusCancelled, want: workflow.UpdateStatusCancelled},
		{name: "unknown", in: update.StatusUnknown, want: workflow.UpdateStatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := updateStatusForWorkflow(tt.in); got != tt.want {
				t.Fatalf("updateStatusForWorkflow(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHandleUpdateRequestUsesWorkflowRuntime(t *testing.T) {
	_, err := handleUpdateRequest(
		&workflow.UpdateRequest{Command: "update", CheckOnly: true, Keep: update.DefaultKeep},
		workflow.Metadata{ScriptName: "duplicacy-backup", Version: "v4.1.8"},
		workflow.Env{
			Stdin:        func() *os.File { return os.Stdin },
			StdinIsTTY:   func() bool { return false },
			Executable:   func() (string, error) { return "/tmp/custom-binary", nil },
			EvalSymlinks: func(path string) (string, error) { return path, nil },
			TempDir:      os.TempDir,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "managed stable command path") {
		t.Fatalf("handleUpdateRequest() err = %v", err)
	}
}

func TestUpdateOptionsFromNilRequestUsesDefaultKeep(t *testing.T) {
	got := updateOptionsFromRequest(nil)
	if got.Keep != update.DefaultKeep {
		t.Fatalf("Keep = %d, want %d", got.Keep, update.DefaultKeep)
	}
	if got.RequestedVersion != "" || got.CheckOnly || got.Force || got.Yes || got.Attestations != "" {
		t.Fatalf("updateOptionsFromRequest(nil) = %+v", got)
	}
}
