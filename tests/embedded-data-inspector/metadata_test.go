package embedded_data_inspector_test

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// `dump metadata` plain-text default = an aligned "key: value" Info block
// plus the XMP packet. The Info field values and the verbatim XMP marker
// must both appear.
// ---------------------------------------------------------------------------

func TestMetadata_PlainShowsInfoAndXMP(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "metadata.pdf", metadataPDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "metadata", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, stdout)

	// Aligned key: value Info block -- a "key: value" line must be present.
	if !strings.Contains(stdout, ":") {
		t.Errorf("expected an aligned key: value Info line:\n%s", stdout)
	}
	for _, want := range []string{"Invoice 2024-001", "ACME GmbH", "pdfdebug-test"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected Info value %q in plain output:\n%s", want, stdout)
		}
	}
	// The XMP packet is included; its verbatim marker survives.
	if !strings.Contains(stdout, "marker-VERBATIM-XMP") {
		t.Errorf("expected the verbatim XMP packet in plain output:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// `dump metadata --json` = {info:{...}, xmp:"..."}.
// ---------------------------------------------------------------------------

func TestMetadata_JSONShape(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "metadata.pdf", metadataPDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "metadata", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	var env struct {
		Info map[string]string `json:"info"`
		XMP  string            `json:"xmp"`
	}
	mustParseJSON(t, stdout, &env)

	if env.Info["Title"] != "Invoice 2024-001" {
		t.Errorf("info.Title = %q, want %q", env.Info["Title"], "Invoice 2024-001")
	}
	if !strings.Contains(env.XMP, "marker-VERBATIM-XMP") {
		t.Errorf("xmp must carry the verbatim packet, got %q", env.XMP)
	}
}

// ---------------------------------------------------------------------------
// Plain-text-default contract: plain default is ASCII-only with a trailing
// newline.
// ---------------------------------------------------------------------------

func TestMetadata_PlainIsASCIIWithTrailingNewline(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "metadata.pdf", metadataPDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "metadata", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertASCII(t, stdout)
	assertTrailingNewline(t, stdout)
}

// ---------------------------------------------------------------------------
// A document with no /Metadata and no /Info still succeeds (exit 0) with an
// empty metadata view (not an error).
// ---------------------------------------------------------------------------

func TestMetadata_MissingIsEmptyExitZero(t *testing.T) {
	bin := buildCLI(t)
	// embeddedPDF has attachments but NO /Metadata and NO /Info.
	pdf := writeTempPDF(t, "no-metadata.pdf", embeddedPDF("<x/>"))

	stdout, stderr, ec := runCLI(t, bin, "dump", "metadata", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0 for no-metadata doc, got %d (stderr: %s)", ec, stderr)
	}
	var env struct {
		Info map[string]string `json:"info"`
		XMP  string            `json:"xmp"`
	}
	mustParseJSON(t, stdout, &env)
	if env.XMP != "" {
		t.Errorf("expected empty xmp, got %q", env.XMP)
	}
	if len(env.Info) != 0 {
		t.Errorf("expected empty info, got %+v", env.Info)
	}
}

// ---------------------------------------------------------------------------
// Plain-text-default contract: `dump metadata --json --pretty` indents the
// JSON (the pretty branch in cmd_metadata.go). The output still parses to
// the same {info, xmp} shape and is multi-line.
// ---------------------------------------------------------------------------

func TestMetadata_PrettyJSONIsIndented(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "metadata.pdf", metadataPDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "metadata", "--json", "--pretty", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	var env struct {
		Info map[string]string `json:"info"`
		XMP  string            `json:"xmp"`
	}
	mustParseJSON(t, stdout, &env)
	if env.Info["Title"] != "Invoice 2024-001" {
		t.Errorf("info.Title = %q, want %q", env.Info["Title"], "Invoice 2024-001")
	}
	// Pretty output is multi-line and indented; the compact form is single-line.
	if !strings.Contains(stdout, "\n") {
		t.Errorf("--pretty output must be multi-line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  ") {
		t.Errorf("--pretty output must be indented:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// The PLAIN-text metadata view for a document with no /Info and no /Metadata
// renders a "(none)" placeholder (not a blank/error), exit 0, ASCII +
// trailing newline (cmd_metadata.go printMetadataPlain).
// ---------------------------------------------------------------------------

func TestMetadata_PlainNoneWhenEmpty(t *testing.T) {
	bin := buildCLI(t)
	// embeddedPDF has attachments but NO /Metadata and NO /Info.
	pdf := writeTempPDF(t, "no-metadata.pdf", embeddedPDF("<x/>"))

	stdout, stderr, ec := runCLI(t, bin, "dump", "metadata", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0 for empty metadata, got %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, stdout)
	assertASCII(t, stdout)
	assertTrailingNewline(t, stdout)
	if !strings.Contains(stdout, "(none)") {
		t.Errorf("expected a '(none)' placeholder for empty metadata:\n%s", stdout)
	}
}
