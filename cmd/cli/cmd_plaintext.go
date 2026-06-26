package main

import (
	"fmt"
	"io"
	"os"
)

// runPlaintextDump parses flags and dispatches the document-level plain-text
// dump. Default writes the document text raw to stdout; --json wraps it.
func runPlaintextDump(args []string) int {
	filePath, f, ok := parseDocViewFlags("plaintext", args)
	if !ok {
		return 1
	}
	return execPlaintextDump(filePath, f)
}

// execPlaintextDump opens the PDF and emits the document text.
//
// The --json path wraps the GetPlainText payload as exactly
// {"totalBytes","content"} -- the CLI-internal tabId field is deliberately
// excluded. content is the Latin-1-decoded view (lossy for control bytes per
// pdfcore.latin1Decode), matching what the GUI Plain Text view shows.
//
// The default (raw) path writes the verbatim source bytes to stdout. We read
// the file bytes directly rather than re-encoding GetPlainText.Content, because
// latin1Decode replaces control bytes (< 0x20, plus 0x7F) with U+FFFD -- that
// substitution is lossy and cannot be reversed, so re-encoding the decoded
// string would corrupt every control byte. Reading the file directly keeps the
// raw dump truly byte-for-byte without adding any PDF semantics (thin presenter).
func execPlaintextDump(filePath string, f docViewFlags) (exitCode int) {
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

	if f.json {
		// Only the --json path needs the decoded payload. GetPlainText performs a
		// full file read + Latin-1 decode + string allocation, so calling it on
		// the raw path (which re-streams the file from disk below) would do that
		// expensive work and then discard it -- defeating the raw-by-default
		// "keep the common case cheap" contract. Gate it behind --json.
		doc, err := ins.GetPlainText("cli")
		if err != nil {
			writeJSONError(os.Stderr, err.Error())
			return 2
		}
		wrapper := struct {
			TotalBytes int64  `json:"totalBytes"`
			Content    string `json:"content"`
		}{TotalBytes: doc.TotalBytes, Content: doc.Content}
		if err := emit(os.Stdout, wrapper, f.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}

	// Verbatim source bytes: stream the file directly (the decoded Content is
	// lossy for control bytes and cannot reproduce the source). io.Copy streams
	// in fixed-size chunks rather than allocating the whole file like os.ReadFile.
	src, err := os.Open(filePath)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	defer func() { _ = src.Close() }()
	if _, err := io.Copy(os.Stdout, src); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
		return 2
	}
	return 0
}
