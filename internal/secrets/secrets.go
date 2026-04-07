// Package secrets handles loading and validating per-repository secret files
// for remote backup operations (e.g., Storj S3 credentials).
package secrets

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

// LoadSecretsFile loads and validates a secrets .env file.
func LoadSecretsFile(path string) (*Secrets, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("secrets file not found: %s", path)
		}
		return nil, fmt.Errorf("cannot stat secrets file: %w", err)
	}

	// Check permissions (600)
	perm := info.Mode().Perm()
	if perm != 0600 {
		return nil, fmt.Errorf("secrets file permissions are %04o, expected 0600: %s", perm, path)
	}

	// Check ownership (root:root)
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if stat.Uid != 0 || stat.Gid != 0 {
			return nil, fmt.Errorf("secrets file ownership is %d:%d, expected 0:0 (root:root): %s", stat.Uid, stat.Gid, path)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("secrets file is not readable: %s", path)
	}
	defer f.Close()

	values := make(map[string]string)
	lineno := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineno++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if !strings.Contains(line, "=") {
			return nil, fmt.Errorf("secrets file has invalid line at %d: %s", lineno, line)
		}

		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, fmt.Errorf("secrets file has malformed key=value pair with missing key at line %d", lineno)
		}

		if !allowedSecretKeys[key] {
			return nil, fmt.Errorf("unexpected key '%s' in secrets file at line %d", key, lineno)
		}

		// Strip surrounding quotes
		if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = value[1 : len(value)-1]
		}

		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading secrets file: %w", err)
	}

	s := &Secrets{
		StorjS3ID:     values["STORJ_S3_ID"],
		StorjS3Secret: values["STORJ_S3_SECRET"],
	}

	if s.StorjS3ID == "" {
		return nil, fmt.Errorf("required secret 'STORJ_S3_ID' is missing after loading %s", path)
	}
	if s.StorjS3Secret == "" {
		return nil, fmt.Errorf("required secret 'STORJ_S3_SECRET' is missing after loading %s", path)
	}

	return s, nil
}

// Validate checks minimum length requirements for secrets.
func (s *Secrets) Validate() error {
	if len(s.StorjS3ID) < MinStorjS3IDLen {
		return fmt.Errorf("STORJ_S3_ID must be at least %d characters (was %d)", MinStorjS3IDLen, len(s.StorjS3ID))
	}
	if len(s.StorjS3Secret) < MinStorjS3SecretLen {
		return fmt.Errorf("STORJ_S3_SECRET must be at least %d characters (was %d)", MinStorjS3SecretLen, len(s.StorjS3Secret))
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
