// Package main provides the pdfdebug CLI binary for PDF inspection.
//
// The CLI consumes internal/pdfcore directly (zero Wails dependency) and
// exposes these dump subcommands: tree, object, stream, page, font, image,
// source, reverserefs, xref, objects, plaintext, embedded, metadata, and
// signatures, plus the top-level `validate` command (bounded structural
// PDF/A-1b and PDF/UA-1 conformance checks).
//
// Output is human-readable PLAIN TEXT on stdout by default; pass --json to emit
// structured JSON instead. The plain-text form is for reading and is NOT a
// stable contract (it may change between releases) - parse the --json form.
// Two payload selectors stand outside the format rule: `dump stream --raw` /
// `--ops` and the raw `dump plaintext` bytes are deliberate machine formats.
// `dump page --info --json` is EXPERIMENTAL and carries an in-band
// "_stability":"experimental" marker. Errors are always JSON on stderr.
//
// Exit codes (dump subcommands):
//
//	0 - success
//	1 - usage error (bad flags, missing file argument)
//	2 - runtime error (file not found, malformed PDF, decode failure, internal panic)
//
// The `validate` command uses a DIFFERENT three-way exit contract so CI can
// distinguish a non-compliant file from a broken tool (it must not overload the
// dump exit 2):
//
//	0 - ran successfully, no structural errors found (NOT a compliance/valid verdict)
//	1 - ran successfully AND found >=1 structural error (the compliance-gate signal)
//	2 - operational error (missing/unreadable file, unknown profile, view failure)
package main
