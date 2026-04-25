package workflow

import "testing"

func TestNewUpdateRequestProjectsOnlyUpdateIntent(t *testing.T) {
	req := &Request{
		UpdateCommand:      "update",
		ConfigDir:          "/etc/duplicacy-backup",
		UpdateVersion:      "v7.1.1",
		UpdateKeep:         3,
		UpdateAttestations: "required",
		UpdateCheckOnly:    true,
		UpdateYes:          true,
		UpdateForce:        true,
		RestoreCommand:     "run",
		RestoreRevision:    2403,
		NotifyProvider:     "ntfy",
		DoBackup:           true,
		Source:             "homes",
	}

	got := NewUpdateRequest(req)
	if got.Command != "update" || got.ConfigDir != "/etc/duplicacy-backup" {
		t.Fatalf("update identity projection failed: %#v", got)
	}
	if got.Version != "v7.1.1" || got.Keep != 3 || got.Attestations != "required" {
		t.Fatalf("update option projection failed: %#v", got)
	}
	if !got.CheckOnly || !got.Yes || !got.Force {
		t.Fatalf("update boolean projection failed: %#v", got)
	}
}

func TestNewUpdateRequestFromNilIsZeroValue(t *testing.T) {
	got := NewUpdateRequest(nil)
	if got.Command != "" || got.ConfigDir != "" || got.Version != "" || got.Keep != 0 || got.Attestations != "" || got.CheckOnly || got.Yes || got.Force {
		t.Fatalf("NewUpdateRequest(nil) = %#v", got)
	}
}
