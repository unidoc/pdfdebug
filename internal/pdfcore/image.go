package pdfcore

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"strconv"
	"strings"

	pdfcpu_render "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

const (
	// maxImageBytes caps the base64-encoded image payload to avoid OOM on
	// pathologically large streams. 50 MB decoded is ~67 MB base64.
	maxImageBytes = 50 * 1024 * 1024
	// maxImagePixels caps total pixel count for in-memory decode (TIFF->PNG).
	// 100 megapixels at 4 bytes/pixel = ~400 MB working set.
	maxImagePixels = 100_000_000
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

	// Set CSComponents before Decode for DCT streams
	if lastFilter == "DCTDecode" {
		err = safeCall(func() error {
			comp, e := pdfcpu_render.ColorSpaceComponents(xrt, &sd)
			if e != nil {
				return e
			}
			sd.CSComponents = comp
			return nil
		})
		if err != nil {
			result.Error = fmt.Sprintf("failed to determine color space components: %v", err)
			return result, nil
		}
	}

	// Decode the stream
	if sd.FilterPipeline != nil {
		err = safeCall(func() error {
			return sd.Decode()
		})
		if err != nil {
			result.Error = fmt.Sprintf("failed to decode image stream: %v", err)
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

// appendWarning joins warnings with "; " so multiple non-fatal issues are visible.
func appendWarning(existing, addition string) string {
	if existing == "" {
		return addition
	}
	return existing + "; " + addition
}
