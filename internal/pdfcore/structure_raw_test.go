package pdfcore

import (
	"fmt"
	"strings"
)

// In-memory documents for the structure walk. Most of them carry shapes
// pdfcpu's open-time validation rejects (or recurses on), so tests read them
// with openUnvalidated, never through Inspector.Open or writeTempPDF.

// structPagesObjs returns a page tree of n pages. Page k (1-based) is object
// 2+k and its /Contents is the single stream object 9000+k holding contents[k-1]
// (an empty stream when contents is shorter). pageExtra[k-1], when present, is
// added to page k's dictionary.
func structPagesObjs(n int, contents []string, pageExtra []string) []rawObj {
	kids := make([]string, n)
	for k := 1; k <= n; k++ {
		kids[k-1] = fmt.Sprintf("%d 0 R", 2+k)
	}
	objs := []rawObj{{2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n)}}
	for k := 1; k <= n; k++ {
		extra := ""
		if k-1 < len(pageExtra) {
			extra = pageExtra[k-1]
		}
		data := ""
		if k-1 < len(contents) {
			data = contents[k-1]
		}
		objs = append(objs,
			rawObj{2 + k, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R %s >>", 9000+k, extra)},
			rawObj{9000 + k, rawStream(data)},
		)
	}
	return objs
}

// structTreePDF is a one-page document (page object 3, content stream 9001)
// whose catalog points at a /StructTreeRoot object 5 with the given body.
// Element objects start at 10 by convention in the tests.
func structTreePDF(rootBody string, objs ...rawObj) []byte {
	all := []rawObj{
		{1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"},
		{5, rootBody},
	}
	all = append(all, structPagesObjs(1, nil, nil)...)
	all = append(all, objs...)
	return rawPDF(all...)
}

// structChainPDF is a structure tree whose root /K is a chain of depth nested
// /Div elements, objects 10 .. 10+depth-1, each the only kid of the previous.
func structChainPDF(depth int) []byte {
	objs := make([]rawObj, 0, depth)
	for i := 0; i < depth; i++ {
		k := "[]"
		if i < depth-1 {
			k = fmt.Sprintf("[%d 0 R]", 11+i)
		}
		objs = append(objs, rawObj{10 + i, fmt.Sprintf("<< /Type /StructElem /S /Div /K %s >>", k)})
	}
	return structTreePDF("<< /Type /StructTreeRoot /K 10 0 R >>", objs...)
}

// structSharedChildPDF is a structure tree whose root /K lists n /Div elements
// (objects 10 .. 10+n-1) that all name one /Span element (object 5000) as
// their only kid.
func structSharedChildPDF(n int) []byte {
	refs := make([]string, n)
	objs := make([]rawObj, 0, n+1)
	for i := 0; i < n; i++ {
		refs[i] = fmt.Sprintf("%d 0 R", 10+i)
		objs = append(objs, rawObj{10 + i, "<< /Type /StructElem /S /Div /K [5000 0 R] >>"})
	}
	objs = append(objs, rawObj{5000, "<< /Type /StructElem /S /Span >>"})
	return structTreePDF(fmt.Sprintf("<< /Type /StructTreeRoot /K [%s] >>", strings.Join(refs, " ")), objs...)
}

// structSingleBackRefPDF has a single-object /K (not an array) leading back
// to an ancestor: root -> 10 (/Div) -> 11 (/P) -> 10.
func structSingleBackRefPDF() []byte {
	return structTreePDF("<< /Type /StructTreeRoot /K 10 0 R >>",
		rawObj{10, "<< /Type /StructElem /S /Div /K 11 0 R >>"},
		rawObj{11, "<< /Type /StructElem /S /P /K 10 0 R >>"},
	)
}

// structRejectedKidsPDF has one /Div element (object 10, on page 1) whose /K
// array holds, in order: a string, a name, a real, a dangling ref (99 0 R), a
// dict with /Type /Foo and no /S, an MCR with no /MCID, a negative integer, a
// direct null, the MCID 0, and a /P element (object 11) with MCID 1.
func structRejectedKidsPDF() []byte {
	return structTreePDF("<< /Type /StructTreeRoot /K 10 0 R >>",
		rawObj{10, "<< /Type /StructElem /S /Div /Pg 3 0 R /K [(str) /Nm 1.5 99 0 R << /Type /Foo >> << /Type /MCR /Pg 3 0 R >> -3 null 0 11 0 R] >>"},
		rawObj{11, "<< /Type /StructElem /S /P /Pg 3 0 R /K 1 >>"},
	)
}

// structRoleMapPDF has a /RoleMap with a cycle (/A -> /B -> /A) and a plain
// mapping (/Heading1 -> /H1); element 10 has /S /A, element 11 /S /Heading1,
// element 12 an empty /S name, element 13 no /S at all.
func structRoleMapPDF() []byte {
	return structTreePDF("<< /Type /StructTreeRoot /RoleMap << /Heading1 /H1 /B /A /A /B >> /K [10 0 R 11 0 R 12 0 R 13 0 R] >>",
		rawObj{10, "<< /Type /StructElem /S /A >>"},
		rawObj{11, "<< /Type /StructElem /S /Heading1 >>"},
		rawObj{12, "<< /Type /StructElem /S / >>"},
		rawObj{13, "<< /Type /StructElem /T (untitled) >>"},
	)
}

// structDirectElementPDF has a direct-dictionary /P element on page 1 that
// claims MCID 0, a ParentTree whose page array slot 0 is null, and page
// content that marks MCID 0.
func structDirectElementPDF() []byte {
	all := []rawObj{
		{1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"},
		{5, "<< /Type /StructTreeRoot /K [<< /Type /StructElem /S /P /Pg 3 0 R /K 0 >>] /ParentTree 6 0 R >>"},
		{6, "<< /Nums [0 [null]] >>"},
	}
	all = append(all, structPagesObjs(1, []string{"/P <</MCID 0>> BDC (x) Tj EMC"}, []string{"/StructParents 0"})...)
	return rawPDF(all...)
}

// markedPDF is a one-page document with no structure tree. The page /Contents
// is an array of the given stream object bodies (objects 20, 21, ...), and
// resources, when not empty, is the page /Resources entry. extra objects are
// added as given.
func markedPDF(resources string, streams []string, extra ...rawObj) []byte {
	refs := make([]string, len(streams))
	all := []rawObj{{1, rawCatalog}, {2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"}}
	for i, s := range streams {
		refs[i] = fmt.Sprintf("%d 0 R", 20+i)
		all = append(all, rawObj{20 + i, s})
	}
	res := ""
	if resources != "" {
		res = "/Resources " + resources
	}
	all = append(all, rawObj{3, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents [%s] %s >>", strings.Join(refs, " "), res)})
	all = append(all, extra...)
	return rawPDF(all...)
}

// undecodableStream is a stream object body whose /FlateDecode data is not
// zlib, so decoding it fails.
const undecodableStream = "<< /Length 9 /Filter /FlateDecode >>\nstream\nnot-zlib!\nendstream"

// numberTreePDF is a document with an empty page tree plus objs, for reading
// number trees directly.
func numberTreePDF(objs ...rawObj) []byte {
	all := []rawObj{{1, rawCatalog}, {2, "<< /Type /Pages /Kids [] /Count 0 >>"}}
	return rawPDF(append(all, objs...)...)
}
