package cli_output_format_normalization_test

import (
	"strings"
	"testing"
)

// pageInfoFixture is the relative path to the render-info fixture exercised by
// the page --info tests.
const pageInfoFixture = "page-render/render-info.pdf"

// ---------------------------------------------------------------------------
// Default (no --json) flips from always-JSON to plain-text sections.
// Structural: aligned "key: value" sections with section labels (geometry /
// extgstates / xobjects / forms).
// ---------------------------------------------------------------------------

func TestPageInfo_DefaultPlainSections(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--info", "1", fixture(t, pageInfoFixture))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, stdout)
	assertTrailingNewline(t, stdout)

	// A single-record presenter renders aligned "key: value" lines; the geometry
	// section must surface the page's MediaBox (a geometry field always present).
	if !containsLineWith(stdout, ":") {
		t.Errorf("expected aligned \"key: value\" lines in page render info:\n%s", stdout)
	}
	lower := strings.ToLower(stdout)
	if !strings.Contains(lower, "mediabox") {
		t.Errorf("expected the MediaBox geometry field in plain output:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// --json includes the top-level in-band stability marker
// "_stability":"experimental" on the FULL object. (The story leaves
// section-view placement to the implementer; this asserts the unambiguous
// full-object case only.)
// ---------------------------------------------------------------------------

func TestPageInfo_JSONCarriesStabilityMarker(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--info", "1", "--json", fixture(t, pageInfoFixture))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	var obj map[string]any
	mustParseJSON(t, stdout, &obj)
	v, ok := obj["_stability"]
	if !ok {
		t.Fatalf("--json full object missing top-level \"_stability\" key:\n%s", stdout)
	}
	if s, _ := v.(string); s != "experimental" {
		t.Errorf("\"_stability\" = %v, want \"experimental\"", v)
	}
}

// ---------------------------------------------------------------------------
// The --section filter is honored in the plain presenter. Structural: a
// sectioned plain dump renders ONLY that section and omits a sibling section's
// distinctive content. We compare the geometry section (carries MediaBox)
// against the extgstates section (does not).
// ---------------------------------------------------------------------------

func TestPageInfo_SectionFilterHonoredInPlain(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, pageInfoFixture)

	geom, stderr, ec := runCLI(t, bin, "dump", "page", "--info", "1", "--section", "geometry", file)
	if ec != 0 {
		t.Fatalf("--section geometry exit %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, geom)
	if !strings.Contains(strings.ToLower(geom), "mediabox") {
		t.Errorf("--section geometry must include MediaBox:\n%s", geom)
	}

	exg, stderr, ec := runCLI(t, bin, "dump", "page", "--info", "1", "--section", "extgstates", file)
	if ec != 0 {
		t.Fatalf("--section extgstates exit %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, exg)
	// The extgstates section must NOT carry the geometry MediaBox field; that is
	// proof the --section filter scoped the plain output.
	if strings.Contains(strings.ToLower(exg), "mediabox") {
		t.Errorf("--section extgstates leaked the geometry MediaBox field (filter not honored):\n%s", exg)
	}
}

// ---------------------------------------------------------------------------
// --forms-recursive / --forms-depth behavior is unchanged under the new
// default. The flags are accepted (exit 0) on the plain default path and
// produce a forms-bearing render picture.
// ---------------------------------------------------------------------------

func TestPageInfo_FormsFlagsUnchanged(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, pageInfoFixture)

	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--info", "1", "--forms-recursive", "--forms-depth", "2", file)
	if ec != 0 {
		t.Fatalf("--forms-recursive --forms-depth 2 must be accepted, got exit %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, stdout)

	// The same flags under --json must still yield a parseable forms-bearing
	// object (no behavior regression on the JSON path).
	jsonOut, _, ecj := runCLI(t, bin, "dump", "page", "--info", "1", "--forms-recursive", "--forms-depth", "2", "--json", file)
	if ecj != 0 {
		t.Fatalf("--json variant exit %d", ecj)
	}
	var obj map[string]any
	mustParseJSON(t, jsonOut, &obj)
	if _, ok := obj["forms"]; !ok {
		t.Errorf("--json render info missing \"forms\" key:\n%s", jsonOut)
	}
}
