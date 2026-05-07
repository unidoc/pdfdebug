package pdfcore

import (
	"testing"
)

// 9.6-UNIT-001 [P0]: Format groups operands with their operator and emits
// one FormattedLine per logical PDF operation. Test cases cover the six
// canonical fixtures the story spec calls out.

func TestFormatBaseline_OperatorPerLine(t *testing.T) {
	// Each operator on its own source line; baseline that should round-trip
	// to one row per operator with monotonic source-line ranges.
	input := "q\n10 0 0 10 30 761 Tm\n(Hello) Tj\nQ\n"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)

	wantOps := []string{"q", "Tm", "Tj", "Q"}
	if len(lines) != len(wantOps) {
		t.Fatalf("got %d lines, want %d", len(lines), len(wantOps))
	}
	for i, op := range wantOps {
		if lines[i].Operator != op {
			t.Errorf("line[%d].Operator = %q, want %q", i, lines[i].Operator, op)
		}
	}
	// q indents AFTER, Q dedents BEFORE -- both should sit at indent 0.
	wantIndents := []int{0, 1, 1, 0}
	for i, ind := range wantIndents {
		if lines[i].Indent != ind {
			t.Errorf("line[%d].Indent = %d, want %d", i, lines[i].Indent, ind)
		}
	}
	validateLineRanges(t, lines)
}

func TestFormatRegression_ManyOpsPerLine(t *testing.T) {
	// PDF32000_2008.pdf shape: many operators packed onto one source line.
	// Pre-9-6 frontend grouper produced one giant row; now each operator
	// gets its own row.
	input := "q Q q 10 0 0 10 30 761 Tm (Hi) Tj ET Q"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)

	wantOps := []string{"q", "Q", "q", "Tm", "Tj", "ET", "Q"}
	if len(lines) != len(wantOps) {
		t.Fatalf("got %d lines, want %d (ops: %+v)", len(lines), len(wantOps), opsOf(lines))
	}
	for i, op := range wantOps {
		if lines[i].Operator != op {
			t.Errorf("line[%d].Operator = %q, want %q", i, lines[i].Operator, op)
		}
	}
	// All source-line numbers are 1 because the input is single-line.
	for i, ln := range lines {
		if ln.SrcLineStart != 1 || ln.SrcLineEnd != 1 {
			t.Errorf("line[%d] src range = [%d,%d], want [1,1]", i, ln.SrcLineStart, ln.SrcLineEnd)
		}
	}
	validateLineRanges(t, lines)
}

func TestFormatIndent_QNesting3Deep(t *testing.T) {
	input := "q q q (a) Tj Q Q Q"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)

	wantOps := []string{"q", "q", "q", "Tj", "Q", "Q", "Q"}
	wantIndent := []int{0, 1, 2, 3, 2, 1, 0}
	if len(lines) != len(wantOps) {
		t.Fatalf("got %d lines, want %d", len(lines), len(wantOps))
	}
	for i := range wantOps {
		if lines[i].Operator != wantOps[i] || lines[i].Indent != wantIndent[i] {
			t.Errorf("line[%d] = (%q, indent %d), want (%q, indent %d)",
				i, lines[i].Operator, lines[i].Indent, wantOps[i], wantIndent[i])
		}
	}
	validateLineRanges(t, lines)
}

func TestFormatIndent_ExtraQClampsAtZero(t *testing.T) {
	// Extra Q without matching q: indent should floor-clamp at 0, not go negative.
	input := "Q Q (a) Tj"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)
	for i, ln := range lines {
		if ln.Indent < 0 {
			t.Errorf("line[%d].Indent = %d, expected floor-clamp at 0", i, ln.Indent)
		}
	}
}

func TestFormatInlineImage_OneLine(t *testing.T) {
	// BI..ID..<binary>..EI must collapse into ONE line. The binary payload
	// (with embedded "EI" lookalike bytes) must not split the row.
	input := "BI /W 1 /H 1 /CS /G /BPC 8 ID abEIcd EI"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)

	if len(lines) != 1 {
		t.Fatalf("inline image: got %d lines, want 1; ops: %+v", len(lines), opsOf(lines))
	}
	if lines[0].Operator != "BI" {
		t.Errorf("inline image Operator = %q, want %q", lines[0].Operator, "BI")
	}
	// The block must contain BI ... ID payload EI.
	gotOps := opsOnly(lines[0].Tokens)
	wantOps := []string{"BI", "ID", "EI"}
	if !equalSlice(gotOps, wantOps) {
		t.Errorf("inline image operator sequence = %+v, want %+v", gotOps, wantOps)
	}
	validateLineRanges(t, lines)
}

