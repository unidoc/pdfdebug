package pdfservice

import (
	"fmt"
	"log"
	"runtime"
	"runtime/debug"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// inspectorAPI is the method set PDFService uses from the inspector backend.
// Declaring it as an unexported interface (rather than holding a concrete
// pointer) is the AC5 seam: tests inject a stub that panics with a synthetic
// runtime.Error to drive the recoverRuntimePanic path without needing a real
// malformed PDF.
//
// All methods that PDFService binds to Wails are listed here. The plaintext
// methods are included even though they are NOT wrapped by
// recoverRuntimePanic (they do not touch the PDF backend) so the production
// pdfcore.Inspector satisfies the interface.
type inspectorAPI interface {
	Open(tabID, filePath string) (*pdfcore.DocumentInfo, error)
	Close(tabID string) error
	GetTreeRoot(tabID string) (*pdfcore.TreeNode, error)
	GetChildren(tabID string, nodeID string) ([]*pdfcore.TreeNode, error)
	GetObjectDetail(tabID string, nodeID string) (*pdfcore.ObjectDetail, error)
	GetAncestorPath(tabID string, nodeID string) ([]string, error)
	GetContentStream(tabID string, nodeID string) (*pdfcore.ContentStreamData, error)
	GetImageData(tabID string, nodeID string) (*pdfcore.ImageData, error)
	GetFontDetail(tabID string, nodeID string) (*pdfcore.FontDetail, error)
	GetFontResourceMap(tabID string, nodeID string) (*pdfcore.FontResourceMap, error)
	GetFontView(tabID string, nodeID string) (*pdfcore.FontView, error)
	GetObjectSource(tabID string, nodeID string) (string, error)
	GetReverseRefs(tabID string, nodeID string) ([]pdfcore.ReverseRef, error)
	GetPageContentStreamNodeID(tabID string, pageNum int) (string, error)
	GetObjectIndex(tabID string) ([]*pdfcore.ObjectIndexEntry, error)
	GetXRefTable(tabID string) (*pdfcore.XRefTable, error)
	GetPlainText(tabID string) (*pdfcore.PlainTextDocument, error)
	CancelPlainText(tabID string) error
	GetPlainTextSize(tabID string) (int64, error)
}

// PDFService is the Wails-bound service that exposes PDF inspection to the
// frontend. It delegates to the inspector backend for all PDF operations.
//
// The inspector field is typed as the inspectorAPI interface (Story 10-5
// AC5 seam) rather than the concrete *pdfcore.Inspector; the production
// constructor injects a *pdfcore.Inspector via pdfcore.NewInspector() and
// tests inject a stub that panics with a runtime.Error to drive the
// recoverRuntimePanic path.
type PDFService struct {
	inspector inspectorAPI
	app       *application.App
}

// NewPDFService creates a PDFService backed by a fresh Inspector.
func NewPDFService(app *application.App) PDFService {
	return PDFService{
		inspector: pdfcore.NewInspector(),
		app:       app,
	}
}

// recoverRuntimePanic is the deferred recover invoked at the top of every
// PDF-backend-touching PDFService method. It catches runtime.Error panics
// that the inspector's safeCall re-panics across the pdfservice boundary,
// converts them to ErrMalformedPDF: internal error, and logs the stack
// trace. Non-runtime.Error panics are re-panicked (preserves the
// test-binary-crash diagnostic for genuine bugs not in the inspector's
// documented panic surface).
//
// Story 10-5 AC5: each wrapper invokes the inspector call inside an
// anonymous closure that owns the deferred recover; the closure writes its
// result and error into outer locals (via shadow) and the recover overwrites
// the error local via pointer if a runtime.Error fires. This pattern keeps
// the method signatures stable (no named returns; the existing Story 10-3
// signature-preservation contract continues to hold) while still letting
// the recover replace the returned error.
func recoverRuntimePanic(methodName string, errOut *error) {
	if r := recover(); r != nil {
		if _, ok := r.(runtime.Error); !ok {
			panic(r)
		}
		// Defense in depth: if a future caller forgets to pass &err, re-panic
		// rather than nil-deref inside the deferred recover (which would mask
		// the original runtime.Error and crash the goroutine with a less
		// informative stack).
		if errOut == nil {
			panic(r)
		}
		log.Printf("pdfservice: runtime.Error in %s: %v\n%s", methodName, r, debug.Stack())
		*errOut = fmt.Errorf("%w: internal error", pdfcore.ErrMalformedPDF)
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
	var result *pdfcore.DocumentInfo
	var err error
	func() {
		defer recoverRuntimePanic("OpenFile", &err)
		result, err = s.inspector.Open(uuid.New().String(), path)
	}()
	return result, err
}

// CloseDocument releases resources for the given tab.
func (s *PDFService) CloseDocument(tabID string) error {
	return s.inspector.Close(tabID)
}

// GetTreeRoot returns the catalog root node for the document in tabID.
func (s *PDFService) GetTreeRoot(tabID string) (*pdfcore.TreeNode, error) {
	var result *pdfcore.TreeNode
	var err error
	func() {
		defer recoverRuntimePanic("GetTreeRoot", &err)
		result, err = s.inspector.GetTreeRoot(tabID)
	}()
	return result, err
}

// GetChildren returns child nodes for the specified tree node.
func (s *PDFService) GetChildren(tabID string, nodeID string) ([]*pdfcore.TreeNode, error) {
	var result []*pdfcore.TreeNode
	var err error
	func() {
		defer recoverRuntimePanic("GetChildren", &err)
		result, err = s.inspector.GetChildren(tabID, nodeID)
	}()
	return result, err
}

// GetObjectDetail returns the full detail view for a PDF object node.
func (s *PDFService) GetObjectDetail(tabID string, nodeID string) (*pdfcore.ObjectDetail, error) {
	var result *pdfcore.ObjectDetail
	var err error
	func() {
		defer recoverRuntimePanic("GetObjectDetail", &err)
		result, err = s.inspector.GetObjectDetail(tabID, nodeID)
	}()
	return result, err
}

// GetAncestorPath returns the path from root to the given node ID.
func (s *PDFService) GetAncestorPath(tabID string, nodeID string) ([]string, error) {
	var result []string
	var err error
	func() {
		defer recoverRuntimePanic("GetAncestorPath", &err)
		result, err = s.inspector.GetAncestorPath(tabID, nodeID)
	}()
	return result, err
}

// GetContentStream returns decoded content stream data for the given node.
func (s *PDFService) GetContentStream(tabID string, nodeID string) (*pdfcore.ContentStreamData, error) {
	var result *pdfcore.ContentStreamData
	var err error
	func() {
		defer recoverRuntimePanic("GetContentStream", &err)
		result, err = s.inspector.GetContentStream(tabID, nodeID)
	}()
	return result, err
}

// GetImageData extracts and encodes an image from the given node.
func (s *PDFService) GetImageData(tabID string, nodeID string) (*pdfcore.ImageData, error) {
	var result *pdfcore.ImageData
	var err error
	func() {
		defer recoverRuntimePanic("GetImageData", &err)
		result, err = s.inspector.GetImageData(tabID, nodeID)
	}()
	return result, err
}

// GetFontDetail returns the consolidated font inspection payload for a
// /Type /Font dict node. Returns pdfcore.ErrNotAFont when the resolved dict
// is not a Font dict (e.g. the iconHint='font' false positive on the
// /Resources /Font resource map); the frontend treats this sentinel as a
// signal to silently render the generic DictView (Story 9-9 AC1).
func (s *PDFService) GetFontDetail(tabID string, nodeID string) (*pdfcore.FontDetail, error) {
	var result *pdfcore.FontDetail
	var err error
	func() {
		defer recoverRuntimePanic("GetFontDetail", &err)
		result, err = s.inspector.GetFontDetail(tabID, nodeID)
	}()
	return result, err
}

// GetObjectSource returns the reserialized PDF-syntax representation of an
// indirect object. Inline-node selections return ("", nil) so the frontend
// can render the AC3 empty state.
func (s *PDFService) GetObjectSource(tabID string, nodeID string) (string, error) {
	var result string
	var err error
	func() {
		defer recoverRuntimePanic("GetObjectSource", &err)
		result, err = s.inspector.GetObjectSource(tabID, nodeID)
	}()
	return result, err
}

// GetFontResourceMap returns the per-font roster summary for a /Resources
// /Font dict. Returns pdfcore.ErrNotAFont when the resolved dict is not a
// font resource map (no entry resolves to a /Type /Font dict); the frontend
// falls back to the generic DictView in that case.
//
// Deprecated: the running app uses GetFontView; GetFontResourceMap remains
// bound only because the Go unit tests pin its sentinel contract.
func (s *PDFService) GetFontResourceMap(tabID string, nodeID string) (*pdfcore.FontResourceMap, error) {
	var result *pdfcore.FontResourceMap
	var err error
	func() {
		defer recoverRuntimePanic("GetFontResourceMap", &err)
		result, err = s.inspector.GetFontResourceMap(tabID, nodeID)
	}()
	return result, err
}

// GetFontView returns the unified font-inspection payload for a node. The
// FontView Kind field disambiguates the three outcomes ("detail" / "roster" /
// "neither") server-side so the frontend issues one call per click and the
// non-font case never produces a Wails error log.
func (s *PDFService) GetFontView(tabID string, nodeID string) (*pdfcore.FontView, error) {
	var result *pdfcore.FontView
	var err error
	func() {
		defer recoverRuntimePanic("GetFontView", &err)
		result, err = s.inspector.GetFontView(tabID, nodeID)
	}()
	return result, err
}

// GetReverseRefs returns the inbound dict-graph references for the indirect
// object identified by nodeID, sourced from the per-document reverse-ref
// index built lazily on first call (Story 10-5 AC7). Returns
// pdfcore.ErrReverseRefIndexUnavailable when the index could not be built.
//
// Returns []*pdfcore.ReverseRef so the Wails-generated TS binding produces a
// nullable element type that mirrors Go's pointer semantics (the per-row
// ParentType is already *string).
func (s *PDFService) GetReverseRefs(tabID string, nodeID string) ([]*pdfcore.ReverseRef, error) {
	var result []*pdfcore.ReverseRef
	var err error
	func() {
		defer recoverRuntimePanic("GetReverseRefs", &err)
		refs, e := s.inspector.GetReverseRefs(tabID, nodeID)
		if e != nil {
			err = e
			return
		}
		out := make([]*pdfcore.ReverseRef, len(refs))
		for i := range refs {
			rr := refs[i]
			out[i] = &rr
		}
		result = out
	}()
	return result, err
}

// GoToPage resolves a 1-based page number to the node ID of that page's
// content stream, suitable for the frontend to dispatch as a NAVIGATE_TO_REF
// target. Returns an error if the page number is out of range, the page has
// no content stream, or the document/tab is unknown.
func (s *PDFService) GoToPage(tabID string, pageNum int) (string, error) {
	var result string
	var err error
	func() {
		defer recoverRuntimePanic("GoToPage", &err)
		result, err = s.inspector.GetPageContentStreamNodeID(tabID, pageNum)
	}()
	return result, err
}

// GetObjectIndex returns the full xref-derived object index for the document
// in tabID. Powers the Cmd+K command palette (Story 9-8). Lazy on first call,
// cached per document state.
func (s *PDFService) GetObjectIndex(tabID string) ([]*pdfcore.ObjectIndexEntry, error) {
	var result []*pdfcore.ObjectIndexEntry
	var err error
	func() {
		defer recoverRuntimePanic("GetObjectIndex", &err)
		result, err = s.inspector.GetObjectIndex(tabID)
	}()
	return result, err
}

// GetXRefTable returns the cross-reference table view for the document in
// tabID. Lazy on first call, cached per document state. Story 9-11.
func (s *PDFService) GetXRefTable(tabID string) (*pdfcore.XRefTable, error) {
	var result *pdfcore.XRefTable
	var err error
	func() {
		defer recoverRuntimePanic("GetXRefTable", &err)
		result, err = s.inspector.GetXRefTable(tabID)
	}()
	return result, err
}

// GetPlainText returns the Latin-1-decoded file bytes for the document in
// tabID. Story 10-1 (replaces the 9-11 25 MiB cap + 9-12 "Load all" two-tier
// model with a single uncapped lazy-load + cancellable chunked read). The
// read is cancellable via CancelPlainText; cancellation surfaces an error
// satisfying errors.Is(err, context.Canceled).
//
// Story 10-5 AC5: NOT wrapped by recoverRuntimePanic. GetPlainText reads raw
// disk bytes and never calls into the PDF backend; a runtime.Error here is a
// Go bug in our non-backend code and SHOULD crash loudly rather than be
// laundered as "malformed PDF" (which would mislead the user).
func (s *PDFService) GetPlainText(tabID string) (*pdfcore.PlainTextDocument, error) {
	return s.inspector.GetPlainText(tabID)
}

// CancelPlainText cancels an in-flight GetPlainText for tabID. No-op when no
// load is in flight. Returns ErrDocumentNotFound for unknown tabs. Story 10-1.
//
// Story 10-5 AC5: NOT wrapped by recoverRuntimePanic (non-backend code path).
func (s *PDFService) CancelPlainText(tabID string) error {
	return s.inspector.CancelPlainText(tabID)
}

// GetPlainTextSize returns the on-disk byte size of the PDF backing tabID.
// Powers the loading-card size disclosure on PlainTextView. Story 10-1.
//
// Story 10-5 AC5: NOT wrapped by recoverRuntimePanic (non-backend code path).
func (s *PDFService) GetPlainTextSize(tabID string) (int64, error) {
	return s.inspector.GetPlainTextSize(tabID)
}
