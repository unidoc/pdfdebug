package main

import (
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
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output")
	resolveFlag := fs.Bool("resolve", false, "Follow indirect refs inline via ResolveRef (adds a 'resolved' field)")
	resolveDepthFlag := fs.Int("resolve-depth", defaultResolveDepth, "Ref-following depth for --resolve")
	_ = fs.Bool("json", false, "Output as JSON (default, always on)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, `Usage: pdfdebug dump object [--json] [--resolve [--resolve-depth N]] --ref "N G R" <file>`)
		return 1
	}

	if *refFlag == "" {
		fmt.Fprintln(os.Stderr, `Usage: pdfdebug dump object [--json] [--resolve [--resolve-depth N]] --ref "N G R" <file>`)
		return 1
	}
	if *resolveDepthFlag < 0 {
		writeJSONError(os.Stderr, "invalid --resolve-depth: must be >= 0")
		return 1
	}

	objNum, genNum, err := parseObjectRef(*refFlag)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 1
	}

	filePath := fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, `Usage: pdfdebug dump object [--json] [--resolve [--resolve-depth N]] --ref "N G R" <file>`)
		return 1
	}

	return execObjectDump(filePath, objNum, genNum, *prettyFlag, *resolveFlag, *resolveDepthFlag)
}

// objectDumpOutput wraps an ObjectDetail with the optional --resolve inline
// expansion. Without --resolve the Resolved field is absent (omitempty), so the
// default output is byte-for-byte the bare ObjectDetail (AC6 no-regression).
type objectDumpOutput struct {
	*pdfcore.ObjectDetail
	Resolved *pdfcore.ResolvedNode `json:"resolved,omitempty"`
}

// refFormatHint is the canonical error/usage text naming both accepted --ref
// forms and pointing at dump tree's pdfRef field. Kept coherent regardless of
// which form the user attempted (AC2/AC4).
const refFormatHint = `invalid reference format: expected "N G R" (e.g., "5 0 R"); ` +
	`the obj:G:N id form (e.g. "obj:0:5") is also accepted; ` +
	`tip: dump tree emits a ready-to-paste pdfRef per node`

// parseObjectRef parses a PDF indirect reference string "N G R" into object
// and generation numbers. Returns a descriptive error for malformed input.
func parseObjectRef(ref string) (objNum int, genNum int, err error) {
	// Postel's law: accept the obj:G:N id form that dump tree emits directly,
	// mirroring pdfcore's parseNodeID obj-case (first part is gen, second num).
	if strings.HasPrefix(strings.TrimSpace(ref), "obj:") {
		return parseObjIDRef(strings.TrimSpace(ref))
	}

	parts := strings.Fields(ref)
	if len(parts) != 3 || parts[2] != "R" {
		return 0, 0, fmt.Errorf("%s", refFormatHint)
	}
	objNum, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("%s", refFormatHint)
	}
	genNum, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("%s", refFormatHint)
	}
	if objNum < 0 || genNum < 0 {
		return 0, 0, fmt.Errorf("%s", refFormatHint)
	}
	return objNum, genNum, nil
}

// parseObjIDRef parses the "obj:G:N" tree-node id form into object and
// generation numbers (gen first, num second), rejecting malformed input with
// the both-forms format hint.
func parseObjIDRef(ref string) (objNum int, genNum int, err error) {
	parts := strings.Split(ref, ":")
	if len(parts) != 3 || parts[0] != "obj" {
		return 0, 0, fmt.Errorf("%s", refFormatHint)
	}
	genNum, err = strconv.Atoi(parts[1])
	if err != nil || genNum < 0 {
		return 0, 0, fmt.Errorf("%s", refFormatHint)
	}
	objNum, err = strconv.Atoi(parts[2])
	if err != nil || objNum < 0 {
		return 0, 0, fmt.Errorf("%s", refFormatHint)
	}
	return objNum, genNum, nil
}

// execObjectDump opens the PDF, queries the object, and writes JSON to stdout.
func execObjectDump(filePath string, objNum, genNum int, pretty, resolve bool, resolveDepth int) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	inspector, _, code := openForCLI(filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = inspector.Close("cli") }()

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
		msg := fmt.Sprintf("object not found: %d %d R", objNum, genNum)
		// Reversal heuristic: generation numbers are almost always 0, so a zero
		// object number with a nonzero generation is the common operand-swap
		// signature (e.g. user typed "0 25 R" for "25 0 R"). Object 0 gen 0 (the
		// free-list head) stays a plain not-found.
		if objNum == 0 && genNum > 0 {
			msg += fmt.Sprintf(` (did you mean: dump object --ref "%d %d R")`, genNum, objNum)
		}
		writeJSONError(os.Stderr, msg)
		return 2
	}

	out := objectDumpOutput{ObjectDetail: detail}
	if resolve {
		// --resolve: inline the object's ref graph via the ResolveRef keystone
		// (cycle-guarded). Additive: a 'resolved' field alongside the existing
		// ObjectDetail. A resolve failure leaves the field absent rather than
		// failing the whole command, but warns on stderr so the absence is not
		// silently indistinguishable from "object had no refs".
		if rn, rerr := inspector.ResolveRef("cli", nodeID, pdfcore.ResolveOpts{MaxDepth: resolveDepth}); rerr == nil {
			out.Resolved = rn
		} else {
			fmt.Fprintf(os.Stderr, "warning: --resolve failed for %s: %v\n", nodeID, rerr)
		}
	}

	if err := emit(os.Stdout, out, pretty); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}
