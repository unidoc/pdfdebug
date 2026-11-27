package pdfcore

import (
	"bytes"
	"fmt"
	"math"
)

// maxBERDepth caps BER nesting. The top-level element is at depth 1; segments
// of a constructed string count like any other element.
const maxBERDepth = 64

// berTail locates non-zero bytes after the top-level element. count is 0 when
// the element is followed by nothing or by zero bytes only.
type berTail struct {
	offset int // first byte after the element
	count  int // bytes from offset through the last non-zero byte
}

// normalizeBER reads exactly one top-level BER element from in and returns a
// new buffer holding its DER-form re-encoding, and end, the offset of the
// first byte after the element in in. Bytes after the element do not
// fail the call: zero bytes (the /Contents padding) are ignored, and when any
// is non-zero tail reports where they start and how far they reach. in is
// never written to.
//
// The rewrite turns indefinite lengths into minimal definite lengths,
// shortens non-minimal definite lengths, recomputes every constructed length
// from its normalized content, and flattens constructed universal strings into
// one primitive element. Tags, primitive contents and element order (SET
// contents included) are copied unchanged. "DER" here therefore means definite
// minimal lengths and no constructed strings; SET OF ordering, minimal INTEGER
// encoding, BOOLEAN values and string charsets are not checked.
//
// isBER is true when the element uses any of the forms above. A first,
// check-only pass validates the element and detects those forms without
// allocating; when it finds none the element is returned as a copy with isBER
// false. Otherwise a recording pass stores, in pre-order, the normalized
// content length of every constructed element and the unused-bits octet of
// every flattened BIT STRING, and an emitting pass writes the re-encoding from
// those records. Lengths are bounded by the input (a declared length past the
// end of its enclosing element is a truncation), nesting by maxBERDepth. Each
// output element is at most 4 bytes longer than its input encoding beyond what
// its children add. For DER input the only allocation is the returned copy.
// For BER input peak allocation is the output buffer plus the records, which
// are proportional to the input: an 8-byte length per constructed element
// (each takes at least 2 input bytes) and 1 byte per flattened BIT STRING.
func normalizeBER(in []byte) (der []byte, isBER bool, end int, tail berTail, err error) {
	n := &berNormalizer{in: in}
	end, _, err = n.element(0, len(in), 1)
	if err != nil {
		return nil, false, 0, berTail{}, err
	}
	for i := len(in) - 1; i >= end; i-- {
		if in[i] != 0 {
			tail = berTail{offset: end, count: i - end + 1}
			break
		}
	}
	if !n.ber {
		return bytes.Clone(in[:end]), false, end, tail, nil
	}
	n.pass = berRecord
	_, total, err := n.element(0, len(in), 1)
	if err != nil {
		return nil, false, 0, berTail{}, err
	}
	// The emitting pass writes into a buffer of the final size, using the
	// records from the recording pass.
	n.out = make([]byte, 0, total)
	n.pass = berEmit
	if _, _, err := n.element(0, len(in), 1); err != nil {
		return nil, false, 0, berTail{}, err
	}
	return n.out, true, end, tail, nil
}

// berPass selects what a walk of the input does besides validating it.
type berPass int

const (
	berCheck  berPass = iota // validate and detect BER-only forms
	berRecord                // also record constructed lengths and unused bits
	berEmit                  // write the output from the records
)

// berNormalizer walks the input once for DER (check only) or three times for
// BER (check, record, emit); see normalizeBER.
type berNormalizer struct {
	in         []byte
	lens       []int
	unused     []byte
	nextLen    int
	nextUnused int
	out        []byte
	pass       berPass
	ber        bool
}

type berHeader struct {
	tagEnd      int
	hdrEnd      int
	constructed bool
	indefinite  bool
	length      int
}

// bitState tracks the primitive segments of a flattened BIT STRING.
type bitState struct {
	seen       bool
	lastOff    int
	lastUnused byte
}

// isBERStringTag reports whether a universal tag number names a string type
// that BER allows in constructed form.
func isBERStringTag(t byte) bool {
	switch t {
	case 3, 4, 7, 12, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 30:
		return true
	}
	return false
}

func berTruncated(off, need, limit int) error {
	return fmt.Errorf("BER element at offset %d needs %d bytes, %d left", off, need, limit-off)
}

// berLenSize is the number of octets of the minimal DER length for n.
func berLenSize(n int) int {
	if n < 0x80 {
		return 1
	}
	size := 1
	for ; n > 0; n >>= 8 {
		size++
	}
	return size
}

