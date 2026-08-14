package pdfcore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// chunkSize is the per-iteration read size of the cancellable plaintext load.
// At 1 MiB, cancel latency upper-bounds at one chunk-read time (story 10-1
// Dev Notes "Chunk size choice").
const chunkSize = 1 << 20

// maxPlainTextAlloc caps the single contiguous []byte allocation for the
// plaintext load. 4 GiB. Prevents 32-bit int overflow on `make([]byte, 0,
// int(totalBytes))` and guards against single-call OOM-killer triggers on
// 64-bit hosts. Files above this ceiling return ErrUnsupportedPDF before any
// read begins.
const maxPlainTextAlloc = int64(4) << 30

// GetPlainText reads the on-disk bytes of the PDF backing tabID and returns a
// Latin-1-decoded view of the FULL file. Story 10-1 (replaces the 9-11 25 MiB
// cap + 9-12 "Load all" two-tier model).
//
// The read is cancellable: a per-document context.CancelFunc is stored under
// the dedicated plainTextCancelMu mutex so CancelPlainText can preempt the
// read without contending against plainTextMu (which is held for the entire
// I/O). Cancellation returns context.Canceled UNWRAPPED -- wrapPDFError would
// reclassify it as ErrMalformedPDF and break errors.Is(err, context.Canceled).
//
// Concurrent callers for the same tab serialize on plainTextMu. The first
// performs the disk read; subsequent callers see the cached pointer.
// Cancelled and errored loads do NOT populate the cache.
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

	// Create a per-call cancel context and publish it under the cancel mutex
	// (NOT plainTextMu, which is already held). Defer cleanup that clears the
	// slot AND calls cancel() idempotently -- safe to call after success.
	//
	// plainTextClosed check closes the race window where Inspector.Close fires
	// between GetDocument and this registration: Close would observe a nil
	// cancel func and the read would otherwise run to natural completion,
	// defeating the "one chunk-read cycle" promise. By checking the closed
	// flag while holding plainTextCancelMu, we either see Close's flag (bail
	// with context.Canceled) or our cancel func is published before Close can
	// observe it.
	ctx, cancel := context.WithCancel(context.Background())
	doc.plainTextCancelMu.Lock()
	if doc.plainTextClosed {
		doc.plainTextCancelMu.Unlock()
		cancel()
		return nil, context.Canceled
	}
	doc.plainTextLoadCancel = cancel
	doc.plainTextCancelMu.Unlock()
	defer func() {
		doc.plainTextCancelMu.Lock()
		doc.plainTextLoadCancel = nil
		doc.plainTextCancelMu.Unlock()
		cancel()
	}()

	var out *PlainTextDocument
	err = safeCall(func() error {
		var e error
		out, e = readPlainText(ctx, doc.FilePath, doc.FileSize, tabID)
		return e
	})
	if err != nil {
		// Bypass wrapPDFError for cancellation AND ErrUnsupportedPDF (the 4 GiB
		// ceiling guard) so errors.Is(err, ...) survives across the boundary.
		// The wrapper's %v verb stringifies the inner error and severs the
		// chain; for context.Canceled the result is a misleading "malformed PDF"
		// message, for ErrUnsupportedPDF the sentinel contract breaks. See Dev
		// Notes "Why bypass wrapPDFError for context.Canceled".
		if errors.Is(err, context.Canceled) || errors.Is(err, ErrUnsupportedPDF) {
			return nil, err
		}
		return nil, wrapPDFError(err)
	}
	doc.plainTextCache = out
	return out, nil
}

// CancelPlainText cancels an in-flight GetPlainText for tabID. No-op if no
// load is in flight or the load already completed. Returns ErrDocumentNotFound
// for unknown tabs. MUST acquire only plainTextCancelMu -- acquiring
// plainTextMu would deadlock against the active read.
//
// Returns immediately; the in-flight goroutine observes ctx.Done() between
// chunks and unwinds on its own.
func (ins *Inspector) CancelPlainText(tabID string) error {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return err
	}
	doc.plainTextCancelMu.Lock()
	cancel := doc.plainTextLoadCancel
	doc.plainTextCancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// GetPlainTextSize returns the on-disk byte size of the PDF backing tabID.
