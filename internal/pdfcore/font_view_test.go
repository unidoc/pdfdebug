package pdfcore

import (
	"errors"
	"testing"
)

// TestGetFontView_FontDict pins Kind:"detail" + populated Detail / nil Roster
// for a /Type /Font dict (object 4 in fonts-mixed.pdf -- unembedded Helvetica).
func TestGetFontView_FontDict(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	view, err := ins.GetFontView(tabID, "obj:0:4")
	if err != nil {
		t.Fatalf("GetFontView returned error: %v", err)
	}
	if view == nil {
		t.Fatal("view is nil")
	}
	if view.Kind != FontViewKindDetail {
		t.Fatalf("Kind = %q, want %q", view.Kind, FontViewKindDetail)
	}
	if view.Detail == nil {
		t.Fatal("Detail should be populated")
	}
	if view.Roster != nil {
		t.Errorf("Roster should be nil when Kind=detail, got %+v", view.Roster)
	}
	if view.Detail.BaseFont != "/Helvetica" {
		t.Errorf("Detail.BaseFont = %q, want /Helvetica", view.Detail.BaseFont)
	}
}

// TestGetFontView_ResourceMap pins Kind:"roster" + populated Roster /
// nil Detail for the /Resources /Font dict on page 3, with entries sorted.
func TestGetFontView_ResourceMap(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	view, err := ins.GetFontView(tabID, resourceFontNodeID)
	if err != nil {
		t.Fatalf("GetFontView returned error: %v", err)
	}
	if view.Kind != FontViewKindRoster {
		t.Fatalf("Kind = %q, want %q", view.Kind, FontViewKindRoster)
	}
	if view.Detail != nil {
		t.Errorf("Detail should be nil when Kind=roster, got %+v", view.Detail)
	}
	if view.Roster == nil {
		t.Fatal("Roster should be populated")
	}
	if len(view.Roster.Entries) != 4 {
		t.Fatalf("Entries count = %d, want 4 (F1..F4)", len(view.Roster.Entries))
	}
	want := []string{"F1", "F2", "F3", "F4"}
	for i, w := range want {
		if view.Roster.Entries[i].Name != w {
			t.Errorf("Entries[%d].Name = %q, want %q", i, view.Roster.Entries[i].Name, w)
		}
	}
}

// TestGetFontView_NeitherForNonFontDict pins Kind:"neither" with nil Detail /
// nil Roster when the resolved dict is neither a Font dict nor a font roster
// (the catalog at obj:0:1).
func TestGetFontView_NeitherForNonFontDict(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	view, err := ins.GetFontView(tabID, "obj:0:1")
	if err != nil {
		t.Fatalf("GetFontView returned error: %v (must NOT error for non-font dict)", err)
	}
	if view.Kind != FontViewKindNeither {
		t.Fatalf("Kind = %q, want %q", view.Kind, FontViewKindNeither)
	}
	if view.Detail != nil || view.Roster != nil {
		t.Errorf("Detail/Roster should be nil for Kind=neither, got Detail=%+v Roster=%+v",
			view.Detail, view.Roster)
	}
}

// TestGetFontView_EmptyNodeID pins the empty-nodeID short circuit:
// Kind:"neither" with NO error. Real failures only.
func TestGetFontView_EmptyNodeID(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	view, err := ins.GetFontView(tabID, "")
	if err != nil {
		t.Fatalf("GetFontView err = %v, want nil for empty nodeID", err)
	}
	if view.Kind != FontViewKindNeither {
		t.Fatalf("Kind = %q, want %q", view.Kind, FontViewKindNeither)
	}
}

// TestGetFontView_ErrorPrefixNodeID pins the "error:" sentinel short circuit.
func TestGetFontView_ErrorPrefixNodeID(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	view, err := ins.GetFontView(tabID, "error:something")
	if err != nil {
		t.Fatalf("GetFontView err = %v, want nil for error: nodeID", err)
	}
	if view.Kind != FontViewKindNeither {
		t.Fatalf("Kind = %q, want %q", view.Kind, FontViewKindNeither)
	}
}

// TestGetFontView_UnknownTab confirms a missing tab surfaces a REAL Go error
// (ErrDocumentNotFound), not Kind:"neither".
func TestGetFontView_UnknownTab(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetFontView("nope", "obj:0:5")
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("err = %v, want ErrDocumentNotFound", err)
	}
}

// TestGetFontView_NeverReturnsErrNotAFont is a regression guard for the
// primary contract of this refactor: the binding layer never logs
// "ERR Binding call failed: not a font" because GetFontView never wraps
// ErrNotAFont. We sweep every interesting node shape and assert that any
// returned error is NOT ErrNotAFont.
func TestGetFontView_NeverReturnsErrNotAFont(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	cases := []string{
		"",
		"error:malformed",
		"obj:0:1",          // catalog: non-font dict
		"obj:0:4",          // Font dict
		"obj:0:3",          // page dict
		resourceFontNodeID, // resource map
		"obj:0:99",         // nonexistent
	}
	for _, nodeID := range cases {
		_, err := ins.GetFontView(tabID, nodeID)
		if errors.Is(err, ErrNotAFont) {
			t.Errorf("nodeID %q: err is ErrNotAFont, must never be (frontend cannot disambiguate)", nodeID)
		}
	}
}
