package embedded_data_inspector_test

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 13.2-INTG-001 [P0] AC4: `dump embedded` plain-text default lists the embedded
// files as a human-readable table carrying name, relationship, MIME, and size.
// ---------------------------------------------------------------------------

func TestEmbeddedList_PlainTableHasFields(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "embedded.pdf", embeddedPDF("<?xml version=\"1.0\"?><Invoice/>"))

	stdout, stderr, ec := runCLI(t, bin, "dump", "embedded", pdf)
	if ec != 0 {
		t.Fatalf("[P0] 13.2-INTG-001: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, "13.2-INTG-001", stdout)

	lower := strings.ToLower(stdout)
	// The display name must appear.
	if !strings.Contains(stdout, "factur-x.xml") {
		t.Errorf("[P0] 13.2-INTG-001: expected display name in table:\n%s", stdout)
	}
	// AFRelationship (Data) is the ZUGFeRD discriminator -- must be visible.
	if !strings.Contains(stdout, "Data") {
		t.Errorf("[P0] 13.2-INTG-001: expected AFRelationship 'Data' in table:\n%s", stdout)
	}
	// MIME subtype.
	if !strings.Contains(lower, "text/xml") {
		t.Errorf("[P0] 13.2-INTG-001: expected MIME text/xml in table:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-002 [P0] AC4: `dump embedded --json` emits a structured array, one
// element per embedded file, carrying the discriminating fields.
// ---------------------------------------------------------------------------

func TestEmbeddedList_JSONIsStructuredArray(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "embedded.pdf", embeddedPDF("<?xml version=\"1.0\"?><Invoice/>"))

	stdout, stderr, ec := runCLI(t, bin, "dump", "embedded", "--json", pdf)
	if ec != 0 {
		t.Fatalf("[P0] 13.2-INTG-002: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" || trimmed[0] != '[' {
		t.Fatalf("[P0] 13.2-INTG-002: --json must emit a top-level array, got:\n%s", stdout)
	}
	var arr []map[string]any
	mustParseJSON(t, stdout, &arr)
	if len(arr) != 1 {
		t.Fatalf("[P0] 13.2-INTG-002: expected 1 embedded file, got %d:\n%s", len(arr), stdout)
	}
	e := arr[0]
	if name, _ := e["name"].(string); name != "factur-x.xml" {
		t.Errorf("[P0] 13.2-INTG-002: name = %v, want factur-x.xml", e["name"])
	}
	// The AFRelationship key must be present and equal to Data.
	rel, _ := e["afRelationship"].(string)
	if rel != "Data" {
		t.Errorf("[P0] 13.2-INTG-002: afRelationship = %v, want Data", e["afRelationship"])
	}
}

// ---------------------------------------------------------------------------
// 13.2-INTG-003 [P1] AC4 + 13-1 contract: the plain-text list is ASCII-only and
// ends with a trailing newline.
// ---------------------------------------------------------------------------

func TestEmbeddedList_PlainIsASCIIWithTrailingNewline(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "embedded.pdf", embeddedPDF("<x/>"))

	stdout, stderr, ec := runCLI(t, bin, "dump", "embedded", pdf)
	if ec != 0 {
		t.Fatalf("[P1] 13.2-INTG-003: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertASCII(t, "13.2-INTG-003", stdout)
	assertTrailingNewline(t, "13.2-INTG-003", stdout)
}

// ---------------------------------------------------------------------------
// 13.2-INTG-004 [P1] AC1/AC4: a document with no embedded files lists an empty
// result (exit 0), not an error.
// ---------------------------------------------------------------------------

func TestEmbeddedList_NoneIsEmptyExitZero(t *testing.T) {
	bin := buildCLI(t)
	// metadataPDF has /Metadata + /Info but NO embedded files.
	pdf := writeTempPDF(t, "no-embedded.pdf", metadataPDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "embedded", "--json", pdf)
	if ec != 0 {
		t.Fatalf("[P1] 13.2-INTG-004: expected exit 0 for no-attachments doc, got %d (stderr: %s)", ec, stderr)
	}
	var arr []map[string]any
	mustParseJSON(t, stdout, &arr)
	if len(arr) != 0 {
		t.Errorf("[P1] 13.2-INTG-004: expected empty array, got %d entries:\n%s", len(arr), stdout)
	}
}
