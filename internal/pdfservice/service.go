package pdfservice

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// PDFService is the Wails-bound service that exposes PDF inspection to the
// frontend. It delegates to pdfcore.Inspector for all PDF operations.
type PDFService struct {
	inspector *pdfcore.Inspector
	app       *application.App
}

// NewPDFService creates a PDFService backed by a fresh Inspector.
func NewPDFService(app *application.App) PDFService {
	return PDFService{
		inspector: pdfcore.NewInspector(),
		app:       app,
	}
}

// OpenFileDialog shows a native file picker filtered to PDF files and returns
// every selected path. An empty slice means the user cancelled. Multi-select
// is enabled so users can open several PDFs into multiple tabs in one gesture
// (parity with the drag-and-drop multi-file flow).
func (s *PDFService) OpenFileDialog() ([]string, error) {
	if s.app == nil {
		return nil, fmt.Errorf("app not initialized")
	}
	paths, err := s.app.Dialog.OpenFile().
		SetTitle("Open PDF").
		AddFilter("PDF Files", "*.pdf").
		AddFilter("All Files", "*.*").
		PromptForMultipleSelection()
	// On Windows, cancelling the dialog returns an ole.Error instead of nil.
	// Treat an empty selection as cancel regardless of error.
	if len(paths) == 0 {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// OpenFile parses a PDF at path and assigns it a new tab ID.
func (s *PDFService) OpenFile(path string) (*pdfcore.DocumentInfo, error) {
	tabID := uuid.New().String()
	return s.inspector.Open(tabID, path)
}

// CloseDocument releases resources for the given tab.
func (s *PDFService) CloseDocument(tabID string) error {
	return s.inspector.Close(tabID)
}

// GetTreeRoot returns the catalog root node for the document in tabID.
func (s *PDFService) GetTreeRoot(tabID string) (*pdfcore.TreeNode, error) {
	return s.inspector.GetTreeRoot(tabID)
}

// GetChildren returns child nodes for the specified tree node.
func (s *PDFService) GetChildren(tabID string, nodeID string) ([]*pdfcore.TreeNode, error) {
	return s.inspector.GetChildren(tabID, nodeID)
}

// GetObjectDetail returns the full detail view for a PDF object node.
func (s *PDFService) GetObjectDetail(tabID string, nodeID string) (*pdfcore.ObjectDetail, error) {
	return s.inspector.GetObjectDetail(tabID, nodeID)
}

// GetAncestorPath returns the path from root to the given node ID.
func (s *PDFService) GetAncestorPath(tabID string, nodeID string) ([]string, error) {
	return s.inspector.GetAncestorPath(tabID, nodeID)
}

// GetContentStream returns decoded content stream data for the given node.
func (s *PDFService) GetContentStream(tabID string, nodeID string) (*pdfcore.ContentStreamData, error) {
	return s.inspector.GetContentStream(tabID, nodeID)
}

// GetImageData extracts and encodes an image from the given node.
func (s *PDFService) GetImageData(tabID string, nodeID string) (*pdfcore.ImageData, error) {
	return s.inspector.GetImageData(tabID, nodeID)
}

// GetObjectSource returns the reserialized PDF-syntax representation of an
// indirect object. Inline-node selections return ("", nil) so the frontend
// can render the AC3 empty state.
func (s *PDFService) GetObjectSource(tabID string, nodeID string) (string, error) {
	return s.inspector.GetObjectSource(tabID, nodeID)
}

// GetReverseRefs returns the inbound dict-graph references for the indirect
// object identified by nodeID, sourced from the per-document reverse-ref
// index built at Open. Returns pdfcore.ErrReverseRefIndexUnavailable when the
// index could not be built (panic-wrapped failure mode -- AC6).
//
// Returns []*pdfcore.ReverseRef so the Wails-generated TS binding produces a
// nullable element type that mirrors Go's pointer semantics (the per-row
// ParentType is already *string).
func (s *PDFService) GetReverseRefs(tabID string, nodeID string) ([]*pdfcore.ReverseRef, error) {
	refs, err := s.inspector.GetReverseRefs(tabID, nodeID)
	if err != nil {
		return nil, err
	}
	out := make([]*pdfcore.ReverseRef, len(refs))
	for i := range refs {
		rr := refs[i]
		out[i] = &rr
	}
	return out, nil
}

// GoToPage resolves a 1-based page number to the node ID of that page's
// content stream, suitable for the frontend to dispatch as a NAVIGATE_TO_REF
// target. Returns an error if the page number is out of range, the page has
// no content stream, or the document/tab is unknown.
func (s *PDFService) GoToPage(tabID string, pageNum int) (string, error) {
	return s.inspector.GetPageContentStreamNodeID(tabID, pageNum)
}
