// The sample-interpretation verdict: one answer joining the PDF-side /Decode
// array with the JPEG-side Adobe APP14 marker.
//
// Each says "invert the samples" in its own layer, written by tools that never
// spoke to each other, so the outcome depends on the combination rather than on
// either alone. The raw array and the transform number stay visible beside the
// verdict so an expert can check the arithmetic.

package pdfcore

// The verdict vocabulary. Fixed here rather than composed at the call site: a
// verdict whose wording varies by code path is neither testable nor quotable in
// a bug report, and these strings are meant to be pasted into one.
const (
	verdictNormalDefault  = "Normal (default)"
	verdictNormalAdobe    = "Normal: /Decode compensates for Adobe-inverted CMYK"
	verdictInvertedAdobe  = "Inverted: Adobe CMYK is stored inverted and no /Decode compensates"
	verdictInvertedNoMark = "Inverted: /Decode inverts, no Adobe APP14 marker"
	verdictInvertedDecode = "Inverted by /Decode"
	verdictNonDefault     = "Non-default /Decode"
	verdictNotClassified  = "Not classified: /Decode on an Indexed or Lab image is not a simple inversion"
	verdictUnknownDecode  = "Unknown: /Decode array unreadable"
	verdictUnknownArity   = "Unknown: /Decode not checkable (colour-component count unresolved)"
	verdictUnknownChain   = "Unknown: JPEG marker chain unreadable"
	verdictUnknownReach   = "Unknown: JPEG bytes not reachable (DCTDecode behind another filter)"
)

// sampleInterpretationVerdict joins the /Decode array and the Adobe APP14
// outcome into one answer. colorSpace is the name ImageData carries (the array
// head for an array-typed /ColorSpace), imageMask is the stencil-mask flag,
// components is the resolved colour-component count (negative when it could not
// be resolved), decode is the array as read (nil when the key is absent),
// decodeRejected says the key is present but its array was rejected as
// malformed, and markerOutcome is one of the AdobeMarker* discriminators.
//
// Five rules carry the logic:
//
//   - An Adobe APP14 CMYK JPEG stores INVERTED CMYK. Neither poppler nor mupdf
//     un-inverts it in the codec, so the identity /Decode renders the negative
//     and /Decode [1 0 1 0 1 0 1 0] is what renders it correctly. Measured on
//     both renderers against a CMYK JPEG carrying transform 2; the story records
//     the numbers.
//
//   - The marker arm fires only at four components. The stored inversion, and
//     Go's un-inversion of it in applyBlack, live in the 4-component path alone;
//     on 1 or 3 components the transform selects a colour transform and inverts
//     nothing. Photoshop writes an Adobe APP14 into ordinary RGB JPEGs, so a
//     rule that fires below four components mislabels a very large share of real
//     images. An unresolved component count is not four components.
//
//   - Presence decides, not the transform value. Every valid Adobe record marks
//     a 4-component stream as inverted; the number only chooses between
//     YCCK-to-CMYK and a direct interleave. The transform is evidence, never an
//     input.
//
//   - An array that is there but cannot be established says so. A rejected array
//     stores nothing, and the nil that leaves behind is the same nil an absent
//     key leaves; an array whose arity cannot be checked because the component
//     count was never resolved is neither the default nor an inversion. Reading
//     either as the default would answer "checked, nothing here" on a file that
//     does set the array.
//
//   - Indexed and Lab are not classified when they carry an array. Their default
//     /Decode is [0 2^bpc - 1] and their /Range respectively, so the [0 1]
//     identity test would call a perfectly ordinary array an inversion. With no
//     array at all there is nothing to misclassify: an absent key is the default
//     whatever the colour space.
func sampleInterpretationVerdict(colorSpace string, imageMask bool, components int, decode []float64, decodeRejected bool, markerOutcome string) string {
	decodePresent := decodeRejected || len(decode) > 0
	if !imageMask && decodePresent && (colorSpace == "Indexed" || colorSpace == "Lab") {
		return verdictNotClassified
	}

	// A stencil mask is one component whatever else the dictionary says.
	if imageMask {
		components = 1
	}
	fourComponents := components == 4

	// Four components: the marker is live, and an outcome that could not be read
	// outranks any verdict that would depend on it. Whether a compensating
	// inversion exists is precisely what is unknown there.
	if fourComponents {
		switch markerOutcome {
		case AdobeMarkerUnparseable:
			return verdictUnknownChain
		case AdobeMarkerNotExamined:
			return verdictUnknownReach
		}
	}

	// The array is set and what it says cannot be established. readDecodeArray's
	// bound is an upper one, so an array of the wrong arity is stored rather than
	// rejected, and without a component count the arity cannot be checked at all.
	if decodeRejected {
		return verdictUnknownDecode
	}
	if len(decode) > 0 && components <= 0 {
		return verdictUnknownArity
	}

	identity, inverted := decodePattern(decode, components)

	if fourComponents && markerOutcome == AdobeMarkerPresent {
		switch {
		case inverted:
			return verdictNormalAdobe
		case identity:
			return verdictInvertedAdobe
		}
		return verdictNonDefault
	}
	if fourComponents && markerOutcome == AdobeMarkerAbsent {
		// A chain that was walked to SOS and carries no record. A 4-component DCT
		// stream with no Adobe record stores direct CMYK, so the identity array is
		// normal and an inverting one is the negative. Only here does the absence
		// of a marker answer anything, so only here is it named.
		switch {
		case identity:
			return verdictNormalDefault
		case inverted:
			return verdictInvertedNoMark
		}
		return verdictNonDefault
	}
	// No marker question to join: below four components the marker inverts
	// nothing, and on a stream that is not a DCT image there is no chain at all.
	return decodeOnlyVerdict(identity, inverted)
}

// decodeOnlyVerdict is the verdict when /Decode is the only switch in play:
// below four components, and on a stream that is not a JPEG at all.
func decodeOnlyVerdict(identity, inverted bool) string {
	switch {
	case identity:
		return verdictNormalDefault
	case inverted:
		return verdictInvertedDecode
	}
	return verdictNonDefault
}

// decodePattern classifies a /Decode array as the identity mapping ([0 1] per
// component), full inversion ([1 0] per component), or neither. An absent array
// is the identity. An array that inverts some components and not others is
// neither, and is never silently called normal.
//
// Both patterns are defined per component, so an array carrying a different
// number of pairs than the image has components matches neither, and neither
// does an odd-length array with a pair left dangling. components is the resolved
// count; a non-empty array with an unresolved count never reaches here, because
// an arity that cannot be checked is its own verdict.
func decodePattern(decode []float64, components int) (identity, inverted bool) {
	if len(decode) == 0 {
		return true, false
	}
	if len(decode) != 2*components {
		return false, false
	}
	identity, inverted = true, true
	for i := 0; i+1 < len(decode); i += 2 {
		lo, hi := decode[i], decode[i+1]
		if lo != 0 || hi != 1 {
			identity = false
		}
		if lo != 1 || hi != 0 {
			inverted = false
		}
	}
	return identity, inverted
}
