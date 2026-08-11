// Story 11-6 RED-PHASE acceptance tests: recursive Form walk (AC4) and
// --section scoping (AC5). Black-box; MUST FAIL until 11-6 is implemented.
//
// Run: cd tests/11-6-page-render-info && go test -v -count=1 ./...
package page_render_info_test

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 11.6-INTG-AC4-001 [P1]: WITHOUT --forms-recursive, forms are LISTED (Fm0
// appears under xobjects) but NOT walked: there is no nested `forms` tree.
// ---------------------------------------------------------------------------

func TestForms_NotWalkedByDefault(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "nested-forms.pdf", nestedFormFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var m map[string]any
	mustParseJSON(t, stdout, &m)

	// Fm0 is still listed as a page XObject.
	xobjs, _ := asArray(m, "xobjects")
	if findByName(xobjs, "Fm0") == nil {
		t.Errorf("Fm0 should be listed under xobjects even without --forms-recursive")
	}
	// But the recursive forms tree must be absent or empty (not walked).
	if forms, ok := asArray(m, "forms"); ok && len(forms) > 0 {
		t.Errorf("forms tree must not be walked without --forms-recursive, got %d entries", len(forms))
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC4-002 [P0]: WITH --forms-recursive, a page that does `Do /Fm0`
// where Fm0 does `Do /Inner` (and /Inner lives in Fm0's OWN /Resources, not the
// page's) yields a forms tree where the nested /Inner form is resolved against
// the OUTER FORM's resources. The page resources do NOT contain /Inner, so a
// walk that resolved against the page would miss it.
// ---------------------------------------------------------------------------

func TestForms_NestedResolvedAgainstOwnResources(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "nested-forms.pdf", nestedFormFixturePDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--forms-recursive", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d (--forms-recursive not implemented?)\nstderr: %s", ec, stderr)
	}

	// /Inner (object 6 0 R) must appear SOMEWHERE in the recursive forms output.
	// It is reachable only via Fm0's own /Resources, so its presence proves the
	// walk used the form's own resource dict (feedback item 11 gotcha).
	if !strings.Contains(stdout, "6 0 R") {
		t.Errorf("[P0] 11.6-INTG-AC4-002: nested /Inner form (6 0 R) absent from --forms-recursive output; "+
			"walk must resolve nested forms against the form's OWN /Resources\nstdout: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC4-003 [P0]: a self-referential form (Fm0 whose content does
// `Do /Fm0` against its own /Resources) terminates under --forms-recursive: the
// command exits cleanly (0 or a bounded non-panic), never hangs, never panics.
// Guards the form-object-ref visited set required by AC4.
// ---------------------------------------------------------------------------

func TestForms_SelfReferentialTerminates(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "self-ref-form.pdf", selfRefFormFixturePDF())

	// A high --forms-depth that, absent a visited set, would recurse forever.
	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1",
		"--forms-recursive", "--forms-depth", "100", pdfPath)

	if strings.Contains(stdout, "panic") || strings.Contains(stderr, "panic") {
		t.Fatalf("self-referential form caused a panic\nstderr: %s", stderr)
	}
	if ec != 0 {
		t.Fatalf("self-referential form should terminate with exit 0, got %d\nstderr: %s", ec, stderr)
	}
	// The output must still be a single valid JSON object (the walk terminated
	// and produced a bounded result, not an endless stream).
	var m map[string]any
	mustParseJSON(t, stdout, &m)
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC4-004 [P1]: --forms-depth bounds the recursion. With
// --forms-depth 1 the nested /Inner form (a SECOND level below the page) must
// NOT be expanded into the tree, while the first-level Fm0 still is.
// (--forms-depth is a distinct axis from --depth and --resolve-depth.)
// ---------------------------------------------------------------------------

func TestForms_FormsDepthBoundsRecursion(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "nested-forms.pdf", nestedFormFixturePDF())

	depth1, _, ec1 := runCLI(t, bin, "dump", "page", "--json", "--info", "1",
		"--forms-recursive", "--forms-depth", "1", pdfPath)
	if ec1 != 0 {
		t.Fatalf("--forms-depth 1 exit %d", ec1)
	}
	depth2, _, ec2 := runCLI(t, bin, "dump", "page", "--json", "--info", "1",
		"--forms-recursive", "--forms-depth", "2", pdfPath)
	if ec2 != 0 {
		t.Fatalf("--forms-depth 2 exit %d", ec2)
	}

	// /Inner (6 0 R) is two form levels deep: present at depth 2, absent at depth 1.
	if strings.Contains(depth1, "6 0 R") {
		t.Errorf("--forms-depth 1 must NOT expand the 2nd-level /Inner form (6 0 R)")
	}
	if !strings.Contains(depth2, "6 0 R") {
		t.Errorf("--forms-depth 2 must expand the 2nd-level /Inner form (6 0 R)")
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC4-005 [P2]: --forms-depth with a malformed (non-integer) value is
// a USAGE error (exit 1).
// ---------------------------------------------------------------------------

func TestForms_MalformedFormsDepthUsageError(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "nested-forms.pdf", nestedFormFixturePDF())

	_, stderr, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1",
		"--forms-recursive", "--forms-depth", "abc", pdfPath)
	// Guard against a false green: the resource must be recognized first.
	if strings.Contains(stderr, "Unknown resource") {
		t.Fatalf("`dump page` not implemented (got %q)", strings.TrimSpace(stderr))
	}
	if ec != 1 {
		t.Errorf("malformed --forms-depth expected usage exit 1, got %d", ec)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Errorf("stderr should carry a usage/error message")
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC5-001 [P0]: --section geometry emits ONLY the geometry slice
// (page/pageRef/mediaBox/cropBox/rotate) and OMITS the resource arrays.
// ---------------------------------------------------------------------------

func TestSection_GeometryOnly(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--section", "geometry", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var m map[string]any
	mustParseJSON(t, stdout, &m)

	for _, want := range []string{"mediaBox", "cropBox", "rotate"} {
		if _, ok := m[want]; !ok {
			t.Errorf("--section geometry missing geometry key %q", want)
		}
	}
	for _, omit := range []string{"extGStates", "xobjects", "patterns", "shadings"} {
		if _, present := m[omit]; present {
			t.Errorf("--section geometry must OMIT resource section %q", omit)
		}
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC5-002 [P0]: --section extgstates emits ONLY the extGStates
// section (and omits xobjects/patterns/shadings).
// ---------------------------------------------------------------------------

func TestSection_ExtGStatesOnly(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--section", "extgstates", pdfPath)
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var m map[string]any
	mustParseJSON(t, stdout, &m)

	if _, ok := m["extGStates"]; !ok {
		t.Errorf("--section extgstates must emit the extGStates section")
	}
	for _, omit := range []string{"xobjects", "patterns", "shadings"} {
		if _, present := m[omit]; present {
			t.Errorf("--section extgstates must OMIT %q", omit)
		}
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC5-003 [P0]: an UNRECOGNIZED --section value is a usage error
// (exit 1). The enum is exactly geometry|extgstates|xobjects|forms; patterns
// and shadings are explicitly NOT selectable sections.
// ---------------------------------------------------------------------------

func TestSection_UnrecognizedIsUsageError(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	for _, bad := range []string{"bogus", "patterns", "shadings"} {
		_, stderr, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--section", bad, pdfPath)
		// Guard against a false green: the resource must be recognized first.
		if strings.Contains(stderr, "Unknown resource") {
			t.Fatalf("`dump page` not implemented (got %q)", strings.TrimSpace(stderr))
		}
		if ec != 1 {
			t.Errorf("--section %q expected usage exit 1, got %d", bad, ec)
		}
		if strings.TrimSpace(stderr) == "" {
			t.Errorf("--section %q should carry a usage/error message on stderr", bad)
		}
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-AC5-004 [P1]: --section xobjects emits the xobjects section and
// omits the others; --section forms emits the forms section.
// ---------------------------------------------------------------------------

func TestSection_XObjectsAndForms(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "renderinfo.pdf", renderInfoFixturePDF())

	xoOut, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--section", "xobjects", pdfPath)
	if ec != 0 {
		t.Fatalf("--section xobjects exit %d", ec)
	}
	var xm map[string]any
	mustParseJSON(t, xoOut, &xm)
	if _, ok := xm["xobjects"]; !ok {
		t.Errorf("--section xobjects must emit the xobjects section")
	}
	if _, present := xm["extGStates"]; present {
		t.Errorf("--section xobjects must OMIT extGStates")
	}

	fmOut, _, ef := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--section", "forms", pdfPath)
	if ef != 0 {
		t.Fatalf("--section forms exit %d", ef)
	}
	var fm map[string]any
	mustParseJSON(t, fmOut, &fm)
	if _, ok := fm["forms"]; !ok {
		t.Errorf("--section forms must emit the forms section")
	}
}
