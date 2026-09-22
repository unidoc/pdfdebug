// Co-located unit tests for the Adobe APP14 marker walk:
//
//	scanAdobeMarker(raw []byte) (outcome string, transform int)
//
// The input is a byte slice, so every chain shape is built here rather than
// wrapped in a PDF. The two ceilings in particular are cheaper to drive on bytes
// than on a fixture file.
package pdfcore

import "testing"

// adobeSegment returns the 16-byte Adobe APP14 segment carrying the given
// transform: marker, a length of 14 covering itself plus the 12-byte record, the
// identifier, version 100, two flag words and the transform byte.
func adobeSegment(transform byte) []byte {
	return []byte{
		0xFF, 0xEE, 0x00, 0x0E,
		'A', 'd', 'o', 'b', 'e',
		0x00, 0x64,
		0x00, 0x00,
		0x00, 0x00,
		transform,
	}
}

// app14Segment returns an APP14 of the same length as the Adobe record whose
// identifier is something else. A walk that branches on the marker rather than
// the identifier reports its last payload byte as a transform.
func app14Segment(identifier string, last byte) []byte {
	seg := []byte{0xFF, 0xEE, 0x00, 0x0E}
	seg = append(seg, identifier...)
	for len(seg) < 15 {
		seg = append(seg, 0x00)
	}
	return append(seg, last)
}

// genericSegment returns a length-bearing segment of the given marker with n
// payload bytes beyond the length field.
func genericSegment(marker byte, n int) []byte {
	seg := []byte{0xFF, marker, byte((n + 2) >> 8), byte((n + 2) & 0xFF)}
	return append(seg, make([]byte, n)...)
}

// chain wraps segments in SOI, SOS and a few bytes of entropy data. Nothing
// after SOS is a marker, so a walk that reads past it is reading samples.
func chain(segments ...[]byte) []byte {
	out := []byte{0xFF, 0xD8}
	for _, s := range segments {
		out = append(out, s...)
	}
	out = append(out, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00)
	out = append(out, 0x12, 0x34, 0x56, 0x78)
	return append(out, 0xFF, 0xD9)
}

// padding returns a run of 0xFF fill bytes, which is legal between segments and
// is not a marker.
func padding(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = 0xFF
	}
	return out
}

// standalones returns D0 through D7, which carry no length field. A walk that
// reads two length bytes after one of them consumes whatever follows, so a
// segment placed after them disappears.
func standalones() []byte {
	out := make([]byte, 0, 16)
	for m := byte(0xD0); m <= 0xD7; m++ {
		out = append(out, 0xFF, m)
	}
	return out
}

func TestScanAdobeMarker(t *testing.T) {
	cases := []struct {
		name          string
		raw           []byte
		wantOutcome   string
		wantTransform int
	}{
		{
			name:        "a record with no colour transform",
			raw:         chain(adobeSegment(0)),
			wantOutcome: AdobeMarkerPresent,
		},
		{
			name:          "a record selecting YCbCr",
			raw:           chain(adobeSegment(1)),
			wantOutcome:   AdobeMarkerPresent,
			wantTransform: 1,
		},
		{
			name:          "a record selecting YCCK",
			raw:           chain(adobeSegment(2)),
			wantOutcome:   AdobeMarkerPresent,
			wantTransform: 2,
		},
		{
			name:          "an unassigned transform value is reported as written",
			raw:           chain(adobeSegment(7)),
			wantOutcome:   AdobeMarkerPresent,
			wantTransform: 7,
		},
		{
			name:        "no application segment at all",
			raw:         chain(),
			wantOutcome: AdobeMarkerAbsent,
		},
		{
			name:        "an APP14 whose identifier is not Adobe is skipped",
			raw:         chain(app14Segment("Other", 0x07)),
			wantOutcome: AdobeMarkerAbsent,
		},
		{
			name:        "an identifier that only shares a prefix is skipped",
			raw:         chain(app14Segment("Adob", 0x07)),
			wantOutcome: AdobeMarkerAbsent,
		},
		{
			name:        "an APP14 too short to hold the record is skipped",
			raw:         chain([]byte{0xFF, 0xEE, 0x00, 0x06, 'A', 'd', 'o', 'b'}),
			wantOutcome: AdobeMarkerAbsent,
		},
		{
			name:          "a record behind other segments is still found",
			raw:           chain(genericSegment(0xE1, 40), app14Segment("Other", 0x03), adobeSegment(2)),
			wantOutcome:   AdobeMarkerPresent,
			wantTransform: 2,
		},
		{
			name:          "padding between segments is skipped rather than read as a marker",
			raw:           chain(padding(5), adobeSegment(1)),
			wantOutcome:   AdobeMarkerPresent,
			wantTransform: 1,
		},
		{
			name:          "standalone markers carry no length and hide nothing behind them",
			raw:           chain(standalones(), adobeSegment(2)),
			wantOutcome:   AdobeMarkerPresent,
			wantTransform: 2,
		},
		{
			name:        "a record after SOS is never reached",
			raw:         append(chain(), adobeSegment(2)...),
			wantOutcome: AdobeMarkerAbsent,
		},
		{
			name:        "a declared length running past the end fails closed",
			raw:         []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x40, 0x01, 0x02, 0x03},
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			name:        "a length below its own two bytes fails closed",
			raw:         []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x01, 0x41, 0x42, 0xFF, 0xDA, 0x00, 0x02},
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			name:        "a length field truncated mid-word fails closed",
			raw:         []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00},
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			name:        "a chain that never reaches SOS fails closed",
			raw:         append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, make([]byte, 14)...),
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			name:        "a stuffed FF 00 is not a marker and fails closed",
			raw:         chain([]byte{0xFF, 0x00, 0x00, 0x04, 0x41, 0x42}),
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			// Read as a length-bearing segment, the two bytes behind the 0x00
			// hop the walk over six bytes and land it on the record that
			// follows. The outcome is reported as unreadable instead.
			name:        "a stuffed FF 00 does not hop the walk into the bytes behind it",
			raw:         chain(append([]byte{0xFF, 0x00, 0x00, 0x04, 0x41, 0x42}, adobeSegment(2)...)),
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			name:        "a byte that is not a marker boundary fails closed",
			raw:         []byte{0xFF, 0xD8, 0x41, 0x42, 0x43, 0x44},
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			name:        "a chain of nothing but padding fails closed",
			raw:         append([]byte{0xFF, 0xD8}, padding(6)...),
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			name:        "bytes that do not open with SOI fail closed",
			raw:         []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A},
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			name:        "an empty stream fails closed",
			raw:         nil,
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			name:        "a one-byte stream fails closed",
			raw:         []byte{0xFF},
			wantOutcome: AdobeMarkerUnparseable,
		},
		{
			name:        "SOI alone fails closed",
			raw:         []byte{0xFF, 0xD8},
			wantOutcome: AdobeMarkerUnparseable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outcome, transform := scanAdobeMarker(tc.raw)
			if outcome != tc.wantOutcome {
				t.Fatalf("outcome = %q, want %q", outcome, tc.wantOutcome)
			}
			if outcome == AdobeMarkerPresent && transform != tc.wantTransform {
				t.Errorf("transform = %d, want %d", transform, tc.wantTransform)
			}
		})
	}
}

