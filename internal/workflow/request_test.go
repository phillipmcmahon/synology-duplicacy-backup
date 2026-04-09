package workflow

import "testing"

func TestParseRequest_HelpHandled(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--help"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled {
		t.Fatal("expected handled result")
	}
	if result.Output == "" {
		t.Fatal("expected usage output")
	}
}

func TestParseRequest_DefaultBackupMode(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.Mode != "backup" {
		t.Fatalf("Mode = %q, want backup", result.Request.Mode)
	}
	if !result.Request.DoBackup {
		t.Fatal("expected DoBackup true")
	}
	if result.Request.DefaultNotice == "" {
		t.Fatal("expected default notice")
	}
}

func TestParseRequest_FixPermsOnly(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--fix-perms", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.FixPermsOnly {
		t.Fatal("expected FixPermsOnly true")
	}
	if result.Request.DoBackup || result.Request.DoPrune {
		t.Fatal("expected no backup or prune modes")
	}
}

func TestParseRequest_InvalidCombo(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"--fix-perms", "--remote", "homes"}, meta, DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRequest_InvalidLabel(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"../etc"}, meta, DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
}
