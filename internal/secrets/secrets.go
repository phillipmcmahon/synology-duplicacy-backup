// Package secrets handles loading and validating per-label secret files
// for target-specific remote backup operations (e.g., Storj S3 credentials).
package secrets

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

// MinStorjS3IDLen is the minimum length for storj_s3_id.
const MinStorjS3IDLen = 28

// MinStorjS3SecretLen is the minimum length for storj_s3_secret.
const MinStorjS3SecretLen = 53

// Secrets holds loaded secret values.
type Secrets struct {
	StorjS3ID     string
	StorjS3Secret string
}

type fileTargetSecrets struct {
	StorjS3ID                *string `toml:"storj_s3_id"`
	StorjS3Secret            *string `toml:"storj_s3_secret"`
	HealthWebhookBearerToken *string `toml:"health_webhook_bearer_token"`
}

type fileSecrets struct {
	Targets map[string]fileTargetSecrets `toml:"targets"`
}

var upperCaseSecretsKeyPattern = regexp.MustCompile(`(?m)^\s*[A-Z][A-Z0-9_]*\s*=`)

// GetSecretsFilePath returns the expected secrets file path for a label.
func GetSecretsFilePath(secretsDir, label string) string {
	return filepath.Join(secretsDir, fmt.Sprintf("%s-secrets.toml", label))
}

// ValidateFileAccess checks that the secrets file at path exists, has 0600
// permissions, and is owned by root:root.
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
		if stat.Uid != 0 || stat.Gid != 0 {
			return apperrors.NewSecretsError("ownership", fmt.Errorf("secrets file ownership is %d:%d, expected 0:0 (root:root): %s", stat.Uid, stat.Gid, path), "path", path)
		}
	}
	return nil
}

// ParseSecrets decodes a TOML secrets file from r for a specific target.
func ParseSecrets(r io.Reader, source, target string) (*Secrets, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, apperrors.NewSecretsError("read", fmt.Errorf("error reading secrets file: %w", err), "source", source)
	}
	text := string(body)
	if match := upperCaseSecretsKeyPattern.FindString(text); match != "" {
		key := strings.TrimSpace(strings.TrimSuffix(match, "="))
		return nil, apperrors.NewSecretsError("parse", fmt.Errorf("secrets key %q must use lower snake case in TOML files", key), "source", source)
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
	if section.StorjS3ID == nil {
		return nil, apperrors.NewSecretsError("required", fmt.Errorf("required secret 'storj_s3_id' is missing under [targets.%s] in %s", target, source), "source", source, "target", target)
	}
	if section.StorjS3Secret == nil {
		return nil, apperrors.NewSecretsError("required", fmt.Errorf("required secret 'storj_s3_secret' is missing under [targets.%s] in %s", target, source), "source", source, "target", target)
	}

	return &Secrets{
		StorjS3ID:     *section.StorjS3ID,
		StorjS3Secret: *section.StorjS3Secret,
	}, nil
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
	text := string(body)
	if match := upperCaseSecretsKeyPattern.FindString(text); match != "" {
		key := strings.TrimSpace(strings.TrimSuffix(match, "="))
		return "", apperrors.NewSecretsError("parse", fmt.Errorf("secrets key %q must use lower snake case in TOML files", key), "source", path)
	}

	var raw fileSecrets
	meta, err := toml.Decode(text, &raw)
	if err != nil {
		return "", apperrors.NewSecretsError("parse", fmt.Errorf("secrets file %s contains invalid TOML: %w", path, err), "source", path)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		key := undecoded[0].String()
		return "", apperrors.NewSecretsError("parse", fmt.Errorf("unexpected key %q in secrets file %s", key, path), "source", path)
	}

	section, ok := raw.Targets[target]
	if !ok || section.HealthWebhookBearerToken == nil {
		return "", nil
	}
	return *section.HealthWebhookBearerToken, nil
}

// Validate checks minimum length requirements for secrets.
func (s *Secrets) Validate() error {
	if len(s.StorjS3ID) < MinStorjS3IDLen {
		return apperrors.NewSecretsError("validate", fmt.Errorf("storj_s3_id must be at least %d characters (was %d)", MinStorjS3IDLen, len(s.StorjS3ID)))
	}
	if len(s.StorjS3Secret) < MinStorjS3SecretLen {
		return apperrors.NewSecretsError("validate", fmt.Errorf("storj_s3_secret must be at least %d characters (was %d)", MinStorjS3SecretLen, len(s.StorjS3Secret)))
	}
	return nil
}

// MaskedID returns the last 4 characters of the S3 ID, prefixed with ****.
func (s *Secrets) MaskedID() string {
	if len(s.StorjS3ID) < 4 {
		return "****"
	}
	return "****" + s.StorjS3ID[len(s.StorjS3ID)-4:]
}

// MaskedSecret returns the last 4 characters of the S3 secret, prefixed with ****.
func (s *Secrets) MaskedSecret() string {
	if len(s.StorjS3Secret) < 4 {
		return "****"
	}
	return "****" + s.StorjS3Secret[len(s.StorjS3Secret)-4:]
}
