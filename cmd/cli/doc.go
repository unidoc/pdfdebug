// Package main provides the pdfdebug CLI binary for PDF inspection.
//
// The CLI consumes internal/pdfcore directly (zero Wails dependency) and
// exposes these dump subcommands: tree, object, stream, page, font, image,
// source, reverserefs, xref, objects, bytes, embedded, metadata, and
// signatures, plus the top-level `validate` command (bounded structural
// PDF/A-1b and PDF/UA-1 conformance checks).
//
// Output is human-readable PLAIN TEXT on stdout by default; pass --json to emit
// structured JSON instead. The plain-text form is for reading and is NOT a
// stable contract (it may change between releases) - parse the --json form.
// Two payload selectors stand outside the format rule: `dump stream --raw` /
// `--ops` and the raw `dump bytes` output are deliberate machine formats.
// `dump page --info --json` is EXPERIMENTAL and carries an in-band
// "_stability":"experimental" marker.
//
// Stderr, per exit path (dump subcommands):
//
//	0 - empty, except that a structurally damaged but parseable file draws a
//	    JSON warning object while the command still succeeds
//	1 - a bare plain-text usage line, NOT JSON
//	2 - a single JSON object carrying an `error` key, with one standing
//	    exception: the raw `dump bytes` path reports a write failure as plain
//	    text
//
// The deprecated `dump plaintext` alias prefixes a one-line deprecation notice
// to stderr on every one of those paths, so under that spelling stderr taken as
// a whole never parses as JSON.
//
// Exit codes (dump subcommands):
//
//	0 - success
//	1 - usage error (bad flags, missing file argument, extra positional argument)
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
