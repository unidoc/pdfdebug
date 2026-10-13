package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Plain-text format toolkit shared by the dump presenters. These render the
// SAME structs the --json path marshals, but for human reading: plain text is
// the default output and is explicitly NON-CONTRACTUAL (it may change between
// releases; callers that parse must pass --json). Output is ASCII-only and
// ends with a trailing newline. There is intentionally no Presenter interface
// (the 11 data shapes are unrelated); only these small format helpers are
// shared, extracted where real duplication appeared across presenters.

// kvWriter accumulates key/value rows and renders them as an aligned block:
// the colon column is aligned across all rows so the eye drops straight down.
// Blank lines (added via Gap) separate sub-sections. It is a single-use builder
// per presenter call, not a long-lived object.
type kvWriter struct {
	rows []kvRow
}

// kvRow is one entry in a kvWriter: a key/value pair, or a section gap/heading.
type kvRow struct {
	key     string
	value   string
	gap     bool // true: emit a blank separator line, ignore key/value
	heading string
}

// Add appends an aligned "key: value" row.
func (w *kvWriter) Add(key, value string) {
	w.rows = append(w.rows, kvRow{key: key, value: value})
}

// Addf appends an aligned "key: value" row with a formatted value.
func (w *kvWriter) Addf(key, format string, args ...any) {
	w.rows = append(w.rows, kvRow{key: key, value: fmt.Sprintf(format, args...)})
}

// Gap appends a blank separator line between sub-sections.
func (w *kvWriter) Gap() {
	w.rows = append(w.rows, kvRow{gap: true})
}

// Heading appends a standalone section heading line (no colon alignment).
func (w *kvWriter) Heading(text string) {
	w.rows = append(w.rows, kvRow{heading: text})
}

// Render writes the accumulated rows to out with the colon column aligned
// across all key/value rows.
func (w *kvWriter) Render(out io.Writer) error {
	width := 0
	for _, r := range w.rows {
		if !r.gap && r.heading == "" && len(r.key) > width {
			width = len(r.key)
		}
	}
	var b strings.Builder
	for _, r := range w.rows {
		switch {
		case r.gap:
			b.WriteByte('\n')
		case r.heading != "":
			b.WriteString(r.heading)
			b.WriteByte('\n')
		default:
			// key, then colon, then padding to align the value column.
			b.WriteString(r.key)
			b.WriteByte(':')
			pad := width - len(r.key) + 1
			for range pad {
				b.WriteByte(' ')
			}
			b.WriteString(r.value)
			b.WriteByte('\n')
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// asciiSafe renders a value on the ASCII-only plain-text surface. A printable
// ASCII value is returned unchanged; anything carrying a byte outside
// 0x20-0x7e is escaped with strconv.QuoteToASCII. Escaping also keeps
// tableWriter's len()-based column padding aligned, which raw multi-byte UTF-8
// breaks.
func asciiSafe(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return strconv.QuoteToASCII(s)
		}
	}
	return s
}

// tableWriter renders uniform repeated records as a header row plus aligned
// data columns (it is a spreadsheet; draw it like one). Columns are
// space-padded to the widest cell. A single-use builder per presenter call.
type tableWriter struct {
	header []string
	rows   [][]string
}

// newTable creates a tableWriter with the given column headers.
func newTable(header ...string) *tableWriter {
	return &tableWriter{header: header}
}

// AddRow appends one data row. The cell count should match the header; short
// rows are padded with empty cells, long rows are kept verbatim.
func (t *tableWriter) AddRow(cells ...string) {
	t.rows = append(t.rows, cells)
}

// Render writes the header and data rows with each column padded to the widest
// cell in that column. A two-space gutter separates columns. When there are
// zero data rows, only the header line is emitted (so the shape is still
// legible).
func (t *tableWriter) Render(out io.Writer) error {
	cols := len(t.header)
	for _, r := range t.rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	cell := func(row []string, i int) string {
		if i < len(row) {
			return row[i]
		}
		return ""
	}
	for i := range cols {
		if i < len(t.header) && len(t.header[i]) > widths[i] {
			widths[i] = len(t.header[i])
		}
		for _, r := range t.rows {
			if c := cell(r, i); len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	var b strings.Builder
	writeLine := func(row []string) {
		for i := range cols {
			c := cell(row, i)
			b.WriteString(c)
			// Pad every column except the last to its width + a two-space gutter.
			if i < cols-1 {
				for range widths[i] - len(c) + 2 {
					b.WriteByte(' ')
				}
			}
		}
		b.WriteByte('\n')
	}
	writeLine(t.header)
	for _, r := range t.rows {
		writeLine(r)
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// humanizeBytes renders a byte count as a compact human-readable size (e.g.
// "1.5 KB", "240 B"). Used by single-record presenters that surface stream or
// payload sizes. Plain-text only; the --json path carries the raw integer.
func humanizeBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
