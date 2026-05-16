package pdfcore

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// plainTextByteCap is the maximum number of file bytes read for the Plain
// Text view. Files larger than this surface a truncation banner to the user;
// the displayed prefix is still scrollable. 5 MiB cap matches the IPC
// payload-size budget for Wails' default in-memory buffer.
const plainTextByteCap int64 = 5 * 1024 * 1024

// GetPlainText reads the on-disk bytes of the PDF backing tabID and returns a
// Latin-1-decoded view capped at plainTextByteCap. Story 9-11.
//
// The mutex covers the I/O so two concurrent callers share one disk read.
// Lifetime of the cache = lifetime of DocumentState (replaced on re-Open,
// freed on Close).
func (ins *Inspector) GetPlainText(tabID string) (*PlainTextDocument, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}

	doc.plainTextMu.Lock()
	defer doc.plainTextMu.Unlock()
	if doc.plainTextCache != nil {
		return doc.plainTextCache, nil
	}

	var out *PlainTextDocument
	err = safeCall(func() error {
		var e error
		out, e = readPlainText(doc.FilePath, tabID)
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}
	doc.plainTextCache = out
	return out, nil
}

// readPlainText opens path, reads up to plainTextByteCap bytes, Latin-1-decodes
// them with the control-byte normalization mandated by AC6, and returns the
// payload with the truncation flag set from a separate stat (not from
// len(read)) so a file exactly equal to the cap does not falsely flag as
// truncated.
func readPlainText(path, tabID string) (*PlainTextDocument, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	totalBytes := fi.Size()

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	limited := io.LimitReader(f, plainTextByteCap)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	content := latin1Decode(buf)

	return &PlainTextDocument{
		TabID:      tabID,
		Content:    content,
		TotalBytes: totalBytes,
		Truncated:  totalBytes > plainTextByteCap,
		CapBytes:   plainTextByteCap,
	}, nil
}

// latin1Decode maps each input byte to its Unicode codepoint via rune(b),
// replacing forbidden control bytes with U+FFFD. Permitted whitespace bytes
// are 0x09 (TAB), 0x0A (LF), 0x0D (CR); everything else under 0x20 is
// replaced. 0x7F (DEL) is also replaced. Form-feed 0x0C IS replaced (AC6).
// Go's `string(byteSlice)` produces UTF-8, which would mojibake non-ASCII
// bytes; the byte-by-byte rune cast is the lossless path.
func latin1Decode(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		switch {
		case c == 0x09 || c == 0x0A || c == 0x0D:
			sb.WriteRune(rune(c))
		case c >= 0x20 && c != 0x7F:
			sb.WriteRune(rune(c))
		default:
			sb.WriteRune('�')
		}
	}
	return sb.String()
}
