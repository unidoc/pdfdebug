package object_tree_scalar_values_test

import (
	"strings"
	"testing"
)

// Dictionary-entry scalar leaves carry their value; containers, refs and array
// elements do not. The plain row shape is "<label> <type> = <value>", and the
// JSON key is additive and omitted rather than emitted empty.

func TestTreeRow_DictionaryScalarsPrintTheirValue(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	want := []string{
		"Nm name = /Foo",
		"Int number = 2",
		"Real number = 1.5",
		"Flag boolean = true",
		"Off boolean = false",
		"RowSpan number = 2",
		"ColSpan number = 3",
		"O name = /Table",
		"S name = /TD",
	}
	for _, w := range want {
		if !hasLine(out, w) {
			t.Errorf("dump tree has no row %q\n--- output ---\n%s", w, out)
		}
	}
}

func TestTreeRow_DictionaryScalarValuesInJSON(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	scalars := nodeAt(t, root, "/Scalars")

	cases := map[string]struct{ valueType, value string }{
		"/Nm":   {"name", "/Foo"},
		"/Int":  {"number", "2"},
		"/Real": {"number", "1.5"},
		"/Flag": {"boolean", "true"},
		"/Off":  {"boolean", "false"},
	}
	for key, want := range cases {
		node := nodeAt(t, scalars, key)
		if node.ValueType != want.valueType {
			t.Errorf("%s valueType = %q, want %q", key, node.ValueType, want.valueType)
		}
		if got := mustValue(t, node); got != want.value {
			t.Errorf("%s value = %q, want %q", key, got, want.value)
		}
	}
}

func TestTreeRow_ContainersAndRefsCarryNoValue(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))

	if _, ok := value(root); ok {
		t.Errorf("catalog root carries a value key; containers must not")
	}
	for _, path := range [][]string{
		{"/StructTreeRoot"},                    // ref node: ObjectRef already carries the pointer
		{"/StructTreeRoot", "/K"},              // array
		{"/StructTreeRoot", "/K", "[0]"},       // dict reached through a ref
		{"/StructTreeRoot", "/K", "[0]", "/A"}, // inline dict
		{"/Arrays", "/Nums"},                   // array
	} {
		node := nodeAt(t, root, path...)
		if v, ok := value(node); ok {
			t.Errorf("%v (nodeType %s) carries value %q; only dictionary scalar leaves may",
				path, node.NodeType, v)
		}
	}
}

func TestTreeRow_RefNodeKeepsItsObjectRefAndIsNotDereferenced(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	node := nodeAt(t, root, "/StructTreeRoot")

	if node.PdfRef == "" {
		t.Errorf("/StructTreeRoot lost its pdfRef")
	}
	if v, ok := value(node); ok {
		t.Errorf("/StructTreeRoot was dereferenced to fill a value: %q", v)
	}
}

// Array elements keep their current row shape: the value stays in the label and
// no second copy appears. Their content does change, because the label is
// rendered through the same decoder.

func TestArrayElement_RowShapeIsUnchanged(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	for _, row := range []string{"0 number", "1 number", "612 number"} {
		if !hasLine(out, row) {
			t.Errorf("array element row %q is missing\n--- output ---\n%s", row, out)
		}
	}
	for _, row := range []string{"0 number = 0", "1 number = 1", "612 number = 612"} {
		if hasLine(out, row) {
			t.Errorf("array element row %q gained a second copy of its value", row)
		}
	}
}

func TestArrayElement_CarriesNoValueKeyInJSON(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	elem := nodeAt(t, root, "/Arrays", "/Nums", "[0]")

	if elem.Label != "0" {
		t.Errorf("array element label = %q, want %q", elem.Label, "0")
	}
	if v, ok := value(elem); ok {
		t.Errorf("array element carries value %q; its value already lives in label", v)
	}
}

func TestArrayElement_LabelIsDecoded(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))

	elem := nodeAt(t, root, "/Arrays", "/Strs", "[0]")
	if elem.Label != eText {
		t.Errorf("array string element label = %q, want the decoded %q", elem.Label, eText)
	}

	plain := nodeAt(t, root, "/Arrays", "/Strs", "[1]")
	if plain.Label != "plain" {
		t.Errorf("ASCII array string element label = %q, want %q", plain.Label, "plain")
	}
}

func TestArrayElement_LabelObeysTheCap(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	row := oneLineStartingWith(t, out, strings.Repeat("x", plainTextCap))
	if !strings.Contains(row, truncationMarker(plainTextCap, 81)) {
		t.Errorf("long array element row carries no truncation marker: %q", row)
	}
	if strings.Contains(row, strings.Repeat("x", plainTextCap+1)) {
		t.Errorf("long array element row was not cut at %d runes: %q", plainTextCap, row)
	}
}

// An empty text string renders as its delimited stored form, never as the empty
// string: an omitempty key that vanishes is indistinguishable from a container.

func TestEmptyString_RendersAsItsDelimitedStoredForm(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	scalars := nodeAt(t, root, "/Scalars")

	cases := map[string]string{
		"/Empty":    "()",
		"/HexEmpty": "<>",
		"/BomOnly":  "<FEFF>",
	}
	for key, want := range cases {
		node := nodeAt(t, scalars, key)
		got, ok := value(node)
		if !ok {
			t.Errorf("%s emits no value key; an empty string must still be visible", key)
			continue
		}
		if got != want {
			t.Errorf("%s value = %q, want %q", key, got, want)
		}
	}
}

func TestEmptyString_PlainRowShowsTheDelimitedForm(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	for _, row := range []string{
		"Empty string = ()",
		"HexEmpty string = <>",
		"BomOnly string = <FEFF>",
	} {
		if !hasLine(out, row) {
			t.Errorf("dump tree has no row %q\n--- output ---\n%s", row, out)
		}
	}
	for _, row := range []string{"Empty string", "HexEmpty string", "BomOnly string"} {
		if hasLine(out, row) {
			t.Errorf("row %q fell back to the bare type; an empty string must render its stored form", row)
		}
	}
}

// One dump tree run answers the question the tool used to send a reader to
// another command for: every cell's row and column span.

func TestTaggedTable_SpansAreReadableFromOneRun(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	for _, row := range []string{
		"RowSpan number = 2",
		"ColSpan number = 3",
		"RowSpan number = 1",
		"ColSpan number = 4",
	} {
		if !hasLine(out, row) {
			t.Errorf("dump tree has no row %q; the spans still need a second command\n--- output ---\n%s", row, out)
		}
	}
}

// Exit codes are untouched by any of this: value elision is a view concern.

func TestTreeDump_ExitCodesAreUnchanged(t *testing.T) {
	pdf := fixturePath(t, "scalar-values.pdf")

	if _, _, code := runCLI(t, "dump", "tree", "--depth", "20", pdf); code != 0 {
		t.Errorf("successful walk exited %d, want 0", code)
	}
	if _, _, code := runCLI(t, "dump", "tree", "--json", "--depth", "20", pdf); code != 0 {
		t.Errorf("successful --json walk exited %d, want 0", code)
	}
	if _, _, code := runCLI(t, "dump", "tree", "--depth", "-1", pdf); code != 1 {
		t.Errorf("negative --depth exited %d, want 1", code)
	}
	if _, _, code := runCLI(t, "dump", "tree", "/nonexistent/missing.pdf"); code != 2 {
		t.Errorf("missing file exited %d, want 2", code)
	}
}
