package duplicacy

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

// StorageSpec describes the Duplicacy storage string from configuration.
//
// The backup tool does not model storage backends itself. It passes this value
// to Duplicacy and only understands enough about the string to validate local
// path access and determine whether well-known Duplicacy backend keys are
// required in the secrets file.
type StorageSpec struct {
	Raw string
}

func NewStorageSpec(raw string) StorageSpec {
	return StorageSpec{Raw: strings.TrimSpace(raw)}
}

func (s StorageSpec) Value() string {
	return s.Raw
}

// Scheme returns the Duplicacy URL scheme used to identify well-known storage
// backends. Local paths and unparsable values fall back to "local" here; full
// config validation still reports malformed storage strings through
// ValidateForConfig.
func (s StorageSpec) Scheme() string {
	parsed, err := url.Parse(s.Raw)
	if err != nil || parsed.Scheme == "" {
		return "local"
	}
	return strings.ToLower(parsed.Scheme)
}

// IsLocalPath reports whether Duplicacy storage is accessed through an OS
// filesystem path rather than a URL-style backend. Bare absolute paths and
// file:// URLs include local disks as well as mounted USB, NFS, SMB, or FUSE
// paths. Target location still decides the security posture: a path can be a
// remote mounted repository whose access is governed by mount credentials.
func (s StorageSpec) IsLocalPath() bool {
	if s.Raw == "" {
		return false
	}
	parsed, err := url.Parse(s.Raw)
	if err != nil || parsed.Scheme == "" {
		return !strings.Contains(s.Raw, "://")
	}
	return strings.EqualFold(parsed.Scheme, "file")
}

func (s StorageSpec) RequiredSecretKeys() []string {
	switch s.Scheme() {
	case "s3", "s3c", "minio", "minios":
		return []string{"s3_id", "s3_secret"}
	case "storj":
		return []string{"storj_key", "storj_passphrase"}
	default:
		return nil
	}
}

func (s StorageSpec) NeedsSecrets() bool {
	return len(s.RequiredSecretKeys()) > 0
}

func (s StorageSpec) ValidateSecrets(sec *secrets.Secrets) error {
	required := s.RequiredSecretKeys()
	for _, key := range required {
		if sec == nil || strings.TrimSpace(sec.Keys[key]) == "" {
			return fmt.Errorf("storage %q requires %s in [targets.<name>.keys]", s.Scheme(), strings.Join(required, " and "))
		}
	}
	return nil
}

func (s StorageSpec) ValidateForConfig() (string, error) {
	if s.Raw == "" {
		return "", fmt.Errorf("storage must not be empty")
	}
	parsed, err := url.Parse(s.Raw)
	if err != nil {
		return "", fmt.Errorf("duplicacy storage is not a valid URL-like storage target: %v", err)
	}
	if parsed.Scheme == "" {
		return validateLocalStorage(s.Raw)
	}
	if parsed.Host == "" && parsed.Path == "" {
		return "", fmt.Errorf("duplicacy storage must include a target after the scheme (was %q)", s.Raw)
	}
	return "Resolved", nil
}

func validateLocalStorage(storage string) (string, error) {
	if !filepath.IsAbs(storage) {
		return "", fmt.Errorf("duplicacy local storage must be an absolute path or a URL-like storage target (was %q)", storage)
	}
	info, err := os.Stat(storage)
	if err != nil {
		if os.IsNotExist(err) {
			parent := filepath.Dir(storage)
			if _, parentErr := os.Stat(parent); parentErr != nil {
				if os.IsNotExist(parentErr) {
					return "", fmt.Errorf("duplicacy local storage parent does not exist: %s", parent)
				}
				return "", fmt.Errorf("duplicacy local storage parent is not accessible: %v", parentErr)
			}
			return validateWritableStorageDirectory(parent, "duplicacy local storage parent")
		}
		return "", fmt.Errorf("duplicacy local storage is not accessible: %v", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("duplicacy local storage must be a directory: %s", storage)
	}
	return validateWritableStorageDirectory(storage, "duplicacy local storage")
}

func validateWritableStorageDirectory(path, description string) (string, error) {
	probe, err := os.CreateTemp(path, ".duplicacy-backup-config-validate-*")
	if err != nil {
		return "", fmt.Errorf("%s is not writable: %s", description, path)
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return "Writable", nil
}
