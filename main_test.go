package main

import (
	"testing"
)

// 4.4-UNIT-005 [P2]: extractPDFPaths extracts .pdf paths from args.
//
// AC#2: Given a second instance is launched with args containing PDF paths,
// When extractPDFPaths parses the args,
// Then it returns only the .pdf arguments (case-insensitive extension).
func TestExtractPDFPaths(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "single pdf arg",
			args: []string{"unidoc-pdf-debugger", "/path/to/file.pdf"},
			want: []string{"/path/to/file.pdf"},
		},
		{
			name: "empty args slice",
			args: []string{},
			want: nil,
		},
		{
			name: "nil args slice",
			args: nil,
			want: nil,
		},
		{
			name: "no args beyond binary name",
			args: []string{"unidoc-pdf-debugger"},
			want: nil,
		},
		{
			name: "non-pdf arg",
			args: []string{"unidoc-pdf-debugger", "/path/to/file.txt"},
			want: nil,
		},
		{
			name: "mixed extensions including uppercase PDF",
			args: []string{"unidoc-pdf-debugger", "/a.pdf", "/b.PDF", "/c.txt"},
			want: []string{"/a.pdf", "/b.PDF"},
		},
		{
			name: "multiple pdfs",
			args: []string{"unidoc-pdf-debugger", "/doc1.pdf", "/doc2.pdf"},
			want: []string{"/doc1.pdf", "/doc2.pdf"},
		},
		{
			name: "mixed case extension",
			args: []string{"unidoc-pdf-debugger", "/file.Pdf"},
			want: []string{"/file.Pdf"},
		},
		{
			name: "path with spaces",
			args: []string{"unidoc-pdf-debugger", "/my docs/my file.pdf"},
			want: []string{"/my docs/my file.pdf"},
		},
		{
			name: "arg with no extension",
			args: []string{"unidoc-pdf-debugger", "noext"},
			want: nil,
		},
		{
			name: "bare .pdf extension only",
			args: []string{"unidoc-pdf-debugger", ".pdf"},
			want: []string{".pdf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPDFPaths(tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("extractPDFPaths(%v) = %v, want %v", tt.args, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractPDFPaths(%v)[%d] = %q, want %q", tt.args, i, got[i], tt.want[i])
				}
			}
		})
	}
}
