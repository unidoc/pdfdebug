package pdfcore

import (
	"fmt"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// GetPageContentStreamRef resolves a 1-based page number to its content
// stream's node ID AND the number of content streams in its /Contents entry, in
// a SINGLE page-dict resolution. Node ID is empty (no error) when the page has
// no /Contents; for an array it is the first ref's node ID (error if the first
// element is not an indirect reference). streamCount is 0 with no /Contents, 1
// for a single indirect ref, and for an array the count of its indirect-ref
// elements only - a degenerate null / non-ref element contributes no stream per
// ISO 32000-1 7.8.2, so counting len(v) would falsely report "showing stream 1
// of N" for e.g. [ref null] where the single stream is shown in full. Combining
// the two reads keeps the Story 14.3 multi-stream truncation marker from
// resolving (and cache-mutating) the page dict twice per dump.
func (ins *Inspector) GetPageContentStreamRef(tabID string, pageNum int) (nodeID string, streamCount int, err error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return "", 0, err
	}
	// PageDict mutates the pdfcpu page-resolution cache; serialize.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	var pageDict pdfcpu_types.Dict
	err = safeCall(func() error {
		var e error
		pageDict, _, _, e = doc.PDFContext.PageDict(pageNum, false)
		return e
	})
	if err != nil {
		return "", 0, wrapPDFError(err)
	}

	if pageDict == nil {
		return "", 0, nil
	}

	contents, found := pageDict.Find("Contents")
	if !found || contents == nil {
		return "", 0, nil
	}

	switch v := contents.(type) {
	case pdfcpu_types.IndirectRef:
		return fmt.Sprintf("obj:%d:%d", v.GenerationNumber.Value(), v.ObjectNumber.Value()), 1, nil
	case pdfcpu_types.Array:
		if len(v) == 0 {
			return "", 0, nil
		}
		count := 0
		for _, e := range v {
			if _, ok := e.(pdfcpu_types.IndirectRef); ok {
				count++
			}
		}
		ref, ok := v[0].(pdfcpu_types.IndirectRef)
		if !ok {
			return "", 0, fmt.Errorf("contents array element is not an indirect reference")
		}
		return fmt.Sprintf("obj:%d:%d", ref.GenerationNumber.Value(), ref.ObjectNumber.Value()), count, nil
	default:
		return "", 0, fmt.Errorf("unexpected Contents type: %T", contents)
	}
}

// GetPageContentStreamNodeID resolves a 1-based page number to the node ID of
// its content stream. Returns empty string (no error) when the page has no
// Contents entry. For pages with an array of content stream refs, returns the
// first ref's node ID. Thin wrapper over GetPageContentStreamRef that discards
// the stream count (bound in pdfservice.Service; signature preserved).
func (ins *Inspector) GetPageContentStreamNodeID(tabID string, pageNum int) (string, error) {
	nodeID, _, err := ins.GetPageContentStreamRef(tabID, pageNum)
	return nodeID, err
}

