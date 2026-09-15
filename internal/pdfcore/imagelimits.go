package pdfcore

// Image extraction and preview limits, centralized. Each constant bounds a
// distinct stage of the image pipeline; the comments note how they relate so a
// change to one is made with the others in view.
//
// Pipeline: renderImage decodes the stream (bounded by imageDecodeCeiling, which
// is floored at maxImageBytes and capped at maxImageDecodeBytes) and reads the
// full-resolution encoded bytes (capped at maxImageDecodeBytes). makeThumbnail
// then downsamples a source larger than maxThumbnailEdge, gated by the decoded
// footprint against maxImageDecodeBytes, and re-encodes the preview (JPEG at
// jpegThumbnailQuality for opaque results, PNG otherwise).
const (
	// maxImageBytes is the floor for imageDecodeCeiling: a small image is never
	// held to a decode bound tighter than this.
	maxImageBytes = 50 * 1024 * 1024

	// maxImagePixels caps total pixel count for the in-memory TIFF->PNG re-encode
	// in renderImage. 100 megapixels at 4 bytes/pixel = ~400 MB working set.
	maxImagePixels = 100_000_000

	// maxImageDecodeBytes serves two roles, both bounding a decoded raster to the
	// same 512 MB ceiling:
	//   1. The absolute ceiling on a decoded image stream in imageDecodeCeiling,
	//      whatever geometry the dictionary declares, so an absurd /Width or
	//      /Height cannot authorize an unbounded allocation. It also caps the
	//      full-resolution encoded read in renderImage.
	//   2. The decoded-footprint budget makeThumbnail checks before decoding a
	//      source into memory to downsample it (pixels x bytesPerPixel).
	// Because the encoded render is still held while makeThumbnail decodes, peak
	// memory during preview build approaches encoded + decoded, so this is the
	// high end of what a low-memory machine tolerates. An image whose decoded
	// footprint exceeds it is not previewed inline (save-only).
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

	// maxThumbnailEdge bounds the longer side of the downsampled preview shipped
	// over IPC. Fixed, not panel-relative, so GetImageData stays a pure function
	// of the document. A source within this bound on both sides ships unchanged.
	maxThumbnailEdge = 2048

	// jpegThumbnailQuality is the JPEG encoder quality (1-100) for a downsampled
	// preview whose scaled result is opaque. Balanced for line-art and text
	// scans, where JPEG ringing shows more than on photos: 80 stays legible while
	// cutting the payload well below PNG. A preview with transparency is PNG
	// (JPEG has no alpha). Only the downsampled path re-encodes; images within
	// maxThumbnailEdge ship their original bytes untouched.
	jpegThumbnailQuality = 80
)

// Outcome discriminators for ImageData.Kind. The frontend branches the three
// cases on this rather than parsing Error: an ordinary preview, the lying-stream
// geometry-ceiling refusal (a finding, not offered for consent), and any other
// per-image failure.
const (
	imageKindOK             = "ok"
	imageKindCeilingRefusal = "ceiling-refusal"
	imageKindError          = "error"
)
