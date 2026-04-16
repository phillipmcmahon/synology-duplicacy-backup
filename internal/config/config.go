// Package config handles TOML configuration file parsing and validation.
// It supports the current single-file-per-label model with shared [common]
// values plus [targets.<name>] sections, and retains older layouts as
// fallback parsers where useful.
package config

import (
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

// Defaults for safe prune thresholds.
const (
	DefaultSafePruneMaxDeletePercent   = 10
	DefaultSafePruneMaxDeleteCount     = 25
	DefaultSafePruneMinTotalForPercent = 20
	DefaultLogRetentionDays            = 30
	DefaultSecretsDir                  = "/root/.secrets"
	DefaultAppConfigFile               = "duplicacy-backup.toml"
	MaxThreads                         = 16
	MaxHealthThresholdHours            = 24 * 366
)

var ownerPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
var upperCaseConfigKeyPattern = regexp.MustCompile(`(?m)^\s*[A-Z][A-Z0-9_]*\s*=`)
var pruneKeepValuePattern = regexp.MustCompile(`^\d+:\d+$`)

// Config holds all parsed and validated configuration values.
type Config struct {
	Label                       string
	Target                      string
	StorageType                 string
	Location                    string
	SourcePath                  string
	Destination                 string
	Repository                  string
	Filter                      string
	LocalOwner                  string
	LocalGroup                  string
	AllowLocalAccounts          bool
	LogRetentionDays            int
	Prune                       string
	Threads                     int
	SafePruneMaxDeletePercent   int
	SafePruneMaxDeleteCount     int
	SafePruneMinTotalForPercent int
	PruneArgs                   []string
	Health                      HealthConfig
}

type HealthConfig struct {
	FreshnessWarnHours int
	FreshnessFailHours int
	DoctorWarnAfter    int
	VerifyWarnAfter    int
	Notify             HealthNotifyConfig
}

type HealthNotifyConfig struct {
	WebhookURL  string
	Ntfy        HealthNotifyNtfyConfig
	NotifyOn    []string
	SendFor     []string
	Interactive bool
}

type HealthNotifyNtfyConfig struct {
	URL   string
	Topic string
}

type AppConfig struct {
	Update UpdateConfig
}

type UpdateConfig struct {
	Notify HealthNotifyConfig
}

type appFile struct {
	Update *tableUpdate `toml:"update"`
}

type tableUpdate struct {
	Notify *tableHealthNotify `toml:"notify"`
}

type tableCommon struct {
	Destination                 *string `toml:"destination"`
	Filter                      *string `toml:"filter"`
	LogRetentionDays            *int    `toml:"log_retention_days"`
	Prune                       *string `toml:"prune"`
	Threads                     *int    `toml:"threads"`
	SafePruneMaxDeletePercent   *int    `toml:"safe_prune_max_delete_percent"`
	SafePruneMaxDeleteCount     *int    `toml:"safe_prune_max_delete_count"`
	SafePruneMinTotalForPercent *int    `toml:"safe_prune_min_total_for_percent"`
}

type tableLocal struct {
	Destination                 *string `toml:"destination"`
	Filter                      *string `toml:"filter"`
	LocalOwner                  *string `toml:"local_owner"`
	LocalGroup                  *string `toml:"local_group"`
	LogRetentionDays            *int    `toml:"log_retention_days"`
	Prune                       *string `toml:"prune"`
	Threads                     *int    `toml:"threads"`
	SafePruneMaxDeletePercent   *int    `toml:"safe_prune_max_delete_percent"`
	SafePruneMaxDeleteCount     *int    `toml:"safe_prune_max_delete_count"`
	SafePruneMinTotalForPercent *int    `toml:"safe_prune_min_total_for_percent"`
}

type tableTargetMeta struct {
	Name               *string `toml:"name"`
	Type               *string `toml:"type"`
	Location           *string `toml:"location"`
	AllowLocalAccounts *bool   `toml:"allow_local_accounts"`
	LocalOwner         *string `toml:"local_owner"`
	LocalGroup         *string `toml:"local_group"`
}

type tableTarget struct {
	Type                        *string      `toml:"type"`
	Location                    *string      `toml:"location"`
	Destination                 *string      `toml:"destination"`
	Repository                  *string      `toml:"repository"`
	Filter                      *string      `toml:"filter"`
	Threads                     *int         `toml:"threads"`
	Prune                       *string      `toml:"prune"`
	LogRetentionDays            *int         `toml:"log_retention_days"`
	SafePruneMaxDeletePercent   *int         `toml:"safe_prune_max_delete_percent"`
	SafePruneMaxDeleteCount     *int         `toml:"safe_prune_max_delete_count"`
	SafePruneMinTotalForPercent *int         `toml:"safe_prune_min_total_for_percent"`
	AllowLocalAccounts          *bool        `toml:"allow_local_accounts"`
	LocalOwner                  *string      `toml:"local_owner"`
	LocalGroup                  *string      `toml:"local_group"`
	Health                      *tableHealth `toml:"health"`
}

type tableStorage struct {
	Destination *string `toml:"destination"`
	Repository  *string `toml:"repository"`
}

type tableCapture struct {
	Filter  *string `toml:"filter"`
	Threads *int    `toml:"threads"`
}

type tableRetention struct {
	Prune                       *string  `toml:"prune"`
	Keep                        []string `toml:"keep"`
	LogRetentionDays            *int     `toml:"log_retention_days"`
	SafePruneMaxDeletePercent   *int     `toml:"safe_prune_max_delete_percent"`
	SafePruneMaxDeleteCount     *int     `toml:"safe_prune_max_delete_count"`
	SafePruneMinTotalForPercent *int     `toml:"safe_prune_min_total_for_percent"`
}

// File holds the decoded TOML config file before mode-specific resolution.
type File struct {
	Label      *string                `toml:"label"`
	SourcePath *string                `toml:"source_path"`
	Common     *tableCommon           `toml:"common"`
	Targets    map[string]tableTarget `toml:"targets"`
	Local      *tableLocal            `toml:"local"`
	Remote     *tableCommon           `toml:"remote"`
	TargetMeta *tableTargetMeta       `toml:"target"`
	Storage    *tableStorage          `toml:"storage"`
	Capture    *tableCapture          `toml:"capture"`
	Retention  *tableRetention        `toml:"retention"`
	Health     *tableHealth           `toml:"health"`
	Notify     *tableHealthNotify     `toml:"notify"`
}

type tableHealth struct {
	FreshnessWarnHours *int               `toml:"freshness_warn_hours"`
	FreshnessFailHours *int               `toml:"freshness_fail_hours"`
	DoctorWarnAfter    *int               `toml:"doctor_warn_after_hours"`
	VerifyWarnAfter    *int               `toml:"verify_warn_after_hours"`
	Notify             *tableHealthNotify `toml:"notify"`
}

type tableHealthNotify struct {
	WebhookURL  *string                `toml:"webhook_url"`
	Ntfy        *tableHealthNotifyNtfy `toml:"ntfy"`
	NotifyOn    []string               `toml:"notify_on"`
	SendFor     []string               `toml:"send_for"`
	Interactive *bool                  `toml:"interactive"`
}

type tableHealthNotifyNtfy struct {
	URL   *string `toml:"url"`
	Topic *string `toml:"topic"`
}

// NewDefaults returns a Config with all default values.
// Note: LocalOwner and LocalGroup are mandatory only for filesystem targets
// that opt into local account ownership / permission management.
func NewDefaults() *Config {
	return &Config{
		LogRetentionDays:            DefaultLogRetentionDays,
		SafePruneMaxDeletePercent:   DefaultSafePruneMaxDeletePercent,
		SafePruneMaxDeleteCount:     DefaultSafePruneMaxDeleteCount,
		SafePruneMinTotalForPercent: DefaultSafePruneMinTotalForPercent,
		Health: HealthConfig{
			FreshnessWarnHours: 30,
			FreshnessFailHours: 48,
			DoctorWarnAfter:    48,
			VerifyWarnAfter:    168,
			Notify: HealthNotifyConfig{
				Ntfy: HealthNotifyNtfyConfig{
					URL: "https://ntfy.sh",
				},
				NotifyOn:    []string{"degraded", "unhealthy"},
				SendFor:     []string{"doctor", "verify"},
				Interactive: false,
			},
		},
	}
}

func NewAppDefaults() *AppConfig {
	return &AppConfig{
		Update: UpdateConfig{
			Notify: HealthNotifyConfig{
				Ntfy: HealthNotifyNtfyConfig{
					URL: "https://ntfy.sh",
				},
				NotifyOn:    []string{"failed"},
				Interactive: false,
			},
		},
	}
}

// ParseFile parses the TOML config file into its raw table model.
func ParseFile(path string) (*File, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, apperrors.NewConfigError("open", fmt.Errorf("cannot open config file %s: %w", path, err), "path", path)
	}
	text := string(body)

	if match := upperCaseConfigKeyPattern.FindString(text); match != "" {
		key := strings.TrimSpace(strings.TrimSuffix(match, "="))
		return nil, apperrors.NewConfigError("parse", fmt.Errorf("config key %q must use lower snake case in TOML files", key), "path", path)
	}

	var raw File
	meta, err := toml.Decode(text, &raw)
	if err != nil {
		return nil, apperrors.NewConfigError("parse", fmt.Errorf("config file %s contains invalid TOML: %w", path, err), "path", path)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return nil, unexpectedTOMLKey(path, undecoded[0])
	}

	return &raw, nil
}

