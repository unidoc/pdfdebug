// Sample-interpretation reporting for image XObjects: the /Decode array, the
// image /SMask, ImageMask, the Adobe APP14 marker, and the single verdict that
// joins the array and the marker into one answer.
//
// Two switches decide whether a CMYK JPEG's samples are read inverted, written
// in two layers by tools that never spoke to each other: the PDF /Decode array
// and the Adobe APP14 segment inside the JPEG. Neither is reported today, and
// the outcome depends on the combination of the two, so the tool reports one
// verdict with the raw evidence beside it.
//
// Run: cd tests/image-extraction && go test -v -count=1 ./...
package image_extraction_test

import (
	"strings"
	"testing"
)

// The verdict vocabulary is fixed rather than left to the implementation: a
// verdict whose wording varies by code path is neither testable nor quotable in
// a bug report.
const (
	verdictNormalDefault  = "Normal (default)"
	verdictNormalAdobe    = "Normal: /Decode compensates for Adobe-inverted CMYK"
	verdictInvertedAdobe  = "Inverted: Adobe CMYK is stored inverted and no /Decode compensates"
	verdictInvertedNoMark = "Inverted: /Decode inverts, no Adobe APP14 marker"
	verdictInvertedDecode = "Inverted by /Decode"
	verdictNonDefault     = "Non-default /Decode"
	verdictNotClassified  = "Not classified: /Decode on an Indexed or Lab image is not a simple inversion"
	verdictUnknownDecode  = "Unknown: /Decode array unreadable"
	verdictUnknownArity   = "Unknown: /Decode not checkable (colour-component count unresolved)"
	verdictUnknownMarker  = "Unknown: Adobe APP14 marker not checkable (colour-component count unresolved)"
	verdictUnknownChain   = "Unknown: JPEG marker chain unreadable"
	verdictUnknownReach   = "Unknown: JPEG bytes not reachable (DCTDecode behind another filter)"
)

// The marker outcome is a discriminator rather than a boolean, so a consumer
// branches on a value instead of parsing prose. The empty zero value is a sixth
// state: the image dictionary was never read at all.
const (
	markerNotApplicable = "not-applicable"
	markerAbsent        = "absent"
	markerPresent       = "present"
	markerUnparseable   = "unparseable"
	markerNotExamined   = "not-examined"
)

// ---------------------------------------------------------------------------
// The /Decode array is read onto the payload, bounded to two entries per
// colour component, and a malformed array is rejected outright rather than
// stored in part.
// ---------------------------------------------------------------------------

func TestDecodeArrayReporting(t *testing.T) {
	jpegBytes := rgbJPEG(t)

	cases := []struct {
		name        string
		entries     string
		want        []float64
		wantNull    bool
		wantWarning bool
	}{
		{
			name:     "absent leaves the field null",
			entries:  "",
			wantNull: true,
		},
		{
			name:    "the default three-component array is reported verbatim",
			entries: "/Decode [0 1 0 1 0 1]",
			want:    []float64{0, 1, 0, 1, 0, 1},
		},
		{
			name:    "a fully inverted array is reported verbatim",
			entries: "/Decode [1 0 1 0 1 0]",
			want:    []float64{1, 0, 1, 0, 1, 0},
		},
		{
			name:    "an array inverting only some components is reported verbatim",
			entries: "/Decode [1 0 0 1 0 1]",
			want:    []float64{1, 0, 0, 1, 0, 1},
		},
		{
			name:    "fractional bounds survive the read",
			entries: "/Decode [0.2 0.8 0 1 0 1]",
			want:    []float64{0.2, 0.8, 0, 1, 0, 1},
		},
		{
			name:        "an array longer than two entries per component is rejected",
			entries:     "/Decode [0 1 0 1 0 1 0 1 0 1]",
			wantNull:    true,
			wantWarning: true,
		},
		{
			name:        "an odd-length array is rejected",
			entries:     "/Decode [0 1 0]",
			wantNull:    true,
			wantWarning: true,
		},
		{
			name:        "a non-numeric element rejects the whole array",
			entries:     "/Decode [0 1 0 null 0 1]",
			wantNull:    true,
			wantWarning: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, raw := dumpImageJSON(t, "decode.pdf", rgbImagePDF(jpegBytes, tc.entries))
			if tc.wantNull {
				if !isJSONNull(raw, "decode") {
					t.Errorf("expected an explicit null decode, got %s", string(raw["decode"]))
				}
				if tc.wantWarning && !strings.Contains(strings.ToLower(img.Warning), "decode") {
					t.Errorf("expected a warning naming the rejected decode array, got %q", img.Warning)
				}
				return
			}
			if !sameFloats(img.Decode, tc.want) {
				t.Errorf("decode = %v, want %v", img.Decode, tc.want)
			}
		})
	}
}

// An indirect /Decode, and an indirect element inside it, resolve to the same
// array a direct one does.
func TestDecodeArrayResolvesThroughIndirectReferences(t *testing.T) {
	jpegBytes := rgbJPEG(t)

	t.Run("the array itself is an indirect reference", func(t *testing.T) {
		pdf := rgbImagePDF(jpegBytes, "/Decode 5 0 R",
			[]byte("5 0 obj\n[1 0 1 0 1 0]\nendobj\n"))
		img, _ := dumpImageJSON(t, "decode-indirect.pdf", pdf)
		if !sameFloats(img.Decode, []float64{1, 0, 1, 0, 1, 0}) {
			t.Errorf("decode = %v, want the dereferenced array", img.Decode)
		}
	})

	t.Run("an element inside the array is an indirect reference", func(t *testing.T) {
		pdf := rgbImagePDF(jpegBytes, "/Decode [1 0 1 0 1 5 0 R]",
			[]byte("5 0 obj\n0\nendobj\n"))
		img, _ := dumpImageJSON(t, "decode-element-indirect.pdf", pdf)
		if !sameFloats(img.Decode, []float64{1, 0, 1, 0, 1, 0}) {
			t.Errorf("decode = %v, want the array with its element dereferenced", img.Decode)
		}
	})
}

