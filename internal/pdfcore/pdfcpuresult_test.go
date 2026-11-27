package pdfcore

import (
	"errors"
	"testing"

	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// docWithXRef returns a DocumentState whose displayed cross-reference table
// holds exactly entries.
func docWithXRef(entries map[int]*pdfcpu_model.XRefTableEntry) *DocumentState {
	return &DocumentState{PDFContext: &pdfcpu_model.Context{
		XRefTable: &pdfcpu_model.XRefTable{Table: entries},
	}}
}

func inUse(gen int) *pdfcpu_model.XRefTableEntry {
	return &pdfcpu_model.XRefTableEntry{Generation: &gen}
}

func notice(objNr int, message string) pdfcpu_model.ValidationNotice {
	return pdfcpu_model.ValidationNotice{
		Disposition:  pdfcpu_model.NoticeRepaired,
		Phase:        pdfcpu_model.NoticePhaseRead,
		Message:      message,
		ObjectNumber: objNr,
	}
}

func TestPdfcpuResult_ObjRefCarriesTheDisplayedNonZeroGeneration(t *testing.T) {
	doc := docWithXRef(map[int]*pdfcpu_model.XRefTableEntry{7: inUse(3)})
	run := pdfcpuRun{
		fatal:   pdfcpu_model.WithValidationErrorObject(errors.New("bad dict"), 7),
		notices: []pdfcpu_model.ValidationNotice{notice(7, "fixed length")},
	}
	res := pdfcpuResult(doc, run)
	assertBothRulesEvaluated(t, res)
	if len(res.Problems) != 2 {
		t.Fatalf("problems = %+v, want one fatal and one notice", res.Problems)
	}
	for _, p := range res.Problems {
		if p.ObjRef != "7 3 R" || p.ObjNodeID != "obj:3:7" {
			t.Errorf("%s problem objRef=%q objNodeId=%q, want 7 3 R and obj:3:7", p.RuleID, p.ObjRef, p.ObjNodeID)
		}
	}
	if res.Summary.Errors != 1 || res.Summary.Warnings != 1 {
		t.Errorf("summary = %+v, want 1 error and 1 warning", res.Summary)
	}
}

func TestPdfcpuResult_ObjectsNotInUseInTheDisplayedTableGetNoObjRef(t *testing.T) {
	entries := map[int]*pdfcpu_model.XRefTableEntry{
		4: {Free: true},
		5: {}, // in use, no generation recorded
	}
	run := pdfcpuRun{
		fatal: pdfcpu_model.WithValidationErrorObject(errors.New("missing object"), 9),
		notices: []pdfcpu_model.ValidationNotice{
			notice(4, "free entry"),
			notice(0, "no object"),
			notice(9, "missing entry"),
			notice(5, "nil generation"),
		},
	}
	cases := map[string]*DocumentState{
		"displayed table":    docWithXRef(entries),
		"no displayed table": {PDFContext: &pdfcpu_model.Context{}},
		"no context":         {},
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			res := pdfcpuResult(doc, run)
			if len(res.Problems) != 5 {
				t.Fatalf("problems = %+v, want 1 fatal and 4 notices", res.Problems)
			}
			for _, p := range res.Problems {
				if p.Message == "" {
					t.Errorf("%s problem has an empty message", p.RuleID)
				}
				wantRef, wantNode := "", ""
				if name == "displayed table" && p.Message == "repaired during read: nil generation" {
					wantRef, wantNode = "5 0 R", "obj:0:5"
				}
				if p.ObjRef != wantRef || p.ObjNodeID != wantNode {
					t.Errorf("%q objRef=%q objNodeId=%q, want %q and %q", p.Message, p.ObjRef, p.ObjNodeID, wantRef, wantNode)
				}
			}
			if got := res.Problems[0]; got.RuleID != "pdfcpu-validator" || got.Message != "missing object" {
				t.Errorf("first problem = %+v, want the pdfcpu-validator fatal with message %q", got, "missing object")
			}
		})
	}
}

