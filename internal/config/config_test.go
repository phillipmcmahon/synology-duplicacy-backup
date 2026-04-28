package config

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return p
}

func currentUserGroup(t *testing.T) (string, string) {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() error = %v", err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Fatalf("user.LookupGroupId() error = %v", err)
	}
	if u.Username != "root" && g.Name != "root" {
		return u.Username, g.Name
	}

	for _, name := range []string{"nobody", "daemon"} {
		if _, err := user.Lookup(name); err == nil {
			u.Username = name
			break
		}
	}
	for _, name := range []string{"nogroup", "nobody", "daemon", "staff", "users"} {
		if _, err := user.LookupGroup(name); err == nil && name != "root" {
			g.Name = name
			break
		}
	}
	if u.Username == "root" || g.Name == "root" {
		t.Skip("no non-root owner/group available on this system")
	}
	return u.Username, g.Name
}

func loadValues(t *testing.T, body, target string) map[string]string {
	t.Helper()
	p := writeTempConfig(t, body)
	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	values, err := raw.ResolveValues(target, p)
	if err != nil {
		t.Fatalf("ResolveValues() error = %v", err)
	}
	return values
}

func TestParseFile_ValidConfig(t *testing.T) {
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[common]
filter = "-e \\.DS_Store"

[targets.onsite-usb]
location = "local"
storage = "/volume1/backups/homes"
threads = 4
allow_local_accounts = true
local_owner = "admin"
local_group = "users"
`)

	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	values, err := raw.ResolveValues("onsite-usb", p)
	if err != nil {
		t.Fatalf("ResolveValues() error = %v", err)
	}

	expect := map[string]string{
		"STORAGE":     "/volume1/backups/homes",
		"FILTER":      `-e \.DS_Store`,
		"THREADS":     "4",
		"LOCAL_OWNER": "admin",
		"LOCAL_GROUP": "users",
	}
	for k, want := range expect {
		if got := values[k]; got != want {
			t.Errorf("values[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestParseFile_TargetConfigAllowsMissingSourcePathForRestoreOnlyUse(t *testing.T) {
	p := writeTempConfig(t, `
label = "homes"

[targets.offsite-storj]
location = "remote"
storage = "s3://gateway.example.invalid/bucket/homes"
`)

	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	values, err := raw.ResolveValues("offsite-storj", p)
	if err != nil {
		t.Fatalf("ResolveValues() error = %v", err)
	}
	if _, ok := values["SOURCE_PATH"]; ok {
		t.Fatalf("SOURCE_PATH should be absent when source_path is omitted: %#v", values)
	}
	if values["STORAGE"] != "s3://gateway.example.invalid/bucket/homes" {
		t.Fatalf("STORAGE = %q", values["STORAGE"])
	}
}

func TestParseFile_RestoreWorkspaceSettings(t *testing.T) {
	values := loadValues(t, `
label = "homes"
source_path = "/volume1/homes"

[restore]
workspace_root = "/volume1/recovery"
workspace_template = "{label}-{revision}-{target}-{run_timestamp}"

[targets.onsite-usb]
location = "local"
storage = "/volume1/backups/homes"
threads = 4
`, "onsite-usb")

	if values["RESTORE_WORKSPACE_ROOT"] != "/volume1/recovery" {
		t.Fatalf("RESTORE_WORKSPACE_ROOT = %q", values["RESTORE_WORKSPACE_ROOT"])
	}
	if values["RESTORE_WORKSPACE_TEMPLATE"] != "{label}-{revision}-{target}-{run_timestamp}" {
		t.Fatalf("RESTORE_WORKSPACE_TEMPLATE = %q", values["RESTORE_WORKSPACE_TEMPLATE"])
	}
}

func TestParseFile_ResolveHealthDefaultsAndOverrides(t *testing.T) {
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[targets.onsite-usb]
location = "local"
storage = "/volume1/backups/homes"
threads = 4
allow_local_accounts = true
local_owner = "admin"
local_group = "users"

[health]
freshness_warn_hours = 12
freshness_fail_hours = 24

[health.notify]
webhook_url = "https://example.invalid/hook"
notify_on = ["unhealthy"]
send_for = ["status", "verify"]
interactive = true

[health.notify.ntfy]
topic = "alerts"
`)

	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	health := raw.ResolveHealth("onsite-usb")
	if health.FreshnessWarnHours != 12 || health.FreshnessFailHours != 24 {
		t.Fatalf("health = %+v", health)
	}
	if health.Notify.WebhookURL != "https://example.invalid/hook" || !health.Notify.Interactive {
		t.Fatalf("health notify = %+v", health.Notify)
	}
	if health.Notify.Ntfy.URL != "https://ntfy.sh" || health.Notify.Ntfy.Topic != "alerts" {
		t.Fatalf("health notify ntfy = %+v", health.Notify.Ntfy)
	}
	if got := strings.Join(health.Notify.SendFor, ","); got != "status,verify" {
		t.Fatalf("SendFor = %q", got)
	}
}

