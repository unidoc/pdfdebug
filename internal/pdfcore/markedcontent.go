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

// mcState is the scan state carried from one content stream of a page to the
// next, so the page's streams scan as one concatenated stream: the stack of
// open marked-content sequences and the operands not yet consumed by an
// operator, with the index of the stream each operand came from.
type mcState struct {
	stack     []mcOpen
	operands  []Token
	opStreams []int
}

// propertyLookup resolves a named property list (BDC with a name operand) in
// the page's /Resources /Properties and returns its /MCID value, nil when the
// list has none. An error means the name was not found, the entry is not a
// dictionary, or its /MCID does not resolve.
type propertyLookup func(name string) (mcid pdfcpu_types.Object, err error)

// scanMarkedContent counts BMC/BDC/EMC in one stream's tokens into ms and
// returns a row for every BDC whose property list carries an /MCID. st is
// updated in place so nesting and pending operands carry into the next
// stream.
//
// The tokenizer types delimiters and the keywords true, false and null as
// operators, so operands are collected by value: <<, >>, [, ], true, false and
// null are operands, and any other operator ends the operand list. A stray <<
// or [ therefore cannot swallow the rest of the stream.
func scanMarkedContent(tokens []Token, streamIdx int, props propertyLookup, st *mcState, ms *MarkedStream) []ContentMCID {
	var rows []ContentMCID
	ms.MinDepth = len(st.stack)
	for _, tok := range tokens {
		if tok.Type == "comment" {
			continue
		}
		if tok.Type != "operator" {
			st.pushOperand(tok, streamIdx)
			continue
		}
		switch tok.Value {
		case "<<", ">>", "[", "]", "true", "false", "null":
			st.pushOperand(tok, streamIdx)
			continue
		}
		switch tok.Value {
		case "BMC":
			ms.Opens++
			st.stack = append(st.stack, mcOpen{})
		case "BDC":
			ms.Opens++
			row, ok := bdcRow(st.operands, st.opStreams, tok, streamIdx, props)
			parent := innermostMCID(st.stack)
			open := mcOpen{}
			if ok {
				row.ParentMCID = parent
				if row.MCID >= 0 {
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
		st.clearOperands()
	}
	ms.DepthAtEnd = len(st.stack)
	return rows
}

// pushOperand records tok, read from stream streamIdx, as a pending operand.
func (st *mcState) pushOperand(tok Token, streamIdx int) {
	st.operands = append(st.operands, tok)
	st.opStreams = append(st.opStreams, streamIdx)
}

// clearOperands drops the pending operands.
func (st *mcState) clearOperands() {
	st.operands = st.operands[:0]
	st.opStreams = st.opStreams[:0]
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

// bdcRow reads a BDC's operands. The property list is the last operand
// object (an inline dictionary or a property list name), or, when that is not
// a closed dictionary, a dictionary still open at the operator; the tag is the object before it, and anything earlier
// is ignored. opStreams holds the stream index of each operand token; the
// row's Stream, Line and Col are the tag's, else the first operand's, else the
// operator's (in stream opStream). ok is false when the property list has no
// /MCID, so the BDC gets no row. Fewer than two operand objects, a tag that is
// not a name, a property list that is neither a dictionary nor a name or
// cannot be read, or an /MCID that is not a non-negative integer give a row
// with MCID -1 and Err set. An inline dictionary with no closing ">>" keeps
// its MCID and sets Err.
func bdcRow(operands []Token, opStreams []int, op Token, opStream int, props propertyLookup) (row ContentMCID, ok bool) {
	row = ContentMCID{MCID: -1, Stream: opStream, Line: op.Line, Col: op.Col}
	at := func(i int) {
		row.Stream, row.Line, row.Col = opStreams[i], operands[i].Line, operands[i].Col
	}
	n := len(operands)
	if n == 0 {
		row.Err = "BDC has 0 of its 2 operands (a tag and a property list)"
		return row, true
	}
	propStart := operandStart(operands, n-1)
	// With no closed dictionary last, a "<<" still open is the property list,
	// even as the first operand.
	if operands[propStart].Value != "<<" {
		if u := unclosedDictStart(operands); u >= 0 {
			propStart = u
		}
	}
	if propStart == 0 {
		at(0)
		if n == 1 && operands[0].Type == "name" {
			row.Tag = strings.TrimPrefix(operands[0].Value, "/")
		}
		row.Err = "BDC has 1 of its 2 operands (a tag and a property list)"
		return row, true
	}
	tagStart := operandStart(operands, propStart-1)
	at(tagStart)
	if tagStart != propStart-1 || operands[tagStart].Type != "name" {
		row.Err = fmt.Sprintf("BDC operand %q before the property list is not a tag name; property list not read", operands[tagStart].Value)
		return row, true
	}
	row.Tag = strings.TrimPrefix(operands[tagStart].Value, "/")
	prop := operands[propStart:]
	switch {
	case prop[0].Type == "operator" && prop[0].Value == "<<":
		val, found := inlineMCID(prop)
		if !found {
			return row, false
		}
		if val.Type == "number" {
			if n, err := strconv.Atoi(val.Value); err == nil {
				setMCID(&row, n)
				if row.Err == "" && unclosedDictStart(prop) == 0 {
					row.Err = "property-list dictionary has no closing >>"
				}
				return row, true
			}
		}
		row.Err = fmt.Sprintf("/MCID %s is not an integer", val.Value)
		return row, true
	case len(prop) == 1 && prop[0].Type == "name":
		// /Properties keys are #xx-decoded by the parser, so the operand is too.
		name, err := pdfcpu_types.DecodeName(strings.TrimPrefix(prop[0].Value, "/"))
		if err != nil {
			row.Err = fmt.Sprintf("property list name %s does not decode: %v", prop[0].Value, err)
			return row, true
		}
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
	row.Err = fmt.Sprintf("BDC property list operand %q is not a dictionary or a name", prop[0].Value)
	return row, true
}

// unclosedDictStart returns the index of the outermost "<<" still open at the
// end of toks, or -1 when none is.
func unclosedDictStart(toks []Token) int {
	var open []int
	for i, t := range toks {
		if t.Type != "operator" {
			continue
		}
		switch t.Value {
		case "<<", "[":
			open = append(open, i)
		case ">>", "]":
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
	}
	for _, i := range open {
		if toks[i].Value == "<<" {
			return i
		}
	}
	return -1
}

// operandStart returns the index of the first token of the operand object
// that ends at toks[end]: a dictionary or array is matched back to its
// opening delimiter. An unmatched closing delimiter extends the object to
// index 0.
func operandStart(toks []Token, end int) int {
	depth := 0
	for i := end; i >= 0; i-- {
		if t := toks[i]; t.Type == "operator" {
			switch t.Value {
			case ">>", "]":
				depth++
			case "<<", "[":
				depth--
			}
		}
		if depth <= 0 {
			return i
		}
	}
	return 0
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
