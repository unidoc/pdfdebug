package images_navigator_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

// ---------------------------------------------------------------------------
// `pdfdebug dump images` renders the image index: one entry per image XObject
// object referenced from page resources (directly, inherited, or through nested
// Form XObjects), deduplicated by object reference, with the pages that
// reference it. Plain text by default, the entry array with --json.
// ---------------------------------------------------------------------------

func TestJSONIsTheEntryArrayWithEveryKey(t *testing.T) {
	raw := dumpImagesRaw(t, writeFixture(t, "order.pdf", orderPDF()))
	if len(raw) != 3 {
		t.Fatalf("got %d entries, want 3", len(raw))
	}
	for i, e := range raw {
		for _, k := range imageEntryKeys {
			if _, ok := e[k]; !ok {
				t.Errorf("entry %d is missing key %q", i, k)
			}
		}
		if string(e["filters"]) != "[]" {
			t.Errorf("entry %d of an unfiltered image must carry filters [], got %s", i, string(e["filters"]))
		}
		if string(e["decode"]) != "null" {
			t.Errorf("entry %d has no /Decode, so decode must be null, got %s", i, string(e["decode"]))
		}
	}
}

func TestImageSharedByManyPagesIsOneEntry(t *testing.T) {
	entries, _ := dumpImagesJSON(t, writeFixture(t, "shared.pdf", sharedImagePDF()))
	if got := objNums(entries); !reflect.DeepEqual(got, []int{5, 6}) {
		t.Fatalf("entries are objects %v, want [5 6]: %+v", got, entries)
	}
	logo := entries[0]
	if logo.NodeID != "obj:0:5" || logo.Gen != 0 {
		t.Errorf("node id %q gen %d, want obj:0:5 gen 0", logo.NodeID, logo.Gen)
	}
	if logo.PageCount != sharedPageCount {
		t.Errorf("pageCount %d, want %d", logo.PageCount, sharedPageCount)
	}
	if logo.FirstPage != 1 {
		t.Errorf("firstPage %d, want 1", logo.FirstPage)
	}
	wantFirst := make([]int, 16)
	for i := range wantFirst {
		wantFirst[i] = i + 1
	}
	if !reflect.DeepEqual(logo.FirstPages, wantFirst) {
		t.Errorf("firstPages %v, want the first 16 pages %v", logo.FirstPages, wantFirst)
	}

	second := entries[1]
	if second.FirstPage != 3 || second.PageCount != 1 || !reflect.DeepEqual(second.FirstPages, []int{3}) {
		t.Errorf("object 6 is used on page 3 only, got firstPage %d pageCount %d firstPages %v",
			second.FirstPage, second.PageCount, second.FirstPages)
	}
}

func TestImageListedTwiceOnOnePageCountsThatPageOnce(t *testing.T) {
	entries, _ := dumpImagesJSON(t, writeFixture(t, "shared.pdf", sharedImagePDF()))
	logo := entryFor(t, entries, 5)
	seen := map[int]bool{}
	for _, p := range logo.FirstPages {
		if seen[p] {
			t.Errorf("page %d is listed twice in firstPages %v", p, logo.FirstPages)
		}
		seen[p] = true
	}
	if logo.PageCount != sharedPageCount {
		t.Errorf("page 2 names the image twice; pageCount %d, want %d", logo.PageCount, sharedPageCount)
	}
}

func TestDefaultOrderIsFirstUsePageThenObjectNumber(t *testing.T) {
	entries, _ := dumpImagesJSON(t, writeFixture(t, "order.pdf", orderPDF()))
	if got := objNums(entries); !reflect.DeepEqual(got, []int{9, 7, 8}) {
		t.Fatalf("order %v, want [9 7 8] (page 1 first, then page 2 by object number)", got)
	}
	for i, want := range []int{1, 2, 2} {
		if entries[i].FirstPage != want {
			t.Errorf("object %d firstPage %d, want %d", entries[i].ObjNum, entries[i].FirstPage, want)
		}
	}
}

