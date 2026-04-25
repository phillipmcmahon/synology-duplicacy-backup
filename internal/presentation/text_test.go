package presentation

import "testing"

func TestTitleCapitalizesOperatorLabels(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "restore workspace", want: "Restore Workspace"},
		{input: "restore-workspace", want: "Restore-Workspace"},
		{input: "restore_workspace", want: "Restore_Workspace"},
		{input: "  already Title  ", want: "Already Title"},
		{input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := Title(tt.input); got != tt.want {
				t.Fatalf("Title(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDisplayEmptyUsesFallbackOnlyForBlankValues(t *testing.T) {
	if got := DisplayEmpty(" value ", "fallback"); got != " value " {
		t.Fatalf("DisplayEmpty(non-empty) = %q, want original", got)
	}
	if got := DisplayEmpty(" \t\n", "fallback"); got != "fallback" {
		t.Fatalf("DisplayEmpty(blank) = %q, want fallback", got)
	}
}
