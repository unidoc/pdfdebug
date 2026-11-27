package pdfcore

import (
	"encoding/hex"
	"strings"
	"unicode/utf8"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// PDF text-string decoding (ISO 32000-1 7.9.2.2), shared by the readers that
// surface human-readable string fields.

// decodeTextString decodes a PDF string object (StringLiteral or HexLiteral) to
// Go text via pdfcpu's StringOrHexLiteral, returning "" for a non-string object
// or a decode failure.
//
// TEXT FIELDS ONLY. Binary-carrying strings (filespec /Params /CheckSum,
// signature /Contents and /Cert, trailer /ID) must stay on stringValue;
// decoding them corrupts the bytes.
//
// The semantics are pdfcpu's. A StringLiteral is unescaped and a HexLiteral
// hex-decoded, then both byte sequences decode the same way:
//
//   - bytes starting FE FF are UTF-16BE and must have an even length, else the
//     decode fails;
//   - bytes starting EF BB BF are UTF-8 with the BOM stripped and must be valid
//     UTF-8, else the decode fails;
//   - a string whose bytes are all ASCII outside 0x18-0x1F passes through
//     unchanged;
//   - any other string is PDFDocEncoding (ISO 32000-1 Annex D) byte by byte:
//     0x80 is U+2022 and 0x92 is U+2122, and a code the encoding leaves
//     undefined becomes U+FFFD, ASCII control codes such as 0x00 included.
//
// A successful decode is always valid UTF-8, and a 0x5C byte in a hex string
// is kept: HexLiteral("415C42") gives "A\B".
//
// Callers that RENDER a field use textStringOrRaw instead.
func decodeTextString(obj pdfcpu_types.Object) string {
	s, _ := decodeTextStringOK(obj)
	return s
}

// decodeTextStringOK is decodeTextString plus whether the decode succeeded. The
// two differ because a text string may legitimately decode to "": the BOM-only
// HexLiteral("FEFF") is well-formed and empty. ok is false only for a
// non-string object or a decode error.
func decodeTextStringOK(obj pdfcpu_types.Object) (string, bool) {
	switch obj.(type) {
	case pdfcpu_types.StringLiteral, pdfcpu_types.HexLiteral:
		if s, err := pdfcpu_types.StringOrHexLiteral(obj); err == nil && s != nil {
			return *s, true
		}
	}
	return "", false
}

// textStringOrRaw decodes a PDF text string for display. A failed decode falls
// back to stringValue's raw rendering, so an undecodable field surfaces as
// visible mojibake instead of vanishing. A non-string object yields "". An
// empty decode stays empty rather than falling back, so an empty /Title never
// renders as the literal text "FEFF".
//
// The result is always valid UTF-8: json.Marshal replaces invalid bytes with
// U+FFFD, which destroys the value. pdfcpu's decoder returns valid UTF-8 when
// it succeeds; a decode yielding invalid UTF-8 would still count as a failure,
// since the decoder is outside this package. A raw fallback that is not valid
// UTF-8 is rendered as uppercase hex digits so the bytes stay recoverable.
// Hex is not re-decoded through a byte-to-codepoint map, which would produce
// plausible-looking wrong text.
//
// Indirect objects are not dereferenced; an indirect /Title or /UF yields "".
func textStringOrRaw(obj pdfcpu_types.Object) string {
	if s, ok := decodeTextStringOK(obj); ok && utf8.ValidString(s) {
		return s
	}
	raw := stringValue(obj)
	if !utf8.ValidString(raw) {
		return strings.ToUpper(hex.EncodeToString([]byte(raw)))
	}
	return raw
}
