package object_tree_scalar_values_test

import (
	"strings"
	"testing"
)

// objectPropertyLine returns the value text of one `dump object` property row,
// with the key, the colon and the alignment padding removed.
func objectPropertyLine(t *testing.T, out, key string) string {
	t.Helper()
	prefix := key + ":"
	for _, line := range trimmedLines(out) {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("dump object has no property row for %q\n--- output ---\n%s", key, out)
	return ""
}

// The detail view's two fields stop being written identically: Display carries
// the decoded text, Raw the byte-exact stored form.

func TestObjectDetail_DisplayIsDecodedAndRawIsByteExact(t *testing.T) {
	detail := objectJSON(t, fixturePath(t, "scalar-values.pdf"), "10 0 R")

	alt := propertyValue(t, detail, "/Alt")
	if alt.Display != altText {
		t.Errorf("/Alt display = %q, want the decoded %q", alt.Display, altText)
	}
	if alt.Raw != "<"+altHex+">" {
		t.Errorf("/Alt raw = %q, want the byte-exact %q", alt.Raw, "<"+altHex+">")
	}
	if alt.Display == alt.Raw {
		t.Errorf("/Alt display and raw are still written identically: %q", alt.Display)
	}

	e := propertyValue(t, detail, "/E")
	if e.Display != eText {
		t.Errorf("/E display = %q, want the decoded %q", e.Display, eText)
	}
	if e.Raw != eLiteral {
		t.Errorf("/E raw = %q, want the byte-exact %q", e.Raw, eLiteral)
	}
}

func TestObjectDetail_NamesKeepTheirLeadingSlash(t *testing.T) {
	detail := objectJSON(t, fixturePath(t, "scalar-values.pdf"), "10 0 R")

	s := propertyValue(t, detail, "/S")
	if s.Display != "/TD" {
		t.Errorf("/S display = %q; a name must stay distinguishable from a string", s.Display)
	}
}

func TestObjectDetail_PlainRowsDropStringDelimiters(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dump", "object", "--ref", "10 0 R", fixturePath(t, "scalar-values.pdf"))
	if code != 0 {
		t.Fatalf("dump object exited %d: %s", code, stderr)
	}

	if got := objectPropertyLine(t, stdout, "/Alt"); got != altText {
		t.Errorf("dump object /Alt = %q, want %q", got, altText)
	}
	if got := objectPropertyLine(t, stdout, "/E"); got != eText {
		t.Errorf("dump object /E = %q, want %q", got, eText)
	}
	if got := objectPropertyLine(t, stdout, "/S"); got != "/TD" {
		t.Errorf("dump object /S = %q, want %q", got, "/TD")
	}
}

func TestObjectDetail_AsciiStringDecodesToItselfWithoutDelimiters(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dump", "object", "--ref", "1 0 R", fixturePath(t, "scalar-values.pdf"))
	if code != 0 {
		t.Fatalf("dump object exited %d: %s", code, stderr)
	}

	if got := objectPropertyLine(t, stdout, "/Lang"); got != langText {
		t.Errorf("dump object /Lang = %q, want %q", got, langText)
	}
}

func TestObjectDetail_EmptyStringStillRendersItsStoredForm(t *testing.T) {
	detail := objectJSON(t, fixturePath(t, "scalar-values.pdf"), "5 0 R")

	cases := map[string]string{
		"/Empty":    "()",
		"/HexEmpty": "<>",
		"/BomOnly":  "<FEFF>",
	}
	for key, want := range cases {
		if got := propertyValue(t, detail, key).Display; got != want {
			t.Errorf("%s display = %q, want %q; an empty display would fall back to raw", key, got, want)
		}
	}
}

// The detail view carves out a signature /Contents the same way the tree does:
// the display shows the fixed-width summary while raw keeps the bytes.

func TestObjectDetail_SignatureContentsStaysBinary(t *testing.T) {
	detail := objectJSON(t, fixturePath(t, "scalar-values.pdf"), "12 0 R")

	contents := propertyValue(t, detail, "/Contents")
	if contents.Display != binarySummary(sigContentsBytes) {
		t.Errorf("signature /Contents display = %q, want %q",
			contents.Display, binarySummary(sigContentsBytes))
	}
	if !strings.HasPrefix(contents.Raw, "<") {
		t.Errorf("signature /Contents raw = %q, want the byte-exact hex form", contents.Raw)
	}
}
