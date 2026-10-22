package pdfcore

import (
	"fmt"
	"strconv"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// mcOpen is one open marked-content sequence: its MCID, when it has one.
type mcOpen struct {
	mcid    int
	hasMCID bool
}

// mcState is the stack of open marked-content sequences, carried from one
// content stream of a page to the next.
type mcState struct {
	stack []mcOpen
}

// propertyLookup resolves a named property list (BDC with a name operand) in
// the page's /Resources /Properties and returns its /MCID value, nil when the
// list has none. An error means the name was not found, the entry is not a
// dictionary, or its /MCID does not resolve.
type propertyLookup func(name string) (mcid pdfcpu_types.Object, err error)

// scanMarkedContent counts BMC/BDC/EMC in one stream's tokens into ms and
// returns a row for every BDC whose property list carries an /MCID. st is
// updated in place so nesting carries into the next stream.
//
// The tokenizer types delimiters and the keywords true, false and null as
// operators, so operands are collected by value: <<, >>, [, ], true, false and
// null are operands, and any other operator ends the operand list. A stray <<
// or [ therefore cannot swallow the rest of the stream.
func scanMarkedContent(tokens []Token, streamIdx int, props propertyLookup, st *mcState, ms *MarkedStream) []ContentMCID {
	var rows []ContentMCID
	ms.MinDepth = len(st.stack)
	var operands []Token
	for _, tok := range tokens {
		if tok.Type == "comment" {
			continue
		}
		if tok.Type != "operator" {
			operands = append(operands, tok)
			continue
		}
		switch tok.Value {
		case "<<", ">>", "[", "]", "true", "false", "null":
			operands = append(operands, tok)
			continue
		}
		switch tok.Value {
		case "BMC":
			ms.Opens++
			st.stack = append(st.stack, mcOpen{})
		case "BDC":
			ms.Opens++
			row, ok := bdcRow(operands, tok, props)
			parent := innermostMCID(st.stack)
			open := mcOpen{}
			if ok {
				row.Stream = streamIdx
				row.ParentMCID = parent
				if row.Err == "" {
					open = mcOpen{mcid: row.MCID, hasMCID: true}
				}
			}
			st.stack = append(st.stack, open)
			if ok {
				row.Depth = len(st.stack)
				rows = append(rows, row)
			}
		case "EMC":
			ms.Closes++
			if len(st.stack) == 0 {
				ms.MinDepth = min(ms.MinDepth, -1)
			} else {
				st.stack = st.stack[:len(st.stack)-1]
				ms.MinDepth = min(ms.MinDepth, len(st.stack))
			}
		}
		operands = operands[:0]
	}
	ms.DepthAtEnd = len(st.stack)
	return rows
}

// innermostMCID returns the MCID of the innermost open sequence that has
// one, or nil.
func innermostMCID(stack []mcOpen) *int {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].hasMCID {
			v := stack[i].mcid
			return &v
		}
	}
	return nil
}

// bdcRow reads a BDC's operands (tag, then an inline dictionary or a property
// list name). ok is false when the property list has no /MCID, so the BDC
// gets no row. Operands that are not a tag name followed by a dictionary or a
// name, a property list that cannot be read, or an /MCID that is not a
// non-negative integer give a row with MCID -1 and Err set.
func bdcRow(operands []Token, op Token, props propertyLookup) (row ContentMCID, ok bool) {
	pos := op
	if len(operands) > 0 {
		pos = operands[0]
	}
	row = ContentMCID{MCID: -1, Line: pos.Line, Col: pos.Col}
	if len(operands) > 0 && operands[0].Type == "name" {
		row.Tag = strings.TrimPrefix(operands[0].Value, "/")
	}
	if len(operands) < 2 {
		row.Err = fmt.Sprintf("BDC has %d of its 2 operands (a tag and a property list)", len(operands))
		return row, true
	}
	if operands[0].Type != "name" {
		row.Err = fmt.Sprintf("BDC operands start with %q, not a tag name; property list not read", operands[0].Value)
		return row, true
	}
	prop := operands[1]
	switch {
	case prop.Type == "operator" && prop.Value == "<<":
		val, found := inlineMCID(operands[1:])
		if !found {
			return row, false
		}
		if val.Type == "number" {
			if n, err := strconv.Atoi(val.Value); err == nil {
				setMCID(&row, n)
				return row, true
			}
		}
		row.Err = fmt.Sprintf("/MCID %s is not an integer", val.Value)
		return row, true
	case prop.Type == "name":
		name := strings.TrimPrefix(prop.Value, "/")
		mcid, err := props(name)
		if err != nil {
			row.Err = err.Error()
			return row, true
		}
		if mcid == nil {
			return row, false
		}
		if n, isInt := mcid.(pdfcpu_types.Integer); isInt {
			setMCID(&row, int(n))
		} else {
			row.Err = fmt.Sprintf("/MCID in property list /%s is %s, not an integer", name, objKind(mcid))
		}
		return row, true
	}
	row.Err = fmt.Sprintf("BDC property list operand %q is not a dictionary or a name", prop.Value)
	return row, true
}

// setMCID stores a non-negative n as the row's MCID, or an error for a
// negative one.
func setMCID(row *ContentMCID, n int) {
	if n < 0 {
		row.Err = fmt.Sprintf("/MCID %d is negative", n)
		return
	}
	row.MCID = n
}

// inlineMCID finds the value token of the top-level /MCID key in an inline
// dictionary's tokens, which start at its "<<". A container value yields its
// opening token.
func inlineMCID(toks []Token) (Token, bool) {
	depth := 0
	expectKey := true
	key := ""
	for _, t := range toks {
		isOp := t.Type == "operator"
		opening := isOp && (t.Value == "<<" || t.Value == "[")
		closing := isOp && (t.Value == ">>" || t.Value == "]")
		switch depth {
		case 0:
			if opening {
				depth = 1
			}
		case 1:
			switch {
			case closing:
				return Token{}, false
			case expectKey:
				if t.Type == "name" {
					key = t.Value
					expectKey = false
				}
			default:
				if key == "/MCID" {
					return t, true
				}
				if opening {
					depth++
				} else {
					expectKey = true
				}
			}
		default:
			if opening {
				depth++
			} else if closing {
				depth--
				if depth == 1 {
					expectKey = true
				}
			}
		}
	}
	return Token{}, false
}
