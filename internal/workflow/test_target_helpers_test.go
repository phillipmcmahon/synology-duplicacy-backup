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
	path := filepath.Join(dir, fmt.Sprintf("%s-%s-backup.toml", label, target))
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeTargetTestSecrets(t *testing.T, dir, label, target string) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("duplicacy-%s-%s.toml", label, target))
	body := "storj_s3_id = \"ABCDEFGHIJKLMNOPQRSTUVWXYZ01\"\nstorj_s3_secret = \"abcdefghijklmnopqrstuvwxyz01234567890ABCDEFGHIJKLMNOPQR\"\n"
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

func localTargetConfig(label, sourcePath, destination, owner, group string, threads int, prune string, extraSections ...string) string {
	return buildTargetConfig(label, "local", "local", sourcePath, destination, label, owner, group, threads, prune, extraSections...)
}

func remoteTargetConfig(label, sourcePath, destination string, threads int, prune string, extraSections ...string) string {
	return buildTargetConfig(label, "remote", "remote", sourcePath, destination, label, "", "", threads, prune, extraSections...)
}

func buildTargetConfig(label, target, targetType, sourcePath, destination, repository, owner, group string, threads int, prune string, extraSections ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "label = %q\n", label)
	fmt.Fprintf(&b, "source_path = %q\n\n", sourcePath)
	fmt.Fprintf(&b, "[target]\nname = %q\ntype = %q\n", target, targetType)
	if targetType == targetLocal {
		if owner != "" || group != "" {
			b.WriteString("allow_local_accounts = true\n")
		} else {
			b.WriteString("allow_local_accounts = false\n")
		}
		if owner != "" {
			fmt.Fprintf(&b, "local_owner = %q\n", owner)
		}
		if group != "" {
			fmt.Fprintf(&b, "local_group = %q\n", group)
		}
	} else {
		b.WriteString("requires_network = true\n")
	}

	fmt.Fprintf(&b, "\n[storage]\ndestination = %q\n", destination)
	if repository != "" {
		fmt.Fprintf(&b, "repository = %q\n", repository)
	}

	if threads > 0 {
		fmt.Fprintf(&b, "\n[capture]\nthreads = %d\n", threads)
	}

	if prune != "" {
		fmt.Fprintf(&b, "\n[retention]\nprune = %q\n", prune)
	}

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
