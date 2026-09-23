// JPEG marker-chain reading, scoped to one question: does a stored DCT stream
// carry an Adobe APP14 record, and what colour transform does that record
// declare?
//
// This is a colour-space-intent feature, not JPEG support. Colour-space intent
// is recorded in three places - /ColorSpace, /Decode and the Adobe APP14
// transform - and the bug class is those three disagreeing. ICC profiles in
// APP2, EXIF orientation in APP1 and everything else inside the file are out of
// scope. No pixel is decoded here and no entropy-coded byte is read: the walk
// hops marker to marker and stops at the Adobe record, or at SOS when there is
// none.

package pdfcore

import (
	"bytes"
	"encoding/binary"
)

// Outcome discriminators for ImageData.AdobeMarker. A consumer branches on the
// value instead of parsing prose, the way it does on ImageData.Kind. The zero
// value, the empty string, is a sixth state and means the image dictionary was
// never read at all.
const (
	// AdobeMarkerNotApplicable means the stream is not a DCT image, so there is
	// no marker chain to walk.
	AdobeMarkerNotApplicable = "not-applicable"
	// AdobeMarkerAbsent means the chain was walked to SOS and carries no Adobe
	// APP14 record.
	AdobeMarkerAbsent = "absent"
	// AdobeMarkerPresent means an Adobe APP14 record was found and its transform
	// byte is reported.
	AdobeMarkerPresent = "present"
	// AdobeMarkerUnparseable means the chain is malformed, carries an Adobe APP14
	// too short to hold its own record, or hit a walk ceiling before any Adobe
	// APP14 record was read. Never reported as absent: a chain that could not be
	// read is not a chain without a marker.
	AdobeMarkerUnparseable = "unparseable"
	// AdobeMarkerNotExamined means DCTDecode sits behind another filter, so the
	// stored bytes are not the JPEG and were not walked.
	AdobeMarkerNotExamined = "not-examined"
)

// adobeIdentifier is the five-byte identifier an APP14 segment must open with
// before any byte after it is read. Non-Adobe APP14 segments exist, and reading
// one as an Adobe record reports somebody else's payload byte as a transform.
var adobeIdentifier = []byte("Adobe")

// adobeRecordBytes is the payload an Adobe APP14 record occupies: the five-byte
// identifier, a version word, two flag words and the transform byte. A payload
// carrying the identifier in fewer bytes than this is a malformed Adobe record
// and fails closed rather than being read past.
const adobeRecordBytes = 12

// scanAdobeMarker walks raw as a JPEG marker chain and reports whether it
// carries an Adobe APP14 record, with the record's transform byte beside the
// outcome. The transform is meaningful only when the outcome is
// AdobeMarkerPresent.
//
// The walk answers AdobeMarkerPresent the moment a complete, identifier-checked
// record has been read, and does not look at what follows. A chain truncated
// after the record still reports the transform it stated: the record is a fact
// about bytes that were there, and retracting it because the rest of the file is
// missing would throw away a known answer. A JPEG that reports present can still
// fail to decode.
//
// The walk is bounded by maxJPEGMarkerSegments and maxJPEGMarkerScanBytes, and
// where no record was read it fails closed: a declared length that runs past the
// end of the buffer, a length below the two bytes the length field itself
// occupies, a 0x00 where a marker code belongs, a chain that ends without
// reaching SOS, an APP14 carrying the Adobe identifier in too few bytes to hold
// the record, and either ceiling all yield AdobeMarkerUnparseable rather than a
// silent AdobeMarkerAbsent.
func scanAdobeMarker(raw []byte) (outcome string, transform int) {
	if len(raw) < 4 || raw[0] != 0xFF || raw[1] != 0xD8 {
		return AdobeMarkerUnparseable, 0
	}
	limit := len(raw)
	if limit > maxJPEGMarkerScanBytes {
		limit = maxJPEGMarkerScanBytes
	}

	i := 2
	for segments := 0; segments < maxJPEGMarkerSegments; segments++ {
		// Runs of 0xFF padding are legal between segments and are not markers.
		// The marker is the first non-0xFF byte after them; marker bytes are
		// never 0xFF, so this cannot swallow one.
		if i >= limit || raw[i] != 0xFF {
			return AdobeMarkerUnparseable, 0
		}
		for i < limit && raw[i] == 0xFF {
			i++
		}
		if i >= limit {
			return AdobeMarkerUnparseable, 0
		}
		marker := raw[i]
		i++

		// 0x00 is not a marker code. FF 00 is a stuffed byte and belongs to
		// entropy-coded data, which begins after SOS and is never walked here.
		// Reading the two bytes behind it as a segment length would hop the walk
		// into arbitrary data, where it can read somebody else's bytes as an
		// Adobe record or land on a byte pair that reads as SOS and answer a
		// confident "absent".
		if marker == 0x00 {
			return AdobeMarkerUnparseable, 0
		}

		// D8, D9, D0-D7 and 01 are standalone: no length field, advance past the
		// marker alone.
		if marker == 0xD8 || marker == 0xD9 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}
		// SOS. Everything after it is entropy-coded data and is never scanned.
		if marker == 0xDA {
			return AdobeMarkerAbsent, 0
		}

		if i+2 > limit {
			return AdobeMarkerUnparseable, 0
		}
		segLen := int(binary.BigEndian.Uint16(raw[i : i+2]))
		if segLen < 2 {
			return AdobeMarkerUnparseable, 0
		}
		end := i + segLen
		if end > limit {
			return AdobeMarkerUnparseable, 0
		}
		payload := raw[i+2 : end]
		// The identifier decides whether this is an Adobe record, the length only
		// whether the record is whole. A payload too short to carry the five
		// identifier bytes fails the prefix test and is skipped like any other
		// non-Adobe APP14; one that carries the identifier and stops short of the
		// record is a malformed Adobe record and fails closed.
		if marker == 0xEE && bytes.HasPrefix(payload, adobeIdentifier) {
			if len(payload) < adobeRecordBytes {
				return AdobeMarkerUnparseable, 0
			}
			return AdobeMarkerPresent, int(payload[adobeRecordBytes-1])
		}
		i = end
	}
	return AdobeMarkerUnparseable, 0
}
