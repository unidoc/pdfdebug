package pdfcore

import (
	"errors"
	"testing"
)

// resourceFontNodeID is the tree-node ID of the /Resources /Font dict on
// page 3 of fonts-mixed.pdf. The dict is inline: dict-keyed under the
// inline /Resources dict, which is itself dict-keyed under obj:0:3.
const resourceFontNodeID = "dict:dict:obj:0:3:Resources:Font"

func TestGetFontResourceMap_HappyPath(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	rmap, err := ins.GetFontResourceMap(tabID, resourceFontNodeID)
	if err != nil {
		t.Fatalf("GetFontResourceMap returned error: %v", err)
	}
	if rmap == nil {
		t.Fatal("rmap is nil")
	}
	if len(rmap.Entries) != 4 {
		t.Fatalf("Entries count = %d, want 4 (F1..F4)", len(rmap.Entries))
	}
	wantNames := []string{"F1", "F2", "F3", "F4"}
	for i, w := range wantNames {
		if rmap.Entries[i].Name != w {
			t.Errorf("Entries[%d].Name = %q, want %q", i, rmap.Entries[i].Name, w)
		}
		if rmap.Entries[i].Unresolved {
			t.Errorf("Entries[%d].Unresolved = true, want false", i)
		}
	}

	f1 := rmap.Entries[0]
	if f1.BaseFont != "/Helvetica" {
		t.Errorf("F1.BaseFont = %q, want /Helvetica", f1.BaseFont)
	}
	if f1.Subtype != "Type1" {
		t.Errorf("F1.Subtype = %q, want Type1", f1.Subtype)
	}
	if f1.EncodingSummary != "/WinAnsiEncoding" {
		t.Errorf("F1.EncodingSummary = %q, want /WinAnsiEncoding", f1.EncodingSummary)
	}
	if f1.NodeID != "obj:0:4" {
		t.Errorf("F1.NodeID = %q, want obj:0:4", f1.NodeID)
	}
	if f1.ObjectRef != "4 0 R" {
		t.Errorf("F1.ObjectRef = %q, want 4 0 R", f1.ObjectRef)
	}
	if f1.Embedded {
		t.Errorf("F1.Embedded = true, want false (no FontDescriptor)")
	}

	// F2 -- object 5: Type1 with /Differences encoding, unembedded.
	f2 := rmap.Entries[1]
	if f2.EncodingSummary != "Differences" {
		t.Errorf("F2.EncodingSummary = %q, want Differences", f2.EncodingSummary)
	}
	if f2.Embedded {
		t.Errorf("F2.Embedded = true, want false (FontDescriptor without FontFile)")
	}

	// F3 -- object 6: Type0 composite with embedded TrueType descendant.
	f3 := rmap.Entries[2]
	if f3.Subtype != "Type0" {
		t.Errorf("F3.Subtype = %q, want Type0", f3.Subtype)
	}
	if !f3.Embedded {
		t.Errorf("F3.Embedded = false, want true (descendant has FontFile2)")
	}

	// F4 -- object 7: TrueType, embedded.
	f4 := rmap.Entries[3]
	if f4.Subtype != "TrueType" {
		t.Errorf("F4.Subtype = %q, want TrueType", f4.Subtype)
	}
	if !f4.Embedded {
		t.Errorf("F4.Embedded = false, want true (FontFile2 on FontDescriptor)")
	}
}

func TestGetFontResourceMap_SortStability(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	rmap, err := ins.GetFontResourceMap(tabID, resourceFontNodeID)
	if err != nil {
		t.Fatalf("GetFontResourceMap returned error: %v", err)
	}
	// Lexicographic order. PDF dict iteration is unspecified, so we re-call
	// and compare to guarantee deterministic emission.
	for i := 0; i < 5; i++ {
		again, err := ins.GetFontResourceMap(tabID, resourceFontNodeID)
		if err != nil {
			t.Fatalf("retry %d errored: %v", i, err)
		}
		for j := range rmap.Entries {
			if again.Entries[j].Name != rmap.Entries[j].Name {
				t.Fatalf("retry %d: entry %d name = %q, want %q",
					i, j, again.Entries[j].Name, rmap.Entries[j].Name)
			}
		}
	}
}

func TestGetFontResourceMap_NonFontDict(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	// The catalog (object 1) is not a font resource map; every value is a
	// non-Font dict or scalar. Expect ErrNotAFont.
	_, err := ins.GetFontResourceMap(tabID, "obj:0:1")
	if !errors.Is(err, ErrNotAFont) {
		t.Fatalf("err = %v, want ErrNotAFont", err)
	}
}

func TestGetFontResourceMap_FontDictItself(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	// Pointing at an actual /Type /Font dict (object 4 -- Helvetica). The
	// entries of obj 4 are not font refs (Type, Subtype, BaseFont, Encoding
	// are all Names), so the call must reject with ErrNotAFont.
	_, err := ins.GetFontResourceMap(tabID, "obj:0:4")
	if !errors.Is(err, ErrNotAFont) {
		t.Fatalf("err = %v, want ErrNotAFont", err)
	}
}

func TestGetFontResourceMap_EmptyNodeID(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	_, err := ins.GetFontResourceMap(tabID, "")
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("err = %v, want ErrDocumentNotFound", err)
	}
}

func TestGetFontResourceMap_ErrorNode(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	_, err := ins.GetFontResourceMap(tabID, "error:something")
	if !errors.Is(err, ErrNotAFont) {
		t.Fatalf("err = %v, want ErrNotAFont", err)
	}
}