// Powers the loading-card size disclosure. Returns the size captured at Open
// time; subsequent moves/deletions of the underlying file do not affect this
// value (Story 10.6 removed the redundant re-stat). Returns ErrDocumentNotFound
// for unknown tabs.
func (ins *Inspector) GetPlainTextSize(tabID string) (int64, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return 0, err
	}
	return doc.FileSize, nil
}

// readPlainText performs the cancellable chunked read of path, Latin-1-decodes
// the result, and returns the payload. The caller (GetPlainText) passes the
// stat-at-Open size threaded through doc.FileSize so no in-function os.Stat is
// needed (Story 10.6). Returns ctx.Err() (context.Canceled) when cancellation
// is observed between chunks -- the caller must NOT wrap this through
// wrapPDFError.
//
// If the underlying file grows between Open and this call, the buffer cap
// under-allocates and append() handles reallocation correctly (the 4 GiB
// ceiling still uses the cached size, so a file that grows past the ceiling
// post-Open is read up to whatever bytes are produced by the read loop).
func readPlainText(ctx context.Context, path string, size int64, tabID string) (*PlainTextDocument, error) {
	totalBytes := size

	// 4 GiB ceiling: protect 32-bit int overflow + single-allocation OOM.
	// Files at the ceiling fail before any read; the wrap chain surfaces
	// ErrUnsupportedPDF.
	if totalBytes > maxPlainTextAlloc {
		return nil, fmt.Errorf("%w: plain text view supports files up to %d bytes (%d)", ErrUnsupportedPDF, maxPlainTextAlloc, totalBytes)
	}
	// Defensive: a pathological filesystem (FUSE, network FS) can stat a
	// negative size. int(negative int64) would feed make() a negative cap
	// and panic. Treat as a zero-byte file.
	if totalBytes < 0 {
		totalBytes = 0
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	// Pre-size the buffer cap so the chunked append never reallocates. The
	// ceiling guard above means int(totalBytes) is safe.
	buf := make([]byte, 0, int(totalBytes))
	chunk := make([]byte, chunkSize)
	for {
		// Cancel check fires BEFORE each read so a Cancel that arrives between
		// chunks short-circuits without one more syscall.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := f.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", path, readErr)
		}
		// Defensive: a Reader returning (0, nil) is allowed by io.Reader's
		// contract but would spin this loop forever. Treat as EOF.
		if n == 0 {
			break
		}
	}

	// One final cancel check after the read loop. Closes the race window
	// where Cancel arrives after the last chunk read but before decode
	// begins: without this, the load completes "successfully" and a test
	// asserting cancellation flakes.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	content := latin1Decode(buf)

	// And one more after decode -- decode iterates totalBytes runes and can
	// dominate the timing on large files. Same race-window argument.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &PlainTextDocument{
		TabID:      tabID,
		Content:    content,
		TotalBytes: totalBytes,
	}, nil
}

// latin1Decode maps each input byte to its Unicode codepoint via rune(b).
// Replacement to U+FFFD is applied ONLY to bytes < 0x20 (except 0x09 TAB,
// 0x0A LF, 0x0D CR; form-feed 0x0C IS replaced) and to 0x7F
// (DEL). C1 controls (0x80-0x9F) and all other Latin-1 supplement bytes
// (0xA0-0xFF) map verbatim -- the Latin-1 decode path is intentionally
// lossless for stream bytes so users debugging a PDF see what's actually
// there. Go's `string(byteSlice)` produces UTF-8, which would mojibake
// non-ASCII bytes; the byte-by-byte rune cast is the lossless path.
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
