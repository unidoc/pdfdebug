// Build tag removed in Story 10-5 implementation: the recoverRuntimePanic
// helper and the inspectorAPI seam now live in service.go, so this test
// compiles and runs as part of `go test ./internal/pdfservice/...`.

package pdfservice

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// panickingInspector simulates a pdfcore.Inspector that panics with a
// runtime.Error from its bound methods. Used by to exercise the
// pdfservice top-level recover that converts runtime.Error to
// ErrMalformedPDF: internal error.
//
// The stub only needs to implement the method PDFService calls in the test
// path -- GetTreeRoot in this case. The full inspectorAPI interface that Dev
// introduces during implementation may have a wider surface; this stub
// satisfies the subset the test exercises.
type panickingInspector struct {
	pdfcore.Inspector // embed for unimplemented methods; the test never calls them
}

// GetTreeRoot panics with a synthetic runtime.Error to drive the recover
// path. The exact runtime error kind is irrelevant; any runtime.Error
// implementation satisfies the type assertion `r.(runtime.Error)` in the
// recoverRuntimePanic helper.
func (p *panickingInspector) GetTreeRoot(_ string) (*pdfcore.TreeNode, error) {
	// Force a nil-pointer deref to manufacture a runtime.Error organically.
	// (Calling panic(someRuntimeErrorImpl) is equivalent but less idiomatic.)
	var doc *pdfcore.DocumentState
	_ = doc.PDFContext // nil pointer deref -- runtime.Error
	return nil, nil
}

// TestServiceRecoversRuntimeError asserts a runtime.Error panicked inside a
// pdfcpu-bound Inspector method called from a PDFService method is converted to a
// returned error satisfying errors.Is(err, pdfcore.ErrMalformedPDF), with no
// crash and a zero-value first return.
//
// The inspector is replaced with a stub that panics on GetTreeRoot. The deferred
// recover in the test body is a net so a missing recoverRuntimePanic fails the
// test rather than killing the runner.
func TestServiceRecoversRuntimeError(t *testing.T) {
	svc := NewPDFService(nil)
	// Substitute the inspector with the panicking stub. Requires the seam:
	// PDFService.inspector field must be assignable to inspectorAPI.
	svc.inspector = &panickingInspector{}

	// Today (pre-fix), this call propagates runtime.Error and crashes the
	// test process. The deferred recover here is a defensive net so the
	// test fails with t.Errorf instead of crashing the runner if Dev forgot
	// the recover helper.
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				t.Errorf("GetTreeRoot propagated runtime.Error (%v) instead of being recovered by pdfservice", r)
			} else {
				t.Errorf("unexpected non-runtime panic propagated: %v", r)
			}
		}
	}()

	result, err := svc.GetTreeRoot("any-tab-id")
	if err == nil {
		t.Fatalf("expected error from recovered runtime.Error, got nil (result=%v)", result)
	}
	if !errors.Is(err, pdfcore.ErrMalformedPDF) {
		t.Errorf("expected errors.Is(err, ErrMalformedPDF) -- the recover helper must convert via `fmt.Errorf(\"%%w: internal error\", pdfcore.ErrMalformedPDF)` so the frontend regex /malformed/i matches. Got: %v", err)
	}
	// Result MUST be the zero value (nil for *TreeNode). Go's named-return
	// semantics guarantee this when the inner call panics before returning.
	if result != nil {
		t.Errorf("expected nil *TreeNode on recovered runtime.Error, got %v -- named-return zero-value semantics violated (helper must NOT touch the first return)", result)
	}

	// Sanity hint -- the recover helper is required to log via log.Printf.
	// Capturing stderr from within the test process is fragile; the
	// structural assertion in the per-story suite pins the log.Printf
	// call shape instead. Keep this string-shape comment so the test
	// docs the contract.
	_ = strings.Contains("pdfservice: runtime.Error in", "GetTreeRoot")
}
