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
	if !result.Handled || result.Output == "" {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_NoArgsHandledAsHelp(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest(nil, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_ConfigHelpHandled(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "--help"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_NotifyHelpHandled(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"notify", "--help"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
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

func TestParseRequest_NotifyHelpFullHandled(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"notify", "--help-full"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_ConfigValidate(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "validate", "--target", "onsite-usb", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.ConfigCommand != "validate" || result.Request.Source != "homes" || result.Request.Target() != "onsite-usb" {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_ConfigExplainExplicitTarget(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "explain", "--target", "offsite-storj", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.ConfigCommand != "explain" || result.Request.Target() != "offsite-storj" {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_TargetFlag(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"health", "verify", "--target", "offsite-storj", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.Target() != "offsite-storj" {
		t.Fatalf("Target() = %q", result.Request.Target())
	}
}

func TestParseRequest_NotifyTest(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"notify", "test", "--target", "offsite-storj", "--provider", "ntfy", "--severity", "critical", "--summary", "Smoke", "--message", "Synthetic", "--json-summary", "--dry-run", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.NotifyCommand != "test" ||
		result.Request.Target() != "offsite-storj" ||
		result.Request.NotifyProvider != "ntfy" ||
		result.Request.NotifySeverity != "critical" ||
		result.Request.NotifySummary != "Smoke" ||
		result.Request.NotifyMessage != "Synthetic" ||
		!result.Request.JSONSummary ||
		!result.Request.DryRun {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_HealthStatus(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"health", "status", "--target", "onsite-usb", "--json-summary", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.HealthCommand != "status" || !result.Request.JSONSummary {
		t.Fatalf("result.Request = %+v", result.Request)
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

func TestParseRequest_RequiresExplicitOperation(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"--target", "onsite-usb", "homes"}, meta, DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
	}
	if got := err.Error(); got != "at least one operation is required: specify --backup, --prune, --cleanup-storage, or --fix-perms" {
		t.Fatalf("error = %q", got)
	}
}

func TestParseRequest_CombinedOperationsIgnoreFlagOrder(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--target", "onsite-usb", "--prune", "--backup", "--fix-perms", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.DoBackup || !result.Request.DoPrune || !result.Request.FixPerms || result.Request.DoCleanupStore || result.Request.FixPermsOnly {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_CleanupStorageStandalone(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--target", "onsite-usb", "--cleanup-storage", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.DoBackup || result.Request.DoPrune || !result.Request.DoCleanupStore {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_BackupAndCleanupStorage(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--target", "onsite-usb", "--backup", "--cleanup-storage", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.DoBackup || result.Request.DoPrune || !result.Request.DoCleanupStore {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_ForcePruneWithoutPruneFails(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"--target", "onsite-usb", "--backup", "--force-prune", "homes"}, meta, DefaultRuntime())
	if err == nil || err.Error() != "--force-prune requires --prune" {
		t.Fatalf("err = %v", err)
	}
}

func TestParseRequest_Verbose(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--target", "onsite-usb", "--backup", "--verbose", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.Verbose {
		t.Fatal("expected Verbose true")
	}
}

func TestParseRequest_JSONSummary(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--target", "onsite-usb", "--backup", "--json-summary", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.JSONSummary {
		t.Fatal("expected JSONSummary true")
	}
}

func TestParseRequest_FixPermsOnly(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--target", "onsite-usb", "--fix-perms", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.FixPermsOnly || result.Request.DoBackup || result.Request.DoPrune {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_InvalidCombo(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--fix-perms", "--target", "offsite-storj", "homes"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.Target() != "offsite-storj" || !result.Request.FixPerms {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_InvalidLabel(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"--target", "onsite-usb", "../etc"}, meta, DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRequest_ExtraPositionalArgsFail(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"--target", "onsite-usb", "homes", "extra"}, meta, DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
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

func TestParseRequest_NotifyNoActionShowsUsage(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"notify"}, meta, DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || !strings.Contains(result.Output, "notify <test>") {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_NotifyUnknownCommandFails(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"notify", "unknown", "homes"}, meta, DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
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
		{"notify", "test", "--provider"},
		{"notify", "test", "--severity"},
		{"notify", "test", "--summary"},
		{"notify", "test", "--message"},
	}
	for _, args := range cases {
		if _, err := ParseRequest(args, meta, DefaultRuntime()); err == nil {
			t.Fatalf("expected error for args %v", args)
		}
	}
}

func TestParseRequest_UnknownOptionsFail(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	cases := [][]string{
		{"--target", "onsite-usb", "--mystery", "homes"},
		{"config", "validate", "--target", "onsite-usb", "--mystery", "homes"},
		{"notify", "test", "--target", "onsite-usb", "--mystery", "homes"},
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
	_, err := ParseRequest([]string{"config", "validate", "--target", "onsite-usb", "homes", "extra"}, meta, DefaultRuntime())
	if err == nil || !strings.Contains(err.Error(), "unexpected extra arguments") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseRequest_TargetRequiredForRuntimeConfigAndHealth(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	for _, args := range [][]string{
		{"--backup", "homes"},
		{"config", "validate", "homes"},
		{"health", "status", "homes"},
		{"notify", "test", "homes"},
	} {
		if _, err := ParseRequest(args, meta, DefaultRuntime()); err == nil || !strings.Contains(err.Error(), "--target is required") {
			t.Fatalf("args %v err = %v", args, err)
		}
	}
}

func TestParseRequest_NotifyRejectsInvalidProviderAndSeverity(t *testing.T) {
	meta := DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	if _, err := ParseRequest([]string{"notify", "test", "--target", "onsite-usb", "--provider", "email", "homes"}, meta, DefaultRuntime()); err == nil || !strings.Contains(err.Error(), "unsupported notify provider") {
		t.Fatalf("provider err = %v", err)
	}
	if _, err := ParseRequest([]string{"notify", "test", "--target", "onsite-usb", "--severity", "low", "homes"}, meta, DefaultRuntime()); err == nil || !strings.Contains(err.Error(), "unsupported notify severity") {
		t.Fatalf("severity err = %v", err)
	}
}
