package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"unidoc-pdf-debugger/internal/pdfcore"
)

const metadataUsage = "Usage: pdfdebug dump metadata [--json] <file>"

// runMetadataDump parses flags and dispatches the document-metadata command,
// printing the /Info dictionary fields and the XMP packet.
func runMetadataDump(args []string) int {
	fs := flag.NewFlagSet("dump metadata", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is human-readable plain text)")
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output (no effect on plain text)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, metadataUsage)
		return 1
	}

	filePath := fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, metadataUsage)
		return 1
	}
	return execMetadataDump(filePath, *jsonFlag, *prettyFlag)
}

// execMetadataDump opens the PDF and writes the metadata view as an aligned
// Info block plus XMP packet (default) or {info:{...}, xmp:"..."} JSON.
func execMetadataDump(filePath string, jsonOut, pretty bool) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins, _, code := openForCLI(filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	md, err := ins.GetDocumentMetadata("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	if jsonOut {
		if err := emit(os.Stdout, md, pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if err := printMetadataPlain(os.Stdout, md); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printMetadataPlain renders DocumentMetadata as an aligned "Key: value" Info
// block followed by the verbatim XMP packet. NON-CONTRACTUAL; use --json to
// parse. infoFieldOrder fixes the row order so output is stable.
//
// ASCII-only applies to the Info block. The XMP packet is UTF-8 XML and is
// passed through verbatim, so this command can emit non-ASCII bytes below the
// "XMP:" heading.
func printMetadataPlain(out io.Writer, md *pdfcore.DocumentMetadata) error {
	var w kvWriter
	w.Heading("Info:")
	any := false
	for _, key := range infoFieldOrder {
		if v, ok := md.Info[key]; ok && v != "" {
			// This surface is ASCII-only.
			w.Add("  "+key, asciiSafe(v))
			any = true
		}
	}
	if !any {
		w.Add("  (none)", "")
	}
	if md.Warning != "" {
		w.Gap()
		w.Add("Warning", md.Warning)
	}
	w.Gap()
	w.Heading("XMP:")
	if err := w.Render(out); err != nil {
		return err
	}
	xmp := md.XMP
	if xmp == "" {
		xmp = "(none)"
	}
	// Ensure a trailing newline so the plain-text-default contract holds even when
	// the XMP packet does not end in one.
	if len(xmp) == 0 || xmp[len(xmp)-1] != '\n' {
		xmp += "\n"
	}
	_, err := io.WriteString(out, xmp)
	return err
}

// infoFieldOrder is the stable plain-text row order for /Info fields, mirroring
// the spec's conventional ordering.
var infoFieldOrder = []string{
	"Title", "Author", "Subject", "Keywords",
	"Creator", "Producer", "CreationDate", "ModDate",
}
