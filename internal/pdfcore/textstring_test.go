// Unit tests for the shared PDF text-string decoder and its two reader call
// sites (collectInfoFields, embeddedFileFromFilespec).
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
	// (pdfcpu's HexLiteralToString silently swallows one).
	utf16TitleHex  = "FEFF0052006500630068006E0075006E006700200047007200F600DF006500204E2D6587"
	utf16TitleText = "Rechnung Größe 中文"

	// utf16UFHex is UTF-16BE-with-BOM for utf16UFText, likewise 0x5C-free.
	utf16UFHex  = "FEFF0067007200F600DF0065002D4E2D6587002E0078006D006C"
	utf16UFText = "größe-中文.xml"

	// undecodableHex is a BOM followed by an unpaired low surrogate, which makes
	// decodeTextString return "". "FEFF005469" does NOT error - do not swap it in.
	undecodableHex = "FEFFDC00"

	// rawCheckSumHex is the binary /Params /CheckSum guard value: it must surface
	// as these literal hex digits, never as decoded bytes.
	rawCheckSumHex = "DEADBEEFCAFEF00D0011223344556677"
)

// --- fixture builders (uniquely named; the package already has embedded-data builders) -

// textStringEmbeddedStreamObj returns an /EmbeddedFile stream whose /Params
// carries a hex-literal /CheckSum, so the binary boundary can be asserted on the
// same dict as the decoded fields. /Subtype must use #2F escapes or pdfcpu
// rejects the doc.
func textStringEmbeddedStreamObj(num int, payload string) string {
	return strconv.Itoa(num) + " 0 obj\n" +
		"<< /Type /EmbeddedFile /Subtype /text#2Fxml /Length " + strconv.Itoa(len(payload)) +
		" /Params << /Size " + strconv.Itoa(len(payload)) +
		" /CheckSum <" + rawCheckSumHex + "> /ModDate (D:20240101000000Z) >> >>\n" +
		"stream\n" + payload + "\nendstream\nendobj\n"
}

// textStringInfoPDF builds a doc whose trailer /Info carries titleObj verbatim
// as /Title, plus ASCII controls. Callers pass "(literal)" or "<HEX>". The value
// is a DIRECT object: the call sites do not dereference, so an indirect one
// would prove nothing.
func textStringInfoPDF(titleObj string) []byte {
	return textStringInfoPDFKey("Title", titleObj)
}

// textStringInfoPDFKey is textStringInfoPDF with the carrying key chosen by the
// caller, so one table can exercise every infoTextFields entry.
func textStringInfoPDFKey(key, valueObj string) []byte {
	// A control key is emitted only when it is not the carrying key; a duplicate
	// would shadow the value under test.
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
// carries ufObj verbatim as /UF, with an ASCII /F fallback. Direct object, for
// the same reason as textStringInfoPDF.
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

// The decoder's per-literal-type semantics as
// exact bytes. Pins what pdfcpu does so the Latin-1 fallback is not "fixed"
// into a CP1252 or PDFDocEncoding table.

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
				t.Errorf("decodeTextString(%T %v) = %q (% x), want %q (% x)\nreason: %s",
					tc.in, tc.in, got, []byte(got), tc.want, []byte(tc.want), tc.why)
			}
		})
	}
}

// "BOM + odd trailing byte" does NOT error - it
// skips the UTF-16 branch and returns non-empty bytes. Not a fallback case.

func TestDecodeTextString_BOMWithOddTrailingByteDoesNotError(t *testing.T) {
	got := decodeTextString(pdfcpu_types.HexLiteral("FEFF005469"))
	if got != "\xfe\xff\x00Ti" {
		t.Errorf("got %q (% x), want the raw bytes \"\\xfe\\xff\\x00Ti\"",
			got, []byte(got))
	}
	if got == "" {
		t.Errorf("this input must NOT produce \"\" -- do not use it as the fallback case")
	}
}

// CollectInfoFields decodes a UTF-16BE-with-BOM /Title.

func TestCollectInfoFields_DecodesUTF16BETitle(t *testing.T) {
	ins, tabID := writeTempPDF(t, "info-utf16.pdf", textStringInfoPDF("<"+utf16TitleHex+">"))

	md, err := ins.GetDocumentMetadata(tabID)
	if err != nil {
		t.Fatalf("GetDocumentMetadata error: %v", err)
	}
	if got := md.Info["Title"]; got != utf16TitleText {
		t.Errorf("Info[\"Title\"] = %q, want %q (UTF-16BE-with-BOM must be decoded, not echoed as hex digits)",
			got, utf16TitleText)
	}
}