// Both ceilings stop the walk with the fail-closed outcome rather than letting
// it run to the end of a large stream. A record placed past either is not found,
// which is the trade: the ceilings are sized for where a real Adobe record sits.
func TestScanAdobeMarkerCeilings(t *testing.T) {
	t.Run("a record inside both ceilings is found", func(t *testing.T) {
		segments := make([][]byte, 0, maxJPEGMarkerSegments-2)
		for range maxJPEGMarkerSegments - 2 {
			segments = append(segments, genericSegment(0xE1, 0))
		}
		outcome, transform := scanAdobeMarker(chain(append(segments, adobeSegment(2))...))
		if outcome != AdobeMarkerPresent || transform != 2 {
			t.Fatalf("outcome = %q, transform = %d, want a record just inside the segment ceiling", outcome, transform)
		}
	})

	t.Run("a chain longer than the segment ceiling fails closed", func(t *testing.T) {
		segments := make([][]byte, 0, maxJPEGMarkerSegments+4)
		for range maxJPEGMarkerSegments + 4 {
			segments = append(segments, genericSegment(0xE1, 0))
		}
		if outcome, _ := scanAdobeMarker(chain(append(segments, adobeSegment(2))...)); outcome != AdobeMarkerUnparseable {
			t.Errorf("outcome = %q, want %q", outcome, AdobeMarkerUnparseable)
		}
	})

	t.Run("a record past the byte ceiling fails closed", func(t *testing.T) {
		// One segment wide enough to push everything after it past the ceiling.
		// The walk stops inside it rather than hopping to the record beyond.
		wide := make([]byte, 0, maxJPEGMarkerScanBytes+64)
		for len(wide) < maxJPEGMarkerScanBytes {
			wide = append(wide, genericSegment(0xE1, 0xFFF0)...)
		}
		if outcome, _ := scanAdobeMarker(chain(wide, adobeSegment(2))); outcome != AdobeMarkerUnparseable {
			t.Errorf("outcome = %q, want %q", outcome, AdobeMarkerUnparseable)
		}
	})

	t.Run("a record behind a large ICC chain is found", func(t *testing.T) {
		// Photoshop writes the Adobe record after the APP2 ICC chunks, and a CMYK
		// output profile runs to megabytes, so the record sits well into the file
		// on the well-formed CMYK images this walk exists for.
		const iccBytes = 4 * 1024 * 1024
		icc := make([]byte, 0, iccBytes+64)
		for len(icc) < iccBytes {
			icc = append(icc, genericSegment(0xE2, 0xFFF0)...)
		}
		outcome, transform := scanAdobeMarker(chain(icc, adobeSegment(2)))
		if outcome != AdobeMarkerPresent || transform != 2 {
			t.Fatalf("outcome = %q, transform = %d, want the record behind the profile", outcome, transform)
		}
	})
}
