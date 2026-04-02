package pdfservice

import (
	"fmt"

	"github.com/google/uuid"

	"unipdf-debugger/internal/pdfcore"
)

type PDFService struct {
	inspector *pdfcore.Inspector
}

func NewPDFService() PDFService {
	return PDFService{
		inspector: pdfcore.NewInspector(),
	}
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
	return nil, fmt.Errorf("not implemented")
}