func appendBERLength(out []byte, n int) []byte {
	if n < 0x80 {
		return append(out, byte(n))
	}
	k := berLenSize(n) - 1
	out = append(out, 0x80|byte(k))
	for i := k - 1; i >= 0; i-- {
		out = append(out, byte(n>>(8*i)))
	}
	return out
}

// header parses the tag and length of the element at off, which must end by
// limit.
func (n *berNormalizer) header(off, limit int) (berHeader, error) {
	var h berHeader
	if off >= limit {
		return h, berTruncated(off, 1, limit)
	}
	b := n.in[off]
	h.constructed = b&0x20 != 0
	p := off + 1
	if b&0x1f == 0x1f {
		// High-tag-number form, restricted to what encoding/asn1 accepts:
		// minimal, at most 5 octets, value at least 31 and within int32.
		var num int64
		for i := 0; ; i++ {
			if i == 5 {
				return h, fmt.Errorf("unsupported tag encoding at offset %d", off)
			}
			if p >= limit {
				return h, berTruncated(off, p-off+1, limit)
			}
			c := n.in[p]
			p++
			if i == 0 && c == 0x80 {
				return h, fmt.Errorf("unsupported tag encoding at offset %d", off)
			}
			num = num<<7 | int64(c&0x7f)
			if c&0x80 == 0 {
				break
			}
		}
		if num < 31 || num > math.MaxInt32 {
			return h, fmt.Errorf("unsupported tag encoding at offset %d", off)
		}
	}
	h.tagEnd = p
	if p >= limit {
		return h, berTruncated(off, p-off+1, limit)
	}
	l := n.in[p]
	p++
	switch {
	case l < 0x80:
		h.length = int(l)
	case l == 0x80:
		if !h.constructed {
			return h, fmt.Errorf("indefinite length on a primitive element at offset %d", off)
		}
		h.indefinite = true
		n.ber = true
	case l == 0xff:
		return h, fmt.Errorf("unsupported length encoding at offset %d", off)
	default:
		k := int(l & 0x7f)
		if k > 4 {
			return h, fmt.Errorf("unsupported length encoding at offset %d", off)
		}
		if p+k > limit {
			return h, berTruncated(off, p+k-off, limit)
		}
		var v uint64
		for _, c := range n.in[p : p+k] {
			v = v<<8 | uint64(c)
		}
		p += k
		if v > uint64(limit-p) {
			return h, berTruncated(off, p-off+int(v), limit)
		}
		h.length = int(v)
		// The long form is minimal only for a value of 128 or more in the
		// fewest octets.
		if k != berLenSize(h.length)-1 {
			n.ber = true
		}
	}
	h.hdrEnd = p
	if !h.indefinite && h.length > limit-p {
		return h, berTruncated(off, p-off+h.length, limit)
	}
	return h, nil
}

// step looks at offset p inside the content of the constructed element at
// off. A definite element's content ends at limit; an indefinite element's
// content ends at its end-of-contents, which must come before limit. done
// reports the end of the content, with end the offset after the element;
// otherwise a child element starts at p.
func (n *berNormalizer) step(off, p, limit int, indefinite bool) (done bool, end int, err error) {
	if p >= limit {
		if indefinite {
			return false, 0, fmt.Errorf("BER element at offset %d has no end-of-contents", off)
		}
		return true, p, nil
	}
	if n.in[p] != 0x00 {
		return false, 0, nil
	}
	if p+1 >= limit {
		return false, 0, berTruncated(p, 2, limit)
	}
	if n.in[p+1] != 0x00 || !indefinite {
		return false, 0, fmt.Errorf("malformed end-of-contents at offset %d", p)
	}
	return true, p + 2, nil
}

