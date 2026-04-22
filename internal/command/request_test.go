package command

import (
	"strings"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func TestParseRequest_HelpHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--help"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_NoArgsHandledAsHelp(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest(nil, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_ConfigHelpHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "--help"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_NotifyHelpHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"notify", "--help"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_UpdateHelpHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"update", "--help"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_DiagnosticsHelpHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"diagnostics", "--help"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_RollbackHelpHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"rollback", "--help"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_RestoreHelpHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"restore", "--help"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_HelpFullHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"--help-full"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_ConfigHelpFullHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "--help-full"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_NotifyHelpFullHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"notify", "--help-full"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_UpdateHelpFullHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"update", "--help-full"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_DiagnosticsHelpFullHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"diagnostics", "--help-full"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_RollbackHelpFullHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"rollback", "--help-full"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_RestoreHelpFullHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"restore", "--help-full"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || result.Output == "" || result.Request != nil {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestUsageTextTemplatesAreFullyResolved(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	rt := workflow.DefaultRuntime()
	for name, text := range map[string]string{
		"usage":         UsageText(meta, rt),
		"full":          FullUsageText(meta, rt),
		"config":        ConfigUsageText(meta, rt),
		"config-full":   FullConfigUsageText(meta, rt),
		"diagnostics":   DiagnosticsUsageText(meta, rt),
		"diag-full":     FullDiagnosticsUsageText(meta, rt),
		"notify":        NotifyUsageText(meta, rt),
		"notify-full":   FullNotifyUsageText(meta, rt),
		"rollback":      RollbackUsageText(meta, rt),
		"rollback-full": FullRollbackUsageText(meta, rt),
		"restore":       RestoreUsageText(meta, rt),
		"restore-full":  FullRestoreUsageText(meta, rt),
		"update":        UpdateUsageText(meta, rt),
		"update-full":   FullUpdateUsageText(meta, rt),
	} {
		if strings.Contains(text, "{{") || strings.Contains(text, "}}") {
			t.Fatalf("%s usage contains unresolved template marker: %q", name, text)
		}
	}
}