// A /Decode whose value reads as the null object is equivalent to no /Decode at
// all, so the verdict is the default and no rejection is reported. Both shapes
// arrive as an indirect reference: a direct null is dropped by the loader before
// the reader sees it.
func TestDecodeArrayReadingAsNullIsTheSameAsAbsent(t *testing.T) {
	jpegBytes := rgbJPEG(t)

	cases := []struct {
		name  string
		extra [][]byte
	}{
		{
			name:  "a reference to the null object",
			extra: [][]byte{[]byte("5 0 obj\nnull\nendobj\n")},
		},
		{
			name: "a reference to an object that does not exist",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pdf := rgbImagePDF(jpegBytes, "/Decode 5 0 R", tc.extra...)
			img, raw := dumpImageJSON(t, "decode-null.pdf", pdf)
			if !isJSONNull(raw, "decode") {
				t.Errorf("decode = %s, want an explicit null", string(raw["decode"]))
			}
			if strings.Contains(strings.ToLower(img.Warning), "decode") {
				t.Errorf("warning = %q, want no rejection for a key that sets nothing", img.Warning)
			}
			if img.SampleInterpretation != verdictNormalDefault {
				t.Errorf("verdict = %q, want %q", img.SampleInterpretation, verdictNormalDefault)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ImageMask is surfaced. The value is already parsed to size the decode
// ceiling and is currently discarded.
// ---------------------------------------------------------------------------

func TestImageMaskIsSurfaced(t *testing.T) {
	t.Run("a stencil mask reports true", func(t *testing.T) {
		img, _ := dumpImageJSON(t, "stencil.pdf", stencilMaskPDF(""))
		if !img.ImageMask {
			t.Errorf("imageMask = false, want true for a stencil mask")
		}
	})

	t.Run("an ordinary image reports false", func(t *testing.T) {
		img, _ := dumpImageJSON(t, "not-a-mask.pdf", rgbImagePDF(rgbJPEG(t), ""))
		if img.ImageMask {
			t.Errorf("imageMask = true, want false for an image with a colour space")
		}
	})
}

// ---------------------------------------------------------------------------
// The image /SMask is reported as presence plus its object reference, with no
// dereference of the mask image. Three states are distinguishable, because two
// of them are not the same answer: absent, present as a reference, and present
// as something that has no reference to report.
// ---------------------------------------------------------------------------

func TestSMaskReporting(t *testing.T) {
	jpegBytes := rgbJPEG(t)
	maskObj := []byte("5 0 obj\n<< /Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 0 >>\nstream\n\nendstream\nendobj\n")

	t.Run("absent leaves the field null", func(t *testing.T) {
		_, raw := dumpImageJSON(t, "no-smask.pdf", rgbImagePDF(jpegBytes, ""))
		if !isJSONNull(raw, "smask") {
			t.Errorf("expected an explicit null smask, got %s", string(raw["smask"]))
		}
	})

	t.Run("an indirect reference reports the reference", func(t *testing.T) {
		img, _ := dumpImageJSON(t, "smask-ref.pdf", rgbImagePDF(jpegBytes, "/SMask 5 0 R", maskObj))
		if img.SMask == nil {
			t.Fatalf("smask is null, want the object reference")
		}
		if *img.SMask != "5 0 R" {
			t.Errorf("smask = %q, want %q", *img.SMask, "5 0 R")
		}
	})

	// The third state - present as a value with no reference to report, which
	// covers both a direct dictionary and the /None name writers borrow from
	// the ExtGState soft-mask entry - has no fixture here: the loader rejects
	// both shapes at the document level, so neither reaches the reader through
	// a file. It is covered in package, over a dictionary built directly.
}

// ---------------------------------------------------------------------------
// The Adobe APP14 marker walk. The stored bytes are walked marker to marker up
// to SOS; no entropy-coded byte is touched and no pixel is decoded.
// ---------------------------------------------------------------------------

func TestAdobeMarkerOutcomeOnDecodableJPEGs(t *testing.T) {
	jpegBytes := rgbJPEG(t)

	cases := []struct {
		name          string
		segment       []byte
		wantOutcome   string
		wantTransform int
		transformNull bool
	}{
		{
			name:          "no application segment at all",
			segment:       nil,
			wantOutcome:   markerAbsent,
			transformNull: true,
		},
		{
			name:        "an Adobe record with no colour transform",
			segment:     adobeAPP14(0),
			wantOutcome: markerPresent,
		},
		{
			name:          "an Adobe record selecting YCbCr",
			segment:       adobeAPP14(1),
			wantOutcome:   markerPresent,
			wantTransform: 1,
		},
		{
			name:          "an Adobe record selecting YCCK",
			segment:       adobeAPP14(2),
			wantOutcome:   markerPresent,
			wantTransform: 2,
		},
		{
			name:          "an APP14 whose identifier is not Adobe is skipped",
			segment:       foreignAPP14(),
			wantOutcome:   markerAbsent,
			transformNull: true,
		},
		{
			name:          "an APP14 too short to hold the record is skipped",
			segment:       shortAPP14(),
			wantOutcome:   markerAbsent,
			transformNull: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := jpegBytes
			if tc.segment != nil {
				raw = spliceAfterAPP0(t, jpegBytes, tc.segment)
			}
			img, rawJSON := dumpImageJSON(t, "marker.pdf", rgbImagePDF(raw, ""))
			if img.AdobeMarker != tc.wantOutcome {
				t.Errorf("adobeMarker = %q, want %q", img.AdobeMarker, tc.wantOutcome)
			}
			if tc.transformNull {
				if !isJSONNull(rawJSON, "adobeTransform") {
					t.Errorf("expected an explicit null transform, got %s", string(rawJSON["adobeTransform"]))
				}
				return
			}
			if img.AdobeTransform == nil {
				t.Fatalf("adobeTransform is null, want %d", tc.wantTransform)
			}
			if *img.AdobeTransform != tc.wantTransform {
				t.Errorf("adobeTransform = %d, want %d", *img.AdobeTransform, tc.wantTransform)
			}
		})
	}
}

// The walk survives the shapes a naive marker loop reads wrong: padding runs
// between segments, standalone markers that carry no length, a declared length
// that runs past the end, a length below the two bytes it occupies, and a chain
// that never reaches SOS.
func TestAdobeMarkerWalkOnMalformedChains(t *testing.T) {
	cases := []struct {
		name        string
		raw         []byte
		wantOutcome string
	}{
		{
			name:        "padding between segments is skipped rather than read as a marker",
			raw:         markerChain(fillBytes(5), adobeAPP14(2)),
			wantOutcome: markerPresent,
		},
		{
			name:        "standalone markers carry no length and hide nothing behind them",
			raw:         markerChain(restartMarkers(), adobeAPP14(1)),
			wantOutcome: markerPresent,
		},
		{
			name:        "a record after a foreign segment is still found",
			raw:         markerChain(foreignAPP14(), shortAPP14(), adobeAPP14(0)),
			wantOutcome: markerPresent,
		},
		{
			name:        "nothing before SOS reports absent",
			raw:         markerChain(foreignAPP14()),
			wantOutcome: markerAbsent,
		},
		{
			// The identifier is there and the record is not. Reporting absent
			// would call a chain that could not be read a chain with no marker.
			name:        "an Adobe APP14 too short to hold the record fails closed",
			raw:         markerChain(truncatedAdobeAPP14()),
			wantOutcome: markerUnparseable,
		},
		{
			name:        "a declared length running past the end fails closed",
			raw:         truncatedChain(),
			wantOutcome: markerUnparseable,
		},
		{
			name:        "a length below its own two bytes fails closed",
			raw:         undersizedLengthChain(),
			wantOutcome: markerUnparseable,
		},
		{
			name:        "a chain that never reaches SOS fails closed",
			raw:         chainWithoutSOS(),
			wantOutcome: markerUnparseable,
		},
		{
			name:        "an empty stream fails closed",
			raw:         nil,
			wantOutcome: markerUnparseable,
		},
		{
			name:        "a one-byte stream fails closed",
			raw:         []byte{0xFF},
			wantOutcome: markerUnparseable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, _ := dumpImageJSON(t, "chain.pdf", rgbImagePDF(tc.raw, ""))
			if img.AdobeMarker != tc.wantOutcome {
				t.Errorf("adobeMarker = %q, want %q", img.AdobeMarker, tc.wantOutcome)
			}
		})
	}
}

// The outcome states that are not about the chain at all: a stream that is not
// a JPEG, a JPEG the stored bytes do not contain, and a node whose image
// dictionary was never read.
func TestAdobeMarkerOutcomeOutsideTheWalk(t *testing.T) {
	t.Run("a non-DCT image is not applicable", func(t *testing.T) {
		img, _ := dumpImageJSON(t, "flate.pdf", flateImagePDF(t, ""))
		if img.AdobeMarker != markerNotApplicable {
			t.Errorf("adobeMarker = %q, want %q for a FlateDecode image", img.AdobeMarker, markerNotApplicable)
		}
	})

	t.Run("DCTDecode behind another filter is not examined, never absent", func(t *testing.T) {
		// The stored bytes are ASCII85 text; the JPEG is what comes out of the
		// first stage. Walking the stored bytes anyway and reporting no marker
		// would be a confident wrong answer on exactly the kind of file that
		// gets inspected.
		dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
			" /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter [/ASCII85Decode /DCTDecode]"
		img, _ := dumpImageJSON(t, "prefiltered.pdf", imagePDF(dict, []byte("Gb\"/&~>")))
		if img.AdobeMarker == markerAbsent {
			t.Fatalf("adobeMarker = %q: the stored bytes are not the JPEG, so a missing marker is not an answer", img.AdobeMarker)
		}
		if img.AdobeMarker != markerNotExamined {
			t.Errorf("adobeMarker = %q, want %q", img.AdobeMarker, markerNotExamined)
		}
	})

	t.Run("a node that is not an image XObject leaves the outcome empty", func(t *testing.T) {
		// The empty zero value is the discriminator for "the dictionary was
		// never read", which is what the plain-text writer branches on.
		path := writeTempPDF(t, "catalog.pdf", rgbImagePDF(rgbJPEG(t), ""))
		stdout, stderr, ec := runCLI(t, "dump", "image", "--ref", "1 0 R", "--json", path)
		if ec != 0 {
			t.Fatalf("expected exit 0, got %d\nstderr: %s", ec, string(stderr))
		}
		var img imageJSON
		if err := unmarshalImage(stdout, &img); err != nil {
			t.Fatalf("parse image JSON: %v\nraw: %s", err, string(stdout))
		}
		if img.Error == "" {
			t.Fatalf("expected an error for a node that is not an image XObject")
		}
		if img.AdobeMarker != "" {
			t.Errorf("adobeMarker = %q, want the empty zero value", img.AdobeMarker)
		}
	})
}

// ---------------------------------------------------------------------------
// The verdict at four components, where both switches are live. None of these
// streams decodes - image/jpeg refuses a four-component JPEG without an Adobe
// record, and a hand-rolled four-component entropy stream is not worth the
// cost - so each case also pins that the fields are computed above the decode.
// ---------------------------------------------------------------------------

func TestVerdictAtFourComponents(t *testing.T) {
	cases := []struct {
		name    string
		raw     []byte
		entries string
		want    string
	}{
		{
			name: "no marker and no array is the default reading",
			raw:  markerChain(),
			want: verdictNormalDefault,
		},
		{
			name: "a marker with no array reads the stored inversion with nothing to compensate",
			raw:  markerChain(adobeAPP14(2)),
			want: verdictInvertedAdobe,
		},
		{
			name:    "an inverting array beside a marker compensates for the stored inversion",
			raw:     markerChain(adobeAPP14(2)),
			entries: "/Decode [1 0 1 0 1 0 1 0]",
			want:    verdictNormalAdobe,
		},
		{
			name:    "an inverting array with no marker is the negative",
			raw:     markerChain(),
			entries: "/Decode [1 0 1 0 1 0 1 0]",
			want:    verdictInvertedNoMark,
		},
		{
			name:    "an array inverting only some components is non-default, never normal",
			raw:     markerChain(),
			entries: "/Decode [1 0 0 1 0 1 0 1]",
			want:    verdictNonDefault,
		},
		{
			name:    "an unreadable chain outranks an inverting array",
			raw:     truncatedChain(),
			entries: "/Decode [1 0 1 0 1 0 1 0]",
			want:    verdictUnknownChain,
		},
		{
			name:    "unreachable JPEG bytes outrank an inverting array",
			entries: "/Decode [1 0 1 0 1 0 1 0]",
			want:    verdictUnknownReach,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var pdf []byte
			if tc.raw == nil {
				dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
					" /ColorSpace /DeviceCMYK /BitsPerComponent 8" +
					" /Filter [/ASCII85Decode /DCTDecode] " + tc.entries
				pdf = imagePDF(dict, []byte("Gb\"/&~>"))
			} else {
				pdf = cmykImagePDF(tc.raw, tc.entries)
			}
			img, _ := dumpImageJSON(t, "cmyk.pdf", pdf)
			if img.SampleInterpretation != tc.want {
				t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, tc.want)
			}
		})
	}
}

