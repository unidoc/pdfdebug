// Unit tests for the BER-to-DER normalizer used before CMS parsing.
//
// Surface under test:
//
//	normalizeBER(in []byte) (der []byte, isBER bool, end int, tail berTail, err error)
//	const maxBERDepth = 64
//
// Inputs are hand-built byte slices. Expected DER is built either by hand or
// with asn1.Marshal of the same value. Every case also checks that the input
// slice is left untouched.
package pdfcore

import (
	"bytes"
	"encoding/asn1"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- byte builders -----------------------------------------------------------

// berLen returns the minimal DER length octets for n.
func berLen(n int) []byte {
	if n < 128 {
		return []byte{byte(n)}
	}
	var b []byte
	for v := n; v > 0; v >>= 8 {
		b = append([]byte{byte(v)}, b...)
	}
	return append([]byte{0x80 | byte(len(b))}, b...)
}

// berDef encodes a one-byte tag, a minimal definite length and the content.
func berDef(tag byte, content ...[]byte) []byte {
	c := bytes.Join(content, nil)
	out := append([]byte{tag}, berLen(len(c))...)
	return append(out, c...)
}

// berIndef encodes a one-byte tag, the indefinite length form, the content
// and an end-of-contents.
func berIndef(tag byte, content ...[]byte) []byte {
	c := bytes.Join(content, nil)
	out := append([]byte{tag, 0x80}, c...)
	return append(out, 0x00, 0x00)
}

// berNestIndef wraps inner in n indefinite-length SEQUENCEs.
func berNestIndef(n int, inner []byte) []byte {
	out := make([]byte, 0, 4*n+len(inner))
	for range n {
		out = append(out, 0x30, 0x80)
	}
	out = append(out, inner...)
	for range n {
		out = append(out, 0x00, 0x00)
	}
	return out
}

// berNestIndefTag wraps inner in n indefinite-length elements with a
// one-byte constructed tag.
func berNestIndefTag(tag byte, n int, inner []byte) []byte {
	out := bytes.Repeat([]byte{tag, 0x80}, n)
	out = append(out, inner...)
	return append(out, make([]byte, 2*n)...)
}

// berNestDef wraps inner in n definite-length SEQUENCEs.
func berNestDef(n int, inner []byte) []byte {
	out := inner
	for range n {
		out = berDef(0x30, out)
	}
	return out
}

// berAllIndef re-encodes every constructed element of a DER value with the
// indefinite length form. Primitive elements are copied unchanged. Only
// one-byte tags are supported, which is all the CMS fixtures use.
func berAllIndef(t testing.TB, der []byte) []byte {
	t.Helper()
	var rv asn1.RawValue
	rest, err := asn1.Unmarshal(der, &rv)
	if err != nil || len(rest) != 0 {
		t.Fatalf("berAllIndef: not a single DER element: err=%v rest=%d", err, len(rest))
	}
	if !rv.IsCompound {
		return rv.FullBytes
	}
	if rv.FullBytes[0]&0x1f == 0x1f {
		t.Fatalf("berAllIndef: multi-byte tag not supported")
	}
	out := []byte{rv.FullBytes[0], 0x80}
	for c := rv.Bytes; len(c) > 0; {
		var child asn1.RawValue
		r, err := asn1.Unmarshal(c, &child)
		if err != nil {
			t.Fatalf("berAllIndef: child parse: %v", err)
		}
		out = append(out, berAllIndef(t, child.FullBytes)...)
		c = r
	}
	return append(out, 0x00, 0x00)
}

func mustMarshal(t testing.TB, v any) []byte {
	t.Helper()
	b, err := asn1.Marshal(v)
	if err != nil {
		t.Fatalf("asn1.Marshal: %v", err)
	}
	return b
}

// fixtureCMSPayload returns the decoded /Contents hex string of a committed
// signature fixture in testdata/. A missing fixture fails the test.
func fixtureCMSPayload(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	start := bytes.Index(data, []byte("/Contents <"))
	if start < 0 {
		t.Fatalf("fixture %s: no /Contents hex string", name)
	}
	start += len("/Contents <")
	end := bytes.IndexByte(data[start:], '>')
	if end < 0 {
		t.Fatalf("fixture %s: unterminated /Contents hex string", name)
	}
	raw, err := hex.DecodeString(string(data[start : start+end]))
	if err != nil {
		t.Fatalf("fixture %s: /Contents is not hex: %v", name, err)
	}
	return raw
}

// --- cases -------------------------------------------------------------------

type berOKCase struct {
	name    string
	in      []byte
	want    []byte
	wantBER bool
}

type berErrCase struct {
	name string
	in   []byte
	want []string // every substring must appear in the error text
}

type berInner struct{ A int }
type berOuter struct{ Inner berInner }
type berBlob struct{ B []byte }

func berOKCases(t testing.TB) []berOKCase {
	blob130 := bytes.Repeat([]byte{0x5A}, 130)
	derSeq := mustMarshal(t, berOuter{Inner: berInner{A: 5}}) // 30 05 30 03 02 01 05
	return []berOKCase{
		{
			name:    "definite DER element round-trips byte-identical",
			in:      derSeq,
			want:    derSeq,
			wantBER: false,
		},
		{
			name:    "indefinite SEQUENCE inside indefinite SEQUENCE becomes DER",
			in:      berIndef(0x30, berIndef(0x30, []byte{0x02, 0x01, 0x05})),
			want:    derSeq,
			wantBER: true,
		},
		{
			name:    "indefinite SEQUENCE inside definite SEQUENCE gets the parent length recomputed",
			in:      berDef(0x30, berIndef(0x30, []byte{0x02, 0x01, 0x05})),
			want:    derSeq,
			wantBER: true,
		},
		{
			name:    "indefinite element with 128 or more content bytes gets a long-form length",
			in:      berIndef(0x30, berDef(0x04, blob130)),
			want:    mustMarshal(t, berBlob{B: blob130}),
			wantBER: true,
		},
		{
			name:    "non-minimal long-form length becomes minimal",
			in:      []byte{0x04, 0x81, 0x03, 0x01, 0x02, 0x03},
			want:    []byte{0x04, 0x03, 0x01, 0x02, 0x03},
			wantBER: true,
		},
		{
			name:    "long-form length with leading zero octets within four octets becomes minimal",
			in:      []byte{0x04, 0x84, 0x00, 0x00, 0x00, 0x01, 0xAA},
			want:    []byte{0x04, 0x01, 0xAA},
			wantBER: true,
		},
		{
			name:    "SET elements out of DER sort order keep their input order",
			in:      berIndef(0x31, []byte{0x02, 0x01, 0x02}, []byte{0x02, 0x01, 0x01}),
			want:    []byte{0x31, 0x06, 0x02, 0x01, 0x02, 0x02, 0x01, 0x01},
			wantBER: true,
		},
		{
			name:    "constructed OCTET STRING of two segments becomes one primitive OCTET STRING",
			in:      berIndef(0x24, []byte{0x04, 0x02, 0xAA, 0xBB}, []byte{0x04, 0x01, 0xCC}),
			want:    []byte{0x04, 0x03, 0xAA, 0xBB, 0xCC},
			wantBER: true,
		},
		{
			name:    "definite constructed OCTET STRING becomes one primitive OCTET STRING",
			in:      berDef(0x24, []byte{0x04, 0x01, 0xAA}, []byte{0x04, 0x01, 0xBB}),
			want:    []byte{0x04, 0x02, 0xAA, 0xBB},
			wantBER: true,
		},
		{
			name:    "nested constructed OCTET STRING flattens",
			in:      berIndef(0x24, berIndef(0x24, []byte{0x04, 0x01, 0xAA}), []byte{0x04, 0x01, 0xBB}),
			want:    []byte{0x04, 0x02, 0xAA, 0xBB},
			wantBER: true,
		},
		{
			name:    "constructed OCTET STRING inside a SEQUENCE flattens and the SEQUENCE length is recomputed",
			in:      berIndef(0x30, berIndef(0x24, []byte{0x04, 0x01, 0xAA}, []byte{0x04, 0x01, 0xBB})),
			want:    []byte{0x30, 0x04, 0x04, 0x02, 0xAA, 0xBB},
			wantBER: true,
		},
		{
			name:    "definite constructed OCTET STRING segment inside an indefinite one flattens",
			in:      berIndef(0x24, berDef(0x24, []byte{0x04, 0x01, 0xAA}, []byte{0x04, 0x01, 0xBB}), []byte{0x04, 0x01, 0xCC}),
			want:    []byte{0x04, 0x03, 0xAA, 0xBB, 0xCC},
			wantBER: true,
		},
		{
			name:    "constructed OCTET STRING segments nested to depth 64 are accepted",
			in:      berNestIndefTag(0x24, 63, []byte{0x04, 0x01, 0xAA}),
			want:    []byte{0x04, 0x01, 0xAA},
			wantBER: true,
		},
		{
			name:    "constructed BIT STRING keeps the final unused-bits count",
			in:      berIndef(0x23, []byte{0x03, 0x02, 0x00, 0xAA}, []byte{0x03, 0x02, 0x04, 0xB0}),
			want:    []byte{0x03, 0x03, 0x04, 0xAA, 0xB0},
			wantBER: true,
		},
		{
			name:    "empty constructed OCTET STRING becomes an empty primitive OCTET STRING",
			in:      []byte{0x24, 0x80, 0x00, 0x00},
			want:    []byte{0x04, 0x00},
			wantBER: true,
		},
		{
			name:    "empty constructed BIT STRING becomes a primitive BIT STRING with zero unused bits",
			in:      []byte{0x23, 0x80, 0x00, 0x00},
			want:    []byte{0x03, 0x01, 0x00},
			wantBER: true,
		},
		{
			name:    "constructed UTF8String becomes one primitive UTF8String",
			in:      berIndef(0x2C, []byte{0x0C, 0x01, 'a'}, []byte{0x0C, 0x01, 'b'}),
			want:    []byte{0x0C, 0x02, 'a', 'b'},
			wantBER: true,
		},
		{
			name:    "constructed UTF8String of OCTET STRING segments becomes one primitive UTF8String",
			in:      berIndef(0x2C, []byte{0x04, 0x01, 'a'}, []byte{0x04, 0x01, 'b'}),
			want:    []byte{0x0C, 0x02, 'a', 'b'},
			wantBER: true,
		},
		{
			name:    "constructed UTF8String with a nested constructed OCTET STRING segment flattens",
			in:      berIndef(0x2C, berIndef(0x24, []byte{0x04, 0x01, 'a'}), []byte{0x0C, 0x01, 'b'}),
			want:    []byte{0x0C, 0x02, 'a', 'b'},
			wantBER: true,
		},
		{
			name:    "high-tag-number constructed element keeps its tag bytes",
			in:      []byte{0xBF, 0x81, 0x00, 0x80, 0x02, 0x01, 0x05, 0x00, 0x00},
			want:    []byte{0xBF, 0x81, 0x00, 0x03, 0x02, 0x01, 0x05},
			wantBER: true,
		},
		{
			name:    "high-tag-number primitive element is copied unchanged",
			in:      []byte{0x9F, 0x1F, 0x01, 0xAA},
			want:    []byte{0x9F, 0x1F, 0x01, 0xAA},
			wantBER: false,
		},
		{
			name:    "high-tag-number element nested in an indefinite SEQUENCE keeps its tag bytes",
			in:      berIndef(0x30, []byte{0x9F, 0x81, 0x00, 0x01, 0xAA}),
			want:    []byte{0x30, 0x05, 0x9F, 0x81, 0x00, 0x01, 0xAA},
			wantBER: true,
		},
		{
			name:    "trailing zero padding is accepted and dropped",
			in:      append(append([]byte{}, derSeq...), 0x00, 0x00, 0x00, 0x00),
			want:    derSeq,
			wantBER: false,
		},
		{
			name: "definite parent whose content grows past 127 when an empty BIT STRING flattens gets a long-form length",
			in:   berDef(0x30, []byte{0x23, 0x00}, berDef(0x04, bytes.Repeat([]byte{0x5A}, 123))),
			want: append([]byte{0x30, 0x81, 0x80, 0x03, 0x01, 0x00},
				berDef(0x04, bytes.Repeat([]byte{0x5A}, 123))...),
			wantBER: true,
		},
		{
			name:    "nesting at depth 64 is accepted",
			in:      berNestIndef(63, []byte{0x04, 0x01, 0xAA}),
			want:    berNestDef(63, []byte{0x04, 0x01, 0xAA}),
			wantBER: true,
		},
	}
}

func berErrCases() []berErrCase {
	return []berErrCase{
		{"empty input", []byte{},
			[]string{"BER element at offset 0 needs 1 bytes"}},
		{"tag with no length octet", []byte{0x30},
			[]string{"BER element at offset 0 needs 2 bytes"}},
		{"high-tag-number tag with no length octet", []byte{0x5F, 0x21},
			[]string{"BER element at offset 0 needs 3 bytes"}},
		{"end-of-contents cut to one byte", []byte{0x30, 0x80, 0x02, 0x01, 0x05, 0x00},
			[]string{"BER element at offset 5 needs 2 bytes"}},
		{"truncated segment of a constructed OCTET STRING", []byte{0x24, 0x80, 0x04},
			[]string{"BER element at offset 2 needs"}},
		{"nested constructed OCTET STRING segment with no end-of-contents",
			[]byte{0x24, 0x80, 0x24, 0x80, 0x04, 0x01, 0xAA},
			[]string{"BER element at offset 2 has no end-of-contents"}},
		{"constructed OCTET STRING segments nested to depth 65",
			berNestIndefTag(0x24, 64, []byte{0x04, 0x01, 0xAA}),
			[]string{"BER nesting deeper than 64 at offset 128"}},
		{"truncated tag inside its parent", []byte{0x30, 0x01, 0x1F},
			[]string{"BER element at offset 2 needs"}},
		{"truncated high-tag-number tag", []byte{0x5F, 0x81, 0x82},
			[]string{"BER element at offset 0 needs"}},
		{"truncated length", []byte{0x04, 0x82, 0x01},
			[]string{"BER element at offset 0 needs"}},
		{"truncated content", []byte{0x04, 0x05, 0x01, 0x02},
			[]string{"BER element at offset 0 needs"}},
		{"nested long-form length far past the input end is a truncation, not an oversize",
			[]byte{0x30, 0x09, 0x04, 0x84, 0xFF, 0xFF, 0xFF, 0xFF, 0x01, 0x02, 0x03},
			[]string{"BER element at offset 2 needs"}},
		{"child content runs past its definite parent end",
			[]byte{0x30, 0x03, 0x04, 0x05, 0x01, 0x00, 0x00, 0x00, 0x00},
			[]string{"BER element at offset 2 needs"}},
		{"indefinite child inside a definite parent has no end-of-contents before the parent end",
			[]byte{0x30, 0x05, 0x30, 0x80, 0x02, 0x01, 0x05, 0x00, 0x00},
			[]string{"BER element at offset 2 has no end-of-contents"}},
		{"indefinite element with no end-of-contents", []byte{0x30, 0x80, 0x02, 0x01, 0x05},
			[]string{"BER element at offset 0 has no end-of-contents"}},
		{"end-of-contents with a non-zero length octet",
			[]byte{0x30, 0x80, 0x02, 0x01, 0x05, 0x00, 0x01},
			[]string{"malformed end-of-contents at offset 5"}},
		{"end-of-contents inside a definite-length element",
			[]byte{0x30, 0x05, 0x02, 0x01, 0x05, 0x00, 0x00},
			[]string{"malformed end-of-contents at offset 5"}},
		{"end-of-contents at top level", []byte{0x00, 0x00},
			[]string{"malformed end-of-contents at offset 0"}},
		{"indefinite length on a primitive", []byte{0x04, 0x80, 0x01, 0x00, 0x00},
			[]string{"indefinite length on a primitive element at offset 0"}},
		{"length form 0xFF", []byte{0x04, 0xFF},
			[]string{"unsupported length encoding at offset 0"}},
		{"five-octet long-form length", []byte{0x04, 0x85, 0x00, 0x00, 0x00, 0x00, 0x01, 0xAA},
			[]string{"unsupported length encoding at offset 0"}},
		{"high-tag-number form for a tag under 31", []byte{0x1F, 0x05, 0x00},
			[]string{"unsupported tag encoding at offset 0"}},
		{"high-tag-number form with a leading 0x80 octet", []byte{0x1F, 0x80, 0x21, 0x00},
			[]string{"unsupported tag encoding at offset 0"}},
		{"high-tag-number form for a tag under 31 inside a SEQUENCE", []byte{0x30, 0x03, 0x9F, 0x05, 0x00},
			[]string{"unsupported tag encoding at offset 2"}},
		{"tag number that does not fit in 31 bits",
			[]byte{0x1F, 0x88, 0x80, 0x80, 0x80, 0x00, 0x00},
			[]string{"unsupported tag encoding at offset 0"}},
		{"nesting at depth 65", berNestIndef(64, []byte{0x04, 0x01, 0xAA}),
			[]string{"BER nesting deeper than 64 at offset 128"}},
		{"constructed OCTET STRING with a segment of another type",
			berIndef(0x24, []byte{0x04, 0x01, 0xAA}, []byte{0x0C, 0x01, 0x41}),
			[]string{"constructed string at offset 0 has a segment of another type"}},
		{"constructed UTF8String with an INTEGER segment",
			berIndef(0x2C, []byte{0x04, 0x01, 'a'}, []byte{0x02, 0x01, 0x05}),
			[]string{"constructed string at offset 0 has a segment of another type"}},
		{"constructed OCTET STRING segment of a UTF8String holding a UTF8String segment",
			berIndef(0x2C, berIndef(0x24, []byte{0x0C, 0x01, 'a'})),
			[]string{"constructed string at offset 2 has a segment of another type"}},
		{"constructed BIT STRING with an OCTET STRING segment",
			berIndef(0x23, []byte{0x04, 0x02, 0x00, 0xA0}),
			[]string{"constructed string at offset 0 has a segment of another type"}},
		{"constructed BIT STRING with non-zero unused bits on a non-final segment",
			berIndef(0x23, []byte{0x03, 0x02, 0x04, 0xA0}, []byte{0x03, 0x02, 0x00, 0xB0}),
			[]string{"malformed BIT STRING segment at offset 2"}},
		{"constructed BIT STRING with an empty segment",
			berIndef(0x23, []byte{0x03, 0x00}, []byte{0x03, 0x02, 0x00, 0xB0}),
			[]string{"malformed BIT STRING segment at offset 2"}},
		{"constructed BIT STRING with an unused-bits octet above 7",
			berIndef(0x23, []byte{0x03, 0x02, 0x08, 0xA0}),
			[]string{"malformed BIT STRING segment at offset 2"}},
		{"constructed BIT STRING whose final segment has no data bits but non-zero unused bits",
			berIndef(0x23, []byte{0x03, 0x02, 0x00, 0xA0}, []byte{0x03, 0x01, 0x05}),
			[]string{"malformed BIT STRING segment at offset 6"}},
	}
}

type berTailCase struct {
	name     string
	in       []byte
	want     []byte
	wantBER  bool
	wantTail berTail
}

func berTailCases() []berTailCase {
	return []berTailCase{
		{"one non-zero byte after a DER element", []byte{0x02, 0x01, 0x05, 0x01},
			[]byte{0x02, 0x01, 0x05}, false, berTail{offset: 3, count: 1}},
		{"non-zero byte after a BER element",
			[]byte{0x30, 0x80, 0x02, 0x01, 0x05, 0x00, 0x00, 0x07},
			[]byte{0x30, 0x03, 0x02, 0x01, 0x05}, true, berTail{offset: 7, count: 1}},
		{"count runs from the element end through the last non-zero byte, zero padding after it excluded",
			[]byte{0x02, 0x01, 0x05, 0x00, 0xDE, 0x00, 0xEF, 0x00, 0x00},
			[]byte{0x02, 0x01, 0x05}, false, berTail{offset: 3, count: 4}},
		{"zero padding only reports no tail", []byte{0x02, 0x01, 0x05, 0x00, 0x00},
			[]byte{0x02, 0x01, 0x05}, false, berTail{}},
	}
}

// --- table tests -------------------------------------------------------------

func TestNormalizeBER_Normalizes(t *testing.T) {
	for _, tc := range berOKCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			orig := bytes.Clone(tc.in)
			out, isBER, _, tail, err := normalizeBER(tc.in)
			if err != nil {
				t.Fatalf("normalizeBER error: %v", err)
			}
			if !bytes.Equal(out, tc.want) {
				t.Errorf("output = % X\nwant     % X", out, tc.want)
			}
			if isBER != tc.wantBER {
				t.Errorf("isBER = %v, want %v", isBER, tc.wantBER)
			}
			if tail != (berTail{}) {
				t.Errorf("tail = %+v, want none", tail)
			}
			if !bytes.Equal(tc.in, orig) {
				t.Errorf("input was modified")
			}
		})
	}
}

