package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

func strPtr(s string) *string { return &s }

func intPtr(n int) *int { return &n }

// lineOf returns the first line of out containing sub, or fails.
func lineOf(t *testing.T, out, sub string) string {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, sub) {
			return line
		}
	}
	t.Fatalf("no line contains %q:\n%s", sub, out)
	return ""
}

func TestStructurePlain_HeaderVariants(t *testing.T) {
	marked := false
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		Marked:        &marked,
		RoleMap:       []pdfcore.RoleMapEntry{{From: "Bad", To: ""}, {From: "Heading1", To: "H1"}},
	}
	out := structurePlain(r)
	for _, want := range []string{
		"Structure tree: (direct)\n",
		"ParentTree: absent\n",
		"MarkInfo /Marked: false\n",
		"Document /Lang: absent\n",
		"RoleMap: Bad -> ?, Heading1 -> H1\n",
		"Elements (0; kids: 0 MCID, 0 MCR, 0 OBJR, 0 error):\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	next := 4
	r = &pdfcore.StructureReport{
		HasStructTree: true, RootRef: "12 0 R", ParentTreeNextKey: &next,
		HasParentTree: true, ParentTreeRef: "31 0 R", DocLang: "de-DE",
	}
	out = structurePlain(r)
	for _, want := range []string{
		"Structure tree: 12 0 R  (ParentTreeNextKey 4)\n",
		"ParentTree: 31 0 R\n",
		"MarkInfo /Marked: absent\n",
		"Document /Lang: de-DE\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "RoleMap:") {
		t.Errorf("empty RoleMap printed:\n%s", out)
	}
}

func TestStructurePlain_UntaggedHasNoHeaderOrElements(t *testing.T) {
	r := &pdfcore.StructureReport{Pages: []*pdfcore.StructPage{{PageNum: 1}, {PageNum: 2}, {PageNum: 4}}}
	out := structurePlain(r)
	if !strings.HasPrefix(out, "No structure tree (/StructTreeRoot absent)\n") {
		t.Errorf("output does not open with the no-tree line:\n%s", out)
	}
	if strings.Contains(out, "Elements (") || strings.Contains(out, "Structure tree:") {
		t.Errorf("untagged output has a header or element section:\n%s", out)
	}
	if !strings.Contains(out, "no marked content: 1-2, 4\n") {
		t.Errorf("empty pages not collapsed into ranges:\n%s", out)
	}
}

func TestStructurePlain_ElementTree(t *testing.T) {
	long := strings.Repeat("a", 100)
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		Counts:        pdfcore.StructureCounts{Elements: 3, MCIDKids: 2, MCRKids: 2, OBJRKids: 2, ErrorKids: 1},
		Elements: []*pdfcore.StructKid{
			{Kind: "element", Element: &pdfcore.StructElem{
				S: "Heading1", MappedS: "H1", Ref: "14 0 R", Page: 1, Title: "Intro", Lang: "en",
				Kids: []*pdfcore.StructKid{
					{Kind: "mcid", MCID: intPtr(0), Page: 1},
					{Kind: "mcid", MCID: intPtr(1), Err: "no page"},
					{Kind: "element", Element: &pdfcore.StructElem{
						S: "P", Page: 2, PageInherited: true, ActualText: strPtr(long), Alt: strPtr("caf\u00e9"), E: strPtr(""),
						Kids: []*pdfcore.StructKid{
							{Kind: "mcr", MCID: intPtr(3), Page: 2, StmRef: "40 0 R"},
							{Kind: "mcr", Err: "MCR has no /MCID"},
							{Kind: "objr", ObjRef: "22 0 R", ObjType: "Annot/Link"},
							{Kind: "objr", Err: "OBJR has no /Obj"},
						},
					}},
				},
			}},
			{Kind: "element", Element: &pdfcore.StructElem{PgRef: "9 0 R", Err: "no /S; /Pg 9 0 R is a dictionary that is not a page in the page tree"}},
			{Kind: "error", Err: "element 14 0 R already listed; not walked again"},
		},
	}
	out := structurePlain(r)

	if !strings.Contains(out, "Elements (3; kids: 2 MCID, 2 MCR, 2 OBJR, 1 error):\n") {
		t.Errorf("counts line missing:\n%s", out)
	}
	lines := []string{
		`Heading1 (H1) 14 0 R  page 1  T "Intro"  Lang "en"`,
		`  MCID 0`,
		`  MCID 1  error: no page`,
		`  P (direct)  page 2 (inherited)  Alt "caf\u00e9"  ActualText "` + strings.Repeat("a", 80) + ` [truncated: 80 of 100]"  E ""`,
		`    MCR MCID 3 page 2 stream 40 0 R`,
		`    MCR  error: MCR has no /MCID`,
		`    OBJR 22 0 R Annot/Link`,
		`    OBJR -  error: OBJR has no /Obj`,
		`(no /S) (direct)  /Pg 9 0 R  error: no /S; /Pg 9 0 R is a dictionary that is not a page in the page tree`,
		`error: element 14 0 R already listed; not walked again`,
	}
	for _, want := range lines {
		if !strings.Contains(out, "\n"+want+"\n") {
			t.Errorf("output missing line %q:\n%s", want, out)
		}
	}
	for line := range strings.SplitSeq(out, "\n") {
		for i := 0; i < len(line); i++ {
			if line[i] < 0x20 || line[i] > 0x7e {
				t.Fatalf("non-ASCII byte in line %q", line)
			}
		}
	}
}

