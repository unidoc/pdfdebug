package main

import (
	"fmt"
	"os"
)

// runFontDump parses flags and dispatches font-view dump execution.
func runFontDump(args []string) int {
	filePath, f, ok := parseByRefFlags("font", args, false, false)
	if !ok {
		return 1
	}
	return execFontDump(filePath, f)
}

// execFontDump opens the PDF and emits the FontView payload as JSON. A non-font
// node yields {"kind":"neither",...} at exit 0 (the method does not error on
// type mismatch -- thin-presenter rule).
func execFontDump(filePath string, f byRefFlags) (exitCode int) {
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

	view, err := ins.GetFontView("cli", nodeID)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	if err := emit(os.Stdout, view, f.pretty); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}
