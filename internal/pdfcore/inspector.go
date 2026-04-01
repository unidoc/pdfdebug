package pdfcore

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

type DocumentState struct {
	FilePath   string
	PDFContext *pdfcpu_model.Context
	PageCount  int
}

type Inspector struct {
	mu        sync.Mutex
	documents map[string]*DocumentState
}

func NewInspector() *Inspector {
	return &Inspector{
		documents: make(map[string]*DocumentState),
	}
}

func (ins *Inspector) Open(tabID, filePath string) (*DocumentInfo, error) {
	if tabID == "" {
		return nil, fmt.Errorf("%w: empty tab ID", ErrDocumentNotFound)
	}
	if filePath == "" {
		return nil, fmt.Errorf("%w: empty file path", ErrDocumentNotFound)
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %v", ErrDocumentNotFound, err)
		}
		return nil, err
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("%w: path is a directory", ErrDocumentNotFound)
	}
	fileSize := fi.Size()

	var ctx *pdfcpu_model.Context
	err = safeCall(func() error {
		var e error
		ctx, e = pdfcpu_api.ReadContextFile(filePath)
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	err = safeCall(func() error {
		return ctx.XRefTable.EnsurePageCount()
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	ins.mu.Lock()
	ins.documents[tabID] = &DocumentState{
		FilePath:   absPath,
		PDFContext: ctx,
		PageCount:  ctx.PageCount,
	}
	ins.mu.Unlock()

	return &DocumentInfo{
		TabID:     tabID,
		FileName:  filepath.Base(filePath),
		FilePath:  absPath,
		PageCount: ctx.PageCount,
		FileSize:  fileSize,
	}, nil
}

func (ins *Inspector) Close(tabID string) error {
	ins.mu.Lock()
	defer ins.mu.Unlock()
	if _, ok := ins.documents[tabID]; !ok {
		return fmt.Errorf("%w: tab %q", ErrDocumentNotFound, tabID)
	}
	delete(ins.documents, tabID)
	return nil
}

func (ins *Inspector) GetDocument(tabID string) (*DocumentState, error) {
	ins.mu.Lock()
	defer ins.mu.Unlock()
	doc, ok := ins.documents[tabID]
	if !ok {
		return nil, fmt.Errorf("%w: tab %q", ErrDocumentNotFound, tabID)
	}
	return doc, nil
}
