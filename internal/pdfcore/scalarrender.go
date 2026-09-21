package pdfcore

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Scalar rendering for every display surface: the object tree (plain text and
// JSON), the object detail view and the structural diff. Two renderings of the
// same object live here, kept apart on purpose:
//
//   - scalarRaw is the byte-exact stored form, PDF delimiters included. It
//     drives the diff's changed/unchanged decision and ValueEntry.Raw.
//   - scalarText is what a reader sees: text strings decoded per ISO 32000-1
//     7.9.2.2 through textstring.go, every other scalar identical to scalarRaw.
//
// dump source keeps its own serializer (writeScalar, objectsource.go): it emits
// PDF syntax rather than rendering for a reader, and its bytes are frozen.

// TreeValueCap is the number of runes a plain-text tree row emits for a scalar
// value before it elides. Fixed rather than terminal-width-adaptive so piped
// output and CI output agree.
const TreeValueCap = 80

// scalarRaw renders a PDF scalar in its byte-exact stored form. The arms that
// can carry arbitrary bytes pass through utf8Safe, because every consumer of
// this rendering reaches a reader through JSON.
func scalarRaw(obj pdfcpu_types.Object) string {
	switch v := obj.(type) {
	case pdfcpu_types.Name:
		return utf8Safe("/" + string(v))
	case pdfcpu_types.StringLiteral:
		return utf8Safe("(" + string(v) + ")")
	case pdfcpu_types.HexLiteral:
		return utf8Safe("<" + string(v) + ">")
	case pdfcpu_types.Integer:
		return strconv.Itoa(int(v))
	case pdfcpu_types.Float:
		return strconv.FormatFloat(float64(v), 'f', -1, 64)
	case pdfcpu_types.Boolean:
		if bool(v) {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		return "Unknown"
	}
}

// scalarText renders a PDF scalar for a reader: a text string decoded, every
// other scalar as scalarRaw renders it. An empty decode keeps the delimited
// stored form ("()", "<>", "<FEFF>") so a value that exists never renders as
// the empty string, which an omitempty JSON key would drop - and so a
// genuinely empty string stays distinguishable from a BOM-only one.
func scalarText(obj pdfcpu_types.Object) string {
	if isStringObject(obj) {
		if s := textStringOrRaw(obj); s != "" {
			return s
		}
	}
	return scalarRaw(obj)
}

// isStringObject reports whether obj is one of the two PDF string literal types.
func isStringObject(obj pdfcpu_types.Object) bool {
	switch obj.(type) {
	case pdfcpu_types.StringLiteral, pdfcpu_types.HexLiteral:
		return true
	}
	return false
}

// utf8Safe returns s unchanged when it is valid UTF-8 and its bytes as
// uppercase hex otherwise. json.Marshal rewrites invalid bytes to U+FFFD,
// which destroys the value a raw rendering exists to preserve; hex keeps it
// recoverable. Same policy textStringOrRaw applies to a decoded text string.
func utf8Safe(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToUpper(hex.EncodeToString([]byte(s)))
}

// binaryStringSummary renders the fixed-width stand-in a binary-carrying string
// shows in place of its blob. The length is the binary payload in bytes - hex
// digits halved, or the unescaped length of a literal - not the length of the
// stored token.
func binaryStringSummary(obj pdfcpu_types.Object) string {
	return fmt.Sprintf("<binary, %d bytes>", binaryPayloadLen(obj))
}

func binaryPayloadLen(obj pdfcpu_types.Object) int {
	switch v := obj.(type) {
	case pdfcpu_types.HexLiteral:
		if b, err := v.Bytes(); err == nil {
			return len(b)
		}
		return len(string(v)) / 2
	case pdfcpu_types.StringLiteral:
		if b, err := pdfcpu_types.Unescape(string(v)); err == nil {
			return len(b)
		}
		return len(string(v))
	}
	return 0
}

// binaryStringKey reports whether a dictionary entry carries binary bytes that
// must never be decoded for display: a signature dictionary's /Contents and
// /Cert, and a filespec /Params /CheckSum. The match is by key name plus
// context, because a bare name match is wrong for /Contents - a markup
// annotation's /Contents is an ordinary text string and decodes like one.
//
// /CheckSum is matched on the name alone: the walker has no knowledge that the
// parent is a /Params dict, and no other PDF key is spelled CheckSum.
func binaryStringKey(parent pdfcpu_types.Dict, bareKey string) bool {
	switch bareKey {
	case "CheckSum":
		return true
	case "Contents", "Cert":
		return isSignatureDict(parent)
	}
	return false
}

// isSignatureDict reports whether d is a signature dictionary: /Type /Sig, or
// /Type /DocTimeStamp, whose /Contents carries the same DER a signature does,
// or a dictionary carrying /ByteRange. /ByteRange is checked whatever /Type
// says rather than only when it is absent, so a signature dictionary typed
// with a name this list does not know still keeps its bytes; no dictionary
// outside the signature family carries that key.
func isSignatureDict(d pdfcpu_types.Dict) bool {
	if d == nil {
		return false
	}
	if t, ok := d["Type"]; ok {
		if name, isName := t.(pdfcpu_types.Name); isName {
			switch string(name) {
			case "Sig", "DocTimeStamp":
				return true
			}
		}
	}
	_, hasByteRange := d["ByteRange"]
	return hasByteRange
}

// decodeChangedContent reports whether decoding a string object produced
// content different from its stored bytes with delimiters and escaping
// removed. The comparison is on content, not on the rendered forms: "(en-US)"
// is never string-equal to "en-US", so comparing renderings would answer yes
// for every string in the document.
func decodeChangedContent(obj pdfcpu_types.Object) bool {
	stored, ok := storedStringContent(obj)
	if !ok {
		// A string whose stored content cannot be recovered (malformed escape,
		// odd or non-hex digits) still gets a raw counterpart: the decode
		// fallback drops the delimiters, so without it the stored form would
		// be unreachable from the output.
		return isStringObject(obj)
	}
	return textStringOrRaw(obj) != stored
}

// storedStringContent returns a string object's stored bytes with its
// delimiters and escaping removed, and whether obj is a string at all.
func storedStringContent(obj pdfcpu_types.Object) (string, bool) {
	switch v := obj.(type) {
	case pdfcpu_types.HexLiteral:
		b, err := v.Bytes()
		if err != nil {
			return "", false
		}
		return string(b), true
	case pdfcpu_types.StringLiteral:
		b, err := pdfcpu_types.Unescape(string(v))
		if err != nil {
			return "", false
		}
		return string(b), true
	}
	return "", false
}

// EscapeDisplayValue renders a value's control characters as escape sequences
// so one value is always exactly one output line: \n, \r and \t by name, every
// other C0 control plus DEL and the whole C1 block as \xHH in uppercase hex,
// and a literal backslash doubled so the escaping stays reversible.
//
// C1 is not optional. The decoder's Latin-1 fallback turns a CP1252 curly
// apostrophe (0x92) into U+0092, which would otherwise reach the screen as an
// invisible character.
func EscapeDisplayValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteString(escapeRune(r))
	}
	return b.String()
}

