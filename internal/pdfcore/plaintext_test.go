// Story 9-11: Plain Text view tests.
//
// Tests are named to match the runPdfcoreTest patterns pinned in
// tests/detail-panel-tabs/detail_panel_tabs_test.go.

package pdfcore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// TestGetPlainTextLatin1HeaderAndSize verifies the %PDF- header appears at the
// start of Content and TotalBytes matches the on-disk file size for a
// well-formed minimal PDF. 9.11-INTG-021.
func TestGetPlainTextLatin1HeaderAndSize(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")
	got, err := ins.GetPlainText(tabID)
	if err != nil {
		t.Fatalf("GetPlainText: %v", err)
	}
	if got == nil {
		t.Fatal("GetPlainText returned nil")
	}
	if got.TabID != tabID {
		t.Errorf("TabID = %q, want %q", got.TabID, tabID)
	}
	if !strings.HasPrefix(got.Content, "%PDF-") {
		// Content may contain header bytes followed by binary. Allow any
		// position within the first 16 bytes.
		if !strings.Contains(got.Content[:min(len(got.Content), 16)], "%PDF-") {
			t.Errorf("Content does not start with %%PDF- (first 16 chars: %q)", got.Content[:min(len(got.Content), 16)])
		}
	}

	fi, err := os.Stat(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}
	if got.TotalBytes != fi.Size() {
		t.Errorf("TotalBytes = %d, want %d", got.TotalBytes, fi.Size())
	}
	if got.Truncated {
		t.Errorf("Truncated = true for small fixture, want false")
	}
	if got.CapBytes != plainTextByteCap {
		t.Errorf("CapBytes = %d, want %d", got.CapBytes, plainTextByteCap)
	}
}

// TestGetPlainTextLatin1FullByteRange verifies every byte 0x00..0xFF
// round-trips per AC6: permitted whitespace (\t \n \r) plus 0x20..0xFF except
// 0x7F survive; everything else maps to U+FFFD. The decoder is exercised
// directly because Inspector.Open cannot ingest a raw byte file. 9.11-INTG-022.
func TestGetPlainTextLatin1FullByteRange(t *testing.T) {
	bytes := make([]byte, 256)
	for i := range bytes {
		bytes[i] = byte(i)
	}
	got := latin1Decode(bytes)
	runes := []rune(got)
	if len(runes) != 256 {
		t.Fatalf("decoded rune count = %d, want 256", len(runes))
	}
	for i, r := range runes {
		b := byte(i)
		switch {
		case b == 0x09, b == 0x0A, b == 0x0D:
			if r != rune(b) {
				t.Errorf("byte 0x%02X (whitespace) decoded to U+%04X, want U+%04X", b, r, b)
			}
		case b >= 0x20 && b != 0x7F:
			if r != rune(b) {
				t.Errorf("byte 0x%02X decoded to U+%04X, want U+%04X", b, r, b)
			}
		default:
			if r != '�' {
				t.Errorf("byte 0x%02X (control) decoded to U+%04X, want U+FFFD", b, r)
			}
		}
	}
}

// TestGetPlainTextFormFeedReplaced pins the AC6 form-feed (0x0C) replacement.
// Story mandates this so the gutter cannot acquire surprise pagination
// artifacts. 9.11-INTG-023.
func TestGetPlainTextFormFeedReplaced(t *testing.T) {
	got := latin1Decode([]byte{0x0C})
	if got != "�" {
		t.Errorf("form-feed (0x0C) decoded to %q, want U+FFFD", got)
	}
}

// TestGetPlainTextWhitespaceBytesPreserved verifies \t \n \r survive. CRLF
// remains two characters in the output -- the frontend regex collapses to one
// logical line break. 9.11-INTG-024.
func TestGetPlainTextWhitespaceBytesPreserved(t *testing.T) {
	got := latin1Decode([]byte{0x09, 0x0A, 0x0D})
	if got != "\t\n\r" {
		t.Errorf("whitespace decoded to %q, want \"\\t\\n\\r\"", got)
	}
	// CRLF round-trip.
	got = latin1Decode([]byte{'a', 0x0D, 0x0A, 'b'})
	if got != "a\r\nb" {
		t.Errorf("CRLF round-trip = %q, want \"a\\r\\nb\"", got)
	}
}

