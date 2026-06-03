package main

import (
	"errors"
	"fmt"
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
	return execTreeDump(filePath, flags.depth, pageNum, flags.pretty)
}

func execTreeDump(filePath string, maxDepth, pageNum int, pretty bool) (exitCode int) {
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
	out := buildTree(inspector, "cli", root, 0, maxDepth, visited)
	if err := emit(os.Stdout, out, pretty); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
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
func buildTree(ins *pdfcore.Inspector, tabID string, node *pdfcore.TreeNode, depth, maxDepth int, visited map[string]bool) *treeNodeOutput {
	out := convertNode(node)
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
		out.Children = append(out.Children, buildTree(ins, tabID, child, depth+1, maxDepth, visited))
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
