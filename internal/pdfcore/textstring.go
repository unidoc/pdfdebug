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
// The semantics are pdfcpu's, and the two literal types differ:
//
//   - StringLiteral: unescape, then UTF-16BE if the bytes start FE FF and the
//     length is even; else strip a UTF-8 BOM and pass through; else map each
//     byte to the codepoint of the same value (Latin-1, despite pdfcpu naming
//     that function CP1252ToUTF8 - 0x80 decodes to U+0080, not U+20AC).
//   - HexLiteral: hex-decode, then UTF-16BE under the same test; else return the
//     bytes verbatim. No encoding fallback, so the result may be invalid UTF-8.
//
// Upstream quirk: HexLiteralToString unescapes the already-decoded bytes, so a
// 0x5C byte inside a hex text string is dropped - HexLiteral("415C42") gives
// "AB".
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
// U+FFFD, which destroys the value. A decode yielding invalid UTF-8 counts as a
// failure, and a raw fallback that is not valid UTF-8 is rendered as uppercase
// hex digits so the bytes stay recoverable. Hex is not re-decoded through a
// byte-to-codepoint map, which would produce plausible-looking wrong text.
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