func LoadAppConfig(path string) (*AppConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, apperrors.NewConfigError("open", fmt.Errorf("cannot open config file %s: %w", path, err), "path", path)
	}
	text := string(body)

	if match := upperCaseConfigKeyPattern.FindString(text); match != "" {
		key := strings.TrimSpace(strings.TrimSuffix(match, "="))
		return nil, apperrors.NewConfigError("parse", fmt.Errorf("config key %q must use lower snake case in TOML files", key), "path", path)
	}

	var raw appFile
	meta, err := toml.Decode(text, &raw)
	if err != nil {
		return nil, apperrors.NewConfigError("parse", fmt.Errorf("config file %s contains invalid TOML: %w", path, err), "path", path)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return nil, unexpectedTOMLKey(path, undecoded[0])
	}

	cfg := NewAppDefaults()
	if raw.Update != nil && raw.Update.Notify != nil {
		applyNotify(&cfg.Update.Notify, raw.Update.Notify)
	}
	if err := cfg.Update.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ResolveValues merges the decoded file into the existing key/value shape used
// by Config.Apply. It supports both the legacy local/remote table layout and
// the current target-aware layouts.
func (f *File) ResolveValues(targetName, path string) (map[string]string, error) {
	if f.usesTargetsLayout() {
		return f.resolveTargetsValues(targetName, path)
	}
	if f.usesTargetLayout() {
		return f.resolveTargetValues(targetName, path)
	}
	return f.resolveLegacyValues(targetName, path)
}

