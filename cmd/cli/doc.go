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
//	    warning object; a non-fatal degradation draws a plain-text line - a
//	    `warning:` for a failed --resolve in `dump tree` / `dump object` and
//	    for an unavailable Do classification in `dump stream --ops`, and a bare
//	    "page has no content stream" note from `dump stream --ops`
//	1 - plain text when the argument SHAPE is wrong (unparseable flags, a
//	    missing --ref or <file>, an extra positional): the usage line, with the
//	    flag list below it in `dump font`. A rejected flag VALUE is JSON
//	    instead - a malformed --ref (object, font, image, source, reverserefs),
//	    an out-of-range --depth / --resolve-depth / --page / --info /
//	    --forms-depth or an unknown --section (tree, object, stream, page), and
//	    a mutually exclusive flag pair in `dump stream` and `dump source`. Two
//	    kinds of case cross that line. The --ref/--name pair of `dump embedded`
//	    is a rejected combination reported as plain text, an `error:` line
//	    above the usage line. An absent mode selector is a missing flag
//	    reported as JSON: no --info on `dump page`, no --page / --ref /
//	    --xobject on `dump stream`, and no --page or --ref to own a
//	    `dump stream --xobject`. The shape check runs ahead of every
//	    flag-value check, so an invocation wrong in both ways draws the usage
//	    line: `dump page file.pdf --info 1` leaves the selector behind the
//	    file, where the parser never sees it, and is reported as the shape
//	    error it is rather than as an absent --info
//	2 - a single JSON object carrying an `error` key, except where a payload
//	    path reports its own failure as plain text: the write failures of the
//	    raw `dump bytes`, of `dump source` on both --raw and the plain-text
//	    default, and of `dump stream` on --raw and the no-content-stream note;
//	    and `dump embedded --name` when the name matches no attachment, matches
//	    several, or matches one carrying no /EmbeddedFile stream
//
// The deprecated `dump plaintext` alias prefixes a one-line deprecation notice
// to stderr on every one of those paths, so under that spelling stderr taken as
// a whole never parses as JSON.
//
// Exit codes (dump subcommands):
//
//	0 - success
//	1 - usage error (bad flags, or a <file> operand that is not exactly the one
//	    non-empty path every dump subcommand takes - a missing file, an empty
//	    path such as `dump tree ""`, a second file, or a flag written after the
//	    file, which Go's flag package delivers as a positional rather than
//	    parsing it)
//	2 - runtime error (file not found, malformed PDF, decode failure, internal panic)
//
// The `validate` command uses a DIFFERENT three-way exit contract so CI can
// distinguish a non-compliant file from a broken tool (it must not overload the
// dump exit 2):
//
//	0 - ran successfully, no structural errors found (NOT a compliance/valid verdict)
//	1 - ran successfully AND found >=1 structural error (the compliance-gate signal)
//	2 - operational error (missing/unreadable file, a <file> operand other than
//	    one non-empty path, unknown profile, view failure)
package main