func TestOrderIsDeterministicAcrossRuns(t *testing.T) {
	path := writeFixture(t, "shared.pdf", sharedImagePDF())
	first, _, _ := runCLI(t, "dump", "images", "--json", path)
	for range 3 {
		again, _, _ := runCLI(t, "dump", "images", "--json", path)
		if again != first {
			t.Fatalf("two runs on the same file differ:\n%s\n%s", first, again)
		}
	}
}

// ---------------------------------------------------------------------------
// The walk: inherited /Resources, nested forms, cycles, the nesting cap, and
// /XObject entries that are not images.
// ---------------------------------------------------------------------------

func TestImageReachedOnlyThroughInheritedResources(t *testing.T) {
	entries, _ := dumpImagesJSON(t, writeFixture(t, "inherited.pdf", inheritedResourcesPDF()))
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.ObjNum != 8 || e.PageCount != 2 || !reflect.DeepEqual(e.FirstPages, []int{1, 2}) {
		t.Errorf("object 8 is inherited by pages 1 and 2 only (page 3 overrides /Resources), got obj %d pageCount %d firstPages %v",
			e.ObjNum, e.PageCount, e.FirstPages)
	}
}

func TestImageTwoFormsDeep(t *testing.T) {
	entries, _ := dumpImagesJSON(t, writeFixture(t, "nested.pdf", nestedFormsPDF()))
	if got := objNums(entries); !reflect.DeepEqual(got, []int{8}) {
		t.Fatalf("entries are objects %v, want only the image [8]; forms are not listed", got)
	}
	e := entries[0]
	if e.PageCount != 2 || !reflect.DeepEqual(e.FirstPages, []int{1, 2}) {
		t.Errorf("object 8 is reached on pages 1 and 2, page 2 both directly and through a form; got pageCount %d firstPages %v",
			e.PageCount, e.FirstPages)
	}
	if e.Err != "" || e.Warning != "" {
		t.Errorf("a well-formed nested image carries no error or warning, got error %q warning %q", e.Err, e.Warning)
	}
}

func TestFormCyclesAreWalkedOnce(t *testing.T) {
	entries, _ := dumpImagesJSON(t, writeFixture(t, "cycle.pdf", formCyclePDF()))
	if got := objNums(entries); !reflect.DeepEqual(got, []int{8}) {
		t.Fatalf("entries are objects %v, want [8]: %+v", got, entries)
	}
	if entries[0].PageCount != 1 {
		t.Errorf("pageCount %d, want 1", entries[0].PageCount)
	}
	if rows := errorRows(entries); len(rows) != 0 {
		t.Errorf("a cycle is not a truncation and writes no error row, got %+v", rows)
	}
}

func TestFormChainPastTheNestingCapWritesAnErrorRow(t *testing.T) {
	entries, _ := dumpImagesJSON(t, writeFixture(t, "deep.pdf", deepFormChainPDF()))
	imgs := images(entries)
	if got := objNums(imgs); !reflect.DeepEqual(got, []int{5}) {
		t.Errorf("images %v, want only [5]; the image past the nesting cap is not reached", got)
	}
	rows := errorRows(entries)
	if len(rows) != 1 {
		t.Fatalf("got %d error rows, want 1 naming the nesting cap: %+v", len(rows), entries)
	}
	if entries[len(entries)-1].NodeID != "" {
		t.Errorf("the error row must come last, got %+v", entries)
	}
	row := rows[0]
	for _, want := range []string{"deeper than 32", "page 1"} {
		if !strings.Contains(row.Err, want) {
			t.Errorf("error row %q must contain %q", row.Err, want)
		}
	}
	if row.FirstPage != 0 || row.PageCount != 0 {
		t.Errorf("an error row has firstPage 0 and pageCount 0, got %d and %d", row.FirstPage, row.PageCount)
	}
}

func TestErrorRowCarriesEmptyListsNotNull(t *testing.T) {
	raw := dumpImagesRaw(t, writeFixture(t, "deep.pdf", deepFormChainPDF()))
	last := raw[len(raw)-1]
	if string(last["nodeId"]) != `""` {
		t.Fatalf("last element is not the error row: nodeId %s", string(last["nodeId"]))
	}
	for _, k := range []string{"filters", "firstPages", "firstPageNodeIds"} {
		if string(last[k]) != "[]" {
			t.Errorf("error row %s must be [], got %s", k, string(last[k]))
		}
	}
}

