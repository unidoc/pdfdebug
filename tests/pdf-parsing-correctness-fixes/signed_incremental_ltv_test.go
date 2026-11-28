package pdf_parsing_correctness_fixes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testdata/signed-incremental-ltv.pdf is a UniPDF-signed file: a %PDF-1.1 base
// revision, five incremental revisions that each add and sign one
// image-appearance signature field (Signature1 to Signature5; only
// Signature1's image has an /SMask), and a last revision that adds a catalog
// /DSS with /Certs and /VRI. Every CLI command opens it.

const signedLTVRel = "testdata/signed-incremental-ltv.pdf"

// signedLTVPath returns the fixture's absolute path, failing the test when it
// is missing.
func signedLTVPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(projectRoot(t), filepath.FromSlash(signedLTVRel))
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s is required: %v", signedLTVRel, err)
	}
	return p
}

// The fixture is a %PDF-1.1 file with at least seven revisions, under 200 KB,
// and no go.mod in the repository requires unipdf.
func TestSignedIncrementalLTV_FixtureOnDisk(t *testing.T) {
	b, err := os.ReadFile(signedLTVPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-1.1")) {
		t.Errorf("header = %q, want %%PDF-1.1", b[:min(len(b), 8)])
	}
	if n := bytes.Count(b, []byte("%%EOF")); n < 7 {
		t.Errorf("found %d end-of-file markers, want at least 7 (base, five signing revisions, LTV revision)", n)
	}
	if len(b) >= 200*1024 {
		t.Errorf("fixture is %d bytes, want under 200 KB", len(b))
	}

	root := projectRoot(t)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if d.IsDir() || d.Name() != "go.mod" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "github.com/unidoc/unipdf") {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s requires github.com/unidoc/unipdf; the fixture is committed, the library is not", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// resolvedNode is one node of `dump object --json --resolve` output.
type resolvedNode struct {
	Key       string          `json:"key"`
	ObjectRef string          `json:"objectRef"`
	NodeType  string          `json:"nodeType"`
	Children  []*resolvedNode `json:"children"`
}

// child returns the descendant reached by following keys, or nil.
func (n *resolvedNode) child(keys ...string) *resolvedNode {
	cur := n
	for _, k := range keys {
		if cur == nil {
			return nil
		}
		var next *resolvedNode
		for _, c := range cur.Children {
			if c.Key == k {
				next = c
			}
		}
		cur = next
	}
	return cur
}

// objectOutput is `dump object --json` output.
type objectOutput struct {
	Properties []struct {
		Key   string `json:"key"`
		Value struct {
			Type      string `json:"type"`
			Display   string `json:"display"`
			RefTarget string `json:"refTarget"`
		} `json:"value"`
	} `json:"properties"`
	Resolved *resolvedNode `json:"resolved"`
}

func (o objectOutput) property(key string) (display, refTarget string, ok bool) {
	for _, p := range o.Properties {
		if p.Key == key {
			return p.Value.Display, p.Value.RefTarget, true
		}
	}
	return "", "", false
}

// runJSON runs a CLI command that must exit 0 and unmarshals its stdout into v.
func runJSON(t *testing.T, bin string, v any, args ...string) {
	t.Helper()
	stdout, stderr, code := runCLI(t, bin, args...)
	if code != 0 {
		t.Fatalf("%v: exit %d, want 0\nstderr: %s", args, code, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), v); err != nil {
		t.Fatalf("%v: stdout is not the expected JSON: %v\n%s", args, err, stdout)
	}
}

// dumpObject runs `dump object --json` on ref, resolving depth ref levels when
// depth > 0.
func dumpObject(t *testing.T, bin, file, ref string, depth int) objectOutput {
	t.Helper()
	args := []string{"dump", "object", "--json"}
	if depth > 0 {
		args = append(args, "--resolve", "--resolve-depth", fmt.Sprint(depth))
	}
	args = append(args, "--ref", ref, file)
	var out objectOutput
	runJSON(t, bin, &out, args...)
	if depth > 0 && out.Resolved == nil {
		t.Fatalf("dump object --resolve --ref %q returned no resolved tree", ref)
	}
	return out
}

// refOfNodeID turns an "obj:<gen>:<num>" node ID into "<num> <gen> R".
func refOfNodeID(t *testing.T, nodeID string) string {
	t.Helper()
	parts := strings.Split(nodeID, ":")
	if len(parts) != 3 || parts[0] != "obj" {
		t.Fatalf("node ID %q does not address an indirect object", nodeID)
	}
	return parts[2] + " " + parts[1] + " R"
}

// signedLTVRefs are the object refs the every-command table needs, found
// through the CLI.
type signedLTVRefs struct {
	catalog string
	dss     string
	font    string
	images  map[string]string // field name -> its widget image
	smasks  map[string]string // field name -> the /SMask target of its widget image, when it has one
}

// widgetImage walks from a signature field to the one image XObject its
// widget's /AP /N form reaches (directly or through nested forms) that is not
// another image's /SMask target, and returns it with its /SMask target.
func widgetImage(t *testing.T, bin, file, field, fieldRef string) (image, smask string) {
	t.Helper()
	fieldObj := dumpObject(t, bin, file, fieldRef, 4).Resolved
	widget := fieldObj
	if widget.child("AP") == nil {
		kids := fieldObj.child("Kids")
		if kids == nil || len(kids.Children) != 1 {
			t.Fatalf("%s has no /AP and not exactly one /Kids widget", field)
		}
		widget = kids.Children[0]
	}
	n := widget.child("AP", "N")
	if n == nil || n.ObjectRef == "" {
		t.Fatalf("%s's widget has no indirect /AP /N form", field)
	}

	var images []string
	smaskOf := map[string]string{}
	seen := map[string]bool{}
	var walk func(form string)
	walk = func(form string) {
		if seen[form] {
			return
		}
		seen[form] = true
		xobjs := dumpObject(t, bin, file, form, 3).Resolved.child("Resources", "XObject")
		if xobjs == nil {
			return
		}
		for _, x := range xobjs.Children {
			if x.ObjectRef == "" {
				continue
			}
			obj := dumpObject(t, bin, file, x.ObjectRef, 0)
			switch subtype, _, _ := obj.property("/Subtype"); subtype {
			case "/Image":
				images = append(images, x.ObjectRef)
				if _, target, ok := obj.property("/SMask"); ok && target != "" {
					smaskOf[x.ObjectRef] = refOfNodeID(t, target)
				}
			case "/Form":
				walk(x.ObjectRef)
			}
		}
	}
	walk(n.ObjectRef)

	targets := map[string]bool{}
	for _, s := range smaskOf {
		targets[s] = true
	}
	var widgetImages []string
	for _, img := range images {
		if !targets[img] {
			widgetImages = append(widgetImages, img)
		}
	}
	if len(widgetImages) != 1 {
		t.Fatalf("%s's appearance reaches %d widget images, want 1: %v", field, len(widgetImages), widgetImages)
	}
	return widgetImages[0], smaskOf[widgetImages[0]]
}

// findSignedLTVRefs finds the catalog from `dump objects`, /DSS and page 1's
// font through `dump object --resolve`, and each field's widget image by
// walking from the field `dump signatures` reports.
func findSignedLTVRefs(t *testing.T, bin, file string) signedLTVRefs {
	t.Helper()
	refs := signedLTVRefs{images: map[string]string{}, smasks: map[string]string{}}

	var objects []struct {
		ObjNum    int    `json:"objNum"`
		Gen       int    `json:"gen"`
		TypeName  string `json:"typeName"`
		Reachable bool   `json:"reachable"`
	}
	runJSON(t, bin, &objects, "dump", "objects", "--json", file)
	for _, o := range objects {
		if o.TypeName == "Catalog" && o.Reachable {
			refs.catalog = fmt.Sprintf("%d %d R", o.ObjNum, o.Gen)
		}
	}
	if refs.catalog == "" {
		t.Fatal("dump objects lists no reachable Catalog row")
	}

	dss := dumpObject(t, bin, file, refs.catalog, 1).Resolved.child("DSS")
	if dss == nil || dss.ObjectRef == "" {
		t.Fatal("the catalog has no indirect /DSS")
	}
	refs.dss = dss.ObjectRef

	var pages []struct {
		PageNum int `json:"pageNum"`
		ObjNum  int `json:"objNum"`
		Gen     int `json:"gen"`
	}
	runJSON(t, bin, &pages, "dump", "pages", "--json", file)
	if len(pages) != 1 || pages[0].PageNum != 1 {
		t.Fatalf("dump pages = %+v, want one page", pages)
	}
	fonts := dumpObject(t, bin, file, fmt.Sprintf("%d %d R", pages[0].ObjNum, pages[0].Gen), 3).Resolved.child("Resources", "Font")
	if fonts == nil {
		t.Fatal("page 1 has no /Resources /Font")
	}
	for _, f := range fonts.Children {
		if f.ObjectRef != "" {
			refs.font = f.ObjectRef
			break
		}
	}
	if refs.font == "" {
		t.Fatal("page 1's /Resources /Font has no indirect font")
	}

	var sigs []struct {
		FieldName   string `json:"fieldName"`
		FieldNodeID string `json:"fieldNodeId"`
	}
	runJSON(t, bin, &sigs, "dump", "signatures", "--json", file)
	if len(sigs) != 5 {
		t.Fatalf("dump signatures lists %d fields, want 5: %+v", len(sigs), sigs)
	}
	for _, s := range sigs {
		img, smask := widgetImage(t, bin, file, s.FieldName, refOfNodeID(t, s.FieldNodeID))
		refs.images[s.FieldName] = img
		if smask != "" {
			refs.smasks[s.FieldName] = smask
		}
	}
	for k := 1; k <= 5; k++ {
		if _, ok := refs.images[fmt.Sprintf("Signature%d", k)]; !ok {
			t.Fatalf("no signature field named Signature%d: %v", k, refs.images)
		}
	}
	return refs
}

// Every command opens the fixture: each exits as its contract says, prints
// JSON on stdout, and never reports a malformed PDF.
func TestSignedIncrementalLTV_EveryCommandOpensTheFile(t *testing.T) {
	file := signedLTVPath(t)
	bin := buildCLI(t)
	refs := findSignedLTVRefs(t, bin, file)
	minimal := filepath.Join(projectRoot(t), "testdata", "minimal.pdf")

	type run struct {
		args  []string
		exits []int
	}
	ok := []int{0}
	runs := []run{
		{[]string{"dump", "tree", "--json", file}, ok},
		{[]string{"dump", "tree", "--json", "--depth", "2", "--resolve", file}, ok},
		{[]string{"dump", "object", "--json", "--ref", refs.catalog, file}, ok},
		{[]string{"dump", "stream", "--json", "--page", "1", file}, ok},
		{[]string{"dump", "page", "--json", "--info", "1", file}, ok},
		{[]string{"dump", "font", "--json", "--ref", refs.font, file}, ok},
		{[]string{"dump", "source", "--json", "--ref", refs.dss, file}, ok},
		{[]string{"dump", "reverserefs", "--json", "--ref", refs.images["Signature1"], file}, ok},
		{[]string{"dump", "xref", "--json", file}, ok},
		{[]string{"dump", "objects", "--json", file}, ok},
		{[]string{"dump", "pages", "--json", file}, ok},
		{[]string{"dump", "structure", "--json", file}, ok},
		{[]string{"dump", "images", "--json", file}, ok},
		{[]string{"dump", "bytes", "--json", file}, ok},
		{[]string{"dump", "embedded", "--json", file}, ok},
		{[]string{"dump", "metadata", "--json", file}, ok},
		{[]string{"dump", "signatures", "--json", file}, ok},
		{[]string{"validate", "--json", "--profile", "pdfa-1b", file}, []int{0, 1}},
		{[]string{"validate", "--json", "--profile", "pdfua-1-structural", file}, []int{0, 1}},
		{[]string{"validate", "--json", "--profile", "pdfcpu", file}, []int{0, 1}},
		{[]string{"diff", "--json", minimal, file}, []int{1}},
		{[]string{"diff", "--json", file, file}, []int{0}},
	}
	for k := 1; k <= 5; k++ {
		runs = append(runs, run{[]string{"dump", "image", "--json", "--metadata", "--ref", refs.images[fmt.Sprintf("Signature%d", k)], file}, ok})
	}

	root := projectRoot(t) + string(filepath.Separator)
	for _, r := range runs {
		name := strings.ReplaceAll(strings.Join(r.args, " "), root, "")
		t.Run(name, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, bin, r.args...)
			exitOK := false
			for _, e := range r.exits {
				if code == e {
					exitOK = true
				}
			}
			if !exitOK {
				t.Errorf("exit %d, want one of %v\nstderr: %s", code, r.exits, stderr)
			}
			var v any
			if err := json.Unmarshal([]byte(stdout), &v); err != nil {
				t.Errorf("stdout does not unmarshal as JSON: %v\n%.500s", err, stdout)
			}
			if strings.Contains(stderr, "malformed PDF") {
				t.Errorf("stderr reports a malformed PDF:\n%s", stderr)
			}
		})
	}
}