func TestParseFile_TargetTableOverridesCommon(t *testing.T) {
	values := loadValues(t, `
label = "homes"
source_path = "/volume1/homes"

[common]
threads = 2

[targets.onsite-usb]
location = "local"
storage = "/volume1/backups/homes"
threads = 8
`, "onsite-usb")

	if values["THREADS"] != "8" {
		t.Errorf("expected THREADS=8, got %q", values["THREADS"])
	}
}

func TestParseFile_ResolveValues_ExplicitTargetSelection(t *testing.T) {
	values := loadValues(t, `
label = "homes"
source_path = "/volume1/homes"

[common]
threads = 4

[targets.onsite-usb]
location = "local"
storage = "/volumeUSB1/usbshare/duplicacy/homes"
allow_local_accounts = true
`, "onsite-usb")

	if values["TARGET"] != "onsite-usb" {
		t.Fatalf("TARGET = %q", values["TARGET"])
	}
	if values["LOCATION"] != "local" {
		t.Fatalf("LOCATION = %q", values["LOCATION"])
	}
}

func TestParseFile_ResolveValues_ProductNeutralTargetLayout(t *testing.T) {
	owner, group := currentUserGroup(t)
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[target]
name = "onsite-usb"
location = "local"
allow_local_accounts = true
local_owner = "`+owner+`"
local_group = "`+group+`"

[storage]
storage = "/volumeUSB1/usbshare/duplicacy/homes"

[capture]
filter = "e:^tmp$"
threads = 8

[retention]
keep = ["0:30", "", "7:14"]
log_retention_days = 14
safe_prune_max_delete_percent = 12
safe_prune_max_delete_count = 34
safe_prune_min_total_for_percent = 56

[health]
freshness_warn_hours = 12

[notify]
webhook_url = "https://example.invalid/hook"
notify_on = ["unhealthy"]
send_for = ["verify"]
interactive = true
`)

	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	values, err := raw.ResolveValues("onsite-usb", p)
	if err != nil {
		t.Fatalf("ResolveValues() error = %v", err)
	}

	expect := map[string]string{
		"LABEL":                            "homes",
		"TARGET":                           "onsite-usb",
		"LOCATION":                         "local",
		"SOURCE_PATH":                      "/volume1/homes",
		"STORAGE":                          "/volumeUSB1/usbshare/duplicacy/homes",
		"FILTER":                           "e:^tmp$",
		"THREADS":                          "8",
		"PRUNE":                            "-keep 0:30 -keep 7:14",
		"LOG_RETENTION_DAYS":               "14",
		"SAFE_PRUNE_MAX_DELETE_PERCENT":    "12",
		"SAFE_PRUNE_MAX_DELETE_COUNT":      "34",
		"SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT": "56",
		"ALLOW_LOCAL_ACCOUNTS":             "true",
		"LOCAL_OWNER":                      owner,
		"LOCAL_GROUP":                      group,
	}
	for key, want := range expect {
		if got := values[key]; got != want {
			t.Fatalf("values[%q] = %q, want %q", key, got, want)
		}
	}

	health := raw.ResolveHealth("onsite-usb")
	if health.FreshnessWarnHours != 12 {
		t.Fatalf("FreshnessWarnHours = %d, want 12", health.FreshnessWarnHours)
	}
	if health.Notify.WebhookURL != "https://example.invalid/hook" || !health.Notify.Interactive {
		t.Fatalf("health notify = %+v", health.Notify)
	}
	if health.Notify.Ntfy.URL != "https://ntfy.sh" || health.Notify.Ntfy.Topic != "" {
		t.Fatalf("health notify ntfy = %+v", health.Notify.Ntfy)
	}
	if got := strings.Join(health.Notify.NotifyOn, ","); got != "unhealthy" {
		t.Fatalf("NotifyOn = %q", got)
	}
	if got := strings.Join(health.Notify.SendFor, ","); got != "verify" {
		t.Fatalf("SendFor = %q", got)
	}
}

func TestParseFile_ResolveValues_ProductNeutralTargetLayoutPrefersExplicitPrune(t *testing.T) {
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[target]
name = "offsite-storj"
location = "remote"

[storage]
storage = "s3://bucket/homes"

[retention]
prune = "-keep 1:365 -keep 30:90"
keep = ["0:30", "7:14"]
`)

	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	values, err := raw.ResolveValues("offsite-storj", p)
	if err != nil {
		t.Fatalf("ResolveValues() error = %v", err)
	}
	if values["PRUNE"] != "-keep 1:365 -keep 30:90" {
		t.Fatalf("PRUNE = %q", values["PRUNE"])
	}
	if values["LOCATION"] != "remote" {
		t.Fatalf("LOCATION = %q", values["LOCATION"])
	}
}

