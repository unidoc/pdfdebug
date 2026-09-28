package pages_navigator_test

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// `pdfdebug dump pages` renders the page index: one row per page leaf in
// document order, plus an unnumbered row for anything in the page tree that is
// not a page. Plain text by default, the entry array with --json.
// ---------------------------------------------------------------------------

func TestJSONIsTheEntryArrayWithEveryKey(t *testing.T) {
	path := writeFixture(t, "well-formed.pdf", wellFormedPDF())
	stdout, stderr, code := runCLI(t, "dump", "pages", "--json", path)
	if code != 0 {
		t.Fatalf("exit %d, want 0\nstderr: %s", code, stderr)
	}
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("stdout is not a JSON array of objects: %v\nraw: %s", err, stdout)
	}
	if len(raw) != 2 {
		t.Fatalf("got %d entries, want 2\nraw: %s", len(raw), stdout)
	}
	for i, e := range raw {
		for _, k := range pageEntryKeys {
			if _, ok := e[k]; !ok {
				t.Errorf("entry %d is missing key %q: %s", i, k, stdout)
			}
		}
		var box []float64
		if err := json.Unmarshal(e["mediaBox"], &box); err != nil || len(box) != 4 {
			t.Errorf("entry %d mediaBox is not four numbers: %s", i, string(e["mediaBox"]))
		}
	}
}

func TestWellFormedDocumentEntries(t *testing.T) {
	entries, _ := dumpPagesJSON(t, writeFixture(t, "well-formed.pdf", wellFormedPDF()))
	want := []pageEntry{
		{PageNum: 1, ObjNum: 3, Gen: 0, NodeID: "obj:0:3", ContentNodeID: "obj:0:4",
			MediaBox: [4]float64{0, 0, 612, 792}, ContentLen: int64(len(contentA))},
		{PageNum: 2, ObjNum: 5, Gen: 0, NodeID: "obj:0:5", ContentNodeID: "",
			MediaBox: [4]float64{0, 0, 595.28, 841.89}, Rotate: 90, AnnotCount: 2},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entry %d:\n got %+v\nwant %+v", i, entries[i], want[i])
		}
	}
}

