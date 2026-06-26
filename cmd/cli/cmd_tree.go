package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// treeNodeOutput extends TreeNode with a recursive children field for CLI output.
type treeNodeOutput struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	RawKey      string            `json:"rawKey,omitempty"`
	NodeType    string            `json:"nodeType"`
	ValueType   string            `json:"valueType,omitempty"`
	HasChildren bool              `json:"hasChildren"`
	ChildCount  int               `json:"childCount"`
	IconHint    string            `json:"iconHint,omitempty"`
	PdfRef      string            `json:"pdfRef,omitempty"`
	TypeName    string            `json:"typeName,omitempty"`
	Error       string            `json:"error,omitempty"`
	Children    []*treeNodeOutput `json:"children,omitempty"`
	// Resolved is the inline ref-following expansion produced by --resolve via
	// pdfcore.ResolveRef. Populated only for indirect-object nodes (id prefix
	// "obj:") when --resolve is set; nil (omitted) otherwise. Strictly additive:
	// without --resolve the field is absent, preserving today's bytes (AC6).
	Resolved *pdfcore.ResolvedNode `json:"resolved,omitempty"`
}

// runTreeDump executes the tree dump command and returns the exit code.
func runTreeDump(args []string) int {
	fs, flags, err := parseDumpFlags("dump tree", args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Usage: pdfdebug dump tree [--json] [--pretty] [--depth N] [--page N] <file>\n")
		return 1
	}

	// Reject negative depth; treat as user error rather than silently clamping.
	if flags.depth < 0 {
		writeJSONError(os.Stderr, "invalid --depth: must be >= 0")
		return 1
	}
	if flags.resolveDepth < 0 {
		writeJSONError(os.Stderr, "invalid --resolve-depth: must be >= 0")
		return 1
	}

	// CLI-side lower-bound check, mirroring dump stream (--page < 1 -> exit 1).
	// Only when --page was explicitly provided: an explicit --page 0 or negative
	// is a usage error; an absent --page roots at the catalog.
	if flags.pageSet && flags.page < 1 {
		writeJSONError(os.Stderr, "invalid --page: must be >= 1 (pages are 1-based)")
		return 1
	}

	filePath := fs.Arg(0)
	if filePath == "" {
		fmt.Fprintf(os.Stderr, "Usage: pdfdebug dump tree [--json] [--pretty] [--depth N] [--page N] <file>\n")
		return 1
	}

	pageNum := 0
	if flags.pageSet {
		pageNum = flags.page
	}
	return execTreeDump(filePath, flags.depth, pageNum, flags.json, flags.pretty, flags.resolve, flags.resolveDepth)
}

