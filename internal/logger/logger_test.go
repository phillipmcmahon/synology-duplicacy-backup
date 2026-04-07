package logger

import (
        "os"
        "testing"
)

func TestStripColour(t *testing.T) {
        tests := []struct {
                name  string
                input string
                want  string
        }{
                {"plain text", "hello world", "hello world"},
                {"bold red", "\033[1;31mERROR\033[0m", "ERROR"},
                {"cyan info", "\033[0;36m[INFO] message\033[0m", "[INFO] message"},
                {"dim gray", "\033[2;37mDEBUG\033[0m", "DEBUG"},
                {"multiple codes", "\033[1;33mWARN\033[0m: \033[1;31mfailed\033[0m", "WARN: failed"},
                {"empty string", "", ""},
                {"no codes", "just text", "just text"},
        }
        for _, tt := range tests {
                t.Run(tt.name, func(t *testing.T) {
                        got := StripColour(tt.input)
                        if got != tt.want {
                                t.Errorf("StripColour(%q) = %q, want %q", tt.input, got, tt.want)
                        }
                })
        }
}

func TestIsTerminal(t *testing.T) {
        // A pipe is definitely not a terminal.
        r, w, err := os.Pipe()
        if err != nil {
                t.Fatalf("failed to create pipe: %v", err)
        }
        defer r.Close()
        defer w.Close()

        if IsTerminal(r) {
                t.Error("IsTerminal(pipe) = true, want false")
        }

        // nil file should return false
        if IsTerminal(nil) {
                t.Error("IsTerminal(nil) = true, want false")
        }
}

func TestLevelString(t *testing.T) {
        tests := []struct {
                level Level
                want  string
        }{
                {DEBUG, "DEBUG"},
                {INFO, "INFO"},
                {SUCCESS, "SUCCESS"},
                {WARNING, "WARNING"},
                {ERROR, "ERROR"},
                {Level(99), "UNKNOWN"},
        }
        for _, tt := range tests {
                t.Run(tt.want, func(t *testing.T) {
                        if got := tt.level.String(); got != tt.want {
                                t.Errorf("Level(%d).String() = %q, want %q", tt.level, got, tt.want)
                        }
                })
        }
}