func TestEntriesThatAreNotImagesAreSkipped(t *testing.T) {
	entries, _ := dumpImagesJSON(t, writeFixture(t, "skipped.pdf", skippedEntriesPDF()))
	if got := objNums(entries); !reflect.DeepEqual(got, []int{10}) {
		t.Errorf("entries are objects %v, want only the image [10]: a dangling reference, a stream with no /Subtype and an indirect /Subtype are skipped", got)
	}
}

func TestNoImagesIsAnEmptyArray(t *testing.T) {
	path := writeFixture(t, "no-images.pdf", noImagesPDF())
	stdout, stderr, code := runCLI(t, "dump", "images", "--json", path)
	if code != 0 {
		t.Fatalf("exit %d, want 0\nstderr: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Errorf("a document with no images must be an empty array, not null; got %q", stdout)
	}
	plain := dumpImagesPlain(t, path)
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "REF") {
		t.Errorf("plain output for no images must be the header line only, got %q", plain)
	}
}

// ---------------------------------------------------------------------------
// Dictionary facts: the badge sources, warnings, and parity with what image
// extraction reports for the same object.
// ---------------------------------------------------------------------------

func TestBadgeFacts(t *testing.T) {
	entries, _ := dumpImagesJSON(t, writeFixture(t, "flags.pdf", flagsPDF(t)))
	if got := objNums(entries); !reflect.DeepEqual(got, []int{5, 6, 8, 9, 10}) {
		t.Fatalf("entries are objects %v, want [5 6 8 9 10]; object 7 is only an /SMask target", got)
	}

	mask := entryFor(t, entries, 5)
	if !mask.ImageMask || mask.ColorSpace != "" || mask.BitsPerComponent != 1 {
		t.Errorf("stencil mask: imageMask %v colorSpace %q bpc %d, want true, \"\", 1", mask.ImageMask, mask.ColorSpace, mask.BitsPerComponent)
	}
	if mask.DecodeNonDefault {
		t.Errorf("a stencil mask with no /Decode is not decodeNonDefault")
	}

	soft := entryFor(t, entries, 6)
	if soft.SMask == nil || *soft.SMask != "7 0 R" {
		t.Errorf("smask %v, want \"7 0 R\"", soft.SMask)
	}

	inv := entryFor(t, entries, 8)
	if !reflect.DeepEqual(inv.Decode, []float64{1, 0}) || !inv.DecodeNonDefault {
		t.Errorf("/Decode [1 0]: decode %v decodeNonDefault %v, want [1 0] and true", inv.Decode, inv.DecodeNonDefault)
	}
	if inv.SampleInterpretation != "Inverted by /Decode" {
		t.Errorf("sampleInterpretation %q, want %q", inv.SampleInterpretation, "Inverted by /Decode")
	}

	ident := entryFor(t, entries, 9)
	if ident.DecodeNonDefault {
		t.Errorf("/Decode [0 1] is the identity for DeviceGray and must not be decodeNonDefault")
	}

	jpg := entryFor(t, entries, 10)
	if jpg.AdobeMarker != "present" || jpg.AdobeTransform == nil || *jpg.AdobeTransform != 1 {
		t.Errorf("APP14: adobeMarker %q adobeTransform %v, want present and 1", jpg.AdobeMarker, jpg.AdobeTransform)
	}
	if !reflect.DeepEqual(jpg.Filters, []string{"DCTDecode"}) {
		t.Errorf("filters %v, want [DCTDecode]", jpg.Filters)
	}
	if jpg.Width != 8 || jpg.Height != 8 || jpg.ColorSpace != "DeviceRGB" || jpg.EstimatedBytes != 192 {
		t.Errorf("geometry %dx%d %s est %d, want 8x8 DeviceRGB 192", jpg.Width, jpg.Height, jpg.ColorSpace, jpg.EstimatedBytes)
	}

	for _, e := range entries {
		if e.Warning != "" || e.Err != "" {
			t.Errorf("object %d: no warning or error expected, got warning %q error %q", e.ObjNum, e.Warning, e.Err)
		}
	}
}