// makeOversizedPDF writes a real PDF file padded past plainTextByteCap so
// Inspector.Open accepts it. Returns the absolute path of the temp file.
// The caller is responsible for removing it.
func makeOversizedPDF(t *testing.T, totalSize int64) string {
	t.Helper()
	srcPath := filepath.Join(testdataDir(t), "minimal.pdf")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read minimal.pdf: %v", err)
	}

	tmp, err := os.CreateTemp("", "pdfcore-plaintext-oversize-*.pdf")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = tmp.Close() }()

	// Write the real PDF first so pdfcpu can parse it; then pad with arbitrary
	// printable bytes that pdfcpu's parser ignores after %%EOF. pdfcpu doesn't
	// scan past the xref/trailer for opening, so trailing bytes are safe.
	if _, err := tmp.Write(src); err != nil {
		t.Fatalf("write src: %v", err)
	}
	remaining := totalSize - int64(len(src))
	if remaining > 0 {
		pad := make([]byte, 4096)
		for i := range pad {
			pad[i] = 'X'
		}
		for remaining > 0 {
			n := min(int64(len(pad)), remaining)
			if _, err := tmp.Write(pad[:n]); err != nil {
				t.Fatalf("pad: %v", err)
			}
			remaining -= n
		}
	}
	return tmp.Name()
}

// TestGetPlainTextTruncationAtCap verifies that a file larger than
// plainTextByteCap surfaces Truncated=true, Content==capBytes, and
// TotalBytes==actual size. 9.11-INTG-025.
func TestGetPlainTextTruncationAtCap(t *testing.T) {
	path := makeOversizedPDF(t, plainTextByteCap+100)
	defer func() { _ = os.Remove(path) }()

	ins := NewInspector()
	tabID := "tab-oversize"
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("Open oversized PDF: %v", err)
	}
	defer func() { _ = ins.Close(tabID) }()

	got, err := ins.GetPlainText(tabID)
	if err != nil {
		t.Fatalf("GetPlainText: %v", err)
	}
	if !got.Truncated {
		t.Errorf("Truncated = false, want true")
	}
	if got.TotalBytes != plainTextByteCap+100 {
		t.Errorf("TotalBytes = %d, want %d", got.TotalBytes, plainTextByteCap+100)
	}
	if got.CapBytes != plainTextByteCap {
		t.Errorf("CapBytes = %d, want %d", got.CapBytes, plainTextByteCap)
	}
	// Content is at-most CapBytes ASCII-decoded characters (every input byte
	// maps to exactly one output rune via latin1Decode; ASCII pad means rune
	// count == byte count for the prefix).
	if int64(len([]rune(got.Content))) != plainTextByteCap {
		t.Errorf("Content rune count = %d, want %d", len([]rune(got.Content)), plainTextByteCap)
	}
}

// TestGetPlainTextExactCapBoundaryNotTruncated verifies the off-by-one: a file
// exactly equal to plainTextByteCap MUST NOT be flagged truncated.
// 9.11-INTG-026.
func TestGetPlainTextExactCapBoundaryNotTruncated(t *testing.T) {
	path := makeOversizedPDF(t, plainTextByteCap)
	defer func() { _ = os.Remove(path) }()

	ins := NewInspector()
	tabID := "tab-exact-cap"
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = ins.Close(tabID) }()

	got, err := ins.GetPlainText(tabID)
	if err != nil {
		t.Fatalf("GetPlainText: %v", err)
	}
	if got.Truncated {
		t.Errorf("file exactly equal to cap flagged truncated; should not be")
	}
	if got.TotalBytes != plainTextByteCap {
		t.Errorf("TotalBytes = %d, want %d", got.TotalBytes, plainTextByteCap)
	}
}

// TestGetPlainTextNoDecryptOrDecode verifies the decoder feeds a controlled
// byte pattern through unchanged (modulo the AC6 control-byte replacement).
// The backend MUST NOT attempt any decryption or decoding. 9.11-INTG-027.
func TestGetPlainTextNoDecryptOrDecode(t *testing.T) {
	// A simulated encrypted-stream slice: high-byte gibberish surrounded by
	// printable ASCII. None of these are control bytes that AC6 normalizes
	// (we exclude 0x00..0x1F and 0x7F).
	input := []byte{'s', 't', 'r', 'e', 'a', 'm', '\n', 0x80, 0xAB, 0xCD, 0xEF, 0xFF, '\n', 'e', 'n', 'd'}
	got := latin1Decode(input)
	want := "stream\n\u0080\u00ab\u00cd\u00ef\u00ff\nend"
	if got != want {
		t.Errorf("decoded = %q, want %q", got, want)
	}
}

// TestGetPlainTextFileMovedReturnsError verifies an os.IsNotExist-class error
// surfaces when the file is removed post-open. The backend must NOT panic.
// 9.11-INTG-028.
func TestGetPlainTextFileMovedReturnsError(t *testing.T) {
	// Copy minimal.pdf to a temp location so we can delete it without
	// affecting other tests.
	srcPath := filepath.Join(testdataDir(t), "minimal.pdf")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read minimal.pdf: %v", err)
	}
	tmp, err := os.CreateTemp("", "pdfcore-plaintext-moved-*.pdf")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := tmp.Write(src); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = tmp.Close()
	path := tmp.Name()

	ins := NewInspector()
	tabID := "tab-moved"
	if _, err := ins.Open(tabID, path); err != nil {
		_ = os.Remove(path)
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = ins.Close(tabID) }()

	// Move the file away.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, err = ins.GetPlainText(tabID)
	if err == nil {
		t.Fatal("expected error after file removal, got nil")
	}
}