// Each widget image reports its geometry, and smask is set on Signature1's
// image (to its soft mask) and null on the other four.
func TestSignedIncrementalLTV_WidgetImageSMaskThroughCLI(t *testing.T) {
	file := signedLTVPath(t)
	bin := buildCLI(t)
	refs := findSignedLTVRefs(t, bin, file)
	for k := 1; k <= 5; k++ {
		name := fmt.Sprintf("Signature%d", k)
		ref := refs.images[name]
		var img struct {
			Width  int     `json:"width"`
			Height int     `json:"height"`
			SMask  *string `json:"smask"`
			Error  string  `json:"error"`
		}
		runJSON(t, bin, &img, "dump", "image", "--json", "--metadata", "--ref", ref, file)
		if img.Width <= 0 || img.Height <= 0 {
			t.Errorf("%s: image %s is %dx%d (error %q), want its geometry", name, ref, img.Width, img.Height, img.Error)
		}
		switch {
		case k == 1 && img.SMask == nil:
			t.Errorf("%s: image %s smask is null, want its soft mask", name, ref)
		case k == 1 && *img.SMask != refs.smasks[name]:
			t.Errorf("%s: image %s smask = %q, want %q", name, ref, *img.SMask, refs.smasks[name])
		case k != 1 && img.SMask != nil:
			t.Errorf("%s: image %s smask = %q, want null", name, ref, *img.SMask)
		}
	}
}