func TestRejectedDecodeArrayIsAWarningNotAnErrorRow(t *testing.T) {
	entries, _ := dumpImagesJSON(t, writeFixture(t, "malformed-decode.pdf", malformedDecodePDF()))
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.NodeID != "obj:0:5" || e.Err != "" {
		t.Errorf("a rejected /Decode keeps the image entry: nodeId %q error %q", e.NodeID, e.Err)
	}
	if e.Decode != nil {
		t.Errorf("a rejected /Decode reads as nil, got %v", e.Decode)
	}
	if !e.DecodeNonDefault {
		t.Errorf("a rejected /Decode is present and not the identity, so decodeNonDefault must be true")
	}
	if !strings.Contains(e.Warning, "decode array metadata") {
		t.Errorf("warning %q must name the rejected /Decode", e.Warning)
	}
	if e.SampleInterpretation != "Unknown: /Decode array unreadable" {
		t.Errorf("sampleInterpretation %q, want %q", e.SampleInterpretation, "Unknown: /Decode array unreadable")
	}
	if e.Width != 2 || e.Height != 2 {
		t.Errorf("geometry %dx%d, want 2x2", e.Width, e.Height)
	}
}

func TestFactsMatchImageExtraction(t *testing.T) {
	paths := map[string]string{"image-xobject.pdf": testdataFile(t, "image-xobject.pdf")}
	for name, content := range allFixtures(t) {
		paths[name] = writeFixture(t, name, content)
	}
	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			entries, _ := dumpImagesJSON(t, path)
			for _, e := range images(entries) {
				ref := fmt.Sprintf("%d %d R", e.ObjNum, e.Gen)
				f := imageFactsFor(t, path, ref)
				got := imageFacts{
					Width:                e.Width,
					Height:               e.Height,
					BitsPerComponent:     e.BitsPerComponent,
					ColorSpace:           e.ColorSpace,
					Filter:               strings.Join(e.Filters, ","),
					ImageMask:            e.ImageMask,
					SMask:                e.SMask,
					Decode:               e.Decode,
					AdobeMarker:          e.AdobeMarker,
					AdobeTransform:       e.AdobeTransform,
					SampleInterpretation: e.SampleInterpretation,
					DecodedBytes:         e.EstimatedBytes,
				}
				if !reflect.DeepEqual(got, f) {
					gj, _ := json.Marshal(got)
					fj, _ := json.Marshal(f)
					t.Errorf("%s: index facts differ from dump image\n index: %s\n image: %s", ref, gj, fj)
				}
			}
		})
	}
}

func TestImageXObjectFixtureIsListed(t *testing.T) {
	entries, _ := dumpImagesJSON(t, testdataFile(t, "image-xobject.pdf"))
	if len(images(entries)) == 0 {
		t.Errorf("testdata/image-xobject.pdf holds an image XObject on a page; the index is empty")
	}
}

// ---------------------------------------------------------------------------
// Plain text.
// ---------------------------------------------------------------------------

