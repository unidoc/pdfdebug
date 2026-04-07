package pdfcore

import (
	"fmt"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

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
	doc.streamMu.Lock()
	doc.streamCache[nodeID] = result
	doc.streamMu.Unlock()
	return result, nil
}
