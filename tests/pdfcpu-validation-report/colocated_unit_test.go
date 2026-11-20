package pdfcpu_validation_report_test

import "testing"

// The pdfcore unit tests for the pdfcpu check, under the race detector since
// the check runs off the document lock.
func TestPdfcoreCheckUnitTests(t *testing.T) {
	out, err := runGoTest(t, "-race", "-count=1", "-run", "TestPdfcpuProfile_|TestPdfcpuCheck_|TestParseFile_", "./internal/pdfcore/")
	if err != nil {
		t.Fatalf("pdfcore pdfcpu check unit tests failed:\n%s", out)
	}
}

// The cmd/cli tests for the pdfcpu help line, the synopsis, and the CLI and
// GUI binding returning the same problems.
func TestCLIPdfcpuUnitTests(t *testing.T) {
	out, err := runGoTest(t, "-count=1", "-run",
		"TestProfileRuleLines_PdfcpuLineAndUnchangedStructuralLines|TestValidateSynopsisNamesThePdfcpuProfile|TestPdfcpuProblemsMatchBetweenCLIAndService",
		"./cmd/cli/")
	if err != nil {
		t.Fatalf("cmd/cli pdfcpu tests failed:\n%s", out)
	}
}
