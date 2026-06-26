package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// byRefFlags holds the parsed flags common to the reference-taking dump
// subcommands (font, image, source, reverserefs).
type byRefFlags struct {
	ref      string
	json     bool // emit structured JSON instead of the plain-text default
	pretty   bool
	raw      bool // source only
	metadata bool // image only
}

// parseByRefFlags builds and parses a FlagSet for a ref-taking dump subcommand.
// resource is the bare resource name (e.g. "font") used in the worked-example
// usage message. withRaw / withMetadata gate the source-only / image-only flags.
// On any parse/usage failure it writes the resource-specific worked example to
// stderr and returns ok=false with the exit code (always 1 for usage).
func parseByRefFlags(resource string, args []string, withRaw, withMetadata bool) (filePath string, f byRefFlags, ok bool) {
	// Build the usage line from the flags this command actually accepts, so the
	// worked example advertises --raw / --metadata rather than the no-op --json.
	opts := "[--pretty]"
	if withRaw {
		opts = "[--raw] " + opts
	}
	if withMetadata {
		opts = "[--metadata] " + opts
	}
	usage := fmt.Sprintf(`Usage: pdfdebug dump %s %s --ref "N G R" <file>`, resource, opts)

	fs := flag.NewFlagSet("dump "+resource, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	refFlag := fs.String("ref", "", `Object reference in "N G R" or obj:G:N form`)
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output (no effect on plain text)")
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is human-readable plain text)")
	var rawFlag, metadataFlag *bool
	if withRaw {
		rawFlag = fs.Bool("raw", false, "Emit verbatim source bytes instead of JSON")
	}
	if withMetadata {
		metadataFlag = fs.Bool("metadata", false, "Omit the base64 payload, keep metadata only")
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, usage)
		return "", byRefFlags{}, false
	}
	if *refFlag == "" {
		fmt.Fprintln(os.Stderr, usage)
		return "", byRefFlags{}, false
	}
	filePath = fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, usage)
		return "", byRefFlags{}, false
	}

	f = byRefFlags{ref: *refFlag, json: *jsonFlag, pretty: *prettyFlag}
	if rawFlag != nil {
		f.raw = *rawFlag
	}
	if metadataFlag != nil {
		f.metadata = *metadataFlag
	}
	return filePath, f, true
}

// openByRef parses the ref, opens the PDF, and returns the live inspector plus
// the constructed nodeID ("obj:<gen>:<num>", gen first) and canonical "N G R"
// objectRef. The caller owns inspector.Close. On any failure it writes the
// error and returns code != 0 (1 for a bad ref, the openForCLI code otherwise);
// the inspector is nil in that case.
func openByRef(ref, filePath string) (ins *pdfcore.Inspector, nodeID, objectRef string, code int) {
	objNum, genNum, err := parseObjectRef(ref)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return nil, "", "", 1
	}

	ins, _, code = openForCLI(filePath)
	if code != 0 {
		return nil, "", "", code
	}

	nodeID = fmt.Sprintf("obj:%d:%d", genNum, objNum)
	objectRef = fmt.Sprintf("%d %d R", objNum, genNum)
	return ins, nodeID, objectRef, 0
}
