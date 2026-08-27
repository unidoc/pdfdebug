package pdfcore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"strconv"
	"strings"

	pdfcpu_render "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

const (
	// maxImageBytes caps the base64-encoded image payload to avoid OOM on
	// pathologically large streams. 50 MB decoded is ~67 MB base64.
	maxImageBytes = 50 * 1024 * 1024
	// maxImagePixels caps total pixel count for in-memory decode (TIFF->PNG).
	// 100 megapixels at 4 bytes/pixel = ~400 MB working set.
	maxImagePixels = 100_000_000
	// maxImageDecodeBytes is the absolute ceiling on a decoded image stream,
	// whatever geometry its dictionary declares, so an absurd /Width or /Height
	// cannot authorize an unbounded allocation. Aligned with maxImagePixels,
	// which already sanctions a ~400 MB working set.
	maxImageDecodeBytes = 512 * 1024 * 1024
	// maxBitsPerComponent and maxComponents bound a plausible image sample: 16 is
	// the widest depth PDF defines and DeviceN carries at most 32 colorants.
	// Beyond either the dictionary is malformed rather than large.
	maxBitsPerComponent = 16
	maxComponents       = 32
	// imageDecodeHeadroom is added to the size the declared geometry implies,
	// covering the framing pdfcpu's gob encoding puts around a 4-component DCT
	// image. Only large images see it: below the maxImageBytes floor it is
	// subsumed, and there the floor is the more generous of the two anyway.
	imageDecodeHeadroom = 1024 * 1024
)

