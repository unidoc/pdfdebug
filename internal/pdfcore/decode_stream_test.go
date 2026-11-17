package pdfcore

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/filter"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// decodeStream decodes a stream the way pdfcpu does, and keeps the inflated
// bytes of a Flate stage whose only fault is a broken Adler-32 checksum.

// brokenChecksum returns zlib-compressed payload with its trailing Adler-32
// corrupted and the deflate data intact.
func brokenChecksum(t *testing.T, payload []byte) []byte {
	t.Helper()
	raw := zlibBytes(t, payload)
	broken := append([]byte(nil), raw...)
	broken[len(broken)-1] ^= 0xff
	return broken
}

func TestDecodeStream_SoleFlateWithBrokenChecksumDecodes(t *testing.T) {
	payload := bytes.Repeat([]byte("BT /F1 12 Tf (broken checksum) Tj ET\n"), 64)

	good := flateStream(zlibBytes(t, payload))
	if err := decodeStream(good); err != nil {
		t.Fatalf("intact stream: %v", err)
	}

	bad := flateStream(brokenChecksum(t, payload))
	if err := decodeStream(bad); err != nil {
		t.Fatalf("a Flate stream whose only fault is its checksum must decode, got %v", err)
	}
	if !bytes.Equal(bad.Content, good.Content) {
		t.Errorf("broken-checksum decode = %d bytes, want the same %d bytes as the intact stream",
			len(bad.Content), len(good.Content))
	}
}

func TestDecodeStream_BrokenChecksumBehindASCIIHexDecodes(t *testing.T) {
	payload := bytes.Repeat([]byte("hex then flate "), 200)
	raw := []byte(strings.ToUpper(hex.EncodeToString(brokenChecksum(t, payload))) + ">")
	sd := pipeline(raw, "ASCIIHexDecode", "FlateDecode")

	if err := decodeStream(sd); err != nil {
		t.Fatalf("expected the pipeline to decode, got %v", err)
	}
	if !bytes.Equal(sd.Content, payload) {
		t.Errorf("got %d bytes, want the %d-byte payload", len(sd.Content), len(payload))
	}
}

func TestDecodeStream_BadFlateHeaderStillFails(t *testing.T) {
	raw := []byte("this-is-not-valid-flate-data")

	want := flateStream(raw).DecodeWithLimit(filter.DefaultMaxDecodeBytes)
	if want == nil {
		t.Fatal("fixture: pdfcpu decodes a stream with no zlib header")
	}
	err := decodeStream(flateStream(raw))
	if err == nil {
		t.Fatal("a Flate stream with a bad header must still fail")
	}
	if err.Error() != want.Error() {
		t.Errorf("error = %q, want pdfcpu's own error unchanged %q", err, want)
	}
}

func TestDecodeStream_PredictorFlateWithBrokenChecksumFails(t *testing.T) {
	const columns = 4
	var rows []byte
	for i := range 32 {
		rows = append(rows, 0, byte(i), byte(i+1), byte(i+2), byte(i+3))
	}
	sd := flateStream(brokenChecksum(t, rows))
	sd.FilterPipeline[0].DecodeParms = pdfcpu_types.Dict{
		"Predictor": pdfcpu_types.Integer(12),
		"Columns":   pdfcpu_types.Integer(columns),
	}

	if err := decodeStream(sd); err == nil {
		t.Errorf("a predictor Flate stream with a broken checksum yields no bytes from pdfcpu and must fail, got %d bytes", len(sd.Content))
	}
}

func TestDecodeStream_ToleratedDecodeCachesContent(t *testing.T) {
	payload := bytes.Repeat([]byte("cached "), 300)
	sd := flateStream(brokenChecksum(t, payload))

	if err := decodeStream(sd); err != nil {
		t.Fatalf("decodeStream: %v", err)
	}
	if !bytes.Equal(sd.Content, payload) {
		t.Fatalf("sd.Content = %d bytes, want the %d-byte payload", len(sd.Content), len(payload))
	}
	// A later pdfcpu decode on the same StreamDict reads the cached bytes.
	if err := sd.DecodeWithLimit(filter.DefaultMaxDecodeBytes); err != nil {
		t.Errorf("pdfcpu decode after a tolerated decode: %v", err)
	}
	if !bytes.Equal(sd.Content, payload) {
		t.Errorf("sd.Content changed after the pdfcpu decode")
	}
}

func TestDecodeStream_UnfilteredStreamIsItsRawBytes(t *testing.T) {
	raw := []byte("0 0 m 10 10 l S\n")
	sd := &pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{}, Raw: raw}
	if err := decodeStream(sd); err != nil {
		t.Fatalf("decodeStream: %v", err)
	}
	if !bytes.Equal(sd.Content, raw) {
		t.Errorf("sd.Content = %q, want %q", sd.Content, raw)
	}
}

// pdfcpu stops at a DCTDecode stage on a stream that is not 4-component and
// returns what the stages before it produced; the retry stops there too.
func TestDecodeStream_BrokenChecksumBeforeAStoppingDCTDecodes(t *testing.T) {
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte("jpeg body "), 100)...)
	sd := pipeline(brokenChecksum(t, jpeg), "FlateDecode", "DCTDecode")
	sd.CSComponents = 3

	if err := decodeStream(sd); err != nil {
		t.Fatalf("expected the Flate stage's output, got %v", err)
	}
	if !bytes.Equal(sd.Content, jpeg) {
		t.Errorf("got %d bytes, want the %d inflated bytes the DCTDecode stage would receive", len(sd.Content), len(jpeg))
	}
}
