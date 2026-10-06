package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// imagesIndexForDump has an image carrying every badge with a crafted name in
// its colour space, an image whose facts read failed, and an error row.
func imagesIndexForDump() []*pdfcore.ImageIndexEntry {
	smask := "7 0 R"
	transform := 2
	return []*pdfcore.ImageIndexEntry{
		{ObjNum: 5, NodeID: "obj:0:5", Width: 8, Height: 1, BitsPerComponent: 1, ColorSpace: "Dev\tGray",
			Filters: []string{"FlateDecode", "DCTDecode"}, ImageMask: true, SMask: &smask, DecodeNonDefault: true,
			AdobeMarker: pdfcore.AdobeMarkerPresent, AdobeTransform: &transform, EstimatedBytes: 1,
			FirstPage: 1, PageCount: 17, FirstPages: []int{1, 2, 3}, Warning: "Width metadata: bad"},
		{ObjNum: 6, Gen: 1, NodeID: "obj:1:6", Width: 2, Height: 2, BitsPerComponent: 8, Filters: []string{},
			FirstPage: 2, PageCount: 1, FirstPages: []int{2}, Warning: "Width metadata: bad", Err: "image dictionary could not be read: boom"},
		{Filters: []string{}, FirstPages: []int{}, Err: "image walk stopped"},
	}
}

func TestImagesDumpPlainRows(t *testing.T) {
	var out bytes.Buffer
	if err := printImagesPlain(&out, imagesIndexForDump()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want header plus 3 rows:\n%s", len(lines), out.String())
	}
	for _, want := range []string{"5 0 R", "8x1", `"Dev\tGray"`, "FlateDecode,DCTDecode", "mask,SMask,Decode,APP14 t=2 ", "17: 1,2,3, ...", "warning: Width metadata: bad"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("row 1 missing %q: %q", want, lines[1])
		}
	}
	if strings.Contains(lines[1], "warn ") || strings.Contains(lines[1], ",warn") {
		t.Errorf("a warning is not a FLAGS word in plain output: %q", lines[1])
	}
	for _, want := range []string{"6 1 R", "1: 2", "image dictionary could not be read: boom; warning: Width metadata: bad"} {
		if !strings.Contains(lines[2], want) {
			t.Errorf("row 2 missing %q: %q", want, lines[2])
		}
	}
	if !strings.HasPrefix(lines[3], "-") || !strings.HasSuffix(lines[3], "image walk stopped") {
		t.Errorf("error row %q, want - for the reference and the error last", lines[3])
	}
}

func TestImagesDumpJSONIsTheEntryArray(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := writeImagesDump(&stdout, &stderr, imagesIndexForDump(), docViewFlags{json: true}); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	var entries []pdfcore.ImageIndexEntry
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil || len(entries) != 3 {
		t.Errorf("stdout %q is not the 3-entry array (%v)", stdout.String(), err)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr %q, want nothing", stderr.String())
	}
}

func TestImagesDumpEmptyIndexIsAnEmptyArray(t *testing.T) {
	var stdout, stderr bytes.Buffer
	writeImagesDump(&stdout, &stderr, []*pdfcore.ImageIndexEntry{}, docViewFlags{json: true})
	if got := strings.TrimSpace(stdout.String()); got != "[]" {
		t.Errorf("stdout = %q, want []", got)
	}
}

func TestImagesDumpWriteFailureExitsTwoWithOneErrorObject(t *testing.T) {
	for _, jsonMode := range []bool{true, false} {
		var stderr bytes.Buffer
		if code := writeImagesDump(failingWriter{}, &stderr, imagesIndexForDump(), docViewFlags{json: jsonMode}); code != 2 {
			t.Errorf("json=%v: exit %d, want 2", jsonMode, code)
		}
		lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
		var obj map[string]string
		if len(lines) != 1 || json.Unmarshal([]byte(lines[0]), &obj) != nil || !strings.HasPrefix(obj["error"], "failed to write output") {
			t.Errorf("json=%v: stderr = %q, want one error object", jsonMode, stderr.String())
		}
	}
}

func TestImageFlagsAPP14WithoutATransformAndNoFlags(t *testing.T) {
	marker := &pdfcore.ImageIndexEntry{AdobeMarker: pdfcore.AdobeMarkerPresent}
	if got := strings.Join(imageFlags(marker), ","); got != "APP14" {
		t.Errorf("flags %q, want APP14 alone when the marker carries no transform", got)
	}
	plain := &pdfcore.ImageIndexEntry{AdobeMarker: pdfcore.AdobeMarkerAbsent}
	if got := imageFlags(plain); len(got) != 0 {
		t.Errorf("flags %v, want none for a plain image", got)
	}
}

func TestImagesDumpPlainImageWithNoFlagsFiltersOrColourSpaceShowsDashes(t *testing.T) {
	var out bytes.Buffer
	index := []*pdfcore.ImageIndexEntry{{ObjNum: 9, NodeID: "obj:0:9", Width: 1, Height: 1, BitsPerComponent: 8,
		Filters: []string{}, FirstPage: 1, PageCount: 1, FirstPages: []int{1}}}
	if err := printImagesPlain(&out, index); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want header plus 1 row:\n%s", len(lines), out.String())
	}
	fields := strings.Fields(lines[1])
	want := []string{"9", "0", "R", "1x1", "8", "-", "-", "-", "0", "1:", "1", "-"}
	if strings.Join(fields, " ") != strings.Join(want, " ") {
		t.Errorf("row fields %q, want %q", fields, want)
	}
}