// ASCII /Info values and the date keys are unchanged.
// The ASCII values alone cannot detect a wrongly-routed date key (decoding them
// is a no-op); TestCollectInfoFields_NonASCIIDateStaysRaw is the falsifiable guard.

func TestCollectInfoFields_ASCIIAndDatesUnchanged(t *testing.T) {
	ins, tabID := writeTempPDF(t, "info-ascii.pdf", textStringInfoPDF("(Invoice 2024-001)"))

	md, err := ins.GetDocumentMetadata(tabID)
	if err != nil {
		t.Fatalf("GetDocumentMetadata error: %v", err)
	}
	for key, want := range map[string]string{
		"Title":        "Invoice 2024-001",
		"Author":       "ACME GmbH",
		"Producer":     "pdfdebug-fixture",
		"CreationDate": "D:20240101000000Z",
		"ModDate":      "D:20240102000000Z",
	} {
		if got := md.Info[key]; got != want {
			t.Errorf("Info[%q] = %q, want %q (decode must be a no-op on ASCII)", key, got, want)
		}
	}
}

// EmbeddedFileFromFilespec decodes a UTF-16BE /UF and
// does not fall through to the ASCII /F.

func TestEmbeddedFileName_DecodesUTF16BEUF(t *testing.T) {
	ins, tabID := writeTempPDF(t, "filespec-utf16.pdf", textStringFilespecPDF("<"+utf16UFHex+">"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].Name; got != utf16UFText {
		t.Errorf("Name = %q, want %q (/UF must be decoded and must still win over the ASCII /F)",
			got, utf16UFText)
	}
}

// An undecodable /UF surfaces as raw bytes rather than
// emptying and deferring to /F.

func TestEmbeddedFileName_UndecodableUFFallsBackToRaw(t *testing.T) {
	ins, tabID := writeTempPDF(t, "filespec-undecodable.pdf", textStringFilespecPDF("<"+undecodableHex+">"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].Name; got != undecodableHex {
		t.Errorf("Name = %q, want the raw rendering %q (an undecodable /UF must not empty the name nor silently fall through to /F)",
			got, undecodableHex)
	}
}

// An empty decode is not a failure. HexLiteral("FEFF")
// is a well-formed BOM-only string worth "", so it must not be rescued into the
// raw hex digits.

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
				t.Fatalf("%T %v was expected to decode without error", tc.in, tc.in)
			}
			if got := textStringOrRaw(tc.in); got != "" {
				t.Errorf("textStringOrRaw(%T %v) = %q (% x), want \"\" -- an empty text string must not be rescued into its raw rendering",
					tc.in, tc.in, got, []byte(got))
			}
		})
	}

	// The failure path is unchanged: a real decode error still falls back to raw.
	if got := textStringOrRaw(pdfcpu_types.HexLiteral(undecodableHex)); got != undecodableHex {
		t.Errorf("undecodable input = %q, want the raw rendering %q (the fallback must survive)",
			got, undecodableHex)
	}
}

// /Params /CheckSum and /ModDate stay raw.

