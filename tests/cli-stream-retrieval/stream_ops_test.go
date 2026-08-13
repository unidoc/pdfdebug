// Story 11-5 acceptance tests for `dump stream --ops` (item 2) and the
// Do -> resourceType lookup.
//
// Black-box: build the CLI binary and run it as a subprocess. Failures surface
// at RUNTIME (unknown flag / wrong output), not compile time, so the main build
// stays green.
//
// Run: cd tests/cli-stream-retrieval && go test -v -count=1 ./...
package cli_stream_retrieval_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseNDJSON splits stdout into one decoded JSON object per non-blank line,
// failing the test if any line is not a standalone JSON object (NDJSON
// contract: jq -c / grep / wc -l must all work).
func parseNDJSON(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	var objs []map[string]any
	for i, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("NDJSON line %d is not a standalone JSON object: %v\nline: %q", i+1, err, line)
		}
		objs = append(objs, m)
	}
	return objs
}

// formXObjectPDF builds a single-page PDF whose page content stream draws a
// Form XObject (Do Fm0) and an Image XObject (Do Im0). The page's
// /Resources/XObject maps Fm0 -> a /Subtype /Form stream and Im0 -> a
// /Subtype /Image stream. Used for Form + Image resourceType resolution.
func formXObjectPDF() []byte {
	pdf := "%PDF-1.4\n"

	pageStream := "q /Fm0 Do Q\nq /Im0 Do Q\n"
	formStream := "0 0 100 100 re f\n"
	// 1x1 raw RGB image sample (3 bytes), no filter, to keep it trivial.
	imgStream := "\xff\x00\x00"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] " +
		"/Resources << /XObject << /Fm0 5 0 R /Im0 6 0 R >> >> /Contents 4 0 R >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Length " + itoa(len(pageStream)) + " >>\nstream\n" + pageStream + "\nendstream\nendobj\n\n"
	obj5 := "5 0 obj\n<< /Type /XObject /Subtype /Form /FormType 1 /BBox [0 0 100 100] /Length " +
		itoa(len(formStream)) + " >>\nstream\n" + formStream + "\nendstream\nendobj\n\n"
	obj6 := "6 0 obj\n<< /Type /XObject /Subtype /Image /Width 1 /Height 1 " +
		"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Length " + itoa(len(imgStream)) + " >>\nstream\n" + imgStream + "\nendstream\nendobj\n\n"

	return assembleXref(pdf, obj1, obj2, obj3, obj4, obj5, obj6)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func pad10(n int) string {
	s := itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// assembleXref concatenates header + objects and appends a classic xref +
// trailer (Root = 1 0 R). parts[0] is the PDF header.
func assembleXref(parts ...string) []byte {
	header := parts[0]
	objs := parts[1:]
	body := header
	offsets := make([]int, len(objs))
	cur := len(header)
	for i, o := range objs {
		offsets[i] = cur
		body += o
		cur += len(o)
	}
	xrefOffset := len(body)
	xref := "xref\n0 " + itoa(len(objs)+1) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + itoa(len(objs)+1) + " /Root 1 0 R >>\nstartxref\n" + itoa(xrefOffset) + "\n%%EOF\n"
	return []byte(body + xref + trailer)
}

// writeTempPDF writes content to a temp file and returns its path.
func writeTempPDF(t *testing.T, name string, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write temp pdf: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// `dump stream --page N --ops` emits NDJSON: one JSON object per operator,
// each {op, params, srcLineStart, srcLineEnd}. The number of NDJSON lines
// equals len(default output's Formatted) and each line's
// op/srcLineStart/srcLineEnd matches the corresponding FormattedLine from the
// DEFAULT `dump stream --page N` output (parity, not re-derivation).
// ---------------------------------------------------------------------------

func TestStreamOps_NDJSONParityWithDefaultFormatted(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	// JSON output -> the Formatted array is the source of truth (--json opt-in).
	defStdout, _, defEC := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", pdfPath)
	if defEC != 0 {
		t.Fatalf("default run exit %d", defEC)
	}
	var def struct {
		Formatted []struct {
			Operator     string `json:"operator"`
			SrcLineStart int    `json:"srcLineStart"`
			SrcLineEnd   int    `json:"srcLineEnd"`
		} `json:"formatted"`
	}
	mustParseJSON(t, defStdout, &def)
	if len(def.Formatted) == 0 {
		t.Fatal("default Formatted is empty -- fixture broken")
	}

	// --ops output -> NDJSON.
	opsStdout, _, opsEC := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", pdfPath)
	if opsEC != 0 {
		t.Fatalf("--ops run exit %d (flag not implemented?)", opsEC)
	}
	objs := parseNDJSON(t, opsStdout)

	if len(objs) != len(def.Formatted) {
		t.Fatalf("NDJSON line count = %d, want len(Formatted) = %d", len(objs), len(def.Formatted))
	}

	for i, o := range objs {
		// Required keys.
		for _, key := range []string{"op", "params", "srcLineStart", "srcLineEnd"} {
			if _, ok := o[key]; !ok {
				t.Errorf("operator[%d] missing key %q", i, key)
			}
		}
		// Parity with the default Formatted line.
		if got, _ := o["op"].(string); got != def.Formatted[i].Operator {
			t.Errorf("operator[%d].op = %q, want %q", i, got, def.Formatted[i].Operator)
		}
		if got, _ := o["srcLineStart"].(float64); int(got) != def.Formatted[i].SrcLineStart {
			t.Errorf("operator[%d].srcLineStart = %v, want %d", i, got, def.Formatted[i].SrcLineStart)
		}
		if got, _ := o["srcLineEnd"].(float64); int(got) != def.Formatted[i].SrcLineEnd {
			t.Errorf("operator[%d].srcLineEnd = %v, want %d", i, got, def.Formatted[i].SrcLineEnd)
		}
	}
}

// ---------------------------------------------------------------------------
// --ops output is NDJSON (one object per line), NOT a JSON array. The whole
// stdout must NOT parse as a single JSON value when there is more than one
// operator.
// ---------------------------------------------------------------------------

func TestStreamOps_IsNDJSONNotArray(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("--ops run exit %d", ec)
	}
	objs := parseNDJSON(t, stdout)
	if len(objs) < 2 {
		t.Fatalf("expected multiple operators, got %d", len(objs))
	}
	// Whole stdout must NOT be a single JSON array/object.
	var whole any
	if json.Unmarshal([]byte(strings.TrimSpace(stdout)), &whole) == nil {
		t.Errorf("stdout parsed as a single JSON value -- expected NDJSON (one object per line)")
	}
}

// ---------------------------------------------------------------------------
// --ops is mutually exclusive with --raw; passing both is a usage error (exit
// 1).
// ---------------------------------------------------------------------------

func TestStreamOps_ConflictWithRaw_UsageError(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--ops", "--raw", "--page", "1", pdfPath)
	if ec != 1 {
		t.Errorf("--ops + --raw expected exit 1 (usage), got %d", ec)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty on usage error, got: %s", stdout)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Error("stderr should carry a usage/error message")
	}
}

// ---------------------------------------------------------------------------
// empty/no-Contents page under --ops emits ZERO NDJSON lines on stdout (not a
// lone non-NDJSON error object), surfaces the condition on stderr, with a
// defined exit code. stdout must stay jq -c -safe (empty or only valid NDJSON
// lines).
// ---------------------------------------------------------------------------

func TestStreamOps_NoContents_ZeroLinesCleanStdout(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf") // page 1 has no /Contents

	stdout, _, _ := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", pdfPath)

	// stdout must be either empty OR only valid NDJSON object lines -- never a
	// bare error object that breaks downstream `jq -c`.
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return // zero lines: the recommended contract
	}
	for i, line := range strings.Split(trimmed, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("stdout line %d not valid NDJSON: %v\nline: %q", i+1, err, line)
		}
		// A per-operator object is fine; an {"error": ...} object on stdout is not.
		if _, isErr := m["error"]; isErr {
			t.Errorf("error object leaked to stdout under --ops -- belongs on stderr")
		}
	}
}

