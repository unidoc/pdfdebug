// Story 14-4 co-located unit tests for the shared PDF text-string decoder
// (AC 1, 2, 3, 4, 6). Authored red in the ATDD step; green since the dev step.
//
// Two layers live here, and the distinction is deliberate:
//
//  1. 14.4-UNIT-001 pins the SEMANTICS of the existing package-level decoder
//     (`decodeTextString`, relocated from validate.go to
//     textstring.go by Task 1). Those rows pass today and MUST keep passing:
//     they are the guard against "fixing" pdfcpu's Latin-1 fallback into a
//     CP1252 or PDFDocEncoding table, which AC1 explicitly forbids. The table
//     is the eight-row table mandated by AC6, verbatim, with the exact bytes
//     empirically produced by pdfcpu v0.12.1.
//
//  2. 14.4-UNIT-002..005 exercise the two NEW call sites through their
//     in-package entry points (`GetDocumentMetadata` -> collectInfoFields,
//     `GetEmbeddedFiles` -> embeddedFileFromFilespec). UNIT-002 and UNIT-003
//     were RED before 14.4: both readers returned raw `stringValue`, so a UTF-16BE
//     `/Title` / `/UF` surfaces as literal hex DIGITS instead of UTF-8.
//     UNIT-004 and UNIT-005 are boundary guards that pass today and must keep
//     passing: they fail the moment a dev wires `decodeTextString` directly
//     (dropping an undecodable field, AC1) or lets the decoder reach a binary
//     field (AC4).
//
// This file COMPILES against the current tree on purpose: while red, the
// failures came from wrong decoded values rather than missing symbols, so
// `go test ./...` stayed runnable for the whole main module throughout.
//
// Naming: 14.4-UNIT-NNN [Px] per the story Testing Requirements (AC6).
//
// Run: go test ./internal/pdfcore/ -run 'TextString|DecodeTextString' -v
package pdfcore

