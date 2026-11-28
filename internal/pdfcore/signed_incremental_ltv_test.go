package pdfcore

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// signed-incremental-ltv.pdf is a UniPDF-signed file: a %PDF-1.1 base revision
// with one page of Helvetica text, five incremental revisions that each add and
// sign one image-appearance signature field (Signature1 to Signature5, in that
// order; only Signature1's image has an /SMask), and a last incremental
// revision that LTV-enables the file with a catalog /DSS holding /Certs and one
// /VRI entry per signature.

const signedLTVFixture = "signed-incremental-ltv.pdf"

const signedLTVFieldCount = 5

// signedLTVBytes reads the fixture, failing the test when it is missing.
func signedLTVBytes(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(testdataDir(t), signedLTVFixture))
	if err != nil {
		t.Fatalf("fixture testdata/%s is required: %v", signedLTVFixture, err)
	}
	return b
}

// openSignedLTV opens the fixture through Inspector.Open and fails the test if
// Open refuses it, warns, or does not count one page.
func openSignedLTV(t *testing.T, tabID string) (*Inspector, *DocumentState) {
	t.Helper()
	signedLTVBytes(t)
	ins := NewInspector()
	info, err := ins.Open(tabID, filepath.Join(testdataDir(t), signedLTVFixture))
	if err != nil {
		t.Fatalf("Open refused %s: %v", signedLTVFixture, err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })
	if info.Error != "" {
		t.Errorf("open warning = %q, want none", info.Error)
	}
	if info.PageCount != 1 {
		t.Errorf("page count = %d, want 1", info.PageCount)
	}
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatal(err)
	}
	return ins, doc
}

// eofMarkers returns, for each %%EOF marker in b, the offset just past the
// marker and the offset just past the one end-of-line (CR, LF or CRLF) that
// follows it.
func eofMarkers(b []byte) (ends, lineEnds []int64) {
	marker := []byte("%%EOF")
	for off := 0; ; {
		i := bytes.Index(b[off:], marker)
		if i < 0 {
			return ends, lineEnds
		}
		end := off + i + len(marker)
		lineEnd := end
		if lineEnd < len(b) && b[lineEnd] == '\r' {
			lineEnd++
		}
		if lineEnd < len(b) && b[lineEnd] == '\n' {
			lineEnd++
		}
		ends = append(ends, int64(end))
		lineEnds = append(lineEnds, int64(lineEnd))
		off = end
	}
}

// refOfNodeID returns the indirect reference an "obj:<gen>:<num>" node ID
// addresses.
func refOfNodeID(t *testing.T, nodeID string) pdfcpu_types.IndirectRef {
	t.Helper()
	parts := strings.Split(nodeID, ":")
	if len(parts) != 3 || parts[0] != "obj" {
		t.Fatalf("node ID %q does not address an indirect object", nodeID)
	}
	gen, err1 := strconv.Atoi(parts[1])
	num, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil {
		t.Fatalf("node ID %q has a non-numeric part", nodeID)
	}
	return *pdfcpu_types.NewIndirectRef(num, gen)
}

// derefDict dereferences o and returns its dictionary (a stream's dictionary
// for a stream), or nil.
func derefDict(t *testing.T, doc *DocumentState, o pdfcpu_types.Object) pdfcpu_types.Dict {
	t.Helper()
	v, err := doc.PDFContext.Dereference(o)
	if err != nil {
		t.Fatalf("dereference %v: %v", o, err)
	}
	switch d := v.(type) {
	case pdfcpu_types.Dict:
		return d
	case pdfcpu_types.StreamDict:
		return d.Dict
	}
	return nil
}

// widgetAppearance is what one signature field's widget shows.
type widgetAppearance struct {
	widget pdfcpu_types.IndirectRef
	images []pdfcpu_types.IndirectRef // image XObjects that are not another image's /SMask
	smasks []pdfcpu_types.IndirectRef // /SMask targets of those images
}

// fieldWidgets returns the widget annotations of a field: the field itself
// when it is merged with its widget, otherwise its /Kids.
func fieldWidgets(t *testing.T, doc *DocumentState, field pdfcpu_types.IndirectRef) []pdfcpu_types.IndirectRef {
	t.Helper()
	d := derefDict(t, doc, field)
	if d == nil {
		t.Fatalf("field %s is not a dictionary", field)
	}
	if st := d.NameEntry("Subtype"); (st != nil && *st == "Widget") || d["AP"] != nil {
		return []pdfcpu_types.IndirectRef{field}
	}
	var widgets []pdfcpu_types.IndirectRef
	if kids, ok := d["Kids"].(pdfcpu_types.Array); ok {
		for _, k := range kids {
			if r, ok := k.(pdfcpu_types.IndirectRef); ok {
				widgets = append(widgets, r)
			}
		}
	}
	return widgets
}

