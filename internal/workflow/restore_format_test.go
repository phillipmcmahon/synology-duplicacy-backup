package workflow

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestMarshalRestoreJSONAndShellQuote(t *testing.T) {
	jsonText, err := marshalRestoreJSON(map[string]string{"path": "docs/readme.md"})
	if err != nil {
		t.Fatalf("marshalRestoreJSON() error = %v", err)
	}
	if !strings.HasSuffix(jsonText, "\n") || !strings.Contains(jsonText, `"path": "docs/readme.md"`) {
		t.Fatalf("jsonText = %q", jsonText)
	}
	if _, err := marshalRestoreJSON(make(chan int)); err == nil {
		t.Fatal("expected marshalRestoreJSON to reject unsupported values")
	}

	if got := shellQuote(""); got != "''" {
		t.Fatalf("shellQuote(empty) = %q", got)
	}
	if got := shellQuote("simple/path"); got != "'simple/path'" {
		t.Fatalf("shellQuote(simple) = %q", got)
	}
	if got := shellQuote("path/with'quote"); got != "'path/with'\\''quote'" {
		t.Fatalf("shellQuote(quote) = %q", got)
	}
}

func TestConfirmRestoreRun(t *testing.T) {
	report := &restoreRunReport{Revision: 8, Workspace: "/volume1/restore-drills/homes-onsite-usb-20260425-130000-rev8"}

	if ok, err := confirmRestoreRun(Runtime{StdinIsTTY: func() bool { return false }}, report); err == nil || ok {
		t.Fatalf("non-interactive confirm = %t, %v", ok, err)
	}

	for _, tc := range []struct {
		name    string
		input   string
		wantOK  bool
		wantErr error
	}{
		{name: "yes", input: "yes\n", wantOK: true},
		{name: "no", input: "n\n", wantOK: false},
		{name: "cancel", input: "q\n", wantErr: ErrRestoreCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdin := restoreTempStdin(t, tc.input)
			rt := Runtime{
				Stdin:      func() *os.File { return stdin },
				StdinIsTTY: func() bool { return true },
			}
			ok, err := confirmRestoreRun(rt, report)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if ok != tc.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tc.wantOK)
			}
		})
	}
}

func restoreTempStdin(t *testing.T, input string) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	if _, err := f.WriteString(input); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