// ---------------------------------------------------------------------------
// A `Do` op on a PAGE content stream naming an Image XObject carries
// resourceType "Image" and objectRef "N G R". Uses the existing
// image-xobject.pdf fixture: its page 1 /Resources/XObject has /Im0 (Subtype
// /Image). NOTE: that fixture's page may have no /Contents stream with a Do op;
// this test additionally builds a fixture with an explicit `Do Im0`.
// ---------------------------------------------------------------------------

func TestStreamOps_DoImage_CarriesResourceType(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "form-image.pdf", formXObjectPDF())

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("--ops run exit %d", ec)
	}
	objs := parseNDJSON(t, stdout)

	im := findDoOp(objs, "/Im0")
	if im == nil {
		t.Fatalf("no `Do /Im0` operator found in --ops output\nstdout: %s", stdout)
	}
	if rt, _ := im["resourceType"].(string); rt != "Image" {
		t.Errorf("Do /Im0 resourceType = %q, want \"Image\"", rt)
	}
	if ref, _ := im["objectRef"].(string); !strings.Contains(ref, "6 0 R") {
		t.Errorf("Do /Im0 objectRef = %q, want it to name \"6 0 R\"", ref)
	}
}

// ---------------------------------------------------------------------------
// A `Do` op on a PAGE content stream naming a Form XObject carries
// resourceType "Form" and objectRef "N G R".
// ---------------------------------------------------------------------------