// appearanceOf walks a widget's /AP /N form and the form XObjects it uses,
// collecting the image XObjects it reaches and their /SMask targets.
func appearanceOf(t *testing.T, doc *DocumentState, widget pdfcpu_types.IndirectRef) widgetAppearance {
	t.Helper()
	out := widgetAppearance{widget: widget}
	wd := derefDict(t, doc, widget)
	ap := derefDict(t, doc, wd["AP"])
	if ap == nil {
		t.Fatalf("widget %s has no /AP dictionary", widget)
	}
	n, ok := ap["N"].(pdfcpu_types.IndirectRef)
	if !ok {
		t.Fatalf("widget %s /AP /N = %v, want an indirect form XObject", widget, ap["N"])
	}
	seen := map[int]bool{}
	var images []pdfcpu_types.IndirectRef
	smaskTargets := map[int]bool{}
	var walk func(form pdfcpu_types.IndirectRef)
	walk = func(form pdfcpu_types.IndirectRef) {
		if seen[form.ObjectNumber.Value()] {
			return
		}
		seen[form.ObjectNumber.Value()] = true
		res := derefDict(t, doc, derefDict(t, doc, form)["Resources"])
		if res == nil {
			return
		}
		xobjs := derefDict(t, doc, res["XObject"])
		for _, v := range xobjs {
			r, ok := v.(pdfcpu_types.IndirectRef)
			if !ok {
				continue
			}
			xd := derefDict(t, doc, r)
			if xd == nil {
				continue
			}
			switch st := xd.NameEntry("Subtype"); {
			case st != nil && *st == "Image":
				images = append(images, r)
				if sm, ok := xd["SMask"].(pdfcpu_types.IndirectRef); ok {
					smaskTargets[sm.ObjectNumber.Value()] = true
					out.smasks = append(out.smasks, sm)
				}
			case st != nil && *st == "Form":
				walk(r)
			}
		}
	}
	walk(n)
	for _, img := range images {
		if !smaskTargets[img.ObjectNumber.Value()] {
			out.images = append(out.images, img)
		}
	}
	return out
}

// signedLTVAppearances maps each signature field name to what its one widget
// shows, failing the test unless the fixture lists exactly Signature1 to
// Signature5, each with one widget and one widget image.
func signedLTVAppearances(t *testing.T, ins *Inspector, doc *DocumentState, tabID string) (map[string]widgetAppearance, map[string]SignatureField) {
	t.Helper()
	list, err := ins.GetSignatures(tabID)
	if err != nil {
		t.Fatalf("GetSignatures: %v", err)
	}
	fields := map[string]SignatureField{}
	for _, s := range list.Signatures {
		fields[s.FieldName] = s
	}
	if len(list.Signatures) != signedLTVFieldCount {
		t.Fatalf("GetSignatures listed %d fields, want %d: %+v", len(list.Signatures), signedLTVFieldCount, fields)
	}
	apps := map[string]widgetAppearance{}
	for k := 1; k <= signedLTVFieldCount; k++ {
		name := fmt.Sprintf("Signature%d", k)
		f, ok := fields[name]
		if !ok {
			t.Fatalf("no signature field named %s; fields: %v", name, fields)
		}
		widgets := fieldWidgets(t, doc, refOfNodeID(t, f.FieldNodeID))
		if len(widgets) != 1 {
			t.Fatalf("%s has %d widgets, want 1", name, len(widgets))
		}
		app := appearanceOf(t, doc, widgets[0])
		if len(app.images) != 1 {
			t.Fatalf("%s's appearance reaches %d widget images, want 1: %v", name, len(app.images), app.images)
		}
		apps[name] = app
	}
	return apps, fields
}

