package pdfcore

import (
	"fmt"
	"strings"
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Number trees are read without pdfcpu validation: most malformed shapes here
// are rejected at Inspector.Open, and a /Kids cycle makes pdfcpu's own
// validator recurse until the process dies.

const numberTreeWhere = "NT"

// walkRawNumberTree reads objs as a document and walks the number tree rooted
// at object 10.
func walkRawNumberTree(t *testing.T, objs ...rawObj) (map[int]pdfcpu_types.Object, []StructureProblem) {
	t.Helper()
	_, doc := openUnvalidated(t, numberTreePDF(objs...))
	entries, problems := walkNumberTree(doc.PDFContext, *pdfcpu_types.NewIndirectRef(10, 0), numberTreeWhere)
	for _, p := range problems {
		if !strings.Contains(p.Where, numberTreeWhere) {
			t.Errorf("problem location %q does not carry the caller's prefix %q", p.Where, numberTreeWhere)
		}
	}
	return entries, problems
}

// problemText is a problem's location and message on one line.
func problemText(p StructureProblem) string { return p.Where + ": " + p.Message }

func problemsText(ps []StructureProblem) string {
	lines := make([]string, len(ps))
	for i, p := range ps {
		lines[i] = problemText(p)
	}
	return strings.Join(lines, "\n")
}

// checkIntEntries requires entries to hold exactly want, each value an integer.
func checkIntEntries(t *testing.T, entries map[int]pdfcpu_types.Object, want map[int]int) {
	t.Helper()
	if len(entries) != len(want) {
		t.Errorf("got %d entries %v, want %d %v", len(entries), entries, len(want), want)
	}
	for k, v := range want {
		got, ok := entries[k]
		if !ok {
			t.Errorf("key %d missing", k)
			continue
		}
		if n, ok := got.(pdfcpu_types.Integer); !ok || int(n) != v {
			t.Errorf("key %d = %v, want %d", k, got, v)
		}
	}
}

func TestWalkNumberTree_WellFormed(t *testing.T) {
	cases := []struct {
		name string
		objs []rawObj
		want map[int]int
	}{
		{"flat /Nums", []rawObj{
			{10, "<< /Nums [0 100 1 101 2 102] >>"},
		}, map[int]int{0: 100, 1: 101, 2: 102}},
		{"two-level /Kids with /Limits", []rawObj{
			{10, "<< /Kids [11 0 R 12 0 R] >>"},
			{11, "<< /Limits [0 1] /Nums [0 100 1 101] >>"},
			{12, "<< /Limits [2 3] /Nums [2 102 3 103] >>"},
		}, map[int]int{0: 100, 1: 101, 2: 102, 3: 103}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries, problems := walkRawNumberTree(t, tc.objs...)
			if len(problems) != 0 {
				t.Errorf("unexpected problems:\n%s", problemsText(problems))
			}
			checkIntEntries(t, entries, tc.want)
		})
	}
}

func TestWalkNumberTree_ReportsMalformedNodes(t *testing.T) {
	cases := []struct {
		name string
		objs []rawObj
		// node is the ref every problem list must name; key, when set, must be
		// named by the same problem.
		node string
		key  string
		want map[int]int
	}{
		{"odd-length /Nums", []rawObj{
			{10, "<< /Nums [0 100 1] >>"},
		}, "10 0 R", "", map[int]int{0: 100}},
		{"non-integer key", []rawObj{
			{10, "<< /Nums [0 100 (x) 101 2 102] >>"},
		}, "10 0 R", "", map[int]int{0: 100, 2: 102}},
		{"key outside /Limits", []rawObj{
			{10, "<< /Kids [11 0 R] >>"},
			{11, "<< /Limits [0 1] /Nums [0 100 5 105] >>"},
		}, "11 0 R", "5", nil},
		{"/Limits reversed", []rawObj{
			{10, "<< /Kids [11 0 R] >>"},
			{11, "<< /Limits [3 0] /Nums [0 100 3 103] >>"},
		}, "11 0 R", "", nil},
		{"keys out of order in a leaf", []rawObj{
			{10, "<< /Nums [2 102 0 100] >>"},
		}, "10 0 R", "", nil},
		{"node not a dict", []rawObj{
			{10, "<< /Kids [11 0 R 12 0 R] >>"},
			{11, "42"},
			{12, "<< /Limits [0 0] /Nums [0 100] >>"},
		}, "11 0 R", "", map[int]int{0: 100}},
		{"node with neither /Kids nor /Nums", []rawObj{
			{10, "<< /Kids [11 0 R 12 0 R] >>"},
			{11, "<< /Limits [0 1] >>"},
			{12, "<< /Limits [2 2] /Nums [2 102] >>"},
		}, "11 0 R", "", map[int]int{2: 102}},
		{"dangling kid ref", []rawObj{
			{10, "<< /Kids [11 0 R 99 0 R] >>"},
			{11, "<< /Limits [0 1] /Nums [0 100 1 101] >>"},
		}, "99 0 R", "", map[int]int{0: 100, 1: 101}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries, problems := walkRawNumberTree(t, tc.objs...)
			if len(problems) == 0 {
				t.Fatalf("no problem reported")
			}
			found := false
			for _, p := range problems {
				text := problemText(p)
				if strings.Contains(text, tc.node) && (tc.key == "" || strings.Contains(p.Message, tc.key)) {
					found = true
				}
			}
			if !found {
				t.Errorf("no problem names node %s (key %q):\n%s", tc.node, tc.key, problemsText(problems))
			}
			if tc.want != nil {
				checkIntEntries(t, entries, tc.want)
			}
		})
	}
}

