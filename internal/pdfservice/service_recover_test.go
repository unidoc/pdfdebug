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
// runtime.Error from its bound methods. Used by AC5 to exercise the
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

// GetTreeRoot panics with a synthetic runtime.Error to drive AC5's recover
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

// Test_10_5_AC5_ServiceRecoversRuntimeError [P0] AC#5:
// A runtime.Error panicked inside a pdfcpu-bound Inspector method called from
// a PDFService method MUST be converted to a returned error satisfying
// errors.Is(err, pdfcore.ErrMalformedPDF), with no app crash.
//
// Expected production code (post-implementation):
//   - service.go declares: type inspectorAPI interface { GetTreeRoot(...) ... ; ... }
//   - PDFService.inspector field type changes from *pdfcore.Inspector to inspectorAPI
//   - Every pdfcpu-touching service method begins with:
//       defer recoverRuntimePanic("MethodName", &err)
//
// Pre-implementation: this test file is build-tag-gated (//go:build
// story_10_5_seam) so it does NOT compile by default and does NOT contribute
// to today's test runs. The structural assertions in the per-story suite
// (Test_10_5_AC5_RecoverRuntimePanicHelperExists,
// Test_10_5_AC5_WrappedMethodsHaveDeferRecover) carry the red signal today.
func Test_10_5_AC5_ServiceRecoversRuntimeError(t *testing.T) {
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
				t.Errorf("[P0] 10-5-AC5: GetTreeRoot propagated runtime.Error (%v) instead of being recovered by pdfservice -- AC5 contract violated", r)
			} else {
				t.Errorf("[P0] 10-5-AC5: unexpected non-runtime panic propagated: %v", r)
			}
		}
	}()

	result, err := svc.GetTreeRoot("any-tab-id")
	if err == nil {
		t.Fatalf("[P0] 10-5-AC5: expected error from recovered runtime.Error, got nil (result=%v)", result)
	}
	if !errors.Is(err, pdfcore.ErrMalformedPDF) {
		t.Errorf("[P0] 10-5-AC5: expected errors.Is(err, ErrMalformedPDF) -- AC5 requires conversion via `fmt.Errorf(\"%%w: internal error\", pdfcore.ErrMalformedPDF)` so the frontend regex /malformed/i matches. Got: %v", err)
	}
	// Result MUST be the zero value (nil for *TreeNode). Go's named-return
	// semantics guarantee this when the inner call panics before returning.
	if result != nil {
		t.Errorf("[P0] 10-5-AC5: expected nil *TreeNode on recovered runtime.Error, got %v -- named-return zero-value semantics violated (helper must NOT touch the first return)", result)
	}

	// Sanity hint -- the recover helper is required to log via log.Printf.
	// Capturing stderr from within the test process is fragile; the
	// structural assertion in the per-story suite pins the log.Printf
	// call shape instead. Keep this string-shape comment so the test
	// docs the contract.
	_ = strings.Contains("pdfservice: runtime.Error in", "GetTreeRoot")
}
