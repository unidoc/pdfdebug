// Unit tests for the end offset normalizeBER returns: where the top-level BER
// element of a /Contents payload ends.
//
//	normalizeBER(in []byte) (der []byte, isBER bool, end int, tail berTail, err error)
//
// end is the offset of the first byte after the element.
package pdfcore

import (
	"bytes"
	"testing"
)

// elementEnd returns only the end and error of normalizeBER.
func elementEnd(in []byte) (int, error) {
	_, _, end, _, err := normalizeBER(in)
	return end, err
}

func TestNormalizeBEREnd_FindsTheElementEnd(t *testing.T) {
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
			got, err := elementEnd(tc.in)
			if err != nil {
				t.Fatalf("normalizeBER error: %v", err)
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

func TestNormalizeBEREnd_RejectsInputWithoutACompleteElement(t *testing.T) {
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
			if end, err := elementEnd(tc.in); err == nil {
				t.Errorf("normalizeBER(% X) end = %d, nil; want an error", tc.in, end)
			}
		})
	}
}

func TestNormalizeBEREnd_FixturePayloadEnds(t *testing.T) {
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
			got, err := elementEnd(fixtureCMSPayload(t, tc.name))
			if err != nil {
				t.Fatalf("normalizeBER error: %v", err)
			}
			if got != tc.want {
				t.Errorf("end = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNormalizeBEREnd_UndelimitedFixturePayloadsFail(t *testing.T) {
	for _, name := range []string{"signed-ber-badeoc.pdf", "signed-badcontents.pdf"} {
		t.Run(name, func(t *testing.T) {
			if end, err := elementEnd(fixtureCMSPayload(t, name)); err == nil {
				t.Errorf("end = %d, nil; want an error", end)
			}
		})
	}
}

// On every normalizeBER seed input that decodes, end agrees with the tail
// offset and the DER copy, and normalizing in[:end] gives the same result
// with no tail.
func TestNormalizeBEREnd_ConsistentWithNormalizedOutput(t *testing.T) {
	var seeds [][]byte
	for _, tc := range berOKCases(t) {
		seeds = append(seeds, tc.in)
	}
	for _, tc := range berTailCases() {
		seeds = append(seeds, tc.in)
	}
	for _, name := range []string{"signed.pdf", "signed-ber.pdf", "signed-ber-detached.pdf", "signed-ber-trailing.pdf"} {
		seeds = append(seeds, fixtureCMSPayload(t, name))
	}

	for i, in := range seeds {
		der, isBER, end, tail, err := normalizeBER(in)
		if err != nil {
			t.Errorf("seed %d: normalizeBER error: %v", i, err)
			continue
		}
		if end <= 0 || end > len(in) {
			t.Errorf("seed %d: end = %d outside the %d-byte input", i, end, len(in))
			continue
		}
		if tail.count > 0 && tail.offset != end {
			t.Errorf("seed %d: end = %d, tail offset = %d", i, end, tail.offset)
		}
		if !isBER && len(der) != end {
			t.Errorf("seed %d: end = %d, DER copy length = %d", i, end, len(der))
		}
		again, againBER, againEnd, againTail, err := normalizeBER(in[:end])
		if err != nil {
			t.Errorf("seed %d: normalizeBER(in[:%d]) error: %v", i, end, err)
			continue
		}
		if !bytes.Equal(again, der) || againBER != isBER || againEnd != end || againTail != (berTail{}) {
			t.Errorf("seed %d: normalizeBER(in[:%d]) = (% X, %v, %d, %+v), want (% X, %v, %d, no tail)", i, end, again, againBER, againEnd, againTail, der, isBER, end)
		}
	}
}