func TestPlainTextTable(t *testing.T) {
	stdout := dumpImagesPlain(t, writeFixture(t, "order.pdf", orderPDF()))
	widths := []int{5, 4, 3, 10, 7, 5, 3, 5}
	row := func(cells ...string) string {
		var b strings.Builder
		for i, c := range cells {
			if i < len(widths) {
				fmt.Fprintf(&b, "%-*s  ", widths[i], c)
			} else {
				b.WriteString(c)
			}
		}
		return b.String() + "\n"
	}
	want := row("REF", "SIZE", "BPC", "COLORSPACE", "FILTERS", "FLAGS", "EST", "PAGES", "ERROR") +
		row("9 0 R", "2x2", "8", "DeviceGray", "-", "-", "4", "1: 1", "-") +
		row("7 0 R", "2x2", "8", "DeviceGray", "-", "-", "4", "1: 2", "-") +
		row("8 0 R", "2x2", "8", "DeviceGray", "-", "-", "4", "1: 2", "-")
	if stdout != want {
		t.Errorf("plain output mismatch\n got:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestPlainTextFlags(t *testing.T) {
	rows := parseTable(t, dumpImagesPlain(t, writeFixture(t, "flags.pdf", flagsPDF(t))))
	want := map[string]string{
		"5 0 R":  "mask",
		"6 0 R":  "SMask",
		"8 0 R":  "Decode",
		"9 0 R":  "-",
		"10 0 R": "APP14 t=1",
	}
	for ref, flags := range want {
		if got := rowFor(t, rows, ref)["FLAGS"]; got != flags {
			t.Errorf("%s FLAGS %q, want %q", ref, got, flags)
		}
	}
	if got := rowFor(t, rows, "10 0 R")["FILTERS"]; got != "DCTDecode" {
		t.Errorf("10 0 R FILTERS %q, want DCTDecode", got)
	}
	if got := rowFor(t, rows, "5 0 R")["COLORSPACE"]; got != "-" {
		t.Errorf("a stencil mask has no colour space; COLORSPACE %q, want -", got)
	}
}

func TestPlainTextWarningFallsBackIntoTheErrorColumn(t *testing.T) {
	rows := parseTable(t, dumpImagesPlain(t, writeFixture(t, "malformed-decode.pdf", malformedDecodePDF())))
	r := rowFor(t, rows, "5 0 R")
	if r["FLAGS"] != "Decode" {
		t.Errorf("FLAGS %q, want %q; the warning is shown in ERROR only", r["FLAGS"], "Decode")
	}
	if !strings.HasPrefix(r["ERROR"], "warning: decode array metadata") {
		t.Errorf("ERROR %q must start with %q", r["ERROR"], "warning: decode array metadata")
	}
}

func TestPlainTextPagesColumnIsCapped(t *testing.T) {
	rows := parseTable(t, dumpImagesPlain(t, writeFixture(t, "shared.pdf", sharedImagePDF())))
	if got, want := rowFor(t, rows, "5 0 R")["PAGES"], "20: 1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16, ..."; got != want {
		t.Errorf("PAGES %q, want %q", got, want)
	}
	if got := rowFor(t, rows, "6 0 R")["PAGES"]; got != "1: 3" {
		t.Errorf("PAGES %q, want %q", got, "1: 3")
	}
}

func TestPlainTextErrorRow(t *testing.T) {
	rows := parseTable(t, dumpImagesPlain(t, writeFixture(t, "deep.pdf", deepFormChainPDF())))
	last := rows[len(rows)-1]
	if last["REF"] != "-" {
		t.Errorf("an error row's REF is -, got %q", last["REF"])
	}
	if !strings.Contains(last["ERROR"], "deeper than 32") {
		t.Errorf("error row ERROR %q must name the nesting cap", last["ERROR"])
	}
}

// ---------------------------------------------------------------------------
// Exit codes, stdout purity, usage and help.
// ---------------------------------------------------------------------------

func TestEveryFixtureExitsZeroWithAPureJSONArray(t *testing.T) {
	for name, content := range allFixtures(t) {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, "dump", "images", "--json", writeFixture(t, name, content))
			if code != 0 {
				t.Fatalf("exit %d, want 0 (error rows are findings)\nstderr: %s", code, stderr)
			}
			var arr []json.RawMessage
			if err := json.Unmarshal([]byte(stdout), &arr); err != nil {
				t.Errorf("stdout is not a pure JSON array: %v\nraw: %s", err, stdout)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Errorf("dump images writes nothing to stderr on success, got: %s", stderr)
			}
		})
	}
}

func TestPrettyJSONIsTheSameArray(t *testing.T) {
	path := writeFixture(t, "shared.pdf", sharedImagePDF())
	compact, _ := dumpImagesJSON(t, path)
	stdout, stderr, code := runCLI(t, "dump", "images", "--json", "--pretty", path)
	if code != 0 {
		t.Fatalf("exit %d\nstderr: %s", code, stderr)
	}
	var pretty []imageEntry
	if err := json.Unmarshal([]byte(stdout), &pretty); err != nil {
		t.Fatalf("pretty stdout is not the entry array: %v", err)
	}
	if !reflect.DeepEqual(compact, pretty) {
		t.Errorf("--pretty changed the content")
	}
}