func (f *File) usesTargetsLayout() bool {
	return len(f.Targets) > 0
}

func (f *File) usesTargetLayout() bool {
	return f.Label != nil || f.SourcePath != nil || f.TargetMeta != nil || f.Storage != nil || f.Capture != nil || f.Retention != nil || f.Notify != nil
}

func (f *File) resolveTargetsValues(targetName, path string) (map[string]string, error) {
	values := make(map[string]string)
	if f.Label != nil {
		values["LABEL"] = strings.TrimSpace(*f.Label)
	}
	if f.SourcePath == nil || strings.TrimSpace(*f.SourcePath) == "" {
		return nil, apperrors.NewConfigError("source-path", fmt.Errorf("config file %s is missing required source_path value", path), "path", path)
	}
	values["SOURCE_PATH"] = strings.TrimSpace(*f.SourcePath)

	if f.Common != nil {
		mergeCommon(values, f.Common)
	}

	selected, target, err := f.selectTarget(targetName, path)
	if err != nil {
		return nil, err
	}
	if target.Destination == nil || strings.TrimSpace(*target.Destination) == "" {
		return nil, apperrors.NewConfigError("destination", fmt.Errorf("config file %s is missing required targets.%s.destination value", path, selected), "path", path, "target", selected)
	}

	values["TARGET"] = selected
	if target.Type != nil {
		values["STORAGE_TYPE"] = strings.TrimSpace(*target.Type)
	}
	if target.Location != nil {
		values["LOCATION"] = strings.TrimSpace(*target.Location)
	}
	values["DESTINATION"] = strings.TrimSpace(*target.Destination)
	if target.Repository != nil && strings.TrimSpace(*target.Repository) != "" {
		values["REPOSITORY"] = strings.TrimSpace(*target.Repository)
	}
	if target.Filter != nil {
		values["FILTER"] = *target.Filter
	}
	if target.Threads != nil {
		values["THREADS"] = fmt.Sprintf("%d", *target.Threads)
	}
	if target.Prune != nil {
		values["PRUNE"] = *target.Prune
	}
	if target.LogRetentionDays != nil {
		values["LOG_RETENTION_DAYS"] = fmt.Sprintf("%d", *target.LogRetentionDays)
	}
	if target.SafePruneMaxDeletePercent != nil {
		values["SAFE_PRUNE_MAX_DELETE_PERCENT"] = fmt.Sprintf("%d", *target.SafePruneMaxDeletePercent)
	}
	if target.SafePruneMaxDeleteCount != nil {
		values["SAFE_PRUNE_MAX_DELETE_COUNT"] = fmt.Sprintf("%d", *target.SafePruneMaxDeleteCount)
	}
	if target.SafePruneMinTotalForPercent != nil {
		values["SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT"] = fmt.Sprintf("%d", *target.SafePruneMinTotalForPercent)
	}
	if target.AllowLocalAccounts != nil {
		values["ALLOW_LOCAL_ACCOUNTS"] = fmt.Sprintf("%t", *target.AllowLocalAccounts)
	}
	if target.LocalOwner != nil {
		values["LOCAL_OWNER"] = *target.LocalOwner
	}
	if target.LocalGroup != nil {
		values["LOCAL_GROUP"] = *target.LocalGroup
	}
	return values, nil
}

func (f *File) selectTarget(targetName, path string) (string, tableTarget, error) {
	selected := strings.TrimSpace(targetName)
	if selected == "" {
		return "", tableTarget{}, apperrors.NewConfigError("target-selection", fmt.Errorf("config file %s requires an explicit target selection", path), "path", path)
	}
	if target, ok := f.Targets[selected]; ok {
		return selected, target, nil
	}
	return "", tableTarget{}, apperrors.NewConfigError("section-target", fmt.Errorf("config file %s is missing required [targets.%s] table", path, selected), "path", path, "section", selected)
}

func (f *File) resolveLegacyValues(targetName, path string) (map[string]string, error) {
	return nil, apperrors.NewConfigError(
		"legacy-layout",
		fmt.Errorf("config file %s uses the retired [common]/[local]/[remote] layout; migrate to [targets.<name>] or [target]/[storage] using type = \"filesystem\"|\"object\" and location = \"local\"|\"remote\"", path),
		"path", path,
	)
}

