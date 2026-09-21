package object_tree_scalar_values_test

import (
	"testing"
)

// PDF text strings render decoded through the shared decoder, on every display
// surface. A UTF-16BE /Alt reads as text wherever it appears.

func TestDecoding_TreeRowShowsTheDecodedText(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	for _, row := range []string{
		"Alt string = " + altText,
		"E string = " + eText,
		"Lang string = " + langText,
	} {
		if !hasLine(out, row) {
			t.Errorf("dump tree has no row %q\n--- output ---\n%s", row, out)
		}
	}
}

func TestDecoding_TreeJSONCarriesTheDecodedText(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	cell := nodeAt(t, root, "/StructTreeRoot", "/K", "[0]")

	if got := mustValue(t, nodeAt(t, cell, "/Alt")); got != altText {
		t.Errorf("/Alt value = %q, want %q", got, altText)
	}
	if got := mustValue(t, nodeAt(t, cell, "/E")); got != eText {
		t.Errorf("/E value = %q, want %q", got, eText)
	}
}

func TestDecoding_HexTextStringInheritsTheUpstreamEscapeQuirk(t *testing.T) {
	// pdfcpu runs escape processing over already-decoded bytes, so the 0x5C in
	// <415C42> is dropped. The decoder's behaviour is inherited whole, and it
	// is pinned here so a future decoder change is a visible decision.
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	node := nodeAt(t, root, "/Scalars", "/Backslash")

	if got := mustValue(t, node); got != backslashText {
		t.Errorf("<%s> value = %q, want %q", backslashHex, got, backslashText)
	}
}

// The raw counterpart is emitted only where decoding actually changed the
// content, so its absence carries information rather than duplicating every
// node.

func TestValueRaw_EmittedWhenDecodingChangedTheContent(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	cell := nodeAt(t, root, "/StructTreeRoot", "/K", "[0]")

	cases := map[string]struct {
		node *treeNodeJSON
		want string
	}{
		"/Alt":       {nodeAt(t, cell, "/Alt"), "<" + altHex + ">"},
		"/E":         {nodeAt(t, cell, "/E"), eLiteral},
		"/Backslash": {nodeAt(t, root, "/Scalars", "/Backslash"), "<" + backslashHex + ">"},
	}
	for key, c := range cases {
		got, ok := valueRaw(c.node)
		if !ok {
			t.Errorf("%s emits no valueRaw although decoding changed the content", key)
			continue
		}
		if got != c.want {
			t.Errorf("%s valueRaw = %q, want the delimited raw form %q", key, got, c.want)
		}
	}
}

func TestValueRaw_OmittedWhenDecodingWasANoOp(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	scalars := nodeAt(t, root, "/Scalars")

	// An ASCII string is the case a rendered-form comparison gets wrong: "(en-US)"
	// is never string-equal to "en-US", so comparing renderings would emit the
	// key on every string in the document.
	for _, key := range []string{"/Lang", "/Nm", "/Int", "/Real", "/Flag", "/Off"} {
		node := nodeAt(t, scalars, key)
		if raw, ok := valueRaw(node); ok {
			t.Errorf("%s emits valueRaw %q although decoding was a no-op", key, raw)
		}
	}
}

// One helper behind every display surface: the same bytes read the same way on
// the tree, the detail view and the diff.

func TestSharedRenderer_AllSurfacesAgreeOnTheSameBytes(t *testing.T) {
	pdf := fixturePath(t, "scalar-values.pdf")

	tree := mustValue(t, nodeAt(t, treeJSON(t, pdf), "/StructTreeRoot", "/K", "[0]", "/Alt"))
	detail := propertyValue(t, objectJSON(t, pdf, "10 0 R"), "/Alt").Display
	diff := diffNodeAt(t, diffJSON(t,
		fixturePath(t, "text-change-a.pdf"),
		fixturePath(t, "text-change-b.pdf"),
	), "/Root/StructTreeRoot/Alt").LeftSummary

	if tree != altText || detail != altText || diff != altText {
		t.Errorf("the surfaces disagree about the same bytes: tree=%q detail=%q diff=%q, want %q on all three",
			tree, detail, diff, altText)
	}
}
