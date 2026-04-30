package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTargetTestConfig(t *testing.T, dir, label, target, body string) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("%s-backup.toml", label))
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeTargetTestSecrets(t *testing.T, dir, label, target string) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("%s-secrets.toml", label))
	body := fmt.Sprintf("[storage.%s.keys]\ns3_id = \"ABCDEFGHIJKLMNOPQRSTUVWXYZ01\"\ns3_secret = \"abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR\"\n", target)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if os.Getuid() == 0 {
		if err := os.Chown(path, 0, 0); err != nil {
			t.Fatalf("Chown() error = %v", err)
		}
	}
	return path
}

func localTargetConfig(label, sourcePath, storageRoot string, threads int, prune string, extraSections ...string) string {
	return buildLabelConfig(label, "onsite-usb", locationLocal, sourcePath, filepath.Join(storageRoot, label), threads, prune, extraSections...)
}

func remoteTargetConfig(label, sourcePath, storage string, threads int, prune string, extraSections ...string) string {
	return buildLabelConfig(label, "offsite-storj", locationRemote, sourcePath, storage, threads, prune, extraSections...)
}

func localDuplicacyTargetConfig(label, sourcePath, storage string, threads int, prune string, extraSections ...string) string {
	return buildLabelConfig(label, "onsite-rustfs", locationLocal, sourcePath, storage, threads, prune, extraSections...)
}

func buildTargetConfig(label, target, location, sourcePath, storage string, threads int, prune string, extraSections ...string) string {
	return buildLabelConfig(label, target, location, sourcePath, storage, threads, prune, extraSections...)
}

func buildLabelConfig(label, target, location, sourcePath, storage string, threads int, prune string, extraSections ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "label = %q\n", label)
	fmt.Fprintf(&b, "source_path = %q\n", sourcePath)

	if threads > 0 || prune != "" {
		b.WriteString("\n[common]\n")
		if threads > 0 {
			fmt.Fprintf(&b, "threads = %d\n", threads)
		}
		if prune != "" {
			fmt.Fprintf(&b, "prune = %q\n", prune)
		}
	}

	fmt.Fprintf(&b, "\n[storage.%s]\n", target)
	fmt.Fprintf(&b, "location = %q\n", location)
	fmt.Fprintf(&b, "storage = %q\n", storage)

	for _, extra := range extraSections {
		extra = strings.TrimSpace(extra)
		if extra == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(extra)
		b.WriteString("\n")
	}

	return b.String()
}