func TestWalkNumberTree_DuplicateKeyKeepsFirstValueAndNamesBothNodes(t *testing.T) {
	entries, problems := walkRawNumberTree(t,
		rawObj{10, "<< /Kids [11 0 R 12 0 R] >>"},
		rawObj{11, "<< /Limits [0 1] /Nums [0 100 1 101] >>"},
		rawObj{12, "<< /Limits [1 2] /Nums [1 201 2 202] >>"},
	)
	checkIntEntries(t, entries, map[int]int{0: 100, 1: 101, 2: 202})
	found := false
	for _, p := range problems {
		text := problemText(p)
		if strings.Contains(text, "11 0 R") && strings.Contains(text, "12 0 R") && strings.Contains(p.Message, "1") {
			found = true
		}
	}
	if !found {
		t.Errorf("no problem names key 1 and both nodes 11 0 R and 12 0 R:\n%s", problemsText(problems))
	}
}

func TestWalkNumberTree_KidPointingBackAtRootIsReportedOnce(t *testing.T) {
	entries, problems := walkRawNumberTree(t,
		rawObj{10, "<< /Kids [11 0 R 10 0 R] >>"},
		rawObj{11, "<< /Limits [0 1] /Nums [0 100 1 101] >>"},
	)
	checkIntEntries(t, entries, map[int]int{0: 100, 1: 101})
	n := 0
	for _, p := range problems {
		if strings.Contains(p.Message, "reached twice") {
			n++
			if !strings.Contains(problemText(p), "10 0 R") {
				t.Errorf("the reached-twice problem does not name 10 0 R: %s", problemText(p))
			}
		}
	}
	if n != 1 {
		t.Errorf("got %d \"reached twice\" problems, want 1:\n%s", n, problemsText(problems))
	}
}

func TestWalkNumberTree_DepthPastCapIsReported(t *testing.T) {
	const levels = 65
	objs := make([]rawObj, 0, levels)
	for i := 0; i < levels; i++ {
		num := 10 + i
		switch {
		case i == 0:
			objs = append(objs, rawObj{num, fmt.Sprintf("<< /Kids [%d 0 R] >>", num+1)})
		case i < levels-1:
			objs = append(objs, rawObj{num, fmt.Sprintf("<< /Limits [0 0] /Kids [%d 0 R] >>", num+1)})
		default:
			objs = append(objs, rawObj{num, "<< /Limits [0 0] /Nums [0 100] >>"})
		}
	}
	_, problems := walkRawNumberTree(t, objs...)
	found := false
	for _, p := range problems {
		if strings.Contains(p.Message, "64") && strings.Contains(p.Message, "deep") {
			found = true
		}
	}
	if !found {
		t.Errorf("no depth problem naming the 64-level cap:\n%s", problemsText(problems))
	}
}

func TestWalkNumberTree_ReturnsValuesAsStored(t *testing.T) {
	entries, problems := walkRawNumberTree(t,
		rawObj{10, "<< /Nums [0 11 0 R] >>"},
		rawObj{11, "[12 0 R null]"},
		rawObj{12, "<< /S /P >>"},
	)
	if len(problems) != 0 {
		t.Errorf("unexpected problems:\n%s", problemsText(problems))
	}
	ref, ok := entries[0].(pdfcpu_types.IndirectRef)
	if !ok || ref.ObjectNumber != 11 {
		t.Errorf("key 0 = %#v, want the reference 11 0 R as stored", entries[0])
	}
}

func TestWalkNumberTree_RepeatedKeyInOneLeafIsReportedOnceAsDuplicate(t *testing.T) {
	entries, problems := walkRawNumberTree(t, rawObj{10, "<< /Nums [1 101 1 102 3 103] >>"})
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "key 1 already in node") {
		t.Errorf("problems = %q, want one duplicate-key problem", problemsText(problems))
	}
	checkIntEntries(t, entries, map[int]int{1: 101, 3: 103})
}
