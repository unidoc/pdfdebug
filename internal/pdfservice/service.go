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
// the selected path, or an empty string if cancelled.
func (s *PDFService) OpenFileDialog() (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("app not initialized")
	}
	path, err := s.app.Dialog.OpenFile().
		SetTitle("Open PDF").
		AddFilter("PDF Files", "*.pdf").
		AddFilter("All Files", "*.*").
		PromptForSingleSelection()
	// On Windows, cancelling the dialog returns an ole.Error instead of nil.
	// Treat empty path as cancel regardless of error.
	if path == "" {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return path, nil
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
