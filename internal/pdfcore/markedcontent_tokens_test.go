package pdfcore

import (
	"errors"
	"strings"
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// scanMarkedContent driven directly on tokenizer output, without a document.

// noProps is a property lookup for streams that name no property list.
func noProps(t *testing.T) propertyLookup {
	return func(name string) (pdfcpu_types.Object, error) {
		t.Errorf("unexpected property list lookup for /%s", name)
		return nil, nil
	}
}

// scanTokens scans each content in order with one carried stack and returns
// every row and the per-stream counters.
func scanTokens(props propertyLookup, contents ...string) ([]ContentMCID, []MarkedStream) {
	st := &mcState{}
	var rows []ContentMCID
	streams := make([]MarkedStream, len(contents))
	for i, c := range contents {
		rows = append(rows, scanMarkedContent(tokenizeContentStream(c), i, props, st, &streams[i])...)
	}
	return rows, streams
}

func rowParent(c ContentMCID) int {
	if c.ParentMCID == nil {
		return -1
	}
	return *c.ParentMCID
}

func TestScanMarkedContentTokens_InlinePropertyLists(t *testing.T) {
	cases := []struct {
		name    string
		content string
		mcid    int
		tag     string
	}{
		{"no spaces", "/P<</MCID 0>>BDC EMC", 0, "P"},
		{"spaces", "/P << /MCID 7 >> BDC EMC", 7, "P"},
		{"booleans, null and an array before the MCID", "/Span <</X true /Y [1 2] /Z null /W false /MCID 2>> BDC EMC", 2, "Span"},
		{"nested dict before the MCID", "/P <</A <</B <</C 1>> >> /MCID 3>> BDC EMC", 3, "P"},
		{"array of dicts before the MCID", "/P <</A [<</B 1>> <</C 2>>] /MCID 4>> BDC EMC", 4, "P"},
		{"comment inside the operands", "/P % note\n<</MCID 5>> BDC EMC", 5, "P"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, streams := scanTokens(noProps(t), tc.content)
			if len(rows) != 1 {
				t.Fatalf("rows = %+v, want one", rows)
			}
			r := rows[0]
			if r.MCID != tc.mcid || r.Tag != tc.tag || r.Err != "" || r.Depth != 1 || r.ParentMCID != nil {
				t.Errorf("row = %+v, want MCID %d tag %s depth 1, no parent, no error", r, tc.mcid, tc.tag)
			}
			checkStream(t, streams[0], 1, 1, 0, 0)
		})
	}
}

func TestScanMarkedContentTokens_InlineDictWithoutTopLevelMCIDHasNoRow(t *testing.T) {
	cases := []struct {
		name    string
		content string
		depth   int
	}{
		{"no MCID key", "/P <</X 1>> BDC EMC", 0},
		{"MCID only inside a nested dict", "/P <</A <</MCID 9>> >> BDC EMC", 0},
		{"empty dict", "/P <<>> BDC EMC", 0},
		{"dict never closed", "/P << /X 1 BDC", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, streams := scanTokens(noProps(t), tc.content)
			if len(rows) != 0 {
				t.Errorf("rows = %+v, want none", rows)
			}
			if streams[0].Opens != 1 || streams[0].DepthAtEnd != tc.depth {
				t.Errorf("stream = %+v, want 1 open and depth %d at end", streams[0], tc.depth)
			}
		})
	}
}

func TestScanMarkedContentTokens_BadInlineMCIDIsAnErrorRow(t *testing.T) {
	cases := []struct {
		name    string
		content string
		err     string
	}{
		{"name", "/P <</MCID /X>> BDC EMC", "/MCID /X is not an integer"},
		{"real", "/P <</MCID 1.5>> BDC EMC", "/MCID 1.5 is not an integer"},
		{"array", "/P <</MCID [1]>> BDC EMC", "/MCID [ is not an integer"},
		{"negative", "/P <</MCID -2>> BDC EMC", "/MCID -2 is negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, _ := scanTokens(noProps(t), tc.content)
			if len(rows) != 1 || rows[0].MCID != -1 || rows[0].Err != tc.err {
				t.Errorf("rows = %+v, want one row with MCID -1 and error %q", rows, tc.err)
			}
		})
	}
}