func TestParseRequest_ConfigValidate(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "validate", "--target", "onsite-usb", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.ConfigCommand != "validate" || result.Request.Source != "homes" || result.Request.Target() != "onsite-usb" {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_ConfigValidateVerbose(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "validate", "--verbose", "--target", "onsite-usb", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.ConfigCommand != "validate" || result.Request.Source != "homes" || result.Request.Target() != "onsite-usb" || !result.Request.Verbose {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_ConfigExplainExplicitTarget(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config", "explain", "--target", "offsite-storj", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.ConfigCommand != "explain" || result.Request.Target() != "offsite-storj" {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_Diagnostics(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"diagnostics", "--target", "offsite-storj", "--json-summary", "--config-dir", "/cfg", "--secrets-dir", "/sec", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.DiagnosticsCommand != "diagnostics" ||
		result.Request.Target() != "offsite-storj" ||
		result.Request.Source != "homes" ||
		result.Request.ConfigDir != "/cfg" ||
		result.Request.SecretsDir != "/sec" ||
		!result.Request.JSONSummary {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_TargetFlag(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"health", "verify", "--target", "offsite-storj", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.Target() != "offsite-storj" {
		t.Fatalf("Target() = %q", result.Request.Target())
	}
}

func TestParseRequest_NotifyTest(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"notify", "test", "--target", "offsite-storj", "--provider", "ntfy", "--severity", "critical", "--summary", "Smoke", "--message", "Synthetic", "--json-summary", "--dry-run", "homes"}, meta, workflow.DefaultRuntime())
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

func TestParseRequest_NotifyTestAllowsOmittedEvent(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"notify", "test", "--target", "offsite-storj", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.NotifyEvent != "" {
		t.Fatalf("NotifyEvent = %q, want empty", result.Request.NotifyEvent)
	}
}

func TestParseRequest_NotifyTestUpdate(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"notify", "test", "update", "--provider", "ntfy", "--event", "update_install_failed", "--dry-run", "--config-dir", "/cfg"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.NotifyCommand != "test" ||
		result.Request.NotifyScope != "update" ||
		result.Request.NotifyProvider != "ntfy" ||
		result.Request.NotifyEvent != "update_install_failed" ||
		result.Request.ConfigDir != "/cfg" ||
		result.Request.Source != "" ||
		result.Request.Target() != "" ||
		!result.Request.DryRun {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_NotifyTestUpdateRejectsTarget(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"notify", "test", "update", "--target", "onsite-usb"}, meta, workflow.DefaultRuntime())
	if err == nil || !strings.Contains(err.Error(), "does not use --target") {
		t.Fatalf("ParseRequest() err = %v", err)
	}
}

func TestParseRequest_RestorePlan(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"restore", "plan", "--target", "offsite-storj", "--config-dir", "/cfg", "--secrets-dir", "/sec", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.RestoreCommand != "plan" ||
		result.Request.Target() != "offsite-storj" ||
		result.Request.ConfigDir != "/cfg" ||
		result.Request.SecretsDir != "/sec" ||
		result.Request.Source != "homes" {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_RestorePrepare(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"restore", "prepare", "--target", "offsite-storj", "--workspace", "/restore/homes", "--config-dir", "/cfg", "--secrets-dir", "/sec", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.RestoreCommand != "prepare" ||
		result.Request.Target() != "offsite-storj" ||
		result.Request.RestoreWorkspace != "/restore/homes" ||
		result.Request.ConfigDir != "/cfg" ||
		result.Request.SecretsDir != "/sec" ||
		result.Request.Source != "homes" {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_RestorePlanRejectsRuntimeFlags(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"restore", "plan", "--target", "onsite-usb", "--dry-run", "homes"}, meta, workflow.DefaultRuntime())
	if err == nil || !strings.Contains(err.Error(), "unknown option --dry-run") {
		t.Fatalf("ParseRequest() err = %v", err)
	}
}

func TestParseRequest_RestoreUnknownCommandFails(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"restore", "run", "--target", "onsite-usb", "homes"}, meta, workflow.DefaultRuntime())
	if err == nil || !strings.Contains(err.Error(), "unknown restore command") {
		t.Fatalf("ParseRequest() err = %v", err)
	}
}

func TestParseFailureContext_HealthUsesSharedFlags(t *testing.T) {
	ctx := ParseFailureContext([]string{
		"health", "verify",
		"--target", "offsite-storj",
		"--json-summary",
		"--verbose",
		"--config-dir", "/cfg",
		"--secrets-dir", "/sec",
		"homes",
	})
	if ctx.Kind != FailureRequestHealth || !ctx.JSONSummary {
		t.Fatalf("ctx = %+v", ctx)
	}
	if ctx.Request.HealthCommand != "verify" ||
		ctx.Request.RequestedTarget != "offsite-storj" ||
		ctx.Request.ConfigDir != "/cfg" ||
		ctx.Request.SecretsDir != "/sec" ||
		ctx.Request.Source != "homes" ||
		!ctx.Request.JSONSummary ||
		!ctx.Request.Verbose {
		t.Fatalf("request = %+v", ctx.Request)
	}
}

func TestParseFailureContext_NotifyUsesSharedFlags(t *testing.T) {
	ctx := ParseFailureContext([]string{
		"notify", "test",
		"--target", "offsite-storj",
		"--provider", "ntfy",
		"--severity", "critical",
		"--summary", "Synthetic summary",
		"--message", "Synthetic message",
		"--event", "notification_test",
		"--dry-run",
		"--json-summary",
		"--config-dir", "/cfg",
		"--secrets-dir", "/sec",
		"homes",
	})
	if ctx.Kind != FailureRequestNotify || !ctx.JSONSummary {
		t.Fatalf("ctx = %+v", ctx)
	}
	if ctx.Request.NotifyCommand != "test" ||
		ctx.Request.RequestedTarget != "offsite-storj" ||
		ctx.Request.NotifyProvider != "ntfy" ||
		ctx.Request.NotifySeverity != "critical" ||
		ctx.Request.NotifySummary != "Synthetic summary" ||
		ctx.Request.NotifyMessage != "Synthetic message" ||
		ctx.Request.NotifyEvent != "notification_test" ||
		ctx.Request.ConfigDir != "/cfg" ||
		ctx.Request.SecretsDir != "/sec" ||
		ctx.Request.Source != "homes" ||
		!ctx.Request.DryRun ||
		!ctx.Request.JSONSummary {
		t.Fatalf("request = %+v", ctx.Request)
	}
}

func TestParseRequest_HealthStatus(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"health", "status", "--target", "onsite-usb", "--json-summary", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.HealthCommand != "status" || !result.Request.JSONSummary {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_Update(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"update", "--check-only", "--force", "--keep", "3", "--version", "v4.1.8", "--attestations", "required", "--yes", "--config-dir", "/cfg"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.UpdateCommand != "update" ||
		!result.Request.UpdateCheckOnly ||
		!result.Request.UpdateForce ||
		!result.Request.UpdateYes ||
		result.Request.UpdateKeep != 3 ||
		result.Request.UpdateVersion != "v4.1.8" ||
		result.Request.UpdateAttestations != "required" ||
		result.Request.ConfigDir != "/cfg" {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_Rollback(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"rollback", "--check-only", "--version", "v5.1.1", "--yes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.RollbackCommand != "rollback" ||
		!result.Request.RollbackCheckOnly ||
		!result.Request.RollbackYes ||
		result.Request.RollbackVersion != "v5.1.1" {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_RollbackRejectsUnexpectedArgs(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"rollback", "homes"}, meta, workflow.DefaultRuntime())
	if err == nil || !strings.Contains(err.Error(), "unexpected extra arguments: homes") {
		t.Fatalf("ParseRequest() err = %v", err)
	}
}

func TestParseRequest_UpdateRejectsInvalidAttestationMode(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"update", "--attestations", "maybe"}, meta, workflow.DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*workflow.RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
	}
	if got := err.Error(); got != "--attestations must be one of: off, auto, required" {
		t.Fatalf("error = %q", got)
	}
}

func TestParseRequest_UpdateRejectsUnexpectedArgs(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"update", "homes"}, meta, workflow.DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*workflow.RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
	}
	if got := err.Error(); got != "unexpected extra arguments: homes" {
		t.Fatalf("error = %q", got)
	}
}

func TestParseRequest_HealthUnknownCommandFails(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"health", "nope", "homes"}, meta, workflow.DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*workflow.RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
	}
}

func TestParseRequest_RejectsOldTopLevelOperationFlags(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"--target", "onsite-usb", "homes"}, meta, workflow.DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*workflow.RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
	}
	if got := err.Error(); got != "unknown top-level option --target; use a command such as backup, prune, cleanup-storage, or fix-perms" {
		t.Fatalf("error = %q", got)
	}
}

func TestParseRequest_BackupCommand(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"backup", "--target", "onsite-usb", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.DoBackup || result.Request.DoPrune || result.Request.FixPerms || result.Request.DoCleanupStore || result.Request.FixPermsOnly {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_CleanupStorageCommand(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"cleanup-storage", "--target", "onsite-usb", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.DoBackup || result.Request.DoPrune || !result.Request.DoCleanupStore {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_RuntimeCommandsRejectOldOperationFlags(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	for _, args := range [][]string{
		{"backup", "--target", "onsite-usb", "--prune", "homes"},
		{"prune", "--target", "onsite-usb", "--backup", "homes"},
		{"cleanup-storage", "--target", "onsite-usb", "--fix-perms", "homes"},
	} {
		_, err := ParseRequest(args, meta, workflow.DefaultRuntime())
		if err == nil || !strings.Contains(err.Error(), "unknown option") {
			t.Fatalf("args %v err = %v", args, err)
		}
	}
}

func TestParseRequest_PruneForce(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"prune", "--target", "onsite-usb", "--force", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.DoPrune || !result.Request.ForcePrune {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_Verbose(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"backup", "--target", "onsite-usb", "--verbose", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.Verbose {
		t.Fatal("expected Verbose true")
	}
}

func TestParseRequest_JSONSummary(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"backup", "--target", "onsite-usb", "--json-summary", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.JSONSummary {
		t.Fatal("expected JSONSummary true")
	}
}

func TestParseRequest_FixPermsOnly(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"fix-perms", "--target", "onsite-usb", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Request.FixPermsOnly || result.Request.DoBackup || result.Request.DoPrune {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_InvalidCombo(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"fix-perms", "--target", "offsite-storj", "homes"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if result.Request.Target() != "offsite-storj" || !result.Request.FixPerms {
		t.Fatalf("result.Request = %+v", result.Request)
	}
}

func TestParseRequest_InvalidLabel(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"backup", "--target", "onsite-usb", "../etc"}, meta, workflow.DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRequest_ExtraPositionalArgsFail(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"backup", "--target", "onsite-usb", "homes", "extra"}, meta, workflow.DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*workflow.RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
	}
	if got := err.Error(); got != "unexpected extra arguments: extra" {
		t.Fatalf("error = %q", got)
	}
}

func TestParseRequest_VersionHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.2.3", "now", t.TempDir())
	result, err := ParseRequest([]string{"--version"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || !strings.Contains(result.Output, "duplicacy-backup 1.2.3") {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_VersionShortHandled(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.2.3", "now", t.TempDir())
	result, err := ParseRequest([]string{"-v"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || !strings.Contains(result.Output, "duplicacy-backup 1.2.3") {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_ConfigNoActionShowsUsage(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"config"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || !strings.Contains(result.Output, "config <validate|explain|paths>") {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_NotifyNoActionShowsUsage(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	result, err := ParseRequest([]string{"notify"}, meta, workflow.DefaultRuntime())
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if !result.Handled || !strings.Contains(result.Output, "notify <test>") {
		t.Fatalf("unexpected parse result: %+v", result)
	}
}

func TestParseRequest_NotifyUnknownCommandFails(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"notify", "unknown", "homes"}, meta, workflow.DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*workflow.RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
	}
}

func TestParseRequest_ConfigUnknownCommandFails(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"config", "unknown", "homes"}, meta, workflow.DefaultRuntime())
	if err == nil {
		t.Fatal("expected error")
	}
	reqErr, ok := err.(*workflow.RequestError)
	if !ok || !reqErr.ShowUsage {
		t.Fatalf("error = %#v", err)
	}
}

func TestParseRequest_OptionValueErrors(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	cases := [][]string{
		{"backup", "--config-dir"},
		{"backup", "--secrets-dir"},
		{"config", "validate", "--config-dir"},
		{"config", "validate", "--secrets-dir"},
		{"notify", "test", "--provider"},
		{"notify", "test", "--severity"},
		{"notify", "test", "--summary"},
		{"notify", "test", "--message"},
	}
	for _, args := range cases {
		if _, err := ParseRequest(args, meta, workflow.DefaultRuntime()); err == nil {
			t.Fatalf("expected error for args %v", args)
		}
	}
}

func TestParseRequest_UnknownOptionsFail(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	cases := [][]string{
		{"backup", "--target", "onsite-usb", "--mystery", "homes"},
		{"config", "validate", "--target", "onsite-usb", "--mystery", "homes"},
		{"notify", "test", "--target", "onsite-usb", "--mystery", "homes"},
	}
	for _, args := range cases {
		_, err := ParseRequest(args, meta, workflow.DefaultRuntime())
		if err == nil || !strings.Contains(err.Error(), "unknown option") {
			t.Fatalf("args %v err = %v", args, err)
		}
	}
}

func TestParseRequest_ConfigExtraArgsFail(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	_, err := ParseRequest([]string{"config", "validate", "--target", "onsite-usb", "homes", "extra"}, meta, workflow.DefaultRuntime())
	if err == nil || !strings.Contains(err.Error(), "unexpected extra arguments") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseRequest_TargetRequiredForRuntimeConfigAndHealth(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	for _, args := range [][]string{
		{"backup", "homes"},
		{"config", "validate", "homes"},
		{"diagnostics", "homes"},
		{"health", "status", "homes"},
		{"notify", "test", "homes"},
	} {
		if _, err := ParseRequest(args, meta, workflow.DefaultRuntime()); err == nil || !strings.Contains(err.Error(), "--target is required") {
			t.Fatalf("args %v err = %v", args, err)
		}
	}
}

func TestParseRequest_NotifyRejectsInvalidProviderAndSeverity(t *testing.T) {
	meta := workflow.DefaultMetadata("duplicacy-backup", "1.0.0", "now", t.TempDir())
	if _, err := ParseRequest([]string{"notify", "test", "--target", "onsite-usb", "--provider", "email", "homes"}, meta, workflow.DefaultRuntime()); err == nil || !strings.Contains(err.Error(), "unsupported notify provider") {
		t.Fatalf("provider err = %v", err)
	}
	if _, err := ParseRequest([]string{"notify", "test", "--target", "onsite-usb", "--severity", "low", "homes"}, meta, workflow.DefaultRuntime()); err == nil || !strings.Contains(err.Error(), "unsupported notify severity") {
		t.Fatalf("severity err = %v", err)
	}
}

func TestVersionText(t *testing.T) {
	text := VersionText(workflow.DefaultMetadata("duplicacy-backup", "2.1.3", "now", t.TempDir()))
	if !strings.Contains(text, "duplicacy-backup 2.1.3 (built now)") {
		t.Fatalf("VersionText() = %q", text)
	}
}