// A four-component image that is not a JPEG has no marker chain to join, so the
// verdict comes from /Decode alone and must not name a marker: a reader told
// there is no Adobe APP14 goes looking for a JPEG that does not exist.
func TestFourComponentImageWithNoMarkerChain(t *testing.T) {
	cases := []struct {
		name    string
		entries string
		want    string
	}{
		{name: "an inverting array", entries: "/Decode [1 0 1 0 1 0 1 0]", want: verdictInvertedDecode},
		{name: "the default array", entries: "/Decode [0 1 0 1 0 1 0 1]", want: verdictNormalDefault},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
				" /ColorSpace /DeviceCMYK /BitsPerComponent 8 /Filter /FlateDecode " + tc.entries
			img, _ := dumpImageJSON(t, "flate-cmyk.pdf", imagePDF(dict, zlibBytes(t, make([]byte, 256))))
			if img.AdobeMarker != markerNotApplicable {
				t.Fatalf("adobeMarker = %q, want %q", img.AdobeMarker, markerNotApplicable)
			}
			if img.SampleInterpretation != tc.want {
				t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, tc.want)
			}
		})
	}
}

// An array carrying a different number of pairs than the image has components
// is neither the default nor a full inversion: both are defined per component.
// The bound on the read is an upper one, so a short array reaches the verdict.
func TestDecodeArrayWithTooFewPairsIsNotClassified(t *testing.T) {
	dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /ColorSpace /DeviceCMYK /BitsPerComponent 8 /Filter /FlateDecode /Decode [1 0]"
	img, _ := dumpImageJSON(t, "short-decode.pdf", imagePDF(dict, zlibBytes(t, make([]byte, 256))))
	if !sameFloats(img.Decode, []float64{1, 0}) {
		t.Fatalf("decode = %v, want the array as written", img.Decode)
	}
	if img.SampleInterpretation != verdictNonDefault {
		t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, verdictNonDefault)
	}
}