func TestScanMarkedContentTokens_NamedPropertyLists(t *testing.T) {
	lists := map[string]pdfcpu_types.Object{
		"Int":  pdfcpu_types.Integer(4),
		"Neg":  pdfcpu_types.Integer(-1),
		"Real": pdfcpu_types.Float(2.5),
		"Str":  pdfcpu_types.StringLiteral("x"),
		"None": nil,
	}
	var asked []string
	props := func(name string) (pdfcpu_types.Object, error) {
		asked = append(asked, name)
		v, ok := lists[name]
		if !ok {
			return nil, errors.New("property list /" + name + " not found")
		}
		return v, nil
	}
	cases := []struct {
		name string
		mcid int
		err  string
		row  bool
	}{
		{"Int", 4, "", true},
		{"Neg", -1, "/MCID -1 is negative", true},
		{"Real", -1, "/MCID in property list /Real is a real, not an integer", true},
		{"Str", -1, "/MCID in property list /Str is a string, not an integer", true},
		{"None", 0, "", false},
		{"Missing", -1, "property list /Missing not found", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asked = nil
			rows, streams := scanTokens(props, "/Span /"+tc.name+" BDC EMC")
			if len(asked) != 1 || asked[0] != tc.name {
				t.Errorf("lookups = %v, want [%s] (leading slash stripped)", asked, tc.name)
			}
			checkStream(t, streams[0], 1, 1, 0, 0)
			if !tc.row {
				if len(rows) != 0 {
					t.Errorf("rows = %+v, want none for a property list with no /MCID", rows)
				}
				return
			}
			if len(rows) != 1 {
				t.Fatalf("rows = %+v, want one", rows)
			}
			if rows[0].MCID != tc.mcid || rows[0].Err != tc.err || rows[0].Tag != "Span" {
				t.Errorf("row = %+v, want MCID %d tag Span error %q", rows[0], tc.mcid, tc.err)
			}
		})
	}
}

func TestScanMarkedContentTokens_Nesting(t *testing.T) {
	cases := []struct {
		name    string
		content string
		depths  []int
		parents []int
	}{
		{"MCID inside BMC has no parent MCID", "/A BMC /P <</MCID 0>> BDC EMC EMC", []int{2}, []int{-1}},
		{"MCID inside MCID", "/P <</MCID 0>> BDC /Span <</MCID 1>> BDC EMC EMC", []int{1, 2}, []int{-1, 0}},
		{"innermost MCID wins over an outer one", "/P <</MCID 0>> BDC /Q <</MCID 1>> BDC /A BMC /S <</MCID 2>> BDC EMC EMC EMC EMC", []int{1, 2, 4}, []int{-1, 0, 1}},
		{"error row is no parent", "/P <</MCID -1>> BDC /Span <</MCID 1>> BDC EMC EMC", []int{1, 2}, []int{-1, -1}},
		{"BDC with no MCID is no parent", "/P <</X 1>> BDC /Span <</MCID 1>> BDC EMC EMC", []int{2}, []int{-1}},
		{"sibling after close has no parent", "/P <</MCID 0>> BDC EMC /P <</MCID 1>> BDC EMC", []int{1, 1}, []int{-1, -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, streams := scanTokens(noProps(t), tc.content)
			if len(rows) != len(tc.depths) {
				t.Fatalf("rows = %+v, want %d", rows, len(tc.depths))
			}
			for i, r := range rows {
				if r.Depth != tc.depths[i] || rowParent(r) != tc.parents[i] {
					t.Errorf("row %d = depth %d parent %d, want depth %d parent %d", i, r.Depth, rowParent(r), tc.depths[i], tc.parents[i])
				}
			}
			if streams[0].DepthAtEnd != 0 || streams[0].MinDepth != 0 || streams[0].Opens != streams[0].Closes {
				t.Errorf("stream = %+v, want balanced", streams[0])
			}
		})
	}
}

func TestScanMarkedContentTokens_EMCOnEmptyStack(t *testing.T) {
	rows, streams := scanTokens(noProps(t), "EMC EMC /P <</MCID 0>> BDC EMC")
	checkStream(t, streams[0], 1, 3, 0, -1)
	if len(rows) != 1 || rows[0].Depth != 1 {
		t.Errorf("rows = %+v, want MCID 0 at depth 1 (an unmatched EMC pops nothing)", rows)
	}
}

func TestScanMarkedContentTokens_StackCarriedAcrossStreams(t *testing.T) {
	rows, streams := scanTokens(noProps(t),
		"/P <</MCID 0>> BDC",
		"/Span <</MCID 1>> BDC EMC",
		"EMC",
	)
	checkStream(t, streams[0], 1, 0, 1, 0)
	checkStream(t, streams[1], 1, 1, 1, 1)
	checkStream(t, streams[2], 0, 1, 0, 0)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want two", rows)
	}
	if rows[1].Stream != 1 || rows[1].Depth != 2 || rowParent(rows[1]) != 0 {
		t.Errorf("second row = %+v, want stream 1, depth 2, parent MCID 0 from the earlier stream", rows[1])
	}
}

func TestScanMarkedContentTokens_InlineImagePayloadIsNotScanned(t *testing.T) {
	rows, streams := scanTokens(noProps(t), "BI /W 1 /H 1 /BPC 8 /CS /G ID /P <</MCID 0>> BDC EMC EI /Q <</MCID 1>> BDC EMC")
	if len(rows) != 1 || rows[0].MCID != 1 {
		t.Errorf("rows = %+v, want only MCID 1 from outside the image data", rows)
	}
	checkStream(t, streams[0], 1, 1, 0, 0)
}

