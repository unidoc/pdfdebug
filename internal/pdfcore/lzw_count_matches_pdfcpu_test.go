package pdfcore

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/filter"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The LZWDecode stage counter reports exactly the length pdfcpu's own decoder
// produces, including once the 4096-entry dictionary is full.

func lzwSD(raw []byte, earlyChange int) *pdfcpu_types.StreamDict {
	return &pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{{
			Name:        "LZWDecode",
			DecodeParms: pdfcpu_types.Dict{"EarlyChange": pdfcpu_types.Integer(earlyChange)},
		}},
		Raw: raw,
	}
}

// pdfcpuLZWEncode encodes payload with pdfcpu's own LZWDecode encoder.
func pdfcpuLZWEncode(t *testing.T, payload []byte, earlyChange int) []byte {
	t.Helper()
	f, err := filter.NewFilter("LZWDecode", map[string]int{"EarlyChange": earlyChange}, filter.DefaultMaxDecodeBytes)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	r, err := f.Encode(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read encoded: %v", err)
	}
	return out
}

// pdfcpuDecodedLen decodes raw through pdfcpu and returns the output length.
func pdfcpuDecodedLen(t *testing.T, raw []byte, earlyChange int) int64 {
	t.Helper()
	sd := lzwSD(raw, earlyChange)
	if err := sd.DecodeWithLimit(filter.DefaultMaxDecodeBytes); err != nil {
		t.Fatalf("pdfcpu decode: %v", err)
	}
	if len(sd.Content) == 0 {
		t.Fatal("pdfcpu decoded nothing; the fixture is not a usable LZW stream")
	}
	return int64(len(sd.Content))
}

// assertCountEquals checks that the counter accepts exactly n bytes: in bounds
// at a ceiling of n, over it at n-1.
func assertCountEquals(t *testing.T, raw []byte, earlyChange int, n int64) {
	t.Helper()
	if _, err := countStages(lzwSD(raw, earlyChange), n); err != nil {
		t.Errorf("count exceeds pdfcpu's %d decoded bytes: %v", n, err)
	}
	if _, err := countStages(lzwSD(raw, earlyChange), n-1); !errors.Is(err, ErrUnsupportedPDF) {
		t.Errorf("count is below pdfcpu's %d decoded bytes: a ceiling of %d was not exceeded (err %v)", n, n-1, err)
	}
}

func TestCountStages_LZWMatchesPdfcpuOnDictionaryFillingStreams(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	random := make([]byte, 256*1024)
	for i := range random {
		random[i] = byte(rng.IntN(64))
	}
	var text bytes.Buffer
	for i := range 20000 {
		fmt.Fprintf(&text, "row %d col %d value %x\n", i, i%17, i*7919)
	}
	payloads := map[string][]byte{"random 64-symbol": random, "varied text": text.Bytes()}

	for _, ec := range []int{0, 1} {
		for name, payload := range payloads {
			t.Run(fmt.Sprintf("%s EarlyChange %d", name, ec), func(t *testing.T) {
				raw := pdfcpuLZWEncode(t, payload, ec)
				n := pdfcpuDecodedLen(t, raw, ec)
				if n != int64(len(payload)) {
					t.Fatalf("fixture: pdfcpu round trip gave %d bytes, want %d", n, len(payload))
				}
				assertCountEquals(t, raw, ec, n)
			})
		}
	}
}

// fullDictionaryNoClear builds a code stream that fills the dictionary, never
// sends a clear code, and then refers to dictionary codes, the last entry
// (4095) included. The entries saved last are three bytes long, so a decoder
// that drops or mis-sizes an entry once the table is full produces a different
// length. Code widths follow the PDF schedule: 9 bits up to 12, widening one
// code early when oneOff is set. decodedLen is the length the stream expands to.
func fullDictionaryNoClear(oneOff bool) (raw []byte, decodedLen int) {
	const tableSize = 4096
	var out bytes.Buffer
	var acc uint32
	var nAcc uint
	width, hi, overflow := uint(9), uint16(257), uint16(512)
	var entryLen [tableSize]int
	lastLen := 0
	put := func(code uint16) {
		acc = acc<<width | uint32(code)
		nAcc += width
		for nAcc >= 8 {
			out.WriteByte(byte(acc >> (nAcc - 8)))
			nAcc -= 8
		}
	}
	emit := func(code uint16) {
		put(code)
		n := 1
		if code > 257 {
			n = entryLen[code]
		}
		if lastLen > 0 && hi < tableSize {
			entryLen[hi] = lastLen + 1
		}
		decodedLen += n
		lastLen = n
		if hi < tableSize {
			hi++
		}
		ui := hi
		if oneOff {
			ui++
		}
		if ui >= overflow && width < 12 {
			width++
			overflow <<= 1
		}
	}
	for i := range 3800 {
		emit(uint16('a' + i%26))
	}
	// Code 258 expands to two bytes, so every entry saved from here on is three.
	for hi < tableSize {
		emit(258)
	}
	for _, code := range []uint16{4095, 4094, 4000, 300} {
		emit(code)
	}
	emit('z')
	put(257)
	if nAcc > 0 {
		out.WriteByte(byte(acc << (8 - nAcc)))
	}
	return out.Bytes(), decodedLen
}

func TestCountStages_LZWMatchesPdfcpuAfterTheDictionaryFillsWithoutAClear(t *testing.T) {
	for _, ec := range []int{0, 1} {
		t.Run(fmt.Sprintf("EarlyChange %d", ec), func(t *testing.T) {
			raw, want := fullDictionaryNoClear(ec == 1)
			n := pdfcpuDecodedLen(t, raw, ec)
			if n != int64(want) {
				t.Fatalf("fixture: pdfcpu decoded %d bytes, want %d", n, want)
			}
			assertCountEquals(t, raw, ec, n)
		})
	}
}
