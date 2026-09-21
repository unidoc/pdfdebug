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
//	0 - usually empty. A structurally damaged but parseable file draws a JSON
//	    warning object, and a non-fatal degradation draws a plain-text
//	    `warning:` line (a failed --resolve in `dump tree` / `dump object`, an
//	    unavailable Do classification in `dump stream`)
//	1 - plain text: on the document-level dumps (bytes, xref, objects,
//	    signatures) the usage line alone, and in `dump embedded` an `error:`
//	    line above it. `dump stream` and `dump page` are the exception - they
//	    report a rejected flag combination as JSON
//	2 - a single JSON object carrying an `error` key, except where a payload
//	    path reports its own failure as plain text: the raw `dump bytes` and
//	    `dump source --raw` write failures, and `dump embedded --name` naming
//	    no attachment
//
// The deprecated `dump plaintext` alias prefixes a one-line deprecation notice
// to stderr on every one of those paths, so under that spelling stderr taken as
// a whole never parses as JSON.
//
// Exit codes (dump subcommands):
//
//	0 - success
//	1 - usage error (bad flags, missing file argument, and on the
//	    document-level dumps an extra positional argument)
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
