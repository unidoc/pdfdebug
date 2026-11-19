package pdfcore

import (
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Text strings decode as ISO 32000-1 7.9.2.2 says: a FE FF prefix is UTF-16BE
// of even length, an EF BB BF prefix is UTF-8, plain ASCII passes through, and
// anything else is PDFDocEncoding with U+FFFD for codes it leaves undefined.
// StringLiteral and HexLiteral decode the same way.

func TestDecodeTextString_PDFDocEncodingTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   pdfcpu_types.Object
		want string
	}{
		{"0x80 on StringLiteral is PDFDocEncoding's bullet", pdfcpu_types.StringLiteral("\x80"), "\u2022"},
		{"0x80 on HexLiteral is PDFDocEncoding's bullet", pdfcpu_types.HexLiteral("80"), "\u2022"},
		{"escaped 0x92 on StringLiteral is the trade mark sign", pdfcpu_types.StringLiteral(`don\222t`), "don\u2122t"},
		{"undefined byte in a non-ASCII string is U+FFFD", pdfcpu_types.HexLiteral("9F41"), "\uFFFDA"},
		{"0x5C in a hex string is kept", pdfcpu_types.HexLiteral("415C42"), `A\B`},
		{"UTF-8 BOM is stripped", pdfcpu_types.HexLiteral("EFBBBF41"), "A"},
		{"UTF-16BE with BOM", pdfcpu_types.HexLiteral("FEFF00E9"), "\u00e9"},
		{"plain ASCII passes through", pdfcpu_types.StringLiteral("Invoice 2024-001"), "Invoice 2024-001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := decodeTextStringOK(tc.in)
			if !ok || got != tc.want {
				t.Errorf("decodeTextStringOK(%T %v) = %q (% x), %v; want %q (% x), true",
					tc.in, tc.in, got, []byte(got), ok, tc.want, []byte(tc.want))
			}
		})
	}
}

// An odd byte count after a UTF-16BE BOM is a decode failure, so the display
// falls back to the raw hex digits.
func TestDecodeTextString_BOMWithOddTrailingByteIsADecodeFailure(t *testing.T) {
	in := pdfcpu_types.HexLiteral("FEFF005469")
	if got, ok := decodeTextStringOK(in); ok {
		t.Errorf("decodeTextStringOK = %q (% x), true; want a decode failure", got, []byte(got))
	}
	if got := textStringOrRaw(in); got != "FEFF005469" {
		t.Errorf("textStringOrRaw = %q, want the raw fallback %q", got, "FEFF005469")
	}
}

// A UTF-8 BOM followed by invalid UTF-8 fails to decode, so the display is the
// raw rendering: a HexLiteral's stored hex digits, and for a StringLiteral,
// whose raw bytes are not valid UTF-8, the uppercase hex of those bytes.
// pdfcpu's decoder returns valid UTF-8 whenever it succeeds, so the
// decode-yields-invalid-UTF-8 branch of textStringOrRaw is not reachable
// through it; that guard stays because the decoder is outside this package.
func TestTextStringOrRaw_UndecodableStringWithInvalidRawBytesShowsHex(t *testing.T) {
	in := pdfcpu_types.HexLiteral("EFBBBF80")
	if got, ok := decodeTextStringOK(in); ok {
		t.Errorf("decodeTextStringOK = %q (% x), true; want a decode failure", got, []byte(got))
	}
	if got := textStringOrRaw(in); got != "EFBBBF80" {
		t.Errorf("textStringOrRaw = %q, want the hex fallback %q", got, "EFBBBF80")
	}
	lit := pdfcpu_types.StringLiteral("\xef\xbb\xbf\x80")
	if got, ok := decodeTextStringOK(lit); ok {
		t.Errorf("decodeTextStringOK(StringLiteral) = %q (% x), true; want a decode failure", got, []byte(got))
	}
	if got := textStringOrRaw(lit); got != "EFBBBF80" {
		t.Errorf("textStringOrRaw(StringLiteral) = %q, want the hex fallback %q", got, "EFBBBF80")
	}
}