func (f *File) resolveTargetValues(targetName, path string) (map[string]string, error) {
	values := make(map[string]string)
	if f.TargetMeta == nil {
		return nil, apperrors.NewConfigError("section-target", fmt.Errorf("config file %s is missing required [target] table", path), "path", path)
	}
	if f.Storage == nil {
		return nil, apperrors.NewConfigError("section-storage", fmt.Errorf("config file %s is missing required [storage] table", path), "path", path)
	}
	if f.TargetMeta.Name == nil || strings.TrimSpace(*f.TargetMeta.Name) == "" {
		return nil, apperrors.NewConfigError("target-name", fmt.Errorf("config file %s is missing required target.name value", path), "path", path)
	}
	resolvedTarget := strings.TrimSpace(*f.TargetMeta.Name)
	if targetName != "" && targetName != resolvedTarget {
		return nil, apperrors.NewConfigError("target-name", fmt.Errorf("config file %s defines target %q, expected %q", path, resolvedTarget, targetName), "path", path, "target", resolvedTarget)
	}
	if f.SourcePath == nil || strings.TrimSpace(*f.SourcePath) == "" {
		return nil, apperrors.NewConfigError("source-path", fmt.Errorf("config file %s is missing required source_path value", path), "path", path)
	}
	if f.Storage.Destination == nil || strings.TrimSpace(*f.Storage.Destination) == "" {
		return nil, apperrors.NewConfigError("destination", fmt.Errorf("config file %s is missing required storage.destination value", path), "path", path)
	}
	values["TARGET"] = resolvedTarget
	values["SOURCE_PATH"] = strings.TrimSpace(*f.SourcePath)
	if f.Label != nil {
		values["LABEL"] = strings.TrimSpace(*f.Label)
	}
	if f.TargetMeta.Type != nil {
		values["STORAGE_TYPE"] = strings.TrimSpace(*f.TargetMeta.Type)
	}
	if f.TargetMeta.Location != nil {
		values["LOCATION"] = strings.TrimSpace(*f.TargetMeta.Location)
	}
	if f.TargetMeta.AllowLocalAccounts != nil {
		values["ALLOW_LOCAL_ACCOUNTS"] = fmt.Sprintf("%t", *f.TargetMeta.AllowLocalAccounts)
	}
	if f.TargetMeta.LocalOwner != nil {
		values["LOCAL_OWNER"] = *f.TargetMeta.LocalOwner
	}
	if f.TargetMeta.LocalGroup != nil {
		values["LOCAL_GROUP"] = *f.TargetMeta.LocalGroup
	}
	values["DESTINATION"] = strings.TrimSpace(*f.Storage.Destination)
	if f.Storage.Repository != nil && strings.TrimSpace(*f.Storage.Repository) != "" {
		values["REPOSITORY"] = strings.TrimSpace(*f.Storage.Repository)
	}
	mergeCapture(values, f.Capture)
	mergeRetention(values, f.Retention)
	return values, nil
}

func (f *File) ResolveHealth(targetName string) HealthConfig {
	cfg := NewDefaults().Health
	if f == nil {
		return cfg
	}

	applyHealth := func(src *tableHealth) {
		if src == nil {
			return
		}
		if src.FreshnessWarnHours != nil {
			cfg.FreshnessWarnHours = *src.FreshnessWarnHours
		}
		if src.FreshnessFailHours != nil {
			cfg.FreshnessFailHours = *src.FreshnessFailHours
		}
		if src.DoctorWarnAfter != nil {
			cfg.DoctorWarnAfter = *src.DoctorWarnAfter
		}
		if src.VerifyWarnAfter != nil {
			cfg.VerifyWarnAfter = *src.VerifyWarnAfter
		}
		if src.Notify != nil {
			applyNotify(&cfg.Notify, src.Notify)
		}
	}

	applyHealth(f.Health)
	if f.Notify != nil {
		applyNotify(&cfg.Notify, f.Notify)
	}
	if len(f.Targets) > 0 {
		if selected, target, err := f.selectTarget(targetName, "<label>-backup.toml"); err == nil && selected != "" {
			applyHealth(target.Health)
		}
	} else if targetName != "" {
		if target, ok := f.Targets[targetName]; ok {
			applyHealth(target.Health)
		}
	}
	return cfg
}

func applyNotify(dst *HealthNotifyConfig, src *tableHealthNotify) {
	if src == nil {
		return
	}
	if src.WebhookURL != nil {
		dst.WebhookURL = *src.WebhookURL
	}
	if src.Ntfy != nil {
		if src.Ntfy.URL != nil {
			dst.Ntfy.URL = *src.Ntfy.URL
		}
		if src.Ntfy.Topic != nil {
			dst.Ntfy.Topic = *src.Ntfy.Topic
		}
	}
	if src.NotifyOn != nil {
		dst.NotifyOn = append([]string(nil), src.NotifyOn...)
	}
	if src.SendFor != nil {
		dst.SendFor = append([]string(nil), src.SendFor...)
	}
	if src.Interactive != nil {
		dst.Interactive = *src.Interactive
	}
}

func mergeCommon(dst map[string]string, src *tableCommon) {
	if src == nil {
		return
	}
	if src.Destination != nil {
		dst["DESTINATION"] = *src.Destination
	}
	if src.Filter != nil {
		dst["FILTER"] = *src.Filter
	}
	if src.LogRetentionDays != nil {
		dst["LOG_RETENTION_DAYS"] = fmt.Sprintf("%d", *src.LogRetentionDays)
	}
	if src.Prune != nil {
		dst["PRUNE"] = *src.Prune
	}
	if src.Threads != nil {
		dst["THREADS"] = fmt.Sprintf("%d", *src.Threads)
	}
	if src.SafePruneMaxDeletePercent != nil {
		dst["SAFE_PRUNE_MAX_DELETE_PERCENT"] = fmt.Sprintf("%d", *src.SafePruneMaxDeletePercent)
	}
	if src.SafePruneMaxDeleteCount != nil {
		dst["SAFE_PRUNE_MAX_DELETE_COUNT"] = fmt.Sprintf("%d", *src.SafePruneMaxDeleteCount)
	}
	if src.SafePruneMinTotalForPercent != nil {
		dst["SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT"] = fmt.Sprintf("%d", *src.SafePruneMinTotalForPercent)
	}
}

