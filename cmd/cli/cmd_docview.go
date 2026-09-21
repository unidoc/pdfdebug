package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// docViewFlags holds the parsed flags common to the document-level dump
// subcommands (xref, objects, bytes, signatures). None take --ref.
type docViewFlags struct {
	pretty bool
	json   bool // opt into JSON: structured for xref/objects/signatures, the text wrapper for bytes
}

// parseDocViewFlags builds and parses a FlagSet for a document-level dump
// subcommand. resource is the bare resource name used in the usage message.
// --json is read into f.json uniformly; the handler decides its meaning: a
// format switch for xref/objects/signatures (plain-text default -> JSON) or a
// payload wrapper for bytes (raw bytes default -> decoded JSON payload). On
// failure it writes usage and returns ok=false.
func parseDocViewFlags(resource string, args []string) (filePath string, f docViewFlags, ok bool) {
	usage := fmt.Sprintf("Usage: pdfdebug dump %s [--json] [--pretty] <file>", resource)

	fs := flag.NewFlagSet("dump "+resource, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output (no effect on plain text)")
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is human-readable plain text)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, usage)
		return "", docViewFlags{}, false
	}
	// Exactly one positional argument, the file.
	if !requirePositionals(fs, 1, usage) {
		return "", docViewFlags{}, false
	}
	return fs.Arg(0), docViewFlags{pretty: *prettyFlag, json: *jsonFlag}, true
}
