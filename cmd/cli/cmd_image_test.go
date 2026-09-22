// Co-located unit tests for the plain-text image writer's conditional rows.
// printImagePlain takes an ImageData and an io.Writer, so the row shapes are
// driven directly rather than through a fixture PDF.
package main

import (
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// imageRowValue returns the value of the named row, and false when the block
// carries no such row. kvWriter aligns the value column, so the label is
// matched with its colon and the padding is trimmed off.
func imageRowValue(block, label string) (string, bool) {
	for _, line := range strings.Split(block, "\n") {
		if rest, ok := strings.CutPrefix(line, label+":"); ok {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}

// A soft mask present with no reference to report - a direct stream, or the
// /None name writers borrow from the ExtGState entry - is said in words. The
// dash this writer uses elsewhere means the value is not there at all, which is
// the one thing the three-state pointer exists to keep distinguishable.
func TestPrintImagePlainSMaskWithoutAReference(t *testing.T) {
	inline := ""
	var out strings.Builder
	if err := printImagePlain(&out, &pdfcore.ImageData{
		ObjectRef:            "4 0 R",
		AdobeMarker:          pdfcore.AdobeMarkerNotApplicable,
		SampleInterpretation: "Normal (default)",
		SMask:                &inline,
	}); err != nil {
		t.Fatalf("printImagePlain: %v", err)
	}
	value, ok := imageRowValue(out.String(), "SMask")
	if !ok {
		t.Fatalf("expected a soft-mask row:\n%s", out.String())
	}
	if value == "-" {
		t.Fatalf("a soft mask that is present reads as absent")
	}
	if value != "present (no reference)" {
		t.Errorf("soft-mask row = %q, want presence stated in words", value)
	}
}

// The evidence row is shown when the array carries something, matching the
// panel. An array that is present but empty has no numbers to show.
func TestPrintImagePlainDecodeRowNeedsNumbers(t *testing.T) {
	var out strings.Builder
	if err := printImagePlain(&out, &pdfcore.ImageData{
		ObjectRef:            "4 0 R",
		AdobeMarker:          pdfcore.AdobeMarkerNotApplicable,
		SampleInterpretation: "Normal (default)",
		Decode:               []float64{},
	}); err != nil {
		t.Fatalf("printImagePlain: %v", err)
	}
	if value, ok := imageRowValue(out.String(), "Decode"); ok {
		t.Errorf("an empty array earns no evidence row, got %q", value)
	}
}

// A stream that failed to decode carries no MimeType, and the error row no
// longer ends the block, so the rows behind it have to say what they say for a
// value that is not there.
func TestPrintImagePlainErrorArmUsesTheDashForMissingValues(t *testing.T) {
	var out strings.Builder
	if err := printImagePlain(&out, &pdfcore.ImageData{
		ObjectRef:            "4 0 R",
		Error:                "failed to decode image stream: invalid JPEG format",
		AdobeMarker:          pdfcore.AdobeMarkerAbsent,
		SampleInterpretation: "Normal (default)",
	}); err != nil {
		t.Fatalf("printImagePlain: %v", err)
	}
	value, ok := imageRowValue(out.String(), "MimeType")
	if !ok {
		t.Fatalf("expected a mime-type row:\n%s", out.String())
	}
	if value != "-" {
		t.Errorf("mime-type row = %q, want the dash this writer uses for an absent value", value)
	}
}

// pdfcpu's messages end with a newline. The error row sits mid-block now, so an
// untrimmed value opens a blank line between the rows.
func TestPrintImagePlainErrorRowDoesNotBreakTheBlock(t *testing.T) {
	var out strings.Builder
	if err := printImagePlain(&out, &pdfcore.ImageData{
		ObjectRef:            "4 0 R",
		Error:                "failed to render image: pdfcpu: corrupt image object\n",
		AdobeMarker:          pdfcore.AdobeMarkerNotApplicable,
		SampleInterpretation: "Normal (default)",
	}); err != nil {
		t.Fatalf("printImagePlain: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			t.Fatalf("blank line %d inside the block:\n%s", i+1, out.String())
		}
	}
}

// The transform byte is reported with its meaning beside the number. A bare
// number only helps a reader who already knows what it means, and an
// unassigned value is still reported as written rather than dropped.
func TestPrintImagePlainNamesTheAdobeTransform(t *testing.T) {
	cases := []struct {
		transform int
		want      string
	}{
		{transform: 0, want: "None (transform 0)"},
		{transform: 1, want: "YCbCr (transform 1)"},
		{transform: 2, want: "YCCK (transform 2)"},
		{transform: 7, want: "Unknown (transform 7)"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			transform := tc.transform
			var out strings.Builder
			if err := printImagePlain(&out, &pdfcore.ImageData{
				ObjectRef:            "4 0 R",
				AdobeMarker:          pdfcore.AdobeMarkerPresent,
				AdobeTransform:       &transform,
				SampleInterpretation: "Normal (default)",
			}); err != nil {
				t.Fatalf("printImagePlain: %v", err)
			}
			value, ok := imageRowValue(out.String(), "AdobeTransform")
			if !ok {
				t.Fatalf("expected a transform row:\n%s", out.String())
			}
			if value != tc.want {
				t.Errorf("transform row = %q, want %q", value, tc.want)
			}
		})
	}
}

// The size row carries the stored size with the decoded estimate after it, each
// half only when it is non-zero, and no row at all when neither is known.
func TestPrintImagePlainSizeRowHalves(t *testing.T) {
	cases := []struct {
		name    string
		stored  int64
		decoded int64
		want    string
		absent  bool
	}{
		{name: "both halves", stored: 2048, decoded: 192, want: "2.0 KB (~192 B in memory)"},
		{name: "a stored size the decode estimate could not be computed for", stored: 2048, want: "2.0 KB"},
		{name: "a decode estimate with no stored bytes", decoded: 192, want: "~192 B in memory"},
		{name: "neither half earns a row", absent: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			if err := printImagePlain(&out, &pdfcore.ImageData{
				ObjectRef:            "4 0 R",
				AdobeMarker:          pdfcore.AdobeMarkerNotApplicable,
				SampleInterpretation: "Normal (default)",
				StoredBytes:          tc.stored,
				DecodedBytes:         tc.decoded,
			}); err != nil {
				t.Fatalf("printImagePlain: %v", err)
			}
			value, ok := imageRowValue(out.String(), "Size")
			if tc.absent {
				if ok {
					t.Fatalf("expected no size row, got %q", value)
				}
				return
			}
			if !ok {
				t.Fatalf("expected a size row:\n%s", out.String())
			}
			if value != tc.want {
				t.Errorf("size row = %q, want %q", value, tc.want)
			}
		})
	}
}

// A colour-space lookup that fails stops the read above the marker walk, but the
// geometry was already taken from the dictionary and still has to be printed.
func TestPrintImagePlainKeepsGeometryWhenTheColorSpaceIsUnreadable(t *testing.T) {
	var out strings.Builder
	if err := printImagePlain(&out, &pdfcore.ImageData{
		ObjectRef:        "4 0 R",
		Error:            "failed to determine color space components: color space could not be resolved: interface conversion",
		Width:            640,
		Height:           480,
		ColorSpace:       "DeviceN",
		BitsPerComponent: 8,
		Filter:           "DCTDecode",
	}); err != nil {
		t.Fatalf("printImagePlain: %v", err)
	}
	for _, row := range []struct{ label, want string }{
		{label: "Width", want: "640"},
		{label: "Height", want: "480"},
		{label: "ColorSpace", want: "DeviceN"},
		{label: "BitsPerComponent", want: "8"},
		{label: "Filter", want: "DCTDecode"},
	} {
		value, ok := imageRowValue(out.String(), row.label)
		if !ok {
			t.Fatalf("expected a %s row:\n%s", row.label, out.String())
		}
		if value != row.want {
			t.Errorf("%s row = %q, want %q", row.label, value, row.want)
		}
	}
}

// A node whose image dictionary was never read carries no object reference, and
// has nothing beyond the two rows it already wrote.
func TestPrintImagePlainStopsWhenTheDictionaryWasNeverRead(t *testing.T) {
	var out strings.Builder
	if err := printImagePlain(&out, &pdfcore.ImageData{
		Error: "not an image XObject",
	}); err != nil {
		t.Fatalf("printImagePlain: %v", err)
	}
	if value, ok := imageRowValue(out.String(), "Width"); ok {
		t.Errorf("geometry row on a node that was never read: Width = %q", value)
	}
	if value, ok := imageRowValue(out.String(), "Interpretation"); ok {
		t.Errorf("verdict row on a node that was never read: Interpretation = %q", value)
	}
}