func mergeLocal(dst map[string]string, src *tableLocal) {
	if src == nil {
		return
	}
	if src.Destination != nil {
		dst["DESTINATION"] = *src.Destination
	}
	if src.Filter != nil {
		dst["FILTER"] = *src.Filter
	}
	if src.LocalOwner != nil {
		dst["LOCAL_OWNER"] = *src.LocalOwner
	}
	if src.LocalGroup != nil {
		dst["LOCAL_GROUP"] = *src.LocalGroup
	}
	if src.LogRetentionDays != nil {
		dst["LOG_RETENTION_DAYS"] = fmt.Sprintf("%d", *src.LogRetentionDays)
	}
	if src.Prune != nil {
		dst["PRUNE"] = *src.Prune
	}
	if src.Threads != nil {
		dst["THREADS"] = fmt.Sprintf("%d", *src.Threads)
	}
	if src.SafePruneMaxDeletePercent != nil {
		dst["SAFE_PRUNE_MAX_DELETE_PERCENT"] = fmt.Sprintf("%d", *src.SafePruneMaxDeletePercent)
	}
	if src.SafePruneMaxDeleteCount != nil {
		dst["SAFE_PRUNE_MAX_DELETE_COUNT"] = fmt.Sprintf("%d", *src.SafePruneMaxDeleteCount)
	}
	if src.SafePruneMinTotalForPercent != nil {
		dst["SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT"] = fmt.Sprintf("%d", *src.SafePruneMinTotalForPercent)
	}
}

func mergeCapture(dst map[string]string, src *tableCapture) {
	if src == nil {
		return
	}
	if src.Filter != nil {
		dst["FILTER"] = *src.Filter
	}
	if src.Threads != nil {
		dst["THREADS"] = fmt.Sprintf("%d", *src.Threads)
	}
}

func mergeRetention(dst map[string]string, src *tableRetention) {
	if src == nil {
		return
	}
	if src.Prune != nil {
		dst["PRUNE"] = *src.Prune
	} else if len(src.Keep) > 0 {
		dst["PRUNE"] = buildKeepPolicy(src.Keep)
	}
	if src.LogRetentionDays != nil {
		dst["LOG_RETENTION_DAYS"] = fmt.Sprintf("%d", *src.LogRetentionDays)
	}
	if src.SafePruneMaxDeletePercent != nil {
		dst["SAFE_PRUNE_MAX_DELETE_PERCENT"] = fmt.Sprintf("%d", *src.SafePruneMaxDeletePercent)
	}
	if src.SafePruneMaxDeleteCount != nil {
		dst["SAFE_PRUNE_MAX_DELETE_COUNT"] = fmt.Sprintf("%d", *src.SafePruneMaxDeleteCount)
	}
	if src.SafePruneMinTotalForPercent != nil {
		dst["SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT"] = fmt.Sprintf("%d", *src.SafePruneMinTotalForPercent)
	}
}

func buildKeepPolicy(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts = append(parts, "-keep "+value)
	}
	return strings.Join(parts, " ")
}

func unexpectedTOMLKey(path string, key toml.Key) error {
	parts := []string(key)
	if len(parts) == 0 {
		return apperrors.NewConfigError("parse", fmt.Errorf("config file %s contains an unexpected TOML key", path), "path", path)
	}

	if len(parts) == 1 {
		return apperrors.NewConfigError("parse", fmt.Errorf("config table [%s] is not permitted in %s", parts[0], path), "path", path)
	}

	section := parts[0]
	field := parts[len(parts)-1]
	if (field == "local_owner" || field == "local_group") && section != "local" && section != "target" && section != "targets" {
		return apperrors.NewConfigError("parse", fmt.Errorf("config key '%s' is only allowed in [local], [target], or [targets.<name>] table, not [%s]", field, section), "path", path)
	}

	return apperrors.NewConfigError("parse", fmt.Errorf("config key '%s' is not permitted in [%s]", field, section), "path", path)
}

// Apply populates a Config struct from parsed key-value pairs.
// Returns an error if any numeric value is not a valid non-negative integer.
func (c *Config) Apply(values map[string]string) error {
	if v, ok := values["LABEL"]; ok {
		c.Label = v
	}
	if v, ok := values["TARGET"]; ok {
		c.Target = v
	}
	if v, ok := values["STORAGE_TYPE"]; ok {
		c.StorageType = v
	}
	if v, ok := values["LOCATION"]; ok {
		c.Location = v
	}
	if v, ok := values["SOURCE_PATH"]; ok {
		c.SourcePath = v
	}
	if v, ok := values["DESTINATION"]; ok {
		c.Destination = v
	}
	if v, ok := values["REPOSITORY"]; ok {
		c.Repository = v
	}
	if v, ok := values["FILTER"]; ok {
		c.Filter = v
	}
	if v, ok := values["LOCAL_OWNER"]; ok {
		c.LocalOwner = v
	}
	if v, ok := values["LOCAL_GROUP"]; ok {
		c.LocalGroup = v
	}
	if v, ok := values["PRUNE"]; ok {
		c.Prune = v
	}
	if v, ok := values["ALLOW_LOCAL_ACCOUNTS"]; ok {
		c.AllowLocalAccounts = strings.EqualFold(v, "true")
	}
	if v, ok := values["LOG_RETENTION_DAYS"]; ok {
		n, err := strictAtoi(v)
		if err != nil {
			return apperrors.NewConfigError("log-retention-days", fmt.Errorf("log_retention_days: %w", err))
		}
		c.LogRetentionDays = n
	}
	if v, ok := values["THREADS"]; ok {
		n, err := strictAtoi(v)
		if err != nil {
			return apperrors.NewConfigError("threads", fmt.Errorf("threads: %w", err))
		}
		c.Threads = n
	}
	if v, ok := values["SAFE_PRUNE_MAX_DELETE_PERCENT"]; ok {
		n, err := strictAtoi(v)
		if err != nil {
			return apperrors.NewConfigError("safe-prune-max-delete-percent", fmt.Errorf("safe_prune_max_delete_percent: %w", err))
		}
		c.SafePruneMaxDeletePercent = n
	}
	if v, ok := values["SAFE_PRUNE_MAX_DELETE_COUNT"]; ok {
		n, err := strictAtoi(v)
		if err != nil {
			return apperrors.NewConfigError("safe-prune-max-delete-count", fmt.Errorf("safe_prune_max_delete_count: %w", err))
		}
		c.SafePruneMaxDeleteCount = n
	}
	if v, ok := values["SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT"]; ok {
		n, err := strictAtoi(v)
		if err != nil {
			return apperrors.NewConfigError("safe-prune-min-total-for-percent", fmt.Errorf("safe_prune_min_total_for_percent: %w", err))
		}
		c.SafePruneMinTotalForPercent = n
	}
	return nil
}

