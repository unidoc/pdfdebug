/**
 * @file FontPreview -- presentational component for the Story 9-9 font
 * inspection view. Renders the consolidated FontDetail payload (metadata
 * header, encoding section, ToUnicode table, FontDescriptor card, optional
 * descendant section) returned by GetFontDetail.
 *
 * Pure presentational: receives all data plus an onReferenceClick handler.
 * No Wails calls, no useAppDispatch. Mirrors the ImagePreview pattern.
 */

/** Decoded FontDescriptor info matching backend FontDescriptorInfo. */
export interface FontDescriptorInfoData {
  nodeId: string;
  objectRef: string;
  fontName: string;
  flags: number;
  flagNames: string[];
  italicAngle: number;
  ascent: number;
  descent: number;
  capHeight: number;
  stemV: number;
  fontBBox: number[];
  fontFileFormat: string;
  fontFileSize: number;
}

/** One row in an /Encoding /Differences table. */
export interface EncodingDifferenceData {
  code: number;
  glyphName: string;
}

/** One row in a /ToUnicode CMap mapping table. */
export interface ToUnicodeMappingData {
  code: number;
  unicode: string;
  glyph: string;
}

/** /CIDSystemInfo triplet for CIDFont descendants. */
export interface CIDSystemInfoData {
  registry: string;
  ordering: string;
  supplement: number;
}

/** Full payload returned by GetFontDetail. */
export interface FontDetailData {
  nodeId: string;
  objectRef: string;
  subtype: string;
  baseFont: string;
  firstChar: number;
  lastChar: number;
  encodingName: string;
  baseEncoding: string;
  differences: EncodingDifferenceData[];
  toUnicodeMappings: ToUnicodeMappingData[];
  toUnicodeError: string;
  embedded: boolean;
  fontDescriptor: FontDescriptorInfoData | null;
  descendant: FontDetailData | null;
  /** CIDFont-only (Subtype CIDFontType0 / CIDFontType2). Null on parent
   *  Type0 / non-composite fonts. AC7. */
  cidSystemInfo: CIDSystemInfoData | null;
  /** "Identity" for the Name form or "Stream (<N> bytes)" for the stream
   *  form. Empty when /CIDToGIDMap is absent. */
  cidToGIDMap: string;
  /** /DW default width for CIDFonts. 0 when absent. */
  defaultWidth: number;
}

/** PDF 1.7 spec section 9.8.2 Table 123 -- valid flag names. Anything outside
 *  this allowlist is filtered (frontend regression guard against a buggy
 *  backend release leaking reserved-bit names like "Reserved5"). */
const VALID_FLAG_NAMES = new Set([
  'FixedPitch',
  'Serif',
  'Symbolic',
  'Script',
  'Nonsymbolic',
  'Italic',
  'AllCap',
  'SmallCap',
  'ForceBold',
]);

/** Returns a human-readable file size: "14.6 KB" / "2.3 MB" / "512 bytes". */
function formatBytes(n: number): string {
  if (n <= 0) return '0 bytes';
  if (n < 1024) return `${n} bytes`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

/** Returns hex form of a non-negative int with at least 2 digits, "0x" prefix. */
function toHex(n: number): string {
  const h = n.toString(16).toUpperCase();
  return `0x${h.length < 2 ? '0' + h : h}`;
}

/** Returns the IndirectRef nodeID derived from an "N G R" string,
 *  or "" when the string is not in that form. */
function nodeIdFromRef(ref: string): string {
  const m = /^(\d+)\s+(\d+)\s+R$/.exec(ref);
  if (!m) return '';
  return `obj:${m[2]}:${m[1]}`;
}

/** A clickable indirect-ref token. Inline element with keyboard activation
 *  (Enter/Space) matching the existing ValueDisplay pattern in DetailShared. */
function RefToken({
  refText,
  target,
  onClick,
}: {
  refText: string;
  target: string;
  onClick: (target: string) => void;
}) {
  return (
    <span
      role="button"
      tabIndex={0}
      className="font-mono text-xs text-type-reference underline cursor-pointer"
      data-ref-target={target}
      onClick={() => onClick(target)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onClick(target);
        }
      }}
    >
      {refText}
    </span>
  );
}

