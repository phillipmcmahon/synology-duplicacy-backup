// Package config handles INI-style configuration file parsing and validation.
// It supports [common] and per-mode sections ([local]/[remote]) with strict
// key validation matching the original bash script's behaviour.
package config

import (
        "bufio"
        "fmt"
        "os"
        "regexp"
        "strings"
)

// Defaults for safe prune thresholds.
const (
        DefaultSafePruneMaxDeletePercent   = 10
        DefaultSafePruneMaxDeleteCount     = 25
        DefaultSafePruneMinTotalForPercent = 20
        DefaultLocalOwner                  = "phillipmcmahon"
        DefaultLocalGroup                  = "users"
        DefaultLogRetentionDays            = 30
        DefaultSecretsDir = "/root/.secrets"
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
func NewDefaults() *Config {
        return &Config{
                LocalOwner:                  DefaultLocalOwner,
                LocalGroup:                  DefaultLocalGroup,
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
                return nil, fmt.Errorf("cannot open config file %s: %w", path, err)
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
                                return nil, fmt.Errorf("config file has duplicate section [%s] at line %d", currentSection, lineno)
                        }
                        seenSections[currentSection] = true
                        continue
                }

                // Must be inside a section
                if currentSection == "" {
                        return nil, fmt.Errorf("config file has content outside a section at line %d: %s", lineno, raw)
                }

                // Must contain =
                if !strings.Contains(line, "=") {
                        return nil, fmt.Errorf("config file has invalid line at %d (expected key=value): %s", lineno, raw)
                }

                parts := strings.SplitN(line, "=", 2)
                key := strings.TrimSpace(parts[0])
                value := strings.TrimSpace(parts[1])

                if key == "" {
                        return nil, fmt.Errorf("config file has malformed key=value pair with missing key at line %d", lineno)
                }

                if !keyPattern.MatchString(key) {
                        return nil, fmt.Errorf("invalid config key '%s' at line %d in section [%s]", key, lineno, currentSection)
                }

                if !AllowedConfigKeys[key] {
                        return nil, fmt.Errorf("config key '%s' is not permitted at line %d in section [%s]", key, lineno, currentSection)
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
                return nil, fmt.Errorf("error reading config file: %w", err)
        }

        if !seenSections["common"] {
                return nil, fmt.Errorf("config file %s is missing required [common] section", path)
        }
        if !seenSections[targetSection] {
                return nil, fmt.Errorf("config file %s is missing required [%s] section for current mode", path, targetSection)
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
                        return fmt.Errorf("LOG_RETENTION_DAYS: %w", err)
                }
                c.LogRetentionDays = n
        }
        if v, ok := values["THREADS"]; ok {
                n, err := strictAtoi(v)
                if err != nil {
                        return fmt.Errorf("THREADS: %w", err)
                }
                c.Threads = n
        }
        if v, ok := values["SAFE_PRUNE_MAX_DELETE_PERCENT"]; ok {
                n, err := strictAtoi(v)
                if err != nil {
                        return fmt.Errorf("SAFE_PRUNE_MAX_DELETE_PERCENT: %w", err)
                }
                c.SafePruneMaxDeletePercent = n
        }
        if v, ok := values["SAFE_PRUNE_MAX_DELETE_COUNT"]; ok {
                n, err := strictAtoi(v)
                if err != nil {
                        return fmt.Errorf("SAFE_PRUNE_MAX_DELETE_COUNT: %w", err)
                }
                c.SafePruneMaxDeleteCount = n
        }
        if v, ok := values["SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT"]; ok {
                n, err := strictAtoi(v)
                if err != nil {
                        return fmt.Errorf("SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT: %w", err)
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
                return fmt.Errorf("missing required config variables: %s", strings.Join(missing, ", "))
        }
        return nil
}

// ValidateThresholds validates numeric threshold values.
func (c *Config) ValidateThresholds() error {
        if c.SafePruneMaxDeletePercent < 0 || c.SafePruneMaxDeletePercent > 100 {
                return fmt.Errorf("SAFE_PRUNE_MAX_DELETE_PERCENT must be between 0 and 100 (was %d)", c.SafePruneMaxDeletePercent)
        }
        if c.SafePruneMaxDeleteCount < 0 {
                return fmt.Errorf("SAFE_PRUNE_MAX_DELETE_COUNT must be non-negative (was %d)", c.SafePruneMaxDeleteCount)
        }
        if c.SafePruneMinTotalForPercent < 0 {
                return fmt.Errorf("SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT must be non-negative (was %d)", c.SafePruneMinTotalForPercent)
        }
        if c.LogRetentionDays < 0 {
                return fmt.Errorf("LOG_RETENTION_DAYS must be non-negative (was %d)", c.LogRetentionDays)
        }
        return nil
}

// ValidateOwnerGroup validates local owner and group names.
func (c *Config) ValidateOwnerGroup() error {
        if !ownerPattern.MatchString(c.LocalOwner) {
                return fmt.Errorf("LOCAL_OWNER has invalid value '%s'", c.LocalOwner)
        }
        if !ownerPattern.MatchString(c.LocalGroup) {
                return fmt.Errorf("LOCAL_GROUP has invalid value '%s'", c.LocalGroup)
        }
        return nil
}

// ValidateThreads checks the THREADS value is a power of 2 and within limits.
func (c *Config) ValidateThreads() error {
        t := c.Threads
        if t <= 0 || t > MaxThreads || (t&(t-1)) != 0 {
                return fmt.Errorf("THREADS must be a power of 2 and <= %d (was %d)", MaxThreads, t)
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
// or represents a negative number. This ensures operators are notified
// immediately when config values are invalid, rather than silently
// falling back to defaults.
func strictAtoi(s string) (int, error) {
        if s == "" {
                return 0, fmt.Errorf("value is empty; expected a non-negative integer")
        }
        // Reject explicitly negative values
        if strings.HasPrefix(s, "-") {
                return 0, fmt.Errorf("value %q is negative; expected a non-negative integer", s)
        }
        n := 0
        for i, ch := range s {
                if ch < '0' || ch > '9' {
                        return 0, fmt.Errorf("value %q is not a valid integer (invalid character %q at position %d)", s, ch, i)
                }
                n = n*10 + int(ch-'0')
        }
        return n, nil
}