import (
	"strconv"
	"testing"
	"unicode/utf8"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// --- shared expectations ----------------------------------------------------

const (
	// utf16TitleHex is UTF-16BE-with-BOM for utf16TitleText. No 0x5C byte
	// (AC1: pdfcpu's HexLiteralToString silently swallows one).
	utf16TitleHex  = "FEFF0052006500630068006E0075006E006700200047007200F600DF006500204E2D6587"
	utf16TitleText = "Rechnung Größe 中文"

	// utf16UFHex is UTF-16BE-with-BOM for utf16UFText, likewise 0x5C-free.
	utf16UFHex  = "FEFF0067007200F600DF0065002D4E2D6587002E0078006D006C"
	utf16UFText = "größe-中文.xml"

	// undecodableHex is a BOM followed by an UNPAIRED LOW surrogate. Verified to
	// make decodeTextString return "" (Source verification #3). Do NOT swap in
	// "FEFF005469" ("BOM + odd trailing byte"): that one does NOT error.
	undecodableHex = "FEFFDC00"

	// rawCheckSumHex is the binary /Params /CheckSum guard value (AC4). It must
	// surface as these literal hex DIGITS, never as decoded bytes.
	rawCheckSumHex = "DEADBEEFCAFEF00D0011223344556677"
)

// --- fixture builders (uniquely named; the package already has 13-2 builders) -

// textStringEmbeddedStreamObj returns an /EmbeddedFile stream whose /Params
// carries a HEX-literal /CheckSum, so AC4's "binary hex field is not
// text-decoded" boundary can be asserted on the same dict as the changed
// fields. /Subtype must be a Name with #2F escapes or pdfcpu rejects the doc.
func textStringEmbeddedStreamObj(num int, payload string) string {
	return strconv.Itoa(num) + " 0 obj\n" +
		"<< /Type /EmbeddedFile /Subtype /text#2Fxml /Length " + strconv.Itoa(len(payload)) +
		" /Params << /Size " + strconv.Itoa(len(payload)) +
		" /CheckSum <" + rawCheckSumHex + "> /ModDate (D:20240101000000Z) >> >>\n" +
		"stream\n" + payload + "\nendstream\nendobj\n"
}

// textStringInfoPDF builds a doc whose trailer /Info (object 4) carries
// titleObj VERBATIM as the /Title value, plus ASCII control values. titleObj is
// written as-is, so callers pass either "(literal)" or "<HEX>". The /Title is a
// DIRECT string object: neither new call site dereferences (Source
// verification #6), so an indirect value would make the test prove nothing.
func textStringInfoPDF(titleObj string) []byte {
	return textStringInfoPDFKey("Title", titleObj)
}

// textStringInfoPDFKey is textStringInfoPDF with the carrying key chosen by the
// caller, so one table can exercise all six infoTextFields entries. The two date
// keys are always ASCII controls.
func textStringInfoPDFKey(key, valueObj string) []byte {
	// Every control key is emitted only when it is not the carrying key, so no
	// key is ever written twice (a duplicate would silently shadow the value
	// under test).
	info := "4 0 obj\n<< /" + key + " " + valueObj
	for _, ctrl := range [][2]string{
		{"CreationDate", "(D:20240101000000Z)"},
		{"ModDate", "(D:20240102000000Z)"},
		{"Author", "(ACME GmbH)"},
		{"Producer", "(pdfdebug-fixture)"},
	} {
		if key != ctrl[0] {
			info += " /" + ctrl[0] + " " + ctrl[1]
		}
	}
	info += " >>\nendobj\n"
	return assembleWithInfo([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		info,
	}, 1, 4)
}

// textStringFilespecPDF builds a doc with one attachment whose /Filespec
// (object 5) carries ufObj VERBATIM as /UF and an ASCII /F fallback. ufObj is a
// DIRECT string object for the same reason as textStringInfoPDF.
func textStringFilespecPDF(ufObj string) []byte {
	return assembleWithInfo([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [5 0 R] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		textStringEmbeddedStreamObj(4, "<?xml version=\"1.0\"?><Invoice/>"),
		"5 0 obj\n<< /Type /Filespec /F (groesse.xml) /UF " + ufObj +
			" /AFRelationship /Data /EF << /F 4 0 R /UF 4 0 R >> >>\nendobj\n",
	}, 1, 0)
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-001 [P1] AC1/AC6 (R-14-05): the shared decoder's per-literal-type
// semantics, asserted as EXACT bytes. This is the eight-row AC6 table.
//
// GREEN TODAY BY DESIGN. These rows pin what pdfcpu actually does so the dev
// cannot "improve" the fallback into CP1252 (U+20AC for 0x80) or PDFDocEncoding
// (U+2022 for 0x80) while relocating the helper. AC1 accepts pdfcpu as-is.
// ---------------------------------------------------------------------------

func TestDecodeTextString_SemanticsTable(t *testing.T) {
	tests := []struct {
		name string
		in   pdfcpu_types.Object
		want string
		why  string
	}{
		{
			name: "UTF-16BE CJK",
			in:   pdfcpu_types.HexLiteral("FEFF4E2D"),
			want: "中",
			why:  "BOM + even byte length routes through decodeUTF16String",
		},
		{
			name: "UTF-16BE accented Latin",
			in:   pdfcpu_types.HexLiteral("FEFF00E9"),
			want: "é",
			why:  "same UTF-16BE branch, BMP Latin-1 supplement codepoint",
		},
		{
			name: "high byte on StringLiteral takes the LATIN-1 fallback",
			in:   pdfcpu_types.StringLiteral("\x80"),
			want: "\u0080", // U+0080, written as an escape so the source stays ASCII
			why: "pdfcpu's CP1252ToUTF8 (types/string.go:218) is a misnamed " +
				"byte->codepoint identity map. NOT U+20AC (CP1252), NOT U+2022 (PDFDoc)",
		},
		{
			name: "high byte on HexLiteral gets NO fallback",
			in:   pdfcpu_types.HexLiteral("80"),
			want: "\x80",
			why: "HexLiteralToString (utf16.go:147) returns non-UTF16 bytes verbatim; " +
				"the result is deliberately invalid UTF-8",
		},
		{
			name: "plain ASCII StringLiteral is unchanged",
			in:   pdfcpu_types.StringLiteral("Invoice 2024-001"),
			want: "Invoice 2024-001",
			why:  "valid UTF-8, no BOM: passthrough with no fallback applied",
		},
		{
			name: "non-string object yields empty",
			in:   pdfcpu_types.Integer(42),
			want: "",
			why:  "the type switch never calls StringOrHexLiteral",
		},
		{
			name: "undecodable: BOM + unpaired low surrogate",
			in:   pdfcpu_types.HexLiteral(undecodableHex),
			want: "",
			why:  "decodeUTF16String errors; decodeTextString swallows it and returns \"\"",
		},
		{
			name: "undecodable: odd hex-digit count",
			in:   pdfcpu_types.HexLiteral("FEF"),
			want: "",
			why:  "encoding/hex: odd length hex string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeTextString(tc.in)
			if got != tc.want {
				t.Errorf("[P1] 14.4-UNIT-001: decodeTextString(%T %v) = %q (% x), want %q (% x)\nreason: %s",
					tc.in, tc.in, got, []byte(got), tc.want, []byte(tc.want), tc.why)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-001b [P1] AC1/AC6: the non-erroring lookalike. "BOM + odd trailing
// BYTE" is NOT an undecodable case -- IsUTF16BE rejects odd byte length, the
// UTF-16 branch is skipped, and the raw bytes come back. Pinned so nobody
// substitutes it for a real fallback case.
// ---------------------------------------------------------------------------

func TestDecodeTextString_BOMWithOddTrailingByteDoesNotError(t *testing.T) {
	got := decodeTextString(pdfcpu_types.HexLiteral("FEFF005469"))
	if got != "\xfe\xff\x00Ti" {
		t.Errorf("[P1] 14.4-UNIT-001b: got %q (% x), want the raw bytes \"\\xfe\\xff\\x00Ti\"",
			got, []byte(got))
	}
	if got == "" {
		t.Errorf("[P1] 14.4-UNIT-001b: this input must NOT produce \"\" -- do not use it as the AC1 fallback case")
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-002 [P1] AC2 (R-14-05): collectInfoFields decodes a UTF-16BE-with-
// BOM /Title into UTF-8.
//
// Before 14.4: metadata.go used stringValue, so a HexLiteral /Title surfaced
// as the literal hex DIGITS "FEFF0052...".
// ---------------------------------------------------------------------------

func TestCollectInfoFields_DecodesUTF16BETitle(t *testing.T) {
	ins, tabID := writeTempPDF(t, "info-utf16.pdf", textStringInfoPDF("<"+utf16TitleHex+">"))

	md, err := ins.GetDocumentMetadata(tabID)
	if err != nil {
		t.Fatalf("[P1] 14.4-UNIT-002: GetDocumentMetadata error: %v", err)
	}
	if got := md.Info["Title"]; got != utf16TitleText {
		t.Errorf("[P1] 14.4-UNIT-002: Info[\"Title\"] = %q, want %q (UTF-16BE-with-BOM must be decoded, not echoed as hex digits)",
			got, utf16TitleText)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-002b [P1] AC2: the ASCII /Info values and the two DATE keys are
// unchanged by the decode wiring. /CreationDate and /ModDate stay on
// stringValue (they are ASCII "D:YYYY..." and need no decode).
//
// GREEN TODAY: boundary guard against routing the whole infoFields list
// through the decoder.
//
// NOTE on the rationale (corrected at code review): the date keys stay on
// stringValue as a SCOPE decision, not because decoding would harm them. ISO
// 32000-1 7.9.4 defines a date AS a text string, so a UTF-16BE-with-BOM
// /ModDate is legal, and routing a conforming ASCII date through the decoder
// would be a harmless no-op. That is why the ASCII rows below cannot on their
// own detect a wrongly-routed date key - the non-ASCII row in
// TestCollectInfoFields_NonASCIIDateStaysRaw is what actually pins the boundary.
// ---------------------------------------------------------------------------

func TestCollectInfoFields_ASCIIAndDatesUnchanged(t *testing.T) {
	ins, tabID := writeTempPDF(t, "info-ascii.pdf", textStringInfoPDF("(Invoice 2024-001)"))

	md, err := ins.GetDocumentMetadata(tabID)
	if err != nil {
		t.Fatalf("[P1] 14.4-UNIT-002b: GetDocumentMetadata error: %v", err)
	}
	for key, want := range map[string]string{
		"Title":        "Invoice 2024-001",
		"Author":       "ACME GmbH",
		"Producer":     "pdfdebug-fixture",
		"CreationDate": "D:20240101000000Z",
		"ModDate":      "D:20240102000000Z",
	} {
		if got := md.Info[key]; got != want {
			t.Errorf("[P1] 14.4-UNIT-002b: Info[%q] = %q, want %q (decode must be a no-op on ASCII)", key, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-003 [P1] AC3 (R-14-05): embeddedFileFromFilespec decodes the
// filespec /UF display name, keeping /UF-preferred-else-/F precedence.
//
// Before 14.4: embedded.go used stringValue, so /UF surfaced as hex digits.
// ---------------------------------------------------------------------------

func TestEmbeddedFileName_DecodesUTF16BEUF(t *testing.T) {
	ins, tabID := writeTempPDF(t, "filespec-utf16.pdf", textStringFilespecPDF("<"+utf16UFHex+">"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("[P1] 14.4-UNIT-003: GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("[P1] 14.4-UNIT-003: expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].Name; got != utf16UFText {
		t.Errorf("[P1] 14.4-UNIT-003: Name = %q, want %q (/UF must be decoded and must still win over the ASCII /F)",
			got, utf16UFText)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-004 [P1] AC1 (R-14-05): NEVER trade mojibake for absence. A display
// field whose decode FAILS must still be present, falling back to stringValue's
// raw rendering -- decodeTextString's ""-on-failure contract would otherwise
// delete the row via the `v != ""` write guard at metadata.go:77 (and empty the
// name at embedded.go:149).
//
// The guard is built on the filespec /UF, not on /Info /Title, for a hard
// reason verified during ATDD authoring: pdfcpu's reader VALIDATES /Info text
// strings, so a document carrying `/Title <FEFFDC00>` is rejected outright by
// ReadContextFile ("decodeUTF16String: corrupt UTF16BE byte length") and never
// reaches collectInfoFields. The filespec /UF is not validated that way, so it
// is the only reachable surface for this contract. The wrapper is shared, so
// pinning it here pins it for both call sites.
//
// GREEN TODAY (stringValue already yields the raw digits) and it must STAY
// green: it fails the moment the dev wires decodeTextString directly.
// An undecodable /UF must not empty the Name and must not fall through to /F --
// /UF is present, so /UF wins, raw.
// ---------------------------------------------------------------------------

func TestEmbeddedFileName_UndecodableUFFallsBackToRaw(t *testing.T) {
	ins, tabID := writeTempPDF(t, "filespec-undecodable.pdf", textStringFilespecPDF("<"+undecodableHex+">"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("[P1] 14.4-UNIT-004: GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("[P1] 14.4-UNIT-004: expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].Name; got != undecodableHex {
		t.Errorf("[P1] 14.4-UNIT-004: Name = %q, want the raw rendering %q (an undecodable /UF must not empty the name nor silently fall through to /F)",
			got, undecodableHex)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-004b [P1] AC1 (R-14-05): the OTHER side of the raw fallback. An
// EMPTY text string is not an undecodable one. HexLiteral("FEFF") is a
// well-formed, BOM-only UTF-16BE string that decodes cleanly to "" with no
// error, so the wrapper must return "" -- not stringValue's raw hex digits
// "FEFF". Keying the fallback on `decoded != ""` instead of on the decode's
// success bit would render an empty /Title as the literal text "FEFF": a
// fabricated value, the same quiet lie AC1's fallback exists to prevent.
// ---------------------------------------------------------------------------

func TestTextStringOrRaw_EmptyDecodeIsNotAFailure(t *testing.T) {
	tests := []struct {
		name string
		in   pdfcpu_types.Object
	}{
		{"BOM-only HexLiteral", pdfcpu_types.HexLiteral("FEFF")},
		{"BOM-only StringLiteral", pdfcpu_types.StringLiteral("\xfe\xff")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Precondition: this input decodes SUCCESSFULLY to the empty string.
			if _, ok := decodeTextStringOK(tc.in); !ok {
				t.Fatalf("[P1] 14.4-UNIT-004b: %T %v was expected to decode without error", tc.in, tc.in)
			}
			if got := textStringOrRaw(tc.in); got != "" {
				t.Errorf("[P1] 14.4-UNIT-004b: textStringOrRaw(%T %v) = %q (% x), want \"\" -- an empty text string must not be rescued into its raw rendering",
					tc.in, tc.in, got, []byte(got))
			}
		})
	}

	// The failure path is unchanged: a real decode error still falls back to raw.
	if got := textStringOrRaw(pdfcpu_types.HexLiteral(undecodableHex)); got != undecodableHex {
		t.Errorf("[P1] 14.4-UNIT-004b: undecodable input = %q, want the raw rendering %q (the AC1 fallback must survive)",
			got, undecodableHex)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-005 [P1] AC4 (R-14-06): the binary boundary. /Params /CheckSum is a
// HEX literal carrying binary bytes and must stay RAW (the literal hex digits);
// /Params /ModDate is an ASCII date and stays verbatim. Both live in the SAME
// dict family as the fields being changed, so this pins the exact boundary.
//
// GREEN TODAY and must stay green: it fails if the decoder leaks past the
// enumerated text keys.
// ---------------------------------------------------------------------------

func TestEmbeddedFileParams_BinaryCheckSumStaysRaw(t *testing.T) {
	ins, tabID := writeTempPDF(t, "filespec-binary.pdf", textStringFilespecPDF("<"+utf16UFHex+">"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("[P1] 14.4-UNIT-005: GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("[P1] 14.4-UNIT-005: expected 1 embedded file, got %d", len(list.Files))
	}
	f := list.Files[0]
	if f.CheckSum != rawCheckSumHex {
		t.Errorf("[P1] 14.4-UNIT-005: CheckSum = %q, want the raw hex digits %q (a binary field must NOT be routed through the text decoder)",
			f.CheckSum, rawCheckSumHex)
	}
	if f.ModDate != "D:20240101000000Z" {
		t.Errorf("[P1] 14.4-UNIT-005: ModDate = %q, want the verbatim ASCII date", f.ModDate)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-006 [P1] AC4: stringValue survives as the RAW renderer. It must NOT
// be deleted or re-pointed at the decoder -- /CheckSum, /ModDate, catalog /Lang
// and the five deferred signature text fields still depend on it.
// ---------------------------------------------------------------------------

func TestStringValue_StaysRaw(t *testing.T) {
	if got := stringValue(pdfcpu_types.HexLiteral(utf16TitleHex)); got != utf16TitleHex {
		t.Errorf("[P1] 14.4-UNIT-006: stringValue(HexLiteral) = %q, want the hex digits %q verbatim", got, utf16TitleHex)
	}
	if got := stringValue(pdfcpu_types.StringLiteral("\x80")); got != "\x80" {
		t.Errorf("[P1] 14.4-UNIT-006: stringValue(StringLiteral) = %q, want the raw byte verbatim", got)
	}
	if got := stringValue(pdfcpu_types.Integer(42)); got != "" {
		t.Errorf("[P1] 14.4-UNIT-006: stringValue(non-string) = %q, want \"\"", got)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-007 [P1] AC1/AC5: a decode that SUCCEEDS but yields invalid UTF-8
// must be treated as a failure and fall back to the raw rendering.
//
// Only the HexLiteral branch can produce this: it returns non-UTF-16 bytes
// verbatim with no encoding fallback (the StringLiteral branch always ends in a
// byte->codepoint map, so it is always valid UTF-8). Without the guard the bad
// bytes reach json.Marshal, which silently substitutes one U+FFFD per invalid
// byte -- destroying the value on the MACHINE-READABLE surface, where the raw
// hex rendering it replaced was at least recoverable.
// ---------------------------------------------------------------------------

func TestTextStringOrRaw_InvalidUTF8FallsBackToRaw(t *testing.T) {
	const badHex = "80CAFE" // not UTF-16BE (no BOM), not valid UTF-8

	// Precondition: pdfcpu reports SUCCESS here, returning the bytes verbatim.
	decoded, ok := decodeTextStringOK(pdfcpu_types.HexLiteral(badHex))
	if !ok {
		t.Fatalf("[P1] 14.4-UNIT-007: HexLiteral(%q) was expected to decode without error", badHex)
	}
	if utf8.ValidString(decoded) {
		t.Fatalf("[P1] 14.4-UNIT-007: HexLiteral(%q) decoded to valid UTF-8 %q; this case no longer exercises the guard",
			badHex, decoded)
	}

	got := textStringOrRaw(pdfcpu_types.HexLiteral(badHex))
	if got != badHex {
		t.Errorf("[P1] 14.4-UNIT-007: textStringOrRaw(HexLiteral(%q)) = %q (% x), want the raw rendering %q -- invalid UTF-8 must not reach the JSON surface",
			badHex, got, []byte(got), badHex)
	}
	if !utf8.ValidString(got) {
		t.Errorf("[P1] 14.4-UNIT-007: result %q is still invalid UTF-8; json.Marshal would replace it with U+FFFD", got)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-008 [P1] AC2: ALL SIX enumerated /Info text keys decode, not just
// /Title. infoTextFields is a hand-maintained subset of infoFields, so without
// this table dropping a key from the map leaves every other test green.
// ---------------------------------------------------------------------------

func TestCollectInfoFields_AllSixTextKeysDecode(t *testing.T) {
	for _, key := range []string{"Title", "Author", "Subject", "Keywords", "Creator", "Producer"} {
		t.Run(key, func(t *testing.T) {
			if !infoTextFields[key] {
				t.Fatalf("[P1] 14.4-UNIT-008: %q is missing from infoTextFields", key)
			}
			ins, tabID := writeTempPDF(t, "info-utf16-"+key+".pdf",
				textStringInfoPDFKey(key, "<"+utf16TitleHex+">"))
			md, err := ins.GetDocumentMetadata(tabID)
			if err != nil {
				t.Fatalf("[P1] 14.4-UNIT-008: GetDocumentMetadata error: %v", err)
			}
			if got := md.Info[key]; got != utf16TitleText {
				t.Errorf("[P1] 14.4-UNIT-008: Info[%q] = %q, want %q", key, got, utf16TitleText)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-009 [P1] AC4 (R-14-06): the tree renderer boundary. scalarDisplay
// is the raw renderer AC4 names FIRST, and diff.go:501/550 reuses it for the
// 13-6 structural diff. It has no key context, so it cannot tell a text /Title
// from a binary /ID -- routing it through the decoder is the story's headline
// failure mode. Pin that the very bytes the readers now decode still render as
// raw hex here.
//
// (Deviation: the story's AC6 asked for this guard at the `dump tree` CLI level.
// The CLI tree view exposes scalarDisplay only for ARRAY-ELEMENT scalars -- keyed
// dict values render as a type label ("CheckSum string"), not a value -- and the
// fixture has no hex string in an array position, so the guard lives here at the
// unit level instead. Record at trace time.)
// ---------------------------------------------------------------------------

func TestScalarDisplay_TextBytesStillRenderRaw(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   pdfcpu_types.Object
		want string
	}{
		{"UTF-16BE title hex", pdfcpu_types.HexLiteral(utf16TitleHex), "<" + utf16TitleHex + ">"},
		{"undecodable hex", pdfcpu_types.HexLiteral(undecodableHex), "<" + undecodableHex + ">"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := scalarDisplay(tc.in); got != tc.want {
				t.Errorf("[P1] 14.4-UNIT-009: scalarDisplay = %q, want %q (the tree renderer must NOT be text-decoded)",
					got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-007b [P1] AC1/AC5: the OTHER branch of the UTF-8 guard. A
// StringLiteral carrying raw bytes that FAIL to decode reaches the raw fallback,
// and stringValue hands back those same invalid bytes - so without a check the
// fallback path re-opens the U+FFFD hole that UNIT-007 closes on the decode
// path. UNIT-007 alone cannot catch this: it uses a HexLiteral, whose raw
// rendering is ASCII hex digits by construction, so it is green for the wrong
// reason.
//
// This input is PRE-EXISTING behavior, not a 14.4 regression (stringValue fed
// json.Marshal the same bytes at c69af69, verified). The guard exists so that
// textStringOrRaw's result is valid UTF-8 on EVERY branch.
// ---------------------------------------------------------------------------

func TestTextStringOrRaw_RawFallbackIsAlwaysValidUTF8(t *testing.T) {
	// BOM + unpaired low surrogate, carried as a literal rather than as hex.
	in := pdfcpu_types.StringLiteral("\xfe\xff\xdc\x00")

	if _, ok := decodeTextStringOK(in); ok {
		t.Fatalf("[P1] 14.4-UNIT-007b: this input was expected to FAIL to decode")
	}
	if utf8.ValidString(stringValue(in)) {
		t.Fatalf("[P1] 14.4-UNIT-007b: stringValue already returns valid UTF-8; this case no longer exercises the guard")
	}

	got := textStringOrRaw(in)
	if !utf8.ValidString(got) {
		t.Errorf("[P1] 14.4-UNIT-007b: textStringOrRaw = %q (% x), which is invalid UTF-8 - json.Marshal would replace it with U+FFFD",
			got, []byte(got))
	}
	if got != "FEFFDC00" {
		t.Errorf("[P1] 14.4-UNIT-007b: textStringOrRaw = %q, want the uppercase hex digits \"FEFFDC00\" (undecodable input must render the same shape whichever literal type carried it)",
			got)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-008b [P1] AC2/AC4: infoTextFields exhaustiveness in the direction
// that actually matters. UNIT-008 asserts the six known keys ARE in the map;
// this asserts the CONVERSE - that every infoFields member is either a known
// date key or a decoded text key. Without it, a text key added to infoFields
// alone would silently fall into collectInfoFields' else branch, whose comment
// claims the value is a date.
// ---------------------------------------------------------------------------

func TestInfoTextFields_CoversEveryNonDateInfoKey(t *testing.T) {
	dateKeys := map[string]bool{"CreationDate": true, "ModDate": true}
	for _, key := range infoFields {
		if dateKeys[key] || infoTextFields[key] {
			continue
		}
		t.Errorf("[P1] 14.4-UNIT-008b: infoFields key %q is neither a known date key nor a member of infoTextFields, so it silently renders RAW while collectInfoFields' else branch claims it is a date. Add it to infoTextFields or to this test's dateKeys set.",
			key)
	}
	if len(infoTextFields)+len(dateKeys) != len(infoFields) {
		t.Errorf("[P1] 14.4-UNIT-008b: infoTextFields(%d) + dateKeys(%d) != infoFields(%d); the sets have drifted",
			len(infoTextFields), len(dateKeys), len(infoFields))
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-003b [P2] AC3: records a PRECEDENCE change that AC3's "precedence is
// unchanged" claim does not cover. A BOM-only /UF <FEFF> decodes cleanly to ""
// (UNIT-004b), so it now falls through to /F. Before 14.4 the same input
// rendered as the raw digits "FEFF" - non-empty - and /UF won.
//
// The new behavior is the correct one (an empty text string IS empty, and a
// fabricated "FEFF" display name is the quiet lie AC1 forbids), but it is a
// user-visible change and was shipping unpinned.
// ---------------------------------------------------------------------------

func TestEmbeddedFileName_EmptyUFFallsThroughToF(t *testing.T) {
	ins, tabID := writeTempPDF(t, "filespec-empty-uf.pdf", textStringFilespecPDF("<FEFF>"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("[P2] 14.4-UNIT-003b: GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("[P2] 14.4-UNIT-003b: expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].Name; got != "groesse.xml" {
		t.Errorf("[P2] 14.4-UNIT-003b: Name = %q, want the /F value \"groesse.xml\" (an empty /UF must defer to /F, not render as \"FEFF\")",
			got)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-002c [P2] AC2 boundary: the date keys are FALSIFIABLY off the
// decoder. 14.4-UNIT-002b's values are all printable ASCII, where decoding is a
// no-op, so it passes whether or not the date keys are routed through the
// decoder - the guard it advertises does not exist. This row does: a UTF-16BE-
// with-BOM /ModDate WOULD decode to readable text, so it stays as the raw hex
// digits only while the date keys remain on stringValue.
//
// This also pins the current DEFERRED behavior (see deferred-work.md): a
// <FEFF...> date renders as a hex dump. If that debt is ever closed, this test
// is the one to update, deliberately.
// ---------------------------------------------------------------------------

func TestCollectInfoFields_NonASCIIDateStaysRaw(t *testing.T) {
	// UTF-16BE-with-BOM for "D:20240101000000Z".
	const utf16DateHex = "FEFF0044003A00320030003200340030003100300031003000300030003000300030005A"

	ins, tabID := writeTempPDF(t, "info-utf16-date.pdf", textStringInfoPDFKey("ModDate", "<"+utf16DateHex+">"))
	md, err := ins.GetDocumentMetadata(tabID)
	if err != nil {
		t.Fatalf("[P2] 14.4-UNIT-002c: GetDocumentMetadata error: %v", err)
	}
	if got := md.Info["ModDate"]; got != utf16DateHex {
		t.Errorf("[P2] 14.4-UNIT-002c: Info[\"ModDate\"] = %q, want the raw hex digits %q - the date keys must stay on stringValue (AC2 scope boundary)",
			got, utf16DateHex)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-005b [P1] AC4 (R-14-06): a FALSIFIABLE binary-boundary guard.
//
// 14.4-UNIT-005 and 14.4-INTG-004 both assert the fixture's /Params /CheckSum
// equals its literal hex digits, and both claim to fail "the moment the decoder
// leaks past the enumerated text keys". They do not. Verified against pdfcpu
// v0.12.1: HexLiteral("DEADBEEFCAFEF00D...") decodes SUCCESSFULLY to bytes that
// are invalid UTF-8, so textStringOrRaw's own guard rejects the decode and falls
// back to the same hex digits - wiring /CheckSum through the wrapper leaves all
// of those assertions green.
//
// A checksum whose bytes happen to begin FE FF is the case that separates them:
// HexLiteral("FEFF0041") decodes cleanly to "A". So this test fails loudly if
// /Params /CheckSum is ever routed through textStringOrRaw OR decodeTextString.
// ---------------------------------------------------------------------------

func TestEmbeddedFileCheckSum_BOMPrefixedBinaryStaysRaw(t *testing.T) {
	const bomCheckSum = "FEFF0041" // decodes to "A" if text-decoded

	// Precondition: this value really does decode to something different.
	if decoded, ok := decodeTextStringOK(pdfcpu_types.HexLiteral(bomCheckSum)); !ok || decoded == bomCheckSum {
		t.Fatalf("[P1] 14.4-UNIT-005b: HexLiteral(%q) must decode to a DIFFERENT value for this guard to be falsifiable; got %q ok=%v",
			bomCheckSum, decoded, ok)
	}

	pdf := assembleWithInfo([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [5 0 R] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Type /EmbeddedFile /Subtype /text#2Fxml /Length 5" +
			" /Params << /Size 5 /CheckSum <" + bomCheckSum + "> /ModDate (D:20240101000000Z) >> >>\n" +
			"stream\nhello\nendstream\nendobj\n",
		"5 0 obj\n<< /Type /Filespec /F (plain.xml) /UF (plain.xml)" +
			" /AFRelationship /Data /EF << /F 4 0 R /UF 4 0 R >> >>\nendobj\n",
	}, 1, 0)

	ins, tabID := writeTempPDF(t, "checksum-bom.pdf", pdf)
	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("[P1] 14.4-UNIT-005b: GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("[P1] 14.4-UNIT-005b: expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].CheckSum; got != bomCheckSum {
		t.Errorf("[P1] 14.4-UNIT-005b: CheckSum = %q, want the raw hex digits %q - a binary field must NOT be text-decoded even when its bytes look like a UTF-16BE BOM",
			got, bomCheckSum)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-003c [P1] AC3 (R-14-05): the /F decode branch, which AC3 names as a
// call site but which no other scenario pins.
//
// Every existing filespec test carries a /UF, so the else-branch that decodes
// /F never runs under assertion. Verified during the trace step: reverting
// embedded.go's `textStringOrRaw(fs["F"])` to `stringValue(fs["F"])` left all 30
// scenarios green. A /F-only filespec (legal - /UF is optional) is the only
// shape that reaches it.
// ---------------------------------------------------------------------------

func TestEmbeddedFileName_FOnlyFilespecIsDecoded(t *testing.T) {
	pdf := assembleWithInfo([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [5 0 R] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		textStringEmbeddedStreamObj(4, "<?xml version=\"1.0\"?><Invoice/>"),
		// No /UF at all, so the /F branch is the one that runs.
		"5 0 obj\n<< /Type /Filespec /F <" + utf16UFHex + ">" +
			" /AFRelationship /Data /EF << /F 4 0 R /UF 4 0 R >> >>\nendobj\n",
	}, 1, 0)

	ins, tabID := writeTempPDF(t, "filespec-f-only.pdf", pdf)
	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("[P1] 14.4-UNIT-003c: GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("[P1] 14.4-UNIT-003c: expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].Name; got != utf16UFText {
		t.Errorf("[P1] 14.4-UNIT-003c: Name = %q, want %q - a /F-only filespec must decode /F, not echo its hex digits",
			got, utf16UFText)
	}
}

// ---------------------------------------------------------------------------
// 14.4-UNIT-005c [P1] AC3/AC4 (R-14-06): the filespec /Params /ModDate binary-
// boundary guard, and the last raw field in that dict with no falsifiable pin.
//
// AC3 and AC4 name THREE fields that must stay on stringValue: /Params
// /CheckSum, /Params /ModDate, and the /Info date keys. Two of the three got a
// falsifiable guard (14.4-UNIT-005b for /CheckSum, 14.4-UNIT-002c for /Info
// /ModDate); the filespec /ModDate did not. 14.4-UNIT-005's ModDate assertion
// is the same tautology those two replaced: its value is the printable-ASCII
// "D:20240101000000Z", where decoding is a no-op, so it passes whether or not
// the field is routed through the decoder. Verified by mutation - swapping
// embedded.go's `stringValue(params["ModDate"])` for either textStringOrRaw OR
// decodeTextString left all 31 scenarios green.
//
// A date whose bytes carry a UTF-16BE BOM separates them: ISO 32000-1 7.9.4
// defines a date AS a text string, so <FEFF0044003A...> is legal and decodes
// cleanly to readable "D:..." text. Asserting it stays as hex digits therefore
// fails loudly the moment /Params /ModDate reaches either decoder, and doubles
// as the pin for the deferred date-decode debt (change it deliberately if that
// debt is ever closed).
// ---------------------------------------------------------------------------

func TestEmbeddedFileModDate_BOMPrefixedDateStaysRaw(t *testing.T) {
	// UTF-16BE-with-BOM for "D:20240101000000Z".
	const utf16ModDateHex = "FEFF0044003A00320030003200340030003100300031003000300030003000300030005A"
	const utf16ModDateText = "D:20240101000000Z"

	// Precondition: this value really does decode to something different, so a
	// leak into either decoder cannot pass unnoticed.
	if decoded, ok := decodeTextStringOK(pdfcpu_types.HexLiteral(utf16ModDateHex)); !ok || decoded != utf16ModDateText {
		t.Fatalf("[P1] 14.4-UNIT-005c: HexLiteral(%q) must decode to %q for this guard to be falsifiable; got %q ok=%v",
			utf16ModDateHex, utf16ModDateText, decoded, ok)
	}

	pdf := assembleWithInfo([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [5 0 R] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Type /EmbeddedFile /Subtype /text#2Fxml /Length 5" +
			" /Params << /Size 5 /CheckSum <" + rawCheckSumHex + "> /ModDate <" + utf16ModDateHex + "> >> >>\n" +
			"stream\nhello\nendstream\nendobj\n",
		"5 0 obj\n<< /Type /Filespec /F (plain.xml) /UF (plain.xml)" +
			" /AFRelationship /Data /EF << /F 4 0 R /UF 4 0 R >> >>\nendobj\n",
	}, 1, 0)

	ins, tabID := writeTempPDF(t, "moddate-bom.pdf", pdf)
	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("[P1] 14.4-UNIT-005c: GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("[P1] 14.4-UNIT-005c: expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].ModDate; got != utf16ModDateHex {
		t.Errorf("[P1] 14.4-UNIT-005c: ModDate = %q, want the raw hex digits %q - filespec /Params /ModDate must stay on stringValue even when its bytes decode as a UTF-16BE text string",
			got, utf16ModDateHex)
	}
}
