package pdfservice

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"

	"unidoc-pdf-debugger/internal/pdfcore"
	"unidoc-pdf-debugger/internal/pendingopen"
)

// inspectorAPI is the method set PDFService uses from the inspector backend.
// Declaring it as an unexported interface (rather than holding a concrete
// pointer) is the test seam: tests inject a stub that panics with a synthetic
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
	GetEmbeddedFiles(tabID string) (*pdfcore.EmbeddedFileList, error)
	GetEmbeddedFileBytes(tabID string, nodeID string) ([]byte, error)
	GetDocumentMetadata(tabID string) (*pdfcore.DocumentMetadata, error)
	GetSignatures(tabID string) (*pdfcore.SignatureList, error)
	Validate(tabID, profile string) (*pdfcore.ValidationResult, error)
	DiffDocuments(leftTabID, rightTabID string) (*pdfcore.DiffResult, error)
}

// PDFService is the Wails-bound service that exposes PDF inspection to the
// frontend. It delegates to the inspector backend for all PDF operations.
//
// The inspector field is typed as the inspectorAPI interface (the test seam)
// rather than the concrete *pdfcore.Inspector; the production constructor
// injects a *pdfcore.Inspector via pdfcore.NewInspector() and tests inject a
// stub that panics with a runtime.Error to drive the recoverRuntimePanic path.
type PDFService struct {
	inspector inspectorAPI
	app       *application.App
	// pendingOpens buffers cold-start file-association paths until the
	// frontend drains them. Injected from main.go via
	// SetPendingOpens; nil in tests that do not exercise the cold-start path.
	pendingOpens *pendingopen.Queue
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
// Each wrapper invokes the inspector call inside an anonymous closure that
// owns the deferred recover; the closure writes its result and error into
// outer locals (via shadow) and the recover overwrites the error local via
// pointer if a runtime.Error fires. This pattern keeps the method signatures
// stable -- the signature-preservation contract continues to hold, with no
// named returns -- while still letting the recover replace the returned error.
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
// signal to silently render the generic DictView.
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
// can render the empty state.
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
// index built lazily on first call. Returns
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
// in tabID. Powers the Cmd+K command palette. Lazy on first call,
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
// tabID. Lazy on first call, cached per document state.
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
// tabID. There is no size cap and no two-tier "Load all" step: a single
// uncapped lazy load feeds a cancellable chunked read. The
// read is cancellable via CancelPlainText; cancellation surfaces an error
// satisfying errors.Is(err, context.Canceled).
//
// NOT wrapped by recoverRuntimePanic. GetPlainText reads raw
// disk bytes and never calls into the PDF backend; a runtime.Error here is a
// Go bug in our non-backend code and SHOULD crash loudly rather than be
// laundered as "malformed PDF" (which would mislead the user).
func (s *PDFService) GetPlainText(tabID string) (*pdfcore.PlainTextDocument, error) {
	return s.inspector.GetPlainText(tabID)
}

// CancelPlainText cancels an in-flight GetPlainText for tabID. No-op when no
// load is in flight. Returns ErrDocumentNotFound for unknown tabs.
//
// NOT wrapped by recoverRuntimePanic (non-backend code path).
func (s *PDFService) CancelPlainText(tabID string) error {
	return s.inspector.CancelPlainText(tabID)
}

// GetPlainTextSize returns the on-disk byte size of the PDF backing tabID.
// Powers the loading-card size disclosure on PlainTextView.
//
// NOT wrapped by recoverRuntimePanic (non-backend code path).
func (s *PDFService) GetPlainTextSize(tabID string) (int64, error) {
	return s.inspector.GetPlainTextSize(tabID)
}

// SetPendingOpens injects the cold-start file-association queue from main.go.
// The queue is constructed in main.go (app-shell state) and wired
// here so the bound ConsumePendingOpenFiles method can drain it.
func (s *PDFService) SetPendingOpens(q *pendingopen.Queue) {
	s.pendingOpens = q
}

// ConsumePendingOpenFiles drains and returns any file-association paths that
// were buffered before the frontend was ready (cold start). A thin
// delegation to Queue.Drain -- the frontend calls this immediately after
// registering its document:opened listener. Returns nil when no queue is wired
// (the unwired-or-empty case marshals to JSON null, which the frontend treats
// as an empty list).
func (s *PDFService) ConsumePendingOpenFiles() []string {
	if s.pendingOpens == nil {
		return nil
	}
	return s.pendingOpens.Drain()
}

// GetEmbeddedFiles enumerates the embedded/associated files for the document in
// tabID (catalog /AF + /Names/EmbeddedFiles, merged and deduped).
func (s *PDFService) GetEmbeddedFiles(tabID string) (*pdfcore.EmbeddedFileList, error) {
	var result *pdfcore.EmbeddedFileList
	var err error
	func() {
		defer recoverRuntimePanic("GetEmbeddedFiles", &err)
		result, err = s.inspector.GetEmbeddedFiles(tabID)
	}()
	return result, err
}

// GetEmbeddedFileBytes returns the decoded bytes of one embedded file, addressed
// by the obj:G:N nodeID of its /EmbeddedFile stream. Wails marshals the []byte
// as a base64 string to the frontend.
func (s *PDFService) GetEmbeddedFileBytes(tabID string, nodeID string) ([]byte, error) {
	var result []byte
	var err error
	func() {
		defer recoverRuntimePanic("GetEmbeddedFileBytes", &err)
		result, err = s.inspector.GetEmbeddedFileBytes(tabID, nodeID)
	}()
	return result, err
}

// GetDocumentMetadata returns the document's XMP packet and /Info dictionary
// fields for the document in tabID.
func (s *PDFService) GetDocumentMetadata(tabID string) (*pdfcore.DocumentMetadata, error) {
	var result *pdfcore.DocumentMetadata
	var err error
	func() {
		defer recoverRuntimePanic("GetDocumentMetadata", &err)
		result, err = s.inspector.GetDocumentMetadata(tabID)
	}()
	return result, err
}

// GetSignatures returns the decomposed digital-signature fields for the
// document in tabID as a flat array (the frontend consumes the entry list
// directly; an empty document yields an empty array). Structural
// decomposition only - no trust verdict of any kind.
func (s *PDFService) GetSignatures(tabID string) ([]pdfcore.SignatureField, error) {
	var result []pdfcore.SignatureField
	var err error
	func() {
		defer recoverRuntimePanic("GetSignatures", &err)
		list, e := s.inspector.GetSignatures(tabID)
		if e != nil {
			err = e
			return
		}
		result = list.Signatures
	}()
	return result, err
}

// Validate runs the bounded structural conformance rule set for profile
// against the document in tabID and returns the problem list, tally, and
// disclaimer. Structural checks only - not authoritative conformance. Story
// 13.5.
func (s *PDFService) Validate(tabID, profile string) (*pdfcore.ValidationResult, error) {
	var result *pdfcore.ValidationResult
	var err error
	func() {
		defer recoverRuntimePanic("Validate", &err)
		result, err = s.inspector.Validate(tabID, profile)
	}()
	return result, err
}

// DiffDocuments computes the path-aligned structural delta between two already
// open documents (both tab IDs must be open in this service's inspector). It is
// a read-only walk over both documents aligned by structural path, NOT object
// number.
func (s *PDFService) DiffDocuments(leftTabID, rightTabID string) (*pdfcore.DiffResult, error) {
	var result *pdfcore.DiffResult
	var err error
	func() {
		defer recoverRuntimePanic("DiffDocuments", &err)
		result, err = s.inspector.DiffDocuments(leftTabID, rightTabID)
	}()
	return result, err
}

// SaveBytesToFile shows a native Save-file dialog seeded with suggestedName and
// writes data to the chosen path, returning the saved path. An empty path (user
// cancelled) returns ("", nil) so the frontend can treat cancel as a no-op.
// The GUI "Save..." action for an extracted embedded file. This is
// the first save-dialog path in the app (no prior SaveFile usage); it is NOT
// part of inspectorAPI because it is an app-level dialog, not a PDF-backend
// call.
func (s *PDFService) SaveBytesToFile(suggestedName string, data []byte) (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("app not initialized")
	}
	path, err := s.app.Dialog.SaveFile().
		SetFilename(suggestedName).
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if path == "" {
		// User cancelled the dialog.
		return "", nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
