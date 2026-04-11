package workflow

import (
	"strings"
	"testing"
)

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

func TestParseRequest_NoArgsHandledAsHelp(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest(nil, meta, DefaultRuntime())
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

func TestParseRequest_ConfigHelpHandled(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "--help"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled {
		t.Fatal("expected handled result")
	}
	if result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_HelpFullHandled(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--help-full"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_ConfigHelpFullHandled(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "--help-full"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_ConfigValidate(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "validate", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.ConfigCommand != "validate" {
		t.Fatalf("ConfigCommand = %q", result.Request.ConfigCommand)
	}
	if result.Request.Source != "homes" {
		t.Fatalf("Source = %q", result.Request.Source)
	}
}

func TestParseRequest_ConfigExplainRemote(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "explain", "--remote", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.ConfigCommand != "explain" {
		t.Fatalf("ConfigCommand = %q", result.Request.ConfigCommand)
	}
	if !result.Request.RemoteMode {
		t.Fatal("expected RemoteMode true")
	}
	if result.Request.Target() != "remote" {
		t.Fatalf("Target() = %q", result.Request.Target())
	}
}

func TestParseRequest_TargetFlag(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"health", "verify", "--target", "offsite", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.Target() != "offsite" {
		t.Fatalf("Target() = %q", result.Request.Target())
	}
	if result.Request.RemoteMode {
		t.Fatal("expected RemoteMode false for non-remote target")
	}
}

func TestParseRequest_HealthStatus(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"health", "status", "--json-summary", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.HealthCommand != "status" {
		t.Fatalf("HealthCommand = %q", result.Request.HealthCommand)
	}
	if !result.Request.JSONSummary {
		t.Fatal("expected JSONSummary true")
	}
}

func TestParseRequest_HealthUnknownCommandFails(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"health", "nope", "homes"}, meta, DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
	}
}

func TestParseRequest_DefaultBackupMode(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.DoBackup {
		t.Fatal("expected DoBackup true")
	}
	if result.Request.DefaultNotice == "" {
		t.Fatal("expected default notice")
	}
}

func TestParseRequest_CombinedOperationsIgnoreFlagOrder(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--prune", "--backup", "--fix-perms", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.DoBackup {
		t.Fatal("expected DoBackup true")
	}
	if !result.Request.DoPrune {
		t.Fatal("expected DoPrune true")
	}
	if result.Request.DoCleanupStore {
		t.Fatal("expected DoCleanupStore false")
	}
	if !result.Request.FixPerms {
		t.Fatal("expected FixPerms true")
	}
	if result.Request.FixPermsOnly {
		t.Fatal("expected FixPermsOnly false")
	}
}

func TestParseRequest_CleanupStorageStandalone(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--cleanup-storage", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.DoBackup {
		t.Fatal("expected DoBackup false")
	}
	if result.Request.DoPrune {
		t.Fatal("expected DoPrune false")
	}
	if !result.Request.DoCleanupStore {
		t.Fatal("expected DoCleanupStore true")
	}
}

func TestParseRequest_BackupAndCleanupStorage(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--backup", "--cleanup-storage", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.DoBackup {
		t.Fatal("expected DoBackup true")
	}
	if result.Request.DoPrune {
		t.Fatal("expected DoPrune false")
	}
	if !result.Request.DoCleanupStore {
		t.Fatal("expected DoCleanupStore true")
	}
}

func TestParseRequest_ForcePruneWithoutPruneFails(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"--backup", "--force-prune", "homes"}, meta, DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "--force-prune requires --prune" {
		t.Fatalf("error = %q", err)
	}
}

func TestParseRequest_Verbose(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--verbose", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.Verbose {
		t.Fatal("expected Verbose true")
	}
}

func TestParseRequest_JSONSummary(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--json-summary", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.JSONSummary {
		t.Fatal("expected JSONSummary true")
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

func TestParseRequest_ExtraPositionalArgsFail(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"homes", "extra"}, meta, DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*RequestError)
	if !ok {
		t.Fatalf("error type = %T, want *RequestError", err)
	}
	if !reqErr.ShowUsage {
		t.Fatal("expected ShowUsage true")
	}
	if got := err.Error(); got != "unexpected extra arguments: extra" {
		t.Fatalf("error = %q", got)
	}
}

func TestParseRequest_VersionHandled(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.2.3", "now", t.TempDir())
	result, err := ParseRequest([]string{"--version"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || !strings.Contains(result.Output, "duplicacy-backup 1.2.3") {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_VersionShortHandled(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.2.3", "now", t.TempDir())
	result, err := ParseRequest([]string{"-v"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || !strings.Contains(result.Output, "duplicacy-backup 1.2.3") {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_ConfigNoActionShowsUsage(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || !strings.Contains(result.Output, "config <validate|explain|paths>") {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_ConfigUnknownCommandFails(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"config", "unknown", "homes"}, meta, DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
	}
}

func TestParseRequest_OptionValueErrors(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	cases := [][]string{
		{"--config-dir"},
		{"--secrets-dir"},
		{"config", "validate", "--config-dir"},
		{"config", "validate", "--secrets-dir"},
	}

	for _, args := range cases {
		_, err := ParseRequest(args, meta, DefaultRuntime())
		if err == nil {
			t.Fatalf("expected error for args %v", args)
		}
	}
}

func TestParseRequest_UnknownOptionsFail(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	cases := [][]string{
		{"--mystery", "homes"},
		{"config", "validate", "--mystery", "homes"},
	}

	for _, args := range cases {
		_, err := ParseRequest(args, meta, DefaultRuntime())
		if err == nil || !strings.Contains(err.Error(), "unknown option") {
			t.Fatalf("args %v err = %v", args, err)
		}
	}
}

func TestParseRequest_ConfigExtraArgsFail(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"config", "validate", "homes", "extra"}, meta, DefaultRuntime())
	if err == nil || !strings.Contains(err.Error(), "unexpected extra arguments") {
		t.Fatalf("err = %v", err)
	}
}