func TestStreamOps_DoForm_CarriesResourceType(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "form-image.pdf", formXObjectPDF())

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("--ops run exit %d", ec)
	}
	objs := parseNDJSON(t, stdout)

	fm := findDoOp(objs, "/Fm0")
	if fm == nil {
		t.Fatalf("no `Do /Fm0` operator found in --ops output\nstdout: %s", stdout)
	}
	if rt, _ := fm["resourceType"].(string); rt != "Form" {
		t.Errorf("Do /Fm0 resourceType = %q, want \"Form\"", rt)
	}
	if ref, _ := fm["objectRef"].(string); !strings.Contains(ref, "5 0 R") {
		t.Errorf("Do /Fm0 objectRef = %q, want it to name \"5 0 R\"", ref)
	}
}

// unknownDoPDF builds a single-page PDF whose content stream draws a Do naming
// "/Ghost", which is absent from the page's /Resources/XObject (only /Fm0 is
// present). Used for the negative path: a Do whose operand does not resolve
// must emit the op WITHOUT resourceType and must not crash.
func unknownDoPDF() []byte {
	pdf := "%PDF-1.4\n"
	pageStream := "q /Ghost Do Q\n"
	formStream := "0 0 100 100 re f\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] " +
		"/Resources << /XObject << /Fm0 5 0 R >> >> /Contents 4 0 R >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Length " + itoa(len(pageStream)) + " >>\nstream\n" + pageStream + "\nendstream\nendobj\n\n"
	obj5 := "5 0 obj\n<< /Type /XObject /Subtype /Form /FormType 1 /BBox [0 0 100 100] /Length " +
		itoa(len(formStream)) + " >>\nstream\n" + formStream + "\nendstream\nendobj\n\n"

	return assembleXref(pdf, obj1, obj2, obj3, obj4, obj5)
}

// ---------------------------------------------------------------------------
// A `Do` op whose operand name is NOT in the page's /Resources/XObject is
// emitted WITHOUT a resourceType key and does not crash (exit 0, valid
// NDJSON). Guards classifyDo's "leave unannotated" branch (the negative path:
// do not crash, do not mislabel).
// ---------------------------------------------------------------------------

func TestStreamOps_DoUnknownName_EmitsWithoutResourceType(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "unknown-do.pdf", unknownDoPDF())

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("--ops run exit %d (unresolved Do should not error)", ec)
	}
	objs := parseNDJSON(t, stdout)

	ghost := findDoOp(objs, "/Ghost")
	if ghost == nil {
		t.Fatalf("no `Do /Ghost` operator found in --ops output\nstdout: %s", stdout)
	}
	if _, has := ghost["resourceType"]; has {
		t.Errorf("Do /Ghost (unresolved) must omit resourceType, got %v", ghost["resourceType"])
	}
	if _, has := ghost["objectRef"]; has {
		t.Errorf("Do /Ghost (unresolved) must omit objectRef, got %v", ghost["objectRef"])
	}
}

