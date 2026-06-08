package main

import (
	"fmt"
	"io"
	"os"
)

// runSourceDump parses flags and dispatches object-source dump execution.
func runSourceDump(args []string) int {
	filePath, f, ok := parseByRefFlags("source", args, true, false)
	if !ok {
		return 1
	}
	return execSourceDump(filePath, f)
}

// execSourceDump opens the PDF and emits the reserialized object source. Default
// emits JSON {"objectRef","source"} (objectRef is the canonical "N G R" form
// built by the CLI; GetObjectSource returns only the string). With --raw it
// writes the verbatim source bytes to stdout (mirroring dump stream --raw). An
// unresolved ref yields the "N G obj\nnull\nendobj" envelope at exit 0; a
// non-obj nodeID yields "" (empty stdout under --raw) at exit 0.
func execSourceDump(filePath string, f byRefFlags) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins, nodeID, objectRef, code := openByRef(f.ref, filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	source, err := ins.GetObjectSource("cli", nodeID)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	if f.raw {
		if _, err := io.WriteString(os.Stdout, source); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write raw output: %v\n", err)
			return 2
		}
		return 0
	}

	envelope := struct {
		ObjectRef string `json:"objectRef"`
		Source    string `json:"source"`
	}{ObjectRef: objectRef, Source: source}
	if err := emit(os.Stdout, envelope, f.pretty); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}
