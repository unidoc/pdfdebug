// CLI ergonomics & discoverability -- acceptance tests.
//
// Black-box: build the CLI, run it as a subprocess, parse stdout/stderr.
//
// Covers in this suite: pdfRef + typeName on tree nodes; --help worked
// examples; --pretty for dump tree; dump tree --page N.
//
// Run: cd tests/cli-tree-dump && go test -v -count=1 ./...
package cli_tree_dump_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// treeNodeView is a minimal view of the tree JSON: id, pdfRef, typeName and
// children.
type treeNodeView struct {
	ID       string         `json:"id"`
	PdfRef   string         `json:"pdfRef"`
	TypeName string         `json:"typeName"`
	Children []treeNodeView `json:"children"`
}

// collectIndirect returns all nodes whose id is of the form obj:G:N.
func collectIndirect(n treeNodeView, out *[]treeNodeView) {
	if strings.HasPrefix(n.ID, "obj:") {
		*out = append(*out, n)
	}
	for _, c := range n.Children {
		collectIndirect(c, out)
	}
}

// ---------------------------------------------------------------------------
// Every indirect (obj:G:N) tree node carries a `pdfRef` equal to its
// canonical "N G R" string; nodes with a /Type carry a `typeName`. The root
// catalog (id "root", non-indirect) omits `pdfRef`. The `pdfRef` value,
// pasted into `dump object --ref`, resolves the same object.
// ---------------------------------------------------------------------------

func TestTreeDump_NodesCarryPdfRefAndTypeName(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "multipage.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	var root treeNodeView
	mustParseJSON(t, stdout, &root)

	// Root catalog node is not an indirect obj: node -> must omit pdfRef.
	if root.PdfRef != "" {
		t.Errorf("root node (id=%q) should omit pdfRef, got %q", root.ID, root.PdfRef)
	}

	var indirect []treeNodeView
	collectIndirect(root, &indirect)
	if len(indirect) == 0 {
		t.Fatal("no indirect (obj:) nodes found in multipage.pdf tree")
	}

	foundTypeName := false
	for _, n := range indirect {
		// Every indirect node must carry pdfRef.
		if n.PdfRef == "" {
			t.Errorf("indirect node %q missing pdfRef", n.ID)
			continue
		}
		// pdfRef must equal the canonical "N G R" derived from obj:G:N.
		parts := strings.SplitN(n.ID, ":", 3)
		if len(parts) == 3 {
			wantRef := parts[2] + " " + parts[1] + " R"
			if n.PdfRef != wantRef {
				t.Errorf("node %q pdfRef=%q, want %q", n.ID, n.PdfRef, wantRef)
			}
		}
		if n.TypeName != "" {
			foundTypeName = true
		}
	}

	// multipage.pdf has /Type-bearing dicts (Catalog/Pages/Page), so at least
	// one indirect node must surface a typeName.
	if !foundTypeName {
		t.Error("expected at least one indirect node to carry a typeName")
	}

	// Round-trip: feed the first indirect node's pdfRef into dump object.
	ref := indirect[0].PdfRef
	objOut, _, objExit := runCLI(t, bin, "dump", "object", "--json", "--ref", ref, pdfPath)
	if objExit != 0 {
		t.Fatalf("pdfRef %q did not resolve via dump object (exit %d)", ref, objExit)
	}
	var detail map[string]any
	mustParseJSON(t, objOut, &detail)
	if got, _ := detail["objectRef"].(string); got != ref {
		t.Errorf("round-trip objectRef=%q, want %q", got, ref)
	}
}

// ---------------------------------------------------------------------------
// `dump tree --pretty` emits indented multi-line JSON; default (no flag)
// stays compact single-line. Both parse to the same logical tree.
// ---------------------------------------------------------------------------

func TestTreeDump_PrettyVsCompact(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	compact, _, ec := runCLI(t, bin, "dump", "tree", "--json", pdfPath)
	if ec != 0 {
		t.Fatalf("compact run exit %d", ec)
	}
	pretty, _, ep := runCLI(t, bin, "dump", "tree", "--json", "--pretty", pdfPath)
	if ep != 0 {
		t.Fatalf("--pretty run exit %d", ep)
	}

	// Compact output must be single-line (the trailing newline from Encode is
	// the only allowed newline).
	if strings.Count(strings.TrimRight(compact, "\n"), "\n") != 0 {
		t.Errorf("default tree output is not single-line compact:\n%s", compact)
	}
	// Pretty output must be multi-line and contain indentation.
	if !strings.Contains(pretty, "\n  ") {
		t.Errorf("--pretty tree output is not indented multi-line:\n%s", pretty)
	}

	// Same logical content regardless of formatting.
	var a, b any
	mustParseJSON(t, compact, &a)
	mustParseJSON(t, pretty, &b)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Error("--pretty and compact tree decode to different content")
	}
}

