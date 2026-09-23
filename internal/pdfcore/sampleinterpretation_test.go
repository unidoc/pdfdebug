// Co-located unit tests for the sample-interpretation verdict:
//
//	sampleInterpretationVerdict(colorSpace string, imageMask bool, components int,
//		decode []float64, decodeRejected bool, markerOutcome string) string
//
// A table over synthetic inputs, because the combination table is what is being
// pinned and several of its rows have no decodable fixture: Go cannot encode a
// four-component JPEG, and it refuses one that carries no Adobe record, so the
// row that produces the visible negative is decided here and nowhere else.
package pdfcore

import "testing"

var (
	cmykDefault  = []float64{0, 1, 0, 1, 0, 1, 0, 1}
	cmykInverted = []float64{1, 0, 1, 0, 1, 0, 1, 0}
	cmykPartial  = []float64{1, 0, 0, 1, 0, 1, 0, 1}
	rgbDefault   = []float64{0, 1, 0, 1, 0, 1}
	rgbInverted  = []float64{1, 0, 1, 0, 1, 0}
	rgbPartial   = []float64{1, 0, 0, 1, 0, 1}
)

func TestSampleInterpretation(t *testing.T) {
	cases := []struct {
		name           string
		colorSpace     string
		imageMask      bool
		components     int
		decode         []float64
		decodeRejected bool
		marker         string
		want           string
	}{
		// The combination table at four components, where both switches are live.
		{
			name:       "no marker and no array",
			colorSpace: "DeviceCMYK",
			components: 4,
			marker:     AdobeMarkerAbsent,
			want:       verdictNormalDefault,
		},
		{
			name:       "no marker and the default array",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykDefault,
			marker:     AdobeMarkerAbsent,
			want:       verdictNormalDefault,
		},
		{
			name:       "a marker with no array reads the stored inversion with nothing to compensate",
			colorSpace: "DeviceCMYK",
			components: 4,
			marker:     AdobeMarkerPresent,
			want:       verdictInvertedAdobe,
		},
		{
			name:       "a marker with the default array reads the stored inversion too",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykDefault,
			marker:     AdobeMarkerPresent,
			want:       verdictInvertedAdobe,
		},
		{
			name:       "an inverting array beside a marker compensates for the stored inversion",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykInverted,
			marker:     AdobeMarkerPresent,
			want:       verdictNormalAdobe,
		},
		{
			name:       "an inverting array with no marker is the negative",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykInverted,
			marker:     AdobeMarkerAbsent,
			want:       verdictInvertedNoMark,
		},
		{
			name:       "a partial inversion is non-default, never normal",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykPartial,
			marker:     AdobeMarkerAbsent,
			want:       verdictNonDefault,
		},
		{
			name:       "a partial inversion beside a marker is still non-default",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykPartial,
			marker:     AdobeMarkerPresent,
			want:       verdictNonDefault,
		},
		{
			// A stream that is not a JPEG has no marker chain, so the verdict
			// must not name one: there is nothing for a reader to go and check.
			name:       "a four-component stream that is not DCT reads from the array alone",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykInverted,
			marker:     AdobeMarkerNotApplicable,
			want:       verdictInvertedDecode,
		},
		{
			name:       "a four-component stream that is not DCT with the default array",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykDefault,
			marker:     AdobeMarkerNotApplicable,
			want:       verdictNormalDefault,
		},

		// The default and the full inversion are both defined per component, so
		// an array with the wrong number of pairs is neither.
		{
			name:       "four components with a single inverting pair is not a full inversion",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     []float64{1, 0},
			marker:     AdobeMarkerPresent,
			want:       verdictNonDefault,
		},
		{
			name:       "four components with a single identity pair is not the default array",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     []float64{0, 1},
			marker:     AdobeMarkerAbsent,
			want:       verdictNonDefault,
		},
		{
			name:       "three components carrying four pairs is neither",
			colorSpace: "DeviceRGB",
			components: 3,
			decode:     cmykInverted,
			marker:     AdobeMarkerAbsent,
			want:       verdictNonDefault,
		},
		{
			name:       "an odd-length array leaves a pair dangling and is neither",
			colorSpace: "DeviceRGB",
			components: 3,
			decode:     []float64{0, 1, 0},
			marker:     AdobeMarkerNotApplicable,
			want:       verdictNonDefault,
		},

		// A rejected array is stored as nothing, which is the same nil an absent
		// key leaves. It must not read as the default: the file does set the
		// array, and what it sets is what could not be read.
		{
			name:           "a rejected array does not read as the default",
			colorSpace:     "DeviceRGB",
			components:     3,
			decodeRejected: true,
			marker:         AdobeMarkerAbsent,
			want:           verdictUnknownDecode,
		},
		{
			name:           "a rejected array at four components with no marker",
			colorSpace:     "DeviceCMYK",
			components:     4,
			decodeRejected: true,
			marker:         AdobeMarkerAbsent,
			want:           verdictUnknownDecode,
		},
		{
			name:           "a rejected array at four components beside a marker",
			colorSpace:     "DeviceCMYK",
			components:     4,
			decodeRejected: true,
			marker:         AdobeMarkerPresent,
			want:           verdictUnknownDecode,
		},
		{
			name:           "an unreadable chain still outranks a rejected array",
			colorSpace:     "DeviceCMYK",
			components:     4,
			decodeRejected: true,
			marker:         AdobeMarkerUnparseable,
			want:           verdictUnknownChain,
		},

		// An unreadable marker outranks any verdict that would depend on it:
		// whether a compensating inversion exists is precisely what is unknown.
		{
			name:       "an unreadable chain outranks an inverting array",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykInverted,
			marker:     AdobeMarkerUnparseable,
			want:       verdictUnknownChain,
		},
		{
			name:       "an unreadable chain outranks the default array too",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykDefault,
			marker:     AdobeMarkerUnparseable,
			want:       verdictUnknownChain,
		},
		{
			name:       "unreachable JPEG bytes outrank an inverting array",
			colorSpace: "DeviceCMYK",
			components: 4,
			decode:     cmykInverted,
			marker:     AdobeMarkerNotExamined,
			want:       verdictUnknownReach,
		},

		// Below four components the marker never inverts anything, so it never
		// reaches the verdict - including the two Unknown outcomes.
		{
			name:       "three components with a marker and no array",
			colorSpace: "DeviceRGB",
			components: 3,
			marker:     AdobeMarkerPresent,
			want:       verdictNormalDefault,
		},
		{
			name:       "three components with a marker and the default array",
			colorSpace: "DeviceRGB",
			components: 3,
			decode:     rgbDefault,
			marker:     AdobeMarkerPresent,
			want:       verdictNormalDefault,
		},
		{
			name:       "three components with a marker and an inverting array",
			colorSpace: "DeviceRGB",
			components: 3,
			decode:     rgbInverted,
			marker:     AdobeMarkerPresent,
			want:       verdictInvertedDecode,
		},
		{
			name:       "three components with a partial inversion",
			colorSpace: "DeviceRGB",
			components: 3,
			decode:     rgbPartial,
			marker:     AdobeMarkerAbsent,
			want:       verdictNonDefault,
		},
		{
			name:       "an unreadable chain below four components does not reach the verdict",
			colorSpace: "DeviceRGB",
			components: 3,
			decode:     rgbInverted,
			marker:     AdobeMarkerUnparseable,
			want:       verdictInvertedDecode,
		},
		{
			name:       "unreachable bytes below four components do not reach the verdict",
			colorSpace: "DeviceRGB",
			components: 3,
			marker:     AdobeMarkerNotExamined,
			want:       verdictNormalDefault,
		},
		{
			name:       "one component with a marker and an inverting array",
			colorSpace: "DeviceGray",
			components: 1,
			decode:     []float64{1, 0},
			marker:     AdobeMarkerPresent,
			want:       verdictInvertedDecode,
		},

		// An unresolved component count is not four components. Widening the guess
		// here would invent an inversion rather than a ceiling. It is not the
		// default either: whether the marker arm applies is exactly what is
		// missing, so the answer is an unknown of its own.
		{
			name:       "an unresolved component count does not take the marker arm",
			components: -1,
			marker:     AdobeMarkerPresent,
			want:       verdictUnknownMarker,
		},
		{
			name:       "an unresolved component count with an unreadable chain and no array",
			components: -1,
			marker:     AdobeMarkerUnparseable,
			want:       verdictUnknownMarker,
		},
		{
			name:       "an unresolved component count with unreachable JPEG bytes and no array",
			components: -1,
			marker:     AdobeMarkerNotExamined,
			want:       verdictUnknownMarker,
		},
		{
			// A chain walked to SOS with no record reads the same at any component
			// count, so nothing is unknown and the default stands.
			name:       "an unresolved component count beside a chain carrying no record",
			components: -1,
			marker:     AdobeMarkerAbsent,
			want:       verdictNormalDefault,
		},
		{
			name:       "an unresolved component count on a stream that is not DCT",
			components: -1,
			marker:     AdobeMarkerNotApplicable,
			want:       verdictNormalDefault,
		},
		{
			// The arity test needs a component count. Without one an inverting
			// array cannot be shown to invert every component, so a definite
			// inversion verdict is not warranted.
			name:       "an unresolved component count with an inverting array is not a definite inversion",
			components: -1,
			decode:     cmykInverted,
			marker:     AdobeMarkerPresent,
			want:       verdictUnknownArity,
		},
		{
			name:       "an unresolved component count with a two-entry array on an unknown colour space",
			components: -1,
			decode:     []float64{1, 0},
			marker:     AdobeMarkerNotApplicable,
			want:       verdictUnknownArity,
		},
		{
			name:       "an unresolved component count with an unreadable chain",
			components: -1,
			decode:     cmykInverted,
			marker:     AdobeMarkerUnparseable,
			want:       verdictUnknownArity,
		},

		// A stencil mask is one component whatever else the dictionary says.
		{
			name:      "a stencil mask with no array",
			imageMask: true,
			marker:    AdobeMarkerNotApplicable,
			want:      verdictNormalDefault,
		},
		{
			name:      "a stencil mask with an inverting array",
			imageMask: true,
			decode:    []float64{1, 0},
			marker:    AdobeMarkerNotApplicable,
			want:      verdictInvertedDecode,
		},
		{
			name:       "a stencil mask claiming four components is still one",
			imageMask:  true,
			components: 4,
			decode:     []float64{1, 0},
			marker:     AdobeMarkerPresent,
			want:       verdictInvertedDecode,
		},

		// Indexed and Lab are not classified: their defaults are not [0 1], so the
		// identity test would call an ordinary array an inversion.
		{
			name:       "an Indexed image",
			colorSpace: "Indexed",
			components: 1,
			decode:     []float64{0, 255},
			marker:     AdobeMarkerNotApplicable,
			want:       verdictNotClassified,
		},
		{
			name:       "an Indexed image whose array happens to be the identity",
			colorSpace: "Indexed",
			components: 1,
			decode:     []float64{0, 1},
			marker:     AdobeMarkerNotApplicable,
			want:       verdictNotClassified,
		},
		{
			name:       "a Lab image",
			colorSpace: "Lab",
			components: 3,
			decode:     []float64{0, 100, -100, 100, -100, 100},
			marker:     AdobeMarkerNotApplicable,
			want:       verdictNotClassified,
		},
		{
			// An absent array is the default whatever the colour space, so the
			// carve-out must not answer with a sentence about an array that is
			// not there.
			name:       "an Indexed image with no array reads as the default",
			colorSpace: "Indexed",
			components: 1,
			marker:     AdobeMarkerNotApplicable,
			want:       verdictNormalDefault,
		},
		{
			name:       "a Lab image with no array reads as the default",
			colorSpace: "Lab",
			components: 3,
			marker:     AdobeMarkerNotApplicable,
			want:       verdictNormalDefault,
		},
		{
			// The carve-out is about an array that is there to look at. An array
			// that could not be read at all is unreadable whatever the colour
			// space, and saying it is "not a simple inversion" asserts a
			// well-formed array nobody ever saw.
			name:           "a rejected array on an Indexed image reads as unreadable",
			colorSpace:     "Indexed",
			components:     1,
			decodeRejected: true,
			marker:         AdobeMarkerNotApplicable,
			want:           verdictUnknownDecode,
		},
		{
			name:           "a rejected array on a Lab image reads as unreadable",
			colorSpace:     "Lab",
			components:     3,
			decodeRejected: true,
			marker:         AdobeMarkerNotApplicable,
			want:           verdictUnknownDecode,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sampleInterpretationVerdict(tc.colorSpace, tc.imageMask, tc.components, tc.decode,
				tc.decodeRejected, tc.marker)
			if got != tc.want {
				t.Errorf("verdict = %q, want %q", got, tc.want)
			}
		})
	}
}
