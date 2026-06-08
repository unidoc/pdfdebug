package main

import (
	"fmt"
	"os"
)

// runObjectsDump parses flags and dispatches the document-level object-index
// dump. NOTE: this is the PLURAL command (object index, no --ref); the SINGULAR
// `dump object` is a different command that inspects one object and requires --ref.
func runObjectsDump(args []string) int {
	filePath, f, ok := parseDocViewFlags("objects", args, false)
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
	if err := emit(os.Stdout, index, f.pretty); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}