func TestParseFile_ResolveValues_ProductNeutralTargetMismatchFails(t *testing.T) {
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[target]
name = "onsite-usb"
location = "local"

[storage]
storage = "/volumeUSB1/usbshare/duplicacy/homes"
`)

	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	_, err = raw.ResolveValues("offsite-storj", p)
	if err == nil || !strings.Contains(err.Error(), `defines target "onsite-usb", expected "offsite-storj"`) {
		t.Fatalf("ResolveValues() err = %v", err)
	}
}

func TestParseFile_ResolveHealth_TargetSpecificOverride(t *testing.T) {
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[health]
freshness_warn_hours = 20
freshness_fail_hours = 40

[notify]
notify_on = ["degraded"]
send_for = ["doctor"]

[targets.offsite-storj]
location = "remote"
storage = "s3://bucket/homes"

[targets.offsite-storj.health]
freshness_warn_hours = 10
verify_warn_after_hours = 72

[targets.offsite-storj.health.notify]
send_for = ["verify"]
interactive = true
`)

	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	health := raw.ResolveHealth("offsite-storj")
	if health.FreshnessWarnHours != 10 || health.FreshnessFailHours != 40 || health.VerifyWarnAfter != 72 {
		t.Fatalf("health = %+v", health)
	}
	if got := strings.Join(health.Notify.NotifyOn, ","); got != "degraded" {
		t.Fatalf("NotifyOn = %q", got)
	}
	if got := strings.Join(health.Notify.SendFor, ","); got != "verify" {
		t.Fatalf("SendFor = %q", got)
	}
	if !health.Notify.Interactive {
		t.Fatalf("health notify = %+v", health.Notify)
	}
	if health.Notify.Ntfy.URL != "https://ntfy.sh" || health.Notify.Ntfy.Topic != "" {
		t.Fatalf("health notify ntfy = %+v", health.Notify.Ntfy)
	}
}

func TestParseFile_ResolveValues_MissingExplicitTargetFails(t *testing.T) {
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[targets.onsite-usb]
location = "local"
storage = "/volumeUSB1/usbshare/duplicacy/homes"
	allow_local_accounts = true
`)
	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	_, err = raw.ResolveValues("", p)
	if err == nil || !strings.Contains(err.Error(), "requires an explicit target selection") {
		t.Fatalf("ResolveValues() err = %v", err)
	}
}

func TestParseFile_ResolveValues_TargetTypeKeyRejected(t *testing.T) {
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[targets.onsite-usb]
type = "duplicacy"
location = "local"
storage = "/volumeUSB1/usbshare/duplicacy/homes"
`)
	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	_, err = raw.ResolveValues("onsite-usb", p)
	if err == nil || !strings.Contains(err.Error(), "remove this key because storage is always delegated to Duplicacy") {
		t.Fatalf("ResolveValues() err = %v", err)
	}
}