// ValidateRequired checks that required config keys are present based on mode.
func (c *Config) ValidateRequired(doBackup, doPrune bool) error {
	var missing []string

	if c.Destination == "" {
		missing = append(missing, "storage.destination")
	}
	if doBackup && c.Threads == 0 {
		missing = append(missing, "capture.threads")
	}
	if doPrune && c.Prune == "" {
		missing = append(missing, "retention.keep/prune")
	}

	if len(missing) > 0 {
		return apperrors.NewConfigError("required", fmt.Errorf("missing required config values: %s", strings.Join(missing, ", ")))
	}
	return nil
}

// ValidateThresholds validates numeric threshold values.
func (c *Config) ValidateThresholds() error {
	if c.SafePruneMaxDeletePercent < 0 || c.SafePruneMaxDeletePercent > 100 {
		return apperrors.NewConfigError("safe-prune-max-delete-percent", fmt.Errorf("safe_prune_max_delete_percent must be between 0 and 100 (was %d)", c.SafePruneMaxDeletePercent))
	}
	if c.SafePruneMaxDeleteCount < 0 {
		return apperrors.NewConfigError("safe-prune-max-delete-count", fmt.Errorf("safe_prune_max_delete_count must be non-negative (was %d)", c.SafePruneMaxDeleteCount))
	}
	if c.SafePruneMinTotalForPercent < 0 {
		return apperrors.NewConfigError("safe-prune-min-total-for-percent", fmt.Errorf("safe_prune_min_total_for_percent must be non-negative (was %d)", c.SafePruneMinTotalForPercent))
	}
	if c.LogRetentionDays < 0 {
		return apperrors.NewConfigError("log-retention-days", fmt.Errorf("log_retention_days must be non-negative (was %d)", c.LogRetentionDays))
	}
	if c.Health.FreshnessWarnHours < 0 {
		return apperrors.NewConfigError("health-freshness-warn-hours", fmt.Errorf("health.freshness_warn_hours must be non-negative (was %d)", c.Health.FreshnessWarnHours))
	}
	if c.Health.FreshnessFailHours < 0 {
		return apperrors.NewConfigError("health-freshness-fail-hours", fmt.Errorf("health.freshness_fail_hours must be non-negative (was %d)", c.Health.FreshnessFailHours))
	}
	if c.Health.DoctorWarnAfter < 0 {
		return apperrors.NewConfigError("health-doctor-warn-after-hours", fmt.Errorf("health.doctor_warn_after_hours must be non-negative (was %d)", c.Health.DoctorWarnAfter))
	}
	if c.Health.VerifyWarnAfter < 0 {
		return apperrors.NewConfigError("health-verify-warn-after-hours", fmt.Errorf("health.verify_warn_after_hours must be non-negative (was %d)", c.Health.VerifyWarnAfter))
	}
	if c.Health.FreshnessWarnHours > MaxHealthThresholdHours {
		return apperrors.NewConfigError("health-freshness-warn-hours", fmt.Errorf("health.freshness_warn_hours must be less than or equal to %d hours (was %d)", MaxHealthThresholdHours, c.Health.FreshnessWarnHours))
	}
	if c.Health.FreshnessFailHours > MaxHealthThresholdHours {
		return apperrors.NewConfigError("health-freshness-fail-hours", fmt.Errorf("health.freshness_fail_hours must be less than or equal to %d hours (was %d)", MaxHealthThresholdHours, c.Health.FreshnessFailHours))
	}
	if c.Health.DoctorWarnAfter > MaxHealthThresholdHours {
		return apperrors.NewConfigError("health-doctor-warn-after-hours", fmt.Errorf("health.doctor_warn_after_hours must be less than or equal to %d hours (was %d)", MaxHealthThresholdHours, c.Health.DoctorWarnAfter))
	}
	if c.Health.VerifyWarnAfter > MaxHealthThresholdHours {
		return apperrors.NewConfigError("health-verify-warn-after-hours", fmt.Errorf("health.verify_warn_after_hours must be less than or equal to %d hours (was %d)", MaxHealthThresholdHours, c.Health.VerifyWarnAfter))
	}
	if c.Health.FreshnessFailHours > 0 && c.Health.FreshnessWarnHours > c.Health.FreshnessFailHours {
		return apperrors.NewConfigError("health-freshness-range", fmt.Errorf("health.freshness_warn_hours must be less than or equal to health.freshness_fail_hours"))
	}
	if c.StorageType == "local" || c.StorageType == "remote" {
		return apperrors.NewConfigError("target-type", fmt.Errorf("target.type must use \"filesystem\" or \"object\"; old values \"local\" and \"remote\" are no longer supported (was %q)", c.StorageType))
	}
	if c.StorageType != "" && c.StorageType != "filesystem" && c.StorageType != "object" {
		return apperrors.NewConfigError("target-type", fmt.Errorf("target.type must be either \"filesystem\" or \"object\" (was %q)", c.StorageType))
	}
	if c.Location != "" && c.Location != "local" && c.Location != "remote" {
		return apperrors.NewConfigError("target-location", fmt.Errorf("target.location must be either \"local\" or \"remote\" (was %q)", c.Location))
	}
	if err := c.Health.Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateOwnerGroup validates that local owner and group are specified and
// contain valid Unix username characters. These fields are mandatory for local
// permission-management operations only and must be set on the selected target
// entry in the label config. This method should NOT be called for targets that
// do not allow local account ownership.
func (c *Config) ValidateOwnerGroup() error {
	if !c.UsesFilesystem() {
		return apperrors.NewConfigError("local-accounts", fmt.Errorf("target %q does not support local account ownership or permission management because it uses %s storage", c.Target, c.StorageType))
	}
	if (c.Target != "" || c.StorageType != "" || c.AllowLocalAccounts) && !c.AllowLocalAccounts {
		return apperrors.NewConfigError("local-accounts", fmt.Errorf("target %q does not allow local account ownership or permission management", c.Target))
	}
	if c.LocalOwner == "" {
		return apperrors.NewConfigError("local-owner", fmt.Errorf("local_owner is mandatory: set it under [targets.%s] to the non-root user that should own backup files (e.g. local_owner = \"myuser\")", c.Target))
	}
	if c.LocalGroup == "" {
		return apperrors.NewConfigError("local-group", fmt.Errorf("local_group is mandatory: set it under [targets.%s] to the group that should own backup files (e.g. local_group = \"users\")", c.Target))
	}
	if !ownerPattern.MatchString(c.LocalOwner) {
		return apperrors.NewConfigError("local-owner", fmt.Errorf("local_owner has invalid value %q", c.LocalOwner), "value", c.LocalOwner)
	}
	if !ownerPattern.MatchString(c.LocalGroup) {
		return apperrors.NewConfigError("local-group", fmt.Errorf("local_group has invalid value %q", c.LocalGroup), "value", c.LocalGroup)
	}
	if strings.EqualFold(c.LocalOwner, "root") {
		return apperrors.NewConfigError("local-owner", fmt.Errorf("local_owner must not be 'root' for security reasons: backup files should be owned by a non-root user"))
	}
	if strings.EqualFold(c.LocalGroup, "root") {
		return apperrors.NewConfigError("local-group", fmt.Errorf("local_group must not be 'root' for security reasons: backup files should be owned by a non-root group"))
	}
	if _, err := user.Lookup(c.LocalOwner); err != nil {
		return apperrors.NewConfigError("local-owner", fmt.Errorf("local_owner %q does not exist on this system: %w", c.LocalOwner, err), "value", c.LocalOwner)
	}
	if _, err := user.LookupGroup(c.LocalGroup); err != nil {
		return apperrors.NewConfigError("local-group", fmt.Errorf("local_group %q does not exist on this system: %w", c.LocalGroup, err), "value", c.LocalGroup)
	}
	return nil
}

func (c *Config) UsesFilesystem() bool {
	return c != nil && c.StorageType == "filesystem"
}

func (c *Config) UsesObjectStorage() bool {
	return c != nil && c.StorageType == "object"
}

func (c *Config) IsRemoteLocation() bool {
	return c != nil && c.Location == "remote"
}

// ValidateThreads checks the threads value is a power of 2 and within limits.
func (c *Config) ValidateThreads() error {
	t := c.Threads
	if t <= 0 || t > MaxThreads || (t&(t-1)) != 0 {
		return apperrors.NewConfigError("threads", fmt.Errorf("threads must be a power of 2 and <= %d (was %d)", MaxThreads, t))
	}
	return nil
}

// ValidatePrunePolicy checks the configured prune policy has a supported,
// syntactically valid shape before it is passed to Duplicacy.
func (c *Config) ValidatePrunePolicy() error {
	if strings.TrimSpace(c.Prune) == "" {
		return nil
	}

	tokens := strings.Fields(c.Prune)
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		switch token {
		case "-keep":
			if i+1 >= len(tokens) {
				return apperrors.NewConfigError("prune", fmt.Errorf("prune policy is missing a retention value after -keep"))
			}
			value := tokens[i+1]
			if !pruneKeepValuePattern.MatchString(value) {
				return apperrors.NewConfigError("prune", fmt.Errorf("prune policy keep value %q must use <age>:<count> format", value))
			}
			i++
		case "-all", "-exclusive", "-exhaustive":
			continue
		default:
			if strings.HasPrefix(token, "-") {
				return apperrors.NewConfigError("prune", fmt.Errorf("prune policy contains unsupported option %q", token))
			}
			return apperrors.NewConfigError("prune", fmt.Errorf("prune policy contains unexpected bare value %q", token))
		}
	}

	return nil
}

