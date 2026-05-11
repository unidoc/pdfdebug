package pdfcore

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// Sentinel errors for PDF processing failures.
var (
	// ErrDocumentNotFound indicates a requested document or tab does not exist.
	ErrDocumentNotFound = errors.New("document not found")
	// ErrMalformedPDF indicates the PDF structure is invalid or corrupt.
	ErrMalformedPDF = errors.New("malformed PDF")
	// ErrEncryptedPDF indicates the PDF requires a password to open.
	ErrEncryptedPDF = errors.New("encrypted PDF: password required")
	// ErrUnsupportedPDF indicates the PDF uses a version or feature not handled.
	ErrUnsupportedPDF = errors.New("unsupported PDF version or feature")
	// ErrReverseRefIndexUnavailable indicates the reverse-reference index could
	// not be built for the document (e.g. a panic during BFS). Distinct from
	// "empty list" because the latter is the orphan-hunt signal and must not
	// be confused with a build failure.
	ErrReverseRefIndexUnavailable = errors.New("reverse-ref index unavailable")
)

// safeCall executes fn inside a panic-recovery wrapper. pdfcpu can panic on
// malformed input (string panics from internal validation and the os.Exit ->
// panic library-safety fix), so every call into the library goes through this.
//
// A runtime.Error (nil deref, slice OOB, bad type assertion) signals a genuine
// Go bug in our code path, not a malformed PDF. It is re-panicked so it
// surfaces loudly instead of being laundered into ErrMalformedPDF.
func safeCall(fn func() error) error {
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(runtime.Error); ok {
					panic(r)
				}
				err = fmt.Errorf("pdf parsing panic: %v", r)
			}
		}()
		err = fn()
	}()
	return err
}

// wrapPDFError classifies a raw error into the appropriate sentinel category
// (encrypted, malformed) so callers can match with errors.Is.
func wrapPDFError(err error) error {
	msg := err.Error()
	if strings.HasPrefix(msg, "pdf parsing panic:") {
		return fmt.Errorf("%w: %v", ErrMalformedPDF, err)
	}
	if strings.Contains(msg, "password") {
		return fmt.Errorf("%w: %v", ErrEncryptedPDF, err)
	}
	return fmt.Errorf("%w: %v", ErrMalformedPDF, err)
}