func TestPlainTextTable(t *testing.T) {
	stdout, _ := dumpPagesPlain(t, writeFixture(t, "well-formed.pdf", wellFormedPDF()))
	widths := []int{4, 5, 17, 6, 9, 6, 10}
	row := func(cells ...string) string {
		var b strings.Builder
		for i, c := range cells {
			if i < len(widths) {
				fmt.Fprintf(&b, "%-*s  ", widths[i], c)
			} else {
				b.WriteString(c)
			}
		}
		return b.String() + "\n"
	}
	want := row("PAGE", "REF", "MEDIABOX", "ROTATE", "INHERITED", "ANNOTS", "CONTENTLEN", "ERROR") +
		row("1", "3 0 R", "0 0 612 792", "0", "-", "0", "15", "-") +
		row("2", "5 0 R", "0 0 595.28 841.89", "90", "-", "2", "0", "-")
	if stdout != want {
		t.Errorf("plain output mismatch\n got:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestEmptyPageTree(t *testing.T) {
	path := writeFixture(t, "no-pages.pdf", noPagesPDF())
	stdout, stderr, code := runCLI(t, "dump", "pages", "--json", path)
	if code != 0 {
		t.Fatalf("exit %d, want 0\nstderr: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Errorf("an empty page tree must be an empty array, not null; got %q", stdout)
	}
	plain, _ := dumpPagesPlain(t, path)
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PAGE") {
		t.Errorf("plain output for no pages must be the header line only, got %q", plain)
	}
}

// ---------------------------------------------------------------------------
// Inherited attributes. /Resources, /MediaBox, /CropBox and /Rotate are pushed
// down from /Pages ancestors; a page's own value wins, and a bitmask records
// which ones came from an ancestor.
// ---------------------------------------------------------------------------

func TestInheritedAttributes(t *testing.T) {
	entries, _ := dumpPagesJSON(t, writeFixture(t, "inherited.pdf", inheritedPDF()))
	if len(entries) != 7 {
		t.Fatalf("got %d entries, want 7: %+v", len(entries), entries)
	}
	for i, e := range entries {
		if e.PageNum != i+1 || e.ObjNum != 10+i {
			t.Errorf("entry %d is page %d obj %d, want page %d obj %d", i, e.PageNum, e.ObjNum, i+1, 10+i)
		}
		if e.Err != "" {
			t.Errorf("page %d resolved with an error: %q", e.PageNum, e.Err)
		}
	}

	if entries[0].Inherited != 0 {
		t.Errorf("a page declaring all four attributes inherits nothing, got bitmask %d", entries[0].Inherited)
	}
	single := map[string]uint8{
		"Resources": entries[1].Inherited,
		"MediaBox":  entries[2].Inherited,
		"CropBox":   entries[3].Inherited,
		"Rotate":    entries[4].Inherited,
	}
	var union uint8
	seen := map[uint8]string{}
	for attr, bit := range single {
		if bits.OnesCount8(bit) != 1 {
			t.Errorf("a page inheriting only %s must have exactly one bit set, got %08b", attr, bit)
		}
		if other, dup := seen[bit]; dup {
			t.Errorf("%s and %s share inherited bit %08b", attr, other, bit)
		}
		seen[bit] = attr
		union |= bit
	}
	for _, i := range []int{5, 6} {
		if entries[i].Inherited != union {
			t.Errorf("page %d declares nothing and must inherit all four (%08b), got %08b",
				entries[i].PageNum, union, entries[i].Inherited)
		}
	}

	checks := []struct {
		i      int
		box    [4]float64
		rotate int
		why    string
	}{
		{0, [4]float64{0, 0, 100, 200}, 180, "own values"},
		{2, [4]float64{0, 0, 612, 792}, 180, "MediaBox from the root"},
		{4, [4]float64{0, 0, 100, 200}, 90, "Rotate from the root"},
		{5, [4]float64{0, 0, 612, 792}, 90, "everything from the root"},
		{6, [4]float64{0, 0, 595, 842}, 90, "MediaBox from the nearest ancestor, not the root"},
	}
	for _, c := range checks {
		e := entries[c.i]
		if e.MediaBox != c.box || e.Rotate != c.rotate {
			t.Errorf("page %d (%s): MediaBox %v Rotate %d, want %v and %d",
				e.PageNum, c.why, e.MediaBox, e.Rotate, c.box, c.rotate)
		}
	}
}

func TestPlainTextNamesInheritedAttributes(t *testing.T) {
	stdout, _ := dumpPagesPlain(t, writeFixture(t, "inherited.pdf", inheritedPDF()))
	rows := parseTable(t, stdout)
	want := []string{"-", "Resources", "MediaBox", "CropBox", "Rotate",
		"Resources,MediaBox,CropBox,Rotate", "Resources,MediaBox,CropBox,Rotate"}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d\n%s", len(rows), len(want), stdout)
	}
	for i, w := range want {
		if got := rows[i]["INHERITED"]; got != w {
			t.Errorf("row %d INHERITED = %q, want %q", i+1, got, w)
		}
	}
}

// ---------------------------------------------------------------------------
// The walk survives page trees that are wrong: a bad kid becomes an unnumbered
// row with an error instead of failing the command, and page numbers after it
// still match what a viewer shows.
// ---------------------------------------------------------------------------

// pdfcpu's validator drops null /Kids entries while opening the document, so
// the walk never sees this null; the null-kid error row is covered in package.
// What stays observable is that page numbering around it is unshifted.
func TestNullKidDoesNotShiftPageNumbers(t *testing.T) {
	path := writeFixture(t, "null-kid.pdf", nullKidPDF())
	entries, stderr := dumpPagesJSON(t, path)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (the null is dropped at open): %+v", len(entries), entries)
	}
	if entries[0].PageNum != 1 || entries[0].ObjNum != 3 || entries[1].PageNum != 2 || entries[1].ObjNum != 4 {
		t.Errorf("the null kid must not shift page numbers: got %+v", entries)
	}
	for _, e := range entries {
		if e.Err != "" {
			t.Errorf("page %d resolved with an error: %q", e.PageNum, e.Err)
		}
	}
	if strings.Contains(stderr, `"error"`) {
		t.Errorf("stderr carries an error: %s", stderr)
	}
}

