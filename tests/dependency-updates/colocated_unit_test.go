package dependency_updates_test

import "testing"

// The pdfcore unit tests for the open path, text strings, the diff collision
// rule and the colour-space fallback.
func TestPdfcoreOpenPathTextStringDiffAndColorSpaceUnitTests(t *testing.T) {
	out, err := runGoTest(t, "-count=1", "-run",
		"TestOpen_|TestDecodeTextString_PDFDocEncodingTable|TestDecodeTextString_BOMWithOddTrailingByteIsADecodeFailure|"+
			"TestTextStringOrRaw_UndecodableStringWithInvalidRawBytesShowsHex|TestScalarLeaf_StringsThatDecodeAlikeShowByteExactSummaries|"+
			"TestScalarLeaf_HexStringBackslashByteShowsDecodedText|TestDeclaredComponents_UnresolvableColorSpaceFallsBackOnError|"+
			"TestDeclaredComponents_MalformedColorSpaceShapesReturnErrorsAndFallBack",
		"./internal/pdfcore/")
	if err != nil {
		t.Fatalf("pdfcore unit tests failed:\n%s", out)
	}
}

// The pdfcore unit tests for the stream decode helper, the LZW counter and the
// image limits against pdfcpu's defaults.
func TestPdfcoreDecodeHelperLZWAndLimitUnitTests(t *testing.T) {
	out, err := runGoTest(t, "-count=1", "-run",
		"TestDecodeStream_|TestCountStages_LZWMatchesPdfcpu|TestImageCeilingsFitPdfcpuDefaultLimits",
		"./internal/pdfcore/")
	if err != nil {
		t.Fatalf("pdfcore unit tests failed:\n%s", out)
	}
}