func TestStructurePlain_PageErrorLines(t *testing.T) {
	sp := 0
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		Pages: []*pdfcore.StructPage{
			{PageNum: 1, StructParents: &sp, Err: "contents array element 0 is not an indirect reference"},
			{PageNum: 2, ParentTreeErr: "ParentTree has no key 5"},
			{PageNum: 3, Streams: []pdfcore.MarkedStream{
				{Ref: "20 0 R", Err: "failed to decode stream: bad"},
				{Ref: "21 0 R", Opens: 1, Closes: 2, DepthAtEnd: 0, MinDepth: -1},
				{Ref: "23 0 R", Opens: 1, Closes: 1},
				{Ref: "22 0 R", Opens: 1, Closes: 0, DepthAtEnd: 1, MinDepth: 0},
			}, ContentMCIDs: []pdfcore.ContentMCID{
				{MCID: -1, Tag: "Span", Stream: 1, Line: 2, Col: 3, Err: "property list /MC9 not found"},
				{MCID: -1, Stream: 7, Line: 1, Col: 1, Err: "BDC has 0 of its 2 operands"},
				{MCID: 0, Stream: 3},
			}},
		},
	}
	out := structurePlain(r)
	for _, want := range []string{
		"Page 1  /StructParents 0  0 streams  BDC/BMC 0, EMC 0\n  error: contents array element 0 is not an indirect reference\n",
		"Page 2  /StructParents absent  0 streams  BDC/BMC 0, EMC 0\n  ParentTree error: ParentTree has no key 5\n",
		"Page 3  /StructParents absent  4 streams  BDC/BMC 3, EMC 3\n",
		"  stream 20 0 R: error: failed to decode stream: bad\n",
		"  stream 21 0 R: BDC/BMC 1, EMC 2, depth at end 0, min depth -1\n",
		"  stream 22 0 R: BDC/BMC 1, EMC 0, depth at end 1, min depth 0\n",
		"  stream 21 0 R line 2 col 3: Span property list /MC9 not found\n",
		"  stream 7 line 1 col 1: ? BDC has 0 of its 2 operands\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "stream 23 0 R") {
		t.Errorf("balanced stream printed a line:\n%s", out)
	}
	if strings.Contains(out, "Pages with no /StructParents") {
		t.Errorf("pages with errors collapsed:\n%s", out)
	}
}

func TestStructurePlain_SequenceClosedInALaterStreamIsNotReportedOpen(t *testing.T) {
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		Pages: []*pdfcore.StructPage{
			{PageNum: 1, Streams: []pdfcore.MarkedStream{
				{Ref: "30 0 R", Opens: 1, DepthAtEnd: 1},
				{Ref: "31 0 R", Closes: 1, MinDepth: 0},
			}},
			{PageNum: 2, Streams: []pdfcore.MarkedStream{
				{Ref: "40 0 R", Opens: 2, Closes: 1, DepthAtEnd: 1},
				{Ref: "41 0 R", Opens: 1, DepthAtEnd: 2, MinDepth: 1},
			}},
		},
	}
	out := structurePlain(r)
	for _, absent := range []string{"stream 30 0 R", "stream 31 0 R", "stream 40 0 R"} {
		if strings.Contains(out, absent) {
			t.Errorf("output has %q for a sequence that a later stream closes or that is still counted on the last stream:\n%s", absent, out)
		}
	}
	if want := "  stream 41 0 R: BDC/BMC 1, EMC 0, depth at end 2, min depth 1\n"; !strings.Contains(out, want) {
		t.Errorf("output missing %q for the page's last stream:\n%s", want, out)
	}
}