func TestUsageErrors(t *testing.T) {
	path := writeFixture(t, "order.pdf", orderPDF())
	cases := map[string][]string{
		"no file":             {"dump", "images"},
		"two files":           {"dump", "images", path, path},
		"flag after the file": {"dump", "images", path, "--json"},
		"unknown flag":        {"dump", "images", "--ref", "3 0 R", path},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, args...)
			if code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
			if !strings.Contains(stderr, "Usage: pdfdebug dump images") {
				t.Errorf("stderr must carry the dump images usage line, got: %s", stderr)
			}
			if stdout != "" {
				t.Errorf("a usage error writes nothing to stdout, got: %s", stdout)
			}
		})
	}
}

func TestMissingFileMatchesDumpPages(t *testing.T) {
	missing := t.TempDir() + "/does-not-exist.pdf"
	_, pagesErr, pagesCode := runCLI(t, "dump", "pages", missing)
	_, imagesErr, imagesCode := runCLI(t, "dump", "images", missing)
	if imagesCode != pagesCode || imagesCode != 2 {
		t.Errorf("dump images on a missing file exited %d, dump pages exits %d; want 2", imagesCode, pagesCode)
	}
	if strings.TrimSpace(imagesErr) != strings.TrimSpace(pagesErr) {
		t.Errorf("dump images stderr %q, dump pages stderr %q", imagesErr, pagesErr)
	}
	var obj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(imagesErr)), &obj); err != nil || obj["error"] == "" {
		t.Errorf("stderr must be one JSON error object, got %q", imagesErr)
	}
}

func TestHelpListsDumpImagesAndSaysInlineImagesAreNotListed(t *testing.T) {
	_, stderr, _ := runCLI(t, "--help")
	for line := range strings.SplitSeq(stderr, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "dump images [--json] [--pretty] <file>") {
			continue
		}
		desc := strings.TrimSpace(strings.TrimPrefix(trimmed, "dump images [--json] [--pretty] <file>"))
		if !strings.Contains(desc, "BI/ID/EI") {
			t.Errorf("the dump images help line must say inline BI/ID/EI images are not listed: %q", desc)
		}
		if strings.IndexFunc(desc, unicode.IsDigit) >= 0 {
			t.Errorf("the dump images help line must not carry a count: %q", desc)
		}
		return
	}
	t.Errorf("--help has no `dump images [--json] [--pretty] <file>` line:\n%s", stderr)
}

// ---------------------------------------------------------------------------
// Walk rules whose fixtures pdfcpu refuses or rewrites at open (a /PS XObject,
// a direct or non-stream /XObject entry, a non-integer /Width, an Indexed image
// with an explicit /Decode), the lowered walk budget, a facts read that panics,
// the cache, a re-Open, concurrency, node-id resolution of every listed image,
// the grouped-by-page API and the service wrappers are covered in package.
// ---------------------------------------------------------------------------

func TestImageIndexUnitCoverageExists(t *testing.T) {
	runGoTest(t, "^Test.*ImageIndex", "./internal/pdfcore/")
}

func TestImagePagesUnitCoverageExists(t *testing.T) {
	runGoTest(t, "^Test.*GetImagePages", "./internal/pdfcore/")
}

func TestImagePageGroupsUnitCoverageExists(t *testing.T) {
	runGoTest(t, "^Test.*ImagePageGroups", "./internal/pdfcore/")
}

func TestServiceWrapperUnitCoverageExists(t *testing.T) {
	runGoTest(t, "^Test.*(GetImageIndex|GetImagePages|GetImagePageGroups)", "./internal/pdfservice/")
}

func TestCLIWriterUnitCoverageExists(t *testing.T) {
	runGoTest(t, "^Test.*(ImagesDump|DumpImages)", "./cmd/cli/")
}
