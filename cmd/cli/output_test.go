package main

import (
	"strings"
	"testing"
)

// TestKVWriter_AlignsColon verifies the colon column is aligned across rows
// (the value column starts at the same offset for every key/value row).
func TestKVWriter_AlignsColon(t *testing.T) {
	var w kvWriter
	w.Add("MediaBox", "0 0 612 792")
	w.Add("Rotate", "90")
	var b strings.Builder
	if err := w.Render(&b); err != nil {
		t.Fatalf("Render: %v", err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), b.String())
	}
	// The value column offset must match: index of the value start after the
	// "key: " prefix is len(widest key)+2 for both rows.
	col0 := strings.Index(lines[0], "0 0 612 792")
	col1 := strings.Index(lines[1], "90")
	if col0 != col1 {
		t.Errorf("value columns not aligned: %d vs %d\n%s", col0, col1, b.String())
	}
	if !strings.HasSuffix(b.String(), "\n") {
		t.Errorf("output must end with a trailing newline")
	}
}

// TestTableWriter_HeaderAndRows verifies the header row plus aligned data
// columns, and that a zero-row table still emits the header.
func TestTableWriter_HeaderAndRows(t *testing.T) {
	tbl := newTable("OBJ", "GEN", "STATUS")
	tbl.AddRow("1", "0", "in-use")
	tbl.AddRow("10", "0", "free")
	var b strings.Builder
	if err := tbl.Render(&b); err != nil {
		t.Fatalf("Render: %v", err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows, got %d: %q", len(lines), b.String())
	}
	if !strings.HasPrefix(lines[0], "OBJ") {
		t.Errorf("first line should be the header, got %q", lines[0])
	}
	// Empty table emits only the header.
	empty := newTable("OBJ", "GEN")
	var eb strings.Builder
	if err := empty.Render(&eb); err != nil {
		t.Fatalf("Render empty: %v", err)
	}
	if strings.Count(strings.TrimRight(eb.String(), "\n"), "\n") != 0 {
		t.Errorf("empty table should emit exactly one (header) line, got %q", eb.String())
	}
}

// TestHumanizeBytes spot-checks the byte humanizer across unit boundaries.
func TestHumanizeBytes(t *testing.T) {
	cases := map[int64]string{
		0:        "0 B",
		512:      "512 B",
		1024:     "1.0 KB",
		1536:     "1.5 KB",
		1048576:  "1.0 MB",
	}
	for n, want := range cases {
		if got := humanizeBytes(n); got != want {
			t.Errorf("humanizeBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
