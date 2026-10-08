# Image and font commands

The commands for image XObjects and fonts: the image index, one image's data
and sample interpretation, and a font's encoding and glyph mapping.

Back to the [CLI usage guide](../cli-usage.md).

### `dump images`

`pdfdebug dump images [--json] [--pretty] <file>`

Prints one row per image XObject object, however many pages use it: its
reference, `WxH`, BitsPerComponent, colour space, filters, a FLAGS column
(`mask`, `SMask`, `Decode` for a `/Decode` that is not the colour space's
default, `APP14 t=N`; the default is `[0 1]` per component, `[0 2^bpc-1]` for
Indexed, and `[0 100]` plus the `/Range` for Lab), the decoded size the
dictionary implies, and PAGES as `<count>: <pages>`, the page list capped with
`, ...`. Rows come in first-use page order, then by object number.

An image counts as used on a page when it is referenced from the page's
resources, inherited ones and nested Form XObjects included; content streams
are not read, so it is not checked against `Do` operators. Inline images
(`BI`/`ID`/`EI`) are not listed. No image is decoded.

A row with `-` for the reference reports each problem in ERROR, and the command
still exits 0. An unreadable resources dictionary, `/XObject` entry or form gets
one row naming every page it affected. A Form XObject nested deeper than 32 is
skipped along with the forms below it, with one row naming that form and its
pages, and the walk carries on. Only the walk's budgets stop it: past 1,000,000
resource entries or 4,000,000 image uses the walk ends, and that row names the
pages left unwalked. An unreadable page tree also gets a row, naming the last
page reached. A warning on a readable image, such as a rejected `/Decode`,
shows in ERROR as `warning: ...`, after the image's error when it has one.

### `dump image`

`pdfdebug dump image [--json] [--pretty] [--metadata] --ref "N G R" <file>`

Prints an image XObject's dictionary fields, its stored and decoded sizes, and
how its samples are interpreted. `--json` includes the image data as base64;
`--metadata` leaves it out.

`Interpretation` is one verdict on whether the samples are read inverted. It
joins two switches that live in different layers: the `/Decode` array in the
PDF and the Adobe APP14 transform inside a JPEG. An Adobe CMYK JPEG stores its
channels inverted, so on a four-component stream a present marker with no
`/Decode` reads as inverted and `/Decode [1 0 1 0 1 0 1 0]` compensates. Below
four components the verdict comes from `/Decode` alone.

The other rows print only when they apply. `Decode` shows the array when the
dictionary sets one. `AdobeMarker` prints for every JPEG and reads `present`,
`absent`, `unparseable` or `not-examined` (DCTDecode behind another filter);
it is left out for other filters. `AdobeTransform` follows a present marker,
`SMask` names the soft-mask reference, `ImageMask` appears for a stencil mask,
and `Warning` carries a dictionary problem. Finding the marker reads no pixel:
the walk hops from segment to segment and stops at the Adobe record or at SOS.
In `--json` the same fields are `sampleInterpretation`, `decode`,
`adobeMarker`, `adobeTransform`, `smask` and `imageMask`.

When the stream fails to decode, the error row comes first and the dictionary
fields still follow it.

### `dump font`

`pdfdebug dump font [--json] [--pretty] [--glyphs] --ref "N G R" <file>`

Shows a font's encoding, CMap and glyph mapping with health signals for missing
or broken mappings. `--glyphs` prints the full per-code mapping table.