func TestNormalizeBER_ReportsNonZeroBytesAfterTheElement(t *testing.T) {
	for _, tc := range berTailCases() {
		t.Run(tc.name, func(t *testing.T) {
			orig := bytes.Clone(tc.in)
			out, isBER, _, tail, err := normalizeBER(tc.in)
			if err != nil {
				t.Fatalf("normalizeBER error: %v", err)
			}
			if !bytes.Equal(out, tc.want) {
				t.Errorf("output = % X\nwant     % X", out, tc.want)
			}
			if isBER != tc.wantBER {
				t.Errorf("isBER = %v, want %v", isBER, tc.wantBER)
			}
			if tail != tc.wantTail {
				t.Errorf("tail = %+v, want %+v", tail, tc.wantTail)
			}
			if !bytes.Equal(tc.in, orig) {
				t.Errorf("input was modified")
			}
		})
	}
}

func TestNormalizeBER_RejectsMalformedInput(t *testing.T) {
	for _, tc := range berErrCases() {
		t.Run(tc.name, func(t *testing.T) {
			orig := bytes.Clone(tc.in)
			_, _, _, _, err := normalizeBER(tc.in)
			if err == nil {
				t.Fatalf("normalizeBER accepted % X, want an error containing %q", tc.in, tc.want)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), w)
				}
			}
			if !bytes.Equal(tc.in, orig) {
				t.Errorf("input was modified")
			}
		})
	}
}

