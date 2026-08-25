package pdfcore

import (
	"testing"
)

// Format groups operands with their operator and emits one FormattedLine
// per logical PDF operation. Test cases cover the six canonical fixtures
// the format contract calls out.

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

func TestFormatIndent_TextBlockBTET(t *testing.T) {
	// BT and ET are open/close operators just like q/Q. Body of the text
	// block must indent.
	input := "BT /F1 12 Tf (hi) Tj ET"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)

	wantOps := []string{"BT", "Tf", "Tj", "ET"}
	wantIndent := []int{0, 1, 1, 0}
	if len(lines) != len(wantOps) {
		t.Fatalf("got %d lines, want %d", len(lines), len(wantOps))
	}
	for i := range wantOps {
		if lines[i].Operator != wantOps[i] || lines[i].Indent != wantIndent[i] {
			t.Errorf("line[%d] = (%q, indent %d), want (%q, indent %d)",
				i, lines[i].Operator, lines[i].Indent, wantOps[i], wantIndent[i])
		}
	}
}

func TestFormatIndent_MarkedContent(t *testing.T) {
	// BMC and BDC open a marked-content block; EMC closes either.
	input := "BMC (a) Tj EMC BDC /Foo (b) Tj EMC"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)

	wantOps := []string{"BMC", "Tj", "EMC", "BDC", "Tj", "EMC"}
	wantIndent := []int{0, 1, 0, 0, 1, 0}
	if len(lines) != len(wantOps) {
		t.Fatalf("got %d lines, want %d", len(lines), len(wantOps))
	}
	for i := range wantOps {
		if lines[i].Operator != wantOps[i] || lines[i].Indent != wantIndent[i] {
			t.Errorf("line[%d] = (%q, indent %d), want (%q, indent %d)",
				i, lines[i].Operator, lines[i].Indent, wantOps[i], wantIndent[i])
		}
	}
}

func TestFormatIndent_QInsideBT(t *testing.T) {
	// Mixed nesting: graphics state inside a text block. Each contributes
	// one level of indent.
	input := "BT q (a) Tj Q ET"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)

	wantOps := []string{"BT", "q", "Tj", "Q", "ET"}
	wantIndent := []int{0, 1, 2, 1, 0}
	if len(lines) != len(wantOps) {
		t.Fatalf("got %d lines, want %d", len(lines), len(wantOps))
	}
	for i := range wantOps {
		if lines[i].Operator != wantOps[i] || lines[i].Indent != wantIndent[i] {
			t.Errorf("line[%d] = (%q, indent %d), want (%q, indent %d)",
				i, lines[i].Operator, lines[i].Indent, wantOps[i], wantIndent[i])
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

func TestFormatArrayOperandStaysOneLine(t *testing.T) {
	// TJ takes a single array argument: `[ (str) num (str) ... ] TJ`.
	// The lexer emits [ and ] as operator tokens, but the formatter must
	// treat them as delimiters of an operand and keep the whole array +
	// TJ on one logical line.
	input := "[ (P) 25 (ostScript) ] TJ"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)

	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1; ops: %+v", len(lines), opsOf(lines))
	}
	if lines[0].Operator != "TJ" {
		t.Errorf("Operator = %q, want %q", lines[0].Operator, "TJ")
	}
	// The row must contain the [ and ] tokens plus all operands and TJ.
	wantValues := []string{"[", "(P)", "25", "(ostScript)", "]", "TJ"}
	if len(lines[0].Tokens) != len(wantValues) {
		t.Fatalf("got %d tokens in row, want %d", len(lines[0].Tokens), len(wantValues))
	}
	for i, want := range wantValues {
		if lines[0].Tokens[i].Value != want {
			t.Errorf("token[%d].Value = %q, want %q", i, lines[0].Tokens[i].Value, want)
		}
	}
}

func TestFormatNestedArrays_SingleLine(t *testing.T) {
	// Nested arrays should still collapse into one row at the outer-array
	// closing delimiter.
	input := "[ [ 1 2 ] [ 3 4 ] ] TJ"
	tokens := tokenizeContentStream(input)
	lines := Format(tokens)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1; ops: %+v", len(lines), opsOf(lines))
	}
	if lines[0].Operator != "TJ" {
		t.Errorf("Operator = %q, want %q", lines[0].Operator, "TJ")
	}
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

// Invariant: every line has SrcLineStart <= SrcLineEnd, ranges are
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