// TestGetPlainTextCacheReturnsSamePointer verifies the cache returns the same
// pointer across calls, dropping it forces a rebuild, and the rebuild has
// equal contents. 9.11-INTG-029.
func TestGetPlainTextCacheReturnsSamePointer(t *testing.T) {
	ins, tabID, doc := openWithFixture(t, "minimal.pdf")

	first, err := ins.GetPlainText(tabID)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := ins.GetPlainText(tabID)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first != second {
		t.Errorf("cache returned different pointers across consecutive calls")
	}

	doc.plainTextMu.Lock()
	doc.plainTextCache = nil
	doc.plainTextMu.Unlock()

	third, err := ins.GetPlainText(tabID)
	if err != nil {
		t.Fatalf("third (post-drop): %v", err)
	}
	if third == second {
		t.Errorf("post-drop call returned same pointer; expected rebuild")
	}
	if !reflect.DeepEqual(third, second) {
		t.Errorf("rebuilt payload not deeply equal to cached one")
	}
}

// TestGetPlainTextConcurrentSharesIO verifies the mutex serializes I/O so
// concurrent callers converge on one cached pointer.
func TestGetPlainTextConcurrentSharesIO(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")
	var wg sync.WaitGroup
	ptrs := make([]*PlainTextDocument, 8)
	wg.Add(len(ptrs))
	for i := range ptrs {
		go func(i int) {
			defer wg.Done()
			pt, err := ins.GetPlainText(tabID)
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
				return
			}
			ptrs[i] = pt
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(ptrs); i++ {
		if ptrs[i] != ptrs[0] {
			t.Errorf("concurrent callers got different pointers")
			break
		}
	}
}

// TestGetPlainTextUnknownTab verifies unknown tabID surfaces an error.
func TestGetPlainTextUnknownTab(t *testing.T) {
	ins := NewInspector()
	if _, err := ins.GetPlainText("no-such-tab"); err == nil {
		t.Errorf("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Story 9-12: GetPlainTextFull -- on-demand uncapped read.
//
// Red phase: these tests fail to compile until Inspector.GetPlainTextFull,
// DocumentState.plainTextFullCache, and DocumentState.plainTextFullMu exist.
// Test names mirror story Task 3 (3.1-3.6).
// ---------------------------------------------------------------------------

// TestGetPlainTextFull_ReturnsAllBytes verifies a file larger than
// plainTextByteCap returns its full content with Truncated=false and
// CapBytes=0. Pins AC14 + AC15 (full payload, no implicit precondition).
// 9.12-INTG-001.
func TestGetPlainTextFull_ReturnsAllBytes(t *testing.T) {
	const overage = 100
	path := makeOversizedPDF(t, plainTextByteCap+overage)
	defer func() { _ = os.Remove(path) }()

	ins := NewInspector()
	tabID := "tab-full-oversize"
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("Open oversized PDF: %v", err)
	}
	defer func() { _ = ins.Close(tabID) }()

	got, err := ins.GetPlainTextFull(tabID)
	if err != nil {
		t.Fatalf("GetPlainTextFull: %v", err)
	}
	if got == nil {
		t.Fatal("GetPlainTextFull returned nil")
	}
	if got.TabID != tabID {
		t.Errorf("TabID = %q, want %q", got.TabID, tabID)
	}
	if got.Truncated {
		t.Errorf("Truncated = true, want false (full payload must never flag truncated)")
	}
	if got.CapBytes != 0 {
		t.Errorf("CapBytes = %d, want 0 (full payload uses zero cap to signal uncapped)", got.CapBytes)
	}
	if got.TotalBytes != plainTextByteCap+overage {
		t.Errorf("TotalBytes = %d, want %d", got.TotalBytes, plainTextByteCap+overage)
	}
	// Every byte maps to exactly one rune via latin1Decode; the pad is ASCII
	// 'X', so rune count must equal byte count.
	if int64(len([]rune(got.Content))) != got.TotalBytes {
		t.Errorf("Content rune count = %d, want %d", len([]rune(got.Content)), got.TotalBytes)
	}
}

// TestGetPlainTextFull_Cached verifies the second call returns the cached
// pointer without re-reading. Pins AC13.
// 9.12-INTG-002.
func TestGetPlainTextFull_Cached(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")

	first, err := ins.GetPlainTextFull(tabID)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := ins.GetPlainTextFull(tabID)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first != second {
		t.Errorf("cache returned different pointers across consecutive calls")
	}
}

// TestGetPlainTextFull_TruncatedAndFullAreIndependent verifies the two cache
// slots (plainTextCache, plainTextFullCache) coexist: a full-payload call does
// NOT replace, evict, or mutate the truncated cache entry, and vice versa.
// Pins AC15 cache-coexistence note + Dev Notes "Why cache truncated and full
// payloads separately".
// 9.12-INTG-003.
func TestGetPlainTextFull_TruncatedAndFullAreIndependent(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")

	truncBefore, err := ins.GetPlainText(tabID)
	if err != nil {
		t.Fatalf("GetPlainText: %v", err)
	}
	full, err := ins.GetPlainTextFull(tabID)
	if err != nil {
		t.Fatalf("GetPlainTextFull: %v", err)
	}
	truncAfter, err := ins.GetPlainText(tabID)
	if err != nil {
		t.Fatalf("GetPlainText (second): %v", err)
	}

	// The truncated-cache pointer must be preserved across the full call.
	if truncBefore != truncAfter {
		t.Errorf("truncated cache pointer changed after GetPlainTextFull; expected coexistence")
	}
	// The two caches are distinct objects: full call must not alias truncated.
	if full == truncBefore {
		t.Errorf("GetPlainTextFull returned the truncated cache pointer; the two slots must be independent")
	}
	// The shared invariants on minimal.pdf (which is well under the cap): both
	// payloads carry the same content and same TotalBytes but different
	// CapBytes (truncated=cap, full=0).
	if truncBefore.TotalBytes != full.TotalBytes {
		t.Errorf("TotalBytes mismatch: truncated=%d full=%d", truncBefore.TotalBytes, full.TotalBytes)
	}
	if truncBefore.CapBytes != plainTextByteCap {
		t.Errorf("truncated CapBytes = %d, want %d", truncBefore.CapBytes, plainTextByteCap)
	}
	if full.CapBytes != 0 {
		t.Errorf("full CapBytes = %d, want 0", full.CapBytes)
	}
}

// TestGetPlainTextFull_UnknownTab verifies the unknown-tab path returns the
// ErrDocumentNotFound sentinel (errors.Is-based, may be wrapped). Pins AC12.
// 9.12-INTG-004.
func TestGetPlainTextFull_UnknownTab(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetPlainTextFull("no-such-tab")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("err = %v, want errors.Is(..., ErrDocumentNotFound)", err)
	}
}

// TestGetPlainTextFull_ConcurrentSharesIO mirrors TestGetPlainTextConcurrentSharesIO:
// 8 concurrent callers must converge on the same cached pointer (proves
// plainTextFullMu serializes the disk read). Pins AC14 "concurrent callers
// share one disk read".
// 9.12-INTG-005.
func TestGetPlainTextFull_ConcurrentSharesIO(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")
	var wg sync.WaitGroup
	ptrs := make([]*PlainTextDocument, 8)
	wg.Add(len(ptrs))
	for i := range ptrs {
		go func(i int) {
			defer wg.Done()
			pt, err := ins.GetPlainTextFull(tabID)
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
				return
			}
			ptrs[i] = pt
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(ptrs); i++ {
		if ptrs[i] != ptrs[0] {
			t.Errorf("concurrent callers got different pointers")
			break
		}
	}
}

// TestGetPlainTextFull_SubCapFile verifies AC15: calling GetPlainTextFull on a
// file smaller than plainTextByteCap returns the full content with
// Truncated=false, CapBytes=0, and content length == TotalBytes. Guards
// against a future regression that adds an implicit "only call this for files
// over the cap" precondition.
// 9.12-INTG-006.
func TestGetPlainTextFull_SubCapFile(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")
	got, err := ins.GetPlainTextFull(tabID)
	if err != nil {
		t.Fatalf("GetPlainTextFull: %v", err)
	}
	if got.Truncated {
		t.Errorf("Truncated = true on sub-cap file, want false")
	}
	if got.CapBytes != 0 {
		t.Errorf("CapBytes = %d, want 0", got.CapBytes)
	}
	if int64(len([]rune(got.Content))) != got.TotalBytes {
		// The fixture is ASCII-only minimal.pdf, so rune count == byte count.
		// If this regresses with a real Latin-1 fixture in the future, switch
		// to a byte-length check on a UTF-8 re-encoding.
		t.Errorf("Content rune count = %d, want %d", len([]rune(got.Content)), got.TotalBytes)
	}
	// Reference-shape sanity: TabID propagated, Content non-empty for a real
	// PDF.
	if got.TabID != tabID {
		t.Errorf("TabID = %q, want %q", got.TabID, tabID)
	}
	if !strings.HasPrefix(got.Content, "%PDF-") {
		first := got.Content
		if len(first) > 16 {
			first = first[:16]
		}
		if !strings.Contains(first, "%PDF-") {
			t.Errorf("Content does not start with %%PDF- (first 16 chars: %q)", first)
		}
	}
}
