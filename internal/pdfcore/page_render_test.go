package pdfcore

import (
	"path/filepath"
	"strings"
	"testing"
)

// renderInfoFixture returns an Inspector with the committed render-info.pdf
// fixture (testdata/page-render/render-info.pdf) opened under tab "cli".
func renderInfoFixture(t *testing.T) *Inspector {
	t.Helper()
	ins := NewInspector()
	path := filepath.Join(testdataDir(t), "page-render", "render-info.pdf")
	if _, err := ins.Open("cli", path); err != nil {
		t.Fatalf("open render-info fixture: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close("cli") })
	return ins
}

// 11.6-UNIT-001 [P0] (AC1): page geometry resolves MediaBox + Rotate from the
// /Pages ancestor (neither is on the page dict itself).
func TestPageRenderInfo_GeometryInheritance(t *testing.T) {
	ins := renderInfoFixture(t)
	info, err := ins.PageRenderInfo("cli", 1, PageRenderOpts{})
	if err != nil {
		t.Fatalf("PageRenderInfo: %v", err)
	}
	if got, want := info.PageRef, "3 0 R"; got != want {
		t.Errorf("pageRef = %q, want %q", got, want)
	}
	wantMB := []float64{0, 0, 300, 400}
	if len(info.MediaBox) != 4 {
		t.Fatalf("mediaBox = %v, want %v (inherited from /Pages)", info.MediaBox, wantMB)
	}
	for i := range wantMB {
		if info.MediaBox[i] != wantMB[i] {
			t.Errorf("mediaBox[%d] = %v, want %v (inheritance)", i, info.MediaBox[i], wantMB[i])
		}
	}
	if info.Rotate != 90 {
		t.Errorf("rotate = %d, want 90 (inherited from /Pages)", info.Rotate)
	}
}

// 11.6-UNIT-002 [P0] (AC2): ExtGState carries BM/ca/CA and a resolved SMask
// descriptor (None-vs-dict resolution).
func TestPageRenderInfo_ExtGStateAndSMask(t *testing.T) {
	ins := renderInfoFixture(t)
	info, err := ins.PageRenderInfo("cli", 1, PageRenderOpts{})
	if err != nil {
		t.Fatalf("PageRenderInfo: %v", err)
	}
	if len(info.ExtGStates) != 1 {
		t.Fatalf("extGStates = %d entries, want 1", len(info.ExtGStates))
	}
	gs := info.ExtGStates[0]
	if gs.Name != "GS0" || gs.Ref != "5 0 R" {
		t.Errorf("extGState name/ref = %q/%q, want GS0/5 0 R", gs.Name, gs.Ref)
	}
	if gs.BM != "Multiply" {
		t.Errorf("BM = %q, want Multiply", gs.BM)
	}
	if gs.CA == nil || *gs.CA != 0.5 {
		t.Errorf("ca = %v, want 0.5", gs.CA)
	}
	if gs.Ca == nil || *gs.Ca != 1.0 {
		t.Errorf("CA = %v, want 1.0", gs.Ca)
	}
	desc, ok := gs.SMask.(*SMaskDescriptor)
	if !ok || desc == nil {
		t.Fatalf("SMask = %v (%T), want a resolved *SMaskDescriptor", gs.SMask, gs.SMask)
	}
	if desc.S != "Luminosity" {
		t.Errorf("SMask /S = %q, want Luminosity", desc.S)
	}
	if desc.GRef != "7 0 R" {
		t.Errorf("SMask /G ref = %q, want 7 0 R", desc.GRef)
	}
}

// 11.6-UNIT-003 [P0] (AC3): Form vs Image classification; Form carries
// bbox/matrix/group, Image carries width/height + ICCBased colorspace family
// with N + profile size.
func TestPageRenderInfo_XObjectClassification(t *testing.T) {
	ins := renderInfoFixture(t)
	info, err := ins.PageRenderInfo("cli", 1, PageRenderOpts{})
	if err != nil {
		t.Fatalf("PageRenderInfo: %v", err)
	}
	byName := map[string]XObjectRenderInfo{}
	for _, x := range info.XObjects {
		byName[x.Name] = x
	}

	fm0, ok := byName["Fm0"]
	if !ok {
		t.Fatalf("Fm0 not in xobjects")
	}
	if fm0.Subtype != "Form" {
		t.Errorf("Fm0 subtype = %q, want Form", fm0.Subtype)
	}
	if len(fm0.BBox) != 4 || len(fm0.Matrix) != 6 {
		t.Errorf("Fm0 bbox/matrix lengths = %d/%d, want 4/6", len(fm0.BBox), len(fm0.Matrix))
	}
	if fm0.Group == nil || fm0.Group.S != "Transparency" || fm0.Group.CS != "DeviceRGB" || !fm0.Group.K {
		t.Errorf("Fm0 group = %+v, want S=Transparency CS=DeviceRGB K=true", fm0.Group)
	}

	im0, ok := byName["Im0"]
	if !ok {
		t.Fatalf("Im0 not in xobjects")
	}
	if im0.Subtype != "Image" || im0.Width != 4 || im0.Height != 4 {
		t.Errorf("Im0 = %+v, want Image 4x4", im0)
	}
	if im0.ColorSpace == nil || im0.ColorSpace.Family != "ICCBased" {
		t.Fatalf("Im0 colorSpace = %+v, want family ICCBased", im0.ColorSpace)
	}
	if im0.ColorSpace.N != 3 {
		t.Errorf("Im0 ICC N = %d, want 3", im0.ColorSpace.N)
	}
	if im0.ColorSpace.ICCProfileSize <= 0 {
		t.Errorf("Im0 ICC profile size = %d, want > 0", im0.ColorSpace.ICCProfileSize)
	}
	if im0.ColorSpace.AltFamily != "DeviceRGB" {
		t.Errorf("Im0 ICC altFamily = %q, want DeviceRGB", im0.ColorSpace.AltFamily)
	}
}

// 11.6-UNIT-004 [P0] (AC1/AC7): Pattern/Shading entries are structural only
// (name + ref + type integer), no evaluation.
func TestPageRenderInfo_PatternShadingStructuralOnly(t *testing.T) {
	ins := renderInfoFixture(t)
	info, err := ins.PageRenderInfo("cli", 1, PageRenderOpts{})
	if err != nil {
		t.Fatalf("PageRenderInfo: %v", err)
	}
	if len(info.Patterns) != 1 || info.Patterns[0].Name != "P0" || info.Patterns[0].PatternType != 2 {
		t.Errorf("patterns = %+v, want one P0 patternType 2", info.Patterns)
	}
	if len(info.Shadings) != 1 || info.Shadings[0].Name != "Sh0" || info.Shadings[0].ShadingType != 2 {
		t.Errorf("shadings = %+v, want one Sh0 shadingType 2", info.Shadings)
	}
}

// 11.6-UNIT-005 [P0] (AC4): recursive form walk resolves nested forms against
// their OWN resources AND terminates a self-referential Do chain via the
// form-object-ref visited set (no infinite loop).
func TestPageRenderInfo_RecursiveFormWalkTerminates(t *testing.T) {
	ins := renderInfoFixture(t)
	info, err := ins.PageRenderInfo("cli", 1, PageRenderOpts{FormsRecursive: true, FormsDepth: 5})
	if err != nil {
		t.Fatalf("PageRenderInfo: %v", err)
	}
	byName := map[string]FormRenderInfo{}
	for _, f := range info.Forms {
		byName[f.Name] = f
	}

	// Fm0 -> Fm1, resolved against Fm0's OWN /Resources (the inner-Fm0 gotcha).
	fm0, ok := byName["Fm0"]
	if !ok {
		t.Fatalf("Fm0 not in forms")
	}
	if len(fm0.XObjects) != 1 || fm0.XObjects[0].Name != "Fm1" {
		t.Errorf("Fm0 own xobjects = %+v, want [Fm1]", fm0.XObjects)
	}

	// FmSelf Do-chains to itself: the inner FmSelf must be marked cyclic and NOT
	// recursed into (the walk terminates).
	fmSelf, ok := byName["FmSelf"]
	if !ok {
		t.Fatalf("FmSelf not in forms")
	}
	if len(fmSelf.Forms) != 1 {
		t.Fatalf("FmSelf nested forms = %d, want 1 (the cyclic back-edge)", len(fmSelf.Forms))
	}
	if !fmSelf.Forms[0].Cyclic {
		t.Errorf("FmSelf self-reference not marked cyclic: %+v", fmSelf.Forms[0])
	}
}

// 11.6-UNIT-006 [P1] (AC4): --forms-depth bounds the walk. --forms-depth 1
// expands exactly one level: the first-level Fm0 is walked (its OWN resources
// are classified) and marked Truncated because it has nested forms, but those
// 2nd-level forms are NOT emitted at all (AC4-004: a form below the cap must not
// appear in the tree, not even as a marker naming its ref).
func TestPageRenderInfo_FormsDepthTruncates(t *testing.T) {
	ins := renderInfoFixture(t)
	info, err := ins.PageRenderInfo("cli", 1, PageRenderOpts{FormsRecursive: true, FormsDepth: 1})
	if err != nil {
		t.Fatalf("PageRenderInfo: %v", err)
	}
	var fm0 *FormRenderInfo
	for i := range info.Forms {
		if info.Forms[i].Name == "Fm0" {
			fm0 = &info.Forms[i]
		}
	}
	if fm0 == nil {
		t.Fatalf("Fm0 not in forms")
	}
	if len(fm0.Forms) != 0 || !fm0.Truncated {
		t.Errorf("Fm0 at depth 1 should be Truncated with no expanded child forms: truncated=%v forms=%+v", fm0.Truncated, fm0.Forms)
	}
}

// 11.6-UNIT-007 [P1] (AC4): without FormsRecursive, forms are listed in
// xobjects but the Forms tree is not populated.
func TestPageRenderInfo_NoRecursionByDefault(t *testing.T) {
	ins := renderInfoFixture(t)
	info, err := ins.PageRenderInfo("cli", 1, PageRenderOpts{})
	if err != nil {
		t.Fatalf("PageRenderInfo: %v", err)
	}
	if info.Forms != nil {
		t.Errorf("forms walked without FormsRecursive: %+v", info.Forms)
	}
}

// 11.6-UNIT-008 [P0] (AC6): out-of-range / non-positive page returns an error
// whose message contains "not found" (the CLI maps that to exit 2), never a
// panic.
func TestPageRenderInfo_OutOfRangeError(t *testing.T) {
	ins := renderInfoFixture(t)
	for _, p := range []int{0, -1, 999} {
		_, err := ins.PageRenderInfo("cli", p, PageRenderOpts{})
		if err == nil {
			t.Errorf("page %d: expected error, got nil", p)
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), "not found") {
			t.Errorf("page %d error = %q, want it to mention \"not found\"", p, err.Error())
		}
	}
}

// 11.6-UNIT-009 [P1] (AC6): a valid page with no /Resources yields empty
// (non-nil) arrays at no error - an absent resource is a valid empty result.
func TestPageRenderInfo_NoResourcesEmptyArrays(t *testing.T) {
	ins := NewInspector()
	path := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := ins.Open("cli", path); err != nil {
		t.Fatalf("open minimal.pdf: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close("cli") })

	info, err := ins.PageRenderInfo("cli", 1, PageRenderOpts{})
	if err != nil {
		t.Fatalf("PageRenderInfo on no-/Resources page: %v", err)
	}
	if info.ExtGStates == nil || info.XObjects == nil || info.Patterns == nil || info.Shadings == nil {
		t.Errorf("no-/Resources page must emit non-nil empty arrays, got eg=%v xo=%v pat=%v sh=%v",
			info.ExtGStates, info.XObjects, info.Patterns, info.Shadings)
	}
	if len(info.ExtGStates) != 0 || len(info.XObjects) != 0 {
		t.Errorf("no-/Resources page must emit empty arrays, got %d extGStates, %d xobjects",
			len(info.ExtGStates), len(info.XObjects))
	}
}
