//go:build !mcp

package pdfservice

// takeQueuedDialogPaths returns nil; the dialog queue exists only in builds
// with the mcp tag.
func takeQueuedDialogPaths() []string { return nil }
