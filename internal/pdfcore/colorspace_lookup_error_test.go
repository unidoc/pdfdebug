package pdfcore

import (
	"fmt"
	"testing"

	pdfcpu_render "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// pdfcpu's colour-space component lookup answers a /ColorSpace it cannot read
// with an error, with or without a cross-reference table, and declaredComponents
// answers that error with the caller's fallback.

func colorSpaceTables(t *testing.T) map[string]*pdfcpu_model.XRefTable {
	t.Helper()
	ctx, err := pdfcpu_render.CreateContextWithXRefTable(nil, pdfcpu_types.PaperSize["A4"])
	if err != nil {
		t.Fatalf("CreateContextWithXRefTable: %v", err)
	}
	return map[string]*pdfcpu_model.XRefTable{"nil table": nil, "real table": ctx.XRefTable}
}

func assertLookupErrorsAndFallsBack(t *testing.T, xrt *pdfcpu_model.XRefTable, cs pdfcpu_types.Object) {
	t.Helper()
	sd := &pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{"ColorSpace": cs}}

	var lookupErr error
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("ColorSpaceComponents panicked: %v", p)
			}
		}()
		_, lookupErr = pdfcpu_render.ColorSpaceComponents(xrt, sd)
	}()
	if lookupErr == nil {
		t.Errorf("ColorSpaceComponents returned no error")
	}

	if got := declaredComponents(xrt, sd, maxComponents); got != maxComponents {
		t.Errorf("declaredComponents = %d, want the fallback %d", got, maxComponents)
	}
}

func TestDeclaredComponents_UnresolvableColorSpaceFallsBackOnError(t *testing.T) {
	shapes := []pdfcpu_types.Object{
		pdfcpu_types.NewNameArray("ICCBased"),
		pdfcpu_types.Array{pdfcpu_types.Name("ICCBased"), *pdfcpu_types.NewIndirectRef(999, 0)},
		pdfcpu_types.Array{pdfcpu_types.Name("DeviceN"), *pdfcpu_types.NewIndirectRef(999, 0), pdfcpu_types.Name("DeviceRGB")},
	}
	for name, xrt := range colorSpaceTables(t) {
		for _, cs := range shapes {
			t.Run(fmt.Sprintf("%s %v", name, cs), func(t *testing.T) {
				assertLookupErrorsAndFallsBack(t, xrt, cs)
			})
		}
	}
}

func TestDeclaredComponents_MalformedColorSpaceShapesReturnErrorsAndFallBack(t *testing.T) {
	shapes := []pdfcpu_types.Object{
		pdfcpu_types.Array{},
		pdfcpu_types.NewNameArray("Indexed"),
		pdfcpu_types.Array{pdfcpu_types.Integer(5)},
	}
	for name, xrt := range colorSpaceTables(t) {
		for _, cs := range shapes {
			t.Run(fmt.Sprintf("%s %v", name, cs), func(t *testing.T) {
				assertLookupErrorsAndFallsBack(t, xrt, cs)
			})
		}
	}
}
