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
[common]
destination = "/volume1/backups"
filter = "-e \\.DS_Store"

[local]
threads = 4
local_owner = "admin"
local_group = "users"
`)

	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	values, err := raw.ResolveValues("local", p)
	if err != nil {
		t.Fatalf("ResolveValues() error = %v", err)
	}

	expect := map[string]string{
		"DESTINATION": "/volume1/backups",
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

func TestParseFile_ResolveHealthDefaultsAndOverrides(t *testing.T) {
	p := writeTempConfig(t, `
[common]
destination = "/volume1/backups"

[local]
threads = 4
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
`)

	raw, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	health := raw.ResolveHealth("local")
	if health.FreshnessWarnHours != 12 || health.FreshnessFailHours != 24 {
		t.Fatalf("health = %+v", health)
	}
	if health.Notify.WebhookURL != "https://example.invalid/hook" || !health.Notify.Interactive {
		t.Fatalf("health notify = %+v", health.Notify)
	}
	if got := strings.Join(health.Notify.SendFor, ","); got != "status,verify" {
		t.Fatalf("SendFor = %q", got)
	}
}

func TestParseFile_TargetTableOverridesCommon(t *testing.T) {
	values := loadValues(t, `
[common]
destination = "/volume1/backups"
threads = 2

[local]
threads = 8
`, "local")

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
type = "local"
destination = "/volumeUSB1/usbshare/duplicacy"
repository = "homes"
allow_local_accounts = true
`, "onsite-usb")

	if values["TARGET"] != "onsite-usb" {
		t.Fatalf("TARGET = %q", values["TARGET"])
	}
	if values["TARGET_TYPE"] != "local" {
		t.Fatalf("TARGET_TYPE = %q", values["TARGET_TYPE"])
	}
}

func TestParseFile_ResolveValues_ProductNeutralTargetLayout(t *testing.T) {
	owner, group := currentUserGroup(t)
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[target]
name = "onsite-usb"
type = "local"
allow_local_accounts = true
local_owner = "`+owner+`"
local_group = "`+group+`"

[storage]
destination = "/volumeUSB1/usbshare/duplicacy"
repository = "homes"

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
		"TARGET_TYPE":                      "local",
		"SOURCE_PATH":                      "/volume1/homes",
		"DESTINATION":                      "/volumeUSB1/usbshare/duplicacy",
		"REPOSITORY":                       "homes",
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
type = "remote"
requires_network = true

[storage]
destination = "s3://bucket/homes"
repository = "homes"

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
	if values["REQUIRES_NETWORK"] != "true" {
		t.Fatalf("REQUIRES_NETWORK = %q", values["REQUIRES_NETWORK"])
	}
}

func TestParseFile_ResolveValues_ProductNeutralTargetMismatchFails(t *testing.T) {
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[target]
name = "onsite-usb"
type = "local"

[storage]
destination = "/volumeUSB1/usbshare/duplicacy"
repository = "homes"
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
type = "remote"
destination = "s3://bucket/homes"
repository = "homes"

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
}

func TestParseFile_ResolveValues_MissingExplicitTargetFails(t *testing.T) {
	p := writeTempConfig(t, `
label = "homes"
source_path = "/volume1/homes"

[targets.onsite-usb]
type = "local"
destination = "/volumeUSB1/usbshare/duplicacy"
repository = "homes"
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

func TestParseFile_IgnoresUnrelatedTable(t *testing.T) {
	values := loadValues(t, `
[common]
destination = "/volume1/backups"

[remote]
threads = 2

[local]
threads = 8
`, "local")

	if values["THREADS"] != "8" {
		t.Errorf("expected THREADS=8, got %q", values["THREADS"])
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
	if err == nil {
		t.Fatal("expected error for missing [common]")
	}
	if !strings.Contains(err.Error(), "[common]") {
		t.Errorf("error should mention [common], got: %v", err)
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
	if err == nil {
		t.Fatal("expected error for missing [local]")
	}
	if !strings.Contains(err.Error(), "[local]") {
		t.Errorf("error should mention [local], got: %v", err)
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
destination = "/volume1/backups"
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
[common]
destination = "/volume1/backups"
log_retention_days = 0
safe_prune_max_delete_count = 0
safe_prune_max_delete_percent = 0
safe_prune_min_total_for_percent = 0

[local]
threads = 0
`, "local")

	if values["THREADS"] != "0" {
		t.Errorf("THREADS = %q, want 0", values["THREADS"])
	}
	if values["LOG_RETENTION_DAYS"] != "0" {
		t.Errorf("LOG_RETENTION_DAYS = %q, want 0", values["LOG_RETENTION_DAYS"])
	}
}

func TestParseFile_MultilineFilterPreserved(t *testing.T) {
	values := loadValues(t, `
[common]
destination = "/volume1/backups"
filter = '''
-exclude tmp
-exclude .DS_Store
'''

[local]
threads = 4
`, "local")

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
		"DESTINATION": "/volume1/backups",
		"FILTER":      "-e *.tmp",
		"LOCAL_OWNER": "admin",
		"LOCAL_GROUP": "staff",
		"PRUNE":       "-keep 0:365 -keep 30:180",
	}
	if err := cfg.Apply(vals); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Destination != "/volume1/backups" || cfg.Filter != "-e *.tmp" || cfg.LocalOwner != "admin" || cfg.LocalGroup != "staff" || cfg.Prune != "-keep 0:365 -keep 30:180" {
		t.Fatalf("cfg after Apply = %+v", cfg)
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
	cfg := &Config{Destination: "/vol", Threads: 4, Prune: "-keep 0:30"}
	if err := cfg.ValidateRequired(true, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := (&Config{Threads: 4, Prune: "-keep 0:30"}).ValidateRequired(true, true); err == nil {
		t.Fatal("expected missing destination")
	}
	if err := (&Config{Destination: "/vol", Prune: "-keep 0:30"}).ValidateRequired(true, true); err == nil {
		t.Fatal("expected missing threads")
	}
	if err := (&Config{Destination: "/vol", Threads: 4}).ValidateRequired(true, true); err == nil {
		t.Fatal("expected missing prune")
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
	if err := (&Config{LocalOwner: owner, LocalGroup: group}).ValidateOwnerGroup(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"missing owner", Config{LocalGroup: group}, "local_owner is mandatory"},
		{"missing group", Config{LocalOwner: owner}, "local_group is mandatory"},
		{"invalid owner", Config{LocalOwner: "admin/bad", LocalGroup: group}, "invalid"},
		{"invalid group", Config{LocalOwner: owner, LocalGroup: "bad group"}, "invalid"},
		{"root owner", Config{LocalOwner: "root", LocalGroup: group}, "must not be 'root'"},
		{"root group", Config{LocalOwner: owner, LocalGroup: "root"}, "must not be 'root'"},
		{"missing user", Config{LocalOwner: "no_such_user_xyz_999", LocalGroup: group}, "does not exist"},
		{"missing group", Config{LocalOwner: owner, LocalGroup: "no_such_group_xyz_999"}, "does not exist"},
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

	invalidSendFor := valid
	invalidSendFor.Notify.SendFor = []string{"backup"}
	if err := invalidSendFor.Validate(); err == nil || !strings.Contains(err.Error(), "send_for") {
		t.Fatalf("Validate() send_for err = %v", err)
	}
}
