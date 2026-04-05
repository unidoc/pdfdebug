package pdfservice

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"

	"unipdf-debugger/internal/pdfcore"
)

type PDFService struct {
	inspector *pdfcore.Inspector
	app       *application.App
}

func NewPDFService(app *application.App) PDFService {
	return PDFService{
		inspector: pdfcore.NewInspector(),
		app:       app,
	}
}

func (s *PDFService) OpenFileDialog() (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("app not initialized")
	}
	path, err := s.app.Dialog.OpenFile().
		SetTitle("Open PDF").
		AddFilter("PDF Files", "*.pdf").
		AddFilter("All Files", "*.*").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	return path, nil
}

func (s *PDFService) OpenFile(path string) (*pdfcore.DocumentInfo, error) {
	tabID := uuid.New().String()
	return s.inspector.Open(tabID, path)
}

func (s *PDFService) CloseDocument(tabID string) error {
	return s.inspector.Close(tabID)
}

func (s *PDFService) GetTreeRoot(tabID string) (*pdfcore.TreeNode, error) {
	return s.inspector.GetTreeRoot(tabID)
}

func (s *PDFService) GetChildren(tabID string, nodeID string) ([]*pdfcore.TreeNode, error) {
	return s.inspector.GetChildren(tabID, nodeID)
}

func (s *PDFService) GetObjectDetail(tabID string, nodeID string) (*pdfcore.ObjectDetail, error) {
	return s.inspector.GetObjectDetail(tabID, nodeID)
}

func (s *PDFService) GetAncestorPath(tabID string, nodeID string) ([]string, error) {
	return s.inspector.GetAncestorPath(tabID, nodeID)
}