func TestStructurePlain_LegendOnlyUnderFirstStarredTable(t *testing.T) {
	inconsistent := []pdfcore.MCIDAlignment{{MCID: 0, ContentCount: 2, TreeElements: []string{}}}
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		Pages: []*pdfcore.StructPage{
			{PageNum: 1, Alignment: []pdfcore.MCIDAlignment{{MCID: 0, ContentCount: 1, ParentTreeElement: "10 0 R", ParentTreeS: "P", TreeElements: []string{"10 0 R"}, Consistent: true}}},
			{PageNum: 2, Alignment: inconsistent},
			{PageNum: 3, Alignment: inconsistent},
		},
	}
	out := structurePlain(r)
	if n := strings.Count(out, "* = content count"); n != 1 {
		t.Errorf("legend printed %d times, want once:\n%s", n, out)
	}
	if strings.Index(out, "* = content count") < strings.Index(out, "Page 2 ") {
		t.Errorf("legend before the first starred table:\n%s", out)
	}
	if got := lineOf(t, out, "10 0 R P"); strings.HasSuffix(got, "*") {
		t.Errorf("consistent row starred: %q", got)
	}
	after := out[strings.Index(out, "Page 2 "):]
	row := after[strings.Index(after, "\n  0 ")+1:]
	row = row[:strings.Index(row, "\n")]
	if got := strings.Fields(row); strings.Join(got, " ") != "0 2 - - *" {
		t.Errorf("inconsistent row with no claimants = %q, want fields 0 2 - - *", row)
	}
}

func TestStructurePlain_OtherParentTreeAndProblems(t *testing.T) {
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		OtherParentTree: []pdfcore.ParentTreeEntry{
			{Key: 2, ElementRef: "17 0 R", S: "Link"},
			{Key: 7, Note: "array value no page's /StructParents names; may belong to a Form XObject's /StructParents"},
			{Key: 9, Err: "value is an integer, not a reference to an element"},
		},
		Problems: []pdfcore.StructureProblem{{Where: "ParentTree 31 0 R, node 33 0 R", Message: "key 1 outside /Limits [0 0]"}},
	}
	out := structurePlain(r)
	for _, want := range []string{
		"\nOther ParentTree keys:\n  2  ->  17 0 R Link\n  7  ->    note: array value no page's /StructParents names; may belong to a Form XObject's /StructParents\n  9  ->    error: value is an integer, not a reference to an element\n",
		"\nProblems (1):\n  ParentTree 31 0 R, node 33 0 R: key 1 outside /Limits [0 0]\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestStructurePlain_NoOptionalSectionsWhenEmpty(t *testing.T) {
	out := structurePlain(&pdfcore.StructureReport{HasStructTree: true})
	for _, absent := range []string{"Other ParentTree keys", "Problems (", "Pages with no"} {
		if strings.Contains(out, absent) {
			t.Errorf("output has %q with nothing to list:\n%s", absent, out)
		}
	}
}

