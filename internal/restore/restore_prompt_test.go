package restore

import (
	"bufio"
	"strings"
	"testing"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

func TestRestorePromptPureHelpers(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{" docs/readme.md ", "docs/readme.md"},
		{"docs/../photos/image.jpg", "photos/image.jpg"},
		{"", ""},
	} {
		got, err := cleanRestorePath(tc.input)
		if err != nil {
			t.Fatalf("cleanRestorePath(%q) error = %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("cleanRestorePath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
	for _, bad := range []string{"/absolute/path", "../outside", "docs/\x00bad"} {
		if _, err := cleanRestorePath(bad); err == nil {
			t.Fatalf("cleanRestorePath(%q) should fail", bad)
		}
	}

	created := time.Date(2026, 4, 25, 13, 0, 0, 0, time.UTC)
	if got := formatRestorePointChoice(duplicacy.RevisionInfo{Revision: 8, CreatedAt: created}); got != "2026-04-25 13:00:00 | rev 8" {
		t.Fatalf("formatRestorePointChoice(created) = %q", got)
	}
	if got := formatRestorePointChoice(duplicacy.RevisionInfo{Revision: 8}); got != "rev 8" {
		t.Fatalf("formatRestorePointChoice(no time) = %q", got)
	}

	paths := []string{"home/docs/a.txt", "home/docs/sub/b.txt", "home/photos/c.jpg"}
	if root, isDir := restoreSelectionRoot(paths, "home/docs"); root != "home/docs" || !isDir {
		t.Fatalf("restoreSelectionRoot(dir) = %q, %t", root, isDir)
	}
	if root, isDir := restoreSelectionRoot(paths, "home/photos/c.jpg"); root != "home/photos/c.jpg" || isDir {
		t.Fatalf("restoreSelectionRoot(file) = %q, %t", root, isDir)
	}
	if root, isDir := restoreSelectionRoot(paths, "missing"); root != "missing" || !isDir {
		t.Fatalf("restoreSelectionRoot(missing) = %q, %t", root, isDir)
	}
	if got := restoreScopedDirectoryPattern("home/docs/"); got != "home/docs/*" {
		t.Fatalf("restoreScopedDirectoryPattern() = %q", got)
	}
	if got := restoreScopedDirectoryPattern(""); got != "" {
		t.Fatalf("restoreScopedDirectoryPattern(empty) = %q", got)
	}
	if !restorePathPrefixHasMatches(paths, "home/docs") || restorePathPrefixHasMatches(paths, "home/music") {
		t.Fatal("restorePathPrefixHasMatches returned unexpected result")
	}
}

func TestPromptRestoreRevisionAndIntent(t *testing.T) {
	revisions := []duplicacy.RevisionInfo{{Revision: 8}, {Revision: 7}, {Revision: 6}}
	var output strings.Builder
	deps := RestoreDeps{PromptOutput: &output}.withDefaults()

	revision, err := promptRestoreRevision(bufio.NewReader(strings.NewReader("2\n")), revisions, 2, deps)
	if err != nil {
		t.Fatalf("promptRestoreRevision(list choice) error = %v", err)
	}
	if revision.Revision != 7 {
		t.Fatalf("revision = %d", revision.Revision)
	}
	revision, err = promptRestoreRevision(bufio.NewReader(strings.NewReader("6\n")), revisions, 2, deps)
	if err != nil {
		t.Fatalf("promptRestoreRevision(id choice) error = %v", err)
	}
	if revision.Revision != 6 {
		t.Fatalf("revision = %d", revision.Revision)
	}
	if _, err := promptRestoreRevision(bufio.NewReader(strings.NewReader("q\n")), revisions, 2, deps); err != ErrRestoreCancelled {
		t.Fatalf("cancel err = %v", err)
	}
	if _, err := promptRestoreRevision(bufio.NewReader(strings.NewReader("bad\n")), revisions, 2, deps); err == nil {
		t.Fatal("expected invalid revision selection")
	}

	intent, err := promptRestoreSelectIntent(bufio.NewReader(strings.NewReader("inspect\n")), "", deps)
	if err != nil || intent != restoreSelectIntentInspect {
		t.Fatalf("inspect intent = %q, %v", intent, err)
	}
	intent, err = promptRestoreSelectIntent(bufio.NewReader(strings.NewReader("full\n")), "home/docs", deps)
	if err != nil || intent != restoreSelectIntentFull {
		t.Fatalf("full intent = %q, %v", intent, err)
	}
	intent, err = promptRestoreSelectIntent(bufio.NewReader(strings.NewReader("selective\n")), "", deps)
	if err != nil || intent != restoreSelectIntentSelective {
		t.Fatalf("selective intent = %q, %v", intent, err)
	}
	if _, err := promptRestoreSelectIntent(bufio.NewReader(strings.NewReader("q\n")), "", deps); err != ErrRestoreCancelled {
		t.Fatalf("cancel intent err = %v", err)
	}
	if _, err := promptRestoreSelectIntent(bufio.NewReader(strings.NewReader("9\n")), "", deps); err == nil {
		t.Fatal("expected invalid intent")
	}
}

func TestPromptRestoreYesNo(t *testing.T) {
	deps := RestoreDeps{PromptOutput: &strings.Builder{}}.withDefaults()
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"yes\n", true},
		{"\n", false},
		{"n\n", false},
	} {
		got, err := promptRestoreYesNo(bufio.NewReader(strings.NewReader(tc.input)), "Continue? [y/N]: ", deps)
		if err != nil {
			t.Fatalf("promptRestoreYesNo(%q) error = %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("promptRestoreYesNo(%q) = %t, want %t", tc.input, got, tc.want)
		}
	}
	if _, err := promptRestoreYesNo(bufio.NewReader(strings.NewReader("q\n")), "Continue? [y/N]: ", deps); err != ErrRestoreCancelled {
		t.Fatalf("cancel err = %v", err)
	}
}