// element normalizes the element at off, which must end by limit. It returns
// the offset after the element in the input and the length of its normalized
// encoding.
func (n *berNormalizer) element(off, limit, depth int) (end, outLen int, err error) {
	if off < limit && n.in[off] == 0x00 {
		return 0, 0, fmt.Errorf("malformed end-of-contents at offset %d", off)
	}
	if depth > maxBERDepth {
		return 0, 0, fmt.Errorf("BER nesting deeper than %d at offset %d", maxBERDepth, off)
	}
	h, err := n.header(off, limit)
	if err != nil {
		return 0, 0, err
	}
	tag := n.in[off:h.tagEnd]
	if !h.constructed {
		content := n.in[h.hdrEnd : h.hdrEnd+h.length]
		if n.pass == berEmit {
			n.out = append(n.out, tag...)
			n.out = appendBERLength(n.out, len(content))
			n.out = append(n.out, content...)
		}
		return h.hdrEnd + h.length, len(tag) + berLenSize(len(content)) + len(content), nil
	}

	childLimit := limit
	if !h.indefinite {
		childLimit = h.hdrEnd + h.length
	}
	// A constructed universal string becomes one primitive element. Universal
	// tags under 31 always use the one-octet form here.
	base := tag[0] &^ 0x20
	str := len(tag) == 1 && tag[0]&0xc0 == 0 && isBERStringTag(base)
	bitString := str && base == 3
	if str {
		n.ber = true
	}

	lenIdx, unusedIdx := -1, -1
	switch n.pass {
	case berEmit:
		if str {
			n.out = append(n.out, base)
		} else {
			n.out = append(n.out, tag...)
		}
		n.out = appendBERLength(n.out, n.lens[n.nextLen])
		n.nextLen++
		if bitString {
			n.out = append(n.out, n.unused[n.nextUnused])
			n.nextUnused++
		}
	case berRecord:
		lenIdx = len(n.lens)
		n.lens = append(n.lens, 0)
		if bitString {
			unusedIdx = len(n.unused)
			n.unused = append(n.unused, 0)
		}
	}

	contentLen := 0
	var bs bitState
	if bitString {
		contentLen = 1
	}
	for p := h.hdrEnd; ; {
		done, e, err := n.step(off, p, childLimit, h.indefinite)
		if err != nil {
			return 0, 0, err
		}
		if done {
			end = e
			break
		}
		var l int
		if str {
			e, l, err = n.segment(off, p, childLimit, depth+1, base, &bs)
		} else {
			e, l, err = n.element(p, childLimit, depth+1)
		}
		if err != nil {
			return 0, 0, err
		}
		contentLen += l
		p = e
	}
	if lenIdx >= 0 {
		n.lens[lenIdx] = contentLen
	}
	if unusedIdx >= 0 {
		n.unused[unusedIdx] = bs.lastUnused
	}
	return end, len(tag) + berLenSize(contentLen) + contentLen, nil
}

// segment handles one segment at off of the constructed string at parentOff.
// A BIT STRING or OCTET STRING segment must have the string's own type. A
// character string is encoded as an IMPLICIT OCTET STRING (X.690 8.23.5), so
// its segments are OCTET STRINGs; segments of the character string's own type
// are accepted too. A constructed segment is flattened by the rules of its own
// type. It returns the offset after the segment and the number of content
// bytes it contributes.
func (n *berNormalizer) segment(parentOff, off, limit, depth int, base byte, bs *bitState) (end, contentLen int, err error) {
	if depth > maxBERDepth {
		return 0, 0, fmt.Errorf("BER nesting deeper than %d at offset %d", maxBERDepth, off)
	}
	segBase := n.in[off] &^ 0x20
	if segBase != base && (base == 3 || base == 4 || segBase != 4) {
		return 0, 0, fmt.Errorf("constructed string at offset %d has a segment of another type", parentOff)
	}
	h, err := n.header(off, limit)
	if err != nil {
		return 0, 0, err
	}
	if !h.constructed {
		c := n.in[h.hdrEnd : h.hdrEnd+h.length]
		if base == 3 {
			// Every BIT STRING segment starts with its unused-bits octet (0..7),
			// which must be 0 when the segment has no data bits; only the last
			// segment may leave bits unused.
			if len(c) == 0 || c[0] > 7 || (len(c) == 1 && c[0] != 0) {
				return 0, 0, fmt.Errorf("malformed BIT STRING segment at offset %d", off)
			}
			if bs.seen && bs.lastUnused != 0 {
				return 0, 0, fmt.Errorf("malformed BIT STRING segment at offset %d", bs.lastOff)
			}
			bs.seen, bs.lastOff, bs.lastUnused = true, off, c[0]
			c = c[1:]
		}
		if n.pass == berEmit {
			n.out = append(n.out, c...)
		}
		return h.hdrEnd + h.length, len(c), nil
	}
	childLimit := limit
	if !h.indefinite {
		childLimit = h.hdrEnd + h.length
	}
	for p := h.hdrEnd; ; {
		done, e, err := n.step(off, p, childLimit, h.indefinite)
		if err != nil {
			return 0, 0, err
		}
		if done {
			return e, contentLen, nil
		}
		var l int
		e, l, err = n.segment(off, p, childLimit, depth+1, segBase, bs)
		if err != nil {
			return 0, 0, err
		}
		contentLen += l
		p = e
	}
}
