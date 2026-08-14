// Bindings wire-shape guard for the alpha.96 struct-tag risk.
//
// alpha.96 moved the runtime-facing payload under a stable binding API + JSON
// struct tags. The classic silent regression is a field getting re-tagged
// (e.g. `objectRef` -> `objectref`, or `nodeId` -> `nodeID`): TypeScript still
// compiles, but `payload.objectRef` arrives `undefined` in the UI.
//
// The story prefers option (a): a Go test that json.Marshals a POPULATED
// pdfcore payload and asserts the expected JSON keys/casing. This module is an
// independent go.mod (the project convention: no `replace` link into the main
// module, which would drag the whole Wails tree into a test module). The CLI is
// the executable expression of the SAME internal/pdfcore model.go structs:
// `dump object` runs ObjectDetail (+ nested PropertyEntry/ValueEntry) through
// the real json.Marshal. So we exercise option (a)'s invariant via the
// established CLI-exec harness (mirrors tests/page-render-info/), with zero
// production-code dependency.
//
// This is the AUTOMATED guard for that risk; it does not replace the manual
// round-trip smoke (Smoke item 3).
package wails_alpha2_103_upgrade_test

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

// TestObjectDetailWireShape asserts `dump object` emits the ObjectDetail payload
// with the expected camelCase top-level keys, so a regen that re-tags a field
// (nodeId -> nodeID) fails loud.
func TestObjectDetailWireShape(t *testing.T) {
	bin := buildCLI(t)
	// minimal.pdf object 1 is the /Catalog (a dict with /Pages + /Type).
	stdout, stderr, code := runCLI(t, bin, "dump", "object", "--json", "--ref", "1 0 R", minimalPDFPath(t))
	if code != 0 {
		t.Fatalf("`dump object --ref \"1 0 R\" testdata/minimal.pdf` exited %d\nstderr: %s", code, stderr)
	}
	obj := mustParseJSONObject(t, stdout)
	for _, key := range objectDetailWireKeys {
		if _, ok := obj[key]; !ok {
			t.Errorf("ObjectDetail JSON missing key %q (case-sensitive) -- alpha.96 struct-tag drift would silently break the frontend's payload destructuring\nraw: %s", key, stdout)
		}
	}
	// `type` must be the literal "dict" for the catalog -- pins value casing too.
	if got, _ := obj["type"].(string); got != "dict" {
		t.Errorf("ObjectDetail.type for the catalog dict = %q, want \"dict\" (value contract, not just key presence)", got)
	}
}

// TestNestedPropertyValueWireShape asserts the nested PropertyEntry and ValueEntry
// shape on the same payload carries its camelCase keys. These are the deepest part
// of the wire contract the frontend walks (reference navigation reads
// value.refTarget), so a re-tag here is the hardest drift to spot.
func TestNestedPropertyValueWireShape(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, code := runCLI(t, bin, "dump", "object", "--json", "--ref", "1 0 R", minimalPDFPath(t))
	if code != 0 {
		t.Fatalf("dump object exited %d\nstderr: %s", code, stderr)
	}
	obj := mustParseJSONObject(t, stdout)
	props, ok := obj["properties"].([]any)
	if !ok || len(props) == 0 {
		t.Fatalf("ObjectDetail.properties must be a non-empty array for the catalog dict (got %T)\nraw: %s", obj["properties"], stdout)
	}
	// PropertyEntry: { "key": ..., "value": { ValueEntry } }.
	var sawReference bool
	for _, p := range props {
		entry, ok := p.(map[string]any)
		if !ok {
			t.Fatalf("each properties element must be an object (PropertyEntry)\nraw: %s", stdout)
		}
		if _, ok := entry["key"]; !ok {
			t.Errorf("PropertyEntry missing key \"key\" -- alpha.96 re-tag")
		}
		val, ok := entry["value"].(map[string]any)
		if !ok {
			t.Errorf("PropertyEntry missing object \"value\" (ValueEntry) -- alpha.96 re-tag")
			continue
		}
		// ValueEntry load-bearing keys: type, display, raw, refTarget.
		for _, vk := range []string{"type", "display", "raw", "refTarget"} {
			if _, ok := val[vk]; !ok {
				t.Errorf("ValueEntry missing key %q (case-sensitive) -- the frontend reads value.refTarget for ref navigation; a re-tag is a silent break", vk)
			}
		}
		// The /Pages entry is a reference: pin that refTarget is the node-id form.
		if vt, _ := val["type"].(string); vt == "reference" {
			sawReference = true
			if rt, _ := val["refTarget"].(string); rt == "" {
				t.Errorf("reference ValueEntry must carry a non-empty refTarget node id")
			}
		}
	}
	if !sawReference {
		t.Errorf("catalog properties must include the /Pages reference ValueEntry (the wire-shape fixture relies on it)\nraw: %s", stdout)
	}
}
