package pdfcore

// Format groups a flat token slice into one FormattedLine per logical PDF
// operation (operands followed by an operator), tracking q/Q nesting depth
// for indent and merging inline-image BI..ID..<bytes>..EI sequences into a
// single line.
//
// PDF content streams are operand-then-operator (RPN-like). For
// `10 0 0 10 30 761 Tm` the formatter accumulates the six numbers as operands,
// then on hitting Tm flushes one FormattedLine{Tokens=[...numbers..., Tm],
// Operator="Tm"}. Comments are emitted on their own line (Operator=""). A
// trailing run of operands without an operator is tolerated and emitted as a
// final dangling line (Operator="") iff non-empty.
//
// SrcLineStart/SrcLineEnd are inclusive 1-based source byte-line indices,
// computed as the min and max Token.Line across the line's tokens. The
// frontend uses this range to scroll-sync the formatted view with the raw
// view.
//
// Indent rules (v1):
//   - q -> indent++ AFTER the q line is emitted (so q itself sits at the
//     enclosing indent and its body is one level deeper).
//   - Q -> indent-- BEFORE the Q line is emitted (so Q sits at the
//     enclosing indent, not nested). Floor-clamped at zero on extra Q.
//   - BT/ET, BMC/EMC, BDC/EMC are NOT indented in v1; defer to v2 if user
//     feedback warrants.
//
// Inline images (BI..EI) collapse into one line. Per the tokenizer contract
// (locked by TestTokenizeInlineImagePayloadOpaque), the binary payload is
// delivered as a single string token between the ID and EI operator tokens,
// so the formatter just consumes from BI through the matching EI.
//
// The function does not error: malformed streams surface at tokenize time,
// and dangling operands are emitted rather than rejected.
func Format(tokens []Token) []FormattedLine {
	if len(tokens) == 0 {
		return []FormattedLine{}
	}

	lines := make([]FormattedLine, 0, len(tokens)/4+1)
	indent := 0
	var pending []Token
	i := 0
	for i < len(tokens) {
		tk := tokens[i]

		// Comments are their own line, regardless of any pending operands.
		// PDF content streams put comments on their own line by convention,
		// and treating them as operators-of-their-own keeps the model simple.
		if tk.Type == "comment" {
			if len(pending) > 0 {
				lines = append(lines, dangling(pending, indent))
				pending = nil
			}
			lines = append(lines, FormattedLine{
				Tokens:       []Token{tk},
				Indent:       indent,
				Operator:     "",
				SrcLineStart: tk.Line,
				SrcLineEnd:   tk.Line,
			})
			i++
			continue
		}

		if tk.Type != "operator" {
			pending = append(pending, tk)
			i++
			continue
		}

		// Inline image: BI swallows everything through the matching EI into
		// one line. The tokenizer guarantees the EI is the literal whitespace-
		// bounded terminator (not a literal "EI" inside the binary payload).
		if tk.Value == "BI" {
			start := i
			j := i + 1
			for j < len(tokens) {
				if tokens[j].Type == "operator" && tokens[j].Value == "EI" {
					break
				}
				j++
			}
			// Include EI if present; otherwise fall through to end-of-slice.
			end := j
			if end < len(tokens) {
				end = j + 1
			}
			block := append([]Token(nil), pending...)
			block = append(block, tokens[start:end]...)
			lines = append(lines, FormattedLine{
				Tokens:       block,
				Indent:       indent,
				Operator:     "BI",
				SrcLineStart: lineMinMax(block, true),
				SrcLineEnd:   lineMinMax(block, false),
			})
			pending = nil
			i = end
			continue
		}

		// Q dedents BEFORE its own line is emitted, so Q sits at the
		// enclosing indent rather than the nested-one.
		if tk.Value == "Q" {
			if indent > 0 {
				indent--
			}
		}

		row := append([]Token(nil), pending...)
		row = append(row, tk)
		lines = append(lines, FormattedLine{
			Tokens:       row,
			Indent:       indent,
			Operator:     tk.Value,
			SrcLineStart: lineMinMax(row, true),
			SrcLineEnd:   lineMinMax(row, false),
		})
		pending = nil

		// q indents AFTER its own line is emitted, so q sits at the enclosing
		// indent and the next line is one level deeper.
		if tk.Value == "q" {
			indent++
		}
		i++
	}

	// Trailing operands without an operator: emit as one dangling line.
	if len(pending) > 0 {
		lines = append(lines, dangling(pending, indent))
	}

	return lines
}

// dangling builds a FormattedLine for a tail run of operands that never met
// their operator. Operator is empty.
func dangling(toks []Token, indent int) FormattedLine {
	row := append([]Token(nil), toks...)
	return FormattedLine{
		Tokens:       row,
		Indent:       indent,
		Operator:     "",
		SrcLineStart: lineMinMax(row, true),
		SrcLineEnd:   lineMinMax(row, false),
	}
}

// lineMinMax returns the min (when wantMin) or max Token.Line across toks.
// Returns 0 for an empty slice; tokens are guaranteed non-empty by callers.
func lineMinMax(toks []Token, wantMin bool) int {
	if len(toks) == 0 {
		return 0
	}
	v := toks[0].Line
	for _, t := range toks[1:] {
		if wantMin && t.Line < v {
			v = t.Line
		}
		if !wantMin && t.Line > v {
			v = t.Line
		}
	}
	return v
}
