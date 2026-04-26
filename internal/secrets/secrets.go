// Package secrets handles loading and validating per-label secret files
// for target-specific Duplicacy storage keys and notification credentials.
package secrets

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

// Secrets holds loaded secret values.
type Secrets struct {
	Keys map[string]string
}

type fileTargetSecrets struct {
	Keys                     map[string]string `toml:"keys"`
	HealthWebhookBearerToken *string           `toml:"health_webhook_bearer_token"`
	HealthNtfyToken          *string           `toml:"health_ntfy_token"`
}

type fileSecrets struct {
	Targets map[string]fileTargetSecrets `toml:"targets"`
}

var upperCaseSecretsKeyPattern = regexp.MustCompile(`(?m)^\s*[A-Z][A-Z0-9_]*\s*=`)

const maskedSecretPlaceholder = "****"

var (
	effectiveUID = os.Geteuid
	lookupEnv    = os.Getenv
)

// GetSecretsFilePath returns the expected secrets file path for a label.
func GetSecretsFilePath(secretsDir, label string) string {
	return filepath.Join(secretsDir, fmt.Sprintf("%s-secrets.toml", label))
}

// ValidateFileAccess checks that the secrets file at path exists, has 0600
// permissions, and is owned by the effective execution user.
func ValidateFileAccess(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return apperrors.NewSecretsError("stat", fmt.Errorf("secrets file not found: %s", path), "path", path)
		}
		return apperrors.NewSecretsError("stat", fmt.Errorf("cannot stat secrets file: %w", err), "path", path)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		return apperrors.NewSecretsError("permissions", fmt.Errorf("secrets file permissions are %04o, expected 0600: %s", perm, path), "path", path)
	}

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if err := validateFileOwner(stat.Uid, path); err != nil {
			return err
		}
	}
	return nil
}

func validateFileOwner(ownerUID uint32, path string) error {
	expectedUID := uint32(effectiveUID())
	if ownerUID == expectedUID {
		return nil
	}
	if expectedUID == 0 {
		if sudoUID, ok := sudoUserUID(); ok && ownerUID == sudoUID {
			return nil
		}
	}
	return apperrors.NewSecretsError("ownership", fmt.Errorf("secrets file owner is uid %d, expected %s: %s", ownerUID, expectedOwnerDescription(expectedUID), path), "path", path)
}

func expectedOwnerDescription(effective uint32) string {
	if effective != 0 {
		return fmt.Sprintf("uid %d for the current execution user", effective)
	}
	if sudoUID, ok := sudoUserUID(); ok {
		return fmt.Sprintf("uid 0 or sudo user uid %d", sudoUID)
	}
	return "uid 0 for the current execution user"
}

func sudoUserUID() (uint32, bool) {
	value := strings.TrimSpace(lookupEnv("SUDO_UID"))
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(parsed), true
}

// ParseSecrets decodes a TOML secrets file from r for a specific target.
func ParseSecrets(r io.Reader, source, target string) (*Secrets, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, apperrors.NewSecretsError("read", fmt.Errorf("error reading secrets file: %w", err), "source", source)
	}
	text := string(body)
	if err := validateSecretsKeysUseLowerSnakeCase(text, source); err != nil {
		return nil, err
	}

	var raw fileSecrets
	meta, err := toml.Decode(text, &raw)
	if err != nil {
		return nil, apperrors.NewSecretsError("parse", fmt.Errorf("secrets file %s contains invalid TOML: %w", source, err), "source", source)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		key := undecoded[0].String()
		return nil, apperrors.NewSecretsError("parse", fmt.Errorf("unexpected key %q in secrets file %s", key, source), "source", source)
	}

	section, ok := raw.Targets[target]
	if !ok {
		return nil, apperrors.NewSecretsError("required", fmt.Errorf("secrets file %s is missing required [targets.%s] table", source, target), "source", source, "target", target)
	}
	return &Secrets{Keys: copyStringMap(section.Keys)}, nil
}

