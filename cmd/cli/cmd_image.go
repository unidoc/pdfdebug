package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runImageDump parses flags and dispatches image-data dump execution.
func runImageDump(args []string) int {
	filePath, f, ok := parseByRefFlags("image", args, false, true)
	if !ok {
		return 1
	}
	return execImageDump(filePath, f)
}

// execImageDump opens the PDF and emits the ImageData payload as JSON. A
// non-image node yields ImageData with a populated `error` field at exit 0 (the
// method reports type mismatch in the payload, not via a Go error). With
// --metadata the base64 key is omitted entirely (not blanked) so the output
// stays small; literal omission requires a projection because ImageData.Base64
// has no omitempty tag.
func execImageDump(filePath string, f byRefFlags) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins, nodeID, _, code := openByRef(f.ref, filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	img, err := ins.GetImageData("cli", nodeID)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	if !f.json {
		// Plain-text default: an aligned key/value block. The base64 payload is
		// never printed in plain text (it would flood the terminal); --metadata is
		// therefore a JSON-only distinction. NON-CONTRACTUAL; use --json to parse.
		if err := printImagePlain(os.Stdout, img); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}

	if f.metadata {
		// Project to a map and drop the base64 key entirely. Decode into
		// json.RawMessage (not any) so the surviving fields re-emit byte-for-byte
		// -- decoding into map[string]any would relabel every number as float64.
		b, err := json.Marshal(img)
		if err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		delete(m, "base64")
		if err := emit(os.Stdout, m, f.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}

	if err := emit(os.Stdout, img, f.pretty); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printImagePlain renders ImageData as an aligned key/value block (omitting the
// base64 payload). A populated error/warning field is surfaced as its own row.
func printImagePlain(out io.Writer, img *pdfcore.ImageData) error {
	var w kvWriter
	w.Add("Object", img.ObjectRef)
	if img.Error != "" {
		w.Add("Error", img.Error)
		return w.Render(out)
	}
	w.Add("MimeType", img.MimeType)
	w.Addf("Width", "%d", img.Width)
	w.Addf("Height", "%d", img.Height)
	w.Add("ColorSpace", dashIfEmpty(img.ColorSpace))
	w.Add("BitsPerComponent", strconv.Itoa(img.BitsPerComponent))
	w.Add("Filter", dashIfEmpty(img.Filter))
	if img.Warning != "" {
		w.Add("Warning", img.Warning)
	}
	return w.Render(out)
}