// GetImageData extracts and encodes an image from the given XObject Image node.
// Returns metadata and base64-encoded image data for frontend display.
func (ins *Inspector) GetImageData(tabID, nodeID string) (*ImageData, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrDocumentNotFound)
	}

	if strings.HasPrefix(nodeID, "error:") {
		return &ImageData{
			NodeID: nodeID,
			Error:  "cannot extract image for error node",
		}, nil
	}

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access. Image extraction dereferences indirect refs
	// (Subtype, Filter, ColorSpace) and reads pdfcpu's XRefTable.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	var obj pdfcpu_types.Object
	err = safeCall(func() error {
		var e error
		obj, e = resolveNodeObject(doc, nodeID)
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	// Must be a StreamDict (not ObjectStreamDict, XRefStreamDict, Dict, etc.)
	sd, ok := obj.(pdfcpu_types.StreamDict)
	if !ok {
		return &ImageData{
			NodeID: nodeID,
			Error:  "not an image XObject",
		}, nil
	}

	// Verify Subtype == Image
	subtypeObj, found := sd.Find("Subtype")
	if !found {
		return &ImageData{
			NodeID: nodeID,
			Error:  "not an image XObject",
		}, nil
	}
	subtypeName, ok := subtypeObj.(pdfcpu_types.Name)
	if !ok || string(subtypeName) != "Image" {
		actual := fmt.Sprintf("%v", subtypeObj)
		if ok {
			actual = string(subtypeName)
		}
		return &ImageData{
			NodeID: nodeID,
			Error:  fmt.Sprintf("not an image XObject (Subtype: %s)", actual),
		}, nil
	}

	result := &ImageData{
		NodeID:    nodeID,
		ObjectRef: objectRefFromNodeID(nodeID),
	}

	// Extract metadata -- wrap each pdfcpu call in safeCall.
	xrt := doc.PDFContext.XRefTable

	// Width
	err = safeCall(func() error {
		wObj, found := sd.Find("Width")
		if !found {
			return nil
		}
		i, e := xrt.DereferenceInteger(wObj)
		if e != nil {
			return e
		}
		if i != nil {
			result.Width = int(*i)
		}
		return nil
	})
	if err != nil {
		result.Warning = appendWarning(result.Warning, fmt.Sprintf("width metadata: %v", err))
	}

	// Height
	err = safeCall(func() error {
		hObj, found := sd.Find("Height")
		if !found {
			return nil
		}
		i, e := xrt.DereferenceInteger(hObj)
		if e != nil {
			return e
		}
		if i != nil {
			result.Height = int(*i)
		}
		return nil
	})
	if err != nil {
		result.Warning = appendWarning(result.Warning, fmt.Sprintf("height metadata: %v", err))
	}

	// BitsPerComponent -- use DereferenceInteger to handle IndirectRef values,
	// matching the pattern for Width/Height.
	result.BitsPerComponent = 8 // default for ImageMask or missing entry
	err = safeCall(func() error {
		bpcObj, found := sd.Find("BitsPerComponent")
		if !found {
			return nil
		}
		i, e := xrt.DereferenceInteger(bpcObj)
		if e != nil {
			return e
		}
		if i != nil {
			result.BitsPerComponent = int(*i)
		}
		return nil
	})
	if err != nil {
		result.Warning = appendWarning(result.Warning, fmt.Sprintf("bitsPerComponent metadata: %v", err))
	}

	// ColorSpace
	err = safeCall(func() error {
		csObj, found := sd.Find("ColorSpace")
		if !found {
			return nil
		}
		deref, e := xrt.Dereference(csObj)
		if e != nil {
			return e
		}
		switch cs := deref.(type) {
		case pdfcpu_types.Name:
			result.ColorSpace = string(cs)
		case pdfcpu_types.Array:
			if len(cs) > 0 {
				if n, ok := cs[0].(pdfcpu_types.Name); ok {
					result.ColorSpace = string(n)
				}
			}
		}
		return nil
	})
	if err != nil {
		result.Warning = appendWarning(result.Warning, fmt.Sprintf("colorSpace metadata: %v", err))
	}

	// Filter
	if sd.FilterPipeline != nil {
		names := make([]string, len(sd.FilterPipeline))
		for i, f := range sd.FilterPipeline {
			names[i] = f.Name
		}
		result.Filter = strings.Join(names, ",")
	}

	// CMYK warning
	if result.ColorSpace == "DeviceCMYK" || strings.Contains(result.ColorSpace, "CMYK") {
		result.Warning = appendWarning(result.Warning, "Image uses CMYK color space (colors may be inaccurate)")
	}

	// Determine last filter for CSComponents setup
	lastFilter := ""
	if len(sd.FilterPipeline) > 0 {
		lastFilter = sd.FilterPipeline[len(sd.FilterPipeline)-1].Name
	}

	// Set CSComponents before Decode for DCT streams. The lookup asserts types and
	// dereferences unchecked, so a colour space it cannot read faults it, and
	// safeCall re-panics a runtime error by design. Absorbing it here keeps one
	// unreadable image to a per-image error instead of ending the request. A
	// well-formed document can reach this: a /DeviceN whose colorant array is an
	// indirect reference opens cleanly and then fails the Array assertion.
	if lastFilter == "DCTDecode" {
		err = func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("color space could not be resolved: %v", r)
				}
			}()
			return safeCall(func() error {
				comp, e := pdfcpu_render.ColorSpaceComponents(xrt, &sd)
				if e != nil {
					return e
				}
				sd.CSComponents = comp
				return nil
			})
		}()
		if err != nil {
			result.Error = fmt.Sprintf("failed to determine color space components: %v", err)
			return result, nil
		}
	}

	// Decode under a ceiling derived from the geometry the dictionary declares,
	// so a compressed bitmap cannot inflate far past the size it claims before
	// it is rejected. A large image that is honest about its dimensions still
	// decodes; only a stream inflating well beyond its own declaration stops.
	if sd.FilterPipeline != nil {
		ceiling := imageDecodeCeiling(result.Width, result.Height, result.BitsPerComponent,
			declaredComponents(xrt, &sd))
		if _, err := decodeBounded(&sd, ceiling); err != nil {
			if errors.Is(err, ErrUnsupportedPDF) {
				result.Error = fmt.Sprintf("image data too large (exceeds the %d byte ceiling for a %dx%d image)",
					ceiling, result.Width, result.Height)
			} else {
				result.Error = fmt.Sprintf("failed to decode image stream: %v", err)
			}
			return result, nil
		}
	} else {
		// No filters -- raw content is the image data.
		if sd.Content == nil {
			if sd.Raw == nil {
				result.Error = "empty image stream"
				return result, nil
			}
			sd.Content = sd.Raw
		}
	}

	// Extract object number from nodeID for RenderImage.
	// For non-obj nodes lastPart may not be numeric; fall back to 0.
	_, _, lastPart := parseNodeID(nodeID)
	objNr, parseErr := strconv.Atoi(lastPart)
	if parseErr != nil {
		objNr = 0
	}

	// RenderImage
	var reader io.Reader
	var format string
	err = safeCall(func() error {
		var e error
		reader, format, e = pdfcpu_render.RenderImage(xrt, &sd, false, "", objNr)
		return e
	})
	if err != nil {
		result.Error = fmt.Sprintf("failed to render image: %v", err)
		return result, nil
	}
	if reader == nil {
		result.Error = fmt.Sprintf("unsupported image format: %s", result.Filter)
		return result, nil
	}

	// Handle format
	switch format {
	case "jpg":
		result.MimeType = "image/jpeg"
	case "png":
		result.MimeType = "image/png"
	case "jpx":
		result.Error = "unsupported image format: JPEG 2000 (JPX)"
		return result, nil
	case "tif":
		// Guard against huge images that would OOM during decode+re-encode.
		if result.Width > 0 && result.Height > 0 &&
			int64(result.Width)*int64(result.Height) > maxImagePixels {
			result.Error = fmt.Sprintf("image too large for re-encoding (%dx%d pixels)", result.Width, result.Height)
			return result, nil
		}
		// Re-encode TIFF to PNG for browser display
		var img image.Image
		err = safeCall(func() error {
			var e error
			img, _, e = image.Decode(reader)
			return e
		})
		if err != nil {
			result.Error = fmt.Sprintf("failed to decode TIFF for re-encoding: %v", err)
			return result, nil
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			result.Error = fmt.Sprintf("failed to re-encode TIFF as PNG: %v", err)
			return result, nil
		}
		if buf.Len() > maxImageBytes {
			result.Error = fmt.Sprintf("image data too large (>%d MB)", maxImageBytes/(1024*1024))
			return result, nil
		}
		result.Base64 = base64.StdEncoding.EncodeToString(buf.Bytes())
		result.MimeType = "image/png"
		return result, nil
	default:
		result.Error = fmt.Sprintf("unsupported image format: %s", format)
		return result, nil
	}

	// Read image bytes and base64-encode, capped to avoid OOM.
	imgBytes, err := io.ReadAll(io.LimitReader(reader, maxImageBytes+1))
	if err != nil {
		result.Error = fmt.Sprintf("failed to read image data: %v", err)
		return result, nil
	}
	if len(imgBytes) > maxImageBytes {
		result.Error = fmt.Sprintf("image data too large (>%d MB)", maxImageBytes/(1024*1024))
		return result, nil
	}
	result.Base64 = base64.StdEncoding.EncodeToString(imgBytes)

	return result, nil
}

