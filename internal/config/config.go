// Package config handles TOML configuration file parsing and validation.
// It supports the current single-file-per-label model with shared [common]
// values plus [targets.<name>] sections.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

// Defaults for safe prune thresholds.
const (
	DefaultSafePruneMaxDeletePercent   = 10
	DefaultSafePruneMaxDeleteCount     = 25
	DefaultSafePruneMinTotalForPercent = 20
	DefaultLogRetentionDays            = 30
	DefaultAppConfigFile               = "duplicacy-backup.toml"
	MaxThreads                         = 16
	MaxHealthThresholdHours            = 24 * 366
)

var upperCaseConfigKeyPattern = regexp.MustCompile(`(?m)^\s*[A-Z][A-Z0-9_]*\s*=`)
var pruneKeepValuePattern = regexp.MustCompile(`^\d+:\d+$`)

// Config holds all parsed and validated configuration values.
type Config struct {
	Label                       string
	Target                      string
	Location                    string
	SourcePath                  string
	Storage                     string
	RestoreWorkspaceRoot        string
	RestoreWorkspaceTemplate    string
	Filter                      string
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
	Filter                      *string `toml:"filter"`
	LogRetentionDays            *int    `toml:"log_retention_days"`
	Prune                       *string `toml:"prune"`
	Threads                     *int    `toml:"threads"`
	SafePruneMaxDeletePercent   *int    `toml:"safe_prune_max_delete_percent"`
	SafePruneMaxDeleteCount     *int    `toml:"safe_prune_max_delete_count"`
	SafePruneMinTotalForPercent *int    `toml:"safe_prune_min_total_for_percent"`
}

type tableRestore struct {
	WorkspaceRoot     *string `toml:"workspace_root"`
	WorkspaceTemplate *string `toml:"workspace_template"`
}

type tableTarget struct {
	Location                    *string      `toml:"location"`
	Storage                     *string      `toml:"storage"`
	Filter                      *string      `toml:"filter"`
	Threads                     *int         `toml:"threads"`
	Prune                       *string      `toml:"prune"`
	LogRetentionDays            *int         `toml:"log_retention_days"`
	SafePruneMaxDeletePercent   *int         `toml:"safe_prune_max_delete_percent"`
	SafePruneMaxDeleteCount     *int         `toml:"safe_prune_max_delete_count"`
	SafePruneMinTotalForPercent *int         `toml:"safe_prune_min_total_for_percent"`
	Health                      *tableHealth `toml:"health"`
}

// File holds the decoded TOML config file before mode-specific resolution.
type File struct {
	Label      *string                `toml:"label"`
	SourcePath *string                `toml:"source_path"`
	Common     *tableCommon           `toml:"common"`
	Restore    *tableRestore          `toml:"restore"`
	Targets    map[string]tableTarget `toml:"targets"`
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

// ResolveValues merges the decoded file into the key/value shape used by
// Config.Apply. Only the current [targets.<name>] layout is supported.
func (f *File) ResolveValues(targetName, path string) (map[string]string, error) {
	values := make(map[string]string)
	if f.Label != nil {
		values["LABEL"] = strings.TrimSpace(*f.Label)
	}
	if f.SourcePath != nil && strings.TrimSpace(*f.SourcePath) != "" {
		values["SOURCE_PATH"] = strings.TrimSpace(*f.SourcePath)
	}

	selected, target, err := f.selectTarget(targetName, path)
	if err != nil {
		return nil, err
	}

	if f.Common != nil {
		mergeCommon(values, f.Common)
	}
	mergeRestore(values, f.Restore)

	values["TARGET"] = selected
	if target.Location != nil {
		values["LOCATION"] = strings.TrimSpace(*target.Location)
	}
	if target.Storage == nil || strings.TrimSpace(*target.Storage) == "" {
		return nil, apperrors.NewConfigError("storage", fmt.Errorf("config file %s is missing required targets.%s.storage value", path, selected), "path", path, "target", selected)
	}
	values["STORAGE"] = strings.TrimSpace(*target.Storage)
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
	if selected, target, err := f.selectTarget(targetName, "<label>-backup.toml"); err == nil && selected != "" {
		applyHealth(target.Health)
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

func mergeRestore(dst map[string]string, src *tableRestore) {
	if src == nil {
		return
	}
	if src.WorkspaceRoot != nil {
		dst["RESTORE_WORKSPACE_ROOT"] = strings.TrimSpace(*src.WorkspaceRoot)
	}
	if src.WorkspaceTemplate != nil {
		dst["RESTORE_WORKSPACE_TEMPLATE"] = strings.TrimSpace(*src.WorkspaceTemplate)
	}
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
	if v, ok := values["LOCATION"]; ok {
		c.Location = v
	}
	if v, ok := values["SOURCE_PATH"]; ok {
		c.SourcePath = v
	}
	if v, ok := values["STORAGE"]; ok {
		c.Storage = v
	}
	if v, ok := values["RESTORE_WORKSPACE_ROOT"]; ok {
		c.RestoreWorkspaceRoot = v
	}
	if v, ok := values["RESTORE_WORKSPACE_TEMPLATE"]; ok {
		c.RestoreWorkspaceTemplate = v
	}
	if v, ok := values["FILTER"]; ok {
		c.Filter = v
	}
	if v, ok := values["PRUNE"]; ok {
		c.Prune = v
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

	if c.Storage == "" {
		missing = append(missing, "targets.<name>.storage")
	}
	if doBackup && c.SourcePath == "" {
		missing = append(missing, "source_path")
	}
	if doBackup && c.Threads == 0 {
		missing = append(missing, "common.threads or targets.<name>.threads")
	}
	if doPrune && c.Prune == "" {
		missing = append(missing, "common.prune or targets.<name>.prune")
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
	if c.Location != "" && c.Location != "local" && c.Location != "remote" {
		return apperrors.NewConfigError("target-location", fmt.Errorf("target.location must be either \"local\" or \"remote\" (was %q)", c.Location))
	}
	if err := c.Health.Validate(); err != nil {
		return err
	}
	return nil
}

// UsesPathStorage reports whether Duplicacy accesses the repository through an
// OS filesystem path: bare paths and file:// URLs. This describes the access
// method only, not the target's resilience location. For example, an SMB share
// mounted under /volume1 can be path-based storage while still being a remote
// target.
func (c *Config) UsesPathStorage() bool {
	return c != nil && duplicacy.NewStorageSpec(c.Storage).IsLocalPath()
}

// UsesRootProtectedLocalRepository is the sudo boundary for repository access.
// Only path-based targets whose configured location is local are treated as
// root-protected local repositories. Remote mounted filesystems are governed by
// their mount credentials and permissions, like object-storage targets.
func (c *Config) UsesRootProtectedLocalRepository() bool {
	return c != nil && c.Location == "local" && c.UsesPathStorage()
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
	case c.Location == "":
		return apperrors.NewConfigError("target-location", fmt.Errorf("target.location must be set to \"local\" or \"remote\""))
	}
	return nil
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
