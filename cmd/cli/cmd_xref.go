package main

import (
	"fmt"
	"os"
)

// runXRefDump parses flags and dispatches the document-level xref-table dump.
func runXRefDump(args []string) int {
	filePath, f, ok := parseDocViewFlags("xref", args, false)
	if !ok {
		return 1
	}
	return execXRefDump(filePath, f)
}

// execXRefDump opens the PDF and emits the XRefTable payload as JSON.
func execXRefDump(filePath string, f docViewFlags) (exitCode int) {
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

	table, err := ins.GetXRefTable("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	if err := emit(os.Stdout, table, f.pretty); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}