// declaredComponents returns the colour-component count pdfcpu derives from the
// image's /ColorSpace, falling back to four when it cannot derive one. A DCT
// stream already had it resolved for the decode, so that value is reused. The
// count only sizes the decode ceiling, so an unresolvable or unrecognised colour
// space widens the estimate rather than collapsing it: collapsing would pin a
// large image to the smallest ceiling and refuse an extraction that works today.
func declaredComponents(xrt *pdfcpu_model.XRefTable, sd *pdfcpu_types.StreamDict) (components int) {
	// An unresolved colour space must widen the estimate, not tighten it, or the
	// ceiling refuses an extraction that would otherwise work. The widest a PDF
	// colour space goes is 32, and the shape that most often lands here is a
	// /DeviceN whose colorant array is an indirect reference - precisely the
	// space that carries many components. Guessing four would leave room for
	// about eight after the doubling and reject the rest as oversized.
	const unknownComponents = maxComponents
	if sd.CSComponents > 0 {
		return sd.CSComponents
	}
	components = unknownComponents
	// Absorb panics as well as errors, via the named return so a recovered panic
	// yields the fallback rather than a zero. pdfcpu's lookup asserts types and
	// dereferences without checking, so a colour space it cannot read faults it,
	// and safeCall re-panics a runtime error by design. Absorbing is safe HERE and
	// nowhere else in this file: the result is only a size hint, every failure
	// answers with the wider fallback, and no byte returned to the caller depends
	// on it.
	defer func() {
		if recover() != nil {
			components = unknownComponents
		}
	}()
	if err := safeCall(func() error {
		c, e := pdfcpu_render.ColorSpaceComponents(xrt, sd)
		if e == nil && c > 0 {
			components = c
		}
		return e
	}); err != nil {
		return unknownComponents
	}
	return components
}

