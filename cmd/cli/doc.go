// Package main provides the pdfdebug CLI binary for PDF inspection.
//
// The CLI consumes internal/pdfcore directly (zero Wails dependency) and
// exposes these dump subcommands: tree, object, stream, page, font, image,
// source, reverserefs, xref, objects, plaintext, embedded, metadata, and
// signatures.
//
// Output is human-readable PLAIN TEXT on stdout by default; pass --json to emit
// structured JSON instead. The plain-text form is for reading and is NOT a
// stable contract (it may change between releases) - parse the --json form.
// Two payload selectors stand outside the format rule: `dump stream --raw` /
// `--ops` and the raw `dump plaintext` bytes are deliberate machine formats.
// `dump page --info --json` is EXPERIMENTAL and carries an in-band
// "_stability":"experimental" marker. Errors are always JSON on stderr.
//
// Exit codes:
//
//	0 - success
//	1 - usage error (bad flags, missing file argument)
//	2 - runtime error (file not found, malformed PDF, decode failure, internal panic)
package main
