// Package config handles INI-style configuration file parsing and validation.
// It supports [common] and per-mode sections ([local]/[remote]) with strict
// key validation matching the original bash script's behaviour.
package config

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"

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

// AllowedConfigKeys is the set of permitted configuration keys.
var AllowedConfigKeys = map[string]bool{
	"DESTINATION":                      true,
	"FILTER":                           true,
	"LOCAL_OWNER":                      true,
	"LOCAL_GROUP":                      true,
	"LOG_RETENTION_DAYS":               true,
	"PRUNE":                            true,
	"THREADS":                          true,
	"SAFE_PRUNE_MAX_DELETE_COUNT":      true,
	"SAFE_PRUNE_MAX_DELETE_PERCENT":    true,
	"SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT": true,
}

// LocalOnlyKeys lists config keys that are only valid in the [local] section.
// These keys are rejected if they appear in [common] or [remote] sections.
var LocalOnlyKeys = map[string]bool{
	"LOCAL_OWNER": true,
	"LOCAL_GROUP": true,
}

var keyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
var ownerPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

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

// NewDefaults returns a Config with all default values.
// Note: LocalOwner and LocalGroup are mandatory for local operations and have
// no defaults — they must be explicitly set in the configuration file when
// running in local mode.  They are not required for remote operations.
func NewDefaults() *Config {
	return &Config{
		LogRetentionDays:            DefaultLogRetentionDays,
		SafePruneMaxDeletePercent:   DefaultSafePruneMaxDeletePercent,
		SafePruneMaxDeleteCount:     DefaultSafePruneMaxDeleteCount,
		SafePruneMinTotalForPercent: DefaultSafePruneMinTotalForPercent,
	}
}

// ParseFile parses the INI-style config file for [common] + [targetSection].
// Values from both sections are merged, with later definitions winning.
func ParseFile(path, targetSection string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, apperrors.NewConfigError("open", fmt.Errorf("cannot open config file %s: %w", path, err), "path", path)
	}
	defer f.Close()

	result := make(map[string]string)
	seenSections := make(map[string]bool)
	currentSection := ""
	lineno := 0

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineno++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = line[1 : len(line)-1]
			if seenSections[currentSection] {
				return nil, apperrors.NewConfigError("parse", fmt.Errorf("config file has duplicate section [%s] at line %d", currentSection, lineno), "path", path)
			}
			seenSections[currentSection] = true
			continue
		}

		// Must be inside a section
		if currentSection == "" {
			return nil, apperrors.NewConfigError("parse", fmt.Errorf("config file has content outside a section at line %d: %s", lineno, raw), "path", path)
		}

		// Must contain =
		if !strings.Contains(line, "=") {
			return nil, apperrors.NewConfigError("parse", fmt.Errorf("config file has invalid line at %d (expected key=value): %s", lineno, raw), "path", path)
		}

		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, apperrors.NewConfigError("parse", fmt.Errorf("config file has malformed key=value pair with missing key at line %d", lineno), "path", path)
		}

		if !keyPattern.MatchString(key) {
			return nil, apperrors.NewConfigError("parse", fmt.Errorf("invalid config key '%s' at line %d in section [%s]", key, lineno, currentSection), "path", path)
		}

		if !AllowedConfigKeys[key] {
			return nil, apperrors.NewConfigError("parse", fmt.Errorf("config key '%s' is not permitted at line %d in section [%s]", key, lineno, currentSection), "path", path)
		}

		// LOCAL_OWNER and LOCAL_GROUP are only meaningful for local operations;
		// reject them if they appear outside the [local] section.
		if currentSection != "local" && LocalOnlyKeys[key] {
			return nil, apperrors.NewConfigError("parse", fmt.Errorf("config key '%s' at line %d is only allowed in [local] section, not [%s]", key, lineno, currentSection), "path", path)
		}

		// Strip surrounding quotes
		if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = value[1 : len(value)-1]
		}

		// Apply values from [common] and [targetSection]
		if currentSection == "common" || currentSection == targetSection {
			result[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, apperrors.NewConfigError("read", fmt.Errorf("error reading config file: %w", err), "path", path)
	}

	if !seenSections["common"] {
		return nil, apperrors.NewConfigError("section-common", fmt.Errorf("config file %s is missing required [common] section", path), "path", path)
	}
	if !seenSections[targetSection] {
		return nil, apperrors.NewConfigError("section-target", fmt.Errorf("config file %s is missing required [%s] section for current mode", path, targetSection), "path", path, "section", targetSection)
	}

	return result, nil
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
			return apperrors.NewConfigError("log-retention-days", fmt.Errorf("LOG_RETENTION_DAYS: %w", err))
		}
		c.LogRetentionDays = n
	}
	if v, ok := values["THREADS"]; ok {
		n, err := strictAtoi(v)
		if err != nil {
			return apperrors.NewConfigError("threads", fmt.Errorf("THREADS: %w", err))
		}
		c.Threads = n
	}
	if v, ok := values["SAFE_PRUNE_MAX_DELETE_PERCENT"]; ok {
		n, err := strictAtoi(v)
		if err != nil {
			return apperrors.NewConfigError("safe-prune-max-delete-percent", fmt.Errorf("SAFE_PRUNE_MAX_DELETE_PERCENT: %w", err))
		}
		c.SafePruneMaxDeletePercent = n
	}
	if v, ok := values["SAFE_PRUNE_MAX_DELETE_COUNT"]; ok {
		n, err := strictAtoi(v)
		if err != nil {
			return apperrors.NewConfigError("safe-prune-max-delete-count", fmt.Errorf("SAFE_PRUNE_MAX_DELETE_COUNT: %w", err))
		}
		c.SafePruneMaxDeleteCount = n
	}
	if v, ok := values["SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT"]; ok {
		n, err := strictAtoi(v)
		if err != nil {
			return apperrors.NewConfigError("safe-prune-min-total-for-percent", fmt.Errorf("SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT: %w", err))
		}
		c.SafePruneMinTotalForPercent = n
	}
	return nil
}