func TestEmbeddedFileParams_BinaryCheckSumStaysRaw(t *testing.T) {
	ins, tabID := writeTempPDF(t, "filespec-binary.pdf", textStringFilespecPDF("<"+utf16UFHex+">"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("expected 1 embedded file, got %d", len(list.Files))
	}
	f := list.Files[0]
	if f.CheckSum != rawCheckSumHex {
		t.Errorf("CheckSum = %q, want the raw hex digits %q (a binary field must NOT be routed through the text decoder)",
			f.CheckSum, rawCheckSumHex)
	}
	if f.ModDate != "D:20240101000000Z" {
		t.Errorf("ModDate = %q, want the verbatim ASCII date", f.ModDate)
	}
}

// StringValue stays the raw renderer.

func TestStringValue_StaysRaw(t *testing.T) {
	if got := stringValue(pdfcpu_types.HexLiteral(utf16TitleHex)); got != utf16TitleHex {
		t.Errorf("stringValue(HexLiteral) = %q, want the hex digits %q verbatim", got, utf16TitleHex)
	}
	if got := stringValue(pdfcpu_types.StringLiteral("\x80")); got != "\x80" {
		t.Errorf("stringValue(StringLiteral) = %q, want the raw byte verbatim", got)
	}
	if got := stringValue(pdfcpu_types.Integer(42)); got != "" {
		t.Errorf("stringValue(non-string) = %q, want \"\"", got)
	}
}

// A decode that succeeds but yields invalid UTF-8
// falls back to raw, so the bytes never reach json.Marshal to become U+FFFD.

func TestTextStringOrRaw_InvalidUTF8FallsBackToRaw(t *testing.T) {
	const badHex = "80CAFE" // not UTF-16BE (no BOM), not valid UTF-8

	// Precondition: pdfcpu reports SUCCESS here, returning the bytes verbatim.
	decoded, ok := decodeTextStringOK(pdfcpu_types.HexLiteral(badHex))
	if !ok {
		t.Fatalf("HexLiteral(%q) was expected to decode without error", badHex)
	}
	if utf8.ValidString(decoded) {
		t.Fatalf("HexLiteral(%q) decoded to valid UTF-8 %q; this case no longer exercises the guard",
			badHex, decoded)
	}

	got := textStringOrRaw(pdfcpu_types.HexLiteral(badHex))
	if got != badHex {
		t.Errorf("textStringOrRaw(HexLiteral(%q)) = %q (% x), want the raw rendering %q -- invalid UTF-8 must not reach the JSON surface",
			badHex, got, []byte(got), badHex)
	}
	if !utf8.ValidString(got) {
		t.Errorf("result %q is still invalid UTF-8; json.Marshal would replace it with U+FFFD", got)
	}
}

// All six /Info text keys decode, not just /Title.

func TestCollectInfoFields_AllSixTextKeysDecode(t *testing.T) {
	for _, key := range []string{"Title", "Author", "Subject", "Keywords", "Creator", "Producer"} {
		t.Run(key, func(t *testing.T) {
			if !infoTextFields[key] {
				t.Fatalf("%q is missing from infoTextFields", key)
			}
			ins, tabID := writeTempPDF(t, "info-utf16-"+key+".pdf",
				textStringInfoPDFKey(key, "<"+utf16TitleHex+">"))
			md, err := ins.GetDocumentMetadata(tabID)
			if err != nil {
				t.Fatalf("GetDocumentMetadata error: %v", err)
			}
			if got := md.Info[key]; got != utf16TitleText {
				t.Errorf("Info[%q] = %q, want %q", key, got, utf16TitleText)
			}
		})
	}
}

// ScalarDisplay still renders text bytes as raw hex.
// It has no key context, so it cannot tell a text /Title from a binary /ID.

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
				t.Errorf("scalarDisplay = %q, want %q (the tree renderer must NOT be text-decoded)",
					got, tc.want)
			}
		})
	}
}

// The raw-fallback branch is UTF-8-validated too,
// and renders as hex digits when it is not.
// TestTextStringOrRaw_InvalidUTF8FallsBackToRaw uses a HexLiteral, whose raw
// form is already ASCII, so it does not cover this branch.

func TestTextStringOrRaw_RawFallbackIsAlwaysValidUTF8(t *testing.T) {
	// BOM + unpaired low surrogate, carried as a literal rather than as hex.
	in := pdfcpu_types.StringLiteral("\xfe\xff\xdc\x00")

	if _, ok := decodeTextStringOK(in); ok {
		t.Fatalf("this input was expected to FAIL to decode")
	}
	if utf8.ValidString(stringValue(in)) {
		t.Fatalf("stringValue already returns valid UTF-8; this case no longer exercises the guard")
	}

	got := textStringOrRaw(in)
	if !utf8.ValidString(got) {
		t.Errorf("textStringOrRaw = %q (% x), which is invalid UTF-8 - json.Marshal would replace it with U+FFFD",
			got, []byte(got))
	}
	if got != "FEFFDC00" {
		t.Errorf("textStringOrRaw = %q, want the uppercase hex digits \"FEFFDC00\" (undecodable input must render the same shape whichever literal type carried it)",
			got)
	}
}

// Every non-date infoFields key is in
// infoTextFields, so a new text key cannot silently render raw.

func TestInfoTextFields_CoversEveryNonDateInfoKey(t *testing.T) {
	dateKeys := map[string]bool{"CreationDate": true, "ModDate": true}
	for _, key := range infoFields {
		if dateKeys[key] || infoTextFields[key] {
			continue
		}
		t.Errorf("infoFields key %q is neither a known date key nor a member of infoTextFields, so it silently renders RAW while collectInfoFields' else branch claims it is a date. Add it to infoTextFields or to this test's dateKeys set.",
			key)
	}
	if len(infoTextFields)+len(dateKeys) != len(infoFields) {
		t.Errorf("infoTextFields(%d) + dateKeys(%d) != infoFields(%d); the sets have drifted",
			len(infoTextFields), len(dateKeys), len(infoFields))
	}
}

