package main

import (
	"bytes"
	"strings"
	"testing"
)

// descriptionColumn is the 1-based column where every single-line command
// description in printUsage's Commands block starts.
const descriptionColumn = 53

// usageText renders printUsage into a string.
func usageText(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	printUsage(&buf)
	return buf.String()
}

// commandLine returns the Commands-block line whose synopsis starts with
// prefix, e.g. "dump bytes ".
func commandLine(t *testing.T, help, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(help, "\n") {
		if strings.HasPrefix(line, "  "+prefix) {
			return line
		}
	}
	t.Fatalf("help text has no command line starting with %q", prefix)
	return ""
}

// descriptionStart returns the 1-based column where a command line's
// description begins: the first non-space after the two-or-more space gap that
// closes the synopsis.
func descriptionStart(t *testing.T, line string) int {
	t.Helper()
	gap := strings.Index(line[2:], "  ")
	if gap < 0 {
		t.Fatalf("command line has no synopsis/description gap:\n%s", line)
	}
	rest := line[2+gap:]
	return 2 + gap + len(rest) - len(strings.TrimLeft(rest, " ")) + 1
}

// The `dump bytes` help line carries the whole corrective sentence, not just
// the pdftotext pointer. "raw document bytes" says what the command emits and
// "not extracted page text" says what it does not, which is the half a reader
// arriving from the old name needs; a reword that kept only the pointer would
// leave the misreading the rename exists to fix.
func TestPrintUsage_BytesLineDescribesRawBytesNotPageText(t *testing.T) {
	line := commandLine(t, usageText(t), "dump bytes ")

	for _, want := range []string{"raw document bytes", "not extracted page text", "pdftotext"} {
		if !strings.Contains(line, want) {
			t.Errorf("the `dump bytes` help line should say %q:\n%s", want, line)
		}
	}
}

// The bytes line holds the Commands block's alignment: it has the longest
// description in the single-line group, so a synopsis edit that overruns the
// shared column shows up here rather than as a ragged help screen. The sibling
// lines are checked alongside it so a deliberate re-alignment of the whole
// block reads as one failure rather than one line looking wrong.
func TestPrintUsage_CommandDescriptionsShareOneColumn(t *testing.T) {
	help := usageText(t)

	prefixes := []string{
		"dump bytes ",
		"dump xref ",
		"dump objects ",
		"dump metadata ",
		"dump signatures ",
	}
	for _, prefix := range prefixes {
		line := commandLine(t, help, prefix)
		if got := descriptionStart(t, line); got != descriptionColumn {
			t.Errorf("`%s` description starts at column %d, expected %d:\n%s",
				strings.TrimSpace(prefix), got, descriptionColumn, line)
		}
	}
}