// The CLI reference, the scripting guide and the changelog describe the open
// error's detail field and diff's file field.
func TestOpenErrorDetail_Documented(t *testing.T) {
	for _, rel := range []string{"cmd/cli/doc.go", "docs/cli/scripting.md"} {
		src := readSource(t, rel)
		if !strings.Contains(src, `"detail"`) {
			t.Errorf("%s does not describe the \"detail\" field of the open error object", rel)
		}
		if !strings.Contains(src, `"file"`) {
			t.Errorf("%s does not describe the \"file\" field diff adds to the open error object", rel)
		}
	}

	// Every version section's ### Changed list is searched, so the entry is
	// found under [Unreleased] and after its release is cut alike.
	found := false
	for _, sec := range strings.Split(readSource(t, "CHANGELOG.md"), "\n## [")[1:] {
		_, changed, ok := strings.Cut(sec, "\n### Changed\n")
		if !ok {
			continue
		}
		if i := strings.Index(changed, "\n### "); i >= 0 {
			changed = changed[:i]
		}
		for _, line := range strings.Split(changed, "\n") {
			if strings.Contains(line, `"detail"`) && strings.Contains(line, "diff") {
				found = true
			}
		}
	}
	if !found {
		t.Error("no CHANGELOG.md ### Changed list has a line naming the open error's \"detail\" field and diff")
	}
}
