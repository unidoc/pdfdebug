package pdfcore

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The structure walk of a validate run, driven through the walker factory
// that Inspector.validate takes.

// uaWalkPDF is a two-page tagged document with clean marked content.
func uaWalkPDF() []byte {
	return rawPDF(
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /Lang (en-US) /StructTreeRoot 5 0 R >>"},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 20 0 R /StructParents 0 >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 21 0 R /StructParents 1 >>"},
		rawObj{5, "<< /Type /StructTreeRoot /K [10 0 R] /ParentTree 6 0 R /ParentTreeNextKey 2 >>"},
		rawObj{6, "<< /Nums [0 [11 0 R] 1 [12 0 R]] >>"},
		rawObj{10, "<< /Type /StructElem /S /Document /P 5 0 R /K [11 0 R 12 0 R] >>"},
		rawObj{11, "<< /Type /StructElem /S /P /P 10 0 R /Pg 3 0 R /K 0 >>"},
		rawObj{12, "<< /Type /StructElem /S /P /P 10 0 R /Pg 4 0 R /K 0 >>"},
		rawObj{20, rawStream("/P <</MCID 0>> BDC EMC")},
		rawObj{21, rawStream("/P <</MCID 0>> BDC EMC")},
	)
}

// walkerWith returns a walker factory whose walkers run hook before each
// pdfMu acquisition.
func walkerWith(hook func(page int)) func(*pdfcpu_model.Context) *structureWalker {
	return func(ctx *pdfcpu_model.Context) *structureWalker {
		w := newStructureWalker(ctx)
		w.beforeLock = hook
		return w
	}
}

// requireLockFree fails when GetPageIndex on tabID does not return promptly,
// which it cannot do while pdfMu is still held.
func requireLockFree(t *testing.T, ins *Inspector, tabID string) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := ins.GetPageIndex(tabID)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("GetPageIndex after the run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GetPageIndex blocked after the run; pdfMu was not released")
	}
}

func TestValidateUA_WalkPanicLeavesStructureRulesNotEvaluated(t *testing.T) {
	cases := map[string]func(){
		"plain value": func() { panic("walk boom") },
		"runtime error": func() {
			var s []int
			_ = s[len(s)]
		},
	}
	for name, boom := range cases {
		t.Run(name, func(t *testing.T) {
			ins, _ := openUnvalidated(t, uaWalkPDF())
			res, err := ins.validate("raw", ProfilePDFUA1Structural, walkerWith(func(page int) {
				if page == 1 {
					boom()
				}
			}))
			if err != nil {
				t.Fatalf("validate returned %v, want a result with the structure rules degraded", err)
			}
			for _, id := range uaStructureRuleIDs {
				r := uaRuleOf(t, res, id)
				if r.Evaluated || r.Findings != 0 {
					t.Errorf("rule %s = %+v, want not evaluated", id, r)
				}
				ps := uaFindings(res, id)
				if len(ps) != 1 || ps[0].Severity != "info" || !strings.Contains(ps[0].Message, "structure walk failed") {
					t.Errorf("rule %s problems = %+v, want one info problem naming the structure walk", id, ps)
				}
			}
			for _, id := range uaCatalogRuleIDs {
				if r := uaRuleOf(t, res, id); !r.Evaluated || r.Findings != 0 {
					t.Errorf("catalog rule %s = %+v, want evaluated with 0 findings", id, r)
				}
			}
			requireLockFree(t, ins, "raw")
		})
	}
}

func TestValidateUA_UnreadableCatalogWinsOverWalkPanic(t *testing.T) {
	ins, doc := openUnvalidated(t, uaWalkPDF())
	walked := false
	res, err := ins.validate("raw", ProfilePDFUA1Structural, walkerWith(func(page int) {
		if page == 1 {
			walked = true
			doc.PDFContext.XRefTable.RootDict = nil
			doc.PDFContext.XRefTable.Root = nil
			panic("walk boom")
		}
	}))
	if err != nil {
		t.Fatalf("validate returned %v, want a result with every rule degraded", err)
	}
	if !walked {
		t.Fatal("walk never reached page 1; the panic was not exercised")
	}
	if len(res.Rules) != 8 {
		t.Fatalf("got %d rules, want 8", len(res.Rules))
	}
	for _, r := range res.Rules {
		if r.Evaluated || r.Findings != 0 {
			t.Errorf("rule %s = %+v, want not evaluated", r.RuleID, r)
		}
		ps := uaFindings(res, r.RuleID)
		if len(ps) != 1 || ps[0].Severity != "info" || !strings.Contains(ps[0].Message, "catalog") || strings.Contains(ps[0].Message, "structure walk failed") {
			t.Errorf("rule %s problems = %+v, want one info problem naming the catalog, not the walk", r.RuleID, ps)
		}
	}
	requireLockFree(t, ins, "raw")
}

func TestValidateUA_CloseDuringWalkCancelsTheRun(t *testing.T) {
	ins, _ := openUnvalidated(t, uaWalkPDF())
	res, err := ins.validate("raw", ProfilePDFUA1Structural, walkerWith(func(page int) {
		if page == 1 {
			if err := ins.Close("raw"); err != nil {
				t.Errorf("Close: %v", err)
			}
		}
	}))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("validate = %+v, %v; want an error satisfying errors.Is(err, context.Canceled)", res, err)
	}
}

func TestValidate_WalkRunsOncePerUARunAndNeverForPDFA(t *testing.T) {
	ins, _ := openUnvalidated(t, uaWalkPDF())
	calls := 0
	counting := func(ctx *pdfcpu_model.Context) *structureWalker {
		calls++
		return newStructureWalker(ctx)
	}

	if _, err := ins.validate("raw", ProfilePDFA1B, counting); err != nil {
		t.Fatalf("validate pdfa-1b: %v", err)
	}
	if calls != 0 {
		t.Errorf("a pdfa-1b run built the structure report %d times, want 0", calls)
	}

	res, err := ins.validate("raw", ProfilePDFUA1Structural, counting)
	if err != nil {
		t.Fatalf("validate pdfua-1-structural: %v", err)
	}
	if calls != 1 {
		t.Errorf("a pdfua-1-structural run built the structure report %d times, want 1", calls)
	}
	uaExpect(t, res, nil)
}

func TestValidateUA_WalkRunsWithoutPdfMuHeld(t *testing.T) {
	ins, doc := openUnvalidated(t, uaWalkPDF())
	var pages []int
	_, err := ins.validate("raw", ProfilePDFUA1Structural, walkerWith(func(page int) {
		pages = append(pages, page)
		if !doc.pdfMu.TryLock() {
			t.Errorf("pdfMu held when the walk reached page %d", page)
			return
		}
		doc.pdfMu.Unlock()
	}))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !slices.Equal(pages, []int{0, 1, 2}) {
		t.Errorf("walk hook ran for %v, want [0 1 2]", pages)
	}
}
