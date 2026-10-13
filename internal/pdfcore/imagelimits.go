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
	// high end of what a low-memory machine tolerates.
	//
	// The two roles have different fallbacks when an image is over the ceiling:
	// an image whose Go-decoded preview raster exceeds role 2 but whose stream
	// decoded within role 1 (a high-pixel low-bit-depth scan) is save-only - the
	// preview is skipped but the full-resolution bytes still exist. An image
	// whose stream decode itself exceeds role 1 (a high-bit-depth large image)
	// cannot be rendered OR saved within the limit and is reported as too large;
	// raising that would require raising this budget.
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

	// maxJPEGMarkerSegments and maxJPEGMarkerScanBytes bound the Adobe APP14
	// marker walk. An Adobe record sits near the head of the file - writers put
	// it with the other application segments, ahead of the tables - so neither
	// ceiling is a limit on where a real record is found; they bound a malformed
	// chain that would otherwise be walked to the end of a large stream. A chain
	// that hits either is unparseable, never absent.
	//
	// Both ceilings are sized for the same head of the file, and a CMYK JPEG is
	// what sets that size. Photoshop writes the Adobe record after the APP2 ICC
	// chain and the APP13 resource block, and a CMYK output profile is large -
	// the FOGRA set runs past a megabyte - so a ceiling that stops inside the ICC
	// chain reports the chain unreadable on exactly the well-formed files this
	// walk exists for. A maximal ICC chain is 255 APP2 chunks of up to 65535
	// bytes, near 16.7 MB, and the byte ceiling clears that with room for the
	// resource block and an EXIF thumbnail. The segment ceiling counts every
	// marker the walk steps over, including the standalone ones, so it also
	// bounds the loop, and 512 clears those same 255 chunks with the tables
	// around them. Neither ceiling costs a read: the walk hops segment to segment
	// inside bytes already held in memory and never touches a pixel.
	maxJPEGMarkerSegments  = 512
	maxJPEGMarkerScanBytes = 20 * 1024 * 1024

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