// otherSubtypeDoPDF builds a single-page PDF whose content stream draws a Do
// naming "/No0", which IS present in the page's /Resources/XObject but resolves
// to an XObject carrying NO /Subtype. classifyDo finds the entry but its subtype
// matches neither Image nor Form, exercising the switch-default branch -- the op
// must emit WITHOUT resourceType (and not mislabel). (A /PS subtype cannot be
// used: pdfcpu rejects a PostScript XObject as malformed at open time.)
func otherSubtypeDoPDF() []byte {
	pdf := "%PDF-1.4\n"
	pageStream := "q /No0 Do Q\n"
	formStream := "0 0 10 10 re f\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] " +
		"/Resources << /XObject << /No0 5 0 R >> >> /Contents 4 0 R >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Length " + itoa(len(pageStream)) + " >>\nstream\n" + pageStream + "\nendstream\nendobj\n\n"
	// /Type /XObject but no /Subtype: GetXObjectResources reports Subtype "".
	obj5 := "5 0 obj\n<< /Type /XObject /FormType 1 /BBox [0 0 10 10] /Length " +
		itoa(len(formStream)) + " >>\nstream\n" + formStream + "\nendstream\nendobj\n\n"

	return assembleXref(pdf, obj1, obj2, obj3, obj4, obj5)
}

// ---------------------------------------------------------------------------
// A `Do` op whose operand resolves to an XObject whose /Subtype is neither
// /Image nor /Form (here: absent) is emitted WITHOUT a resourceType key and
// does not crash. Guards classifyDo's switch-default branch -- distinct from
// the unresolved-name (`!ok`) branch covered by. The spec requires this case
// be decided AND tested.
// ---------------------------------------------------------------------------

func TestStreamOps_DoOtherSubtype_EmitsWithoutResourceType(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "other-subtype-do.pdf", otherSubtypeDoPDF())

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("--ops run exit %d (non-Image/Form subtype should not error)", ec)
	}
	objs := parseNDJSON(t, stdout)

	no := findDoOp(objs, "/No0")
	if no == nil {
		t.Fatalf("no `Do /No0` operator found in --ops output\nstdout: %s", stdout)
	}
	if _, has := no["resourceType"]; has {
		t.Errorf("Do /No0 (no classifiable /Subtype) must omit resourceType, got %v", no["resourceType"])
	}
}

// nonNameDoPDF builds a single-page PDF whose content stream draws a Do with a
// NON-NAME operand ("12 Do" instead of "/Name Do"). classifyDo skips operands
// that are not /-prefixed names, so the Do op must emit WITHOUT resourceType and
// must not crash. The page's /Resources/XObject is present (so the page-resource
// lookup itself succeeds) but the operand never keys into it.
func nonNameDoPDF() []byte {
	pdf := "%PDF-1.4\n"
	pageStream := "q 12 Do Q\n"
	formStream := "0 0 100 100 re f\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] " +
		"/Resources << /XObject << /Fm0 5 0 R >> >> /Contents 4 0 R >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Length " + itoa(len(pageStream)) + " >>\nstream\n" + pageStream + "\nendstream\nendobj\n\n"
	obj5 := "5 0 obj\n<< /Type /XObject /Subtype /Form /FormType 1 /BBox [0 0 100 100] /Length " +
		itoa(len(formStream)) + " >>\nstream\n" + formStream + "\nendstream\nendobj\n\n"

	return assembleXref(pdf, obj1, obj2, obj3, obj4, obj5)
}

// ---------------------------------------------------------------------------
// A `Do` op with a NON-NAME operand (a number, not a /Name) is emitted WITHOUT a
// resourceType key and does not crash (exit 0, valid NDJSON). Guards
// classifyDo's "operand is not a /-prefixed name" skip branch -- distinct from
// the unresolved-name and unclassified-subtype branches. ("A Do with a non-name
// operand ... emits the op without resourceType".)
// ---------------------------------------------------------------------------