// ---------------------------------------------------------------------------
// `dump tree --page N` roots the tree at page N's page dict (/Type /Page),
// not the catalog, and the root node carries the page object's pdfRef and
// typeName (populated from the real TreeNode, not a bare stub).
// ---------------------------------------------------------------------------

func TestTreeDump_PageFlag_RootsAtPageDict(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "multipage.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", "--page", "1", pdfPath)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0 for --page 1, got %d", exitCode)
	}

	var root treeNodeView
	mustParseJSON(t, stdout, &root)

	// Rooted at a page dict -> root is an indirect object node, not the catalog.
	if !strings.HasPrefix(root.ID, "obj:") {
		t.Errorf("--page root id=%q, expected an obj: page-dict node (not catalog)", root.ID)
	}
	// Root must carry a real pdfRef (populated node, not a synthesized stub).
	if root.PdfRef == "" {
		t.Error("--page root node missing pdfRef (must be populated from the real TreeNode)")
	}
	// Root must be the page object: typeName /Page.
	if root.TypeName != "/Page" && root.TypeName != "Page" {
		t.Errorf("--page root typeName=%q, expected the page dict's /Page type", root.TypeName)
	}
}

// ---------------------------------------------------------------------------
// --page boundary exit codes.
//   --page 0 (and < 1)         -> JSON usage error, exit 1.
//   --page 99999 (out of range) -> JSON runtime error, exit 2.
// ---------------------------------------------------------------------------

func TestTreeDump_PageFlag_InvalidExitCodes(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "multipage.pdf")

	cases := []struct {
		name     string
		page     string
		wantExit int
	}{
		{"page zero -> usage error", "0", 1},
		{"page negative -> usage error", "-1", 1},
		{"page out of range -> runtime error", "99999", 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree", "--page", tc.page, pdfPath)

			if exitCode != tc.wantExit {
				t.Errorf("%s: expected exit code %d, got %d", tc.name, tc.wantExit, exitCode)
			}
			if strings.TrimSpace(stdout) != "" {
				t.Errorf("%s: stdout should be empty on error, got: %s", tc.name, stdout)
			}
			var errObj map[string]string
			if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
				t.Fatalf("%s: stderr is not valid JSON: %v\nraw: %s", tc.name, err, stderr)
			}
			if _, ok := errObj["error"]; !ok {
				t.Errorf("%s: stderr JSON missing 'error' key", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// `--help` includes one copy-pasteable example invocation for each of dump
// tree, dump object, and dump stream. The object example must show the --ref
// flag shape; the stream example must show --page. The existing
// "dump"/"tree" substring assertions are unaffected.
// ---------------------------------------------------------------------------

func TestHelp_PerSubcommandExamples(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0 for --help, got %d", exitCode)
	}

	combined := stdout + stderr

	// Requires an explicit "Examples:" block: one copy-pasteable,
	// syntactically-valid invocation per subcommand using a placeholder
	// filename. The pre-existing terse usage grammar (e.g. `dump object
	// [--json] --ref "N G R" <file>`) does NOT satisfy this -- it has no
	// example block and no concrete placeholder filename.
	lower := strings.ToLower(combined)
	if !strings.Contains(lower, "example") {
		t.Errorf("--help missing an Examples block\noutput:\n%s", combined)
	}

	// An example line must combine a concrete subcommand invocation with a
	// placeholder PDF filename. The existing grammar lines use the `<file>`
	// metavar, not a real .pdf filename, so requiring ".pdf" forces a true
	// example line rather than the grammar.
	checks := []struct {
		name     string
		subcmd   string
	}{
		{"dump tree", "dump tree"},
		{"dump object with --ref", "dump object --ref"},
		{"dump stream with --page", "dump stream --page"},
	}
	for _, c := range checks {
		if !exampleLinePresent(combined, c.subcmd) {
			t.Errorf("--help missing a copy-pasteable %q example line ending in a .pdf placeholder\noutput:\n%s", c.subcmd, combined)
		}
	}
}

// exampleLinePresent reports whether any single line contains the subcommand
// invocation fragment AND a ".pdf" placeholder filename (an actual example,
// not the terse usage grammar which uses the <file> metavar).
func exampleLinePresent(out, subcmd string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, subcmd) && strings.Contains(line, ".pdf") {
			return true
		}
	}
	return false
}
