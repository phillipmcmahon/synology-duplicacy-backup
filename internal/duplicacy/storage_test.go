package duplicacy

import (
	"strings"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

func TestStorageSpecClassifiesDuplicacyBackends(t *testing.T) {
	tests := []struct {
		name     string
		storage  string
		scheme   string
		local    bool
		keys     []string
		fixPerms bool
	}{
		{name: "local path", storage: "/volumeUSB1/usbshare/duplicacy/homes", scheme: "local", local: true, fixPerms: true},
		{name: "s3", storage: "s3://EU@gateway.storjshare.io/bucket/homes", scheme: "s3", keys: []string{"s3_id", "s3_secret"}},
		{name: "s3-compatible v2", storage: "s3c://garage.local/bucket/homes", scheme: "s3c", keys: []string{"s3_id", "s3_secret"}},
		{name: "minio", storage: "minio://garage@192.168.202.24:3900/garage/homes", scheme: "minio", keys: []string{"s3_id", "s3_secret"}},
		{name: "minio tls", storage: "minios://garage@storage.example.test/garage/homes", scheme: "minios", keys: []string{"s3_id", "s3_secret"}},
		{name: "storj", storage: "storj://bucket/homes", scheme: "storj", keys: []string{"storj_key", "storj_passphrase"}},
		{name: "b2", storage: "b2://bucket/homes", scheme: "b2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewStorageSpec(tt.storage)
			if spec.Scheme() != tt.scheme {
				t.Fatalf("Scheme() = %q, want %q", spec.Scheme(), tt.scheme)
			}
			if spec.IsLocalPath() != tt.local {
				t.Fatalf("IsLocalPath() = %t, want %t", spec.IsLocalPath(), tt.local)
			}
			if spec.SupportsFixPerms() != tt.fixPerms {
				t.Fatalf("SupportsFixPerms() = %t, want %t", spec.SupportsFixPerms(), tt.fixPerms)
			}
			if strings.Join(spec.RequiredSecretKeys(), ",") != strings.Join(tt.keys, ",") {
				t.Fatalf("RequiredSecretKeys() = %v, want %v", spec.RequiredSecretKeys(), tt.keys)
			}
		})
	}
}

func TestStorageSpecValidateSecrets(t *testing.T) {
	spec := NewStorageSpec("s3://gateway.example.invalid/bucket/homes")
	if err := spec.ValidateSecrets(&secrets.Secrets{Keys: map[string]string{"s3_id": "id", "s3_secret": "secret"}}); err != nil {
		t.Fatalf("ValidateSecrets() unexpected error: %v", err)
	}

	err := spec.ValidateSecrets(&secrets.Secrets{Keys: map[string]string{"s3_id": "id"}})
	if err == nil || !strings.Contains(err.Error(), "s3_id and s3_secret") {
		t.Fatalf("ValidateSecrets() error = %v, want missing key message", err)
	}
}

func TestStorageSpecValidateForConfig(t *testing.T) {
	local := t.TempDir()
	tests := []struct {
		name    string
		storage string
		status  string
		wantErr string
	}{
		{name: "local writable directory", storage: local, status: "Writable"},
		{name: "local missing child with writable parent", storage: local + "/new-repo", status: "Writable"},
		{name: "remote URL-like storage", storage: "s3://gateway.example.invalid/bucket/homes", status: "Resolved"},
		{name: "empty", storage: "", wantErr: "storage must not be empty"},
		{name: "relative local path", storage: "relative/path", wantErr: "absolute path"},
		{name: "missing scheme target", storage: "s3:", wantErr: "must include a target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := NewStorageSpec(tt.storage).ValidateForConfig()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ValidateForConfig() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateForConfig() unexpected error: %v", err)
			}
			if status != tt.status {
				t.Fatalf("ValidateForConfig() status = %q, want %q", status, tt.status)
			}
		})
	}
}
