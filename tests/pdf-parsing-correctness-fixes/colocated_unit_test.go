package pdf_parsing_correctness_fixes_test

import "testing"

// The pdfcore unit tests for the incrementally signed LTV fixture: open at the
// last revision, /DSS resolution, widget image soft masks and signature
// coverage per revision.
func TestPdfcoreSignedIncrementalLTVUnitTests(t *testing.T) {
	out, err := runGoTest(t, "-count=1", "-run", "TestSignedIncrementalLTV_", "./internal/pdfcore/")
	if err != nil {
		t.Fatalf("pdfcore unit tests failed: %v\n%s", err, out)
	}
}

// The pdfcore table test for shapes pdfcpu's validator rejects, each opening
// through Inspector.Open.
func TestPdfcoreValidatorRejectedShapesOpenUnitTest(t *testing.T) {
	out, err := runGoTest(t, "-count=1", "-run", "TestOpen_ShapesThePdfcpuValidatorRejectsOpen", "./internal/pdfcore/")
	if err != nil {
		t.Fatalf("pdfcore unit test failed: %v\n%s", err, out)
	}
}

// The CLI tests for the open error's detail and diff's file field.
func TestCLIOpenErrorDetailUnitTests(t *testing.T) {
	out, err := runGoTest(t, "-count=1", "-run", "TestOpenError_", "./cmd/cli/")
	if err != nil {
		t.Fatalf("cmd/cli tests failed: %v\n%s", err, out)
	}
}
