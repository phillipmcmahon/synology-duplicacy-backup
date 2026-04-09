// Package secrets handles loading and validating per-repository secret files
// for remote backup operations (e.g., Storj S3 credentials).
package secrets

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
)

// MinStorjS3IDLen is the minimum length for STORJ_S3_ID.
const MinStorjS3IDLen = 28

// MinStorjS3SecretLen is the minimum length for STORJ_S3_SECRET.
const MinStorjS3SecretLen = 53

// Secrets holds loaded secret values.
type Secrets struct {
	StorjS3ID     string
	StorjS3Secret string
}

var allowedSecretKeys = map[string]bool{
	"STORJ_S3_ID":     true,
	"STORJ_S3_SECRET": true,
}

// GetSecretsFilePath returns the expected secrets file path for a label.
func GetSecretsFilePath(secretsDir, prefix, label string) string {
	return filepath.Join(secretsDir, fmt.Sprintf("%s-%s.env", prefix, label))
}

// ValidateFileAccess checks that the secrets file at path exists, has 0600
// permissions, and is owned by root:root.  It is called by [LoadSecretsFile]
// before parsing and is separated so that parser logic can be tested
// independently of OS-level access controls.
func ValidateFileAccess(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return apperrors.NewSecretsError("stat", fmt.Errorf("secrets file not found: %s", path), "path", path)
		}
		return apperrors.NewSecretsError("stat", fmt.Errorf("cannot stat secrets file: %w", err), "path", path)
	}

	// Check permissions (600)
	perm := info.Mode().Perm()
	if perm != 0600 {
		return apperrors.NewSecretsError("permissions", fmt.Errorf("secrets file permissions are %04o, expected 0600: %s", perm, path), "path", path)
	}

	// Check ownership (root:root)
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if stat.Uid != 0 || stat.Gid != 0 {
			return apperrors.NewSecretsError("ownership", fmt.Errorf("secrets file ownership is %d:%d, expected 0:0 (root:root): %s", stat.Uid, stat.Gid, path), "path", path)
		}
	}
	return nil
}

// ParseSecrets reads key=value lines from r, validates their format and
// keys, and returns a populated [Secrets] struct.  The source parameter is
// used only for error messages (typically the file path).
//
// This function is separated from file access validation so that the parser
// can be unit-tested without requiring specific file ownership or permissions.
func ParseSecrets(r io.Reader, source string) (*Secrets, error) {
	values := make(map[string]string)
	lineno := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lineno++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if !strings.Contains(line, "=") {
			return nil, apperrors.NewSecretsError("parse", fmt.Errorf("secrets file has invalid line at %d: %s", lineno, line), "source", source)
		}

		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, apperrors.NewSecretsError("parse", fmt.Errorf("secrets file has malformed key=value pair with missing key at line %d", lineno), "source", source)
		}

		if !allowedSecretKeys[key] {
			return nil, apperrors.NewSecretsError("parse", fmt.Errorf("unexpected key '%s' in secrets file at line %d", key, lineno), "source", source)
		}

		// Strip surrounding quotes
		if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = value[1 : len(value)-1]
		}

		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, apperrors.NewSecretsError("read", fmt.Errorf("error reading secrets file: %w", err), "source", source)
	}

	s := &Secrets{
		StorjS3ID:     values["STORJ_S3_ID"],
		StorjS3Secret: values["STORJ_S3_SECRET"],
	}

	if s.StorjS3ID == "" {
		return nil, apperrors.NewSecretsError("required", fmt.Errorf("required secret 'STORJ_S3_ID' is missing after loading %s", source), "source", source)
	}
	if s.StorjS3Secret == "" {
		return nil, apperrors.NewSecretsError("required", fmt.Errorf("required secret 'STORJ_S3_SECRET' is missing after loading %s", source), "source", source)
	}

	return s, nil
}

// LoadSecretsFile loads and validates a secrets .env file.  It first checks
// file access (permissions and ownership) via [ValidateFileAccess], then
// delegates parsing to [ParseSecrets].
func LoadSecretsFile(path string) (*Secrets, error) {
	if err := ValidateFileAccess(path); err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, apperrors.NewSecretsError("open", fmt.Errorf("secrets file is not readable: %s", path), "path", path)
	}
	defer f.Close()

	return ParseSecrets(f, path)
}

// Validate checks minimum length requirements for secrets.
func (s *Secrets) Validate() error {
	if len(s.StorjS3ID) < MinStorjS3IDLen {
		return apperrors.NewSecretsError("validate", fmt.Errorf("STORJ_S3_ID must be at least %d characters (was %d)", MinStorjS3IDLen, len(s.StorjS3ID)))
	}
	if len(s.StorjS3Secret) < MinStorjS3SecretLen {
		return apperrors.NewSecretsError("validate", fmt.Errorf("STORJ_S3_SECRET must be at least %d characters (was %d)", MinStorjS3SecretLen, len(s.StorjS3Secret)))
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
