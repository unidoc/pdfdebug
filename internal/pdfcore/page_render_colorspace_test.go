package pdfcore

import (
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// These unit tests exercise the colorspace-family CLASSIFIER and blend-mode
// reader branches that the end-to-end render-info.pdf fixture does not reach (it
// carries only ICCBased + DeviceRGB/DeviceGray). They cover
// Separation/DeviceN tint-transform function TYPE, Indexed hival, and the
// structural-only constraint. All inputs here are DIRECT (inline) objects, so
// classifyColorSpace never dereferences and a nil *DocumentState is safe.

// A Separation colorspace classifies to family "Separation", carries the
// alternate-space family, and the tint-transform FUNCTION TYPE (structure) -
// never an evaluated tint value.
func TestClassifyColorSpace_Separation(t *testing.T) {
	// [/Separation /SpotColor /DeviceCMYK <tintFn FunctionType 4>]
	cs := pdfcpu_types.Array{
		pdfcpu_types.Name("Separation"),
		pdfcpu_types.Name("SpotColor"),
		pdfcpu_types.Name("DeviceCMYK"),
		pdfcpu_types.Dict{"FunctionType": pdfcpu_types.Integer(4)},
	}
	info := classifyColorSpace(nil, cs)
	if info == nil {
		t.Fatal("classifyColorSpace(Separation) = nil")
	}
	if info.Family != "Separation" {
		t.Errorf("family = %q, want Separation", info.Family)
	}
	if info.AltFamily != "DeviceCMYK" {
		t.Errorf("altFamily = %q, want DeviceCMYK", info.AltFamily)
	}
	if info.TintTransformType != 4 {
		t.Errorf("tintTransformType = %d, want 4 (function TYPE, not an evaluated value)", info.TintTransformType)
	}
}

// A DeviceN colorspace classifies to family "DeviceN" with the alternate
// family and the tint-transform function TYPE.
func TestClassifyColorSpace_DeviceN(t *testing.T) {
	// [/DeviceN [/C1 /C2] /DeviceRGB <tintFn FunctionType 0>]
	cs := pdfcpu_types.Array{
		pdfcpu_types.Name("DeviceN"),
		pdfcpu_types.Array{pdfcpu_types.Name("C1"), pdfcpu_types.Name("C2")},
		pdfcpu_types.Name("DeviceRGB"),
		pdfcpu_types.Dict{"FunctionType": pdfcpu_types.Integer(0)},
	}
	info := classifyColorSpace(nil, cs)
	if info == nil {
		t.Fatal("classifyColorSpace(DeviceN) = nil")
	}
	if info.Family != "DeviceN" {
		t.Errorf("family = %q, want DeviceN", info.Family)
	}
	if info.AltFamily != "DeviceRGB" {
		t.Errorf("altFamily = %q, want DeviceRGB", info.AltFamily)
	}
	if info.TintTransformType != 0 {
		t.Errorf("tintTransformType = %d, want 0 (sampled function TYPE)", info.TintTransformType)
	}
}

// An Indexed colorspace surfaces the palette hival (array element 2),
// classified not evaluated.
func TestClassifyColorSpace_IndexedHiVal(t *testing.T) {
	// [/Indexed /DeviceRGB 255 <lookup>]
	cs := pdfcpu_types.Array{
		pdfcpu_types.Name("Indexed"),
		pdfcpu_types.Name("DeviceRGB"),
		pdfcpu_types.Integer(255),
		pdfcpu_types.StringLiteral("lookup"),
	}
	info := classifyColorSpace(nil, cs)
	if info == nil {
		t.Fatal("classifyColorSpace(Indexed) = nil")
	}
	if info.Family != "Indexed" {
		t.Errorf("family = %q, want Indexed", info.Family)
	}
	if info.HiVal != 255 {
		t.Errorf("hiVal = %d, want 255", info.HiVal)
	}
}

// A bare Name colorspace (DeviceGray/CalGray/Lab/ Pattern) classifies to
// that family with no further structure.
func TestClassifyColorSpace_NameFamilies(t *testing.T) {
	for _, name := range []string{"DeviceGray", "DeviceCMYK", "CalRGB", "Lab", "Pattern"} {
		info := classifyColorSpace(nil, pdfcpu_types.Name(name))
		if info == nil || info.Family != name {
			t.Errorf("classifyColorSpace(/%s) = %+v, want family %q", name, info, name)
		}
	}
}

// Robustness: the alternate-colorspace recursion budget terminates a deeply
// nested Separation->Separation->... alternate chain instead of recursing
// without bound. depth 0 stops the descent so the alternate family is not
// surfaced past the budget; the top family is still classified (no panic, no
// overflow).
func TestClassifyColorSpace_AlternateDepthBudget(t *testing.T) {
	// Two-level direct Separation chain: outer alternate is itself a Separation.
	inner := pdfcpu_types.Array{
		pdfcpu_types.Name("Separation"),
		pdfcpu_types.Name("Spot2"),
		pdfcpu_types.Name("DeviceGray"),
		pdfcpu_types.Dict{"FunctionType": pdfcpu_types.Integer(2)},
	}
	outer := pdfcpu_types.Array{
		pdfcpu_types.Name("Separation"),
		pdfcpu_types.Name("Spot1"),
		inner, // alternate is another Separation
		pdfcpu_types.Dict{"FunctionType": pdfcpu_types.Integer(2)},
	}

	// With budget, the outer's alternate family resolves to the inner family.
	full := classifyColorSpaceDepth(nil, outer, maxColorSpaceDepth)
	if full == nil || full.Family != "Separation" || full.AltFamily != "Separation" {
		t.Errorf("with budget: got %+v, want family/altFamily Separation/Separation", full)
	}

	// At depth 0 the alternate descent is gated off entirely: the top family is
	// still classified (Family set), but the guard refuses to descend so the
	// alternate family is NOT surfaced. This is the branch that terminates a
	// cyclic /Alternate chain instead of overflowing the stack.
	gated := classifyColorSpaceDepth(nil, outer, 0)
	if gated == nil || gated.Family != "Separation" {
		t.Fatalf("at depth 0: got %+v, want top family Separation", gated)
	}
	if gated.AltFamily != "" {
		t.Errorf("at depth 0: altFamily = %q, want empty (alternate descent budget exhausted)", gated.AltFamily)
	}
}

// A /BM array (the mode-fallback list form) reports its first name; an empty
// or non-Name head reports "".
func TestBlendModeName_ArrayForm(t *testing.T) {
	got := blendModeName(pdfcpu_types.Array{pdfcpu_types.Name("Darken"), pdfcpu_types.Name("Normal")})
	if got != "Darken" {
		t.Errorf("blendModeName([/Darken /Normal]) = %q, want Darken", got)
	}
	if got := blendModeName(pdfcpu_types.Array{}); got != "" {
		t.Errorf("blendModeName([]) = %q, want empty", got)
	}
	if got := blendModeName(pdfcpu_types.Name("Multiply")); got != "Multiply" {
		t.Errorf("blendModeName(/Multiply) = %q, want Multiply", got)
	}
}
