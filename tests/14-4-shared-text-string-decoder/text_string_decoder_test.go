package shared_text_string_decoder_test

import (
	"strconv"
	"strings"
	"testing"
)

// `dump metadata --json` returns the /Info /Title
// decoded from UTF-16BE-with-BOM into UTF-8.

func TestMetadataJSON_TitleIsDecodedUTF8(t *testing.T) {
	md := dumpMetadataJSON(t)

	got, present := md.Info["Title"]
	if !present {
		t.Fatalf("/Title absent from info (an undecodable field must fall back to raw, never disappear)")
	}
	if got == rawTitleHex {
		t.Fatalf("info.Title is still the raw hex digits %q; want the decoded %q", got, wantTitle)
	}
	if got != wantTitle {
		t.Errorf("info.Title = %q, want %q", got, wantTitle)
	}
}

// `dump embedded --json` returns the filespec /UF
// decoded, with /UF-before-/F precedence intact.

func TestEmbeddedJSON_UFDisplayNameIsDecodedUTF8(t *testing.T) {
	entries := dumpEmbeddedJSON(t)
	if len(entries) != 2 {
		t.Fatalf("expected 2 embedded files in the fixture, got %d", len(entries))
	}

	e := entries[0]
	if e.FilespecRef != "6 0 R" {
		t.Fatalf("expected the non-ASCII attachment at filespec 6 0 R, got %q", e.FilespecRef)
	}
	if e.Name == rawUFNameHex {
		t.Fatalf("name is still the raw hex digits %q; want the decoded %q", e.Name, wantUFName)
	}
	if e.Name == "groesse.xml" {
		t.Fatalf("name fell through to the ASCII /F; /UF precedence must be preserved")
	}
	if e.Name != wantUFName {
		t.Errorf("name = %q, want %q", e.Name, wantUFName)
	}
}

// Decoding is a no-op on the all-ASCII fields in
// the same dicts.

func TestJSON_ASCIIValuesUnchangedByDecode(t *testing.T) {
	md := dumpMetadataJSON(t)
	for key, want := range map[string]string{
		"Author":       asciiAuthor,
		"Subject":      asciiSubject,
		"Producer":     "pdfdebug-fixture",
		"CreationDate": "D:20240101000000Z",
	} {
		if got := md.Info[key]; got != want {
			t.Errorf("info.%s = %q, want %q (decode must be a no-op on ASCII)", key, got, want)
		}
	}

	entries := dumpEmbeddedJSON(t)
	if len(entries) != 2 {
		t.Fatalf("expected 2 embedded files, got %d", len(entries))
	}
	if got := entries[1].Name; got != asciiName {
		t.Errorf("second attachment name = %q, want %q", got, asciiName)
	}
}

// The binary boundary. /Params /CheckSum is a binary
// hex string in the same dict family as the decoded fields and must equal the
// literal digits from the fixture; /Params /ModDate stays verbatim.

func TestEmbeddedJSON_BinaryCheckSumNotTextDecoded(t *testing.T) {
	entries := dumpEmbeddedJSON(t)
	if len(entries) != 2 {
		t.Fatalf("expected 2 embedded files, got %d", len(entries))
	}
	for i, e := range entries {
		if e.CheckSum != rawCheckSumHex {
			t.Errorf("entry %d checkSum = %q, want the raw hex digits %q (binary fields must NOT be text-decoded)",
				i, e.CheckSum, rawCheckSumHex)
		}
		if e.ModDate != "D:20240101000000Z" {
			t.Errorf("entry %d modDate = %q, want the verbatim ASCII date", i, e.ModDate)
		}
	}
}

// The raw string renderers stay raw. `dump object`
// renders through the ObjectDetail builder, not tree.go scalarDisplay; both must
// keep emitting <...> hex digits for the same /Title and /UF the readers decode.

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
			t.Fatalf("dump object --ref %q exit %d (stderr: %s)", tc.ref, ec, stderr)
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
			t.Fatalf("dump object --ref %q produced no %s line; got:\n%s", tc.ref, tc.key, stdout)
		}
		if !strings.Contains(line, "<"+tc.wantHex+">") {
			t.Errorf("dump object --ref %q must still render %s as the raw hex string <%s>; got line %q in:\n%s",
				tc.ref, tc.key, tc.wantHex, line, stdout)
		}
	}
}