func TestStructurePlain_PageWithOnlyUnmarkedSequencesGetsABlock(t *testing.T) {
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		Pages: []*pdfcore.StructPage{
			{PageNum: 1, Streams: []pdfcore.MarkedStream{{Ref: "5 0 R", Opens: 1, Closes: 1}}},
			{PageNum: 2, Streams: []pdfcore.MarkedStream{{Ref: "6 0 R", Closes: 1, MinDepth: -1}}},
			{PageNum: 3, Streams: []pdfcore.MarkedStream{{Ref: "7 0 R"}}},
		},
	}
	out := structurePlain(r)
	for _, want := range []string{
		"Page 1  /StructParents absent  1 stream  BDC/BMC 1, EMC 1\n",
		"Page 2  /StructParents absent  1 stream  BDC/BMC 0, EMC 1\n",
		"no marked content: 3\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPageRanges(t *testing.T) {
	cases := []struct {
		in   []int
		want string
	}{
		{[]int{1}, "1"},
		{[]int{1, 2, 3}, "1-3"},
		{[]int{1, 3, 5}, "1, 3, 5"},
		{[]int{2, 3, 7, 9, 10, 11}, "2-3, 7, 9-11"},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := pageRanges(tc.in); got != tc.want {
			t.Errorf("pageRanges(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStructureDumpWritesJSONOrPlain(t *testing.T) {
	r := &pdfcore.StructureReport{
		HasStructTree: true, RootRef: "12 0 R",
		RoleMap: []pdfcore.RoleMapEntry{}, Elements: []*pdfcore.StructKid{}, Pages: []*pdfcore.StructPage{},
		OtherParentTree: []pdfcore.ParentTreeEntry{}, Problems: []pdfcore.StructureProblem{},
	}
	var stdout, stderr bytes.Buffer
	if code := writeStructureDump(&stdout, &stderr, r, docViewFlags{json: true}); code != 0 {
		t.Fatalf("json exit %d, want 0", code)
	}
	var got pdfcore.StructureReport
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !got.HasStructTree || got.RootRef != "12 0 R" {
		t.Errorf("json stdout = %q (%v), want the report", stdout.String(), err)
	}
	if strings.Contains(stdout.String(), "\n  ") {
		t.Errorf("compact json is indented: %q", stdout.String())
	}

	stdout.Reset()
	if code := writeStructureDump(&stdout, &stderr, r, docViewFlags{json: true, pretty: true}); code != 0 {
		t.Fatalf("pretty exit %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "\n  ") {
		t.Errorf("--pretty json is not indented: %q", stdout.String())
	}

	stdout.Reset()
	if code := writeStructureDump(&stdout, &stderr, r, docViewFlags{}); code != 0 {
		t.Fatalf("plain exit %d, want 0", code)
	}
	if !strings.HasPrefix(stdout.String(), "Structure tree: 12 0 R\n") {
		t.Errorf("plain stdout = %q, want the text view", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestStructureDumpWriteFailureExits2(t *testing.T) {
	for _, jsonMode := range []bool{true, false} {
		var stderr bytes.Buffer
		code := writeStructureDump(failingWriter{}, &stderr, &pdfcore.StructureReport{}, docViewFlags{json: jsonMode})
		if code != 2 {
			t.Errorf("json=%v: exit %d, want 2", jsonMode, code)
		}
		var obj map[string]string
		if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &obj); err != nil || !strings.HasPrefix(obj["error"], "failed to write output") {
			t.Errorf("json=%v: stderr = %q, want one error object", jsonMode, stderr.String())
		}
	}
}

func TestStructureDumpOnFixtures(t *testing.T) {
	cases := []struct {
		name  string
		check func(t *testing.T, out string)
	}{
		{"structure.pdf", func(t *testing.T, out string) {
			if !strings.Contains(out, `ActualText "Hello world"`) || !strings.Contains(out, `Alt "Company logo"`) {
				t.Errorf("element text fields missing:\n%s", out)
			}
			if strings.Contains(out, "* = content count") {
				t.Errorf("clean file has an inconsistent row:\n%s", out)
			}
		}},
		{"structure-broken.pdf", func(t *testing.T, out string) {
			if !strings.Contains(out, "already listed; not walked again") || !strings.Contains(out, "* = content count") {
				t.Errorf("broken-file findings missing:\n%s", out)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var code int
			out := captureStdout(t, func() { code = execStructureDump(fixture(tc.name), docViewFlags{}) })
			if code != 0 {
				t.Fatalf("exit %d, want 0", code)
			}
			tc.check(t, out)
		})
	}
}

func TestStructurePlain_NonASCIIElementFieldsEscapedOnce(t *testing.T) {
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		Elements: []*pdfcore.StructKid{{Kind: "element", Element: &pdfcore.StructElem{
			S: "\u00dcberschrift", MappedS: "H1", Ref: "10 0 R", Alt: strPtr("Logo"),
		}}},
	}
	out := structurePlain(r)
	want := `"\u00dcberschrift" (H1) 10 0 R  Alt "Logo"`
	if !strings.Contains(out, "\n"+want+"\n") {
		t.Errorf("output missing line %q:\n%s", want, out)
	}
}

func TestStructurePlain_NonASCIIAlignmentCellKeepsColumnsAligned(t *testing.T) {
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		Pages: []*pdfcore.StructPage{{PageNum: 1, Alignment: []pdfcore.MCIDAlignment{
			{MCID: 0, ContentCount: 1, ParentTreeElement: "10 0 R", ParentTreeS: "\u00dcberschrift", TreeElements: []string{"10 0 R"}, Consistent: true},
		}}},
	}
	out := structurePlain(r)
	header := lineOf(t, out, "PARENTTREE")
	row := lineOf(t, out, "10 0 R")
	if !strings.Contains(row, `10 0 R "\u00dcberschrift"`) {
		t.Errorf("row %q does not carry the escaped /S cell", row)
	}
	if strings.HasPrefix(strings.TrimSpace(row), `"`) {
		t.Errorf("row %q is escaped as a whole", row)
	}
	if h, c := strings.LastIndex(header, "TREE"), strings.LastIndex(row, "10 0 R"); h != c {
		t.Errorf("TREE column at %d in header %q but at %d in row %q", h, header, c, row)
	}
}

func TestStructurePlain_ElementSectionOnlyWithStructTree(t *testing.T) {
	r := &pdfcore.StructureReport{Elements: []*pdfcore.StructKid{{Kind: "error", Err: "x"}}}
	if out := structurePlain(r); strings.Contains(out, "Elements (") {
		t.Errorf("element section printed with no structure tree:\n%s", out)
	}
}
