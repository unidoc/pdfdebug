package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// diffUsage is the one-line usage string for the diff command.
const diffUsage = "Usage: pdfdebug diff [--json] [--pretty] [--full] <left.pdf> <right.pdf>"

// runDiff handles the top-level `diff` command: a path-aligned structural diff
// of two PDFs. It is the first command to take TWO positional args (both
// required; a third is rejected). It uses a three-way exit contract distinct
// from the `dump` commands:
//
//	0  ran successfully, the two documents are structurally IDENTICAL
//	1  ran successfully AND the documents DIFFER (the scriptable signal)
//	2  operational error (missing/unreadable file, bad args, parse failure)
//
// The two non-zero codes are distinct so scripts can tell "differ" from
// "broken file".
func runDiff(args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is human-readable plain text)")
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output (no effect on plain text)")
	fullFlag := fs.Bool("full", false, "Include unchanged paths in the plain-text delta")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, diffUsage)
		return 2
	}

	// Exactly two positional args. Guard empty, reject a single arg or a
	// third arg - a usage error is operational (exit 2), never a partial run.
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, diffUsage)
		return 2
	}
	return execDiff(fs.Arg(0), fs.Arg(1), *jsonFlag, *prettyFlag, *fullFlag)
}

// execDiff opens both files into ONE inspector (two synthetic tabs), computes
// the path-aligned delta, renders it, and returns the exit code. An open
// failure on either file is operational (exit 2) via handleOpenError; a
// successful run exits 0 when the documents are identical, 1 when they differ.
func execDiff(leftPath, rightPath string, jsonOut, pretty, full bool) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins := pdfcore.NewInspector()
	if _, err := ins.Open("left", leftPath); err != nil {
		return handleOpenError(err) // operational: exit 2
	}
	defer func() { _ = ins.Close("left") }()
	if _, err := ins.Open("right", rightPath); err != nil {
		return handleOpenError(err) // operational: exit 2
	}
	defer func() { _ = ins.Close("right") }()

	res, err := ins.DiffDocuments("left", "right")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	if jsonOut {
		if err := emit(os.Stdout, res, pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
	} else if err := printDiffPlain(os.Stdout, res, full); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}

	if diffIsIdentical(res.Summary) {
		return 0 // identical
	}
	return 1 // differ
}

// diffIsIdentical reports whether the two documents are structurally identical.
// It considers the document-level flags (encryption, /Info) IN ADDITION to the
// added/removed/changed node counts: encryption (trailer /Encrypt) and /Info
// (trailer /Info) live OFF the catalog walk, so a difference there never bumps
// the counts and must be checked explicitly or a /Producer-only or
// encryption-only change would wrongly report "identical" (exit 0).
func diffIsIdentical(s pdfcore.DiffSummary) bool {
	// A depth-capped walk (TruncatedSubtrees > 0) left a subtree unexplored, so
	// the pair cannot be certified identical even with zero visible deltas
	// Story 14.3: exit 1 ("not provably identical") is the honest signal.
	return s.Added == 0 && s.Removed == 0 && s.Changed == 0 &&
		!s.VersionChanged && !s.EncryptionChanged && !s.InfoChanged && !s.XMPChanged &&
		s.TruncatedSubtrees == 0
}

// printDiffPlain renders the summary plus a path-indented delta. NON-CONTRACTUAL
// (use --json to parse). By default only changed/added/removed paths appear,
// marked +/-/~; --full also prints unchanged paths.
func printDiffPlain(out io.Writer, res *pdfcore.DiffResult, full bool) error {
	var b strings.Builder
	s := res.Summary
	fmt.Fprintf(&b, "Structural diff: %d added, %d removed, %d changed\n", s.Added, s.Removed, s.Changed)
	fmt.Fprintf(&b, "Page count: %d -> %d\n", s.PageCountLeft, s.PageCountRight)
	if s.VersionChanged {
		b.WriteString("Catalog /Version changed\n")
	}
	if s.EncryptionChanged {
		b.WriteString("Encryption changed\n")
	}
	if s.InfoChanged {
		b.WriteString("/Info metadata changed\n")
	}
	if s.XMPChanged {
		b.WriteString("XMP metadata changed\n")
	}
	if s.TruncatedSubtrees > 0 {
		// Depth-cap honesty (Story 14.3): state plainly that the walk was
		// bounded so no consumer mistakes it for a complete comparison.
		fmt.Fprintf(&b, "%d subtree(s) compared only to the depth cap (truncated); deeper differences cannot be ruled out\n", s.TruncatedSubtrees)
	}
	b.WriteString("\n")

	if diffIsIdentical(s) {
		b.WriteString("Documents are structurally identical.\n")
	} else {
		writeDiffLines(&b, res.Root, 0, full)
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// writeDiffLines writes one indented line per node, recursing into children.
// Without --full a node is skipped when neither it nor any descendant carries a
// delta, so the output focuses on the change.
func writeDiffLines(b *strings.Builder, n *pdfcore.DiffNode, depth int, full bool) {
	if n == nil {
		return
	}
	if !full && !diffNodeHasDelta(n) {
		return
	}
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(b, "%s%s %s", indent, diffMarker(n.Status), n.Path)
	switch n.Status {
	case "changed":
		if len(n.Children) == 0 && (n.LeftSummary != "" || n.RightSummary != "") {
			fmt.Fprintf(b, "  %s -> %s", n.LeftSummary, n.RightSummary)
		} else if len(n.ChangedKeys) > 0 {
			fmt.Fprintf(b, "  keys: %s", strings.Join(n.ChangedKeys, ", "))
		}
	case "added":
		if n.RightSummary != "" {
			fmt.Fprintf(b, "  %s", n.RightSummary)
		}
	case "removed":
		if n.LeftSummary != "" {
			fmt.Fprintf(b, "  %s", n.LeftSummary)
		}
	}
	if n.Truncated {
		// The depth-cap tag (Story 14.3): a truncated node carries
		// Status "unchanged", so without this it would print without any
		// indication its subtree was left unwalked.
		b.WriteString("  [truncated: depth cap]")
	}
	b.WriteString("\n")
	for _, c := range n.Children {
		writeDiffLines(b, c, depth+1, full)
	}
}

// diffNodeHasDelta reports whether n or any descendant is not "unchanged", or
// is depth-cap truncated. A truncated node reports Status "unchanged" but must
// surface in the default (non-full) delta so its [truncated: depth cap] tag is
// visible (Story 14.3).
func diffNodeHasDelta(n *pdfcore.DiffNode) bool {
	if n.Status != "unchanged" || n.Truncated {
		return true
	}
	for _, c := range n.Children {
		if diffNodeHasDelta(c) {
			return true
		}
	}
	return false
}

// diffMarker maps a status to its plain-text delta marker.
func diffMarker(status string) string {
	switch status {
	case "added":
		return "+"
	case "removed":
		return "-"
	case "changed":
		return "~"
	default:
		return " "
	}
}