// The plain-text surface stays ASCII-only and carries
// the decoded value escaped per strconv.QuoteToASCII.
//
// Scoped to the Info block: the XMP packet below the "XMP:" heading is UTF-8 XML
// passed through verbatim, so a whole-stdout assertion could not hold.

func TestMetadataPlain_NonASCIIValueIsASCIIEscaped(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "metadata", fixturePath(t, fixtureName))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
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
		t.Errorf("Info block carries non-ASCII byte 0x%02x at offset %d; decoded values must be escaped on this surface",
			b, off)
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Errorf("plain output must end with a trailing newline")
	}

	want := strconv.QuoteToASCII(wantTitle)
	if strings.Contains(stdout, rawTitleHex) {
		t.Fatalf("plain output still shows the raw hex digits for /Title; want the escaped decoded form %s\n%s",
			want, stdout)
	}
	if !strings.Contains(stdout, want) {
		t.Errorf("plain output must carry the decoded /Title escaped as %s; got:\n%s", want, stdout)
	}
}

// The embedded plain table gets the same treatment for
// the non-ASCII Name column, which raw multi-byte text would also mis-align.

func TestEmbeddedPlain_NonASCIINameIsASCIIEscaped(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "embedded", fixturePath(t, fixtureName))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}

	if off, b := firstNonASCII(stdout); off >= 0 {
		t.Errorf("plain output carries non-ASCII byte 0x%02x at offset %d", b, off)
	}

	want := strconv.QuoteToASCII(wantUFName)
	if strings.Contains(stdout, rawUFNameHex) {
		t.Fatalf("plain table still shows the raw hex digits for the name; want the escaped decoded form %s\n%s",
			want, stdout)
	}
	if !strings.Contains(stdout, want) {
		t.Errorf("plain table must carry the decoded name escaped as %s; got:\n%s", want, stdout)
	}
}

// The escape is conditional - an already-printable
// ASCII value renders unquoted, byte-identically.

func TestPlainSurfaces_ASCIIValuesRenderUnquoted(t *testing.T) {
	bin := buildCLI(t)
	path := fixturePath(t, fixtureName)

	mdOut, stderr, ec := runCLI(t, bin, "dump", "metadata", path)
	if ec != 0 {
		t.Fatalf("dump metadata exit %d (stderr: %s)", ec, stderr)
	}
	for _, v := range []string{asciiAuthor, asciiSubject, "D:20240101000000Z"} {
		if !strings.Contains(mdOut, v) {
			t.Errorf("plain metadata must still contain the bare ASCII value %q; got:\n%s", v, mdOut)
		}
		if strings.Contains(mdOut, strconv.Quote(v)) {
			t.Errorf("plain metadata wrapped the all-ASCII value %q in quotes; the escape must be conditional", v)
		}
	}

	embOut, stderr, ec := runCLI(t, bin, "dump", "embedded", path)
	if ec != 0 {
		t.Fatalf("dump embedded exit %d (stderr: %s)", ec, stderr)
	}
	if !strings.Contains(embOut, asciiName) {
		t.Errorf("plain embedded table must still contain the bare ASCII name %q; got:\n%s", asciiName, embOut)
	}
	if strings.Contains(embOut, strconv.Quote(asciiName)) {
		t.Errorf("plain embedded table wrapped the all-ASCII name %q in quotes; the escape must be conditional", asciiName)
	}
}

// `dump embedded --name` selects on the DECODED name.

func TestEmbeddedExtract_NameSelectorUsesDecodedName(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "embedded", "--name", wantUFName, fixturePath(t, fixtureName))
	if ec != 0 {
		t.Fatalf("`--name %q` must select the attachment once /UF is decoded; exit %d (stderr: %s)",
			wantUFName, ec, stderr)
	}
	if !strings.Contains(stdout, "<Invoice") {
		t.Errorf("expected the non-ASCII attachment payload on stdout, got:\n%s", stdout)
	}
}
