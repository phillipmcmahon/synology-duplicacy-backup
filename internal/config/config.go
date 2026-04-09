// Package config handles TOML configuration file parsing and validation.
// It supports [common] and per-mode tables ([local]/[remote]) with strict
// key validation matching the current workflow semantics.
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
	DefaultSecretsPrefix               = "duplicacy"
	MaxThreads                         = 16
)

var ownerPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
var upperCaseConfigKeyPattern = regexp.MustCompile(`(?m)^\s*[A-Z][A-Z0-9_]*\s*=`)

// Config holds all parsed and validated configuration values.
type Config struct {
	Destination                 string
	Filter                      string
	LocalOwner                  string
	LocalGroup                  string
	LogRetentionDays            int
	Prune                       string
	Threads                     int
	SafePruneMaxDeletePercent   int
	SafePruneMaxDeleteCount     int
	SafePruneMinTotalForPercent int
	PruneArgs                   []string
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

// File holds the decoded TOML config file before mode-specific resolution.
type File struct {
	Common *tableCommon `toml:"common"`
	Local  *tableLocal  `toml:"local"`
	Remote *tableCommon `toml:"remote"`
}

// NewDefaults returns a Config with all default values.
// Note: LocalOwner and LocalGroup are mandatory for local operations and have
// no defaults — they must be explicitly set in the configuration file when
// running in local mode. They are not required for remote operations.
func NewDefaults() *Config {
	return &Config{
		LogRetentionDays:            DefaultLogRetentionDays,
		SafePruneMaxDeletePercent:   DefaultSafePruneMaxDeletePercent,
		SafePruneMaxDeleteCount:     DefaultSafePruneMaxDeleteCount,
		SafePruneMinTotalForPercent: DefaultSafePruneMinTotalForPercent,
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

// ResolveValues merges [common] and the active mode table into the existing
// key/value shape used by Config.Apply, with mode values overriding common.
func (f *File) ResolveValues(targetSection, path string) (map[string]string, error) {
	if f.Common == nil {
		return nil, apperrors.NewConfigError("section-common", fmt.Errorf("config file %s is missing required [common] table", path), "path", path)
	}

	values := make(map[string]string)
	mergeCommon(values, f.Common)

	switch targetSection {
	case "local":
		if f.Local == nil {
			return nil, apperrors.NewConfigError("section-target", fmt.Errorf("config file %s is missing required [local] table for current mode", path), "path", path, "section", targetSection)
		}
		mergeLocal(values, f.Local)
	case "remote":
		if f.Remote == nil {
			return nil, apperrors.NewConfigError("section-target", fmt.Errorf("config file %s is missing required [remote] table for current mode", path), "path", path, "section", targetSection)
		}
		mergeCommon(values, f.Remote)
	default:
		return nil, apperrors.NewConfigError("section-target", fmt.Errorf("unsupported target section %q", targetSection), "path", path, "section", targetSection)
	}

	return values, nil
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
	if (field == "local_owner" || field == "local_group") && section != "local" {
		return apperrors.NewConfigError("parse", fmt.Errorf("config key '%s' is only allowed in [local] table, not [%s]", field, section), "path", path)
	}

	return apperrors.NewConfigError("parse", fmt.Errorf("config key '%s' is not permitted in [%s]", field, section), "path", path)
}

// Apply populates a Config struct from parsed key-value pairs.
// Returns an error if any numeric value is not a valid non-negative integer.
func (c *Config) Apply(values map[string]string) error {
	if v, ok := values["DESTINATION"]; ok {
		c.Destination = v
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
		missing = append(missing, "destination")
	}
	if doBackup && c.Threads == 0 {
		missing = append(missing, "threads")
	}
	if doPrune && c.Prune == "" {
		missing = append(missing, "prune")
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
	return nil
}

// ValidateOwnerGroup validates that local owner and group are specified and
// contain valid Unix username characters. These fields are mandatory for
// LOCAL operations only and must appear in the [local] config table — the
// backup runs as root but local repository files must be owned by a non-root
// user for security. This method should NOT be called when operating in
// remote mode (--remote), as remote targets do not use local file ownership.
func (c *Config) ValidateOwnerGroup() error {
	if c.LocalOwner == "" {
		return apperrors.NewConfigError("local-owner", fmt.Errorf("local_owner is mandatory: set it in your TOML config under [local] to the non-root user that should own backup files (e.g. local_owner = \"myuser\")"))
	}
	if c.LocalGroup == "" {
		return apperrors.NewConfigError("local-group", fmt.Errorf("local_group is mandatory: set it in your TOML config under [local] to the group that should own backup files (e.g. local_group = \"users\")"))
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

// ValidateThreads checks the threads value is a power of 2 and within limits.
func (c *Config) ValidateThreads() error {
	t := c.Threads
	if t <= 0 || t > MaxThreads || (t&(t-1)) != 0 {
		return apperrors.NewConfigError("threads", fmt.Errorf("threads must be a power of 2 and <= %d (was %d)", MaxThreads, t))
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