/** Renders the embedded badge. Green chip + format + size when embedded,
 *  red chip with viewer-fallback warning otherwise. AC2 contract. */
function EmbeddedBadge({
  embedded,
  format,
  sizeBytes,
}: {
  embedded: boolean;
  format: string;
  sizeBytes: number;
}) {
  if (embedded && format) {
    return (
      <span
        className="inline-flex items-center px-2 py-0.5 text-xs rounded bg-success/10 text-success font-medium"
        data-testid="font-embedded-badge"
      >
        Embedded ({format}, {formatBytes(sizeBytes)})
      </span>
    );
  }
  return (
    <span
      className="inline-flex items-center px-2 py-0.5 text-xs rounded bg-error/10 text-error font-medium"
      data-testid="font-embedded-badge"
    >
      Not embedded -- viewer falls back to system font
    </span>
  );
}

/** Encoding section: name OR built-in sentinel OR /Differences table. */
function EncodingSection({ detail }: { detail: FontDetailData }) {
  const hasDifferences = detail.differences.length > 0;
  const hasNamed = detail.encodingName !== '';
  const hasBaseEncoding = detail.baseEncoding !== '';

  return (
    <section className="border-t border-border p-3 text-xs" data-testid="font-encoding-section">
      <div className="text-text-secondary font-medium mb-1">Encoding</div>
      {hasNamed && !hasDifferences && (
        <div className="font-mono text-text">{detail.encodingName}</div>
      )}
      {/* BaseEncoding-only: /Encoding is a dict with /BaseEncoding but no
          /Differences. Surface the BaseEncoding rather than fall through to
          the "Built-in" sentinel, which would be misleading. */}
      {!hasNamed && !hasDifferences && hasBaseEncoding && (
        <div className="font-mono text-text-muted">
          BaseEncoding: <span className="text-text">{detail.baseEncoding}</span>
        </div>
      )}
      {!hasNamed && !hasDifferences && !hasBaseEncoding && (
        <div className="text-text-muted">
          Built-in encoding (defined in font file)
        </div>
      )}
      {hasDifferences && (
        <>
          {/* Defensive: when a malformed font dict carries BOTH /Encoding as a
              Name and /Differences (the backend would surface this by populating
              encodingName alongside differences), surface the name so the user
              sees the conflict rather than silently dropping it. */}
          {hasNamed && (
            <div className="font-mono text-text-muted mb-1">
              Encoding: <span className="text-text">{detail.encodingName}</span>
            </div>
          )}
          {hasBaseEncoding && (
            <div className="font-mono text-text-muted mb-1">
              BaseEncoding: <span className="text-text">{detail.baseEncoding}</span>
            </div>
          )}
          <div className="overflow-auto max-h-64 border border-border rounded">
            <table className="w-full text-xs">
              <thead className="bg-hover">
                <tr>
                  <th className="text-left px-2 py-1 text-text-secondary font-medium">Code (decimal)</th>
                  <th className="text-left px-2 py-1 text-text-secondary font-medium">Code (hex)</th>
                  <th className="text-left px-2 py-1 text-text-secondary font-medium">Glyph name</th>
                </tr>
              </thead>
              <tbody>
                {detail.differences.map((d, i) => (
                  <tr key={i} className="border-t border-border">
                    <td className="px-2 py-1 font-mono">{d.code}</td>
                    <td className="px-2 py-1 font-mono text-text-muted">{toHex(d.code)}</td>
                    <td className="px-2 py-1 font-mono text-type-name">{d.glyphName}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </section>
  );
}

/** ToUnicode section: table of code -> U+XXXX -> glyph, with AC9a warning
 *  panel rendered above the table when toUnicodeError is set. */
function ToUnicodeSection({ detail }: { detail: FontDetailData }) {
  const hasMappings = detail.toUnicodeMappings.length > 0;
  const hasError = detail.toUnicodeError !== '';
  if (!hasMappings && !hasError) return null;

  return (
    <section className="border-t border-border p-3 text-xs" data-testid="font-tounicode-section">
      <div className="text-text-secondary font-medium mb-1">ToUnicode CMap</div>
      {hasError && (
        <div className="px-2 py-1 text-warning mb-1" data-testid="font-tounicode-error">
          ToUnicode present but unparseable: {detail.toUnicodeError}
        </div>
      )}
      {hasMappings && (
        <div className="overflow-auto max-h-64 border border-border rounded">
          <table className="w-full text-xs">
            <thead className="bg-hover">
              <tr>
                <th className="text-left px-2 py-1 text-text-secondary font-medium">Code (hex)</th>
                <th className="text-left px-2 py-1 text-text-secondary font-medium">Unicode</th>
                <th className="text-left px-2 py-1 text-text-secondary font-medium">Glyph</th>
              </tr>
            </thead>
            <tbody>
              {detail.toUnicodeMappings.map((m, i) => (
                <tr key={i} className="border-t border-border">
                  <td className="px-2 py-1 font-mono text-text-muted">{toHex(m.code)}</td>
                  <td className="px-2 py-1 font-mono">{m.unicode}</td>
                  <td className="px-2 py-1 font-mono">{m.glyph}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

/** FontDescriptor card: name, decoded flag pills, metric grid, FontFile format. */
function FontDescriptorCard({
  descriptor,
  onReferenceClick,
}: {
  descriptor: FontDescriptorInfoData;
  onReferenceClick: (target: string) => void;
}) {
  // Filter reserved-bit labels defensively (frontend regression guard).
  const filteredFlags = descriptor.flagNames.filter((n) => VALID_FLAG_NAMES.has(n));

  return (
    <section className="border-t border-border p-3 text-xs" data-testid="font-descriptor-section">
      <div className="text-text-secondary font-medium mb-1">FontDescriptor</div>
      <dl className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1 font-mono">
        <dt className="text-text-muted">FontName:</dt>
        <dd>
          {descriptor.fontName}
        </dd>
        <dt className="text-text-muted">Flags:</dt>
        <dd>
          {filteredFlags.length > 0 ? (
            <span className="inline-flex flex-wrap gap-1">
              {filteredFlags.map((name) => (
                <span
                  key={name}
                  className="px-1.5 py-0.5 rounded bg-hover text-text-secondary"
                  data-testid="font-flag-pill"
                >
                  {name}
                </span>
              ))}
            </span>
          ) : (
            <span className="font-mono">{descriptor.flags}</span>
          )}
        </dd>
        <dt className="text-text-muted">Metrics:</dt>
        <dd>
          ItalicAngle {descriptor.italicAngle}, Ascent {descriptor.ascent}, Descent {descriptor.descent}, CapHeight {descriptor.capHeight}, StemV {descriptor.stemV}
        </dd>
        <dt className="text-text-muted">FontBBox:</dt>
        <dd>[{descriptor.fontBBox.join(', ')}]</dd>
        <dt className="text-text-muted">FontFile:</dt>
        <dd>
          {descriptor.fontFileFormat
            ? `${descriptor.fontFileFormat} (${formatBytes(descriptor.fontFileSize)})`
            : 'absent (font is not embedded)'}
        </dd>
        {descriptor.objectRef && (
          <>
            <dt className="text-text-muted">Object:</dt>
            <dd>
              <RefToken
                refText={descriptor.objectRef}
                target={descriptor.nodeId || nodeIdFromRef(descriptor.objectRef)}
                onClick={onReferenceClick}
              />
            </dd>
          </>
        )}
      </dl>
    </section>
  );
}

/** Metadata header section: Subtype, BaseFont, FirstChar, LastChar, embedded badge. */
function MetadataHeader({
  detail,
}: {
  detail: FontDetailData;
}) {
  // Embedded badge data must come from the descriptor that actually carries
  // the FontFile -- on Type0 fonts that's the descendant's FontDescriptor,
  // on everything else it's the parent's. AC2 contract.
  let badgeFormat = '';
  let badgeSize = 0;
  if (detail.subtype === 'Type0' && detail.descendant?.fontDescriptor) {
    badgeFormat = detail.descendant.fontDescriptor.fontFileFormat;
    badgeSize = detail.descendant.fontDescriptor.fontFileSize;
  } else if (detail.fontDescriptor) {
    badgeFormat = detail.fontDescriptor.fontFileFormat;
    badgeSize = detail.fontDescriptor.fontFileSize;
  }

  // Hide the Char range row when both fields are the zero default. Type0
  // (composite) fonts never carry FirstChar/LastChar per PDF spec, and the
  // zero-only case for simple fonts indicates absence rather than a real
  // range starting at code 0 -- showing "0 - 0" is misleading either way.
  const showCharRange = detail.firstChar !== 0 || detail.lastChar !== 0;

  return (
    <section className="p-3 text-xs" data-testid="font-metadata-section">
      <div className="flex items-center gap-2 mb-2 flex-wrap">
        <span className="font-mono text-base text-text">{detail.baseFont || '(no BaseFont)'}</span>
        <EmbeddedBadge embedded={detail.embedded} format={badgeFormat} sizeBytes={badgeSize} />
      </div>
      <dl className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1 font-mono">
        <dt className="text-text-muted">Subtype:</dt>
        <dd>{detail.subtype || '(unknown)'}</dd>
        {showCharRange && (
          <>
            <dt className="text-text-muted">Char range:</dt>
            <dd>{detail.firstChar} - {detail.lastChar}</dd>
          </>
        )}
      </dl>
    </section>
  );
}

/** Descendant Font section (composite Type0 fonts only). Includes a clickable
 *  ref to the descendant CIDFont dict and a recursive nested FontDescriptor view. */
function DescendantSection({
  descendant,
  onReferenceClick,
}: {
  descendant: FontDetailData;
  onReferenceClick: (target: string) => void;
}) {
  const cid = descendant.cidSystemInfo;
  return (
    <section className="border-t border-border p-3 text-xs" data-testid="font-descendant-section">
      <div className="text-text-secondary font-medium mb-1">Descendant Font</div>
      <dl className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1 font-mono">
        <dt className="text-text-muted">Subtype:</dt>
        <dd>{descendant.subtype}</dd>
        <dt className="text-text-muted">BaseFont:</dt>
        <dd>
          {descendant.baseFont}
          {descendant.objectRef && (
            <>
              {' '}
              <RefToken
                refText={descendant.objectRef}
                target={descendant.nodeId || nodeIdFromRef(descendant.objectRef)}
                onClick={onReferenceClick}
              />
            </>
          )}
        </dd>
        {cid && (
          <>
            <dt className="text-text-muted">CIDSystemInfo:</dt>
            <dd data-testid="font-cidsysteminfo">
              Registry: {cid.registry || '(none)'}, Ordering: {cid.ordering || '(none)'}, Supplement: {cid.supplement}
            </dd>
          </>
        )}
        {descendant.cidToGIDMap && (
          <>
            <dt className="text-text-muted">CIDToGIDMap:</dt>
            <dd data-testid="font-cidtogidmap">{descendant.cidToGIDMap}</dd>
          </>
        )}
        {descendant.defaultWidth > 0 && (
          <>
            <dt className="text-text-muted">DW:</dt>
            <dd data-testid="font-defaultwidth">{descendant.defaultWidth}</dd>
          </>
        )}
      </dl>
      {descendant.fontDescriptor && (
        <div className="mt-2">
          <FontDescriptorCard
            descriptor={descendant.fontDescriptor}
            onReferenceClick={onReferenceClick}
          />
        </div>
      )}
    </section>
  );
}

/** Props for the FontPreview component. */
export interface FontPreviewProps {
  detail: FontDetailData;
  onReferenceClick: (refTarget: string) => void;
}

/** Renders the consolidated font inspection view for a /Type /Font dict node. */
export function FontPreview({ detail, onReferenceClick }: FontPreviewProps) {
  return (
    <div className="flex-1 min-h-0 overflow-auto" data-testid="font-preview">
      <MetadataHeader detail={detail} />
      <EncodingSection detail={detail} />
      <ToUnicodeSection detail={detail} />
      {detail.fontDescriptor && (
        <FontDescriptorCard
          descriptor={detail.fontDescriptor}
          onReferenceClick={onReferenceClick}
        />
      )}
      {detail.descendant && (
        <DescendantSection
          descendant={detail.descendant}
          onReferenceClick={onReferenceClick}
        />
      )}
    </div>
  );
}
