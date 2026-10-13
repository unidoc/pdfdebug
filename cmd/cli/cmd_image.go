package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runImageDump parses flags and dispatches image-data dump execution.
func runImageDump(args []string) int {
	filePath, f, ok := parseByRefFlags("image", args, false, true)
	if !ok {
		return 1
	}
	return execImageDump(filePath, f)
}

// execImageDump opens the PDF and emits the ImageData payload as JSON. A
// non-image node yields ImageData with a populated `error` field at exit 0 (the
// method reports type mismatch in the payload, not via a Go error). With
// --metadata the base64 key is omitted entirely (not blanked) so the output
// stays small; literal omission requires a projection because ImageData.Base64
// has no omitempty tag.
func execImageDump(filePath string, f byRefFlags) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins, nodeID, _, code := openByRef(f.ref, filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	img, err := ins.GetImageData(context.Background(), "cli", nodeID)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	if !f.json {
		// Plain-text default: an aligned key/value block. The base64 payload is
		// never printed in plain text (it would flood the terminal); --metadata is
		// therefore a JSON-only distinction. NON-CONTRACTUAL; use --json to parse.
		if err := printImagePlain(os.Stdout, img); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}

	if f.metadata {
		// Project to a map and drop the base64 key entirely. Decode into
		// json.RawMessage (not any) so the surviving fields re-emit byte-for-byte
		// -- decoding into map[string]any would relabel every number as float64.
		b, err := json.Marshal(img)
		if err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		delete(m, "base64")
		if err := emit(os.Stdout, m, f.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}

	// The CLI emits the FULL-resolution image, not the GUI's downsampled
	// preview. GetImageData's Base64 is a thumbnail; replace it with the
	// full-resolution bytes so the base64 matches the reported dimensions.
	if img.Error == "" && img.Base64 != "" {
		if full, ext, berr := ins.GetImageBytes("cli", nodeID); berr == nil {
			img.Base64 = base64.StdEncoding.EncodeToString(full)
			img.MimeType = "image/png"
			if ext == ".jpg" {
				img.MimeType = "image/jpeg"
			}
			// The CLI ships the full-resolution image, so the reported preview
			// dimensions equal the real dimensions (no downsampling here).
			img.ThumbWidth = img.Width
			img.ThumbHeight = img.Height
		}
	}

	if err := emit(os.Stdout, img, f.pretty); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printImagePlain renders ImageData as an aligned key/value block (omitting the
// base64 payload). A populated error/warning field is surfaced as its own row.
//
// Error does not suppress the block. A 4-component DCT stream with no Adobe
// APP14 is refused by the JPEG decoder, and a /DeviceN whose colorant array is
// an indirect reference fails the colour-space lookup: both arrive with Error
// set after the geometry has been read, so stopping there would print an error
// and nothing else on exactly the files this view exists for.
//
// The empty object reference discriminates the node whose image dictionary was
// never read - an error node, a non-stream object, or a wrong /Subtype - where
// there is genuinely nothing else to print. Those returns build an ImageData
// without ObjectRef; every path that reaches the dictionary sets it, so the test
// is true by construction rather than a side effect of how far the read got.
//
// The verdict row is unconditional: "checked, nothing here" is a real answer and
// has to be distinguishable from "the tool did not look". The structural rows
// are conditional, because nobody opens this view asking about them.
func printImagePlain(out io.Writer, img *pdfcore.ImageData) error {
	var w kvWriter
	w.Add("Object", img.ObjectRef)
	if img.Error != "" {
		// pdfcpu's messages end with a newline. The row is no longer the last one
		// in the block, so an untrimmed value opens a blank line inside it.
		w.Add("Error", strings.TrimSpace(img.Error))
		if img.ObjectRef == "" {
			return w.Render(out)
		}
	}
	// MimeType is set by the render, so it is empty on every row the error arm
	// falls through to. The dash is what this writer says for a value that is
	// not there.
	w.Add("MimeType", dashIfEmpty(img.MimeType))
	w.Addf("Width", "%d", img.Width)
	w.Addf("Height", "%d", img.Height)
	w.Add("ColorSpace", dashIfEmpty(img.ColorSpace))
	w.Add("BitsPerComponent", strconv.Itoa(img.BitsPerComponent))
	w.Add("Filter", dashIfEmpty(img.Filter))
	if size := imageSizeText(img); size != "" {
		w.Add("Size", size)
	}
	w.Add("Interpretation", dashIfEmpty(img.SampleInterpretation))
	if len(img.Decode) > 0 {
		w.Add("Decode", formatDecodeArray(img.Decode))
	}
	// The marker outcome is shown for every DCT image, not only where a record
	// was found: "absent", "unparseable" and "not-examined" are answers a reader
	// acts on, and below four components the marker never enters the verdict, so
	// without this row an unreadable chain leaves no trace outside --json. The
	// empty zero value means the dictionary was never read; "not-applicable"
	// means the stream is not a JPEG, and neither is a marker outcome to report.
	if img.AdobeMarker != "" && img.AdobeMarker != pdfcore.AdobeMarkerNotApplicable {
		w.Add("AdobeMarker", img.AdobeMarker)
	}
	if img.AdobeMarker == pdfcore.AdobeMarkerPresent && img.AdobeTransform != nil {
		w.Add("AdobeTransform", adobeTransformText(*img.AdobeTransform))
	}
	if img.SMask != nil {
		w.Add("SMask", smaskText(*img.SMask))
	}
	if img.ImageMask {
		w.Add("ImageMask", "true")
	}
	if img.Warning != "" {
		w.Add("Warning", img.Warning)
	}
	return w.Render(out)
}

// imageSizeText composes the stored size with the decoded estimate after it,
// each half only when it is non-zero, matching how the panel composes the same
// row. The two surfaces use different precision for the decoded half and are not
// expected to match byte for byte.
func imageSizeText(img *pdfcore.ImageData) string {
	decoded := ""
	if img.DecodedBytes > 0 {
		decoded = "~" + humanizeBytes(img.DecodedBytes) + " in memory"
	}
	if img.StoredBytes <= 0 {
		return decoded
	}
	if decoded == "" {
		return humanizeBytes(img.StoredBytes)
	}
	return humanizeBytes(img.StoredBytes) + " (" + decoded + ")"
}

// formatDecodeArray renders a /Decode array as its literal space-separated
// numbers, so the row is the evidence behind the verdict rather than a summary
// of it.
func formatDecodeArray(values []float64) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	return strings.Join(parts, " ")
}

// smaskText renders the image /SMask value. An empty reference means the key is
// present with nothing to point at - a direct stream, or the /None name writers
// borrow from the ExtGState entry - and is said in words rather than as the dash
// this writer uses for a value that is not there at all.
func smaskText(ref string) string {
	if ref == "" {
		return "present (no reference)"
	}
	return ref
}

// adobeTransformText renders an Adobe APP14 transform byte with its meaning
// beside the number, never as a bare number: the number only helps a reader who
// already knows what it means.
func adobeTransformText(transform int) string {
	name := "Unknown"
	switch transform {
	case 0:
		name = "None"
	case 1:
		name = "YCbCr"
	case 2:
		name = "YCCK"
	}
	return fmt.Sprintf("%s (transform %d)", name, transform)
}