// imageDecodeCeiling returns the byte ceiling for decoding an image stream. The
// dictionary's geometry states how big the samples should be, so that size plus
// headroom bounds an honest image while stopping a stream that inflates well
// past what it declares.
//
// Geometry that is missing, non-positive, implausible or wide enough to wrap the
// arithmetic falls back to maxImageBytes. Being merely large does not: a 108
// megapixel scan is a real document and gets room for its samples. The result is
// floored at maxImageBytes so a small picture is never held to a tighter bound
// than the extraction path already allowed, and never exceeds
// maxImageDecodeBytes.
//
// The residual this leaves: a stream that OVERSTATES its dimensions raises its
// own ceiling, up to maxImageDecodeBytes. That is bounded and survivable where
// the unguarded decode was neither, but it is not the tight bound an honestly
// declared image gets, and there is no pre-decode signal that separates a
// generous declaration from a false one.
func imageDecodeCeiling(width, height, bitsPerComponent, components int) int64 {
	if width <= 0 || height <= 0 ||
		bitsPerComponent <= 0 || bitsPerComponent > maxBitsPerComponent ||
		components <= 0 || components > maxComponents {
		return maxImageBytes
	}
	// A side longer than this is not a picture, and a dimension that cannot be
	// trusted must not buy more room to inflate than an honest one gets, so it
	// falls back to maxImageBytes. Being merely LARGE is not grounds for this:
	// a 108 megapixel scan is a real document, and maxImageDecodeBytes bounds
	// the big cases. The bound also keeps every product below from overflowing.
	const maxImageDimension = 1 << 20
	if width > maxImageDimension || height > maxImageDimension {
		return maxImageBytes
	}
	// PDF pads every ROW to a byte boundary, so the samples occupy
	// ceil(width*bitsPerPixel/8) per row rather than a fraction of a byte.
	// width*height*bitsPerPixel/8 understates a narrow sub-byte image.
	bitsPerPixel := int64(bitsPerComponent) * int64(components)
	rowBytes := (int64(width)*bitsPerPixel + 7) / 8
	declared := rowBytes * int64(height)
	// Double it so packing and framing slack cannot reject an honest image,
	// floor at maxImageBytes so a small picture is never held to a tighter
	// ceiling than the extraction path already allowed, and cap the result.
	ceiling := declared*2 + imageDecodeHeadroom
	if ceiling < maxImageBytes {
		return maxImageBytes
	}
	if ceiling > maxImageDecodeBytes {
		return maxImageDecodeBytes
	}
	return ceiling
}

// appendWarning joins warnings with "; " so multiple non-fatal issues are visible.
// Only used by GetImageData for image-metadata warnings; do not promote to a shared utility without a second caller.
func appendWarning(existing, addition string) string {
	if existing == "" {
		return addition
	}
	return existing + "; " + addition
}