func TestScanMarkedContentTokens_RowPositionIsTheTag(t *testing.T) {
	rows, _ := scanTokens(noProps(t), "q\n  /P <</MCID 0>> BDC EMC Q")
	if len(rows) != 1 || rows[0].Line != 2 || rows[0].Col != 3 {
		t.Errorf("rows = %+v, want line 2 col 3 (the tag)", rows)
	}
	rows, _ = scanTokens(noProps(t), "1 2\n /P <</MCID 0>> BDC EMC")
	if len(rows) != 1 || rows[0].Line != 2 || rows[0].Col != 2 {
		t.Errorf("rows = %+v, want line 2 col 2 (the tag, not the stray operands)", rows)
	}
	rows, _ = scanTokens(noProps(t), "  BDC")
	if len(rows) != 1 || rows[0].Line != 1 || rows[0].Col != 3 || !strings.Contains(rows[0].Err, "0 of its 2 operands") {
		t.Errorf("rows = %+v, want the BDC position when it has no operands", rows)
	}
}

func TestScanMarkedContentTokens_StrayLeadingOperandsKeepTheMCID(t *testing.T) {
	cases := []struct {
		name    string
		content string
		mcid    int
		tag     string
	}{
		{"names before the tag", "/X /P <</MCID 0>> BDC EMC", 0, "P"},
		{"numbers before the tag", "1 2 3 /Span <</MCID 4>> BDC EMC", 4, "Span"},
		{"unclosed array before the tag", "[ /P <</MCID 2>> BDC EMC", 2, "P"},
		{"unclosed dict before the tag", "<< /P <</MCID 5>> BDC EMC", 5, "P"},
		{"name and unclosed dict before the tag", "/X << /P <</MCID 8>> BDC EMC", 8, "P"},
		{"stray closer before the tag", "] /P <</MCID 6>> BDC EMC", 6, "P"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, streams := scanTokens(noProps(t), tc.content)
			if len(rows) != 1 || rows[0].MCID != tc.mcid || rows[0].Tag != tc.tag || rows[0].Err != "" {
				t.Errorf("rows = %+v, want MCID %d tag %s with no error", rows, tc.mcid, tc.tag)
			}
			checkStream(t, streams[0], 1, 1, 0, 0)
		})
	}
}

func TestScanMarkedContentTokens_UnclosedPropertyListKeepsMCIDWithError(t *testing.T) {
	cases := []struct {
		name    string
		content string
		mcid    int
	}{
		{"property list dict never closed", "/P << /MCID 3 BDC EMC", 3},
		{"unclosed array then unclosed dict", "[ /P << /MCID 7 BDC EMC", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, streams := scanTokens(noProps(t), tc.content)
			if len(rows) != 1 || rows[0].MCID != tc.mcid || rows[0].Tag != "P" || !strings.Contains(rows[0].Err, "no closing >>") {
				t.Errorf("rows = %+v, want MCID %d tag P with a no-closing->> error", rows, tc.mcid)
			}
			checkStream(t, streams[0], 1, 1, 0, 0)
		})
	}
}

func TestScanMarkedContentTokens_UnclosedPropertyListMCIDIsParentOfNestedBDC(t *testing.T) {
	rows, _ := scanTokens(noProps(t), "/P << /MCID 3 BDC /Span <</MCID 4>> BDC EMC EMC")
	if len(rows) != 2 || rows[0].Err == "" || rows[1].Err != "" {
		t.Fatalf("rows = %+v, want an error row for MCID 3 and a clean row for MCID 4", rows)
	}
	if rowParent(rows[1]) != 3 {
		t.Errorf("MCID 4 parent = %d, want 3", rowParent(rows[1]))
	}
}

func TestScanMarkedContentTokens_TagThatIsAContainerIsAnErrorRow(t *testing.T) {
	rows, _ := scanTokens(noProps(t), "/X [/P] <</MCID 0>> BDC EMC")
	if len(rows) != 1 || rows[0].MCID != -1 || !strings.Contains(rows[0].Err, "not a tag name") {
		t.Errorf("rows = %+v, want one not-a-tag-name error row", rows)
	}
}

func TestScanMarkedContentTokens_OperandsCarryAcrossStreams(t *testing.T) {
	rows, streams := scanTokens(noProps(t), "q /P", "<</MCID 0", ">> BDC EMC Q")
	if len(rows) != 1 || rows[0].MCID != 0 || rows[0].Tag != "P" || rows[0].Err != "" {
		t.Fatalf("rows = %+v, want MCID 0 tag P from operands split over three streams", rows)
	}
	if r := rows[0]; r.Stream != 0 || r.Line != 1 || r.Col != 3 || r.Depth != 1 {
		t.Errorf("row = %+v, want stream 0 line 1 col 3 (the tag) at depth 1", r)
	}
	checkStream(t, streams[0], 0, 0, 0, 0)
	checkStream(t, streams[1], 0, 0, 0, 0)
	checkStream(t, streams[2], 1, 1, 0, 0)
}

func TestScanMarkedContentTokens_OtherOperatorsResetOperands(t *testing.T) {
	rows, streams := scanTokens(noProps(t), "/P <</MCID 0>> Tj BDC EMC")
	if len(rows) != 1 || !strings.Contains(rows[0].Err, "0 of its 2 operands") {
		t.Errorf("rows = %+v, want an operand-count error row after Tj cleared the operands", rows)
	}
	checkStream(t, streams[0], 1, 1, 0, 0)
}
