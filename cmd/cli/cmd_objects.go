package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runObjectsDump parses flags and dispatches the document-level object-index
// dump. NOTE: this is the PLURAL command (object index, no --ref); the SINGULAR
// `dump object` is a different command that inspects one object and requires --ref.
func runObjectsDump(args []string) int {
	filePath, f, ok := parseDocViewFlags("objects", args)
	if !ok {
		return 1
	}
	return execObjectsDump(filePath, f)
}

// execObjectsDump opens the PDF and emits the []ObjectIndexEntry array as JSON
// (one entry per object: num/gen/type/free/reachable/nodeId).
func execObjectsDump(filePath string, f docViewFlags) (exitCode int) {
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

	index, err := ins.GetObjectIndex("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	if f.json {
		if err := emit(os.Stdout, index, f.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if err := printObjectsPlain(os.Stdout, index); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printObjectsPlain renders the object index as an aligned table: one row per
// object (number, generation, type, free/reachable flags, node id).
// NON-CONTRACTUAL; use --json to parse.
func printObjectsPlain(out io.Writer, index []*pdfcore.ObjectIndexEntry) error {
	t := newTable("OBJ", "GEN", "TYPE", "FREE", "REACHABLE", "NODEID")
	for _, e := range index {
		t.AddRow(
			strconv.Itoa(e.ObjNum),
			strconv.Itoa(e.Gen),
			dashIfEmpty(e.TypeName),
			yesNo(e.Free),
			yesNo(e.Reachable),
			dashIfEmpty(e.NodeID),
		)
	}
	return t.Render(out)
}

// yesNo renders a bool as "yes"/"no" for table cells.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// dashIfEmpty renders "-" for an empty string so table columns stay legible.
func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
