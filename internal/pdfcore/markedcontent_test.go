package pdfcore

import (
	"strings"
	"testing"
)

// Marked-content scanning, read through GetStructure on one-page documents
// with no structure tree, so only the content side of the report is filled.

// scanPage returns page 1 of the structure report for a markedPDF document.
func scanPage(t *testing.T, resources string, streams []string, extra ...rawObj) *StructPage {
	t.Helper()
	r := rawStructure(t, markedPDF(resources, streams, extra...))
	if r.HasStructTree {
		t.Error("HasStructTree = true for a document with no /StructTreeRoot")
	}
	if len(r.Pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(r.Pages))
	}
	p := r.Pages[0]
	if len(p.Streams) != len(streams) {
		t.Fatalf("got %d stream rows, want %d", len(p.Streams), len(streams))
	}
	return p
}

// mcidRow is the part of a ContentMCID the scan tests compare.
type mcidRow struct {
	mcid   int
	tag    string
	stream int
	depth  int
	parent int // -1 for no enclosing MCID
	err    string
}

func checkContentMCIDs(t *testing.T, p *StructPage, want []mcidRow) {
	t.Helper()
	if len(p.ContentMCIDs) != len(want) {
		t.Fatalf("ContentMCIDs = %+v, want %d rows", p.ContentMCIDs, len(want))
	}
	for i, w := range want {
		c := p.ContentMCIDs[i]
		parent := -1
		if c.ParentMCID != nil {
			parent = *c.ParentMCID
		}
		if c.MCID != w.mcid || c.Stream != w.stream || c.Depth != w.depth || parent != w.parent {
			t.Errorf("row %d = MCID %d stream %d depth %d parent %d, want MCID %d stream %d depth %d parent %d",
				i, c.MCID, c.Stream, c.Depth, parent, w.mcid, w.stream, w.depth, w.parent)
		}
		if w.tag != "" && c.Tag != w.tag {
			t.Errorf("row %d tag = %q, want %q", i, c.Tag, w.tag)
		}
		switch {
		case w.err == "" && c.Err != "":
			t.Errorf("row %d: unexpected error %q", i, c.Err)
		case w.err != "" && !strings.Contains(c.Err, w.err):
			t.Errorf("row %d: error %q, want it to contain %q", i, c.Err, w.err)
		}
	}
}

func checkStream(t *testing.T, s MarkedStream, opens, closes, depthAtEnd, minDepth int) {
	t.Helper()
	if s.Opens != opens || s.Closes != closes || s.DepthAtEnd != depthAtEnd || s.MinDepth != minDepth {
		t.Errorf("stream = opens %d closes %d depthAtEnd %d minDepth %d, want %d %d %d %d",
			s.Opens, s.Closes, s.DepthAtEnd, s.MinDepth, opens, closes, depthAtEnd, minDepth)
	}
}

func TestMarkedContentScan_PropertyLists(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []mcidRow
	}{
		{"inline dict with spaces", "/P << /MCID 0 >> BDC (x) Tj EMC",
			[]mcidRow{{0, "P", 0, 1, -1, ""}}},
		{"inline dict without spaces", "/P<</MCID 0>>BDC EMC",
			[]mcidRow{{0, "P", 0, 1, -1, ""}}},
		{"booleans, null and an array in the property list", "/Span <</MCID 2 /X true /Y [1 2] /Z null /W false>> BDC EMC",
			[]mcidRow{{2, "Span", 0, 1, -1, ""}}},
		{"nested dict in the property list", "/Span <</A <</MCID 9>> /MCID 4>> BDC EMC",
			[]mcidRow{{4, "Span", 0, 1, -1, ""}}},
		{"property list with no /MCID", "/Artifact <</Type /Pagination>> BDC EMC",
			nil},
		{"/MCID not an integer", "/P <</MCID (a)>> BDC EMC",
			[]mcidRow{{-1, "P", 0, 1, -1, "MCID"}}},
		{"negative /MCID", "/P <</MCID -4>> BDC EMC",
			[]mcidRow{{-1, "P", 0, 1, -1, "MCID"}}},
		{"comment ignored", "% /P <</MCID 5>> BDC\n/P <</MCID 0>> BDC EMC",
			[]mcidRow{{0, "P", 0, 1, -1, ""}}},
		{"BDC inside an inline image payload", "BI /W 1 /H 1 /CS /G /BPC 8 ID /P <</MCID 7>> BDC\nEI /P <</MCID 0>> BDC EMC",
			[]mcidRow{{0, "P", 0, 1, -1, ""}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := scanPage(t, "", []string{rawStream(tc.content)})
			checkContentMCIDs(t, p, tc.want)
			checkStream(t, p.Streams[0], 1, 1, 0, 0)
		})
	}
}

