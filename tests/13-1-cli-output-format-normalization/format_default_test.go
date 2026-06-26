package cli_output_format_normalization_test

import (
	"strings"
	"testing"
)

// cmdInvocation is one dump command exercised by the cross-cutting format
// tests. args are everything AFTER the binary path EXCEPT --json/--pretty and
// the file path; the file path is appended last so we can splice format flags
// in between. wantJSONTop is the byte the JSON document must start with ('{' or
// '[').
type cmdInvocation struct {
	id          string // command label for assertion messages
	args        []string
	file        string // relative fixture path
	wantJSONTop byte
}

// formatCommands spans all four distinct flag-parsing paths the story names so
// the uniform --json rule is proven at every site, not just one:
//   - parseDumpFlags:    tree
//   - inline FlagSet:    object, stream, page
//   - parseDocViewFlags: objects, xref
//   - parseByRefFlags:   font, image, source, reverserefs
//
// plaintext is excluded here (AC5: its default is RAW bytes by design, covered
// in plaintext_test.go). stream --ops/--raw payload axis is covered in
// stream_test.go.
func formatCommands(t *testing.T) []cmdInvocation {
	return []cmdInvocation{
		{"tree", []string{"dump", "tree"}, "minimal.pdf", '{'},
		{"object", []string{"dump", "object", "--ref", "1 0 R"}, "minimal.pdf", '{'},
		{"objects", []string{"dump", "objects"}, "minimal.pdf", '['},
		{"xref", []string{"dump", "xref"}, "minimal.pdf", '{'},
		{"font", []string{"dump", "font", "--ref", "4 0 R"}, "fonts-mixed.pdf", '{'},
		{"image", []string{"dump", "image", "--metadata", "--ref", "4 0 R"}, "image-xobject.pdf", '{'},
		{"source", []string{"dump", "source", "--ref", "1 0 R"}, "minimal.pdf", '{'},
		{"reverserefs", []string{"dump", "reverserefs", "--ref", "2 0 R"}, "minimal.pdf", '['},
		{"stream", []string{"dump", "stream", "--page", "1"}, "content-stream.pdf", '{'},
		{"page", []string{"dump", "page", "--info", "1"}, "page-render/render-info.pdf", '{'},
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-001 [P0] FORMAT-001: Default (no --json) emits PLAIN TEXT, not JSON.
// AC#1: Absence of --json -> human-readable plain text on stdout. No command
// emits JSON by default.
// ---------------------------------------------------------------------------

func TestFormat_DefaultIsPlainTextNotJSON(t *testing.T) {
	bin := buildCLI(t)
	for _, c := range formatCommands(t) {
		c := c
		t.Run(c.id, func(t *testing.T) {
			args := append(append([]string{}, c.args...), fixture(t, c.file))
			stdout, stderr, exit := runCLI(t, bin, args...)
			if exit != 0 {
				t.Fatalf("[P0] 13.1-INTG-001 (%s): expected exit 0, got %d (stderr: %s)", c.id, exit, stderr)
			}
			assertNotJSON(t, "13.1-INTG-001 "+c.id, stdout)
		})
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-002 [P0] FORMAT-002: --json emits a parseable JSON document.
// AC#1: Presence of --json -> structured JSON via the existing emit path.
// ---------------------------------------------------------------------------

func TestFormat_JSONFlagEmitsJSON(t *testing.T) {
	bin := buildCLI(t)
	for _, c := range formatCommands(t) {
		c := c
		t.Run(c.id, func(t *testing.T) {
			args := append(append([]string{}, c.args...), "--json", fixture(t, c.file))
			stdout, stderr, exit := runCLI(t, bin, args...)
			if exit != 0 {
				t.Fatalf("[P0] 13.1-INTG-002 (%s): expected exit 0, got %d (stderr: %s)", c.id, exit, stderr)
			}
			trimmed := strings.TrimSpace(stdout)
			if trimmed == "" {
				t.Fatalf("[P0] 13.1-INTG-002 (%s): --json produced empty stdout", c.id)
			}
			if trimmed[0] != c.wantJSONTop {
				t.Errorf("[P0] 13.1-INTG-002 (%s): --json output starts with %q, want %q\n%s",
					c.id, trimmed[0], c.wantJSONTop, stdout)
			}
			if !parsesAsJSON(trimmed) {
				t.Errorf("[P0] 13.1-INTG-002 (%s): --json output did not parse as JSON:\n%s", c.id, stdout)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-003 [P0] FORMAT-001b: Plain-text default is ASCII-only and ends
// with a trailing newline.
// AC#2: Output is ASCII-only and ends with a trailing newline.
// ---------------------------------------------------------------------------

func TestFormat_PlainTextIsASCIIWithTrailingNewline(t *testing.T) {
	bin := buildCLI(t)
	for _, c := range formatCommands(t) {
		c := c
		t.Run(c.id, func(t *testing.T) {
			args := append(append([]string{}, c.args...), fixture(t, c.file))
			stdout, stderr, exit := runCLI(t, bin, args...)
			if exit != 0 {
				t.Fatalf("[P0] 13.1-INTG-003 (%s): expected exit 0, got %d (stderr: %s)", c.id, exit, stderr)
			}
			assertASCII(t, "13.1-INTG-003 "+c.id, stdout)
			assertTrailingNewline(t, "13.1-INTG-003 "+c.id, stdout)
		})
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-004 [P1] FORMAT-003a: --pretty indents the --json output (multi-line).
// AC#4: --pretty continues to apply to --json (emit unchanged).
// ---------------------------------------------------------------------------

func TestFormat_PrettyIndentsJSON(t *testing.T) {
	bin := buildCLI(t)
	// Use a command whose JSON is a non-trivial object so indentation produces
	// multiple lines.
	file := fixture(t, "minimal.pdf")

	compact, _, ec := runCLI(t, bin, "dump", "tree", "--json", file)
	if ec != 0 {
		t.Fatalf("[P1] 13.1-INTG-004: --json exit %d", ec)
	}
	pretty, _, ec := runCLI(t, bin, "dump", "tree", "--json", "--pretty", file)
	if ec != 0 {
		t.Fatalf("[P1] 13.1-INTG-004: --json --pretty exit %d", ec)
	}

	compactLines := strings.Count(strings.TrimRight(compact, "\n"), "\n") + 1
	prettyLines := strings.Count(strings.TrimRight(pretty, "\n"), "\n") + 1
	if prettyLines <= compactLines {
		t.Errorf("[P1] 13.1-INTG-004: --pretty did not indent JSON (compact lines=%d, pretty lines=%d)",
			compactLines, prettyLines)
	}
	// The indented form must contain an indented line (two-space JSON indent).
	if !strings.Contains(pretty, "\n  ") {
		t.Errorf("[P1] 13.1-INTG-004: --pretty JSON has no indented line:\n%s", pretty)
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-005 [P1] FORMAT-003b: --pretty WITHOUT --json is accepted (exit 0)
// and has NO effect on plain text.
// AC#4: --pretty without --json is accepted and has no effect on plain text.
// ---------------------------------------------------------------------------

func TestFormat_PrettyWithoutJSONNoEffectOnPlainText(t *testing.T) {
	bin := buildCLI(t)
	for _, c := range formatCommands(t) {
		c := c
		t.Run(c.id, func(t *testing.T) {
			file := fixture(t, c.file)
			plain, _, ec1 := runCLI(t, bin, append(append([]string{}, c.args...), file)...)
			if ec1 != 0 {
				t.Fatalf("[P1] 13.1-INTG-005 (%s): plain exit %d", c.id, ec1)
			}
			withPretty, stderr, ec2 := runCLI(t, bin, append(append([]string{}, c.args...), "--pretty", file)...)
			if ec2 != 0 {
				t.Fatalf("[P1] 13.1-INTG-005 (%s): --pretty (no --json) must be accepted, got exit %d (stderr: %s)",
					c.id, ec2, stderr)
			}
			if withPretty != plain {
				t.Errorf("[P1] 13.1-INTG-005 (%s): --pretty changed plain-text output (should be a no-op without --json)\nplain:\n%s\n--pretty:\n%s",
					c.id, plain, withPretty)
			}
		})
	}
}
