package restore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	locationLocal  = "local"
	locationRemote = "remote"
)

func testRuntime() Env {
	rt := DefaultEnv()
	rt.Geteuid = func() int { return 0 }
	rt.LookPath = func(string) (string, error) { return "/usr/bin/true", nil }
	rt.Now = func() time.Time { return time.Date(2026, 4, 9, 18, 0, 0, 0, time.UTC) }
	rt.TempDir = func() string { return os.TempDir() }
	rt.Getpid = func() int { return 4242 }
	return rt
}

func captureHealthOutput(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(stderr) error = %v", err)
	}
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stderr = oldStderr
	output := <-done
	_ = r.Close()
	_ = w.Close()

	return output
}

func saveRunState(meta Metadata, label, target string, state *RunState) error {
	if state == nil {
		return nil
	}
	if err := os.MkdirAll(meta.StateDir, 0700); err != nil {
		return err
	}
	state.Label = label
	state.Storage = target
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(StateFilePath(meta, label, target), body, 0600)
}

func writeTargetTestConfig(t *testing.T, dir, label, target, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-backup.toml", label))
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func writeTargetTestSecrets(t *testing.T, dir, label, target string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-secrets.toml", label))
	body := fmt.Sprintf("[storage.%s.keys]\ns3_id = \"ABCDEFGHIJKLMNOPQRSTUVWXYZ01\"\ns3_secret = \"abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR\"\n", target)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	if os.Getuid() == 0 {
		if err := os.Chown(path, 0, 0); err != nil {
			t.Fatalf("Chown(%q) error = %v", path, err)
		}
	}
	return path
}

func localTargetConfig(label, sourcePath, storageRoot string, threads int, prune string, extraSections ...string) string {
	return buildTargetConfig(label, "onsite-usb", "local", sourcePath, filepath.Join(storageRoot, label), threads, prune, extraSections...)
}

func remoteTargetConfig(label, sourcePath, storage string, threads int, prune string, extraSections ...string) string {
	return buildTargetConfig(label, "offsite-storj", "remote", sourcePath, storage, threads, prune, extraSections...)
}

func buildTargetConfig(label, target, location, sourcePath, storage string, threads int, prune string, extraSections ...string) string {
	return fmt.Sprintf(`label = %q
source_path = %q

[common]
threads = %d
prune = %q

[storage.%s]
location = %q
storage = %q
%s
`, label, sourcePath, threads, prune, target, location, storage, strings.Join(extraSections, "\n"))
}
