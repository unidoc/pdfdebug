package embedded_data_inspector_test

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 13.2-INTG-010 [P0] AC4: `dump embedded --ref "N G R"` writes the RAW decoded
// bytes of the /EmbeddedFile to stdout so it can be redirected to a file.
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_ByRefWritesRawBytes(t *testing.T) {
	bin := buildCLI(t)
	payload := "<?xml version=\"1.0\"?><CrossIndustryInvoice>data</CrossIndustryInvoice>"
	pdf := writeTempPDF(t, "embedded.pdf", embeddedPDF(payload))

	// Object 4 is the /EmbeddedFile stream.
	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--ref", "4 0 R", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, string(stderr))
	}
	if string(stdout) != payload {
		t.Errorf("extracted bytes mismatch\n got: %q\nwant: %q", string(stdout), payload)
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-011 [P1] AC4 (Postel): `--ref obj:G:N` is also accepted.
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_AcceptsNodeIDRefForm(t *testing.T) {
	bin := buildCLI(t)
	payload := "<x>node-id-form</x>"
	pdf := writeTempPDF(t, "embedded.pdf", embeddedPDF(payload))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--ref", "obj:0:4", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, string(stderr))
	}
	if string(stdout) != payload {
		t.Errorf("obj:G:N form extracted bytes mismatch\n got: %q\nwant: %q", string(stdout), payload)
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-012 [P0] AC4: `--name <display>` resolves a single match and
// extracts its bytes.
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_ByNameSingleMatch(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "multi.pdf", twoAttachmentPDF())

	// "solo.xml" is the unique name -> single match extracts its bytes.
	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--name", "solo.xml", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, string(stderr))
	}
	if string(stdout) != "<solo>only</solo>" {
		t.Errorf("--name single-match bytes mismatch, got %q", string(stdout))
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-013 [P1] AC4: extraction emits bytes regardless of --json (it is a
// payload selector, like `dump stream --raw`, not a format).
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_JSONDoesNotWrapPayload(t *testing.T) {
	bin := buildCLI(t)
	payload := "<x>payload-not-json-wrapped</x>"
	pdf := writeTempPDF(t, "embedded.pdf", embeddedPDF(payload))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--ref", "4 0 R", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, string(stderr))
	}
	if string(stdout) != payload {
		t.Errorf("--json must NOT wrap the extraction payload; got %q want %q", string(stdout), payload)
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-014 [P0] AC4: extraction of an unknown ref fails -- stdout stays
// EMPTY, the error goes to stderr, and the exit code is non-zero (so a redirect
// never captures an error blob as the payload).
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_UnknownRefEmptyStdoutNonZero(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "embedded.pdf", embeddedPDF("<x/>"))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--ref", "999 0 R", pdf)
	if ec == 0 {
		t.Fatalf("unknown ref must exit non-zero, got 0")
	}
	if len(stdout) != 0 {
		t.Errorf("stdout must be EMPTY on failure, got %q", string(stdout))
	}
	if strings.TrimSpace(string(stderr)) == "" {
		t.Errorf("expected an error message on stderr")
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-015 [P0] AC4/AC8: a /Filespec ref whose target has no /EmbeddedFile
// (or points at a non-stream) fails with empty stdout + stderr + non-zero exit.
// Here --ref points at the /Pages dict (object 2), which is not an EmbeddedFile.
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_NonEmbeddedFileRefFails(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "embedded.pdf", embeddedPDF("<x/>"))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--ref", "2 0 R", pdf)
	if ec == 0 {
		t.Fatalf("non-EmbeddedFile ref must exit non-zero, got 0")
	}
	if len(stdout) != 0 {
		t.Errorf("stdout must be EMPTY on failure, got %q", string(stdout))
	}
	if strings.TrimSpace(string(stderr)) == "" {
		t.Errorf("expected an error message on stderr")
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-016 [P0] AC4: `--name` with ZERO matches = stderr error + non-zero
// exit + empty stdout.
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_NameZeroMatch(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "embedded.pdf", embeddedPDF("<x/>"))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--name", "does-not-exist.xml", pdf)
	if ec == 0 {
		t.Fatalf("zero-match --name must exit non-zero, got 0")
	}
	if len(stdout) != 0 {
		t.Errorf("stdout must be EMPTY on zero match, got %q", string(stdout))
	}
	if strings.TrimSpace(string(stderr)) == "" {
		t.Errorf("expected an error message on stderr")
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-017 [P0] AC4: `--name` with MULTIPLE matches must NOT silently
// extract one -- it errors to stderr, lists the matching refs, and exits
// non-zero with empty stdout. "dup.xml" matches two attachments.
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_NameMultiMatchListsRefs(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "multi.pdf", twoAttachmentPDF())

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--name", "dup.xml", pdf)
	if ec == 0 {
		t.Fatalf("multi-match --name must exit non-zero, got 0")
	}
	if len(stdout) != 0 {
		t.Errorf("multi-match must NOT emit a payload; stdout must be empty, got %q", string(stdout))
	}
	se := string(stderr)
	if strings.TrimSpace(se) == "" {
		t.Errorf("expected a disambiguation error on stderr")
	}
	// The two matching EmbeddedFile object refs (4 and 8) must be listed so the
	// user can disambiguate with --ref.
	if !strings.Contains(se, "4") || !strings.Contains(se, "8") {
		t.Errorf("stderr must list the matching refs (4 and 8) for disambiguation:\n%s", se)
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-018 [P0] AC4: `--ref` and `--name` together are mutually exclusive
// -- usage error on stderr, non-zero exit, empty stdout.
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_RefAndNameMutuallyExclusive(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "embedded.pdf", embeddedPDF("<x/>"))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--ref", "4 0 R", "--name", "factur-x.xml", pdf)
	if ec == 0 {
		t.Fatalf("--ref + --name must exit non-zero, got 0")
	}
	if len(stdout) != 0 {
		t.Errorf("stdout must be EMPTY on a usage error, got %q", string(stdout))
	}
	if !strings.Contains(strings.ToLower(string(stderr)), "mutually exclusive") {
		t.Errorf("expected a 'mutually exclusive' usage error on stderr:\n%s", string(stderr))
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-019 [P1] AC4/AC8: `--name` resolving to a SINGLE match whose
// filespec has no /EmbeddedFile stream fails -- stdout stays empty, the error
// goes to stderr, and the exit is non-zero (cmd_embedded.go:172-175). The
// single-match path must not blindly extract a stream-less entry.
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_NameSingleMatchNoStreamFails(t *testing.T) {
	bin := buildCLI(t)
	// A uniquely-named filespec (object 6) that carries NO /EF entry.
	pdf := writeTempPDF(t, "no-ef.pdf", assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [6 0 R] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\nnull\nendobj\n",
		"5 0 obj\nnull\nendobj\n",
		"6 0 obj\n<< /Type /Filespec /F (orphan.dat) /UF (orphan.dat) /AFRelationship /Unspecified >>\nendobj\n",
	}, 1, 0))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--name", "orphan.dat", pdf)
	if ec == 0 {
		t.Fatalf("single match without /EmbeddedFile must exit non-zero, got 0")
	}
	if len(stdout) != 0 {
		t.Errorf("stdout must be EMPTY when the named entry has no stream, got %q", string(stdout))
	}
	if strings.TrimSpace(string(stderr)) == "" {
		t.Errorf("expected an error message on stderr")
	}
}
