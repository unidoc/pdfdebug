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
	rawFlag := fs.Bool("raw", false, "Emit verbatim decoded bytes instead of JSON")
	_ = fs.Bool("json", false, "Output as JSON (default, mutually exclusive with --raw)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "Usage: pdfdebug dump stream [--json|--raw] --page N <file>")
		return 1
	}

	if *pageFlag < 1 {
		writeJSONError(os.Stderr, "invalid --page: must be >= 1 (pages are 1-based)")
		return 1
	}

	filePath := fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, "Usage: pdfdebug dump stream [--json|--raw] --page N <file>")
		return 1
	}

	return execStreamDump(filePath, *pageFlag, *rawFlag)
}

// execStreamDump opens the PDF, resolves the page's content stream, and
// writes either the decoded ContentStreamData as JSON (default) or the
// verbatim decoded bytes (when raw is true) to stdout.
func execStreamDump(filePath string, pageNum int, raw bool) (exitCode int) {
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

	if raw {
		// Verbatim decoded bytes; no JSON wrapper. ContentStreamData.Error is
		// non-fatal (decompression failures surface here while raw is empty),
		// so prefer to surface that to stderr and exit non-zero rather than
		// silently writing an empty stdout.
		if result.Error != "" {
			fmt.Fprintln(os.Stderr, result.Error)
			return 2
		}
		if _, err := io.WriteString(os.Stdout, result.Raw); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write raw output: %v\n", err)
			return 2
		}
		return 0
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}