// ValidateTargetSemantics checks target-level configuration combinations that
// are internally inconsistent even before any operation-specific planning.
func (c *Config) ValidateTargetSemantics() error {
	switch {
	case c.StorageType == "":
		return apperrors.NewConfigError("target-type", fmt.Errorf("target.type must be set to \"filesystem\" or \"object\""))
	case c.Location == "":
		return apperrors.NewConfigError("target-location", fmt.Errorf("target.location must be set to \"local\" or \"remote\""))
	case c.UsesObjectStorage() && c.Location == "local":
		return apperrors.NewConfigError("target-location", fmt.Errorf("target.location must not be \"local\" when target.type is \"object\""))
	case !c.UsesFilesystem() && !c.UsesObjectStorage():
		return apperrors.NewConfigError("target-type", fmt.Errorf("target.type must be either \"filesystem\" or \"object\" (was %q)", c.StorageType))
	}

	switch {
	case c.UsesFilesystem():
		if strings.Contains(c.Destination, "://") {
			return apperrors.NewConfigError("destination", fmt.Errorf("filesystem target destination must be a filesystem path, not %q", c.Destination))
		}
	case c.UsesObjectStorage():
		if !strings.Contains(c.Destination, "://") {
			return apperrors.NewConfigError("destination", fmt.Errorf("object target destination must be a URL-like storage target, not %q", c.Destination))
		}
	}

	if !c.UsesFilesystem() {
		if c.AllowLocalAccounts || c.LocalOwner != "" || c.LocalGroup != "" {
			return apperrors.NewConfigError("local-accounts", fmt.Errorf("allow_local_accounts, local_owner, and local_group are only supported for filesystem targets"))
		}
		return nil
	}

	if !c.AllowLocalAccounts {
		if c.LocalOwner != "" || c.LocalGroup != "" {
			return apperrors.NewConfigError("local-accounts", fmt.Errorf("local_owner and local_group require allow_local_accounts = true for target %q", c.Target))
		}
		return nil
	}

	if (c.LocalOwner == "") != (c.LocalGroup == "") {
		return apperrors.NewConfigError("local-accounts", fmt.Errorf("local_owner and local_group must be set together for target %q", c.Target))
	}

	if c.LocalOwner == "" && c.LocalGroup == "" {
		return nil
	}

	return c.ValidateOwnerGroup()
}