// A BOM-only /UF decodes to "" and defers to /F.

func TestEmbeddedFileName_EmptyUFFallsThroughToF(t *testing.T) {
	ins, tabID := writeTempPDF(t, "filespec-empty-uf.pdf", textStringFilespecPDF("<FEFF>"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].Name; got != "groesse.xml" {
		t.Errorf("Name = %q, want the /F value \"groesse.xml\" (an empty /UF must defer to /F, not render as \"FEFF\")",
			got)
	}
}

// A UTF-16BE-with-BOM /ModDate stays raw hex, which
// only holds while the date keys are off the decoder.

func TestCollectInfoFields_NonASCIIDateStaysRaw(t *testing.T) {
	// UTF-16BE-with-BOM for "D:20240101000000Z".
	const utf16DateHex = "FEFF0044003A00320030003200340030003100300031003000300030003000300030005A"

	ins, tabID := writeTempPDF(t, "info-utf16-date.pdf", textStringInfoPDFKey("ModDate", "<"+utf16DateHex+">"))
	md, err := ins.GetDocumentMetadata(tabID)
	if err != nil {
		t.Fatalf("GetDocumentMetadata error: %v", err)
	}
	if got := md.Info["ModDate"]; got != utf16DateHex {
		t.Errorf("Info[\"ModDate\"] = %q, want the raw hex digits %q - the date keys must stay on stringValue",
			got, utf16DateHex)
	}
}

// A /CheckSum whose bytes begin FE FF stays raw.
// TestEmbeddedFileParams_BinaryCheckSumStaysRaw uses a value that decodes to
// invalid UTF-8 and so falls back to the same digits either way; this one
// decodes to "A", so it detects a leak.

func TestEmbeddedFileCheckSum_BOMPrefixedBinaryStaysRaw(t *testing.T) {
	const bomCheckSum = "FEFF0041" // decodes to "A" if text-decoded

	// Precondition: this value really does decode to something different.
	if decoded, ok := decodeTextStringOK(pdfcpu_types.HexLiteral(bomCheckSum)); !ok || decoded == bomCheckSum {
		t.Fatalf("HexLiteral(%q) must decode to a DIFFERENT value for this guard to be falsifiable; got %q ok=%v",
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
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].CheckSum; got != bomCheckSum {
		t.Errorf("CheckSum = %q, want the raw hex digits %q - a binary field must NOT be text-decoded even when its bytes look like a UTF-16BE BOM",
			got, bomCheckSum)
	}
}

// A /F-only filespec decodes /F. Every other filespec
// case carries a /UF, so this is the only one reaching the /F branch.

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
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].Name; got != utf16UFText {
		t.Errorf("Name = %q, want %q - a /F-only filespec must decode /F, not echo its hex digits",
			got, utf16UFText)
	}
}

// A UTF-16BE-with-BOM /Params /ModDate stays raw.
// TestEmbeddedFileParams_BinaryCheckSumStaysRaw's ASCII date cannot detect a
// leak on this field.

func TestEmbeddedFileModDate_BOMPrefixedDateStaysRaw(t *testing.T) {
	// UTF-16BE-with-BOM for "D:20240101000000Z".
	const utf16ModDateHex = "FEFF0044003A00320030003200340030003100300031003000300030003000300030005A"
	const utf16ModDateText = "D:20240101000000Z"

	// Precondition: this value really does decode to something different, so a
	// leak into either decoder cannot pass unnoticed.
	if decoded, ok := decodeTextStringOK(pdfcpu_types.HexLiteral(utf16ModDateHex)); !ok || decoded != utf16ModDateText {
		t.Fatalf("HexLiteral(%q) must decode to %q for this guard to be falsifiable; got %q ok=%v",
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
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("expected 1 embedded file, got %d", len(list.Files))
	}
	if got := list.Files[0].ModDate; got != utf16ModDateHex {
		t.Errorf("ModDate = %q, want the raw hex digits %q - filespec /Params /ModDate must stay on stringValue even when its bytes decode as a UTF-16BE text string",
			got, utf16ModDateHex)
	}
}