func TestPdfcpuResult_FatalWithoutObjectAttributionHasNoObjRef(t *testing.T) {
	doc := docWithXRef(map[int]*pdfcpu_model.XRefTableEntry{1: inUse(0)})
	res := pdfcpuResult(doc, pdfcpuRun{fatal: errors.New("trailer broken")})
	assertBothRulesEvaluated(t, res)
	fatal := problemsOf(res, "pdfcpu-validator")
	if len(fatal) != 1 {
		t.Fatalf("pdfcpu-validator problems = %+v, want 1", fatal)
	}
	if fatal[0].Message != "trailer broken" || fatal[0].ObjRef != "" || fatal[0].ObjNodeID != "" || fatal[0].Severity != "error" {
		t.Errorf("fatal problem = %+v, want error %q with no objRef", fatal[0], "trailer broken")
	}
	if n := len(problemsOf(res, "pdfcpu-notices")); n != 0 {
		t.Errorf("pdfcpu-notices problems = %d, want 0", n)
	}
}

func TestPdfcpuResult_NoticeMessageAppendsPageAndPath(t *testing.T) {
	cases := []struct {
		name string
		page int
		path string
		want string
	}{
		{"neither", 0, "", "digested during validate: x"},
		{"page only", 3, "", "digested during validate: x (page 3)"},
		{"path only", 0, "Root.Pages.Kids[0]", "digested during validate: x (path Root.Pages.Kids[0])"},
		{"page and path", 2, "Resources.Font.F1", "digested during validate: x (page 2) (path Resources.Font.F1)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := pdfcpu_model.ValidationNotice{
				Disposition: pdfcpu_model.NoticeDigested,
				Phase:       pdfcpu_model.NoticePhaseValidate,
				Message:     "x",
				PageNumber:  tc.page,
				Path:        tc.path,
			}
			res := pdfcpuResult(&DocumentState{}, pdfcpuRun{notices: []pdfcpu_model.ValidationNotice{n}})
			got := problemsOf(res, "pdfcpu-notices")
			if len(got) != 1 || got[0].Message != tc.want {
				t.Fatalf("pdfcpu-notices problems = %+v, want one with message %q", got, tc.want)
			}
		})
	}
}

func TestPdfcpuResult_NoticesKeepReportOrder(t *testing.T) {
	run := pdfcpuRun{notices: []pdfcpu_model.ValidationNotice{notice(0, "first"), notice(0, "second"), notice(0, "third")}}
	got := problemsOf(pdfcpuResult(&DocumentState{}, run), "pdfcpu-notices")
	want := []string{"repaired during read: first", "repaired during read: second", "repaired during read: third"}
	if len(got) != len(want) {
		t.Fatalf("pdfcpu-notices problems = %+v, want %d", got, len(want))
	}
	for i, p := range got {
		if p.Message != want[i] {
			t.Errorf("notice %d = %q, want %q", i, p.Message, want[i])
		}
	}
}

func TestPdfcpuResult_DegradedParseFailureLeavesBothRulesUnevaluated(t *testing.T) {
	degraded := errors.New("pdfcpu could not parse the file: bad xref")
	res := pdfcpuResult(&DocumentState{}, pdfcpuRun{degraded: degraded})
	if len(res.Rules) != 2 {
		t.Fatalf("rules = %+v, want both pdfcpu rules", res.Rules)
	}
	for _, r := range res.Rules {
		if r.Evaluated {
			t.Errorf("rule %q reports evaluated after a parse failure", r.RuleID)
		}
		got := problemsOf(res, r.RuleID)
		want := `rule "` + r.RuleID + `" could not be evaluated: pdfcpu could not parse the file: bad xref`
		if len(got) != 1 || got[0].Severity != "info" || got[0].Message != want || got[0].ObjRef != "" {
			t.Errorf("rule %q problems = %+v, want one info %q with no objRef", r.RuleID, got, want)
		}
	}
	if res.Summary.Errors != 0 || res.Summary.Warnings != 0 || res.Summary.Info != 2 {
		t.Errorf("summary = %+v, want 2 info only", res.Summary)
	}
}
