package object_tree_scalar_values_test

import (
	"fmt"
	"strings"
	"testing"
)

// plainTextCap is the fixed number of runes a plain-text row emits before it
// elides. Fixed rather than terminal-width-adaptive so piped output and CI
// output agree.
const plainTextCap = 80

// truncationMarker renders the marker an elided row carries: n runes emitted of
// m the whole escaped value has.
func truncationMarker(n, m int) string {
	return fmt.Sprintf("[truncated: %d of %d]", n, m)
}

// The cap is measured in runes over the escaped value, so escaping happens
// first and a cut never lands between a backslash and its letter.

func TestCap_BelowCeilingIsNotElided(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	row := oneLineStartingWith(t, out, "CapA string = ")
	if row != "CapA string = "+strings.Repeat("x", 79) {
		t.Errorf("79-rune value was altered: %q", row)
	}
	if strings.Contains(row, "[truncated") {
		t.Errorf("79-rune value carries a truncation marker: %q", row)
	}
}

func TestCap_AtCeilingIsNotElided(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	row := oneLineStartingWith(t, out, "CapB string = ")
	if row != "CapB string = "+strings.Repeat("x", plainTextCap) {
		t.Errorf("value at the ceiling was altered: %q", row)
	}
	if strings.Contains(row, "[truncated") {
		t.Errorf("value at the ceiling carries a truncation marker: %q", row)
	}
}

func TestCap_AboveCeilingIsElidedAndMarked(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	row := oneLineStartingWith(t, out, "CapC string = ")
	want := "CapC string = " + strings.Repeat("x", plainTextCap) + " " + truncationMarker(plainTextCap, 81)
	if row != want {
		t.Errorf("elided row =\n  %q\nwant\n  %q", row, want)
	}
}

func TestCap_CountsRunesNotBytes(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	row := oneLineStartingWith(t, out, "CapMulti string = ")
	want := "CapMulti string = " + strings.Repeat(cjkRune, plainTextCap) + " " +
		truncationMarker(plainTextCap, capMultiRunes)
	if row != want {
		t.Errorf("multi-byte row =\n  %q\nwant\n  %q", row, want)
	}
}

func TestCap_NeverCutsAnEscapeSequenceInHalf(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	// 41 tab characters escape to 82 runes, so the cut lands after the 40th
	// escape sequence and the 41st is dropped whole.
	row := oneLineStartingWith(t, out, "CapEscaped string = ")
	want := "CapEscaped string = " + strings.Repeat(`\t`, 40) + " " + truncationMarker(plainTextCap, 82)
	if row != want {
		t.Errorf("escaped-value row =\n  %q\nwant\n  %q", row, want)
	}
}

func TestCap_JSONCarriesTheFullValue(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	caps := nodeAt(t, root, "/Caps")

	if got := mustValue(t, nodeAt(t, caps, "/CapC")); got != strings.Repeat("x", 81) {
		t.Errorf("--json capped the value: got %d runes, want 81", len([]rune(got)))
	}
	if got := mustValue(t, nodeAt(t, caps, "/CapMulti")); got != strings.Repeat(cjkRune, capMultiRunes) {
		t.Errorf("--json capped the multi-byte value: got %d runes, want %d",
			len([]rune(got)), capMultiRunes)
	}
	for _, key := range []string{"/CapC", "/CapMulti", "/CapEscaped"} {
		if strings.Contains(mustValue(t, nodeAt(t, caps, key)), "[truncated") {
			t.Errorf("--json %s carries a truncation marker; the machine contract is uncapped", key)
		}
	}
}

// Control characters are escaped so one node is always exactly one output line.

func TestEscaping_ControlCharactersKeepOneNodeOnOneLine(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	want := map[string]string{
		"Newline":    `Newline string = a\nb`,
		"Return":     `Return string = a\rb`,
		"Tab":        `Tab string = a\tb`,
		"Backslash2": `Backslash2 string = a\\b`,
		"Nul":        `Nul string = a\x00b`,
		"Curly":      `Curly string = don\x92t`,
	}
	for key, row := range want {
		if !hasLine(out, row) {
			t.Errorf("dump tree has no row %q for /%s\n--- output ---\n%s", row, key, out)
		}
	}
}

func TestEscaping_NoRawControlCharacterReachesTheOutput(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	for _, line := range strings.Split(out, "\n") {
		for _, r := range line {
			if r == '\r' || r == '\t' || (r < 0x20) || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
				t.Fatalf("unescaped control character U+%04X reached the output on line %q", r, line)
			}
		}
	}
}

func TestEscaping_DumpObjectRowsStayOnOneLine(t *testing.T) {
	// The same defect lives in the object detail view: a /Alt carrying newlines
	// renders its continuation lines at column zero today.
	stdout, stderr, code := runCLI(t, "dump", "object", "--ref", "6 0 R", fixturePath(t, "scalar-values.pdf"))
	if code != 0 {
		t.Fatalf("dump object exited %d: %s", code, stderr)
	}

	for _, line := range strings.Split(stdout, "\n") {
		for _, r := range line {
			if r == '\r' || r == '\t' || (r < 0x20) || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
				t.Fatalf("unescaped control character U+%04X reached dump object on line %q", r, line)
			}
		}
	}
	if got := objectPropertyLine(t, stdout, "/Newline"); got != `a\nb` {
		t.Errorf("dump object /Newline = %q, want the escaped %q", got, `a\nb`)
	}
}

func TestEscaping_JSONCarriesTheUnescapedString(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	controls := nodeAt(t, root, "/Controls")

	cases := map[string]string{
		"/Newline":    "a\nb",
		"/Return":     "a\rb",
		"/Tab":        "a\tb",
		"/Backslash2": `a\b`,
		"/Nul":        "a\x00b",
		"/Curly":      "don\u0092t",
	}
	for key, want := range cases {
		if got := mustValue(t, nodeAt(t, controls, key)); got != want {
			t.Errorf("--json %s value = %q, want the unescaped %q", key, got, want)
		}
	}
}
