package main

import (
	"fmt"
	"io"
	"os"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runReverseRefsDump parses flags and dispatches reverse-reference dump.
func runReverseRefsDump(args []string) int {
	filePath, f, ok := parseByRefFlags("reverserefs", args, false, false)
	if !ok {
		return 1
	}
	return execReverseRefsDump(filePath, f)
}

// execReverseRefsDump opens the PDF and emits the []ReverseRef payload as a JSON
// array ("who points at this object?"). An unreferenced or nonexistent object
// yields [] at exit 0; only a genuine index-build failure
// (ErrReverseRefIndexUnavailable) exits 2.
func execReverseRefsDump(filePath string, f byRefFlags) (exitCode int) {
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

	refs, err := ins.GetReverseRefs("cli", nodeID)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	if f.json {
		if err := emit(os.Stdout, refs, f.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if err := printReverseRefsPlain(os.Stdout, refs); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printReverseRefsPlain renders the inbound reference edges as an aligned
// table: one row per edge (the parent ref, its /Type, and the path inside the
// parent where the reference lives). An object with no inbound edges renders
// the header row with no data rows. NON-CONTRACTUAL; use --json to parse.
func printReverseRefsPlain(out io.Writer, refs []pdfcore.ReverseRef) error {
	t := newTable("PARENT", "TYPE", "PATH")
	for _, r := range refs {
		parentType := "-"
		if r.ParentType != nil {
			parentType = dashIfEmpty(*r.ParentType)
		}
		t.AddRow(r.ParentRef, parentType, r.Path)
	}
	return t.Render(out)
}