func TestMarkedContentScan_NamedPropertyLists(t *testing.T) {
	t.Run("found in /Resources /Properties", func(t *testing.T) {
		p := scanPage(t, "<< /Properties << /MC0 << /MCID 3 >> >> >>", []string{rawStream("/Span /MC0 BDC EMC")})
		checkContentMCIDs(t, p, []mcidRow{{3, "Span", 0, 1, -1, ""}})
	})
	t.Run("found through indirect /Properties and entry", func(t *testing.T) {
		p := scanPage(t, "<< /Properties 6 0 R >>", []string{rawStream("/Span /MC0 BDC EMC")},
			rawObj{6, "<< /MC0 7 0 R >>"}, rawObj{7, "<< /MCID 3 >>"})
		checkContentMCIDs(t, p, []mcidRow{{3, "Span", 0, 1, -1, ""}})
	})
	t.Run("missing", func(t *testing.T) {
		p := scanPage(t, "<< /Properties << /MC0 << /MCID 3 >> >> >>", []string{rawStream("/Span /MC9 BDC EMC")})
		if len(p.ContentMCIDs) != 1 || !strings.Contains(p.ContentMCIDs[0].Err, "MC9") {
			t.Errorf("ContentMCIDs = %+v, want one row whose error names MC9", p.ContentMCIDs)
		}
		checkStream(t, p.Streams[0], 1, 1, 0, 0)
	})
}

func TestMarkedContentScan_Nesting(t *testing.T) {
	t.Run("BMC around an MCID sequence", func(t *testing.T) {
		p := scanPage(t, "", []string{rawStream("/Artifact BMC /P <</MCID 0>> BDC EMC EMC")})
		checkContentMCIDs(t, p, []mcidRow{{0, "P", 0, 2, -1, ""}})
		checkStream(t, p.Streams[0], 2, 2, 0, 0)
	})
	t.Run("MCID inside MCID", func(t *testing.T) {
		p := scanPage(t, "", []string{rawStream("/P <</MCID 0>> BDC /Span <</MCID 1>> BDC EMC EMC")})
		checkContentMCIDs(t, p, []mcidRow{{0, "P", 0, 1, -1, ""}, {1, "Span", 0, 2, 0, ""}})
	})
	t.Run("innermost enclosing MCID through a BMC", func(t *testing.T) {
		p := scanPage(t, "", []string{rawStream("/P <</MCID 0>> BDC /X BMC /Span <</MCID 1>> BDC EMC EMC EMC")})
		checkContentMCIDs(t, p, []mcidRow{{0, "P", 0, 1, -1, ""}, {1, "Span", 0, 3, 0, ""}})
		checkStream(t, p.Streams[0], 3, 3, 0, 0)
	})
	t.Run("EMC with nothing open", func(t *testing.T) {
		p := scanPage(t, "", []string{rawStream("EMC /P <</MCID 0>> BDC EMC")})
		checkContentMCIDs(t, p, []mcidRow{{0, "P", 0, 1, -1, ""}})
		checkStream(t, p.Streams[0], 1, 2, 0, -1)
	})
	t.Run("BDC with no EMC", func(t *testing.T) {
		p := scanPage(t, "", []string{rawStream("/P <</MCID 0>> BDC (x) Tj")})
		checkStream(t, p.Streams[0], 1, 0, 1, 0)
	})
}

func TestMarkedContentScan_StackCarriedAcrossStreams(t *testing.T) {
	p := scanPage(t, "", []string{
		rawStream("/P <</MCID 0>> BDC (x) Tj"),
		rawStream("/Span <</MCID 1>> BDC EMC EMC"),
	})
	checkContentMCIDs(t, p, []mcidRow{{0, "P", 0, 1, -1, ""}, {1, "Span", 1, 2, 0, ""}})
	checkStream(t, p.Streams[0], 1, 0, 1, 0)
	checkStream(t, p.Streams[1], 1, 2, 0, 0)
}