func TestParseFile_IgnoresUnrelatedTable(t *testing.T) {
	p := writeTempConfig(t, `
[common]
destination = "/volume1/backups"

[remote]
threads = 2

[local]
threads = 8
`)
	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	_, err = raw.ResolveValues("local", p)
	if err == nil || !strings.Contains(err.Error(), "retired [common]/[local]/[remote] layout") {
		t.Fatalf("ResolveValues() err = %v", err)
	}
}

func TestParseFile_MissingCommonTable(t *testing.T) {
	p := writeTempConfig(t, `
[local]
destination = "/volume1/backups"
`)
	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	_, err = raw.ResolveValues("local", p)
	if err == nil || !strings.Contains(err.Error(), "retired [common]/[local]/[remote] layout") {
		t.Fatalf("ResolveValues() err = %v", err)
	}
}

func TestParseFile_MissingTargetTable(t *testing.T) {
	p := writeTempConfig(t, `
[common]
destination = "/volume1/backups"
`)
	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	_, err = raw.ResolveValues("local", p)
	if err == nil || !strings.Contains(err.Error(), "retired [common]/[local]/[remote] layout") {
		t.Fatalf("ResolveValues() err = %v", err)
	}
}

func TestParseFile_UnknownTableRejected(t *testing.T) {
	p := writeTempConfig(t, `
[common]
destination = "/volume1/backups"

[archive]
threads = 4

[local]
threads = 2
`)
	_, err := ParseFile(p)
	if err == nil {
		t.Fatal("expected error for unknown table")
	}
	if !strings.Contains(err.Error(), "[archive]") {
		t.Errorf("error should mention [archive], got: %v", err)
	}
}

func TestParseFile_UnknownKeyRejected(t *testing.T) {
	p := writeTempConfig(t, `
[common]
destination = "/volume1/backups"
unknown_key = "foo"

[local]
threads = 4
`)
	_, err := ParseFile(p)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown_key") {
		t.Errorf("error should mention unknown_key, got: %v", err)
	}
}

