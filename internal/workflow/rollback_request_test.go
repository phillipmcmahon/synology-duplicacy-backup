package workflow

import "testing"

func TestNewRollbackRequestProjectsOnlyRollbackIntent(t *testing.T) {
	req := &Request{
		RollbackCommand:   "rollback",
		RollbackVersion:   "v7.1.0",
		RollbackCheckOnly: true,
		RollbackYes:       true,
		UpdateCommand:     "update",
		UpdateForce:       true,
		RestoreCommand:    "run",
		NotifyProvider:    "ntfy",
		DoBackup:          true,
		Source:            "homes",
	}

	got := NewRollbackRequest(req)
	if got.Command != "rollback" || got.Version != "v7.1.0" {
		t.Fatalf("rollback identity projection failed: %#v", got)
	}
	if !got.CheckOnly || !got.Yes {
		t.Fatalf("rollback boolean projection failed: %#v", got)
	}
}

func TestNewRollbackRequestFromNilIsZeroValue(t *testing.T) {
	got := NewRollbackRequest(nil)
	if got.Command != "" || got.Version != "" || got.CheckOnly || got.Yes {
		t.Fatalf("NewRollbackRequest(nil) = %#v", got)
	}
}
