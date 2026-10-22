package pdfcore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Benchmarks for the document-wide walks on a generated tagged document:
// 2000 pages, one image XObject per 10 pages, and 20 000 structure elements
// with one MCID each, 10 per page. Each iteration opens a fresh tab so no
// cache hides the walk; Open (parse and pdfcpu validation) is not timed.
//
// Run: go test -bench . -run '^$' ./internal/pdfcore/

const (
	benchPages        = 2000
	benchMCIDsPerPage = 10
	benchImageEvery   = 10
)

// benchTaggedPDF writes the benchmark document to a temp dir and returns its
// path. Pages are objects 10.., content streams 3000.., images 6000..,
// elements 10000..
func benchTaggedPDF(b *testing.B) string {
	b.Helper()
	const (
		pageBase    = 10
		contentBase = 3000
		imageBase   = 6000
		elemBase    = 10000
	)
	nElems := benchPages * benchMCIDsPerPage
	objs := make([]rawObj, 0, 2*benchPages+nElems+10)

	kids := make([]string, benchPages)
	nums := make([]string, 0, benchPages)
	for p := 0; p < benchPages; p++ {
		kids[p] = fmt.Sprintf("%d 0 R", pageBase+p)
		slots := make([]string, benchMCIDsPerPage)
		var content strings.Builder
		for m := 0; m < benchMCIDsPerPage; m++ {
			slots[m] = fmt.Sprintf("%d 0 R", elemBase+p*benchMCIDsPerPage+m)
			fmt.Fprintf(&content, "/P <</MCID %d>> BDC BT /F1 12 Tf 72 %d Td (Line) Tj ET EMC\n", m, 700-m*20)
		}
		nums = append(nums, fmt.Sprintf("%d [%s]", p, strings.Join(slots, " ")))
		res := ""
		if p%benchImageEvery == 0 {
			img := imageBase + p/benchImageEvery
			res = fmt.Sprintf("/Resources << /XObject << /Im0 %d 0 R >> >>", img)
			content.WriteString("q 10 0 0 10 0 0 cm /Im0 Do Q\n")
			objs = append(objs, rawObj{img, "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 1 >>\nstream\n\x80\nendstream"})
		}
		objs = append(objs,
			rawObj{pageBase + p, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /StructParents %d %s >>", contentBase+p, p, res)},
			rawObj{contentBase + p, rawStream(content.String())},
		)
	}
	elemRefs := make([]string, nElems)
	for e := 0; e < nElems; e++ {
		elemRefs[e] = fmt.Sprintf("%d 0 R", elemBase+e)
		objs = append(objs, rawObj{elemBase + e, fmt.Sprintf("<< /Type /StructElem /S /P /P 6 0 R /Pg %d 0 R /K %d >>", pageBase+e/benchMCIDsPerPage, e%benchMCIDsPerPage)})
	}
	objs = append(objs,
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 5 0 R >>"},
		rawObj{2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), benchPages)},
		rawObj{5, fmt.Sprintf("<< /Type /StructTreeRoot /K 6 0 R /ParentTree 7 0 R /ParentTreeNextKey %d >>", benchPages)},
		rawObj{6, fmt.Sprintf("<< /Type /StructElem /S /Document /P 5 0 R /K [%s] >>", strings.Join(elemRefs, " "))},
		rawObj{7, fmt.Sprintf("<< /Nums [%s] >>", strings.Join(nums, " "))},
	)
	path := filepath.Join(b.TempDir(), "bench-tagged.pdf")
	if err := os.WriteFile(path, rawPDF(objs...), 0o644); err != nil {
		b.Fatal(err)
	}
	return path
}

// benchWalk opens path in a fresh tab per iteration, untimed, and times run.
func benchWalk(b *testing.B, run func(ins *Inspector) error) {
	path := benchTaggedPDF(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ins := NewInspector()
		if _, err := ins.Open("b", path); err != nil {
			b.Fatalf("Open: %v", err)
		}
		b.StartTimer()
		err := run(ins)
		b.StopTimer()
		_ = ins.Close("b")
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

func BenchmarkGetPageIndex(b *testing.B) {
	benchWalk(b, func(ins *Inspector) error {
		entries, err := ins.GetPageIndex("b")
		if err == nil && len(entries) != benchPages {
			err = fmt.Errorf("got %d page rows, want %d", len(entries), benchPages)
		}
		return err
	})
}

func BenchmarkGetImageIndex(b *testing.B) {
	benchWalk(b, func(ins *Inspector) error {
		entries, err := ins.GetImageIndex("b")
		if err == nil && len(entries) != benchPages/benchImageEvery {
			err = fmt.Errorf("got %d image rows, want %d", len(entries), benchPages/benchImageEvery)
		}
		return err
	})
}

func BenchmarkGetStructure(b *testing.B) {
	benchWalk(b, func(ins *Inspector) error {
		r, err := ins.GetStructure("b")
		if err == nil && r.Counts.ContentMCIDs != benchPages*benchMCIDsPerPage {
			err = fmt.Errorf("got %d content MCIDs, want %d", r.Counts.ContentMCIDs, benchPages*benchMCIDsPerPage)
		}
		return err
	})
}