// A rejected array stores nothing, and the nil it leaves is the nil an absent
// key leaves. The verdict must not read it as the default: the file does set
// /Decode, and what it sets is exactly what could not be read.
func TestRejectedDecodeArrayIsNotReportedAsTheDefault(t *testing.T) {
	jpegBytes := rgbJPEG(t)

	cases := []struct {
		name    string
		entries string
	}{
		{name: "an array longer than two entries per component", entries: "/Decode [0 1 0 1 0 1 0 1 0 1]"},
		{name: "an odd-length array", entries: "/Decode [0 1 0]"},
		{name: "an array carrying a non-numeric element", entries: "/Decode [0 1 0 null 0 1]"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, raw := dumpImageJSON(t, "rejected-decode.pdf", rgbImagePDF(jpegBytes, tc.entries))
			if !isJSONNull(raw, "decode") {
				t.Fatalf("expected an explicit null decode, got %s", string(raw["decode"]))
			}
			if img.SampleInterpretation != verdictUnknownDecode {
				t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, verdictUnknownDecode)
			}
			if !strings.Contains(strings.ToLower(img.Warning), "decode") {
				t.Errorf("expected a warning naming the rejected array, got %q", img.Warning)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Below four components the marker never inverts anything: Go's decoder applies
// it in the four-component path only, and on three components the transform
// selects a colour transform instead. Photoshop writes an Adobe record into
// ordinary RGB JPEGs, so a rule that fires below four components mislabels a
// very large share of real images.
// ---------------------------------------------------------------------------

func TestMarkerDoesNotDecideTheVerdictBelowFourComponents(t *testing.T) {
	jpegBytes := rgbJPEG(t)
	withMarker := spliceAfterAPP0(t, jpegBytes, adobeAPP14(2))

	cases := []struct {
		name    string
		raw     []byte
		entries string
		want    string
	}{
		{
			name: "three components with a marker and no array read normally",
			raw:  withMarker,
			want: verdictNormalDefault,
		},
		{
			name:    "three components with a marker and the default array read normally",
			raw:     withMarker,
			entries: "/Decode [0 1 0 1 0 1]",
			want:    verdictNormalDefault,
		},
		{
			name:    "three components with a marker and an inverting array are inverted",
			raw:     withMarker,
			entries: "/Decode [1 0 1 0 1 0]",
			want:    verdictInvertedDecode,
		},
		{
			name:    "three components with no marker and an inverting array are inverted",
			raw:     jpegBytes,
			entries: "/Decode [1 0 1 0 1 0]",
			want:    verdictInvertedDecode,
		},
		{
			name:    "an unreadable chain below four components does not reach the verdict",
			raw:     truncatedChain(),
			entries: "/Decode [1 0 1 0 1 0]",
			want:    verdictInvertedDecode,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, _ := dumpImageJSON(t, "rgb-marker.pdf", rgbImagePDF(tc.raw, tc.entries))
			if img.SampleInterpretation != tc.want {
				t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, tc.want)
			}
		})
	}
}

// The transform byte is evidence the reader sees, never an input to the
// verdict: every valid Adobe record inverts a four-component stream, and the
// number only chooses between YCCK-to-CMYK and a direct interleave.
func TestTransformValueDoesNotBranchTheVerdict(t *testing.T) {
	for _, transform := range []byte{0, 1, 2} {
		img, _ := dumpImageJSON(t, "transform.pdf",
			cmykImagePDF(markerChain(adobeAPP14(transform)), ""))
		if img.SampleInterpretation != verdictInvertedAdobe {
			t.Errorf("transform %d: sampleInterpretation = %q, want %q",
				transform, img.SampleInterpretation, verdictInvertedAdobe)
		}
		if img.AdobeTransform == nil || *img.AdobeTransform != int(transform) {
			t.Errorf("transform %d: adobeTransform = %v, want it reported as evidence",
				transform, img.AdobeTransform)
		}
	}
}

// An unresolved component count is not a four-component stream, so the marker
// arm does not fire on it. Widening the guess here would invent an inversion
// rather than a ceiling. It is not the plain default either: whether the marker
// applies is exactly what the missing count decides, and the default would read
// as a confident "checked, nothing here" on a stream that may be a stored-
// inverted Adobe CMYK JPEG.
func TestUnresolvedComponentCountDoesNotTakeTheMarkerArm(t *testing.T) {
	// No /ColorSpace at all and no ImageMask: the component count cannot be
	// resolved from the dictionary.
	dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /BitsPerComponent 8 /Filter /DCTDecode"
	img, _ := dumpImageJSON(t, "unresolved.pdf", imagePDF(dict, markerChain(adobeAPP14(2))))
	if img.SampleInterpretation != verdictUnknownMarker {
		t.Errorf("sampleInterpretation = %q, want %q: an unresolved component count neither fires the marker arm nor reads as the default",
			img.SampleInterpretation, verdictUnknownMarker)
	}
	if img.AdobeTransform == nil || *img.AdobeTransform != 2 {
		t.Errorf("adobeTransform = %v, want the transform reported as evidence beside the verdict", img.AdobeTransform)
	}

	// A chain walked to SOS with no record reads the same at any component
	// count, so the unresolved count leaves nothing undecided.
	noRecord, _ := dumpImageJSON(t, "unresolved-no-record.pdf", imagePDF(dict, markerChain(foreignAPP14())))
	if noRecord.SampleInterpretation != verdictNormalDefault {
		t.Errorf("sampleInterpretation = %q, want %q: with no record the count cannot change the answer",
			noRecord.SampleInterpretation, verdictNormalDefault)
	}
}

// Indexed and Lab images are not classified. An Indexed default /Decode is
// [0 2^bpc - 1] and a Lab one is its /Range, so the [0 1] identity test would
// call a perfectly ordinary array an inversion. The array is still shown.
func TestIndexedAndLabAreNotClassified(t *testing.T) {
	cases := []struct {
		name    string
		entries string
	}{
		{
			name: "an Indexed image",
			entries: "/ColorSpace [/Indexed /DeviceRGB 1 <FFFFFF000000>]" +
				" /BitsPerComponent 8 /Decode [0 1]",
		},
		{
			name: "a Lab image",
			entries: "/ColorSpace [/Lab << /WhitePoint [0.9505 1 1.089] /Range [-100 100 -100 100] >>]" +
				" /BitsPerComponent 8 /Decode [0 100 -100 100 -100 100]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8 /Filter /FlateDecode " + tc.entries
			// Every sample is index zero, so the palette lookup stays inside a
			// two-entry table and the fixture is about the colour space rather
			// than about its pixels.
			img, raw := dumpImageJSON(t, "unclassified.pdf", imagePDF(dict, zlibBytes(t, make([]byte, 64))))
			if img.SampleInterpretation != verdictNotClassified {
				t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, verdictNotClassified)
			}
			if isJSONNull(raw, "decode") {
				t.Errorf("the array is still shown for an unclassified colour space, got a null decode")
			}
		})
	}
}

