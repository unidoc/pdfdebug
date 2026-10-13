package main

import (
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// Diff summaries carry decoded text, so a PDF string holding a newline would
// split one path across two lines and break the indentation the plain-text
// delta uses to carry the hierarchy. Every branch that prints a summary has to
// escape it, so all three statuses are walked here.

func TestWriteDiffLines_SummariesAreEscapedSoOneNodeIsOneLine(t *testing.T) {
	root := &pdfcore.DiffNode{
		Path:   "/Root",
		Status: "changed",
		Kind:   "dict",
		Children: []*pdfcore.DiffNode{
			{
				Path:         "/Root/StructTreeRoot/Alt",
				Status:       "changed",
				Kind:         "scalar",
				LeftSummary:  "left\nline",
				RightSummary: "right\tcell",
			},
			{
				Path:         "/Root/StructTreeRoot/E",
				Status:       "added",
				Kind:         "scalar",
				RightSummary: "don\u0092t",
			},
			{
				Path:        "/Root/StructTreeRoot/ActualText",
				Status:      "removed",
				Kind:        "scalar",
				LeftSummary: "gone\rhere",
			},
		},
	}

	var b strings.Builder
	writeDiffLines(&b, root, 0, false)
	out := b.String()

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Errorf("rendered %d lines, want one per node (4)\n--- output ---\n%s", len(lines), out)
	}
	for _, want := range []string{
		`~ /Root/StructTreeRoot/Alt  left\nline -> right\tcell`,
		`+ /Root/StructTreeRoot/E  don\x92t`,
		`- /Root/StructTreeRoot/ActualText  gone\rhere`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output has no row %q\n--- output ---\n%s", want, out)
		}
	}
	for _, r := range out {
		if r == '\n' {
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			t.Errorf("raw control character %U reached the output\n--- output ---\n%s", r, out)
		}
	}
}
