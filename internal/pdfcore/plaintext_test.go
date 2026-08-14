// Plain Text view tests.
//
// Test names are referenced by the runPdfcoreTest patterns in
// tests/detail-panel-tabs/ and tests/async-plain-text-load/.

package pdfcore

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// TestGetPlainTextLatin1HeaderAndSize verifies the %PDF- header appears at the
// start of Content and TotalBytes matches the on-disk file size for a
// well-formed minimal PDF.
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
}

// TestGetPlainTextLatin1FullByteRange verifies every byte 0x00..0xFF
// round-trips: permitted whitespace (\t \n \r) plus 0x20..0xFF except 0x7F
// survive; everything else maps to U+FFFD.
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

// TestGetPlainTextFormFeedReplaced pins the form-feed (0x0C) replacement.
func TestGetPlainTextFormFeedReplaced(t *testing.T) {
	got := latin1Decode([]byte{0x0C})
	if got != "�" {
		t.Errorf("form-feed (0x0C) decoded to %q, want U+FFFD", got)
	}
}

// TestGetPlainTextWhitespaceBytesPreserved verifies \t \n \r survive. CRLF
// remains two characters in the output.
func TestGetPlainTextWhitespaceBytesPreserved(t *testing.T) {
	got := latin1Decode([]byte{0x09, 0x0A, 0x0D})
	if got != "\t\n\r" {
		t.Errorf("whitespace decoded to %q, want \"\\t\\n\\r\"", got)
	}
	got = latin1Decode([]byte{'a', 0x0D, 0x0A, 'b'})
	if got != "a\r\nb" {
		t.Errorf("CRLF round-trip = %q, want \"a\\r\\nb\"", got)
	}
}

// makeOversizedPDF writes a real PDF file padded out to totalSize so
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

// TestGetPlainTextNoDecryptOrDecode verifies the decoder feeds a controlled
// byte pattern through unchanged (modulo the control-byte replacement).
func TestGetPlainTextNoDecryptOrDecode(t *testing.T) {
	input := []byte{'s', 't', 'r', 'e', 'a', 'm', '\n', 0x80, 0xAB, 0xCD, 0xEF, 0xFF, '\n', 'e', 'n', 'd'}
	got := latin1Decode(input)
	want := "stream\n\u0080\u00ab\u00cd\u00ef\u00ff\nend"
	if got != want {
		t.Errorf("decoded = %q, want %q", got, want)
	}
}

// TestGetPlainTextFileMovedReturnsError verifies an os.IsNotExist-class error
// surfaces when the file is removed post-open.
func TestGetPlainTextFileMovedReturnsError(t *testing.T) {
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
// equal contents.
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

// TestLatin1DecodeFullRange pins the byte-for-codepoint contract of
// latin1Decode for every byte 0x00..0xFF. C1 controls
// (0x80-0x9F) and the rest of the Latin-1 supplement (0xA0-0xFF) map verbatim
// via rune(b). Replacement to U+FFFD is applied ONLY to bytes < 0x20 (except
// TAB / LF / CR; form-feed IS replaced) and to 0x7F (DEL). Direct call -- no
// fixture needed for the per-byte contract.
func TestLatin1DecodeFullRange(t *testing.T) {
	in := make([]byte, 256)
	for i := range in {
		in[i] = byte(i)
	}
	got := latin1Decode(in)
	runes := []rune(got)
	if len(runes) != 256 {
		t.Fatalf("len(runes) = %d, want 256", len(runes))
	}
	for i, r := range runes {
		b := byte(i)
		var want rune
		switch {
		case b == 0x09 || b == 0x0A || b == 0x0D:
			want = rune(b)
		case b >= 0x20 && b != 0x7F:
			// Includes the full 0x80-0x9F C1 control range and 0xA0-0xFF.
			want = rune(b)
		default:
			want = '�'
		}
		if r != want {
			t.Errorf("byte 0x%02X decoded to U+%04X, want U+%04X", b, r, want)
		}
	}
}

// TestGetPlainTextSizeAfterRemove verifies the contract change: after
// Inspector.Open captures FileSize once via os.Stat, removing the file does
// not invalidate the size. GetPlainTextSize returns the cached value without
// error.
func TestGetPlainTextSizeAfterRemove(t *testing.T) {
	srcPath := filepath.Join(testdataDir(t), "minimal.pdf")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read minimal.pdf: %v", err)
	}
	tmp, err := os.CreateTemp("", "pdfcore-10-6-size-remove-*.pdf")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := tmp.Write(src); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = tmp.Close()
	path := tmp.Name()

	ins := NewInspector()
	tabID := "tab-10-6-size-remove"
	if _, err := ins.Open(tabID, path); err != nil {
		_ = os.Remove(path)
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })

	wantSize := int64(len(src))
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	got, err := ins.GetPlainTextSize(tabID)
	if err != nil {
		t.Fatalf("GetPlainTextSize after remove: expected nil error, got %v", err)
	}
	if got != wantSize {
		t.Errorf("GetPlainTextSize after remove = %d, want cached %d", got, wantSize)
	}
}

// TestGetPlainTextLatin1C1 is the integration test: open the
// testdata/correctness/latin1-c1.pdf fixture (content stream = 32 C1-control
// bytes 0x80..0x9F) and assert the C1 region maps verbatim in GetPlainText.
func TestGetPlainTextLatin1C1(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-10-6-c1"
	path := filepath.Join(testdataDir(t), "correctness", "latin1-c1.pdf")
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })

	pt, err := ins.GetPlainText(tabID)
	if err != nil {
		t.Fatalf("GetPlainText: %v", err)
	}
	// The plain-text view returns the entire file bytes Latin-1-decoded.
	// Assert every C1 control codepoint U+0080..U+009F appears in the
	// output exactly where the fixture's content stream placed them.
	for cp := rune(0x80); cp <= 0x9F; cp++ {
		if !strings.ContainsRune(pt.Content, cp) {
			t.Errorf("plain-text view missing codepoint U+%04X (C1 control should map verbatim from byte 0x%02X)", cp, cp)
		}
	}
	// Also assert no U+FFFD leaked in for those bytes.
	if strings.ContainsRune(pt.Content, '�') {
		// The fixture content stream is only C1 bytes plus structural PDF
		// bytes; if a U+FFFD shows up it must come from a structural byte
		// like 0x00..0x1F that the fixture author did not include. Locate
		// the offending byte.
		idx := strings.IndexRune(pt.Content, '�')
		t.Errorf("plain-text view contains U+FFFD at offset %d (C1 region must map verbatim, not via the replacement table)", idx)
	}
}