// The fixture has the shape it is described as having: a %PDF-1.1 header,
// seven revisions, five /FT /Sig fields with one widget each on page 1, a font
// in page 1's resources, and a last-revision catalog whose /DSS holds /Certs
// and five /VRI entries.
func TestSignedIncrementalLTV_FixtureShape(t *testing.T) {
	b := signedLTVBytes(t)
	if !bytes.HasPrefix(b, []byte("%PDF-1.1")) {
		t.Errorf("header = %q, want %%PDF-1.1", b[:min(len(b), 8)])
	}
	if ends, _ := eofMarkers(b); len(ends) < 7 {
		t.Errorf("found %d %%%%EOF markers, want at least 7 (base, five signing revisions, LTV revision)", len(ends))
	}

	ins, doc := openSignedLTV(t, "ltv-shape")
	_, fields := signedLTVAppearances(t, ins, doc, "ltv-shape")

	pages, err := ins.GetPageIndex("ltv-shape")
	if err != nil || len(pages) == 0 {
		t.Fatalf("GetPageIndex = %v, %v; want page 1", pages, err)
	}
	page := derefDict(t, doc, *pdfcpu_types.NewIndirectRef(pages[0].ObjNum, pages[0].Gen))
	annots := map[int]bool{}
	if arr, ok := derefObj(t, doc, page["Annots"]).(pdfcpu_types.Array); ok {
		for _, a := range arr {
			if r, ok := a.(pdfcpu_types.IndirectRef); ok {
				annots[r.ObjectNumber.Value()] = true
			}
		}
	}
	for name, f := range fields {
		fd := derefDict(t, doc, refOfNodeID(t, f.FieldNodeID))
		if ft := fd.NameEntry("FT"); ft == nil || *ft != "Sig" {
			t.Errorf("%s /FT = %v, want /Sig", name, fd["FT"])
		}
		for _, w := range fieldWidgets(t, doc, refOfNodeID(t, f.FieldNodeID)) {
			if !annots[w.ObjectNumber.Value()] {
				t.Errorf("%s's widget %s is not in page 1's /Annots", name, w)
			}
		}
	}

	res := derefDict(t, doc, page["Resources"])
	fonts := derefDict(t, doc, res["Font"])
	if len(fonts) == 0 {
		t.Error("page 1 has no font in its resources")
	}

	cat, err := doc.PDFContext.Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	dss := derefDict(t, doc, cat["DSS"])
	if dss == nil {
		t.Fatal("catalog has no /DSS dictionary")
	}
	if vri := derefDict(t, doc, dss["VRI"]); len(vri) != signedLTVFieldCount {
		t.Errorf("/DSS /VRI has %d entries, want %d", len(vri), signedLTVFieldCount)
	}
	if certs, ok := derefObj(t, doc, dss["Certs"]).(pdfcpu_types.Array); !ok || len(certs) == 0 {
		t.Errorf("/DSS /Certs = %v, want a non-empty array", dss["Certs"])
	}
}

// derefObj dereferences o, failing the test on a dereference error.
func derefObj(t *testing.T, doc *DocumentState, o pdfcpu_types.Object) pdfcpu_types.Object {
	t.Helper()
	v, err := doc.PDFContext.Dereference(o)
	if err != nil {
		t.Fatalf("dereference %v: %v", o, err)
	}
	return v
}

// Open reads the last revision: the catalog the tree shows carries /DSS, which
// the base revision's catalog does not, and /DSS /VRI and /DSS /Certs resolve.
// The five widget images and Signature1's soft mask are reachable.
func TestSignedIncrementalLTV_OpensAtTheLastRevision(t *testing.T) {
	b := signedLTVBytes(t)
	ins, doc := openSignedLTV(t, "ltv-open")

	ends, _ := eofMarkers(b)
	if len(ends) == 0 {
		t.Fatal("fixture has no end-of-file marker")
	}
	base, err := pdfcpu_api.ReadContext(context.Background(), bytes.NewReader(b[:ends[0]]), pdfcpu_model.NewStatelessConfiguration())
	if err != nil {
		t.Fatalf("parse of the base revision alone: %v", err)
	}
	baseCat, err := base.Catalog()
	if err != nil {
		t.Fatalf("base revision catalog: %v", err)
	}
	if baseCat["DSS"] != nil {
		t.Fatal("fixture: the base revision's catalog already has /DSS, so it cannot tell the revisions apart")
	}

	children, err := ins.GetChildren("ltv-open", "root")
	if err != nil {
		t.Fatalf("GetChildren(root): %v", err)
	}
	var dssID string
	for _, c := range children {
		if c.RawKey == "/DSS" {
			dssID = c.ID
		}
	}
	if dssID == "" {
		t.Fatal("the catalog shown in the tree has no /DSS; Open did not read the last revision")
	}
	dss, err := ins.ResolveRef("ltv-open", dssID, ResolveOpts{MaxDepth: 1})
	if err != nil {
		t.Fatalf("ResolveRef /DSS: %v", err)
	}
	got := map[string]*ResolvedNode{}
	for _, c := range dss.Children {
		got[c.Key] = c
	}
	if vri := got["VRI"]; vri == nil || vri.NodeType != "dict" || len(vri.Children) != signedLTVFieldCount {
		t.Errorf("/DSS /VRI = %+v, want a dictionary with %d entries", vri, signedLTVFieldCount)
	}
	if certs := got["Certs"]; certs == nil || certs.NodeType != "array" || len(certs.Children) == 0 {
		t.Errorf("/DSS /Certs = %+v, want a non-empty array", certs)
	}

	apps, _ := signedLTVAppearances(t, ins, doc, "ltv-open")
	index, err := ins.GetObjectIndex("ltv-open")
	if err != nil {
		t.Fatalf("GetObjectIndex: %v", err)
	}
	reachable := map[int]bool{}
	for _, e := range index {
		if e.Reachable {
			reachable[e.ObjNum] = true
		}
	}
	for name, app := range apps {
		for _, img := range app.images {
			if !reachable[img.ObjectNumber.Value()] {
				t.Errorf("%s's widget image %s is not reachable in the object index", name, img)
			}
		}
	}
	sm := apps["Signature1"].smasks
	if len(sm) != 1 {
		t.Fatalf("Signature1's image has %d soft masks, want 1", len(sm))
	}
	if !reachable[sm[0].ObjectNumber.Value()] {
		t.Errorf("Signature1's soft mask %s is not reachable in the object index", sm[0])
	}
}

