package pdfcore

import (
	"strings"
	"testing"
)

func TestWalkNumberTree_NodeShapes(t *testing.T) {
	cases := []struct {
		name string
		objs []rawObj
		// problems lists every expected problem as "node label: message part".
		problems []string
		want     map[int]int
	}{
		{"direct kid with /Limits", []rawObj{
			{10, "<< /Kids [<< /Limits [0 1] /Nums [0 100 1 101] >>] >>"},
		}, nil, map[int]int{0: 100, 1: 101}},
		{"direct kid without /Limits", []rawObj{
			{10, "<< /Kids [<< /Nums [0 100] >>] >>"},
		}, []string{"node (direct): non-root node has no /Limits"}, map[int]int{0: 100}},
		{"null kid", []rawObj{
			{10, "<< /Kids [null 11 0 R] >>"},
			{11, "<< /Limits [2 2] /Nums [2 102] >>"},
		}, []string{"node (direct): node is null"}, map[int]int{2: 102}},
		{"/Kids not an array", []rawObj{
			{10, "<< /Kids 5 >>"},
		}, []string{"node 10 0 R: /Kids is an integer, not an array"}, map[int]int{}},
		{"/Kids dangling", []rawObj{
			{10, "<< /Kids 99 0 R >>"},
		}, []string{"node 10 0 R: /Kids is null, not an array"}, map[int]int{}},
		{"/Kids and /Nums indirect", []rawObj{
			{10, "<< /Kids 11 0 R >>"},
			{11, "[12 0 R]"},
			{12, "<< /Limits [0 0] /Nums 13 0 R >>"},
			{13, "[0 100]"},
		}, nil, map[int]int{0: 100}},
		{"/Nums not an array", []rawObj{
			{10, "<< /Nums (x) >>"},
		}, []string{"node 10 0 R: /Nums is a string, not an array"}, map[int]int{}},
		{"/Limits of one entry", []rawObj{
			{10, "<< /Kids [11 0 R] >>"},
			{11, "<< /Limits [0] /Nums [0 100] >>"},
		}, []string{"node 11 0 R: /Limits is not an array of two integers"}, map[int]int{0: 100}},
		{"/Limits not an array", []rawObj{
			{10, "<< /Kids [11 0 R] >>"},
			{11, "<< /Limits 3 /Nums [0 100] >>"},
		}, []string{"node 11 0 R: /Limits is not an array of two integers"}, map[int]int{0: 100}},
		{"/Limits of non-integers", []rawObj{
			{10, "<< /Kids [11 0 R] >>"},
			{11, "<< /Limits [(a) 1] /Nums [0 100] >>"},
		}, []string{"node 11 0 R: /Limits is not an array of two integers"}, map[int]int{0: 100}},
		{"keys on both /Limits bounds", []rawObj{
			{10, "<< /Kids [11 0 R] >>"},
			{11, "<< /Limits [3 5] /Nums [3 103 5 105] >>"},
		}, nil, map[int]int{3: 103, 5: 105}},
		{"root /Limits ignored", []rawObj{
			{10, "<< /Limits [9 1] /Nums [0 100] >>"},
		}, nil, map[int]int{0: 100}},
		{"node with both /Kids and /Nums", []rawObj{
			{10, "<< /Kids [11 0 R] /Nums [0 100] >>"},
			{11, "<< /Limits [1 1] /Nums [1 101] >>"},
		}, nil, map[int]int{0: 100, 1: 101}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries, order, problems := walkRawNumberTree(t, tc.objs...)
			if len(problems) != len(tc.problems) {
				t.Errorf("problems:\n%s\nwant %d: %v", problemsText(problems), len(tc.problems), tc.problems)
			}
			for _, want := range tc.problems {
				found := false
				for _, p := range problems {
					if strings.Contains(problemText(p), want) {
						found = true
					}
				}
				if !found {
					t.Errorf("no problem matching %q in:\n%s", want, problemsText(problems))
				}
			}
			checkIntEntries(t, entries, tc.want)
			if len(order) != len(tc.want) {
				t.Errorf("order = %v, want %d keys", order, len(tc.want))
			}
		})
	}
}