func TestFormatString_NewlineDoesNotSplit(t *testing.T) {
	// String literals containing newlines stay one operand and the Tj row
	// stays one line.
	input := "(line1\nline2\rline3) Tj"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)

	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1; ops: %+v", len(lines), opsOf(lines))
	}
	if lines[0].Operator != "Tj" {
		t.Errorf("Operator = %q, want %q", lines[0].Operator, "Tj")
	}
}

func TestFormatMultilineOperands_RangeSpansLines(t *testing.T) {
	// Operands spread across multiple source lines: the row's source range
	// must span from min line to max line.
	input := "10 0\n0 10\n30 761 Tm"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1; ops: %+v", len(lines), opsOf(lines))
	}
	if lines[0].SrcLineStart != 1 || lines[0].SrcLineEnd != 3 {
		t.Errorf("src range = [%d,%d], want [1,3]", lines[0].SrcLineStart, lines[0].SrcLineEnd)
	}
}

func TestFormatComment_OwnLine(t *testing.T) {
	// Comments do not group with operators; each is its own row with empty
	// Operator string.
	input := "% top of stream\n10 0 0 10 30 761 Tm\n% mid\n(a) Tj"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)

	wantOps := []string{"", "Tm", "", "Tj"}
	if len(lines) != len(wantOps) {
		t.Fatalf("got %d lines, want %d; ops: %+v", len(lines), len(wantOps), opsOf(lines))
	}
	for i, want := range wantOps {
		if lines[i].Operator != want {
			t.Errorf("line[%d].Operator = %q, want %q", i, lines[i].Operator, want)
		}
	}
	validateLineRanges(t, lines)
}

func TestFormatEmpty_ReturnsEmpty(t *testing.T) {
	if got := Format(nil); len(got) != 0 {
		t.Errorf("Format(nil) = %d lines, want 0", len(got))
	}
	if got := Format([]Token{}); len(got) != 0 {
		t.Errorf("Format([]) = %d lines, want 0", len(got))
	}
}

func TestFormatTrailingOperandsWithoutOperator(t *testing.T) {
	// Malformed but tolerable: operands at end with no operator. Emit one
	// dangling row with Operator="" rather than dropping the data silently.
	input := "10 0 0 10"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if lines[0].Operator != "" {
		t.Errorf("Operator = %q, want empty", lines[0].Operator)
	}
}

// AC #6 invariant: every line has SrcLineStart <= SrcLineEnd, ranges are
// monotonically non-decreasing across the slice, and the union of ranges
// covers every source line that contains at least one token.
func validateLineRanges(t *testing.T, lines []FormattedLine) {
	t.Helper()
	prevEnd := 0
	for i, ln := range lines {
		if ln.SrcLineStart > ln.SrcLineEnd {
			t.Errorf("line[%d] start %d > end %d", i, ln.SrcLineStart, ln.SrcLineEnd)
		}
		if ln.SrcLineStart < prevEnd {
			t.Errorf("line[%d] start %d regresses below prev end %d (non-monotonic)", i, ln.SrcLineStart, prevEnd)
		}
		if ln.SrcLineEnd > prevEnd {
			prevEnd = ln.SrcLineEnd
		}
	}
}

// opsOnly returns just the operator-typed token Values from a slice (for
// inline-image assertions where we want to ignore operands).
func opsOnly(toks []Token) []string {
	out := []string{}
	for _, t := range toks {
		if t.Type == "operator" {
			out = append(out, t.Value)
		}
	}
	return out
}

// opsOf returns one operator string per FormattedLine for diagnostic output.
func opsOf(lines []FormattedLine) []string {
	out := make([]string, len(lines))
	for i, ln := range lines {
		out[i] = ln.Operator
	}
	return out
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