func TestStreamOps_DoNonNameOperand_EmitsWithoutResourceType(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "nonname-do.pdf", nonNameDoPDF())

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("--ops run exit %d (non-name Do operand should not error)", ec)
	}
	objs := parseNDJSON(t, stdout)

	// The Do op carries "12" (a non-name operand), so findDoOp's name match
	// won't catch it -- locate it by operator + operand value directly.
	var do map[string]any
	for _, o := range objs {
		if op, _ := o["op"].(string); op != "Do" {
			continue
		}
		params, _ := o["params"].([]any)
		for _, p := range params {
			if s, ok := p.(string); ok && s == "12" {
				do = o
			}
		}
	}
	if do == nil {
		t.Fatalf("no `Do` op with non-name operand \"12\" found\nstdout: %s", stdout)
	}
	if _, has := do["resourceType"]; has {
		t.Errorf("Do with non-name operand must omit resourceType, got %v", do["resourceType"])
	}
	if _, has := do["objectRef"]; has {
		t.Errorf("Do with non-name operand must omit objectRef, got %v", do["objectRef"])
	}
}

// noXObjectResourcesPDF builds a single-page PDF whose content stream draws a Do
// (/Fm0) but whose page /Resources dict has NO /XObject sub-dictionary at all.
// GetXObjectResources returns an empty (non-nil) map for such a page, so
// classifyDo finds no matching entry and the Do op must emit WITHOUT
// resourceType (and not crash).
func noXObjectResourcesPDF() []byte {
	pdf := "%PDF-1.4\n"
	pageStream := "q /Fm0 Do Q\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] " +
		"/Resources << >> /Contents 4 0 R >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Length " + itoa(len(pageStream)) + " >>\nstream\n" + pageStream + "\nendstream\nendobj\n\n"

	return assembleXref(pdf, obj1, obj2, obj3, obj4)
}

// ---------------------------------------------------------------------------
// A `Do` op on a page whose /Resources has NO /XObject sub-dictionary is
// emitted WITHOUT a resourceType key and does not crash (exit 0, valid NDJSON).
// Guards the "page has no /Resources/XObject" path: GetXObjectResources yields
// an empty map and classifyDo finds no entry. ("A Do ... appearing where the
// page has no /Resources/XObject also emits the op without resourceType".)
// ---------------------------------------------------------------------------

func TestStreamOps_DoNoXObjectResources_EmitsWithoutResourceType(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "no-xobj-res.pdf", noXObjectResourcesPDF())

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("--ops run exit %d (page with no /Resources/XObject should not error)", ec)
	}
	objs := parseNDJSON(t, stdout)

	fm := findDoOp(objs, "/Fm0")
	if fm == nil {
		t.Fatalf("no `Do /Fm0` operator found in --ops output\nstdout: %s", stdout)
	}
	if _, has := fm["resourceType"]; has {
		t.Errorf("Do /Fm0 on a page with no /Resources/XObject must omit resourceType, got %v", fm["resourceType"])
	}
	if _, has := fm["objectRef"]; has {
		t.Errorf("Do /Fm0 on a page with no /Resources/XObject must omit objectRef, got %v", fm["objectRef"])
	}
}

// findDoOp returns the first NDJSON operator object whose op is "Do" and whose
// params include the given XObject name (e.g. "/Im0"). Returns nil if none.
func findDoOp(objs []map[string]any, name string) map[string]any {
	for _, o := range objs {
		if op, _ := o["op"].(string); op != "Do" {
			continue
		}
		params, _ := o["params"].([]any)
		for _, p := range params {
			if s, ok := p.(string); ok && s == name {
				return o
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 006: --json is mutually exclusive with the payload selectors --raw and
// --ops. Passing them together is a usage error (exit 1) with empty stdout --
// NET-NEW validation: before the flip --json was a no-op that combined
// silently. Do NOT retrofit --ops under --json.
// ---------------------------------------------------------------------------

func TestStream_RawJSON_Rejected(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--raw", "--json", "--page", "1", pdfPath)
	if ec != 1 {
		t.Errorf("--raw --json expected exit 1 (usage), got %d", ec)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("--raw --json: stdout should be empty on usage error, got: %s", stdout)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Error("--raw --json: stderr should carry a usage/error message")
	}
}

func TestStream_OpsJSON_Rejected(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--ops", "--json", "--page", "1", pdfPath)
	if ec != 1 {
		t.Errorf("--ops --json expected exit 1 (usage), got %d", ec)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("--ops --json: stdout should be empty on usage error, got: %s", stdout)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Error("--ops --json: stderr should carry a usage/error message")
	}
}
