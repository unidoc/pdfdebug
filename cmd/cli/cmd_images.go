package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runImagesDump parses flags and dispatches the document-level image-index
// dump. NOTE: this is the PLURAL command (every image XObject referenced from
// page resources); the SINGULAR `dump image --ref` extracts one image.
func runImagesDump(args []string) int {
	filePath, f, ok := parseDocViewFlags("images", args)
	if !ok {
		return 1
	}
	return execImagesDump(filePath, f)
}

// execImagesDump opens the PDF and renders its image index.
func execImagesDump(filePath string, f docViewFlags) (exitCode int) {
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

	index, err := ins.GetImageIndex("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	return writeImagesDump(os.Stdout, os.Stderr, index, f)
}

// writeImagesDump writes the image index to stdout: the JSON entry array with
// --json, a table otherwise. Error rows and per-image errors are findings, so
// the exit code stays 0. A failed write exits 2 with the error object alone
// on stderr.
func writeImagesDump(stdout, stderr io.Writer, index []*pdfcore.ImageIndexEntry, f docViewFlags) int {
	var err error
	if f.json {
		err = emit(stdout, index, f.pretty)
	} else {
		err = printImagesPlain(stdout, index)
	}
	if err != nil {
		writeJSONError(stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printImagesPlain renders the image index as an aligned table, one row per
// entry. NON-CONTRACTUAL; use --json to parse.
func printImagesPlain(out io.Writer, index []*pdfcore.ImageIndexEntry) error {
	t := newTable("REF", "SIZE", "BPC", "COLORSPACE", "FILTERS", "FLAGS", "EST", "PAGES", "ERROR")
	for _, e := range index {
		if e.NodeID == "" {
			t.AddRow("-", "-", "-", "-", "-", "-", "-", "-", dashIfEmpty(asciiSafe(e.Err)))
			continue
		}
		pages := make([]string, len(e.FirstPages))
		for i, p := range e.FirstPages {
			pages[i] = strconv.Itoa(p)
		}
		pagesCell := fmt.Sprintf("%d: %s", e.PageCount, strings.Join(pages, ","))
		if e.PageCount > len(e.FirstPages) {
			pagesCell += ", ..."
		}
		errCell := e.Err
		if errCell == "" && e.Warning != "" {
			errCell = "warning: " + e.Warning
		}
		t.AddRow(
			fmt.Sprintf("%d %d R", e.ObjNum, e.Gen),
			fmt.Sprintf("%dx%d", e.Width, e.Height),
			strconv.Itoa(e.BitsPerComponent),
			dashIfEmpty(asciiSafe(e.ColorSpace)),
			dashIfEmpty(asciiSafe(strings.Join(e.Filters, ","))),
			dashIfEmpty(strings.Join(imageFlags(e), ",")),
			strconv.FormatInt(e.EstimatedBytes, 10),
			pagesCell,
			dashIfEmpty(asciiSafe(errCell)),
		)
	}
	return t.Render(out)
}

// imageFlags returns the badge words for an entry, in display order.
func imageFlags(e *pdfcore.ImageIndexEntry) []string {
	var flags []string
	if e.ImageMask {
		flags = append(flags, "mask")
	}
	if e.SMask != nil {
		flags = append(flags, "SMask")
	}
	if e.DecodeNonDefault {
		flags = append(flags, "Decode")
	}
	if e.AdobeMarker == pdfcore.AdobeMarkerPresent {
		if e.AdobeTransform != nil {
			flags = append(flags, fmt.Sprintf("APP14 t=%d", *e.AdobeTransform))
		} else {
			flags = append(flags, "APP14")
		}
	}
	if e.Warning != "" {
		flags = append(flags, "warn")
	}
	return flags
}
