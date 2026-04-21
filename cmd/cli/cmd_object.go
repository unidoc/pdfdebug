package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runObjectDump executes the object dump command and returns the exit code.
func runObjectDump(args []string) int {
	fs := flag.NewFlagSet("dump object", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	refFlag := fs.String("ref", "", `Object reference in "N G R" format (e.g., "5 0 R")`)
	_ = fs.Bool("json", false, "Output as JSON (default, always on)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, `Usage: pdfdebug dump object [--json] --ref "N G R" <file>`)
		return 1
	}

	if *refFlag == "" {
		fmt.Fprintln(os.Stderr, `Usage: pdfdebug dump object [--json] --ref "N G R" <file>`)
		return 1
	}

	objNum, genNum, err := parseObjectRef(*refFlag)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 1
	}

	filePath := fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, `Usage: pdfdebug dump object [--json] --ref "N G R" <file>`)
		return 1
	}

	return execObjectDump(filePath, objNum, genNum)
}

// parseObjectRef parses a PDF indirect reference string "N G R" into object
// and generation numbers. Returns a descriptive error for malformed input.
func parseObjectRef(ref string) (objNum int, genNum int, err error) {
	parts := strings.Fields(ref)
	if len(parts) != 3 {
		return 0, 0, fmt.Errorf(`invalid reference format: expected "N G R" (e.g., "5 0 R")`)
	}
	if parts[2] != "R" {
		return 0, 0, fmt.Errorf(`invalid reference format: expected "N G R" (e.g., "5 0 R")`)
	}
	objNum, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf(`invalid reference format: expected "N G R" (e.g., "5 0 R")`)
	}
	genNum, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf(`invalid reference format: expected "N G R" (e.g., "5 0 R")`)
	}
	if objNum < 0 || genNum < 0 {
		return 0, 0, fmt.Errorf(`invalid reference format: expected "N G R" (e.g., "5 0 R")`)
	}
	return objNum, genNum, nil
}

// execObjectDump opens the PDF, queries the object, and writes JSON to stdout.
func execObjectDump(filePath string, objNum, genNum int) (exitCode int) {
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

	nodeID := fmt.Sprintf("obj:%d:%d", genNum, objNum)

	detail, err := inspector.GetObjectDetail("cli", nodeID)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	if detail == nil {
		writeJSONError(os.Stderr, "internal error: no object detail")
		return 2
	}

	// pdfcpu returns null for undefined objects (PDF spec 7.3.10).
	// Detect and convert to error for non-existent refs.
	if detail.Type == "scalar" && detail.ScalarValue != nil && detail.ScalarValue.Type == "null" {
		writeJSONError(os.Stderr, fmt.Sprintf("object not found: %d %d R", objNum, genNum))
		return 2
	}

	if err := json.NewEncoder(os.Stdout).Encode(detail); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}
