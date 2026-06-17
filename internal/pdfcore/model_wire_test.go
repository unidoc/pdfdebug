// Story 12-3 (AC7): wire-shape guard for the DocumentInfo payload.
//
// alpha.96 moved the runtime-facing payload under a stable binding API + JSON
// struct tags. The classic silent regression is a field getting re-tagged (e.g.
// `tabId` -> `tabID`, `pageCount` -> `pageCount`): TypeScript still compiles but
// the field arrives `undefined` in the UI.
//
// The 12-3 acceptance module (tests/12-3-wails-alpha2-103-upgrade/) already pins
// ObjectDetail via the CLI-exec harness, but DocumentInfo is never marshalled to
// CLI stdout, so that harness structurally cannot reach it. DocumentInfo is the
// payload behind the `document:opened` event and the P0 bindings round-trip
// (Smoke item 3) -- AC7 names it as the PREFERRED wire-shape target. This
// co-located unit test is the lowest viable layer for that guard: a pure
// json.Marshal of a populated struct, in-package, no CLI exec, no go.mod barrier.
package pdfcore

import (
	"encoding/json"
	"testing"
)

// Test_12_3_UNIT_001_DocumentInfoWireShape [P0] AC7: a populated DocumentInfo
// marshals to exactly the camelCase keys the frontend destructures off the
// `document:opened` payload (useDocumentState.tsx OPEN_DOCUMENT). Fails loud if
// an alpha.96-style struct-tag drift re-tags any field.
func Test_12_3_UNIT_001_DocumentInfoWireShape(t *testing.T) {
	in := DocumentInfo{
		TabID:     "cli",
		FileName:  "sample.pdf",
		FilePath:  "/tmp/sample.pdf",
		PageCount: 3,
		FileSize:  4096,
		Error:     "",
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("[P0] 12.3-UNIT-001: marshal DocumentInfo: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("[P0] 12.3-UNIT-001: unmarshal DocumentInfo JSON: %v\nraw: %s", err, raw)
	}
	// The exact camelCase keys the frontend reads (see useDocumentState.tsx
	// OPEN_DOCUMENT and the document:opened payload contract in project-context).
	wantKeys := []string{"tabId", "fileName", "filePath", "pageCount", "fileSize", "error"}
	for _, k := range wantKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("[P0] 12.3-UNIT-001: DocumentInfo JSON missing key %q (case-sensitive) -- an alpha.96 struct-tag drift would silently break the document:opened payload destructuring (AC7)\nraw: %s", k, raw)
		}
	}
	// No extra/renamed keys: a drift that renames a field would surface as both a
	// missing wanted key AND an unexpected key, so pin the exact set.
	if len(got) != len(wantKeys) {
		t.Errorf("[P0] 12.3-UNIT-001: DocumentInfo JSON has %d keys, want exactly %d (%v) -- an unexpected key signals a re-tag or an added field that the frontend payload contract does not know about (AC7)\nraw: %s", len(got), len(wantKeys), wantKeys, raw)
	}
}
