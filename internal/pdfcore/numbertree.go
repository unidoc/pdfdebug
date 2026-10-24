package pdfcore

import (
	"fmt"
	"strings"

	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// maxNumberTreeDepth caps how many nodes one descent path of a number tree may
// hold. A deeper node is reported and not walked.
const maxNumberTreeDepth = 64

// ntTooDeepPrefix starts the problem message for a number-tree node past
// maxNumberTreeDepth; its entries are not read.
const ntTooDeepPrefix = "number tree deeper than "

// ntUnreadablePrefix starts the problem message for a number-tree node whose
// read failed part way; some of its entries may be missing.
const ntUnreadablePrefix = "node could not be read: "

// numberTreeWalker carries the state of one number-tree walk.
type numberTreeWalker struct {
	ctx      *pdfcpu_model.Context
	where    string
	entries  map[int]pdfcpu_types.Object
	keyNode  map[int]string // key -> label of the node that first held it
	visited  map[int]bool   // object numbers of nodes already walked
	problems []StructureProblem
}

// walkNumberTree reads the number tree (ISO 32000-1 7.9.7) rooted at root and
// returns every key with its value as stored (possibly an indirect reference;
// callers dereference) and every malformed node it met. A malformed node is reported and the walk goes on: a node that is
// not a dictionary, has neither /Kids nor /Nums, an odd-length /Nums, a
// non-integer key, a bad /Limits or a key outside it, keys out of order
// within one leaf, a
// duplicate key (the first value is kept), a node reached twice, and a node
// deeper than maxNumberTreeDepth. where prefixes every problem location.
// Callers hold pdfMu.
func walkNumberTree(ctx *pdfcpu_model.Context, root pdfcpu_types.Object, where string) (entries map[int]pdfcpu_types.Object, problems []StructureProblem) {
	w := &numberTreeWalker{
		ctx:     ctx,
		where:   where,
		entries: map[int]pdfcpu_types.Object{},
		keyNode: map[int]string{},
		visited: map[int]bool{},
	}
	w.visit(root, true, 1)
	return w.entries, w.problems
}

// ntNodeLabel names a node for problem locations: its reference, or "(direct)".
func ntNodeLabel(o pdfcpu_types.Object) string {
	if ref, ok := o.(pdfcpu_types.IndirectRef); ok {
		return refString(ref)
	}
	return "(direct)"
}

// problem records msg against the node labelled label.
func (w *numberTreeWalker) problem(label, msg string) {
	w.problems = append(w.problems, StructureProblem{Where: w.where + ", node " + label, Message: msg})
}

// visit reads one node and its descendants.
func (w *numberTreeWalker) visit(node pdfcpu_types.Object, isRoot bool, depth int) {
	label := ntNodeLabel(node)
	if depth > maxNumberTreeDepth {
		w.problem(label, fmt.Sprintf(ntTooDeepPrefix+"%d levels; node not walked", maxNumberTreeDepth))
		return
	}
	if ref, ok := node.(pdfcpu_types.IndirectRef); ok {
		n := ref.ObjectNumber.Value()
		if w.visited[n] {
			w.problem(label, "node reached twice; not walked again")
			return
		}
		w.visited[n] = true
	}
	guard(func() { w.read(node, label, isRoot, depth) }, func(msg string) {
		w.problem(label, ntUnreadablePrefix+msg)
	})
}

// read classifies one node and walks its /Kids or /Nums.
func (w *numberTreeWalker) read(node pdfcpu_types.Object, label string, isRoot bool, depth int) {
	obj := derefIn(w.ctx, node)
	if obj == nil {
		if _, ok := node.(pdfcpu_types.IndirectRef); ok {
			w.problem(label, "node does not resolve to an object")
		} else {
			w.problem(label, "node is null")
		}
		return
	}
	d, ok := obj.(pdfcpu_types.Dict)
	if !ok {
		w.problem(label, fmt.Sprintf("node is %s, not a dictionary", objKind(obj)))
		return
	}

	lo, hi, hasLimits := w.limits(d, label, isRoot)

	kidsObj, hasKids := d.Find("Kids")
	numsObj, hasNums := d.Find("Nums")
	if !hasKids && !hasNums {
		w.problem(label, "node has neither /Kids nor /Nums")
		return
	}
	if hasKids {
		kids, ok := derefIn(w.ctx, kidsObj).(pdfcpu_types.Array)
		if !ok {
			w.problem(label, fmt.Sprintf("/Kids is %s, not an array", objKind(derefIn(w.ctx, kidsObj))))
		} else {
			for _, kid := range kids {
				w.visit(kid, false, depth+1)
			}
		}
	}
	if hasNums {
		nums, ok := derefIn(w.ctx, numsObj).(pdfcpu_types.Array)
		if !ok {
			w.problem(label, fmt.Sprintf("/Nums is %s, not an array", objKind(derefIn(w.ctx, numsObj))))
			return
		}
		w.readNums(nums, label, lo, hi, hasLimits)
	}
}

// limits reads a node's /Limits. On a non-root node it must be two ascending
// integers; on the root it is ignored. ok is false when the node has no usable
// /Limits.
func (w *numberTreeWalker) limits(d pdfcpu_types.Dict, label string, isRoot bool) (lo, hi int, ok bool) {
	if isRoot {
		return 0, 0, false
	}
	obj, found := d.Find("Limits")
	if !found {
		w.problem(label, "non-root node has no /Limits")
		return 0, 0, false
	}
	arr, isArr := derefIn(w.ctx, obj).(pdfcpu_types.Array)
	if !isArr || len(arr) != 2 {
		w.problem(label, "/Limits is not an array of two integers")
		return 0, 0, false
	}
	a, aOK := derefIn(w.ctx, arr[0]).(pdfcpu_types.Integer)
	b, bOK := derefIn(w.ctx, arr[1]).(pdfcpu_types.Integer)
	if !aOK || !bOK {
		w.problem(label, "/Limits is not an array of two integers")
		return 0, 0, false
	}
	if a > b {
		w.problem(label, fmt.Sprintf("/Limits [%d %d] is not ascending", a, b))
		return 0, 0, false
	}
	return int(a), int(b), true
}

// readNums records the key/value pairs of one leaf.
func (w *numberTreeWalker) readNums(nums pdfcpu_types.Array, label string, lo, hi int, hasLimits bool) {
	if len(nums)%2 != 0 {
		w.problem(label, fmt.Sprintf("/Nums has odd length %d; the last entry has no value", len(nums)))
	}
	prev, havePrev := 0, false
	for i := 0; i+1 < len(nums); i += 2 {
		keyObj := derefIn(w.ctx, nums[i])
		k, ok := keyObj.(pdfcpu_types.Integer)
		if !ok {
			w.problem(label, fmt.Sprintf("/Nums entry %d: key is %s, not an integer", i, objKind(keyObj)))
			continue
		}
		key := int(k)
		if hasLimits && (key < lo || key > hi) {
			w.problem(label, fmt.Sprintf("key %d outside /Limits [%d %d]", key, lo, hi))
		}
		// An equal key is reported once, as a duplicate below.
		if havePrev && key < prev {
			w.problem(label, fmt.Sprintf("key %d after key %d: keys not in ascending order", key, prev))
		}
		prev, havePrev = key, true
		if first, dup := w.keyNode[key]; dup {
			w.problem(label, fmt.Sprintf("key %d already in node %s; first value kept", key, first))
			continue
		}
		w.keyNode[key] = label
		w.entries[key] = nums[i+1]
	}
}

// objKind names the PDF type of o for messages, with an article.
func objKind(o pdfcpu_types.Object) string {
	l := typeLabel(o)
	switch {
	case o == nil:
		return l
	case strings.ContainsRune("aeiou", rune(l[0])):
		return "an " + l
	default:
		return "a " + l
	}
}
