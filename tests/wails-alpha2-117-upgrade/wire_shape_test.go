// AC3.3: bindings wire-shape guard.
//
// scenario 14.2-INTG-003 [P0] (risk R-14-08): the classic silent regression on
// a Wails bump + binding regen is a JSON field getting re-tagged (e.g.
// `objectRef` -> `objectref`, or `nodeId` -> `nodeID`): TypeScript still
// compiles, but `payload.objectRef` arrives `undefined` in the UI.
//
// Option (a): a Go-side check that json.Marshals a POPULATED pdfcore payload and
// asserts the expected JSON keys/casing. This module is an independent go.mod
// (no `replace` link), so we exercise option (a)'s invariant via the CLI, the
// executable expression of the SAME internal/pdfcore model.go structs: `dump
// object` runs ObjectDetail (+ nested PropertyEntry/ValueEntry) through the real
// json.Marshal (mirrors tests/12-3 INTG-001/002 and tests/11-6-page-render-info).
//
// STANDING regression net: passes on the current tree and stands through the
// alpha2.117 bump. It is the AUTOMATED guard for the wire-shape risk; it does
// NOT replace the manual round-trip smoke (AC4, deferred human/hardware gate).
package story_14_2_wails_alpha2_117_upgrade_test

import (
	"path/filepath"
	"testing"
)

// minimalPDFPath returns the absolute path to testdata/minimal.pdf under the
// project root. The CLI is exec'd with an arbitrary cwd, so the fixture must be
// addressed absolutely.
func minimalPDFPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata", "minimal.pdf")
}

// objectDetailWireKeys are the camelCase JSON keys the frontend reads off an
// ObjectDetail payload (internal/pdfcore/model.go ObjectDetail). DetailPanel.tsx
// destructures these; a struct-tag drift on any one is a silent undefined.
var objectDetailWireKeys = []string{
	"nodeId",
	"objectRef",
	"type",
	"properties",
}

// Test_14_2_INTG_003_ObjectDetailWireShape [P0] AC3.3: `dump object` emits the
// ObjectDetail payload with the expected camelCase top-level keys. Fails loud if
// a regen under the bump re-tags a field (e.g. nodeId -> nodeID). Passes on the
// current tree and stands as the standing regression net through the bump.
func Test_14_2_INTG_003_ObjectDetailWireShape(t *testing.T) {
	bin := buildCLI(t)
	// minimal.pdf object 1 is the /Catalog (a dict with /Pages + /Type).
	stdout, stderr, code := runCLI(t, bin, "dump", "object", "--json", "--ref", "1 0 R", minimalPDFPath(t))
	if code != 0 {
		t.Fatalf("[P0] 14.2-INTG-003: `dump object --ref \"1 0 R\" testdata/minimal.pdf` exited %d\nstderr: %s", code, stderr)
	}
	obj := mustParseJSONObject(t, stdout)
	for _, key := range objectDetailWireKeys {
		if _, ok := obj[key]; !ok {
			t.Errorf("[P0] 14.2-INTG-003: ObjectDetail JSON missing key %q (case-sensitive) -- a bindings-regen struct-tag drift would silently break the frontend's payload destructuring (AC3.3)\nraw: %s", key, stdout)
		}
	}
	// `type` must be the literal "dict" for the catalog -- pins value casing too.
	if got, _ := obj["type"].(string); got != "dict" {
		t.Errorf("[P0] 14.2-INTG-003: ObjectDetail.type for the catalog dict = %q, want \"dict\" (AC3.3: value contract, not just key presence)", got)
	}
}

// Test_14_2_INTG_003_NestedPropertyValueWireShape [P0] AC3.3: the nested
// PropertyEntry + ValueEntry shape on the same payload carries its camelCase
// keys. These are the deepest part of the wire contract the frontend walks
// (reference navigation reads value.refTarget), so a re-tag here is the
// hardest-to-spot drift.
func Test_14_2_INTG_003_NestedPropertyValueWireShape(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, code := runCLI(t, bin, "dump", "object", "--json", "--ref", "1 0 R", minimalPDFPath(t))
	if code != 0 {
		t.Fatalf("[P0] 14.2-INTG-003: dump object exited %d\nstderr: %s", code, stderr)
	}
	obj := mustParseJSONObject(t, stdout)
	props, ok := obj["properties"].([]any)
	if !ok || len(props) == 0 {
		t.Fatalf("[P0] 14.2-INTG-003: ObjectDetail.properties must be a non-empty array for the catalog dict (got %T)\nraw: %s", obj["properties"], stdout)
	}
	// PropertyEntry: { "key": ..., "value": { ValueEntry } }.
	var sawReference bool
	for _, p := range props {
		entry, ok := p.(map[string]any)
		if !ok {
			t.Fatalf("[P0] 14.2-INTG-003: each properties element must be an object (PropertyEntry)\nraw: %s", stdout)
		}
		if _, ok := entry["key"]; !ok {
			t.Errorf("[P0] 14.2-INTG-003: PropertyEntry missing key \"key\" -- regen re-tag (AC3.3)")
		}
		val, ok := entry["value"].(map[string]any)
		if !ok {
			t.Errorf("[P0] 14.2-INTG-003: PropertyEntry missing object \"value\" (ValueEntry) -- regen re-tag (AC3.3)")
			continue
		}
		// ValueEntry load-bearing keys: type, display, raw, refTarget.
		for _, vk := range []string{"type", "display", "raw", "refTarget"} {
			if _, ok := val[vk]; !ok {
				t.Errorf("[P0] 14.2-INTG-003: ValueEntry missing key %q (case-sensitive) -- the frontend reads value.refTarget for ref navigation; a re-tag is a silent break (AC3.3)", vk)
			}
		}
		// The /Pages entry is a reference: pin that refTarget is the node-id form.
		if vt, _ := val["type"].(string); vt == "reference" {
			sawReference = true
			if rt, _ := val["refTarget"].(string); rt == "" {
				t.Errorf("[P0] 14.2-INTG-003: reference ValueEntry must carry a non-empty refTarget node id (AC3.3)")
			}
		}
	}
	if !sawReference {
		t.Errorf("[P0] 14.2-INTG-003: catalog properties must include the /Pages reference ValueEntry (the wire-shape fixture relies on it)\nraw: %s", stdout)
	}
}
