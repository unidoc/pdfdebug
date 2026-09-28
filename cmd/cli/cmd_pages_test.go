package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// pdfcpu refuses to open a document whose root /Count disagrees with its
// leaves, so these drive writePagesDump with a hand-built index instead.

// countMismatchIndex has two page leaves and one unnumbered error row.
func countMismatchIndex() []*pdfcore.PageIndexEntry {
	return []*pdfcore.PageIndexEntry{
		{PageNum: 1, ObjNum: 3, NodeID: "obj:0:3", MediaBox: [4]float64{0, 0, 612, 792}},
		{Err: "/Kids entry 1 is null"},
		{PageNum: 2, ObjNum: 4, NodeID: "obj:0:4", MediaBox: [4]float64{0, 0, 612, 792}},
	}
}

// warningsOf parses every JSON warning object written to stderr.
func warningsOf(t *testing.T, stderr string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if line == "" {
			continue
		}
		var obj map[string]string
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("stderr line is not a JSON object: %q", line)
		}
		if w, ok := obj["warning"]; ok {
			out = append(out, w)
		}
	}
	return out
}

func TestPagesDumpWarnsWhenCountDisagreesInBothModes(t *testing.T) {
	cases := []struct {
		name      string
		pageCount int
		want      string
	}{
		{"count lower than the leaves", 1, "page tree has 2 page leaves but /Count says 1"},
		{"count higher than the leaves", 5, "page tree has 2 page leaves but /Count says 5"},
		{"count zero or unreadable", 0, "page tree has 2 page leaves but /Count is 0 or unreadable"},
	}
	for _, c := range cases {
		for _, jsonMode := range []bool{true, false} {
			var stdout, stderr bytes.Buffer
			code := writePagesDump(&stdout, &stderr, countMismatchIndex(), c.pageCount, docViewFlags{json: jsonMode})
			if code != 0 {
				t.Errorf("%s (json=%v): exit %d, want 0", c.name, jsonMode, code)
			}
			warnings := warningsOf(t, stderr.String())
			if len(warnings) != 1 || warnings[0] != c.want {
				t.Errorf("%s (json=%v): warnings %q, want [%q]", c.name, jsonMode, warnings, c.want)
			}
			if jsonMode {
				var entries []pdfcore.PageIndexEntry
				if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil || len(entries) != 3 {
					t.Errorf("%s: stdout must stay a JSON array of 3 entries, got %q (%v)", c.name, stdout.String(), err)
				}
			} else if !strings.HasPrefix(stdout.String(), "PAGE") {
				t.Errorf("%s: plain stdout must be the table, got %q", c.name, stdout.String())
			}
		}
	}
}

// failingWriter rejects every write, like a closed stdout pipe.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestPagesDumpWriteFailureLeavesOnlyTheErrorOnStderr(t *testing.T) {
	for _, jsonMode := range []bool{true, false} {
		var stderr bytes.Buffer
		code := writePagesDump(failingWriter{}, &stderr, countMismatchIndex(), 5, docViewFlags{json: jsonMode})
		if code != 2 {
			t.Errorf("json=%v: exit %d, want 2", jsonMode, code)
		}
		lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("json=%v: exit-2 stderr must be one JSON object, got %q", jsonMode, stderr.String())
		}
		var obj map[string]string
		if err := json.Unmarshal([]byte(lines[0]), &obj); err != nil || !strings.HasPrefix(obj["error"], "failed to write output") {
			t.Errorf("json=%v: stderr = %q, want a single error object", jsonMode, stderr.String())
		}
	}
}

func TestPagesDumpNoWarningWhenCountAgrees(t *testing.T) {
	for _, jsonMode := range []bool{true, false} {
		var stdout, stderr bytes.Buffer
		if code := writePagesDump(&stdout, &stderr, countMismatchIndex(), 2, docViewFlags{json: jsonMode}); code != 0 {
			t.Errorf("json=%v: exit %d, want 0", jsonMode, code)
		}
		if stderr.Len() != 0 {
			t.Errorf("json=%v: stderr must be empty when /Count agrees, got %q", jsonMode, stderr.String())
		}
	}
}

func TestPagesDumpEmptyIndexIsAnEmptyArray(t *testing.T) {
	var stdout, stderr bytes.Buffer
	writePagesDump(&stdout, &stderr, []*pdfcore.PageIndexEntry{}, 0, docViewFlags{json: true})
	if got := strings.TrimSpace(stdout.String()); got != "[]" {
		t.Errorf("stdout = %q, want []", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("0 leaves against /Count 0 agree; stderr = %q", stderr.String())
	}
}

func TestPagesDumpPlainRowsForErrorAndInheritance(t *testing.T) {
	index := []*pdfcore.PageIndexEntry{
		{PageNum: 1, ObjNum: 3, NodeID: "obj:0:3", MediaBox: [4]float64{0, 0, 595.5, 842},
			Inherited: pdfcore.InheritedMediaBox | pdfcore.InheritedRotate, ContentLen: -1, Err: "tab\there"},
		{Err: "/Kids entry 1 is null"},
	}
	var out bytes.Buffer
	if err := printPagesPlain(&out, index); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want header plus 2 rows:\n%s", len(lines), out.String())
	}
	for _, want := range []string{"0 0 595.5 842", "MediaBox,Rotate", "-1", `"tab\there"`} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("row 1 missing %q: %q", want, lines[1])
		}
	}
	if f := strings.Fields(lines[2]); f[0] != "-" || f[1] != "-" {
		t.Errorf("an unnumbered row with no reference shows - for PAGE and REF: %q", lines[2])
	}
}