// The carve-out speaks about an array that was read. An array the reader
// rejected is unreadable whatever the colour space, and "not a simple
// inversion" would assert a well-formed array that nobody ever saw - on a
// payload whose decode is null, leaving the warning as the only trace.
func TestARejectedArrayOnAnUnclassifiedColorSpaceReadsAsUnreadable(t *testing.T) {
	cases := []struct {
		name    string
		entries string
	}{
		{
			name: "an Indexed image",
			entries: "/ColorSpace [/Indexed /DeviceRGB 1 <FFFFFF000000>]" +
				" /BitsPerComponent 8 /Decode [0 1 2]",
		},
		{
			name: "a Lab image",
			entries: "/ColorSpace [/Lab << /WhitePoint [0.9505 1 1.089] /Range [-100 100 -100 100] >>]" +
				" /BitsPerComponent 8 /Decode [0 100 -100]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8 /Filter /FlateDecode " + tc.entries
			img, raw := dumpImageJSON(t, "rejected-unclassified.pdf", imagePDF(dict, zlibBytes(t, make([]byte, 64))))
			if !isJSONNull(raw, "decode") {
				t.Fatalf("expected the rejected array to be stored as nothing, got %s", string(raw["decode"]))
			}
			if img.SampleInterpretation != verdictUnknownDecode {
				t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, verdictUnknownDecode)
			}
		})
	}
}

