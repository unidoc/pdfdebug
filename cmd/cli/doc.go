// Package main provides the pdfdebug CLI binary for PDF inspection.
//
// The CLI consumes internal/pdfcore directly (zero Wails dependency) and
// exposes these dump subcommands: tree, object, stream, font, image, source,
// reverserefs, xref, objects, and plaintext. Output is JSON on stdout (plaintext
// defaults to raw document bytes); errors are JSON on stderr.
//
// Exit codes:
//
//	0 - success
//	1 - usage error (bad flags, missing file argument)
//	2 - runtime error (file not found, malformed PDF, decode failure, internal panic)
package main
