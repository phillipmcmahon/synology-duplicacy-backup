package workflow

import (
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
)

func TestLocalRepositoryRequiresSudoOnlyForRootProtectedLocalRepositories(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		uid  int
		want bool
	}{
		{
			name: "local path non-root",
			cfg:  &config.Config{Location: locationLocal, Storage: "/volumeUSB1/usbshare/duplicacy/homes"},
			uid:  1000,
			want: true,
		},
		{
			name: "local path root",
			cfg:  &config.Config{Location: locationLocal, Storage: "/volumeUSB1/usbshare/duplicacy/homes"},
			uid:  0,
		},
		{
			name: "remote mounted path non-root",
			cfg:  &config.Config{Location: locationRemote, Storage: "/volume1/duplicacy/usbshare2/homes"},
			uid:  1000,
		},
		{
			name: "remote object non-root",
			cfg:  &config.Config{Location: locationRemote, Storage: "s3://gateway.example.invalid/bucket/homes"},
			uid:  1000,
		},
		{
			name: "local object non-root",
			cfg:  &config.Config{Location: locationLocal, Storage: "s3://rustfs.local/bucket/homes"},
			uid:  1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := testRuntime()
			rt.Geteuid = func() int { return tt.uid }
			if got := localRepositoryRequiresSudo(tt.cfg, rt); got != tt.want {
				t.Fatalf("localRepositoryRequiresSudo() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRestoreStorageRequiresSudoMatchesLocationAndPathStorage(t *testing.T) {
	tests := []struct {
		name     string
		location string
		storage  string
		want     bool
	}{
		{name: "local filesystem repository", location: locationLocal, storage: "/volumeUSB1/usbshare/duplicacy/homes", want: true},
		{name: "remote mounted filesystem repository", location: locationRemote, storage: "/volume1/duplicacy/usbshare2/homes"},
		{name: "remote object repository", location: locationRemote, storage: "s3://gateway.example.invalid/bucket/homes"},
		{name: "local object repository", location: locationLocal, storage: "s3://rustfs.local/bucket/homes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &Plan{Config: PlanConfig{Location: tt.location}}
			if got := restoreStorageRequiresSudo(plan, tt.storage); got != tt.want {
				t.Fatalf("restoreStorageRequiresSudo() = %t, want %t", got, tt.want)
			}
		})
	}
}
