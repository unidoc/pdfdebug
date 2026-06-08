package main

import (
	"fmt"
	"os"
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
	if err := emit(os.Stdout, refs, f.pretty); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}