// The carve-out is about an array that is there. With no /Decode key at all
// there is nothing to misclassify, and a verdict naming an array the file does
// not set sends a reader looking for one.
func TestIndexedAndLabWithNoArrayReadAsTheDefault(t *testing.T) {
	cases := []struct {
		name    string
		entries string
	}{
		{
			name:    "an Indexed image",
			entries: "/ColorSpace [/Indexed /DeviceRGB 1 <FFFFFF000000>] /BitsPerComponent 8",
		},
		{
			name: "a Lab image",
			entries: "/ColorSpace [/Lab << /WhitePoint [0.9505 1 1.089] /Range [-100 100 -100 100] >>]" +
				" /BitsPerComponent 8",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8 /Filter /FlateDecode " + tc.entries
			img, raw := dumpImageJSON(t, "no-array.pdf", imagePDF(dict, zlibBytes(t, make([]byte, 64))))
			if !isJSONNull(raw, "decode") {
				t.Fatalf("expected an absent decode key, got %s", string(raw["decode"]))
			}
			if img.SampleInterpretation != verdictNormalDefault {
				t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, verdictNormalDefault)
			}
		})
	}
}

// An unresolved colour-component count cannot be measured against, so an array
// of any length is neither the default nor a full inversion. The read bound
// widens rather than rejecting, so a two-entry array on a four-colorant
// /DeviceN is stored and reaches the verdict.
func TestDecodeArrayWithAnUnresolvedComponentCountIsNotClassified(t *testing.T) {
	maskObj := []byte("7 0 obj\n<< /Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 0 >>\nstream\n\nendstream\nendobj\n")
	img, _ := dumpImageJSON(t, "unresolved-arity.pdf",
		deviceNImagePDF(markerChain(), "/Decode [1 0]", maskObj))

	if !sameFloats(img.Decode, []float64{1, 0}) {
		t.Fatalf("decode = %v, want the array as written", img.Decode)
	}
	if img.SampleInterpretation != verdictUnknownArity {
		t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, verdictUnknownArity)
	}
}

// A stencil mask is one component whatever else the dictionary says, and its
// identity array is [0 1].
func TestStencilMaskVerdict(t *testing.T) {
	cases := []struct {
		name    string
		entries string
		want    string
	}{
		{name: "no array reads normally", entries: "", want: verdictNormalDefault},
		{name: "an inverting array is inverted", entries: "/Decode [1 0]", want: verdictInvertedDecode},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, _ := dumpImageJSON(t, "mask-verdict.pdf", stencilMaskPDF(tc.entries))
			if img.SampleInterpretation != tc.want {
				t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Everything reported here is computed before the decode is attempted. A
// four-component DCT stream with no Adobe record is refused by the JPEG decoder
// outright, which is the exact shape this reporting exists for: computed after
// the decode, every field would be null on the one file that most needs them.
// ---------------------------------------------------------------------------

func TestFullFieldSetSurvivesAFailedDecode(t *testing.T) {
	entries := "/Decode [1 0 1 0 1 0 1 0] /SMask 5 0 R"
	maskObj := []byte("5 0 obj\n<< /Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 0 >>\nstream\n\nendstream\nendobj\n")
	img, raw := dumpImageJSON(t, "failed-decode.pdf",
		cmykImagePDF(markerChain(), entries, maskObj))

	if img.Error == "" {
		t.Fatalf("expected a decode failure for a four-component stream with no Adobe record")
	}
	if img.AdobeMarker != markerAbsent {
		t.Errorf("adobeMarker = %q, want %q", img.AdobeMarker, markerAbsent)
	}
	if !sameFloats(img.Decode, []float64{1, 0, 1, 0, 1, 0, 1, 0}) {
		t.Errorf("decode = %v, want the array read before the decode was attempted", img.Decode)
	}
	if img.SMask == nil || *img.SMask != "5 0 R" {
		t.Errorf("smask = %v, want the reference read before the decode was attempted", img.SMask)
	}
	if img.SampleInterpretation != verdictInvertedNoMark {
		t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, verdictInvertedNoMark)
	}
	for _, key := range sampleInterpretationKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("key %q is missing from a payload whose decode failed", key)
		}
	}
}

// The other read that ends with an error before the decode is a colour space
// whose component lookup faults. The same fields have to survive it. The count
// stays unresolved there, so the /Decode bound widens to the maximum and the
// arity of the array cannot be checked: the array is still reported, and the
// verdict says the count is what is missing rather than asserting an inversion
// it cannot establish. At a resolved four components beside this record the same
// array reads as the compensated normal.

func TestFullFieldSetSurvivesAnUnreadableColorSpace(t *testing.T) {
	maskObj := []byte("7 0 obj\n<< /Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 0 >>\nstream\n\nendstream\nendobj\n")
	img, raw := dumpImageJSON(t, "unreadable-colorspace.pdf",
		deviceNImagePDF(markerChain(adobeAPP14(2)), "/Decode [1 0 1 0 1 0 1 0] /SMask 7 0 R", maskObj))

	if img.Error == "" {
		t.Fatalf("expected a per-image error for a colour space the lookup cannot read")
	}
	if img.AdobeMarker != markerPresent {
		t.Errorf("adobeMarker = %q, want %q", img.AdobeMarker, markerPresent)
	}
	if img.AdobeTransform == nil || *img.AdobeTransform != 2 {
		t.Errorf("adobeTransform = %v, want 2", img.AdobeTransform)
	}
	if !sameFloats(img.Decode, []float64{1, 0, 1, 0, 1, 0, 1, 0}) {
		t.Errorf("decode = %v, want the array read before the lookup faulted", img.Decode)
	}
	if img.SMask == nil || *img.SMask != "7 0 R" {
		t.Errorf("smask = %v, want the reference read before the lookup faulted", img.SMask)
	}
	if img.SampleInterpretation != verdictUnknownArity {
		t.Errorf("sampleInterpretation = %q, want %q", img.SampleInterpretation, verdictUnknownArity)
	}
	for _, key := range sampleInterpretationKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("key %q is missing from a payload whose colour space could not be read", key)
		}
	}
	// The stored length is the length of bytes already in hand and needs no
	// component count. The decoded estimate does need one, and answers 0 rather
	// than a guess while it is unresolved.
	if img.StoredBytes <= 0 {
		t.Errorf("storedBytes = %d, want the encoded stream length, which the colour space does not decide", img.StoredBytes)
	}
	if img.DecodedBytes != 0 {
		t.Errorf("decodedBytes = %d, want 0: the estimate has no component count to stand on", img.DecodedBytes)
	}
}

