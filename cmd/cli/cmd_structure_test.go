package main

import (
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

func TestStructurePlain_TreeClaimOnUnmarkedPageIsShown(t *testing.T) {
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		Pages: []*pdfcore.StructPage{
			{PageNum: 1, Streams: []pdfcore.MarkedStream{{Ref: "5 0 R"}}, Alignment: []pdfcore.MCIDAlignment{
				{MCID: 0, TreeElements: []string{"10 0 R"}},
			}},
			{PageNum: 2, Streams: []pdfcore.MarkedStream{{Ref: "6 0 R"}}, Alignment: []pdfcore.MCIDAlignment{}},
		},
	}
	out := structurePlain(r)
	if !strings.Contains(out, "Page 1  ") {
		t.Errorf("page with a tree claim but no /StructParents or marked content has no block:\n%s", out)
	}
	if !strings.Contains(out, "10 0 R  *") {
		t.Errorf("tree-only alignment row is not shown as inconsistent:\n%s", out)
	}
	if !strings.Contains(out, "no marked content: 2\n") {
		t.Errorf("empty page 2 is not collapsed:\n%s", out)
	}
}

func TestStructurePlain_ParentTreeSlotErrorIsShown(t *testing.T) {
	sp := 0
	r := &pdfcore.StructureReport{
		HasStructTree: true,
		Pages: []*pdfcore.StructPage{
			{PageNum: 1, StructParents: &sp, Streams: []pdfcore.MarkedStream{{Ref: "5 0 R"}},
				ParentTree: []pdfcore.ParentTreeSlot{
					{MCID: 0, ElementRef: "10 0 R", S: "P"},
					{MCID: 1, ElementRef: "11 0 R", Err: "11 0 R is an integer, not an element"},
				},
				Alignment: []pdfcore.MCIDAlignment{
					{MCID: 0, ParentTreeElement: "10 0 R", ParentTreeS: "P", TreeElements: []string{}},
					{MCID: 1, TreeElements: []string{}},
				}},
		},
	}
	out := structurePlain(r)
	if !strings.Contains(out, "ParentTree slot 1: error: 11 0 R is an integer, not an element\n") {
		t.Errorf("ParentTree slot error is not shown:\n%s", out)
	}
	if strings.Contains(out, "ParentTree slot 0:") {
		t.Errorf("readable ParentTree slot printed as an error:\n%s", out)
	}
}