// GetPageNode resolves a 1-based page number to a fully-populated TreeNode for
// that page's page dict (/Type /Page object), suitable for rooting a tree walk.
// The returned node carries the page object's ObjectRef ("<num> <gen> R") and
// TypeName so callers do not have to synthesize a bare stub. Returns an error
// for out-of-range or non-existent pages (this is the authoritative
// upper-bound check for page-rooted tree walks).
func (ins *Inspector) GetPageNode(tabID string, pageNum int) (*TreeNode, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// PageDict mutates the pdfcpu page-resolution cache; serialize, same as
	// GetPageContentStreamNodeID.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	var pageDict pdfcpu_types.Dict
	var indRef *pdfcpu_types.IndirectRef
	err = safeCall(func() error {
		var e error
		pageDict, indRef, _, e = doc.PDFContext.PageDict(pageNum, false)
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}
	if pageDict == nil || indRef == nil {
		return nil, fmt.Errorf("page %d not found", pageNum)
	}

	objNum := indRef.ObjectNumber.Value()
	gen := indRef.GenerationNumber.Value()
	nodeType, valueType, hasChildren, childCount := classifyObject(pageDict)
	return &TreeNode{
		ID:          fmt.Sprintf("obj:%d:%d", gen, objNum),
		Label:       semanticLabel("", pageDict),
		NodeType:    nodeType,
		ValueType:   valueType,
		HasChildren: hasChildren,
		ChildCount:  childCount,
		IconHint:    iconHint("", nodeType, pageDict),
		ObjectRef:   fmt.Sprintf("%d %d R", objNum, gen),
		TypeName:    extractTypeName(pageDict),
	}, nil
}

// GetContentStream decodes and returns the content stream data for the given
// tree node. The node must resolve to a StreamDict (or variant). Decoded
// results are cached per-document so repeated calls skip decompression.
//
// Story 10-5 AC3: streamMu is held for the ENTIRE resolve+decode+write path,
// not dropped between cache check and write. The previous "drop lock for
// decode, reacquire to write" pattern let two concurrent first-time callers
// both decode and both clobber the cache, so each received a different
// *ContentStreamData pointer. Holding the lock for the duration collapses
// concurrent same-node calls to one decode pass; the second caller blocks,
// then observes the populated cache and returns the same pointer.
//
// Lock order: pdfMu (outer) -> streamMu (inner). pdfcpu's Dereference inside
// resolveNodeObject is guarded by pdfMu; streamMu guards the streamCache map.
func (ins *Inspector) GetContentStream(tabID, nodeID string) (*ContentStreamData, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrDocumentNotFound)
	}

	if strings.HasPrefix(nodeID, "error:") {
		return &ContentStreamData{
			NodeID: nodeID,
			Error:  "cannot decode stream for error node",
		}, nil
	}

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// AC1: serialize pdfcpu access. Outer lock.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()
	// AC3: hold streamMu for the entire critical section (resolve+decode+write).
	// No drop-and-reacquire between cache miss and cache write.
	doc.streamMu.Lock()
	defer doc.streamMu.Unlock()

	// Cache check inside the critical section. Concurrent same-node callers
	// serialize on streamMu; the second observes the populated cache here.
	if cached, ok := doc.streamCache[nodeID]; ok {
		return cached, nil
	}

	var obj pdfcpu_types.Object
	err = safeCall(func() error {
		var e error
		obj, e = resolveNodeObject(doc, nodeID)
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	// Extract the StreamDict from the resolved object.
	var sd pdfcpu_types.StreamDict
	switch v := obj.(type) {
	case pdfcpu_types.StreamDict:
		sd = v
	case pdfcpu_types.ObjectStreamDict:
		sd = v.StreamDict
	case pdfcpu_types.XRefStreamDict:
		sd = v.StreamDict
	default:
		result := &ContentStreamData{
			NodeID: nodeID,
			Error:  "node is not a stream object",
		}
		doc.streamCache[nodeID] = result
		return result, nil
	}

	// Decode the stream inside safeCall (pdfcpu can panic).
	err = safeCall(func() error {
		return sd.Decode()
	})
	if err != nil {
		result := &ContentStreamData{
			NodeID: nodeID,
			Error:  fmt.Sprintf("failed to decode stream: %v", err),
		}
		doc.streamCache[nodeID] = result
		return result, nil
	}

	var raw string
	if sd.Content != nil {
		raw = string(sd.Content)
	} else {
		raw = string(sd.Raw)
	}

	result := &ContentStreamData{
		NodeID: nodeID,
		Raw:    raw,
	}
	if raw != "" {
		result.Tokenized = tokenizeContentStream(raw)
		result.Formatted = Format(result.Tokenized)
	}
	doc.streamCache[nodeID] = result
	return result, nil
}

// tokenizeContentStream performs lexical tokenization of a PDF content stream,
// producing tokens with type, value, and 1-based line/col positions.
func tokenizeContentStream(raw string) []Token {
	var tokens []Token
	i := 0
	line := 1
	col := 1
	n := len(raw)

	for i < n {
		ch := raw[i]

		// Skip whitespace, tracking line/col.
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == '\x00' {
			if ch == '\n' {
				line++
				col = 1
			} else if ch == '\r' {
				line++
				col = 1
				// Consume \r\n as single newline.
				if i+1 < n && raw[i+1] == '\n' {
					i++
				}
			} else {
				col++
			}
			i++
			continue
		}

		startLine := line
		startCol := col

		// Comment: % to end of line
		if ch == '%' {
			j := i
			for j < n && raw[j] != '\n' && raw[j] != '\r' {
				j++
			}
			tokens = append(tokens, Token{Type: "comment", Value: raw[i:j], Line: startLine, Col: startCol})
			col += j - i
			i = j
			continue
		}

		// Literal string: (...)
		if ch == '(' {
			j := i + 1
			depth := 1
			for j < n && depth > 0 {
				if raw[j] == '\\' {
					j += 2 // skip escaped char
					// Guard against backslash as last byte in input.
					if j >= n {
						break
					}
					continue
				}
				if raw[j] == '(' {
					depth++
				} else if raw[j] == ')' {
					depth--
				}
				j++
			}
			if j > n {
				j = n
			}
			val := raw[i:j]
			tokens = append(tokens, Token{Type: "string", Value: val, Line: startLine, Col: startCol})
			// Update line/col by scanning through the value.
			// Handle \r\n as a single newline, matching the main scanner.
			for vi := 0; vi < len(val); vi++ {
				vc := val[vi]
				if vc == '\n' {
					line++
					col = 1
				} else if vc == '\r' {
					line++
					col = 1
					// Consume \r\n as single newline.
					if vi+1 < len(val) && val[vi+1] == '\n' {
						vi++
					}
				} else {
					col++
				}
			}
			i = j
			continue
		}

		// Dict delimiters: << and >>
		if ch == '<' && i+1 < n && raw[i+1] == '<' {
			tokens = append(tokens, Token{Type: "operator", Value: "<<", Line: startLine, Col: startCol})
			col += 2
			i += 2
			continue
		}
		if ch == '>' && i+1 < n && raw[i+1] == '>' {
			tokens = append(tokens, Token{Type: "operator", Value: ">>", Line: startLine, Col: startCol})
			col += 2
			i += 2
			continue
		}

		// Hex string: <...>
		if ch == '<' && (i+1 >= n || raw[i+1] != '<') {
			j := i + 1
			for j < n && raw[j] != '>' {
				j++
			}
			if j < n {
				j++ // include closing >
			}
			tokens = append(tokens, Token{Type: "string", Value: raw[i:j], Line: startLine, Col: startCol})
			col += j - i
			i = j
			continue
		}

		// Name: /...
		if ch == '/' {
			j := i + 1
			for j < n && !isDelimOrWhitespace(raw[j]) {
				j++
			}
			tokens = append(tokens, Token{Type: "name", Value: raw[i:j], Line: startLine, Col: startCol})
			col += j - i
			i = j
			continue
		}

		// Array brackets
		if ch == '[' || ch == ']' {
			tokens = append(tokens, Token{Type: "operator", Value: string(ch), Line: startLine, Col: startCol})
			col++
			i++
			continue
		}

		// Number: optional sign ('+' or '-'), then digits/dot, or '.' then digits.
		// ISO 32000-1 7.3.3 permits a leading '+' or '-' on integers and reals.
		if isNumberStart(raw, i, n) {
			j := i
			// Consume the leading sign so it is part of the number token, not a
			// stray first char of the digit scan. Both '+' and '-' must be
			// handled here or isNumberStart accepting '+' yields a malformed token.
			if raw[j] == '-' || raw[j] == '+' {
				j++
			}
			hasDot := false
			for j < n && (raw[j] >= '0' && raw[j] <= '9' || raw[j] == '.' && !hasDot) {
				if raw[j] == '.' {
					hasDot = true
				}
				j++
			}
			tokens = append(tokens, Token{Type: "number", Value: raw[i:j], Line: startLine, Col: startCol})
			col += j - i
			i = j
			continue
		}

		// Word token (operator or keyword).
		j := i
		for j < n && !isDelimOrWhitespace(raw[j]) {
			j++
		}
		// Guard: if j == i the current byte is an unhandled delimiter (e.g. lone
		// '>', ')', '{', '}'); emit it as a single-char operator to avoid an
		// infinite loop.
		if j == i {
			tokens = append(tokens, Token{Type: "operator", Value: string(ch), Line: startLine, Col: startCol})
			col++
			i++
			continue
		}
		word := raw[i:j]

		// Handle inline image: ID operator
		if word == "ID" {
			tokens = append(tokens, Token{Type: "operator", Value: "ID", Line: startLine, Col: startCol})
			col += 2
			i = j
			// Skip one whitespace byte after ID (per PDF spec)
			if i < n && (raw[i] == ' ' || raw[i] == '\n' || raw[i] == '\r' || raw[i] == '\t') {
				if raw[i] == '\n' {
					line++
					col = 1
				} else if raw[i] == '\r' {
					line++
					col = 1
					if i+1 < n && raw[i+1] == '\n' {
						i++
					}
				} else {
					col++
				}
				i++
			}
			// Scan for EI preceded by whitespace and followed by whitespace or EOF
			dataStart := i
			dataLine := line
			dataCol := col
			found := false
			for k := i; k < n-1; k++ {
				if raw[k] == 'E' && raw[k+1] == 'I' {
					// Check preceding byte is whitespace
					if k > 0 && isWSByte(raw[k-1]) {
						// Check following byte is whitespace or EOF
						afterEI := k + 2
						if afterEI >= n || isWSByte(raw[afterEI]) {
							// Emit data token (without the preceding whitespace).
							// endIdx = k-1 already drops the single delimiter byte
							// adjacent to 'E' (guaranteed whitespace by the check
							// above).
							endIdx := k - 1
							if endIdx < dataStart {
								endIdx = dataStart
							}
							// F3: if that delimiter is the LF of a "\r\nEI" CRLF pair,
							// also drop the CR so the opaque data token carries no
							// stray trailing '\r'. Bounded to a single EOL delimiter
							// (one CRLF unit) -- deliberately NOT an unbounded
							// whitespace run, which would strip whitespace-valued
							// bytes (e.g. a 0x20 sample) that are legitimately part of
							// the opaque inline-image payload.
							// Irreducible caveat: with no inline-image length model
							// (EI is found heuristically), a payload legitimately
							// ending in a real 0x0D byte before a bare-LF delimiter
							// ("...<0x0D>\nEI") is indistinguishable from a CRLF and
							// that byte is dropped. Accepted tradeoff: bounding an
							// opaque display token to a length model is out of scope,
							// and not stripping the CR would reintroduce the stray
							// '\r' this fix removes.
							if endIdx > dataStart && raw[endIdx] == '\n' && raw[endIdx-1] == '\r' {
								endIdx--
							}
							data := raw[dataStart:endIdx]
							if len(data) > 0 {
								tokens = append(tokens, Token{Type: "string", Value: data, Line: dataLine, Col: dataCol})
							}
							// Update line/col through the data
							for idx := dataStart; idx < k; idx++ {
								if raw[idx] == '\n' {
									line++
									col = 1
								} else if raw[idx] == '\r' {
									line++
									col = 1
									if idx+1 < k && raw[idx+1] == '\n' {
										idx++
									}
								} else {
									col++
								}
							}
							// Emit EI
							tokens = append(tokens, Token{Type: "operator", Value: "EI", Line: line, Col: col})
							col += 2
							i = afterEI
							found = true
							break
						}
					}
				}
			}
			if !found {
				// No EI found: consume rest as string
				data := raw[dataStart:]
				if len(data) > 0 {
					tokens = append(tokens, Token{Type: "string", Value: data, Line: dataLine, Col: dataCol})
				}
				i = n
			}
			continue
		}

		tokens = append(tokens, Token{Type: "operator", Value: word, Line: startLine, Col: startCol})
		col += j - i
		i = j
	}

	return tokens
}

// isNumberStart returns true if position i in raw starts a PDF number token.
func isNumberStart(raw string, i, n int) bool {
	ch := raw[i]
	if ch >= '0' && ch <= '9' {
		return true
	}
	if ch == '.' {
		return i+1 < n && raw[i+1] >= '0' && raw[i+1] <= '9'
	}
	// A leading '+' or '-' is a number start only when followed by a digit, or
	// by '.' + digit (ISO 32000-1 7.3.3). A bare sign stays a word/operator.
	if ch == '-' || ch == '+' {
		if i+1 < n {
			next := raw[i+1]
			if next >= '0' && next <= '9' {
				return true
			}
			if next == '.' && i+2 < n && raw[i+2] >= '0' && raw[i+2] <= '9' {
				return true
			}
		}
	}
	return false
}

// isDelimOrWhitespace returns true if b is a PDF whitespace or delimiter byte.
func isDelimOrWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '\x00':
		return true
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// isWSByte returns true if b is a whitespace byte (space, tab, CR, LF).
func isWSByte(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t'
}