// ---------------------------------------------------------------------------
// JSON is a contract: every key is emitted unconditionally with an explicit
// null, on the full payload and on the metadata projection alike. The
// human-readable asymmetry with this is deliberate.
// ---------------------------------------------------------------------------

func TestJSONEmitsEveryKeyWithExplicitNulls(t *testing.T) {
	pdf := rgbImagePDF(rgbJPEG(t), "")

	for _, args := range [][]string{nil, {"--metadata"}} {
		label := "full payload"
		if len(args) > 0 {
			label = "metadata projection"
		}
		t.Run(label, func(t *testing.T) {
			_, raw := dumpImageJSON(t, "nulls.pdf", pdf, args...)
			for _, key := range sampleInterpretationKeys {
				if _, ok := raw[key]; !ok {
					t.Errorf("key %q is missing; an absent value emits null, it does not vanish", key)
				}
			}
			for _, key := range []string{"decode", "smask", "adobeTransform"} {
				if !isJSONNull(raw, key) {
					t.Errorf("key %q = %s, want an explicit null", key, string(raw[key]))
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The plain-text writer. The verdict is unconditional - "checked, nothing
// here" is a real answer and a reader has to be able to tell it apart from
// "the tool did not look". The structural fields are conditional, because
// nobody opens this view asking about them and a permanent "none" row trains
// readers to skim.
// ---------------------------------------------------------------------------

func TestPlainTextVerdictIsUnconditional(t *testing.T) {
	jpegBytes := rgbJPEG(t)

	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "an ordinary image with nothing set", raw: jpegBytes, want: verdictNormalDefault},
		{name: "an image carrying a marker", raw: spliceAfterAPP0(t, jpegBytes, adobeAPP14(2)), want: verdictNormalDefault},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := dumpImagePlain(t, "plain.pdf", rgbImagePDF(tc.raw, ""), "4 0 R")
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected the verdict %q in the plain-text block, got:\n%s", tc.want, out)
			}
		})
	}
}

func TestPlainTextConditionalRows(t *testing.T) {
	jpegBytes := rgbJPEG(t)
	maskObj := []byte("5 0 obj\n<< /Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 0 >>\nstream\n\nendstream\nendobj\n")

	// The row labels are matched with their colon, because the writer aligns the
	// colon column and because "Decode" on its own also matches the DCTDecode
	// filter name.
	t.Run("a bare image shows none of the conditional rows", func(t *testing.T) {
		out := dumpImagePlain(t, "bare.pdf", rgbImagePDF(jpegBytes, ""), "4 0 R")
		for _, label := range []string{"SMask:", "ImageMask:", "Decode:", "AdobeTransform:"} {
			if strings.Contains(out, label) {
				t.Errorf("row %q appears for an image that carries none of it:\n%s", label, out)
			}
		}
	})

	t.Run("the decode array is shown when the key is present", func(t *testing.T) {
		out := dumpImagePlain(t, "decode-row.pdf",
			rgbImagePDF(jpegBytes, "/Decode [1 0 1 0 1 0]"), "4 0 R")
		if !strings.Contains(out, "Decode:") {
			t.Errorf("expected a Decode row:\n%s", out)
		}
		if !strings.Contains(out, "1 0 1 0 1 0") {
			t.Errorf("expected the literal array beside the verdict:\n%s", out)
		}
	})

	t.Run("the transform is shown with its meaning, never as a bare number", func(t *testing.T) {
		out := dumpImagePlain(t, "transform-row.pdf",
			rgbImagePDF(spliceAfterAPP0(t, jpegBytes, adobeAPP14(2)), ""), "4 0 R")
		if !strings.Contains(out, "AdobeTransform:") {
			t.Errorf("expected an AdobeTransform row:\n%s", out)
		}
		if !strings.Contains(out, "YCCK (transform 2)") {
			t.Errorf("expected the transform's decoded meaning with its number:\n%s", out)
		}
	})

	t.Run("the smask and mask rows are shown when they carry something", func(t *testing.T) {
		out := dumpImagePlain(t, "smask-row.pdf",
			rgbImagePDF(jpegBytes, "/SMask 5 0 R", maskObj), "4 0 R")
		if !strings.Contains(out, "SMask:") || !strings.Contains(out, "5 0 R") {
			t.Errorf("expected an SMask row carrying the reference:\n%s", out)
		}

		out = dumpImagePlain(t, "mask-row.pdf", stencilMaskPDF(""), "4 0 R")
		if !strings.Contains(out, "ImageMask:") {
			t.Errorf("expected an ImageMask row for a stencil mask:\n%s", out)
		}
	})
}

// The marker outcome is a row of its own on every DCT image. Only the transform
// was printed before, and only where a record was found, so an unreadable chain
// on an RGB JPEG - which never reaches the verdict - left no trace outside
// --json.
func TestPlainTextPrintsTheMarkerOutcomeForEveryDCTImage(t *testing.T) {
	jpegBytes := rgbJPEG(t)

	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "a chain walked to SOS with no record", raw: jpegBytes, want: markerAbsent},
		{name: "a record found", raw: spliceAfterAPP0(t, jpegBytes, adobeAPP14(2)), want: markerPresent},
		{name: "a chain that could not be read", raw: truncatedChain(), want: markerUnparseable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := dumpImagePlain(t, "marker-row.pdf", rgbImagePDF(tc.raw, ""), "4 0 R")
			if !strings.Contains(out, "AdobeMarker:") {
				t.Fatalf("expected an AdobeMarker row:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected the outcome %q in the block:\n%s", tc.want, out)
			}
		})
	}

	t.Run("a stream that is not a JPEG has no outcome to report", func(t *testing.T) {
		dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
			" /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode"
		out := dumpImagePlain(t, "flate-marker.pdf", imagePDF(dict, zlibBytes(t, make([]byte, 192))), "4 0 R")
		if strings.Contains(out, "AdobeMarker:") {
			t.Errorf("an AdobeMarker row appears for a stream with no marker chain:\n%s", out)
		}
	})
}