// LoadSecretsFile loads and validates a secrets TOML file for a specific target.
func LoadSecretsFile(path, target string) (*Secrets, error) {
	if err := ValidateFileAccess(path); err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, apperrors.NewSecretsError("open", fmt.Errorf("secrets file is not readable: %s", path), "path", path)
	}
	defer f.Close()

	return ParseSecrets(f, path, target)
}

func LoadOptionalHealthWebhookToken(path, target string) (string, error) {
	return loadOptionalTargetToken(path, target, func(section fileTargetSecrets) *string {
		return section.HealthWebhookBearerToken
	})
}

func LoadOptionalHealthNtfyToken(path, target string) (string, error) {
	if path == "" {
		return "", nil
	}
	return loadOptionalTargetToken(path, target, func(section fileTargetSecrets) *string {
		return section.HealthNtfyToken
	})
}

func loadOptionalTargetToken(path, target string, selectToken func(fileTargetSecrets) *string) (string, error) {
	if path == "" {
		return "", nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", apperrors.NewSecretsError("stat", fmt.Errorf("cannot stat secrets file: %w", err), "path", path)
	}
	if err := ValidateFileAccess(path); err != nil {
		return "", err
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return "", apperrors.NewSecretsError("open", fmt.Errorf("secrets file is not readable: %s", path), "path", path)
	}
	return parseOptionalTargetToken(string(body), path, target, selectToken)
}

func parseOptionalTargetToken(text, source, target string, selectToken func(fileTargetSecrets) *string) (string, error) {
	if err := validateSecretsKeysUseLowerSnakeCase(text, source); err != nil {
		return "", err
	}

	var raw fileSecrets
	meta, err := toml.Decode(text, &raw)
	if err != nil {
		return "", apperrors.NewSecretsError("parse", fmt.Errorf("secrets file %s contains invalid TOML: %w", source, err), "source", source)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		key := undecoded[0].String()
		return "", apperrors.NewSecretsError("parse", fmt.Errorf("unexpected key %q in secrets file %s", key, source), "source", source)
	}

	section, ok := raw.Targets[target]
	if !ok {
		return "", nil
	}
	token := selectToken(section)
	if token == nil {
		return "", nil
	}
	return *token, nil
}

// Validate checks generic Duplicacy storage key values.
func (s *Secrets) Validate() error {
	if s == nil {
		return nil
	}
	for key, value := range s.Keys {
		if strings.TrimSpace(key) == "" {
			return apperrors.NewSecretsError("validate", fmt.Errorf("storage key names must not be empty"))
		}
		if strings.TrimSpace(value) == "" {
			return apperrors.NewSecretsError("validate", fmt.Errorf("storage key %q must not be empty", key))
		}
	}
	return nil
}

func validateSecretsKeysUseLowerSnakeCase(text, source string) error {
	keys := upperCaseSecretsKeys(text)
	if len(keys) == 0 {
		return nil
	}
	if len(keys) == 1 {
		return apperrors.NewSecretsError("parse", fmt.Errorf("secrets key %q must use lower snake case in TOML files", keys[0]), "source", source)
	}
	return apperrors.NewSecretsError("parse", fmt.Errorf("secrets keys %s must use lower snake case in TOML files", quoteKeys(keys)), "source", source)
}

func upperCaseSecretsKeys(text string) []string {
	matches := upperCaseSecretsKeyPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}

	keys := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		key := strings.TrimSpace(strings.TrimSuffix(match, "="))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

func quoteKeys(keys []string) string {
	quoted := make([]string, len(keys))
	for i, key := range keys {
		quoted[i] = fmt.Sprintf("%q", key)
	}
	return strings.Join(quoted, ", ")
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

// MaskedKeys returns an opaque summary of loaded storage keys.
func (s *Secrets) MaskedKeys() string {
	if s == nil || len(s.Keys) == 0 {
		return "<none>"
	}
	if len(s.Keys) == 1 {
		return maskedSecretPlaceholder + " (1 key)"
	}
	return fmt.Sprintf("%s (%d keys)", maskedSecretPlaceholder, len(s.Keys))
}
