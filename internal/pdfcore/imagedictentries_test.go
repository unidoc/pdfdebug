// Co-located unit tests for the two image-dictionary readers:
//
//	readSMaskRef(sd *pdfcpu_types.StreamDict) *string
//	readDecodeArray(xrt *pdfcpu_model.XRefTable, sd *pdfcpu_types.StreamDict, bound int) ([]float64, error)
//
// The dictionaries are built in code. Several of the shapes these readers have
// to tolerate cannot be driven through a file at all: an image /SMask that is a
// direct dictionary or the borrowed /None name, and a /Decode element that is a
// name, a string or a nested array, are all rejected by the loader before the
// reader sees them. Direct objects need no cross-reference table, so nil stands
// in for one.
package pdfcore

import (
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// imageDict wraps dictionary entries as the StreamDict the readers take.
func imageDict(entries pdfcpu_types.Dict) *pdfcpu_types.StreamDict {
	return &pdfcpu_types.StreamDict{Dict: entries}
}

func TestImageDictionaryEntries(t *testing.T) {
	t.Run("smask", func(t *testing.T) {
		cases := []struct {
			name    string
			entries pdfcpu_types.Dict
			wantNil bool
			want    string
		}{
			{
				name:    "an absent key is nil, not an empty reference",
				entries: pdfcpu_types.Dict{},
				wantNil: true,
			},
			{
				name:    "an indirect reference reports the reference",
				entries: pdfcpu_types.Dict{"SMask": *pdfcpu_types.NewIndirectRef(12, 0)},
				want:    "12 0 R",
			},
			{
				name:    "a non-zero generation is carried through",
				entries: pdfcpu_types.Dict{"SMask": *pdfcpu_types.NewIndirectRef(12, 3)},
				want:    "12 3 R",
			},
			{
				name: "a direct stream is present with no reference to report",
				entries: pdfcpu_types.Dict{"SMask": pdfcpu_types.Dict{
					"Subtype": pdfcpu_types.Name("Image"),
				}},
				want: "",
			},
			{
				// Not what the image-XObject spec defines - ISO 32000-1 types the
				// entry as a stream and /None belongs to the ExtGState soft mask -
				// but writers borrow it and those files must not fault.
				name:    "the borrowed None name is present with no reference to report",
				entries: pdfcpu_types.Dict{"SMask": pdfcpu_types.Name("None")},
				want:    "",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := readSMaskRef(imageDict(tc.entries))
				if tc.wantNil {
					if got != nil {
						t.Fatalf("smask = %q, want nil", *got)
					}
					return
				}
				if got == nil {
					t.Fatalf("smask is nil, want a pointer to %q", tc.want)
				}
				if *got != tc.want {
					t.Errorf("smask = %q, want %q", *got, tc.want)
				}
			})
		}
	})

	t.Run("decode", func(t *testing.T) {
		cases := []struct {
			name         string
			entries      pdfcpu_types.Dict
			bound        int
			want         []float64
			wantRejected bool
		}{
			{
				name:    "an absent key yields no array and no complaint",
				entries: pdfcpu_types.Dict{},
				bound:   6,
			},
			{
				name: "integers and reals both read as numbers",
				entries: pdfcpu_types.Dict{"Decode": pdfcpu_types.Array{
					pdfcpu_types.Integer(0), pdfcpu_types.Float(0.8),
				}},
				bound: 2,
				want:  []float64{0, 0.8},
			},
			{
				name: "a name element rejects the whole array",
				entries: pdfcpu_types.Dict{"Decode": pdfcpu_types.Array{
					pdfcpu_types.Integer(0), pdfcpu_types.Name("Perceptual"),
				}},
				bound:        2,
				wantRejected: true,
			},
			{
				name: "a string element rejects the whole array",
				entries: pdfcpu_types.Dict{"Decode": pdfcpu_types.Array{
					pdfcpu_types.Integer(0), pdfcpu_types.StringLiteral("1"),
				}},
				bound:        2,
				wantRejected: true,
			},
			{
				name: "a boolean element rejects the whole array",
				entries: pdfcpu_types.Dict{"Decode": pdfcpu_types.Array{
					pdfcpu_types.Integer(0), pdfcpu_types.Boolean(true),
				}},
				bound:        2,
				wantRejected: true,
			},
			{
				name: "a nested array rejects the whole array",
				entries: pdfcpu_types.Dict{"Decode": pdfcpu_types.Array{
					pdfcpu_types.Integer(0), pdfcpu_types.Array{pdfcpu_types.Integer(1)},
				}},
				bound:        2,
				wantRejected: true,
			},
			{
				name:         "a value that is not an array at all is rejected",
				entries:      pdfcpu_types.Dict{"Decode": pdfcpu_types.Name("Default")},
				bound:        6,
				wantRejected: true,
			},
			{
				name:         "an empty array is rejected",
				entries:      pdfcpu_types.Dict{"Decode": pdfcpu_types.Array{}},
				bound:        6,
				wantRejected: true,
			},
			{
				name: "an odd-length array is rejected",
				entries: pdfcpu_types.Dict{"Decode": pdfcpu_types.Array{
					pdfcpu_types.Integer(0), pdfcpu_types.Integer(1), pdfcpu_types.Integer(0),
				}},
				bound:        6,
				wantRejected: true,
			},
			{
				name: "an array over the bound is rejected rather than truncated",
				entries: pdfcpu_types.Dict{"Decode": pdfcpu_types.Array{
					pdfcpu_types.Integer(0), pdfcpu_types.Integer(1),
					pdfcpu_types.Integer(0), pdfcpu_types.Integer(1),
				}},
				bound:        2,
				wantRejected: true,
			},
			{
				name: "an array exactly at the bound is kept",
				entries: pdfcpu_types.Dict{"Decode": pdfcpu_types.Array{
					pdfcpu_types.Integer(1), pdfcpu_types.Integer(0),
					pdfcpu_types.Integer(1), pdfcpu_types.Integer(0),
				}},
				bound: 4,
				want:  []float64{1, 0, 1, 0},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := readDecodeArray(nil, imageDict(tc.entries), tc.bound)
				if tc.wantRejected {
					if err == nil {
						t.Fatalf("decode = %v, want a rejection", got)
					}
					if got != nil {
						t.Errorf("decode = %v, want nothing stored for a rejected array", got)
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected rejection: %v", err)
				}
				if len(got) != len(tc.want) {
					t.Fatalf("decode = %v, want %v", got, tc.want)
				}
				for i := range got {
					if got[i] != tc.want[i] {
						t.Fatalf("decode = %v, want %v", got, tc.want)
					}
				}
			})
		}
	})
}