func TestDuplicatedPageIsNumberedAgain(t *testing.T) {
	entries, _ := dumpPagesJSON(t, writeFixture(t, "duplicate-page.pdf", duplicatePagePDF()))
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(entries), entries)
	}
	for i, wantObj := range []int{3, 3, 4} {
		if entries[i].PageNum != i+1 || entries[i].ObjNum != wantObj {
			t.Errorf("entry %d is page %d obj %d, want page %d obj %d",
				i, entries[i].PageNum, entries[i].ObjNum, i+1, wantObj)
		}
	}
	if entries[0].Err != "" {
		t.Errorf("the first listing is clean, got error %q", entries[0].Err)
	}
	if !strings.Contains(entries[1].Err, "page 1") {
		t.Errorf("the second listing must name the page it repeats, got error %q", entries[1].Err)
	}
	if entries[2].Err != "" {
		t.Errorf("the page after the duplicate is clean, got error %q", entries[2].Err)
	}
}

func TestPageTreeClassification(t *testing.T) {
	cases := []struct {
		name    string
		pdf     []byte
		wantObj []int
		wantErr []string
	}{
		{"a /Type /Page node carrying /Kids is a page and its kids are not walked", pageWithKidsPDF(), []int{3}, []string{"page has a /Kids entry"}},
		{"an empty /Kids yields no rows", emptyKidsPDF(), []int{4}, []string{""}},
		{"a root /Pages with no /Type is still an intermediate", untypedRootPDF(), []int{3, 4}, []string{"", ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entries, _ := dumpPagesJSON(t, writeFixture(t, "fixture.pdf", c.pdf))
			if len(entries) != len(c.wantObj) {
				t.Fatalf("got %d entries, want %d: %+v", len(entries), len(c.wantObj), entries)
			}
			for i, obj := range c.wantObj {
				e := entries[i]
				errOK := e.Err == c.wantErr[i] || (c.wantErr[i] != "" && strings.Contains(e.Err, c.wantErr[i]))
				if e.PageNum != i+1 || e.ObjNum != obj || !errOK {
					t.Errorf("entry %d: page %d obj %d error %q, want page %d obj %d and error %q",
						i, e.PageNum, e.ObjNum, e.Err, i+1, obj, c.wantErr[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// /Contents and /Length are read from the dictionaries. The index never
// decodes a stream: ContentLen is the sum of the /Length entries.
// ---------------------------------------------------------------------------

func TestContentStreamReferenceAndLength(t *testing.T) {
	entries, _ := dumpPagesJSON(t, writeFixture(t, "contents.pdf", contentsPDF()))
	sum := int64(len(contentA) + len(contentB))
	want := []struct {
		obj        int
		contentID  string
		contentLen int64
		why        string
	}{
		{3, "obj:0:6", sum, "an indirect /Contents resolving to an array is summed over the array"},
		{4, "obj:0:6", sum, "a direct /Contents array names its first stream and sums every /Length"},
		{8, "obj:0:9", int64(len(contentB)), "an indirect /Length is dereferenced"},
		// pdfcpu rewrites a missing or non-integer /Length to the stream's raw
		// byte count while reading it; the -1 for an unreadable /Length is
		// covered in package.
		{10, "obj:0:11", 3, "a stream with no /Length reports the length pdfcpu recorded"},
		{12, "obj:0:13", 3, "a /Length that is not an integer reports the length pdfcpu recorded"},
		{14, "", 0, "no /Contents reports 0 and no content node"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i, w := range want {
		e := entries[i]
		if e.PageNum != i+1 || e.ObjNum != w.obj {
			t.Errorf("entry %d is page %d obj %d, want page %d obj %d", i, e.PageNum, e.ObjNum, i+1, w.obj)
			continue
		}
		if e.ContentNodeID != w.contentID || e.ContentLen != w.contentLen {
			t.Errorf("page %d (%s): contentNodeId %q contentLen %d, want %q and %d",
				e.PageNum, w.why, e.ContentNodeID, e.ContentLen, w.contentID, w.contentLen)
		}
	}
	for _, i := range []int{0, 1, 2} {
		if entries[i].Err != "" {
			t.Errorf("page %d has well-formed /Contents and must carry no error, got %q", i+1, entries[i].Err)
		}
	}
}

func TestIndirectAttributeValues(t *testing.T) {
	entries, _ := dumpPagesJSON(t, writeFixture(t, "indirect-attrs.pdf", indirectAttrsPDF()))
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5: %+v", len(entries), entries)
	}
	if got := entries[0].MediaBox; got != [4]float64{0, 0, 300, 400} {
		t.Errorf("page 1 own indirect MediaBox = %v, want [0 0 300 400]", got)
	}
	if entries[0].Inherited != 0 {
		t.Errorf("page 1 declares its MediaBox and inherits nothing, got bitmask %08b", entries[0].Inherited)
	}
	if got := entries[1].MediaBox; got != [4]float64{0, 0, 10, 20} {
		t.Errorf("page 2 inherited indirect MediaBox = %v, want [0 0 10 20]", got)
	}
	if entries[1].Inherited == 0 {
		t.Errorf("page 2 takes its MediaBox from the root and must record it as inherited")
	}
	if entries[2].Rotate != 90 {
		t.Errorf("page 3 indirect /Rotate = %d, want 90", entries[2].Rotate)
	}
	if entries[3].Rotate != -90 {
		t.Errorf("page 4 /Rotate is reported as stored: got %d, want -90", entries[3].Rotate)
	}
	if entries[4].AnnotCount != 2 {
		t.Errorf("page 5 indirect /Annots count = %d, want 2", entries[4].AnnotCount)
	}
	for _, e := range entries {
		if e.Err != "" {
			t.Errorf("page %d resolved with an error: %q", e.PageNum, e.Err)
		}
	}
}

// ---------------------------------------------------------------------------
// Walk order matches the page resolution the rest of the tool uses: the Nth
// numbered row names the same object `dump tree --page N` resolves.
// ---------------------------------------------------------------------------

func TestWalkOrderMatchesPageResolution(t *testing.T) {
	cases := map[string]string{
		"multipage.pdf (testdata)": testdataFile(t, "multipage.pdf"),
		"well-formed.pdf":          writeFixture(t, "well-formed.pdf", wellFormedPDF()),
		"inherited.pdf":            writeFixture(t, "inherited.pdf", inheritedPDF()),
		"null-kid.pdf":             writeFixture(t, "null-kid.pdf", nullKidPDF()),
		"duplicate-page.pdf":       writeFixture(t, "duplicate-page.pdf", duplicatePagePDF()),
		"empty-kids.pdf":           writeFixture(t, "empty-kids.pdf", emptyKidsPDF()),
		"contents.pdf":             writeFixture(t, "contents.pdf", contentsPDF()),
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			entries, _ := dumpPagesJSON(t, path)
			pages := numbered(entries)
			if len(pages) == 0 {
				t.Fatalf("no numbered rows")
			}
			for i, e := range pages {
				if want := pageNodeIDFromTree(t, path, i+1); e.NodeID != want {
					t.Errorf("numbered row %d names %q, dump tree --page %d resolves %q", i+1, e.NodeID, i+1, want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The /Count warning. A disagreement between the root /Count and the leaf count
// goes to stderr as a JSON warning; when they agree nothing is written.
// ---------------------------------------------------------------------------

func TestNoCountWarningWhenCountsAgree(t *testing.T) {
	for name, pdf := range allFixtures() {
		t.Run(name, func(t *testing.T) {
			path := writeFixture(t, name, pdf)
			for _, args := range [][]string{{"dump", "pages", "--json", path}, {"dump", "pages", path}} {
				_, stderr, code := runCLI(t, args...)
				if code != 0 {
					t.Fatalf("%v exited %d, want 0\nstderr: %s", args, code, stderr)
				}
				for _, w := range stderrWarnings(t, stderr) {
					if strings.Contains(w, "/Count") {
						t.Errorf("%v warned about /Count on a tree whose /Count agrees: %q", args, w)
					}
				}
			}
		})
	}
}

// pdfcpu refuses to open a document whose root /Count disagrees with its
// leaves, so the disagreement warning is driven in package, where the
// document is built in code.
func TestCountDisagreementWarningUnitCoverageExists(t *testing.T) {
	runGoTest(t, "^Test.*(PagesDump|DumpPages)", "./cmd/cli/")
}

// ---------------------------------------------------------------------------
// Argument handling matches the other document-level dump commands.
// ---------------------------------------------------------------------------

func TestUsageErrors(t *testing.T) {
	path := writeFixture(t, "well-formed.pdf", wellFormedPDF())
	cases := map[string][]string{
		"no file":             {"dump", "pages"},
		"two files":           {"dump", "pages", path, path},
		"flag after the file": {"dump", "pages", path, "--json"},
		"unknown flag":        {"dump", "pages", "--ref", "3 0 R", path},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, args...)
			if code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
			if !strings.Contains(stderr, "Usage: pdfdebug dump pages") {
				t.Errorf("stderr must carry the dump pages usage line, got: %s", stderr)
			}
			if stdout != "" {
				t.Errorf("a usage error writes nothing to stdout, got: %s", stdout)
			}
		})
	}
}

func TestMissingFileMatchesDumpObjects(t *testing.T) {
	missing := t.TempDir() + "/does-not-exist.pdf"
	_, objErr, objCode := runCLI(t, "dump", "objects", missing)
	_, pagesErr, pagesCode := runCLI(t, "dump", "pages", missing)
	if pagesCode != objCode {
		t.Errorf("dump pages on a missing file exited %d, dump objects exits %d", pagesCode, objCode)
	}
	if strings.TrimSpace(pagesErr) != strings.TrimSpace(objErr) {
		t.Errorf("dump pages stderr %q, dump objects stderr %q", pagesErr, objErr)
	}
}

func TestHelpListsDumpPages(t *testing.T) {
	_, stderr, _ := runCLI(t, "--help")
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "dump pages [--json] <file>") {
			return
		}
	}
	t.Errorf("--help has no `dump pages [--json] <file>` line:\n%s", stderr)
}

// ---------------------------------------------------------------------------
// Walk rules whose fixtures pdfcpu refuses to open (a /Kids cycle, a kid that
// is not a dictionary or dangles, a /Page with no /Type, malformed /MediaBox and
// /Rotate values, /Contents elements that are not references), the lazy cache
// and the service wrapper are covered in package.
// ---------------------------------------------------------------------------

func TestPageIndexUnitCoverageExists(t *testing.T) {
	runGoTest(t, "^Test.*PageIndex", "./internal/pdfcore/")
}

func TestLazyCacheUnitCoverageExists(t *testing.T) {
	runGoTest(t, "^Test.*LazyCache", "./internal/pdfcore/")
}

func TestServiceWrapperUnitCoverageExists(t *testing.T) {
	runGoTest(t, "^Test.*GetPageIndex", "./internal/pdfservice/")
}

// parseTable splits a plain-text table into rows keyed by header name, using
// the header's column starts; cells are padded to those columns.
func parseTable(t *testing.T, out string) []map[string]string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "PAGE") {
		t.Fatalf("output does not start with a PAGE header:\n%s", out)
	}
	header := lines[0]
	names := strings.Fields(header)
	starts := make([]int, len(names))
	from := 0
	for i, n := range names {
		idx := strings.Index(header[from:], n)
		starts[i] = from + idx
		from = starts[i] + len(n)
	}
	var rows []map[string]string
	for _, line := range lines[1:] {
		r := map[string]string{}
		for i, n := range names {
			if starts[i] >= len(line) {
				r[n] = ""
				continue
			}
			end := len(line)
			if i+1 < len(names) && starts[i+1] < end {
				end = starts[i+1]
			}
			r[n] = strings.TrimSpace(line[starts[i]:end])
		}
		rows = append(rows, r)
	}
	return rows
}
