// Consumer-driven binding-presence contract that REPLACES the brittle
// exact-method-count pin the alpha.95 suite carried, plus the
// anti-brittleness meta-guard proving no `count == N` pin was re-introduced.
//
// Per project_struct_grep_tests_brittle.md and the story: assert PRESENCE of the
// methods the frontend actually binds, checked against the REGENERATED ARTIFACT
// (frontend/bindings/...), NOT a `strings.Count(... "func (s *PDFService)")` of
// internal/pdfservice/service.go. No exact count is asserted anywhere.
package wails_alpha2_103_upgrade_test

import (
	"strings"
	"testing"
)

// consumerBoundMethods is the explicit set of PDFService methods the frontend
// PRODUCTION code imports from the generated binding
// (frontend/bindings/.../pdfservice/pdfservice.js). This is a deliberate subset
// of the full receiver surface -- the consumer contract, not the whole API and
// not a count. Sourced from the non-test `import { ... } from
// '.../pdfservice/pdfservice.js'` sites:
//
//	App.jsx                : CloseDocument, ConsumePendingOpenFiles
//	ObjectInfoPanel.tsx    : GetObjectSource
//	DetailPanel.tsx        : GetObjectDetail, GetContentStream, GetImageData,
//	                         GetReverseRefs, GetFontView
//	XRefTableView.tsx      : GetXRefTable
//	CommandPalette.tsx     : GetAncestorPath
//	TreePanel.tsx          : GetChildren, GetAncestorPath
//	TabBar.tsx             : CloseDocument
//	useObjectIndex.ts      : GetObjectIndex
//	usePDFService.ts       : OpenFile, GetTreeRoot, GetChildren, CloseDocument,
//	                         OpenFileDialog, GoToPage
//
// If the dev adds/removes a frontend import in the same change, this list moves
// WITH the real consumer dependency -- which is exactly the contract we want,
// unlike a magic count that churns on every unrelated surface change.
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

// TestBindingsExportConsumerMethods asserts the regenerated binding exports every
// method the frontend actually invokes. This presence contract replaces a count==N
// pin: a `-clean=true` regen that drops or renames a consumer method (as an alpha2
// event/binding reorg could force) fails loud here, while an unrelated change to
// the full method surface does not.
func TestBindingsExportConsumerMethods(t *testing.T) {
	if !fileExists(t, bindingRelPath) {
		t.Fatalf("regenerated binding %s must exist -- run `wails3 generate bindings -clean=true`", bindingRelPath)
	}
	src := readSource(t, bindingRelPath)
	for _, m := range consumerBoundMethods {
		needle := "export function " + m + "("
		if !strings.Contains(src, needle) {
			t.Errorf("pdfservice.js must export consumer-bound method %q after regen -- the frontend imports it directly; a dropped/renamed binding silently breaks the UI (presence contract, NOT a count)", m)
		}
	}
}

// TestBindingsDoNotResurrectGetPlainTextFull asserts a `-clean=true` regen does not
// re-introduce the removed GetPlainTextFull symbol, guarding against a stale or
// non-clean regen re-baselining a dead binding.
func TestBindingsDoNotResurrectGetPlainTextFull(t *testing.T) {
	if !fileExists(t, bindingRelPath) {
		t.Skipf("%s missing -- the regenerated binding does not exist", bindingRelPath)
	}
	src := readSource(t, bindingRelPath)
	if strings.Contains(src, "GetPlainTextFull") {
		t.Errorf("pdfservice.js must NOT export GetPlainTextFull -- the async-load work removed the Go method; a stale (`-clean=false`) regen re-introduces a dead binding")
	}
}

// TestNoExactMethodCountPin is an anti-brittleness meta-guard: no test in this
// suite may re-introduce an exact method-count pin, since such a pin churned from
// 20 to 22 once already. It scans the suite's own *_test.go files for the two
// brittle signatures -- the receiver-line count grep and a `count ==` comparison
// against it -- and fails if a future edit re-pins a magic number.
func TestNoExactMethodCountPin(t *testing.T) {
	// Skip this guard's own file: it holds the forbidden patterns as detection
	// literals, which would otherwise self-trip the scan.
	own := loadOwnTestSources(t, "bindings_contract_test.go")
	// The exact brittle pattern the retired count test used: counting receiver
	// lines.
	if strings.Contains(own, `Count(`) && strings.Contains(own, `func (s *PDFService)`) {
		t.Errorf("this suite must NOT count `func (s *PDFService)` receiver lines -- the exact-count pin is dropped entirely (project_struct_grep_tests_brittle.md). Use the consumer-driven presence contract in TestBindingsExportConsumerMethods instead.")
	}
	// A method-surface count compared to a fixed N (e.g. `count != 22`).
	for _, frag := range []string{"count == 2", "count != 2", "count == len(expectedServiceMethods)", "MethodCount"} {
		if strings.Contains(own, frag) {
			t.Errorf("this suite must NOT assert an exact PDFService method count (found %q) -- re-pinning a magic number is forbidden; the count lives only as informational prose in project-context.md", frag)
		}
	}
}
