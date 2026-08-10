// AC3.2, AC3.3: consumer-driven binding-presence contract.
//
// scenario 14.2-INTG-002 [P0] (risk R-14-08): the REGENERATED
// frontend/bindings/ must still export every PDFService method the frontend
// actually imports. This gates on the CONTRACT, NOT on the informational
// 29-method / 38-model counts (a legitimate refactor can move those; the
// brittle-count pin was dropped at 12-3 per project_struct_grep_tests_brittle).
// A `wails3 generate bindings -clean=true` that drops or renames a consumer
// method under the alpha2.117 bump fails loud here; an unrelated surface change
// does not.
//
// This is a STANDING regression net: it passes on the current tree (the binding
// already exports these) and re-passes after the bump + regen (AC3.2's
// zero-diff / reconciled case), or fails loud if the regen moved the wire.
package story_14_2_wails_alpha2_117_upgrade_test

import (
	"strings"
	"testing"
)

// consumerBoundMethods is the explicit set of PDFService methods the frontend
// PRODUCTION code imports from the generated binding
// (frontend/bindings/.../pdfservice/pdfservice.js). A deliberate subset of the
// full receiver surface -- the consumer contract, not the whole API and not a
// count. Sourced from the non-test `import { ... } from
// '.../pdfservice/pdfservice.js'` sites (App.jsx, ObjectInfoPanel.tsx,
// DetailPanel.tsx, XRefTableView.tsx, CommandPalette.tsx, TreePanel.tsx,
// TabBar.tsx, useObjectIndex.ts, usePDFService.ts). If the dev adds/removes a
// frontend import in the same change, this list moves WITH the real consumer
// dependency -- which is exactly the contract we want, unlike a magic count that
// churns on every unrelated surface change.
var consumerBoundMethods = []string{
	"CloseDocument",
	"ConsumePendingOpenFiles",
	"GetObjectSource",
	"GetObjectDetail",
	"GetContentStream",
	"GetImageData",
	"GetReverseRefs",
	"GetFontView",
	"GetXRefTable",
	"GetAncestorPath",
	"GetChildren",
	"GetObjectIndex",
	"OpenFile",
	"GetTreeRoot",
	"OpenFileDialog",
	"GoToPage",
}

// bindingRelPath is the regenerated Wails JS binding for PDFService.
const bindingRelPath = "frontend/bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js"

// TestBindingsExportConsumerMethods asserts the (re)generated binding exports every
// method the frontend actually invokes. This presence contract replaces a count==N
// pin: a `-clean=true` regen that drops or renames a consumer method fails loud
// here, while an unrelated change to the full method surface does not.
func TestBindingsExportConsumerMethods(t *testing.T) {
	if !fileExists(t, bindingRelPath) {
		t.Fatalf("[P0] 14.2-INTG-002: regenerated binding %s must exist (run `wails3 generate bindings -clean=true`, AC3.2 / Task 2.2)", bindingRelPath)
	}
	src := readSource(t, bindingRelPath)
	for _, m := range consumerBoundMethods {
		needle := "export function " + m + "("
		if !strings.Contains(src, needle) {
			t.Errorf("[P0] 14.2-INTG-002: pdfservice.js must export consumer-bound method %q after regen -- the frontend imports it directly; a dropped/renamed binding silently breaks the UI (AC3.3 presence contract, NOT a count)", m)
		}
	}
}

// TestBindingsDoNotResurrectGetPlainTextFull asserts a `-clean=true` regen does not
// re-introduce the removed GetPlainTextFull symbol, guarding against a stale or
// non-clean regen re-baselining a dead binding.
func TestBindingsDoNotResurrectGetPlainTextFull(t *testing.T) {
	if !fileExists(t, bindingRelPath) {
		t.Skipf("[P1] 14.2-INTG-002: %s missing -- see TestBindingsExportConsumerMethods", bindingRelPath)
	}
	src := readSource(t, bindingRelPath)
	if strings.Contains(src, "GetPlainTextFull") {
		t.Errorf("[P1] 14.2-INTG-002: pdfservice.js must NOT export GetPlainTextFull -- 10-1 removed the Go method; a stale (`-clean=false`) regen re-introduces a dead binding (AC3.2)")
	}
}