func TestNormalizeBER_ReturnsANewBuffer(t *testing.T) {
	in := mustMarshal(t, berOuter{Inner: berInner{A: 5}})
	orig := bytes.Clone(in)
	out, _, _, _, err := normalizeBER(in)
	if err != nil {
		t.Fatalf("normalizeBER error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("empty output")
	}
	out[0] ^= 0xFF
	if !bytes.Equal(in, orig) {
		t.Errorf("writing to the output changed the input: the output aliases the input")
	}
}

func TestNormalizeBER_DERInputTakesTheCopyPath(t *testing.T) {
	elem := mustMarshal(t, berOuter{Inner: berInner{A: 5}})
	in := append(bytes.Clone(elem), 0x00, 0x00, 0x00)
	out, isBER, _, tail, err := normalizeBER(in)
	if err != nil {
		t.Fatalf("normalizeBER error: %v", err)
	}
	if isBER {
		t.Errorf("isBER = true for a DER element")
	}
	if tail != (berTail{}) {
		t.Errorf("tail = %+v, want none", tail)
	}
	if !bytes.Equal(out, in[:len(elem)]) {
		t.Errorf("output = % X, want % X", out, in[:len(elem)])
	}
	if &out[0] == &in[0] {
		t.Errorf("output shares the input's backing array")
	}
}

func TestNormalizeBER_LargeZeroPaddingIsAccepted(t *testing.T) {
	elem := []byte{0x02, 0x01, 0x05}
	in := append(bytes.Clone(elem), make([]byte, 9<<20)...)
	out, isBER, _, tail, err := normalizeBER(in)
	if err != nil {
		t.Fatalf("normalizeBER error on a small element in a large zero-padded buffer: %v", err)
	}
	if !bytes.Equal(out, elem) {
		t.Errorf("output = % X, want % X", out, elem)
	}
	if isBER {
		t.Errorf("isBER = true, want false (zero padding is not part of the element)")
	}
	if tail != (berTail{}) {
		t.Errorf("tail = %+v, want none", tail)
	}
}

func TestNormalizeBER_FullyIndefiniteCMSNormalizesToOriginalDER(t *testing.T) {
	der := fixtureCMSPayload(t, "signed.pdf")
	var rv asn1.RawValue
	if _, err := asn1.Unmarshal(der, &rv); err != nil {
		t.Fatalf("signed.pdf payload: %v", err)
	}
	ci := rv.FullBytes
	ber := berAllIndef(t, ci)

	out, isBER, _, _, err := normalizeBER(ber)
	if err != nil {
		t.Fatalf("normalizeBER error: %v", err)
	}
	if !isBER {
		t.Errorf("isBER = false for a fully indefinite re-encoding")
	}
	if !bytes.Equal(out, ci) {
		t.Errorf("normalized output differs from the original DER ContentInfo (len %d vs %d)", len(out), len(ci))
	}

	out2, isBER2, _, _, err := normalizeBER(der)
	if err != nil {
		t.Fatalf("normalizeBER error on the zero-padded DER payload: %v", err)
	}
	if isBER2 {
		t.Errorf("isBER = true for the DER payload of signed.pdf")
	}
	if !bytes.Equal(out2, ci) {
		t.Errorf("DER payload did not round-trip byte-identical")
	}
}

// Normalizing a 64-deep BER input holding a 12 MiB OCTET STRING must not
// allocate a buffer per nesting level.
func TestNormalizeBER_AllocationStaysProportionalToInput(t *testing.T) {
	in := berNestIndef(63, berDef(0x04, make([]byte, 12<<20)))

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	out, isBER, _, _, err := normalizeBER(in)
	runtime.ReadMemStats(&after)

	if err != nil {
		t.Fatalf("normalizeBER error: %v", err)
	}
	if !isBER {
		t.Errorf("isBER = false for an indefinite-length input")
	}
	// Each of the 64 elements adds at most 4 bytes.
	if len(out) == 0 || len(out) > len(in)+4*64 {
		t.Fatalf("output length %d outside (0, %d]", len(out), len(in)+4*64)
	}
	alloc := after.TotalAlloc - before.TotalAlloc
	if limit := uint64(3 * len(in)); alloc >= limit {
		t.Errorf("normalizeBER allocated %d bytes for a %d-byte input, want less than %d", alloc, len(in), limit)
	}
}

// DER input is validated without building records: the returned copy is the
// only allocation.
func TestNormalizeBER_DERAllocatesOnlyTheCopy(t *testing.T) {
	der := fixtureCMSPayload(t, "signed.pdf")
	allocs := testing.AllocsPerRun(10, func() {
		if _, isBER, _, _, err := normalizeBER(der); err != nil || isBER {
			t.Fatalf("normalizeBER: isBER=%v err=%v, want DER", isBER, err)
		}
	})
	if allocs != 1 {
		t.Errorf("normalizeBER made %v allocations for DER input, want 1", allocs)
	}
}

// derElementCount returns the number of elements in one DER element,
// itself included.
func derElementCount(t *testing.T, der []byte) int {
	t.Helper()
	var rv asn1.RawValue
	if _, err := asn1.Unmarshal(der, &rv); err != nil {
		t.Fatalf("derElementCount: %v", err)
	}
	n := 1
	if !rv.IsCompound {
		return n
	}
	for c := rv.Bytes; len(c) > 0; {
		var child asn1.RawValue
		rest, err := asn1.Unmarshal(c, &child)
		if err != nil {
			t.Fatalf("derElementCount: %v", err)
		}
		n += derElementCount(t, child.FullBytes)
		c = rest
	}
	return n
}

// --- fuzz --------------------------------------------------------------------

// FuzzNormalizeBER holds, for every input: no panic, input unchanged, and on
// success an output that is one complete DER element, no more than 4 bytes
// per element longer than the input, and that normalizes to itself.
func FuzzNormalizeBER(f *testing.F) {
	for _, tc := range berOKCases(f) {
		f.Add(tc.in)
	}
	for _, tc := range berErrCases() {
		f.Add(tc.in)
	}
	for _, tc := range berTailCases() {
		f.Add(tc.in)
	}
	for _, name := range []string{"signed.pdf", "signed-ber.pdf", "signed-ber-detached.pdf"} {
		f.Add(fixtureCMSPayload(f, name))
	}

	f.Fuzz(func(t *testing.T, in []byte) {
		orig := bytes.Clone(in)
		out, _, _, tail, err := normalizeBER(in)
		if !bytes.Equal(in, orig) {
			t.Fatalf("input was modified")
		}
		if err != nil {
			return
		}
		if tail.count < 0 || tail.offset+tail.count > len(in) {
			t.Fatalf("tail %+v outside the %d-byte input", tail, len(in))
		}
		var rv asn1.RawValue
		rest, perr := asn1.Unmarshal(out, &rv)
		if perr != nil {
			t.Fatalf("output does not parse as one DER element: %v\nin  % X\nout % X", perr, in, out)
		}
		if len(rest) != 0 {
			t.Fatalf("output has %d bytes after the element", len(rest))
		}
		// Each element adds at most 4 bytes beyond its children's growth: 1 to
		// 5 length octets out against at least 1 in, contents copied, and an
		// empty flattened BIT STRING gains only its 1-byte unused-bits octet.
		if limit := len(in) + 4*derElementCount(t, out); len(out) > limit {
			t.Fatalf("output length %d exceeds %d (input %d + 4 per output element)", len(out), limit, len(in))
		}
		again, isBER, _, againTail, err := normalizeBER(out)
		if err != nil {
			t.Fatalf("normalizing the output failed: %v\nout % X", err, out)
		}
		if isBER {
			t.Fatalf("normalized output still reports BER\nout % X", out)
		}
		if againTail != (berTail{}) {
			t.Fatalf("normalized output reports a tail %+v", againTail)
		}
		if !bytes.Equal(again, out) {
			t.Fatalf("normalization is not idempotent\nout   % X\nagain % X", out, again)
		}
	})
}
