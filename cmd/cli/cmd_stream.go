package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runStreamDump parses flags and dispatches stream dump execution.
func runStreamDump(args []string) int {
	fs := flag.NewFlagSet("dump stream", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	pageFlag := fs.Int("page", 0, "Page number (1-based)")
	_ = fs.Bool("json", false, "Output as JSON (default, always on)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "Usage: pdfdebug dump stream [--json] --page N <file>")
		return 1
	}

	if *pageFlag < 1 {
		writeJSONError(os.Stderr, "invalid --page: must be >= 1 (pages are 1-based)")
		return 1
	}

	filePath := fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, "Usage: pdfdebug dump stream [--json] --page N <file>")
		return 1
	}

	return execStreamDump(filePath, *pageFlag)
}

// execStreamDump opens the PDF, resolves the page's content stream, and writes
// the decoded ContentStreamData as JSON to stdout.
func execStreamDump(filePath string, pageNum int) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	inspector := pdfcore.NewInspector()

	info, err := inspector.Open("cli", filePath)
	if err != nil {
		return handleOpenError(err)
	}
	defer func() { _ = inspector.Close("cli") }()

	if info.Error != "" {
		writeJSONWarning(os.Stderr, info.Error)
	}

	if info.PageCount == 0 {
		writeJSONError(os.Stderr, "cannot determine page count for this PDF")
		return 2
	}
	if pageNum > info.PageCount {
		writeJSONError(os.Stderr, fmt.Sprintf("page %d out of range: document has %d pages", pageNum, info.PageCount))
		return 2
	}

	nodeID, err := inspector.GetPageContentStreamNodeID("cli", pageNum)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	// No Contents entry: valid PDF, just no stream on this page.
	if nodeID == "" {
		result := &pdfcore.ContentStreamData{
			Raw:   "",
			Error: "page has no content stream",
		}
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}

	result, err := inspector.GetContentStream("cli", nodeID)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}