// BuildPruneArgs splits the prune string into individual arguments.
func (c *Config) BuildPruneArgs() {
	if c.Prune == "" {
		c.PruneArgs = nil
		return
	}
	c.PruneArgs = strings.Fields(c.Prune)
}

func (h HealthConfig) Validate() error {
	if err := validateHealthNotifyValues("health.notify.notify_on", h.Notify.NotifyOn, "degraded", "unhealthy"); err != nil {
		return err
	}
	if err := validateHealthNotifyValues("health.notify.send_for", h.Notify.SendFor, "status", "doctor", "verify", "backup", "prune", "cleanup-storage"); err != nil {
		return err
	}
	if strings.TrimSpace(h.Notify.Ntfy.URL) != "" && strings.TrimSpace(h.Notify.Ntfy.Topic) == "" && h.Notify.Ntfy.URL != "https://ntfy.sh" {
		return fmt.Errorf("health.notify.ntfy.topic must not be empty when health.notify.ntfy.url is set")
	}
	if strings.TrimSpace(h.Notify.Ntfy.Topic) != "" && strings.TrimSpace(h.Notify.Ntfy.URL) == "" {
		return fmt.Errorf("health.notify.ntfy.url must not be empty when health.notify.ntfy.topic is set")
	}
	return nil
}

func (u UpdateConfig) Validate() error {
	if err := validateHealthNotifyValues("update.notify.notify_on", u.Notify.NotifyOn, "failed", "succeeded", "current", "reinstall-requested"); err != nil {
		return err
	}
	if len(u.Notify.SendFor) > 0 {
		return apperrors.NewConfigError("update.notify.send_for", fmt.Errorf("update.notify.send_for is not supported; use update.notify.notify_on to choose update notification outcomes"))
	}
	if strings.TrimSpace(u.Notify.Ntfy.URL) != "" && strings.TrimSpace(u.Notify.Ntfy.Topic) == "" && u.Notify.Ntfy.URL != "https://ntfy.sh" {
		return fmt.Errorf("update.notify.ntfy.topic must not be empty when update.notify.ntfy.url is set")
	}
	if strings.TrimSpace(u.Notify.Ntfy.Topic) != "" && strings.TrimSpace(u.Notify.Ntfy.URL) == "" {
		return fmt.Errorf("update.notify.ntfy.url must not be empty when update.notify.ntfy.topic is set")
	}
	return nil
}

func validateHealthNotifyValues(field string, values []string, allowed ...string) error {
	if len(values) == 0 {
		return nil
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		allowedSet[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := allowedSet[value]; !ok {
			return apperrors.NewConfigError(field, fmt.Errorf("%s contains unsupported value %q", field, value))
		}
	}
	return nil
}

// strictAtoi converts a string to a non-negative integer.
// Returns an error if the string is empty, contains non-digit characters,
// represents a negative number, or overflows a platform int.
func strictAtoi(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("value is empty; expected a non-negative integer")
	}
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("value %q is negative; expected a non-negative integer", s)
	}
	const maxInt = int(^uint(0) >> 1)
	n := 0
	for i, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("value %q is not a valid integer (invalid character %q at position %d)", s, ch, i)
		}
		digit := int(ch - '0')
		if n > (maxInt-digit)/10 {
			return 0, fmt.Errorf("value %q overflows integer range", s)
		}
		n = n*10 + digit
	}
	return n, nil
}
