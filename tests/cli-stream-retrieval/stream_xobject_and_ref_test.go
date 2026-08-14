// Acceptance tests for `dump stream --xobject NAME` and
// `dump stream --ref REF` (item 3).
//
// Black-box: build the CLI binary and run it as a subprocess.
//
// Run: cd tests/cli-stream-retrieval && go test -v -count=1 ./...
package cli_stream_retrieval_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// `dump stream --xobject Fm0 --page 1` resolves the Form XObject named Fm0 in
// page 1's /Resources/XObject to its stream object and emits that stream's
// content as the same JSON shape as a page content stream (nodeId, raw,
// tokenized), exit 0. Fm0's stream draws "0 0 100 100 re f" so raw must
// contain "re".
// ---------------------------------------------------------------------------

func TestStreamXObject_FormByName_EmitsStreamContent(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "form-image.pdf", formXObjectPDF())

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--json", "--xobject", "Fm0", "--page", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("--xobject Fm0 --page 1 exit %d (flag not implemented?)", ec)
	}
	var result map[string]any
	mustParseJSON(t, stdout, &result)
	for _, key := range []string{"nodeId", "raw", "tokenized"} {
		if _, ok := result[key]; !ok {
			t.Errorf("form stream output missing key %q", key)
		}
	}
	if raw, _ := result["raw"].(string); !strings.Contains(raw, "re") {
		t.Errorf("form stream raw = %q, want it to contain the form's `re` operator", raw)
	}
}

// ---------------------------------------------------------------------------
// `--xobject Fm0` honors --ops (same output path as a page stream): emits
// NDJSON operator objects for the form's content.
// ---------------------------------------------------------------------------

func TestStreamXObject_FormByName_HonorsOps(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "form-image.pdf", formXObjectPDF())

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--xobject", "Fm0", "--page", "1", "--ops", pdfPath)
	if ec != 0 {
		t.Fatalf("--xobject --ops exit %d", ec)
	}
	objs := parseNDJSON(t, stdout)
	if len(objs) == 0 {
		t.Fatalf("--xobject --ops produced no NDJSON operators\nstdout: %s", stdout)
	}
	foundRe := false
	for _, o := range objs {
		if op, _ := o["op"].(string); op == "re" {
			foundRe = true
		}
	}
	if !foundRe {
		t.Errorf("form stream --ops did not surface the `re` operator")
	}
}

// ---------------------------------------------------------------------------
// `--xobject NoSuch --page 1` (unknown name) yields a clear JSON error on
// stderr + exit 2.
// ---------------------------------------------------------------------------

func TestStreamXObject_UnknownName_JSONErrorExit2(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "form-image.pdf", formXObjectPDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--xobject", "NoSuch", "--page", "1", pdfPath)
	if ec != 2 {
		t.Errorf("unknown --xobject expected exit 2, got %d", ec)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty on error, got: %s", stdout)
	}
	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr not JSON: %v\nraw: %s", err, stderr)
	}
	if _, ok := errObj["error"]; !ok {
		t.Error("stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// `--xobject Fm0` with NEITHER --page NOR --ref is a usage error (exit 1)
// whose message names the missing flag. This is the reworked guard: the old
// unconditional `--page < 1 -> exit 1` must no longer apply to --xobject
// paths, but --xobject ALONE is still a usage error.
// ---------------------------------------------------------------------------

func TestStreamXObject_NoPageNoRef_UsageError(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "form-image.pdf", formXObjectPDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--xobject", "Fm0", pdfPath)
	if ec != 1 {
		t.Errorf("--xobject alone expected exit 1 (usage), got %d", ec)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty on usage error, got: %s", stdout)
	}
	low := strings.ToLower(stderr)
	if !strings.Contains(low, "page") && !strings.Contains(low, "ref") {
		t.Errorf("usage error should name the missing --page/--ref flag, got: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// An Image-XObject NAME still emits via the stream path (the image bytes
// tokenize to a near-empty/garbage operator list, NOT a crash). Exit 0 and
// valid JSON with a (possibly empty) tokenized array.
// ---------------------------------------------------------------------------

func TestStreamXObject_ImageByName_EmitsWithoutCrash(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "form-image.pdf", formXObjectPDF())

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--json", "--xobject", "Im0", "--page", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("--xobject Im0 exit %d, want 0 (image bytes still tokenize, no crash)", ec)
	}
	var result map[string]any
	mustParseJSON(t, stdout, &result)
	if _, ok := result["tokenized"]; !ok {
		t.Error("image-stream output missing 'tokenized' key")
	}
}

// ---------------------------------------------------------------------------
// `dump stream --ref "4 0 R"` where object 4 is the page content stream emits
// that object's decoded content stream (exit 0, nodeId/raw/tokenized). Object
// 4 in formXObjectPDF is the page content stream
// ("q /Fm0 Do Q ...").
// ---------------------------------------------------------------------------

func TestStreamRef_ContentStreamObject_EmitsContent(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "form-image.pdf", formXObjectPDF())

	stdout, _, ec := runCLI(t, bin, "dump", "stream", "--json", "--ref", "4 0 R", pdfPath)
	if ec != 0 {
		t.Fatalf("--ref \"4 0 R\" exit %d (flag not implemented?)", ec)
	}
	var result map[string]any
	mustParseJSON(t, stdout, &result)
	if raw, _ := result["raw"].(string); !strings.Contains(raw, "Do") {
		t.Errorf("--ref content stream raw = %q, want it to contain `Do`", raw)
	}
}

// ---------------------------------------------------------------------------
// `--ref REF` where REF is NOT a stream object yields a clear type error + exit
// 2. GetContentStream returns a non-error ContentStreamData{Error:"node is not
// a stream object"}; the command MUST detect that Error field and map it to
// exit 2 (NOT exit 0). Object 1 is the catalog dict (not a stream).
// ---------------------------------------------------------------------------

func TestStreamRef_NonStream_TypeErrorExit2(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "form-image.pdf", formXObjectPDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--ref", "1 0 R", pdfPath)
	if ec != 2 {
		t.Errorf("--ref to non-stream expected exit 2, got %d", ec)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty on type error, got: %s", stdout)
	}
	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr not JSON: %v\nraw: %s", err, stderr)
	}
	if msg, ok := errObj["error"]; !ok || !strings.Contains(strings.ToLower(msg), "stream") {
		t.Errorf("error should mention the node is not a stream, got: %v", errObj)
	}
}

// ---------------------------------------------------------------------------
// `--ref REF` to a non-existent object yields a not-found error + exit
// 2.
// ---------------------------------------------------------------------------

func TestStreamRef_NotFound_Exit2(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := writeTempPDF(t, "form-image.pdf", formXObjectPDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--ref", "9999 0 R", pdfPath)
	if ec != 2 {
		t.Errorf("--ref to missing object expected exit 2, got %d", ec)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty on error, got: %s", stdout)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Error("stderr should carry a not-found error")
	}
}