// ValidateRequired checks that required config keys are present based on mode.
func (c *Config) ValidateRequired(doBackup, doPrune bool) error {
	var missing []string

	if c.Destination == "" {
		missing = append(missing, "DESTINATION")
	}
	if doBackup && c.Threads == 0 {
		missing = append(missing, "THREADS")
	}
	if doPrune && c.Prune == "" {
		missing = append(missing, "PRUNE")
	}

	if len(missing) > 0 {
		return apperrors.NewConfigError("required", fmt.Errorf("missing required config variables: %s", strings.Join(missing, ", ")))
	}
	return nil
}

// ValidateThresholds validates numeric threshold values.
func (c *Config) ValidateThresholds() error {
	if c.SafePruneMaxDeletePercent < 0 || c.SafePruneMaxDeletePercent > 100 {
		return apperrors.NewConfigError("safe-prune-max-delete-percent", fmt.Errorf("SAFE_PRUNE_MAX_DELETE_PERCENT must be between 0 and 100 (was %d)", c.SafePruneMaxDeletePercent))
	}
	if c.SafePruneMaxDeleteCount < 0 {
		return apperrors.NewConfigError("safe-prune-max-delete-count", fmt.Errorf("SAFE_PRUNE_MAX_DELETE_COUNT must be non-negative (was %d)", c.SafePruneMaxDeleteCount))
	}
	if c.SafePruneMinTotalForPercent < 0 {
		return apperrors.NewConfigError("safe-prune-min-total-for-percent", fmt.Errorf("SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT must be non-negative (was %d)", c.SafePruneMinTotalForPercent))
	}
	if c.LogRetentionDays < 0 {
		return apperrors.NewConfigError("log-retention-days", fmt.Errorf("LOG_RETENTION_DAYS must be non-negative (was %d)", c.LogRetentionDays))
	}
	return nil
}

// ValidateOwnerGroup validates that local owner and group are specified and
// contain valid Unix username characters.  These fields are mandatory for
// LOCAL operations only and must appear in the [local] config section — the
// backup runs as root but local repository files must be owned by a non-root
// user for security.  This method should NOT be called when operating in
// remote mode (--remote), as remote targets do not use local file ownership.
func (c *Config) ValidateOwnerGroup() error {
	if c.LocalOwner == "" {
		return apperrors.NewConfigError("local-owner", fmt.Errorf("LOCAL_OWNER is mandatory: set it in your .conf file to the non-root user that should own backup files (e.g. LOCAL_OWNER=myuser)"))
	}
	if c.LocalGroup == "" {
		return apperrors.NewConfigError("local-group", fmt.Errorf("LOCAL_GROUP is mandatory: set it in your .conf file to the group that should own backup files (e.g. LOCAL_GROUP=users)"))
	}
	if !ownerPattern.MatchString(c.LocalOwner) {
		return apperrors.NewConfigError("local-owner", fmt.Errorf("LOCAL_OWNER has invalid value '%s'", c.LocalOwner), "value", c.LocalOwner)
	}
	if !ownerPattern.MatchString(c.LocalGroup) {
		return apperrors.NewConfigError("local-group", fmt.Errorf("LOCAL_GROUP has invalid value '%s'", c.LocalGroup), "value", c.LocalGroup)
	}
	// Reject root user/group for security: the backup script runs as root but
	// repository files must never be owned by root to limit blast-radius of
	// any future vulnerability in the backup data path.
	if strings.EqualFold(c.LocalOwner, "root") {
		return apperrors.NewConfigError("local-owner", fmt.Errorf("LOCAL_OWNER must not be 'root' for security reasons: backup files should be owned by a non-root user"))
	}
	if strings.EqualFold(c.LocalGroup, "root") {
		return apperrors.NewConfigError("local-group", fmt.Errorf("LOCAL_GROUP must not be 'root' for security reasons: backup files should be owned by a non-root group"))
	}

	// Verify the specified user actually exists on the system.
	if _, err := user.Lookup(c.LocalOwner); err != nil {
		return apperrors.NewConfigError("local-owner", fmt.Errorf("LOCAL_OWNER '%s' does not exist on this system: %w", c.LocalOwner, err), "value", c.LocalOwner)
	}
	// Verify the specified group actually exists on the system.
	if _, err := user.LookupGroup(c.LocalGroup); err != nil {
		return apperrors.NewConfigError("local-group", fmt.Errorf("LOCAL_GROUP '%s' does not exist on this system: %w", c.LocalGroup, err), "value", c.LocalGroup)
	}

	return nil
}

// ValidateThreads checks the THREADS value is a power of 2 and within limits.
func (c *Config) ValidateThreads() error {
	t := c.Threads
	if t <= 0 || t > MaxThreads || (t&(t-1)) != 0 {
		return apperrors.NewConfigError("threads", fmt.Errorf("THREADS must be a power of 2 and <= %d (was %d)", MaxThreads, t))
	}
	return nil
}

// BuildPruneArgs splits the PRUNE string into individual arguments.
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
// This ensures operators are notified immediately when config values are
// invalid, rather than silently falling back to defaults.
func strictAtoi(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("value is empty; expected a non-negative integer")
	}
	// Reject explicitly negative values
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("value %q is negative; expected a non-negative integer", s)
	}
	const maxInt = int(^uint(0) >> 1) // platform-specific max int
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
