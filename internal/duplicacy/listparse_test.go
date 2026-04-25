package duplicacy

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseListFilesOutputSupportsDuplicacyRowsPlainRowsAndSummaries(t *testing.T) {
	input := strings.NewReader(`
Repository set to /restore/workspace
Storage set to /backups/homes
5585354 2026-04-20 19:29:38 45fcaf55f07a698bd608e892802bd3f7275a8688374de79acbc5ebb078ebdc06 phillipmcmahon/code/archive/v5.0.0/a file.tar.gz
1234 2026-04-20 19:29 45fcaf55 phillipmcmahon/code/archive/v5.1.0/b.tar.gz
plain/path/without-metadata.txt
Files: 2471
Total size: 287254112235, file chunks: 6658, metadata chunks: 4
`)

	got, err := ParseListFilesOutput(input)
	if err != nil {
		t.Fatalf("ParseListFilesOutput() error = %v", err)
	}
	want := []string{
		"phillipmcmahon/code/archive/v5.0.0/a file.tar.gz",
		"phillipmcmahon/code/archive/v5.1.0/b.tar.gz",
		"plain/path/without-metadata.txt",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseListFilesOutput() = %#v, want %#v", got, want)
	}
}

func TestExtractListFilePathKeepsMalformedRowsAsPlainPaths(t *testing.T) {
	got := extractListFilePath("1234 not-a-date 19:29:38 45fcaf55 docs/readme.md")
	if got != "1234 not-a-date 19:29:38 45fcaf55 docs/readme.md" {
		t.Fatalf("extractListFilePath() = %q", got)
	}
}

func TestParseListFilesOutputIgnoresDuplicacyStatusSummaryAndDuplicateRows(t *testing.T) {
	input := strings.NewReader(`
Loaded 1 include/exclude pattern(s)
Parsing filter file /restore/.duplicacy/filters
Restoring /restore to revision 8
Downloaded chunk 1 size 25348363, 24.61MB/s 00:00:17 5.7%
Skipped 0 file, 0 bytes
Snapshot data revision 8 created at 2026-04-25 13:00 -hash
1234 2026-04-25 13:00 45fcaf55 phillipmcmahon/code/readme.md
1234 2026-04-25 13:00 45fcaf55 phillipmcmahon/code/readme.md
Restored /restore to revision 8
Total running time: 00:00:01
`)

	got, err := ParseListFilesOutput(input)
	if err != nil {
		t.Fatalf("ParseListFilesOutput() error = %v", err)
	}
	want := []string{"phillipmcmahon/code/readme.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseListFilesOutput() = %#v, want %#v", got, want)
	}
}

func TestLooksLikeTimeAcceptsMinuteAndSecondPrecision(t *testing.T) {
	for _, value := range []string{"13:00", "13:00:59"} {
		if !looksLikeTime(value) {
			t.Fatalf("looksLikeTime(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"13", "13-00", "13:0x", "13:00:5x"} {
		if looksLikeTime(value) {
			t.Fatalf("looksLikeTime(%q) = true, want false", value)
		}
	}
}

func TestLooksLikeHexDigestRequiresHexCharacters(t *testing.T) {
	if !looksLikeHexDigest("45fcaf55") {
		t.Fatalf("looksLikeHexDigest() rejected valid short digest")
	}
	for _, value := range []string{"45fcaf5", "45fcaf5x"} {
		if looksLikeHexDigest(value) {
			t.Fatalf("looksLikeHexDigest(%q) = true, want false", value)
		}
	}
}
