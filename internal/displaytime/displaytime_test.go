package displaytime

import (
	"errors"
	"testing"
	"time"
)

func TestLocationUsesProcessLocalWhenSystemLocaltimeUnavailable(t *testing.T) {
	withLocalTimeZone(t, time.FixedZone("BST", 3600))
	withNoLocaltimeLink(t)

	got := time.Date(2026, 5, 7, 16, 5, 25, 0, time.UTC).In(Location()).Format("2006-01-02 15:04:05 MST")
	if got != "2026-05-07 17:05:25 BST" {
		t.Fatalf("Location() formatted time = %q", got)
	}
}

func TestLocationPrefersSystemLocaltimeLink(t *testing.T) {
	withLocalTimeZone(t, time.FixedZone("IST", 3600))
	withLocaltimeLink(t, "/usr/share/zoneinfo/Europe/London")

	got := time.Date(2026, 5, 7, 16, 5, 25, 0, time.UTC).In(Location()).Format("2006-01-02 15:04:05 MST")
	if got != "2026-05-07 17:05:25 BST" {
		t.Fatalf("Location() formatted time = %q", got)
	}
}

func TestLocationFallsBackWhenSystemLocaltimeCannotResolve(t *testing.T) {
	tests := []string{
		"/not/zoneinfo/path",
		"/usr/share/zoneinfo/Europe/Missing",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			withLocalTimeZone(t, time.FixedZone("LOCAL", 2*3600))
			withBrokenLocaltimeLocation(t, target)

			got := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC).In(Location()).Format("2006-01-02 15:04:05 MST")
			if got != "2026-05-07 12:00:00 LOCAL" {
				t.Fatalf("Location() formatted time = %q", got)
			}
		})
	}
}

func withLocalTimeZone(t *testing.T, location *time.Location) {
	t.Helper()
	original := time.Local
	time.Local = location
	t.Cleanup(func() {
		time.Local = original
	})
}

func withLocaltimeLink(t *testing.T, target string) {
	t.Helper()
	originalReadlink := readLocaltimeLink
	originalLoadLocation := loadTimeLocation
	readLocaltimeLink = func(path string) (string, error) {
		return target, nil
	}
	loadTimeLocation = time.LoadLocation
	t.Cleanup(func() {
		readLocaltimeLink = originalReadlink
		loadTimeLocation = originalLoadLocation
	})
}

func withNoLocaltimeLink(t *testing.T) {
	t.Helper()
	originalReadlink := readLocaltimeLink
	readLocaltimeLink = func(path string) (string, error) {
		return "", errors.New("no localtime link")
	}
	t.Cleanup(func() {
		readLocaltimeLink = originalReadlink
	})
}

func withBrokenLocaltimeLocation(t *testing.T, target string) {
	t.Helper()
	originalReadlink := readLocaltimeLink
	originalLoadLocation := loadTimeLocation
	readLocaltimeLink = func(path string) (string, error) {
		return target, nil
	}
	loadTimeLocation = func(name string) (*time.Location, error) {
		return nil, errors.New("missing zone")
	}
	t.Cleanup(func() {
		readLocaltimeLink = originalReadlink
		loadTimeLocation = originalLoadLocation
	})
}
