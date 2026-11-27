//go:build !windows

package pdfcore

// isFileBusy is always false off Windows: no other OS reports a sharing
// violation on open or read.
func isFileBusy(error) bool {
	return false
}
