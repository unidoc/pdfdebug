package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// docViewFlags holds the parsed flags common to the document-level dump
// subcommands (xref, objects, plaintext). None take --ref.
type docViewFlags struct {
	pretty bool
	json   bool // plaintext only: wrap the text as JSON instead of raw bytes
}

// parseDocViewFlags builds and parses a FlagSet for a document-level dump
// subcommand. resource is the bare resource name used in the usage message;
// jsonWraps gates the plaintext --json wrapper flag (other commands treat --json
// as the always-on default no-op). On failure it writes usage and returns ok=false.
func parseDocViewFlags(resource string, args []string, jsonWraps bool) (filePath string, f docViewFlags, ok bool) {
	// plaintext's --json wrapper is behavior-changing, so surface it in the
	// worked example; xref/objects treat --json as the always-on default no-op.
	opts := "[--pretty]"
	if jsonWraps {
		opts = "[--json] " + opts
	}
	usage := fmt.Sprintf("Usage: pdfdebug dump %s %s <file>", resource, opts)

	fs := flag.NewFlagSet("dump "+resource, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output")
	jsonFlag := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, usage)
		return "", docViewFlags{}, false
	}
	filePath = fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, usage)
		return "", docViewFlags{}, false
	}
	f = docViewFlags{pretty: *prettyFlag}
	if jsonWraps {
		f.json = *jsonFlag
	}
	return filePath, f, true
}