// Each widget image reports its geometry, and smask is set on Signature1's
// image only.
func TestSignedIncrementalLTV_WidgetImagesReportSMask(t *testing.T) {
	ins, doc := openSignedLTV(t, "ltv-images")
	apps, _ := signedLTVAppearances(t, ins, doc, "ltv-images")
	for k := 1; k <= signedLTVFieldCount; k++ {
		name := fmt.Sprintf("Signature%d", k)
		img := apps[name].images[0]
		nodeID := fmt.Sprintf("obj:%d:%d", img.GenerationNumber.Value(), img.ObjectNumber.Value())
		data, err := ins.GetImageData(context.Background(), "ltv-images", nodeID)
		if err != nil {
			t.Fatalf("%s: GetImageData %s: %v", name, nodeID, err)
		}
		if data.Width <= 0 || data.Height <= 0 {
			t.Errorf("%s: image %s is %dx%d, want its geometry", name, nodeID, data.Width, data.Height)
		}
		switch {
		case k == 1 && data.SMask == nil:
			t.Errorf("%s: image %s smask is null, want its soft mask reference", name, nodeID)
		case k == 1 && *data.SMask != apps[name].smasks[0].PDFString():
			t.Errorf("%s: image %s smask = %q, want %q", name, nodeID, *data.SMask, apps[name].smasks[0].PDFString())
		case k != 1 && data.SMask != nil:
			t.Errorf("%s: image %s smask = %q, want null", name, nodeID, *data.SMask)
		}
	}
}

// Every signature is followed by later revisions, so none covers the whole
// file. Signature K was applied in revision K+1 (the base is revision 1), so
// its trailing gap is the file size minus the end of revision K+1, found by
// scanning the %%EOF markers. UniPDF's ByteRange ends after the one LF that
// follows %%EOF, so the end of a revision is the end of its %%EOF line.
func TestSignedIncrementalLTV_SignatureCoverageFollowsRevisions(t *testing.T) {
	b := signedLTVBytes(t)
	ins, _ := openSignedLTV(t, "ltv-coverage")
	list, err := ins.GetSignatures("ltv-coverage")
	if err != nil {
		t.Fatalf("GetSignatures: %v", err)
	}
	byName := map[string]SignatureField{}
	for _, s := range list.Signatures {
		byName[s.FieldName] = s
	}
	_, lineEnds := eofMarkers(b)
	if len(lineEnds) < signedLTVFieldCount+2 {
		t.Fatalf("found %d %%%%EOF markers, want at least %d", len(lineEnds), signedLTVFieldCount+2)
	}
	size := int64(len(b))
	prev := int64(-1)
	for k := 1; k <= signedLTVFieldCount; k++ {
		name := fmt.Sprintf("Signature%d", k)
		s, ok := byName[name]
		if !ok {
			t.Fatalf("no signature field named %s", name)
		}
		if !s.Signed || s.DecomposeError != "" || s.CoverageError != "" || s.HoleCheckError != "" {
			t.Errorf("%s: signed %v, decomposeError %q, coverageError %q, holeCheckError %q; want signed with no errors",
				name, s.Signed, s.DecomposeError, s.CoverageError, s.HoleCheckError)
		}
		if s.CoversWholeFile {
			t.Errorf("%s covers the whole file, but later revisions follow it", name)
		}
		if !s.HoleMatchesContents {
			t.Errorf("%s: the ByteRange hole does not match /Contents", name)
		}
		if want := size - lineEnds[k]; s.TrailingGap != want {
			t.Errorf("%s: trailingGap = %d, want %d (file size minus the end of revision %d's %%%%EOF line)",
				name, s.TrailingGap, want, k+1)
		}
		if prev >= 0 && s.TrailingGap >= prev {
			t.Errorf("%s: trailingGap %d is not below the previous signature's %d", name, s.TrailingGap, prev)
		}
		prev = s.TrailingGap
	}
}
