package pdfcore

import (
	"fmt"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// GetPageContentStreamNodeID resolves a 1-based page number to the node ID of
// its content stream. Returns empty string (no error) when the page has no
// Contents entry. For pages with an array of content stream refs, returns the
// first ref's node ID.
func (ins *Inspector) GetPageContentStreamNodeID(tabID string, pageNum int) (string, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return "", err
	}

	var pageDict pdfcpu_types.Dict
	err = safeCall(func() error {
		var e error
		pageDict, _, _, e = doc.PDFContext.PageDict(pageNum, false)
		return e
	})
	if err != nil {
		return "", wrapPDFError(err)
	}

	if pageDict == nil {
		return "", nil
	}

	contents, found := pageDict.Find("Contents")
	if !found || contents == nil {
		return "", nil
	}

	switch v := contents.(type) {
	case pdfcpu_types.IndirectRef:
		return fmt.Sprintf("obj:%d:%d", v.GenerationNumber.Value(), v.ObjectNumber.Value()), nil
	case pdfcpu_types.Array:
		if len(v) == 0 {
			return "", nil
		}
		ref, ok := v[0].(pdfcpu_types.IndirectRef)
		if !ok {
			return "", fmt.Errorf("Contents array element is not an indirect reference")
		}
		return fmt.Sprintf("obj:%d:%d", ref.GenerationNumber.Value(), ref.ObjectNumber.Value()), nil
	default:
		return "", fmt.Errorf("unexpected Contents type: %T", contents)
	}
}

// GetContentStream decodes and returns the content stream data for the given
// tree node. The node must resolve to a StreamDict (or variant). Decoded
// results are cached per-document so repeated calls skip decompression.
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

	// Return cached result if available.
	doc.streamMu.Lock()
	if cached, ok := doc.streamCache[nodeID]; ok {
		doc.streamMu.Unlock()
		return cached, nil
	}
	doc.streamMu.Unlock()

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
		doc.streamMu.Lock()
		doc.streamCache[nodeID] = result
		doc.streamMu.Unlock()
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
		doc.streamMu.Lock()
		doc.streamCache[nodeID] = result
		doc.streamMu.Unlock()
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
	}
	doc.streamMu.Lock()
	doc.streamCache[nodeID] = result
	doc.streamMu.Unlock()
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

		// Number: optional '-', then digits/dot, or '.' then digits.
		// Also handle negative numbers: '-' followed by digit or '.'.
		if isNumberStart(raw, i, n) {
			j := i
			if raw[j] == '-' {
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
							// Emit data token (without the preceding whitespace)
							endIdx := k - 1
							if endIdx < dataStart {
								endIdx = dataStart
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
	if ch == '-' {
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
