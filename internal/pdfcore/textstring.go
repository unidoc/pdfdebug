package pdfcore

import (
	"encoding/hex"
	"strings"
	"unicode/utf8"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// PDF text-string decoding (ISO 32000-1 7.9.2.2), shared by every reader that
// surfaces a human-readable string field. Kept in its own file (rather than in
// validate.go, where it originated) so it reads as a general pdfcore helper
// instead of a validation-only one.

// decodeTextString decodes a PDF string object (StringLiteral or HexLiteral) to
// Go text via pdfcpu's StringOrHexLiteral. TEXT FIELDS ONLY: a binary-carrying
// string (filespec /Params /CheckSum, signature /Contents and /Cert, trailer
// /ID) must stay on stringValue - decoding it would corrupt the bytes.
//
// The semantics are pdfcpu's, and the two literal types are NOT symmetric:
//
//   - StringLiteral: unescape, then UTF-16BE when the bytes start with FE FF AND
//     the total length is even; otherwise strip a UTF-8 BOM and pass the bytes
//     through; otherwise (bytes are not valid UTF-8) apply a LATIN-1
//     byte-value-is-codepoint map.
//   - HexLiteral: hex-decode, then UTF-16BE under the same BOM + even-length
//     test; otherwise return the bytes VERBATIM. There is no encoding fallback
//     on this branch at all, so the result may be invalid UTF-8.
//
// The fallback is neither PDFDocEncoding (Annex D.2) nor CP1252: pdfcpu's
// CP1252ToUTF8 (types/string.go:218) is a misnamed identity byte->codepoint
// map, so 0x80 decodes to U+0080, not U+20AC (CP1252) and not U+2022 (PDFDoc).
// Accepting pdfcpu's behavior verbatim is deliberate - it is what the validate
// path already ships, and hand-rolling an encoding table is out of scope.
//
// Caveat (upstream defect, not ours): HexLiteralToString runs Unescape over the
// ALREADY hex-decoded bytes (utf16.go:156), so a 0x5C byte inside a hex text
// string is silently swallowed - HexLiteral("415C42") decodes to "AB".
//
// Returns "" for a non-string object or a decode failure (never guesses). A
// caller that RENDERS the field must use textStringOrRaw instead, so a decode
// failure cannot turn a present field into an absent one.
func decodeTextString(obj pdfcpu_types.Object) string {
	s, _ := decodeTextStringOK(obj)
	return s
}

// decodeTextStringOK is decodeTextString plus the bit decodeTextString throws
// away: whether the decode actually SUCCEEDED. The two are not the same
// question, because a text string may legitimately decode to the EMPTY string -
// HexLiteral("FEFF") is a well-formed, BOM-only UTF-16BE string whose value is
// "". Collapsing that into the ""-means-failure signal would make textStringOrRaw
// "rescue" it with the raw hex digits, i.e. render an empty /Title as the
// literal text "FEFF". ok is false only for a non-string object or a real
// decode error.
func decodeTextStringOK(obj pdfcpu_types.Object) (string, bool) {
	switch obj.(type) {
	case pdfcpu_types.StringLiteral, pdfcpu_types.HexLiteral:
		if s, err := pdfcpu_types.StringOrHexLiteral(obj); err == nil && s != nil {
			return *s, true
		}
	}
	return "", false
}

// textStringOrRaw decodes a PDF text string and NEVER DROPS A PRESENT FIELD:
// when obj is a StringLiteral/HexLiteral whose decode FAILED, it falls back to
// stringValue's raw rendering, so an undecodable /Title or /UF surfaces as
// visible mojibake rather than vanishing from the output. A non-string object
// still yields "". Never panics.
//
// The fallback is keyed on decodeTextStringOK's success bit, NOT on an empty
// result: a text string that decodes cleanly to "" (e.g. the BOM-only
// HexLiteral("FEFF")) IS empty, and must surface as empty rather than as its
// raw hex digits. Rescuing that case would invent a value - the same quiet lie
// the fallback exists to prevent, pointed the other way.
//
// The RESULT IS ALWAYS VALID UTF-8, on both branches. json.Marshal silently
// substitutes one U+FFFD per invalid byte, which destroys the value on the
// machine-readable surface, so neither a decode result nor a raw fallback may
// leave here un-validated:
//
//   - A decode that succeeds but yields invalid UTF-8 counts as a failure. Only
//     the HexLiteral branch can do this (non-UTF-16 bytes come back verbatim,
//     with no encoding fallback); the StringLiteral branch always ends in a
//     byte->codepoint map.
//
//   - A raw fallback that is not valid UTF-8 is rendered as uppercase HEX
//     DIGITS, keeping the bytes recoverable. Deliberately NOT re-decoded through
//     a byte->codepoint map: a Latin-1 reading of undecodable bytes is a
//     quietly-plausible wrong answer, and this epic exists to stop shipping
//     those. (This case is pre-existing, not introduced by story 14.4 -
//     stringValue fed json.Marshal the same invalid bytes before - but leaving
//     one branch of a freshly-added guard un-validated would be incoherent.)
//
//     Two honest limits on that hex rendering, neither a correctness bug: it is
//     NOT distinguishable from a genuine value that happens to be hex-looking
//     text, and it does not normalize every carrier of the same bytes - a
//     StringLiteral written with octal escapes, `(\376\377\334\000)`, has an
//     all-ASCII escaped body, so stringValue's rendering is already valid UTF-8
//     and is returned as that backslash-octal text rather than as hex digits.
//     Both forms are recoverable and visibly not prose, which is what matters.
//
// This is the entry point for display fields. decodeTextString's bare
// ""-on-failure contract is correct only where "" means "skip" (checkXMPMetadata).
//
// Caveat on "never drops a present field": that holds for a DIRECT string
// object. An INDIRECT /Title or /UF is not dereferenced here (a documented
// story-14.4 boundary), so it reaches the non-string branch and still yields "".
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