func TestMarkedContentScan_UndecodableStreamIsSkipped(t *testing.T) {
	p := scanPage(t, "", []string{
		rawStream("/P <</MCID 0>> BDC"),
		undecodableStream,
		rawStream("EMC /Span <</MCID 1>> BDC EMC"),
	})
	if p.Streams[1].Err == "" {
		t.Errorf("undecodable stream row = %+v, want an error", p.Streams[1])
	}
	if p.Streams[1].Opens != 0 || p.Streams[1].Closes != 0 {
		t.Errorf("undecodable stream row = %+v, want nothing counted", p.Streams[1])
	}
	if p.Streams[0].Err != "" || p.Streams[2].Err != "" {
		t.Errorf("decodable streams carry errors: %+v, %+v", p.Streams[0], p.Streams[2])
	}
	checkContentMCIDs(t, p, []mcidRow{{0, "P", 0, 1, -1, ""}, {1, "Span", 2, 1, -1, ""}})
	checkStream(t, p.Streams[2], 1, 2, 0, 0)
}

func TestMarkedContentScan_StreamRefsInContentsOrder(t *testing.T) {
	p := scanPage(t, "", []string{rawStream("q Q"), rawStream("q Q")})
	if p.Streams[0].Ref != "20 0 R" || p.Streams[1].Ref != "21 0 R" {
		t.Errorf("stream refs = %q, %q, want 20 0 R, 21 0 R", p.Streams[0].Ref, p.Streams[1].Ref)
	}
	if len(p.ContentMCIDs) != 0 || len(p.Alignment) != 0 {
		t.Errorf("page with no marked content has ContentMCIDs %+v, Alignment %+v", p.ContentMCIDs, p.Alignment)
	}
}

func TestMarkedContentScan_AlignmentWithoutStructureTree(t *testing.T) {
	p := scanPage(t, "", []string{rawStream("/P <</MCID 1>> BDC EMC /P <</MCID 0>> BDC EMC /P <</MCID 1>> BDC EMC")})
	if p.StructParents != nil || len(p.ParentTree) != 0 || p.ParentTreeErr != "" {
		t.Errorf("page with no /StructParents: StructParents %v ParentTree %+v ParentTreeErr %q", p.StructParents, p.ParentTree, p.ParentTreeErr)
	}
	if got := alignmentMCIDs(p); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("alignment MCIDs = %v, want [0 1] ascending", got)
	}
	if a := p.Alignment[1]; a.ContentCount != 2 || a.ParentTreeElement != "" || len(a.TreeElements) != 0 || a.Consistent {
		t.Errorf("MCID 1 row = %+v, want content 2, no ParentTree element, no tree claimant, not consistent", a)
	}
}

func TestMarkedContentScan_StrayOpenSpanEndsAtNextOperator(t *testing.T) {
	p := scanPage(t, "", []string{rawStream("[ (x) Tj /P <</MCID 0>> BDC EMC")})
	checkContentMCIDs(t, p, []mcidRow{{0, "P", 0, 1, -1, ""}})
	checkStream(t, p.Streams[0], 1, 1, 0, 0)
}

func TestMarkedContentScan_BDCWithoutTagIsAnErrorRow(t *testing.T) {
	p := scanPage(t, "<< /Properties << /MCID << /MCID 5 >> >> >>", []string{rawStream("<</MCID 0>> BDC EMC")})
	checkContentMCIDs(t, p, []mcidRow{{-1, "", 0, 1, -1, "not a tag name"}})
	checkStream(t, p.Streams[0], 1, 1, 0, 0)
}

func TestMarkedContentScan_DanglingMCIDInNamedPropertyListIsAnErrorRow(t *testing.T) {
	p := scanPage(t, "<< /Properties << /MC0 << /MCID 99 0 R >> >> >>", []string{rawStream("/Span /MC0 BDC EMC")})
	checkContentMCIDs(t, p, []mcidRow{{-1, "Span", 0, 1, -1, "does not resolve"}})
}

func TestMarkedContentScan_BDCWithUnreadableOperandsIsAnErrorRow(t *testing.T) {
	cases := []struct {
		name    string
		content string
		err     string
	}{
		{"tag only", "/P BDC EMC", "1 of its 2 operands"},
		{"no operands", "BDC EMC", "0 of its 2 operands"},
		{"number as property list", "/P 5 BDC EMC", "not a dictionary or a name"},
		{"array as property list", "/P [1] BDC EMC", "not a dictionary or a name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := scanPage(t, "", []string{rawStream(tc.content)})
			tag := ""
			if strings.HasPrefix(tc.content, "/P") {
				tag = "P"
			}
			checkContentMCIDs(t, p, []mcidRow{{-1, tag, 0, 1, -1, tc.err}})
			checkStream(t, p.Streams[0], 1, 1, 0, 0)
		})
	}
}