// The stored and decoded sizes the payload already carries reach the plain-text
// writer, which drops them today while the panel shows both.
func TestPlainTextPrintsStoredAndDecodedSizes(t *testing.T) {
	out := dumpImagePlain(t, "sizes.pdf", rgbImagePDF(rgbJPEG(t), ""), "4 0 R")
	if !strings.Contains(out, "Size") {
		t.Fatalf("expected a Size row:\n%s", out)
	}
	if !strings.Contains(out, "B") {
		t.Errorf("expected a humanized byte count in the Size row:\n%s", out)
	}
	// 8x8 RGB at 8 bits is 192 bytes decoded; both halves are non-zero, so both
	// appear, stored first.
	if !strings.Contains(out, "192") {
		t.Errorf("expected the decoded estimate beside the stored size:\n%s", out)
	}
}

// The writer stops after the error only when there is nothing else to say. A
// stream whose decode failed still has a dictionary, and hiding its block
// behind the very error that shape produces hides the case this reporting
// exists for.
func TestPlainTextErrorRowDoesNotSuppressTheBlock(t *testing.T) {
	t.Run("a node that is not an image prints the reference and the error alone", func(t *testing.T) {
		out := dumpImagePlain(t, "catalog-plain.pdf", rgbImagePDF(rgbJPEG(t), ""), "1 0 R")
		if !strings.Contains(out, "Error") {
			t.Fatalf("expected an Error row:\n%s", out)
		}
		lines := nonEmptyLines(out)
		if len(lines) != 2 {
			t.Errorf("expected exactly the object and error rows, got %d lines:\n%s", len(lines), out)
		}
	})

	t.Run("a stream that failed to decode prints the error and the block", func(t *testing.T) {
		pdf := cmykImagePDF(markerChain(), "/Decode [1 0 1 0 1 0 1 0]")
		out := dumpImagePlain(t, "failed-plain.pdf", pdf, "4 0 R")
		if !strings.Contains(out, "Error") {
			t.Fatalf("expected an Error row:\n%s", out)
		}
		if !strings.Contains(out, verdictInvertedNoMark) {
			t.Errorf("expected the verdict after the error row:\n%s", out)
		}
		if !strings.Contains(out, "1 0 1 0 1 0 1 0") {
			t.Errorf("expected the decode array after the error row:\n%s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// The two pure functions are unit-tested in package, where a byte slice and a
// table row are the whole input. This suite is a separate module and cannot
// import them, so it asserts the in-package tests exist and pass: the segment
// and byte ceilings, in particular, are cheaper to drive on bytes than on a
// fixture PDF.
// ---------------------------------------------------------------------------

func TestMarkerWalkUnitCoverageExists(t *testing.T) {
	runGoTest(t, "TestScanAdobeMarker", "./internal/pdfcore/...")
}

func TestVerdictUnitCoverageExists(t *testing.T) {
	runGoTest(t, "TestSampleInterpretation", "./internal/pdfcore/...")
}

// The dictionary shapes the loader rejects at the document level - an /SMask
// that is a direct dictionary or the borrowed /None name, and a /Decode element
// that is neither a number nor null - reach the readers only from a dictionary
// built in code, so they are covered in package too.
func TestImageDictionaryEntryUnitCoverageExists(t *testing.T) {
	runGoTest(t, "TestImageDictionaryEntries", "./internal/pdfcore/...")
}

// nonEmptyLines splits output into its non-blank lines.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The /Decode length bound is two entries per colour component, and the
// component count it derives from is not always the one the dictionary states.
// A stencil mask is one component whatever else it carries, and a colour space
// that could not be resolved widens the bound rather than rejecting the array.
// ---------------------------------------------------------------------------

// A stencil mask carries one 1-bit sample per pixel, so its bound is two
// entries. A four-entry array is over it and is rejected rather than stored, on
// an image whose dictionary states no colour space to derive a wider bound
// from.
func TestStencilMaskBoundsTheDecodeArrayAtOneComponent(t *testing.T) {
	img, raw := dumpImageJSON(t, "mask-decode-bound.pdf", stencilMaskPDF("/Decode [0 1 0 1]"))

	if !isJSONNull(raw, "decode") {
		t.Fatalf("expected an explicit null decode for an array over the mask's bound, got %s",
			string(raw["decode"]))
	}
	if !strings.Contains(strings.ToLower(img.Warning), "decode") {
		t.Errorf("expected a warning naming the rejected array, got %q", img.Warning)
	}
}

// An unresolved colour space widens the bound to two entries per maximum
// component rather than rejecting every array, matching how the decode ceiling
// treats the same unknown. The widened bound is still a bound.
func TestUnresolvedComponentCountWidensTheDecodeBound(t *testing.T) {
	// No /ColorSpace and no /ImageMask: the component count cannot be resolved.
	dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /BitsPerComponent 8 /Filter /DCTDecode /Decode "

	pairs := func(n int) string {
		out := "["
		for range n {
			out += "0 1 "
		}
		return out + "]"
	}

	t.Run("an array at the widened bound is kept", func(t *testing.T) {
		img, _ := dumpImageJSON(t, "wide-bound.pdf",
			imagePDF(dict+pairs(32), markerChain()))
		if len(img.Decode) != 64 {
			t.Fatalf("decode = %v, want the 64-entry array kept", img.Decode)
		}
		if strings.Contains(strings.ToLower(img.Warning), "decode") {
			t.Errorf("an array inside the bound earns no complaint, got %q", img.Warning)
		}
	})

	t.Run("an array over the widened bound is rejected", func(t *testing.T) {
		img, raw := dumpImageJSON(t, "over-wide-bound.pdf",
			imagePDF(dict+pairs(33), markerChain()))
		if !isJSONNull(raw, "decode") {
			t.Fatalf("expected an explicit null decode, got %s", string(raw["decode"]))
		}
		if !strings.Contains(strings.ToLower(img.Warning), "decode") {
			t.Errorf("expected a warning naming the rejected array, got %q", img.Warning)
		}
	})
}
