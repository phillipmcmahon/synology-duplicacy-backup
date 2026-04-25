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