func execTreeDump(filePath string, maxDepth, pageNum int, jsonOut, pretty, resolve bool, resolveDepth int) (exitCode int) {
	// Defense in depth: catch any escaping panics from pdfcpu.
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

	var root *pdfcore.TreeNode
	var err error
	if pageNum > 0 {
		// Page-rooted walk: resolve page N to its populated page-dict node.
		// Out-of-range is reported by pdfcore -> runtime error, exit 2.
		root, err = inspector.GetPageNode("cli", pageNum)
	} else {
		root, err = inspector.GetTreeRoot("cli")
	}
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	visited := make(map[string]bool)
	out := buildTree(inspector, "cli", root, 0, maxDepth, visited, resolve, resolveDepth)
	if jsonOut {
		if err := emit(os.Stdout, out, pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if err := printTreePlain(os.Stdout, out); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printTreePlain renders the object tree as an indented outline: one node per
// line, two-space indents per level, the node label as the spine, with the
// "(N G R)" ref (when indirect) and the value/type as trailing metadata. It
// reads like a table of contents. NON-CONTRACTUAL: for a parseable view use
// --json.
func printTreePlain(out io.Writer, root *treeNodeOutput) error {
	var b strings.Builder
	writeTreeNode(&b, root, 0)
	_, err := io.WriteString(out, b.String())
	return err
}

// writeTreeNode appends one node line (and recurses into Children) to b at the
// given indent depth. The trailing metadata is the ref then the type/value
// classifier, each emitted only when present.
func writeTreeNode(b *strings.Builder, n *treeNodeOutput, depth int) {
	if n == nil {
		return
	}
	for range depth {
		b.WriteString("  ")
	}
	b.WriteString(n.Label)
	if n.PdfRef != "" {
		b.WriteString(" (")
		b.WriteString(n.PdfRef)
		b.WriteByte(')')
	}
	// Type metadata: prefer the resolved dict /Type, else the value type.
	if meta := treeNodeTypeMeta(n); meta != "" {
		b.WriteByte(' ')
		b.WriteString(meta)
	}
	if n.Error != "" {
		b.WriteString(" [error: ")
		b.WriteString(n.Error)
		b.WriteByte(']')
	}
	b.WriteByte('\n')
	for _, c := range n.Children {
		writeTreeNode(b, c, depth+1)
	}
}

// treeNodeTypeMeta returns the trailing type classifier for a tree row: the
// resolved /Type name when present, otherwise the value type (dict/array/...).
func treeNodeTypeMeta(n *treeNodeOutput) string {
	if n.TypeName != "" {
		return n.TypeName
	}
	return n.ValueType
}

// handleOpenError classifies an Open error and writes a JSON error to stderr.
func handleOpenError(err error) int {
	switch {
	case errors.Is(err, pdfcore.ErrEncryptedPDF):
		writeJSONError(os.Stderr, "encrypted PDF: password required")
	case errors.Is(err, pdfcore.ErrMalformedPDF):
		writeJSONError(os.Stderr, "malformed PDF: file is corrupt or invalid")
	case errors.Is(err, pdfcore.ErrDocumentNotFound):
		writeJSONError(os.Stderr, "file not found")
	default:
		writeJSONError(os.Stderr, err.Error())
	}
	return 2
}

// buildTree recursively builds the output tree from pdfcore TreeNodes.
// visited tracks obj:* node IDs to break circular reference loops.
func buildTree(ins *pdfcore.Inspector, tabID string, node *pdfcore.TreeNode, depth, maxDepth int, visited map[string]bool, resolve bool, resolveDepth int) *treeNodeOutput {
	out := convertNode(node)
	// --resolve: attach the inline ref-following expansion for indirect-object
	// nodes via the ResolveRef keystone. Additive; cycle-guarded inside ResolveRef.
	if resolve && strings.HasPrefix(node.ID, "obj:") {
		if rn, err := ins.ResolveRef(tabID, node.ID, pdfcore.ResolveOpts{MaxDepth: resolveDepth}); err == nil {
			out.Resolved = rn
		} else {
			// Warn rather than fail the walk; absence of the field would
			// otherwise be indistinguishable from "node had no refs".
			fmt.Fprintf(os.Stderr, "warning: --resolve failed for %s: %v\n", node.ID, err)
		}
	}
	if !node.HasChildren {
		return out
	}
	if maxDepth > 0 && depth >= maxDepth {
		return out
	}
	// Guard against circular references in the PDF object graph.
	if strings.HasPrefix(node.ID, "obj:") {
		if visited[node.ID] {
			return out
		}
		visited[node.ID] = true
	}
	children, err := ins.GetChildren(tabID, node.ID)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Children = make([]*treeNodeOutput, 0, len(children))
	for _, child := range children {
		if child == nil {
			continue
		}
		out.Children = append(out.Children, buildTree(ins, tabID, child, depth+1, maxDepth, visited, resolve, resolveDepth))
	}
	return out
}

// convertNode maps a pdfcore.TreeNode to the CLI output struct. pdfRef is
// surfaced only for indirect-object nodes (id prefix "obj:"); the synthetic
// catalog "root" node carries an ObjectRef internally but is not addressable as
// "N G R", so AC1 requires it to omit pdfRef.
func convertNode(n *pdfcore.TreeNode) *treeNodeOutput {
	pdfRef := ""
	if strings.HasPrefix(n.ID, "obj:") {
		pdfRef = n.ObjectRef
	}
	return &treeNodeOutput{
		ID:          n.ID,
		Label:       n.Label,
		RawKey:      n.RawKey,
		NodeType:    n.NodeType,
		ValueType:   n.ValueType,
		HasChildren: n.HasChildren,
		ChildCount:  n.ChildCount,
		IconHint:    n.IconHint,
		PdfRef:      pdfRef,
		TypeName:    n.TypeName,
		Error:       n.Error,
	}
}
