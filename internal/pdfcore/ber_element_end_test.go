// Unit tests for berElementEnd, the check-only reader that finds where the
// top-level BER element of a /Contents payload ends.
//
//	berElementEnd(in []byte) (end int, err error)
//
// end is the offset of the first byte after the element. It fails exactly
// where normalizeBER fails, with the same error text.
package pdfcore

import (
	"bytes"
	"testing"
)

func TestBerElementEnd_FindsTheElementEnd(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want int
	}{
		{"DER element followed by zero padding", []byte{0x02, 0x01, 0x05, 0x00, 0x00, 0x00}, 3},
		{"DER element with nothing after it", []byte{0x30, 0x03, 0x02, 0x01, 0x05}, 5},
		{"BER element followed by zero padding keeps its end-of-contents octets",
			[]byte{0x30, 0x80, 0x02, 0x01, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00}, 7},
		{"element followed by non-zero bytes", []byte{0x02, 0x01, 0x05, 0xDE, 0xAD, 0xBE, 0xEF}, 3},
		{"BER element followed by non-zero bytes",
			[]byte{0x30, 0x80, 0x02, 0x01, 0x05, 0x00, 0x00, 0x07, 0x00}, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := bytes.Clone(tc.in)
			got, err := berElementEnd(tc.in)
			if err != nil {
				t.Fatalf("berElementEnd error: %v", err)
			}
			if got != tc.want {
				t.Errorf("end = %d, want %d", got, tc.want)
			}
			if !bytes.Equal(tc.in, orig) {
				t.Errorf("input was modified")
			}
		})
	}
}

func TestBerElementEnd_RejectsInputWithoutACompleteElement(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"empty input", []byte{}},
		{"all-zero input", []byte{0x00, 0x00, 0x00, 0x00}},
		{"truncated definite element", []byte{0x30, 0x05, 0x02, 0x01}},
		{"indefinite element without end-of-contents", []byte{0x30, 0x80, 0x02, 0x01, 0x05}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if end, err := berElementEnd(tc.in); err == nil {
				t.Errorf("berElementEnd(% X) = %d, nil; want an error", tc.in, end)
			}
		})
	}
}

func TestBerElementEnd_FixturePayloadEnds(t *testing.T) {
	cases := []struct {
		name string
		want int
	}{
		{"signed.pdf", 2026},
		{"signed-ber-detached.pdf", 2028},
		{"signed-ber.pdf", 2265},
		{"signed-ber-trailing.pdf", 2028},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := berElementEnd(fixtureCMSPayload(t, tc.name))
			if err != nil {
				t.Fatalf("berElementEnd error: %v", err)
			}
			if got != tc.want {
				t.Errorf("end = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBerElementEnd_UndelimitedFixturePayloadsFail(t *testing.T) {
	for _, name := range []string{"signed-ber-badeoc.pdf", "signed-badcontents.pdf"} {
		t.Run(name, func(t *testing.T) {
			if end, err := berElementEnd(fixtureCMSPayload(t, name)); err == nil {
				t.Errorf("end = %d, nil; want an error", end)
			}
		})
	}
}

// berElementEnd agrees with normalizeBER on every normalizeBER seed input:
// same failure and error text, and an end consistent with what normalizeBER
// returns (normalizeBER does not return the end itself).
func TestBerElementEnd_AgreesWithNormalizeBER(t *testing.T) {
	var seeds [][]byte
	for _, tc := range berOKCases(t) {
		seeds = append(seeds, tc.in)
	}
	for _, tc := range berErrCases() {
		seeds = append(seeds, tc.in)
	}
	for _, tc := range berTailCases() {
		seeds = append(seeds, tc.in)
	}
	for _, name := range []string{"signed.pdf", "signed-ber.pdf", "signed-ber-detached.pdf", "signed-ber-trailing.pdf", "signed-ber-badeoc.pdf", "signed-badcontents.pdf"} {
		seeds = append(seeds, fixtureCMSPayload(t, name))
	}

	for i, in := range seeds {
		end, endErr := berElementEnd(in)
		der, isBER, tail, normErr := normalizeBER(in)
		if (endErr == nil) != (normErr == nil) {
			t.Errorf("seed %d: berElementEnd err = %v, normalizeBER err = %v; want both nil or both set", i, endErr, normErr)
			continue
		}
		if endErr != nil {
			if endErr.Error() != normErr.Error() {
				t.Errorf("seed %d: berElementEnd error %q, normalizeBER error %q; want the same text", i, endErr, normErr)
			}
			continue
		}
		if end <= 0 || end > len(in) {
			t.Errorf("seed %d: end = %d outside the %d-byte input", i, end, len(in))
			continue
		}
		if tail.count > 0 && tail.offset != end {
			t.Errorf("seed %d: end = %d, normalizeBER tail offset = %d", i, end, tail.offset)
		}
		if !isBER && len(der) != end {
			t.Errorf("seed %d: end = %d, DER copy length = %d", i, end, len(der))
		}
		again, againBER, againTail, err := normalizeBER(in[:end])
		if err != nil {
			t.Errorf("seed %d: normalizeBER(in[:%d]) error: %v", i, end, err)
			continue
		}
		if !bytes.Equal(again, der) || againBER != isBER || againTail != (berTail{}) {
			t.Errorf("seed %d: normalizeBER(in[:%d]) = (% X, %v, %+v), want (% X, %v, no tail)", i, end, again, againBER, againTail, der, isBER)
		}
	}
}
