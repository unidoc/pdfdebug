package pdfcore

import (
	"strings"
	"testing"
	"unicode/utf8"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// unhandledObject stands in for a pdfcpu object type the walker does not
// classify. No parsed file can produce one, so classifyObject's default arm and
// the renderers' default arms are only reachable from here.
type unhandledObject struct{}

func (unhandledObject) Clone() pdfcpu_types.Object { return unhandledObject{} }
func (unhandledObject) PDFString() string          { return "" }
func (unhandledObject) String() string             { return "" }

// A dictionary entry whose value is null. pdfcpu drops the key at parse time,
// so the walker only ever sees one when a dict is assembled in memory.

func TestDictionaryEntryNullCarriesTheNullToken(t *testing.T) {
	node := buildTreeNode("dict:root:Nothing", "/Nothing", "Nothing", nil, false)

	if node.NodeType != "scalar" || node.ValueType != "null" {
		t.Fatalf("nodeType/valueType = %q/%q, want %q/%q",
			node.NodeType, node.ValueType, "scalar", "null")
	}
	if node.Value != "null" {
		t.Errorf("value = %q, want %q", node.Value, "null")
	}
	if node.ValueRaw != "" {
		t.Errorf("valueRaw = %q, want it omitted: null is not a string and nothing was decoded", node.ValueRaw)
	}
}

// A string whose decode is empty keeps its delimited stored form as the display
// value, so a raw counterpart would only repeat it.

func TestBomOnlyStringCarriesNoDuplicateRawCounterpart(t *testing.T) {
	node := buildTreeNode("dict:root:BomOnly", "/BomOnly", "BomOnly", pdfcpu_types.HexLiteral("FEFF"), false)

	if node.Value != "<FEFF>" {
		t.Errorf("value = %q, want the delimited stored form %q", node.Value, "<FEFF>")
	}
	if node.ValueRaw != "" {
		t.Errorf("valueRaw = %q, want it omitted: it repeats the value and says nothing extra", node.ValueRaw)
	}
}

// An object type outside classifyObject's vocabulary. It is classified as a
// scalar with an empty valueType, and its value surfaces visibly rather than as
// a silently missing key.

func TestUnhandledObjectTypeIsAVisibleScalar(t *testing.T) {
	nodeType, valueType, hasChildren, childCount := classifyObject(unhandledObject{})

	if nodeType != "scalar" {
		t.Errorf("nodeType = %q, want %q", nodeType, "scalar")
	}
	if valueType != "" {
		t.Errorf("valueType = %q, want the empty classification", valueType)
	}
	if hasChildren || childCount != 0 {
		t.Errorf("hasChildren/childCount = %v/%d, want false/0", hasChildren, childCount)
	}

	node := buildTreeNode("dict:root:Odd", "/Odd", "Odd", unhandledObject{}, false)
	if node.Value != "Unknown" {
		t.Errorf("value = %q, want %q so an unhandled type is not a missing key", node.Value, "Unknown")
	}
	if entry := valueEntryFromObject(unhandledObject{}, false); entry.Type != "unknown" || entry.Display != "Unknown" {
		t.Errorf("detail entry = %q/%q, want %q/%q", entry.Type, entry.Display, "unknown", "Unknown")
	}
}

// The two renderings of the same object, and the rules that keep them apart.

func TestScalarTextDecodesStringsAndLeavesOtherScalarsAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		obj  pdfcpu_types.Object
		want string
	}{
		{"utf16be hex", pdfcpu_types.HexLiteral("FEFF00410042"), "AB"},
		{"ascii literal", pdfcpu_types.StringLiteral("en-US"), "en-US"},
		{"empty literal keeps its delimiters", pdfcpu_types.StringLiteral(""), "()"},
		{"empty hex keeps its delimiters", pdfcpu_types.HexLiteral(""), "<>"},
		{"bom only keeps its stored form", pdfcpu_types.HexLiteral("FEFF"), "<FEFF>"},
		{"name", pdfcpu_types.Name("Table"), "/Table"},
		{"integer", pdfcpu_types.Integer(2), "2"},
		{"boolean", pdfcpu_types.Boolean(true), "true"},
		{"null", nil, "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := scalarText(tc.obj); got != tc.want {
				t.Errorf("scalarText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeChangedContentComparesContentNotRenderedForm(t *testing.T) {
	for _, tc := range []struct {
		name string
		obj  pdfcpu_types.Object
		want bool
	}{
		{"ascii literal decodes to itself", pdfcpu_types.StringLiteral("en-US"), false},
		{"ascii hex decodes to itself", pdfcpu_types.HexLiteral("4142"), false},
		{"utf16be hex changes the bytes", pdfcpu_types.HexLiteral("FEFF00410042"), true},
		{"escaped utf16be literal changes the bytes", pdfcpu_types.StringLiteral(`\376\377\000A`), true},
		{"name is not a string", pdfcpu_types.Name("Table"), false},
		{"integer is not a string", pdfcpu_types.Integer(2), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeChangedContent(tc.obj); got != tc.want {
				t.Errorf("decodeChangedContent = %v, want %v", got, tc.want)
			}
		})
	}
}

// The carve-out is decided by key name plus context, so the same key decodes in
// one dictionary and not in another.

func TestBinaryStringKeyMatchesOnContext(t *testing.T) {
	sigTyped := pdfcpu_types.Dict{"Type": pdfcpu_types.Name("Sig")}
	sigByteRange := pdfcpu_types.Dict{"ByteRange": pdfcpu_types.Array{}}
	annot := pdfcpu_types.Dict{"Type": pdfcpu_types.Name("Annot")}

	for _, tc := range []struct {
		name   string
		parent pdfcpu_types.Dict
		key    string
		want   bool
	}{
		{"signature contents", sigTyped, "Contents", true},
		{"signature cert", sigTyped, "Cert", true},
		{"signature recognised by byte range", sigByteRange, "Contents", true},
		{"document timestamp contents", pdfcpu_types.Dict{"Type": pdfcpu_types.Name("DocTimeStamp")}, "Contents", true},
		{"byte range wins over an unrecognised type", pdfcpu_types.Dict{
			"Type":      pdfcpu_types.Name("Other"),
			"ByteRange": pdfcpu_types.Array{},
		}, "Contents", true},
		{"annotation contents", annot, "Contents", false},
		{"page contents", pdfcpu_types.Dict{"Type": pdfcpu_types.Name("Page")}, "Contents", false},
		{"checksum matches on the name alone", pdfcpu_types.Dict{}, "CheckSum", true},
		{"ordinary key", sigTyped, "SubFilter", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := binaryStringKey(tc.parent, tc.key); got != tc.want {
				t.Errorf("binaryStringKey(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestBinaryStringSummaryReportsThePayloadLength(t *testing.T) {
	if got := binaryStringSummary(pdfcpu_types.HexLiteral(strings.Repeat("AB", 3072))); got != "<binary, 3072 bytes>" {
		t.Errorf("hex summary = %q, want %q", got, "<binary, 3072 bytes>")
	}
	if got := binaryStringSummary(pdfcpu_types.StringLiteral(`ab\000c`)); got != "<binary, 4 bytes>" {
		t.Errorf("literal summary = %q, want %q", got, "<binary, 4 bytes>")
	}
}

// Escaping and the cap, which the plain-text presenters rely on to keep one
// node on exactly one line.

func TestEscapeDisplayValueCoversEveryEscapedClass(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"newline", "a\nb", `a\nb`},
		{"carriage return", "a\rb", `a\rb`},
		{"tab", "a\tb", `a\tb`},
		{"backslash", `a\b`, `a\\b`},
		{"other c0", "a\x00b", `a\x00b`},
		{"del", "a\x7fb", `a\x7Fb`},
		{"c1 from a cp1252 apostrophe", "don\u0092t", `don\x92t`},
		{"printable text is untouched", "Rapport cell", "Rapport cell"},
		{"multi-byte text is untouched", "中文", "中文"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EscapeDisplayValue(tc.in); got != tc.want {
				t.Errorf("EscapeDisplayValue = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClampDisplayValueCutsOnEscapeBoundariesAndNamesBothLengths(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"below the ceiling", strings.Repeat("x", 79), strings.Repeat("x", 79)},
		{"at the ceiling", strings.Repeat("x", 80), strings.Repeat("x", 80)},
		{"above the ceiling", strings.Repeat("x", 81),
			strings.Repeat("x", 80) + " [truncated: 80 of 81]"},
		{"runes not bytes", strings.Repeat("中", 90),
			strings.Repeat("中", 80) + " [truncated: 80 of 90]"},
		{"an escape sequence is never halved", strings.Repeat("\t", 41),
			strings.Repeat(`\t`, 40) + " [truncated: 80 of 82]"},
		{"a cut falling inside an escape drops it whole", "a" + strings.Repeat("\t", 41),
			"a" + strings.Repeat(`\t`, 39) + " [truncated: 79 of 83]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClampDisplayValue(tc.in, TreeValueCap); got != tc.want {
				t.Errorf("ClampDisplayValue =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

// A string holding bytes that are not valid UTF-8. Both renderings reach a
// reader through JSON, where json.Marshal would rewrite those bytes to U+FFFD
// and destroy exactly what the raw counterpart exists to preserve.

func TestInvalidUTF8BytesSurviveAsHex(t *testing.T) {
	obj := pdfcpu_types.StringLiteral("caf\xe9 \x92s")

	value, raw := scalarNodeValue(obj, false)
	if !utf8.ValidString(value) {
		t.Errorf("value = %q, want valid UTF-8", value)
	}
	if !utf8.ValidString(raw) {
		t.Errorf("valueRaw = %q, want valid UTF-8", raw)
	}
	if raw == "" {
		t.Error("valueRaw is empty, want the stored bytes rendered as hex")
	}
	if strings.ContainsRune(raw, '\uFFFD') {
		t.Errorf("valueRaw = %q, want no replacement characters", raw)
	}

	name := pdfcpu_types.Name("caf\xe9")
	if got := scalarText(name); !utf8.ValidString(got) {
		t.Errorf("name value = %q, want valid UTF-8", got)
	}
}

// A string whose stored content cannot be recovered - odd or non-hex digits, a
// malformed escape. The decode fallback drops the delimiters, so without a raw
// counterpart the stored form would be unreachable from the output.

func TestUnrecoverableStringStillCarriesItsRawForm(t *testing.T) {
	for _, tc := range []struct {
		name string
		obj  pdfcpu_types.Object
		want string
	}{
		{"odd hex digit count", pdfcpu_types.HexLiteral("ABC"), "<ABC>"},
		{"non hex digits", pdfcpu_types.HexLiteral("ZZZZ"), "<ZZZZ>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, raw := scalarNodeValue(tc.obj, false); raw != tc.want {
				t.Errorf("valueRaw = %q, want %q", raw, tc.want)
			}
		})
	}
}

// One renderer behind four display surfaces. Each surface also keeps its own
// expectation table, and a table restates what its surface does rather than
// comparing it to the others, so a second copy of the switch can live behind
// three tables that were updated together. These two cases compare the
// surfaces to each other instead: the tree row, the array-element label, the
// detail entry and the diff summary must answer with the same bytes for the
// same object, across the whole scalar vocabulary and not only for a string.

func TestEverySurfaceRendersAScalarTheSameWay(t *testing.T) {
	for _, tc := range []struct {
		name string
		obj  pdfcpu_types.Object
	}{
		{"name", pdfcpu_types.Name("Helvetica")},
		{"ascii literal", pdfcpu_types.StringLiteral("en-US")},
		{"utf16be hex", pdfcpu_types.HexLiteral("FEFF0052006100700070006F00720074002000630065006C006C")},
		{"escaped utf16be literal", pdfcpu_types.StringLiteral(`\376\377\000E\000x\000p`)},
		{"empty literal", pdfcpu_types.StringLiteral("")},
		{"empty hex", pdfcpu_types.HexLiteral("")},
		{"bom only", pdfcpu_types.HexLiteral("FEFF")},
		{"unrecoverable hex", pdfcpu_types.HexLiteral("ZZZZ")},
		{"literal carrying control characters", pdfcpu_types.StringLiteral("a\nb\tc")},
		{"literal over the cap", pdfcpu_types.StringLiteral(strings.Repeat("x", 120))},
		{"integer", pdfcpu_types.Integer(42)},
		{"float", pdfcpu_types.Float(3.14)},
		{"boolean true", pdfcpu_types.Boolean(true)},
		{"boolean false", pdfcpu_types.Boolean(false)},
		{"null", nil},
		{"unhandled type", unhandledObject{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, _ := scalarNodeValue(tc.obj, false)
			entry := valueEntryFromObject(tc.obj, false)

			if entry.Display != value {
				t.Errorf("detail Display = %q, want the tree row's value %q", entry.Display, value)
			}
			if got := diffSummarize(tc.obj); got != value {
				t.Errorf("diff summary = %q, want the tree row's value %q", got, value)
			}
			// An array element carries its value in the label, already escaped
			// and capped the way the plain-text presenter treats a dictionary
			// scalar's value.
			if got, want := semanticLabel("", tc.obj), ClampDisplayValue(value, TreeValueCap); got != want {
				t.Errorf("array element label = %q, want the presented row value %q", got, want)
			}

			raw := scalarRaw(tc.obj)
			if entry.Raw != raw {
				t.Errorf("detail Raw = %q, want the byte-exact rendering %q", entry.Raw, raw)
			}
			if got := diffCompare(tc.obj); got != raw {
				t.Errorf("diff comparison = %q, want the byte-exact rendering %q", got, raw)
			}
		})
	}
}

func TestEverySurfaceSummarizesACarvedOutStringTheSameWay(t *testing.T) {
	for _, tc := range []struct {
		name string
		obj  pdfcpu_types.Object
	}{
		{"hex certificate", pdfcpu_types.HexLiteral(strings.Repeat("AB", 3072))},
		{"literal with escapes", pdfcpu_types.StringLiteral(`ab\000c`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := binaryStringSummary(tc.obj)

			value, raw := scalarNodeValue(tc.obj, true)
			if value != want {
				t.Errorf("tree row value = %q, want %q", value, want)
			}
			if raw != "" {
				t.Errorf("valueRaw = %q, want it omitted: a fixed-width summary has no counterpart to offer", raw)
			}

			element := buildTreeNode("arr:dict:obj:0:12:Cert:0", "[0]", "", tc.obj, true)
			if element.Label != want {
				t.Errorf("array element label = %q, want %q", element.Label, want)
			}
			if element.Value != "" {
				t.Errorf("array element value = %q, want it empty: the value lives in the label", element.Value)
			}

			entry := valueEntryFromObject(tc.obj, true)
			if entry.Display != want {
				t.Errorf("detail Display = %q, want %q", entry.Display, want)
			}
			if got := scalarRaw(tc.obj); entry.Raw != got {
				t.Errorf("detail Raw = %q, want the bytes kept whole: %q", entry.Raw, got)
			}
		})
	}
}