func TestParseFile_LocalOnlyKeysRejectedOutsideLocal(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "common",
			body: `
[common]
destination = "/volume1/backups"
local_owner = "admin"

[local]
threads = 4
`,
		},
		{
			name: "remote",
			body: `
[common]
destination = "s3://bucket"

[remote]
threads = 4
local_group = "users"
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseFile(writeTempConfig(t, tc.body))
			if err == nil {
				t.Fatal("expected parse error")
			}
			if !strings.Contains(err.Error(), "only allowed in [local]") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestParseFile_MalformedTOMLRejected(t *testing.T) {
	_, err := ParseFile(writeTempConfig(t, `
[common
storage = "/volume1/backups/homes"
`))
	if err == nil {
		t.Fatal("expected malformed TOML error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid toml") {
		t.Errorf("error should mention invalid TOML, got: %v", err)
	}
}

func TestParseFile_UppercaseKeysRejected(t *testing.T) {
	_, err := ParseFile(writeTempConfig(t, `
[common]
DESTINATION = "/volume1/backups"

[local]
threads = 4
`))
	if err == nil {
		t.Fatal("expected uppercase key rejection")
	}
	if !strings.Contains(err.Error(), "DESTINATION") {
		t.Errorf("error should mention DESTINATION, got: %v", err)
	}
}

func TestParseFile_ZeroValuesOverrideDefaults(t *testing.T) {
	values := loadValues(t, `
label = "homes"
source_path = "/volume1/homes"

[common]
log_retention_days = 0
safe_prune_max_delete_count = 0
safe_prune_max_delete_percent = 0
safe_prune_min_total_for_percent = 0

[targets.onsite-usb]
location = "local"
storage = "/volume1/backups/homes"
threads = 0
`, "onsite-usb")

	if values["THREADS"] != "0" {
		t.Errorf("THREADS = %q, want 0", values["THREADS"])
	}
	if values["LOG_RETENTION_DAYS"] != "0" {
		t.Errorf("LOG_RETENTION_DAYS = %q, want 0", values["LOG_RETENTION_DAYS"])
	}
}

func TestParseFile_MultilineFilterPreserved(t *testing.T) {
	values := loadValues(t, `
label = "homes"
source_path = "/volume1/homes"

[common]
filter = '''
-exclude tmp
-exclude .DS_Store
'''

[targets.onsite-usb]
location = "local"
storage = "/volume1/backups/homes"
threads = 4
`, "onsite-usb")

	want := "-exclude tmp\n-exclude .DS_Store\n"
	if values["FILTER"] != want {
		t.Errorf("FILTER = %q, want %q", values["FILTER"], want)
	}
}

func TestParseFile_NonexistentFile(t *testing.T) {
	_, err := ParseFile("/nonexistent/config.toml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}

	var configErr *apperrors.ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("expected ConfigError, got %T", err)
	}
}

func TestApply_ValidNumericValues(t *testing.T) {
	cfg := NewDefaults()
	vals := map[string]string{
		"THREADS":                          "8",
		"LOG_RETENTION_DAYS":               "14",
		"SAFE_PRUNE_MAX_DELETE_COUNT":      "50",
		"SAFE_PRUNE_MAX_DELETE_PERCENT":    "20",
		"SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT": "10",
	}
	if err := cfg.Apply(vals); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Threads != 8 || cfg.LogRetentionDays != 14 || cfg.SafePruneMaxDeleteCount != 50 || cfg.SafePruneMaxDeletePercent != 20 || cfg.SafePruneMinTotalForPercent != 10 {
		t.Fatalf("cfg after Apply = %+v", cfg)
	}
}

func TestApply_StringValues(t *testing.T) {
	cfg := NewDefaults()
	vals := map[string]string{
		"FILTER":                     "-e *.tmp",
		"LOCAL_OWNER":                "admin",
		"LOCAL_GROUP":                "staff",
		"PRUNE":                      "-keep 0:365 -keep 30:180",
		"RESTORE_WORKSPACE_ROOT":     "/volume1/recovery",
		"RESTORE_WORKSPACE_TEMPLATE": "{label}-{target}-{revision}",
	}
	if err := cfg.Apply(vals); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Filter != "-e *.tmp" || cfg.LocalOwner != "admin" || cfg.LocalGroup != "staff" || cfg.Prune != "-keep 0:365 -keep 30:180" {
		t.Fatalf("cfg after Apply = %+v", cfg)
	}
	if cfg.RestoreWorkspaceRoot != "/volume1/recovery" || cfg.RestoreWorkspaceTemplate != "{label}-{target}-{revision}" {
		t.Fatalf("restore workspace config after Apply = %+v", cfg)
	}
}

func TestApply_EmptyMapKeepsDefaults(t *testing.T) {
	cfg := NewDefaults()
	if err := cfg.Apply(map[string]string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LocalOwner != "" || cfg.LocalGroup != "" || cfg.LogRetentionDays != DefaultLogRetentionDays {
		t.Fatalf("cfg after Apply = %+v", cfg)
	}
}

func TestApply_InvalidNumericValues(t *testing.T) {
	cases := []struct {
		name string
		key  string
		val  string
		want string
	}{
		{"threads", "THREADS", "ten", "threads"},
		{"log retention", "LOG_RETENTION_DAYS", "-5", "log_retention_days"},
		{"delete count", "SAFE_PRUNE_MAX_DELETE_COUNT", "abc", "safe_prune_max_delete_count"},
		{"delete percent", "SAFE_PRUNE_MAX_DELETE_PERCENT", "50%", "safe_prune_max_delete_percent"},
		{"min total", "SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT", "nope", "safe_prune_min_total_for_percent"},
		{"empty", "THREADS", "", "threads"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := NewDefaults()
			err := cfg.Apply(map[string]string{tc.key: tc.val})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateRequired(t *testing.T) {
	cfg := &Config{SourcePath: "/volume1/homes", Storage: "/vol/homes", Threads: 4, Prune: "-keep 0:30"}
	if err := cfg.ValidateRequired(true, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := (&Config{SourcePath: "/volume1/homes", Threads: 4, Prune: "-keep 0:30"}).ValidateRequired(true, true); err == nil {
		t.Fatal("expected missing storage")
	}
	if err := (&Config{Storage: "/vol/homes", Threads: 4, Prune: "-keep 0:30"}).ValidateRequired(true, true); err == nil {
		t.Fatal("expected missing source_path")
	} else if !strings.Contains(err.Error(), "source_path") {
		t.Fatalf("missing source_path error = %v", err)
	}
	if err := (&Config{SourcePath: "/volume1/homes", Storage: "/vol/homes", Prune: "-keep 0:30"}).ValidateRequired(true, true); err == nil {
		t.Fatal("expected missing threads")
	} else if !strings.Contains(err.Error(), "common.threads or targets.<name>.threads") {
		t.Fatalf("missing threads error = %v", err)
	}
	if err := (&Config{SourcePath: "/volume1/homes", Storage: "/vol/homes", Threads: 4}).ValidateRequired(true, true); err == nil {
		t.Fatal("expected missing prune")
	} else if !strings.Contains(err.Error(), "common.prune or targets.<name>.prune") {
		t.Fatalf("missing prune error = %v", err)
	}
	if err := (&Config{Storage: "/vol/homes"}).ValidateRequired(false, false); err != nil {
		t.Fatalf("restore-style required check should not require source_path: %v", err)
	}
}

func TestValidateThresholds(t *testing.T) {
	if err := NewDefaults().ValidateThresholds(); err != nil {
		t.Fatalf("defaults should be valid: %v", err)
	}

	cases := []Config{
		{SafePruneMaxDeletePercent: 101},
		{SafePruneMaxDeleteCount: -1},
		{SafePruneMinTotalForPercent: -1},
		{LogRetentionDays: -1},
		{Health: HealthConfig{FreshnessWarnHours: MaxHealthThresholdHours + 1}},
		{Health: HealthConfig{FreshnessFailHours: MaxHealthThresholdHours + 1}},
		{Health: HealthConfig{DoctorWarnAfter: MaxHealthThresholdHours + 1}},
		{Health: HealthConfig{VerifyWarnAfter: MaxHealthThresholdHours + 1}},
	}
	for _, cfg := range cases {
		if err := cfg.ValidateThresholds(); err == nil {
			t.Fatalf("expected threshold error for %+v", cfg)
		}
	}
	if err := (&Config{}).ValidateThresholds(); err != nil {
		t.Fatalf("zero values should be valid: %v", err)
	}
}

func TestValidateOwnerGroup(t *testing.T) {
	owner, group := currentUserGroup(t)
	if err := (&Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalOwner: owner, LocalGroup: group}).ValidateOwnerGroup(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"url backend disallowed", Config{Target: "offsite-storj", Location: "remote", Storage: "s3://bucket/homes", AllowLocalAccounts: true, LocalOwner: owner, LocalGroup: group}, "does not support local account ownership"},
		{"missing owner", Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalGroup: group}, "local_owner is mandatory"},
		{"missing group", Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalOwner: owner}, "local_group is mandatory"},
		{"invalid owner", Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalOwner: "admin/bad", LocalGroup: group}, "invalid"},
		{"invalid group", Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalOwner: owner, LocalGroup: "bad group"}, "invalid"},
		{"root owner", Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalOwner: "root", LocalGroup: group}, "must not be 'root'"},
		{"root group", Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalOwner: owner, LocalGroup: "root"}, "must not be 'root'"},
		{"missing user", Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalOwner: "no_such_user_xyz_999", LocalGroup: group}, "does not exist"},
		{"missing group", Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalOwner: owner, LocalGroup: "no_such_group_xyz_999"}, "does not exist"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.ValidateOwnerGroup()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateThreads(t *testing.T) {
	for _, n := range []int{1, 2, 4, 8, 16} {
		if err := (&Config{Threads: n}).ValidateThreads(); err != nil {
			t.Errorf("ValidateThreads(%d) unexpected error: %v", n, err)
		}
	}
	for _, n := range []int{0, -1, 3, 5, 6, 7, 9, 17, 32} {
		if err := (&Config{Threads: n}).ValidateThreads(); err == nil {
			t.Errorf("ValidateThreads(%d) expected error", n)
		}
	}
}

func TestBuildPruneArgs(t *testing.T) {
	cfg := &Config{Prune: "-keep 1:30 -keep 7:7"}
	cfg.BuildPruneArgs()
	if len(cfg.PruneArgs) != 4 {
		t.Fatalf("PruneArgs = %#v", cfg.PruneArgs)
	}
	cfg.Prune = ""
	cfg.BuildPruneArgs()
	if cfg.PruneArgs != nil {
		t.Fatalf("PruneArgs = %#v, want nil", cfg.PruneArgs)
	}
}

func TestValidatePrunePolicy(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "empty", cfg: Config{Prune: ""}},
		{name: "keep only", cfg: Config{Prune: "-keep 0:30 -keep 7:14"}},
		{name: "supported flags", cfg: Config{Prune: "-keep 0:30 -exclusive -exhaustive"}},
		{name: "missing keep value", cfg: Config{Prune: "-keep"}, want: "missing a retention value"},
		{name: "bad keep format", cfg: Config{Prune: "-keep thirty"}, want: "must use <age>:<count> format"},
		{name: "unsupported option", cfg: Config{Prune: "-delete 4"}, want: "unsupported option"},
		{name: "bare value", cfg: Config{Prune: "0:30"}, want: "unexpected bare value"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.ValidatePrunePolicy()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("ValidatePrunePolicy() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidatePrunePolicy() err = %v", err)
			}
		})
	}
}

func TestValidateTargetSemantics(t *testing.T) {
	owner, group := currentUserGroup(t)

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "local disk okay", cfg: Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes"}},
		{name: "remote path okay", cfg: Config{Target: "offsite-usb", Location: "remote", Storage: "/volume1/duplicacy/duplicacy/homes"}},
		{name: "local object storage okay", cfg: Config{Target: "onsite-rustfs", Location: "local", Storage: "s3://rustfs.local/bucket/homes"}},
		{name: "remote object storage okay", cfg: Config{Target: "offsite-storj", Location: "remote", Storage: "s3://gateway.example.invalid/bucket/homes"}},
		{name: "location required", cfg: Config{Target: "onsite-usb", Storage: "/volume2/backups/homes"}, want: "target.location must be set"},
		{name: "owner group require allow local", cfg: Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", LocalOwner: owner, LocalGroup: group}, want: "require allow_local_accounts = true"},
		{name: "owner and group together", cfg: Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalOwner: owner}, want: "must be set together"},
		{name: "url backend disallows local accounts", cfg: Config{Target: "offsite-storj", Location: "remote", Storage: "s3://bucket/homes", AllowLocalAccounts: true}, want: "only supported for path-based Duplicacy storage targets"},
		{name: "local url backend disallows local accounts", cfg: Config{Target: "onsite-rustfs", Location: "local", Storage: "s3://bucket/homes", AllowLocalAccounts: true}, want: "only supported for path-based Duplicacy storage targets"},
		{name: "owner group validated when present", cfg: Config{Target: "onsite-usb", Location: "local", Storage: "/volume2/backups/homes", AllowLocalAccounts: true, LocalOwner: owner, LocalGroup: group}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.ValidateTargetSemantics()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("ValidateTargetSemantics() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateTargetSemantics() err = %v", err)
			}
		})
	}
}

func TestConfigPathStorageAndRootProtectedRepositoryBoundaries(t *testing.T) {
	tests := []struct {
		name          string
		location      string
		storage       string
		pathStorage   bool
		rootProtected bool
	}{
		{name: "local absolute path", location: "local", storage: "/volumeUSB1/usbshare/duplicacy/homes", pathStorage: true, rootProtected: true},
		{name: "remote mounted absolute path", location: "remote", storage: "/volume1/duplicacy/usbshare2/homes", pathStorage: true},
		{name: "local file URL", location: "local", storage: "file:///mnt/nfs/duplicacy/homes", pathStorage: true, rootProtected: true},
		{name: "remote file URL", location: "remote", storage: "file:///mnt/smb/duplicacy/homes", pathStorage: true},
		{name: "s3", storage: "s3://gateway.example.invalid/bucket/homes"},
		{name: "b2", storage: "b2://bucket/homes"},
		{name: "wasabi", storage: "wasabi://bucket/homes"},
		{name: "storj", storage: "storj://bucket/homes"},
		{name: "sftp localhost", storage: "sftp://localhost/duplicacy/homes"},
		{name: "webdav", storage: "webdav://nas.local/duplicacy/homes"},
		{name: "gcd", storage: "gcd://drive-folder/homes"},
		{name: "unknown URL-like scheme", storage: "dummy:///path/to/homes"},
		{name: "empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Location: tt.location, Storage: tt.storage}
			if got := cfg.UsesPathStorage(); got != tt.pathStorage {
				t.Fatalf("UsesPathStorage() = %t, want %t", got, tt.pathStorage)
			}
			if got := cfg.UsesRootProtectedLocalRepository(); got != tt.rootProtected {
				t.Fatalf("UsesRootProtectedLocalRepository() = %t, want %t", got, tt.rootProtected)
			}
		})
	}

	if (*Config)(nil).UsesPathStorage() {
		t.Fatal("UsesPathStorage() on nil config = true, want false")
	}
	if (*Config)(nil).UsesRootProtectedLocalRepository() {
		t.Fatal("UsesRootProtectedLocalRepository() on nil config = true, want false")
	}
}

func TestHealthConfigValidate(t *testing.T) {
	valid := NewDefaults().Health
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() valid defaults error = %v", err)
	}

	invalidNotifyOn := valid
	invalidNotifyOn.Notify.NotifyOn = []string{"warn"}
	if err := invalidNotifyOn.Validate(); err == nil || !strings.Contains(err.Error(), "notify_on") {
		t.Fatalf("Validate() notify_on err = %v", err)
	}

	extendedSendFor := valid
	extendedSendFor.Notify.SendFor = []string{"doctor", "verify", "backup", "prune", "cleanup-storage"}
	if err := extendedSendFor.Validate(); err != nil {
		t.Fatalf("Validate() extended send_for error = %v", err)
	}

	validNtfy := valid
	validNtfy.Notify.Ntfy.Topic = "alerts"
	if err := validNtfy.Validate(); err != nil {
		t.Fatalf("Validate() ntfy config error = %v", err)
	}

	invalidSendFor := valid
	invalidSendFor.Notify.SendFor = []string{"backup", "nope"}
	if err := invalidSendFor.Validate(); err == nil || !strings.Contains(err.Error(), "send_for") {
		t.Fatalf("Validate() send_for err = %v", err)
	}

	invalidNtfy := valid
	invalidNtfy.Notify.Ntfy.URL = ""
	invalidNtfy.Notify.Ntfy.Topic = "alerts"
	if err := invalidNtfy.Validate(); err == nil || !strings.Contains(err.Error(), "health.notify.ntfy.url") {
		t.Fatalf("Validate() ntfy url err = %v", err)
	}

	invalidNtfyTopic := valid
	invalidNtfyTopic.Notify.Ntfy.URL = "https://ntfy.example.com"
	invalidNtfyTopic.Notify.Ntfy.Topic = ""
	if err := invalidNtfyTopic.Validate(); err == nil || !strings.Contains(err.Error(), "health.notify.ntfy.topic") {
		t.Fatalf("Validate() ntfy topic err = %v", err)
	}
}

func TestLoadAppConfig_UpdateNotify(t *testing.T) {
	path := writeTempConfig(t, strings.Join([]string{
		`[update.notify]`,
		`notify_on = ["failed", "succeeded"]`,
		`interactive = true`,
		`webhook_url = "https://example.invalid/update"`,
		`[update.notify.ntfy]`,
		`url = "https://ntfy.example.com"`,
		`topic = "duplicacy-updates"`,
	}, "\n"))

	cfg, err := LoadAppConfig(path)
	if err != nil {
		t.Fatalf("LoadAppConfig() error = %v", err)
	}
	notify := cfg.Update.Notify
	if notify.WebhookURL != "https://example.invalid/update" || !notify.Interactive {
		t.Fatalf("notify = %+v", notify)
	}
	if notify.Ntfy.URL != "https://ntfy.example.com" || notify.Ntfy.Topic != "duplicacy-updates" {
		t.Fatalf("ntfy = %+v", notify.Ntfy)
	}
	if got := strings.Join(notify.NotifyOn, ","); got != "failed,succeeded" {
		t.Fatalf("NotifyOn = %q", got)
	}
}

func TestLoadAppConfig_UpdateNotifyRejectsTargetSendFor(t *testing.T) {
	path := writeTempConfig(t, strings.Join([]string{
		`[update.notify]`,
		`send_for = ["backup"]`,
	}, "\n"))

	_, err := LoadAppConfig(path)
	if err == nil || !strings.Contains(err.Error(), "update.notify.send_for") {
		t.Fatalf("LoadAppConfig() err = %v", err)
	}
}
