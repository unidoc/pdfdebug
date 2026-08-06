package shared_text_string_decoder_test

import (
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 14.4-INTG-001 [P1] AC2 (R-14-05): `dump metadata --json` returns the /Info
// /Title decoded from UTF-16BE-with-BOM into UTF-8.
//
// Before 14.4 this shipped "FEFF0052...": collectInfoFields used raw
// stringValue, and pdfcpu stores a HexLiteral as its hex DIGIT TEXT, so the
// machine contract carried a hex dump masquerading as a title.
// ---------------------------------------------------------------------------

func TestMetadataJSON_TitleIsDecodedUTF8(t *testing.T) {
	md := dumpMetadataJSON(t, "14.4-INTG-001")

	got, present := md.Info["Title"]
	if !present {
		t.Fatalf("[P1] 14.4-INTG-001: /Title absent from info (an undecodable field must fall back to raw, never disappear)")
	}
	if got == rawTitleHex {
		t.Fatalf("[P1] 14.4-INTG-001: info.Title is still the raw hex digits %q; want the decoded %q", got, wantTitle)
	}
	if got != wantTitle {
		t.Errorf("[P1] 14.4-INTG-001: info.Title = %q, want %q", got, wantTitle)
	}
}

// ---------------------------------------------------------------------------
// 14.4-INTG-002 [P1] AC3 (R-14-05): `dump embedded --json` returns the filespec
// /UF display name decoded, with /UF-preferred-else-/F precedence unchanged
// (the fixture's ASCII /F "groesse.xml" must NOT win).
//
// Same pre-14.4 failure mode as INTG-001.
// ---------------------------------------------------------------------------

func TestEmbeddedJSON_UFDisplayNameIsDecodedUTF8(t *testing.T) {
	entries := dumpEmbeddedJSON(t, "14.4-INTG-002")
	if len(entries) != 2 {
		t.Fatalf("[P1] 14.4-INTG-002: expected 2 embedded files in the fixture, got %d", len(entries))
	}

	e := entries[0]
	if e.FilespecRef != "6 0 R" {
		t.Fatalf("[P1] 14.4-INTG-002: expected the non-ASCII attachment at filespec 6 0 R, got %q", e.FilespecRef)
	}
	if e.Name == rawUFNameHex {
		t.Fatalf("[P1] 14.4-INTG-002: name is still the raw hex digits %q; want the decoded %q", e.Name, wantUFName)
	}
	if e.Name == "groesse.xml" {
		t.Fatalf("[P1] 14.4-INTG-002: name fell through to the ASCII /F; /UF precedence must be preserved")
	}
	if e.Name != wantUFName {
		t.Errorf("[P1] 14.4-INTG-002: name = %q, want %q", e.Name, wantUFName)
	}
}

// ---------------------------------------------------------------------------
// 14.4-INTG-003 [P1] AC2/AC3: decoding is a NO-OP on the all-ASCII fields in
// the same dicts. Pins that the change does not perturb existing values (the
// first half of AC7's "13-2 / 13-5 stay green" argument).
//
// GREEN TODAY and must stay green.
// ---------------------------------------------------------------------------

func TestJSON_ASCIIValuesUnchangedByDecode(t *testing.T) {
	md := dumpMetadataJSON(t, "14.4-INTG-003")
	for key, want := range map[string]string{
		"Author":       asciiAuthor,
		"Subject":      asciiSubject,
		"Producer":     "pdfdebug-fixture",
		"CreationDate": "D:20240101000000Z",
	} {
		if got := md.Info[key]; got != want {
			t.Errorf("[P1] 14.4-INTG-003: info.%s = %q, want %q (decode must be a no-op on ASCII)", key, got, want)
		}
	}

	entries := dumpEmbeddedJSON(t, "14.4-INTG-003")
	if len(entries) != 2 {
		t.Fatalf("[P1] 14.4-INTG-003: expected 2 embedded files, got %d", len(entries))
	}
	if got := entries[1].Name; got != asciiName {
		t.Errorf("[P1] 14.4-INTG-003: second attachment name = %q, want %q", got, asciiName)
	}
}

// ---------------------------------------------------------------------------
// 14.4-INTG-004 [P1] AC4 (R-14-06): the binary boundary. /Params /CheckSum is a
// binary hex string living in the SAME dict family as the decoded fields, so it
// pins the exact boundary. It must equal the literal hex DIGITS from the
// fixture source. /Params /ModDate is an ASCII PDF date and stays verbatim.
//
// GREEN TODAY and must stay green: it fails the moment the decoder leaks past
// the enumerated text keys.
// ---------------------------------------------------------------------------

func TestEmbeddedJSON_BinaryCheckSumNotTextDecoded(t *testing.T) {
	entries := dumpEmbeddedJSON(t, "14.4-INTG-004")
	if len(entries) != 2 {
		t.Fatalf("[P1] 14.4-INTG-004: expected 2 embedded files, got %d", len(entries))
	}
	for i, e := range entries {
		if e.CheckSum != rawCheckSumHex {
			t.Errorf("[P1] 14.4-INTG-004: entry %d checkSum = %q, want the raw hex digits %q (binary fields must NOT be text-decoded)",
				i, e.CheckSum, rawCheckSumHex)
		}
		if e.ModDate != "D:20240101000000Z" {
			t.Errorf("[P1] 14.4-INTG-004: entry %d modDate = %q, want the verbatim ASCII date", i, e.ModDate)
		}
	}
}

// ---------------------------------------------------------------------------
// 14.4-INTG-005 [P1] AC4 (R-14-06): the raw string RENDERERS stay raw. `dump
// object` renders through the ObjectDetail/ValueEntry builder (inspector.go),
// NOT through tree.go scalarDisplay -- both must keep emitting `<...>` hex
// digits for the very same /Title and /UF the readers now decode.
//
// (Deviation from epic-14-test-design.md's 14.4-INTG-002 row, which named
// trailer /ID or signature /Contents: /ID is not surfaced by any CLI view, so
// the story moved the guard here. Record at trace time.)
//
// GREEN TODAY and must stay green.
// ---------------------------------------------------------------------------

func TestDumpObject_RawStringRenderersUnchanged(t *testing.T) {
	bin := buildCLI(t)
	path := fixturePath(t, fixtureName)

	for _, tc := range []struct {
		ref     string
		key     string
		wantHex string
	}{
		{ref: "5 0 R", key: "/Title:", wantHex: rawTitleHex}, // the /Info dict
		{ref: "6 0 R", key: "/UF:", wantHex: rawUFNameHex},   // the filespec
	} {
		stdout, stderr, ec := runCLI(t, bin, "dump", "object", "--ref", tc.ref, path)
		if ec != 0 {
			t.Fatalf("[P1] 14.4-INTG-005: dump object --ref %q exit %d (stderr: %s)", tc.ref, ec, stderr)
		}
		// Scope the assertion to the NAMED key's line, not the whole dump: a bare
		// whole-output Contains would pass if the hex appeared under any other key
		// or in an unrelated raw section, for a test whose entire point is that
		// THIS field still renders raw.
		var line string
		for _, l := range strings.Split(stdout, "\n") {
			if strings.Contains(l, tc.key) {
				line = l
				break
			}
		}
		if line == "" {
			t.Fatalf("[P1] 14.4-INTG-005: dump object --ref %q produced no %s line; got:\n%s", tc.ref, tc.key, stdout)
		}
		if !strings.Contains(line, "<"+tc.wantHex+">") {
			t.Errorf("[P1] 14.4-INTG-005: dump object --ref %q must still render %s as the raw hex string <%s>; got line %q in:\n%s",
				tc.ref, tc.key, tc.wantHex, line, stdout)
		}
	}
}

// ---------------------------------------------------------------------------
// 14.4-INTG-006 [P1] AC5: the plain-text surface stays ASCII-only AND actually
// carries the decoded value, escaped consistently with validate.go's
// strconv.QuoteToASCII precedent.
//
// The ASCII assertion is scoped to the INFO BLOCK, not the whole stdout: the XMP
// packet below the "XMP:" heading is UTF-8 XML passed through verbatim by design
// (see printMetadataPlain's godoc), so a whole-stdout assertion would be a
// property the implementation cannot honor - it passes here only because this
// fixture has no /Metadata stream, which is false confidence, not coverage.
// ---------------------------------------------------------------------------

func TestMetadataPlain_NonASCIIValueIsASCIIEscaped(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "metadata", fixturePath(t, fixtureName))
	if ec != 0 {
		t.Fatalf("[P1] 14.4-INTG-006: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}

	// The 13-1 plain-text contract: ASCII only across the Info block. The XMP
	// packet is a documented verbatim-UTF-8 exemption, so cut there.
	// Cut on the whole heading LINE, not a bare substring: an /Info value that
	// happened to contain "XMP:" would otherwise truncate infoBlock to "Info:\n"
	// and the scan below would pass vacuously - the assertion would disappear
	// without failing. printMetadataPlain emits the heading via w.Heading("XMP:").
	infoBlock := stdout
	if i := strings.Index(stdout, "\nXMP:\n"); i >= 0 {
		infoBlock = stdout[:i]
	}
	if off, b := firstNonASCII(infoBlock); off >= 0 {
		t.Errorf("[P1] 14.4-INTG-006: Info block carries non-ASCII byte 0x%02x at offset %d; decoded values must be escaped on this surface",
			b, off)
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Errorf("[P1] 14.4-INTG-006: plain output must end with a trailing newline")
	}

	want := strconv.QuoteToASCII(wantTitle)
	if strings.Contains(stdout, rawTitleHex) {
		t.Fatalf("[P1] 14.4-INTG-006: plain output still shows the raw hex digits for /Title; want the escaped decoded form %s\n%s",
			want, stdout)
	}
	if !strings.Contains(stdout, want) {
		t.Errorf("[P1] 14.4-INTG-006: plain output must carry the decoded /Title escaped as %s; got:\n%s", want, stdout)
	}
}

// ---------------------------------------------------------------------------
// 14.4-INTG-007 [P1] AC5: the embedded plain-text TABLE gets the same treatment
// for the non-ASCII Name column (tableWriter pads on len() bytes, so a raw
// multi-byte name also mis-aligns the table).
// ---------------------------------------------------------------------------

func TestEmbeddedPlain_NonASCIINameIsASCIIEscaped(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "embedded", fixturePath(t, fixtureName))
	if ec != 0 {
		t.Fatalf("[P1] 14.4-INTG-007: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}

	if off, b := firstNonASCII(stdout); off >= 0 {
		t.Errorf("[P1] 14.4-INTG-007: plain output carries non-ASCII byte 0x%02x at offset %d", b, off)
	}

	want := strconv.QuoteToASCII(wantUFName)
	if strings.Contains(stdout, rawUFNameHex) {
		t.Fatalf("[P1] 14.4-INTG-007: plain table still shows the raw hex digits for the name; want the escaped decoded form %s\n%s",
			want, stdout)
	}
	if !strings.Contains(stdout, want) {
		t.Errorf("[P1] 14.4-INTG-007: plain table must carry the decoded name escaped as %s; got:\n%s", want, stdout)
	}
}

// ---------------------------------------------------------------------------
// 14.4-INTG-008 [P1] AC5/AC7: the escape is CONDITIONAL. An already-printable-
// ASCII value must render BYTE-IDENTICALLY to today -- no wrapping quotes.
// An unconditional strconv.QuoteToASCII would rewrite every existing plain row
// and would still slip past 13-2's strings.Contains assertions, so this is the
// pin that forbids it.
//
// GREEN TODAY and must stay green.
// ---------------------------------------------------------------------------

func TestPlainSurfaces_ASCIIValuesRenderUnquoted(t *testing.T) {
	bin := buildCLI(t)
	path := fixturePath(t, fixtureName)

	mdOut, stderr, ec := runCLI(t, bin, "dump", "metadata", path)
	if ec != 0 {
		t.Fatalf("[P1] 14.4-INTG-008: dump metadata exit %d (stderr: %s)", ec, stderr)
	}
	for _, v := range []string{asciiAuthor, asciiSubject, "D:20240101000000Z"} {
		if !strings.Contains(mdOut, v) {
			t.Errorf("[P1] 14.4-INTG-008: plain metadata must still contain the bare ASCII value %q; got:\n%s", v, mdOut)
		}
		if strings.Contains(mdOut, strconv.Quote(v)) {
			t.Errorf("[P1] 14.4-INTG-008: plain metadata wrapped the all-ASCII value %q in quotes; the AC5 escape must be CONDITIONAL", v)
		}
	}

	embOut, stderr, ec := runCLI(t, bin, "dump", "embedded", path)
	if ec != 0 {
		t.Fatalf("[P1] 14.4-INTG-008: dump embedded exit %d (stderr: %s)", ec, stderr)
	}
	if !strings.Contains(embOut, asciiName) {
		t.Errorf("[P1] 14.4-INTG-008: plain embedded table must still contain the bare ASCII name %q; got:\n%s", asciiName, embOut)
	}
	if strings.Contains(embOut, strconv.Quote(asciiName)) {
		t.Errorf("[P1] 14.4-INTG-008: plain embedded table wrapped the all-ASCII name %q in quotes; the AC5 escape must be CONDITIONAL", asciiName)
	}
}

// ---------------------------------------------------------------------------
// 14.4-INTG-009 [P2] AC3 + Source verification #4: `dump embedded --name`
// selects on the DECODED name (cmd_embedded.go matches f.Name == name). A
// caller passing correct UTF-8 must now match -- the intended, user-visible CLI
// behavior change that belongs in the change log. Before 14.4 the selector key
// was the hex-digit text, so correct UTF-8 could not match.
// ---------------------------------------------------------------------------

func TestEmbeddedExtract_NameSelectorUsesDecodedName(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "embedded", "--name", wantUFName, fixturePath(t, fixtureName))
	if ec != 0 {
		t.Fatalf("[P2] 14.4-INTG-009: `--name %q` must select the attachment once /UF is decoded; exit %d (stderr: %s)",
			wantUFName, ec, stderr)
	}
	if !strings.Contains(stdout, "<Invoice") {
		t.Errorf("[P2] 14.4-INTG-009: expected the non-ASCII attachment payload on stdout, got:\n%s", stdout)
	}
}
