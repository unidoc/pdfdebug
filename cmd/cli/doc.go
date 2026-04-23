// Package main provides the pdfdebug CLI binary for PDF inspection.
//
// The CLI consumes internal/pdfcore directly (zero Wails dependency) and
// exposes three dump subcommands: tree, object, and stream. Output is
// JSON on stdout; errors are JSON on stderr. Exit codes: 0 success,
// 1 usage error, 2 file/PDF error.
package main
