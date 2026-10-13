package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runPagesDump parses flags and dispatches the document-level page-index
// dump. NOTE: this is the PLURAL command (every page leaf of the page tree);
// the SINGULAR `dump page --info N` assembles render info for one page.
func runPagesDump(args []string) int {
	filePath, f, ok := parseDocViewFlags("pages", args)
	if !ok {
		return 1
	}
	return execPagesDump(filePath, f)
}

// execPagesDump opens the PDF and renders its page index.
func execPagesDump(filePath string, f docViewFlags) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins, info, code := openForCLI(filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	index, err := ins.GetPageIndex("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	return writePagesDump(os.Stdout, os.Stderr, index, info.PageCount, f)
}

// writePagesDump writes the page index to stdout (the JSON entry array with
// --json, a table otherwise) and, when the numbered-row count disagrees with
// the root /Count, a JSON warning naming both numbers to stderr. The
// disagreement and per-row errors are findings, so the exit code stays 0. A
// failed write exits 2 with the error object alone on stderr.
func writePagesDump(stdout, stderr io.Writer, index []*pdfcore.PageIndexEntry, pageCount int, f docViewFlags) int {
	var err error
	if f.json {
		err = emit(stdout, index, f.pretty)
	} else {
		err = printPagesPlain(stdout, index)
	}
	if err != nil {
		writeJSONError(stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	if msg := pageCountDisagreement(index, pageCount); msg != "" {
		writeJSONWarning(stderr, msg)
	}
	return 0
}

// pageCountDisagreement returns a warning when the number of page leaves in
// index differs from pageCount (the root /Count), or "" when they agree. A
// pageCount of 0 means /Count was 0 or could not be read.
func pageCountDisagreement(index []*pdfcore.PageIndexEntry, pageCount int) string {
	leaves := 0
	for _, e := range index {
		if e.PageNum > 0 {
			leaves++
		}
	}
	if leaves == pageCount {
		return ""
	}
	if pageCount == 0 {
		return fmt.Sprintf("page tree has %d page leaves but /Count is 0 or unreadable", leaves)
	}
	return fmt.Sprintf("page tree has %d page leaves but /Count says %d", leaves, pageCount)
}

// printPagesPlain renders the page index as an aligned table, one row per
// entry. NON-CONTRACTUAL; use --json to parse.
func printPagesPlain(out io.Writer, index []*pdfcore.PageIndexEntry) error {
	t := newTable("PAGE", "REF", "MEDIABOX", "ROTATE", "INHERITED", "ANNOTS", "CONTENTLEN", "ERROR")
	for _, e := range index {
		page, ref := "-", ""
		if e.PageNum > 0 {
			page = strconv.Itoa(e.PageNum)
		}
		if e.NodeID != "" {
			ref = fmt.Sprintf("%d %d R", e.ObjNum, e.Gen)
		}
		box := make([]string, len(e.MediaBox))
		for i, v := range e.MediaBox {
			box[i] = strconv.FormatFloat(v, 'f', -1, 64)
		}
		t.AddRow(
			page,
			dashIfEmpty(ref),
			strings.Join(box, " "),
			strconv.Itoa(e.Rotate),
			inheritedNames(e.Inherited),
			strconv.Itoa(e.AnnotCount),
			strconv.FormatInt(e.ContentLen, 10),
			dashIfEmpty(asciiSafe(e.Err)),
		)
	}
	return t.Render(out)
}

// inheritedNames renders an Inherited bitmask as a comma list of attribute
// names, or "-" when nothing was inherited.
func inheritedNames(mask uint8) string {
	var names []string
	for _, a := range []struct {
		bit  uint8
		name string
	}{
		{pdfcore.InheritedResources, "Resources"},
		{pdfcore.InheritedMediaBox, "MediaBox"},
		{pdfcore.InheritedCropBox, "CropBox"},
		{pdfcore.InheritedRotate, "Rotate"},
	} {
		if mask&a.bit != 0 {
			names = append(names, a.name)
		}
	}
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, ",")
}
