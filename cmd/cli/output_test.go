package main

import (
	"strconv"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// TestASCIISafe_EscapesOnlyWhenNeeded pins asciiSafe's conditional contract: a
// printable-ASCII value is returned unchanged, while any byte outside
// 0x20-0x7e triggers the strconv.QuoteToASCII escape.
func TestASCIISafe_EscapesOnlyWhenNeeded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		in     string
		escape bool // true: expect strconv.QuoteToASCII(in); false: expect in verbatim
	}{
		{name: "empty", in: "", escape: false},
		{name: "printable ASCII unchanged", in: "Invoice 2024-001", escape: false},
		{name: "space and tilde are printable", in: " ~", escape: false},
		{name: "non-ASCII latin", in: "Größe", escape: true},
		{name: "CJK", in: "中文", escape: true},
		{name: "newline", in: "Da\nta", escape: true},
		{name: "tab", in: "a\tb", escape: true},
		{name: "NUL", in: "a\x00b", escape: true},
		{name: "DEL", in: "a\x7fb", escape: true},
		{name: "invalid UTF-8", in: "\x80\xca", escape: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.in
			if tc.escape {
				want = strconv.QuoteToASCII(tc.in)
			}
			got := asciiSafe(tc.in)
			if got != want {
				t.Fatalf("asciiSafe(%q) = %q, want %q", tc.in, got, want)
			}
			// Whatever the branch, the result must be printable ASCII.
			for i := 0; i < len(got); i++ {
				if got[i] < 0x20 || got[i] > 0x7e {
					t.Fatalf("asciiSafe(%q) = %q still carries byte 0x%02x at %d", tc.in, got, got[i], i)
				}
			}
		})
	}
}

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
		0:       "0 B",
		512:     "512 B",
		1024:    "1.0 KB",
		1536:    "1.5 KB",
		1048576: "1.0 MB",
	}
	for n, want := range cases {
		if got := humanizeBytes(n); got != want {
			t.Errorf("humanizeBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestPrintMetadataPlain_XMPIsVerbatimInfoIsEscaped pins the split on the
// `dump metadata` plain surface: /Info values are ASCII-escaped, while the XMP
// packet below the "XMP:" heading is UTF-8 XML passed through verbatim
// (escaping it would corrupt it as XML).
//
// The acceptance fixture has no /Metadata stream, so it cannot cover this.
func TestPrintMetadataPlain_XMPIsVerbatimInfoIsEscaped(t *testing.T) {
	md := &pdfcore.DocumentMetadata{
		Info: map[string]string{"Title": "Rechnung Größe 中文"},
		XMP:  "<x:xmpmeta><dc:title>Größe 中文</dc:title></x:xmpmeta>",
	}
	var b strings.Builder
	if err := printMetadataPlain(&b, md); err != nil {
		t.Fatalf("printMetadataPlain: %v", err)
	}
	out := b.String()

	i := strings.Index(out, "\nXMP:\n")
	if i < 0 {
		t.Fatalf("expected an XMP: heading line in:\n%s", out)
	}
	infoBlock, xmpBlock := out[:i], out[i:]

	// The Info block must be pure printable ASCII (newlines aside).
	for j := 0; j < len(infoBlock); j++ {
		if c := infoBlock[j]; c != '\n' && (c < 0x20 || c > 0x7e) {
			t.Fatalf("Info block carries byte 0x%02x at %d; decoded values must be escaped:\n%s", c, j, infoBlock)
		}
	}
	if want := strconv.QuoteToASCII("Rechnung Größe 中文"); !strings.Contains(infoBlock, want) {
		t.Errorf("Info block must carry the escaped title %s; got:\n%s", want, infoBlock)
	}

	// The XMP packet must survive byte-for-byte, non-ASCII included.
	if !strings.Contains(xmpBlock, md.XMP) {
		t.Errorf("XMP packet must be passed through verbatim; got:\n%s", xmpBlock)
	}
	if !strings.Contains(xmpBlock, "Größe 中文") {
		t.Errorf("XMP packet must retain its non-ASCII bytes (it is UTF-8 XML, not an ASCII surface); got:\n%s", xmpBlock)
	}
}

// TestPrintEmbeddedListPlain_AllPDFDerivedCellsEscaped pins that every
// PDF-derived cell is escaped, not just Name. AFRelationship and Subtype come
// from PDF Names, whose #xx escapes resolve to arbitrary bytes: a raw 0x0A
// splits one logical row across two lines, and a multi-byte rune breaks
// tableWriter's len()-based padding.
//
// The acceptance fixture's values are pure ASCII, so it cannot cover this.
func TestPrintEmbeddedListPlain_AllPDFDerivedCellsEscaped(t *testing.T) {
	files := []pdfcore.EmbeddedFile{{
		Name:            "größe-中文.xml",
		AFRelationship:  "Da\nta",    // a raw newline would split the row
		Subtype:         "text/xéml", // raw multi-byte would break padding
		Size:            70,
		FilespecRef:     "6 0 R",
		EmbeddedFileRef: "4 0 R",
	}}
	var b strings.Builder
	if err := printEmbeddedListPlain(&b, files); err != nil {
		t.Fatalf("printEmbeddedListPlain: %v", err)
	}
	out := b.String()

	for i := 0; i < len(out); i++ {
		if c := out[i]; c != '\n' && (c < 0x20 || c > 0x7e) {
			t.Fatalf("table carries byte 0x%02x at %d; every PDF-derived cell must be escaped:\n%s", c, i, out)
		}
	}
	// Header + exactly one data row: a raw 0x0A in a cell would make three lines.
	if got := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); got != 2 {
		t.Errorf("expected 2 lines (header + 1 row), got %d - a cell newline leaked and split the row:\n%s", got, out)
	}
	for _, want := range []string{
		strconv.QuoteToASCII("größe-中文.xml"),
		strconv.QuoteToASCII("Da\nta"),
		strconv.QuoteToASCII("text/xéml"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table must carry the escaped cell %s; got:\n%s", want, out)
		}
	}
}

// TestAnyNameDiffersFromDisplay covers the reasons a name copied out of the
// plain table will not match the --name selector.
func TestAnyNameDiffersFromDisplay(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []pdfcore.EmbeddedFile
		want  bool
	}{
		{name: "all plain ASCII", files: []pdfcore.EmbeddedFile{{Name: "a.xml"}, {Name: "b.xml"}}, want: false},
		{name: "non-ASCII is escaped for display", files: []pdfcore.EmbeddedFile{{Name: "a.xml"}, {Name: "größe.xml"}}, want: true},
		{name: "control byte is escaped for display", files: []pdfcore.EmbeddedFile{{Name: "a\nb.xml"}}, want: true},
		{name: "empty name renders as a dash", files: []pdfcore.EmbeddedFile{{Name: ""}}, want: true},
		{name: "leading space is invisible in a padded column", files: []pdfcore.EmbeddedFile{{Name: " a.xml"}}, want: true},
		{name: "trailing space is invisible in a padded column", files: []pdfcore.EmbeddedFile{{Name: "a.xml "}}, want: true},
		{name: "no files at all", files: nil, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := anyNameDiffersFromDisplay(tc.files); got != tc.want {
				t.Errorf("anyNameDiffersFromDisplay(%v) = %v, want %v", tc.files, got, tc.want)
			}
		})
	}
}
