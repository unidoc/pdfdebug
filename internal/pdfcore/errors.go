package pdfcore

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrDocumentNotFound = errors.New("document not found")
	ErrMalformedPDF     = errors.New("malformed PDF")
	ErrEncryptedPDF     = errors.New("encrypted PDF: password required")
	ErrUnsupportedPDF   = errors.New("unsupported PDF version or feature")
)

func safeCall(fn func() error) error {
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("pdf parsing panic: %v", r)
			}
		}()
		err = fn()
	}()
	return err
}

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
