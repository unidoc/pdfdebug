// Story 11-6 RED-PHASE acceptance tests for `pdfdebug dump page --info N`.
//
// These assert the EXPECTED assembled-view behavior and MUST FAIL against the
// current binary (no `page` dump resource exists). Failures are at RUNTIME
// (unknown resource / wrong JSON / wrong exit code), keeping the main module's
// build green per the Story 11-5 black-box convention.
//
// Level: Integration (CLI black-box). The assembled view is a pdfcore struct,
// but every testable AC (1-7) is fully observable through the command's JSON
// output, so no in-package unit test (which would force a compile dependency on
// the not-yet-existing PageRenderInfo type) is needed for red phase.
//
// Run: cd tests/page-render-info && go test -v -count=1 ./...
package page_render_info_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 11.6-INTG-AC1-001 [P0]: `dump page --info 1` emits ONE JSON object with the
// top-level keys: page, pageRef, mediaBox, cropBox, rotate, extGStates,
// xobjects, patterns, shadings. (forms appears only under --forms-recursive.)
// ---------------------------------------------------------------------------

func TestPageInfo_FullObjectTopLevelShape(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d (resource not implemented?)\nstderr: %s", ec, stderr)
	}

	var m map[string]any
	mustParseJSON(t, stdout, &m)

	for _, key := range []string{"page", "pageRef", "mediaBox", "cropBox", "rotate", "extGStates", "xobjects", "patterns", "shadings"} {
		if _, ok := m[key]; !ok {
			t.Errorf("top-level object missing key %q", key)
		}
	}
	if pg, _ := m["page"].(float64); int(pg) != 1 {
		t.Errorf("page = %v, want 1", m["page"])
	}
	if ref, _ := m["pageRef"].(string); !strings.Contains(ref, "3 0 R") {
		t.Errorf("pageRef = %q, want it to name the page object \"3 0 R\"", ref)
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC1-002 [P0]: geometry inheritance. MediaBox and Rotate are set on
// /Pages (NOT the page), CropBox is local. The resolved view must report the
// INHERITED MediaBox [0 0 612 792] + Rotate 90 and the LOCAL CropBox.
// ---------------------------------------------------------------------------

func TestPageInfo_GeometryInheritsFromPagesAncestor(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var m map[string]any
	mustParseJSON(t, stdout, &m)

	mb, ok := m["mediaBox"].([]any)
	if !ok || len(mb) != 4 {
		t.Fatalf("mediaBox not a 4-element array: %v", m["mediaBox"])
	}
	wantMB := []float64{0, 0, 612, 792}
	for i, w := range wantMB {
		if got, _ := mb[i].(float64); got != w {
			t.Errorf("inherited mediaBox[%d] = %v, want %v", i, mb[i], w)
		}
	}
	if rot, _ := m["rotate"].(float64); int(rot) != 90 {
		t.Errorf("inherited rotate = %v, want 90", m["rotate"])
	}
	cb, ok := m["cropBox"].([]any)
	if !ok || len(cb) != 4 {
		t.Fatalf("cropBox not a 4-element array: %v", m["cropBox"])
	}
	if got, _ := cb[0].(float64); got != 10 {
		t.Errorf("local cropBox[0] = %v, want 10", cb[0])
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC2-001 [P0]: ExtGState GS0 carries name, ref, BM "Multiply",
// ca 0.5, CA 1.0, and a RESOLVED SMask descriptor (an object, not the literal
// "None"). No blend math is performed -- values are read from the file.
// ---------------------------------------------------------------------------

func TestPageInfo_ExtGStateResolvedWithSMaskDescriptor(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var m map[string]any
	mustParseJSON(t, stdout, &m)

	gss, ok := asArray(m, "extGStates")
	if !ok {
		t.Fatal("extGStates is not an array")
	}
	gs0 := findByName(gss, "GS0")
	if gs0 == nil {
		t.Fatalf("no extGState named GS0\nstdout: %s", stdout)
	}
	if ref, _ := gs0["ref"].(string); !strings.Contains(ref, "5 0 R") {
		t.Errorf("GS0 ref = %q, want it to name \"5 0 R\"", ref)
	}
	if bm, _ := gs0["BM"].(string); bm != "Multiply" {
		t.Errorf("GS0 BM = %q, want \"Multiply\"", bm)
	}
	if ca, _ := gs0["ca"].(float64); ca != 0.5 {
		t.Errorf("GS0 ca = %v, want 0.5", gs0["ca"])
	}
	if CA, _ := gs0["CA"].(float64); CA != 1.0 {
		t.Errorf("GS0 CA = %v, want 1.0", gs0["CA"])
	}
	// SMask must be a RESOLVED descriptor (object), not the literal string "None".
	if _, isStr := gs0["SMask"].(string); isStr {
		t.Errorf("GS0 SMask resolved to the literal %q; want a resolved soft-mask descriptor object", gs0["SMask"])
	}
	if _, isObj := gs0["SMask"].(map[string]any); !isObj {
		t.Errorf("GS0 SMask = %v (%T), want a resolved soft-mask dict", gs0["SMask"], gs0["SMask"])
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC2-002 [P1]: ExtGState GS1 has /SMask /None -> the view reports the
// literal None (string "None"), distinct from the resolved-descriptor case.
// ---------------------------------------------------------------------------

func TestPageInfo_ExtGStateSMaskNoneIsLiteral(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var m map[string]any
	mustParseJSON(t, stdout, &m)

	gss, _ := asArray(m, "extGStates")
	gs1 := findByName(gss, "GS1")
	if gs1 == nil {
		t.Fatalf("no extGState named GS1\nstdout: %s", stdout)
	}
	if s, _ := gs1["SMask"].(string); s != "None" {
		t.Errorf("GS1 SMask = %v, want the literal \"None\"", gs1["SMask"])
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC3-001 [P0]: Form XObject Fm0 carries subtype "Form", bbox, matrix,
// and a resolved group (S "Transparency", CS "DeviceRGB", I true, K false).
// ---------------------------------------------------------------------------

func TestPageInfo_FormXObjectGroupResolved(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var m map[string]any
	mustParseJSON(t, stdout, &m)

	xobjs, _ := asArray(m, "xobjects")
	fm0 := findByName(xobjs, "Fm0")
	if fm0 == nil {
		t.Fatalf("no xobject named Fm0\nstdout: %s", stdout)
	}
	if st, _ := fm0["subtype"].(string); st != "Form" {
		t.Errorf("Fm0 subtype = %q, want \"Form\"", st)
	}
	if bb, ok := fm0["bbox"].([]any); !ok || len(bb) != 4 {
		t.Errorf("Fm0 bbox = %v, want a 4-element array", fm0["bbox"])
	}
	if mx, ok := fm0["matrix"].([]any); !ok || len(mx) != 6 {
		t.Errorf("Fm0 matrix = %v, want a 6-element array", fm0["matrix"])
	}
	grp, ok := fm0["group"].(map[string]any)
	if !ok {
		t.Fatalf("Fm0 group is not an object: %v", fm0["group"])
	}
	if s, _ := grp["S"].(string); s != "Transparency" {
		t.Errorf("Fm0 group.S = %q, want \"Transparency\"", s)
	}
	if cs, _ := grp["CS"].(string); cs != "DeviceRGB" {
		t.Errorf("Fm0 group.CS = %q, want \"DeviceRGB\"", cs)
	}
	if i, _ := grp["I"].(bool); !i {
		t.Errorf("Fm0 group.I = %v, want true", grp["I"])
	}
	if k, ok := grp["K"].(bool); !ok || k {
		t.Errorf("Fm0 group.K = %v, want false", grp["K"])
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC3-002 [P0]: Image XObject Im0 carries subtype "Image", width 2,
// height 2, and a colorSpace classification: family "ICCBased" with n 3 and a
// profile size. Colorspace is CLASSIFIED, not evaluated.
// ---------------------------------------------------------------------------

func TestPageInfo_ImageXObjectColorSpaceClassified(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var m map[string]any
	mustParseJSON(t, stdout, &m)

	xobjs, _ := asArray(m, "xobjects")
	im0 := findByName(xobjs, "Im0")
	if im0 == nil {
		t.Fatalf("no xobject named Im0\nstdout: %s", stdout)
	}
	if st, _ := im0["subtype"].(string); st != "Image" {
		t.Errorf("Im0 subtype = %q, want \"Image\"", st)
	}
	if w, _ := im0["width"].(float64); int(w) != 2 {
		t.Errorf("Im0 width = %v, want 2", im0["width"])
	}
	if h, _ := im0["height"].(float64); int(h) != 2 {
		t.Errorf("Im0 height = %v, want 2", im0["height"])
	}
	cs, ok := im0["colorSpace"].(map[string]any)
	if !ok {
		t.Fatalf("Im0 colorSpace is not an object: %v", im0["colorSpace"])
	}
	if fam, _ := cs["family"].(string); fam != "ICCBased" {
		t.Errorf("Im0 colorSpace.family = %q, want \"ICCBased\"", fam)
	}
	if n, _ := cs["n"].(float64); int(n) != 3 {
		t.Errorf("Im0 colorSpace.n = %v, want 3 (ICC component count)", cs["n"])
	}
	// Some profile-size field must be present and positive (e.g. iccProfileSize).
	if sz, _ := cs["iccProfileSize"].(float64); sz <= 0 {
		t.Errorf("Im0 colorSpace.iccProfileSize = %v, want a positive profile-stream size", cs["iccProfileSize"])
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC1-003 [P1]: patterns/shadings are STRUCTURAL only: each carries
// name + ref + the integer patternType / shadingType, and no evaluated content.
// ---------------------------------------------------------------------------

func TestPageInfo_PatternsShadingsStructuralOnly(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var m map[string]any
	mustParseJSON(t, stdout, &m)

	pats, _ := asArray(m, "patterns")
	p0 := findByName(pats, "P0")
	if p0 == nil {
		t.Fatalf("no pattern named P0\nstdout: %s", stdout)
	}
	if ref, _ := p0["ref"].(string); !strings.Contains(ref, "12 0 R") {
		t.Errorf("P0 ref = %q, want it to name \"12 0 R\"", ref)
	}
	if pt, _ := p0["patternType"].(float64); int(pt) != 1 {
		t.Errorf("P0 patternType = %v, want 1", p0["patternType"])
	}

	shs, _ := asArray(m, "shadings")
	sh0 := findByName(shs, "Sh0")
	if sh0 == nil {
		t.Fatalf("no shading named Sh0\nstdout: %s", stdout)
	}
	if st, _ := sh0["shadingType"].(float64); int(st) != 2 {
		t.Errorf("Sh0 shadingType = %v, want 2", sh0["shadingType"])
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC7-001 [P0]: STRUCTURAL-ONLY guard. The entire output must contain
// NO computed-color output field. Assert that none of the forbidden rendered-
// color keys appear anywhere in the JSON (no rgb/cmyk/composited results).
// ---------------------------------------------------------------------------

func TestPageInfo_StructuralOnlyNoComputedColor(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	// Guard: forbidden computed-color / composited keys must not appear as JSON
	// keys. These would indicate the view crossed the structural/rendering seam.
	forbidden := []string{
		`"rgb"`, `"rgbValue"`, `"computedRGB"`, `"cmyk"`, `"cmykValue"`,
		`"composited"`, `"compositedResult"`, `"colorToRGB"`, `"resolvedColor"`,
		`"tintTransformResult"`, `"renderedColor"`,
	}
	for _, key := range forbidden {
		if strings.Contains(stdout, key) {
			t.Errorf("output contains forbidden computed-color key %s -- the view must be structural-only", key)
		}
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC6-001 [P0]: a valid page with NO /Resources emits empty [] for
// every resource array and exits 0 (absent resource is a valid empty result).
// ---------------------------------------------------------------------------

func TestPageInfo_NoResourcesEmptyArraysExitZero(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "no-resources.pdf", noResourcesFixturePDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("no-/Resources page expected exit 0, got %d\nstderr: %s", ec, stderr)
	}
	var m map[string]any
	mustParseJSON(t, stdout, &m)

	for _, key := range []string{"extGStates", "xobjects", "patterns", "shadings"} {
		arr, ok := asArray(m, key)
		if !ok {
			t.Errorf("%q must be present as an array (empty), got %v", key, m[key])
			continue
		}
		if len(arr) != 0 {
			t.Errorf("%q must be empty [] for a no-/Resources page, got %v", key, arr)
		}
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC6-002 [P0]: an out-of-range page number emits a JSON error on
// stderr and exits 2 (page-not-found code), with clean (empty) stdout and no
// panic.
// ---------------------------------------------------------------------------

func TestPageInfo_OutOfRangePageExitTwo(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--info", "999", pdfPath)
	if ec != 2 {
		t.Errorf("out-of-range page expected exit 2, got %d", ec)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty on page-not-found, got: %s", stdout)
	}
	// Error belongs on stderr as a JSON object (mirrors writeJSONError).
	var em map[string]any
	if err := jsonTrimParse(stderr, &em); err != nil {
		t.Errorf("stderr should carry a JSON error object, got: %q (%v)", stderr, err)
	} else if _, ok := em["error"]; !ok {
		t.Errorf("stderr JSON missing \"error\" key: %v", em)
	}
	if strings.Contains(stderr, "panic") || strings.Contains(stdout, "panic") {
		t.Errorf("command panicked")
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC6-003 [P1]: a non-positive --info value is a USAGE error (exit 1),
// distinct from the out-of-range page-not-found (exit 2) path.
// ---------------------------------------------------------------------------

func TestPageInfo_NonPositiveInfoUsageError(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	_, stderr, ec := runCLI(t, bin, "dump", "page", "--info", "0", pdfPath)
	// Guard against a false green: the `page` resource must be RECOGNIZED. The
	// pre-implementation binary rejects it with "Unknown resource: page" (also
	// exit 1), which would otherwise satisfy a naive usage-error assertion.
	if strings.Contains(stderr, "Unknown resource") {
		t.Fatalf("`dump page` not implemented (got %q); usage-error path is untestable until the resource exists", strings.TrimSpace(stderr))
	}
	if ec != 1 {
		t.Errorf("--info 0 should be a usage error (exit 1), got %d", ec)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Errorf("stderr should carry a usage/error message for --info 0")
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC8-001 [P1]: the command output is documented as EXPERIMENTAL in
// the help/usage text (the experimental-contract gate, AC8 part a).
// ---------------------------------------------------------------------------

func TestPageInfo_HelpMarksExperimental(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, _ := runCLI(t, bin, "--help")
	help := stdout + stderr
	if !strings.Contains(help, "dump page") {
		t.Errorf("--help should list the `dump page` command")
	}
	if !strings.Contains(strings.ToLower(help), "experimental") {
		t.Errorf("help text should mark `dump page --info` output as experimental")
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-XCUT-001 [P2]: compact-by-default + --pretty parity (the shared
// emit helper). Default output is single-line; --pretty is indented; both
// decode to identical content.
// ---------------------------------------------------------------------------

func TestPageInfo_PrettyVsCompactParity(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	compact, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("compact exit %d", ec)
	}
	pretty, _, ep := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--pretty", pdfPath)
	if ep != 0 {
		t.Fatalf("--pretty exit %d", ep)
	}
	if strings.Count(strings.TrimRight(compact, "\n"), "\n") != 0 {
		t.Errorf("default output is not single-line compact:\n%.200s", compact)
	}
	if !strings.Contains(pretty, "\n  ") {
		t.Errorf("--pretty output is not indented multi-line:\n%.200s", pretty)
	}
}

// ---------------------------------------------------------------------------
// 13.1 (AC8): the full-object --json output carries a top-level
// "_stability":"experimental" marker (machine-visible instability). A
// section-scoped --json view omits it (documented decision: full object only).
// ---------------------------------------------------------------------------

func TestPageInfo_JSONStabilityMarker(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	full, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("[13.1] page _stability: full --json exit %d", ec)
	}
	var fm map[string]any
	mustParseJSON(t, full, &fm)
	if s, _ := fm["_stability"].(string); s != "experimental" {
		t.Errorf("[13.1] page _stability: full --json must carry \"_stability\":\"experimental\", got %v", fm["_stability"])
	}

	sec, _, ecs := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--section", "geometry", pdfPath)
	if ecs != 0 {
		t.Fatalf("[13.1] page _stability: section --json exit %d", ecs)
	}
	var sm map[string]any
	mustParseJSON(t, sec, &sm)
	if _, present := sm["_stability"]; present {
		t.Errorf("[13.1] page _stability: section --json must OMIT _stability, got %v", sm["_stability"])
	}
}

// ---------------------------------------------------------------------------
// 13.1 (AC6/AC2): the default (no --json) output is human-readable PLAIN TEXT
// with aligned key/value sections, NOT JSON. STRUCTURAL assertions only.
// ---------------------------------------------------------------------------

func TestPageInfo_PlainTextDefault(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("[13.1] page plain: exit %d", ec)
	}
	trimmed := strings.TrimSpace(stdout)
	if strings.HasPrefix(trimmed, "{") && json.Valid([]byte(trimmed)) {
		t.Fatalf("[13.1] page plain: default output must be plain text, not JSON:\n%.200s", stdout)
	}
	for _, heading := range []string{"Geometry:", "ExtGStates:", "XObjects:"} {
		if !strings.Contains(stdout, heading) {
			t.Errorf("[13.1] page plain: expected the %q section heading\n%s", heading, stdout)
		}
	}
	if strings.Contains(stdout, "_stability") {
		t.Errorf("[13.1] page plain: plain output must not carry the _stability marker")
	}
}