// ClampDisplayValue escapes s and cuts the result to at most limit runes,
// appending "[truncated: N of M]" when it elides: N the runes emitted, M the
// runes the whole escaped value has. The cut lands on an escape-sequence
// boundary, so it never separates a backslash from its letter.
func ClampDisplayValue(s string, limit int) string {
	var b strings.Builder
	emitted, total := 0, 0
	cut := false
	for _, r := range s {
		u := escapeRune(r)
		n := utf8.RuneCountInString(u)
		total += n
		// Once one unit does not fit, no later unit may be emitted either:
		// skipping a wide unit to fit a narrow one behind it would reorder the
		// text. The walk continues only to finish counting total.
		if cut || emitted+n > limit {
			cut = true
			continue
		}
		b.WriteString(u)
		emitted += n
	}
	if !cut {
		return b.String()
	}
	return fmt.Sprintf("%s [truncated: %d of %d]", b.String(), emitted, total)
}

// escapeRune returns one display unit: the rune itself, or the escape sequence
// standing in for it. Cutting on unit boundaries is what keeps a two-rune
// escape sequence from being halved by the cap.
func escapeRune(r rune) string {
	switch {
	case r == '\n':
		return `\n`
	case r == '\r':
		return `\r`
	case r == '\t':
		return `\t`
	case r == '\\':
		return `\\`
	case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
		return fmt.Sprintf(`\x%02X`, r)
	default:
		return string(r)
	}
}
