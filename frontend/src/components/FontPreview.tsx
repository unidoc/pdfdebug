/**
 * @file FontPreview -- presentational component for the Story 9-9 font
 * inspection view. Renders the consolidated FontDetail payload (metadata
 * header, encoding section, ToUnicode table, FontDescriptor card, optional
 * descendant section) returned by GetFontDetail.
 *
 * Pure presentational: receives all data plus an onReferenceClick handler.
 * No Wails calls, no useAppDispatch. Mirrors the ImagePreview pattern.
 *
 * The Story 13.3 joined mapping table is viewport-virtualized with the same
 * hand-rolled windowing approach as PlainTextView (a tall spacer fixes total
 * scroll height; only the visible slice of rows renders), so a CID font with
 * thousands of codes keeps the panel interactive (NFR5) with no new dependency.
 */
import { useCallback, useEffect, useRef, useState } from 'react';

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

/** One assembled row in the joined per-code mapping table (Story 13.3):
 *  the JOIN of /Differences (glyphName) and /ToUnicode (unicode, unicodeText)
 *  keyed by character code. Mirrors backend FontMappingRow. */
export interface FontMappingRowData {
  code: number;
  codeHex: string;
  glyphName: string;
  unicode: string;
  unicodeText: string;
}

/** Coverage/health diagnostic signals for a font (Story 13.3). Mirrors
 *  backend FontHealth. */
export interface FontHealthData {
  declaredCodeCount: number;
  toUnicodeMissing: boolean;
  identityWithoutToUnicode: boolean;
  encodingWithoutToUnicodeCodes: number[];
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
   *  Type0 / non-composite fonts. */
  cidSystemInfo: CIDSystemInfoData | null;
  /** "Identity" for the Name form or "Stream (<N> bytes)" for the stream
   *  form. Empty when /CIDToGIDMap is absent. */
  cidToGIDMap: string;
  /** /DW default width for CIDFonts. 0 when absent. */
  defaultWidth: number;
  /** Assembled per-code mapping table: the JOIN of Differences + ToUnicode
   *  over the union of declared codes (Story 13.3). */
  mappingRows: FontMappingRowData[];
  /** Coverage/health diagnostic signals (Story 13.3). Null when the
   *  backend did not populate it (older payloads). */
  health: FontHealthData | null;
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

// PDF spec 9.6.4: six uppercase letters + '+' on /BaseFont signal a subsetted
// font program. The leading '/' on the displayed BaseFont is stripped here.
const SUBSET_PREFIX_RE = /^\/?([A-Z]{6})\+/;

/** Returns the subset tag (without '+') when baseFont carries the PDF subset
 *  prefix, e.g. "/AAAAAB+Helvetica" -> "AAAAAB". Returns null otherwise. */
function extractSubsetPrefix(baseFont: string): string | null {
  const m = SUBSET_PREFIX_RE.exec(baseFont);
  return m ? m[1] : null;
}

// Standard named encodings per PDF 1.7 Appendix D. When /Encoding is one of
// these names (not a dict with /Differences), the code-to-glyph mapping is
// implicit and well-known, so there is no per-code table to render.
const STANDARD_ENCODING_NAMES = new Set([
  '/WinAnsiEncoding',
  '/MacRomanEncoding',
  '/MacExpertEncoding',
  '/StandardEncoding',
  '/PDFDocEncoding',
  '/Identity-H',
  '/Identity-V',
]);

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
 *  red chip with viewer-fallback warning otherwise. */
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
    <section className="shrink-0 border-t border-border p-3 text-xs" data-testid="font-encoding-section">
      <div className="text-text-secondary font-medium mb-1">Encoding</div>
      {hasNamed && !hasDifferences && (
        <>
          <div className="font-mono text-text">{detail.encodingName}</div>
          {STANDARD_ENCODING_NAMES.has(detail.encodingName) && (
            <div className="text-text-muted mt-1" data-testid="font-encoding-standard-note">
              Standard named encoding. Code-to-glyph mapping is implicit (PDF spec Appendix D) -- no per-code overrides.
            </div>
          )}
        </>
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
              <thead className="bg-surface-hover">
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

/** ToUnicode section: table of code -> U+XXXX -> glyph, with a warning
 *  panel rendered above the table when toUnicodeError is set. Flex-grows to
 *  fill remaining FontPreview height; the inner scroll container is the
 *  table viewport. */
function ToUnicodeSection({ detail }: { detail: FontDetailData }) {
  const hasMappings = detail.toUnicodeMappings.length > 0;
  const hasError = detail.toUnicodeError !== '';
  const isEmpty = !hasMappings && !hasError;

  return (
    <section
      className="flex-1 min-h-[8rem] flex flex-col border-t border-border p-3 text-xs"
      data-testid="font-tounicode-section"
    >
      <div className="text-text-secondary font-medium mb-1 shrink-0">ToUnicode CMap</div>
      {isEmpty && (
        <div className="text-text-muted" data-testid="font-tounicode-empty">
          No /ToUnicode CMap in this font dict. Text extraction relies on the encoding above. Independent of font subsetting.
        </div>
      )}
      {hasError && (
        <div className="px-2 py-1 text-warning mb-1 shrink-0" data-testid="font-tounicode-error">
          ToUnicode present but unparseable: {detail.toUnicodeError}
        </div>
      )}
      {hasMappings && (
        <div className="flex-1 min-h-0 overflow-auto border border-border rounded">
          <table className="w-full text-xs">
            <thead className="sticky top-0 z-10">
              <tr>
                <th className="text-left px-2 py-1 text-text-secondary font-medium bg-surface-hover">Code (hex)</th>
                <th className="text-left px-2 py-1 text-text-secondary font-medium bg-surface-hover">Unicode</th>
                <th className="text-left px-2 py-1 text-text-secondary font-medium bg-surface-hover">Glyph</th>
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
    <section className="shrink-0 border-t border-border p-3 text-xs" data-testid="font-descriptor-section">
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
                  className="px-1.5 py-0.5 rounded bg-surface-hover text-text-secondary"
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
  // on everything else it's the parent's.
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

  const subsetPrefix = extractSubsetPrefix(detail.baseFont);

  return (
    <section className="shrink-0 p-3 text-xs" data-testid="font-metadata-section">
      <div className="flex items-center gap-2 mb-2 flex-wrap">
        <span className="font-mono text-base text-text">{detail.baseFont || '(no BaseFont)'}</span>
        <EmbeddedBadge embedded={detail.embedded} format={badgeFormat} sizeBytes={badgeSize} />
        {subsetPrefix && (
          <span
            className="inline-flex items-center px-2 py-0.5 text-xs rounded bg-info/10 text-info font-medium"
            data-testid="font-subset-badge"
            title={`Subset prefix ${subsetPrefix}+ -- only glyphs used by the document are included in the embedded font program (PDF spec 9.6.4).`}
          >
            Subset
          </span>
        )}
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
    <section className="shrink-0 border-t border-border p-3 text-xs" data-testid="font-descendant-section">
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

/** Health-signals banner (Story 13.3): surfaces the classic
 *  text-extraction failure modes explicitly. Renders nothing when health is
 *  absent or all signals are clear. */
function FontHealthBanner({ health }: { health: FontHealthData }) {
  const missing = health.toUnicodeMissing;
  const identity = health.identityWithoutToUnicode;
  const withoutCount = health.encodingWithoutToUnicodeCodes.length;
  // Nothing to warn about: full coverage.
  if (!missing && !identity && withoutCount === 0) {
    return null;
  }
  return (
    <section
      className="shrink-0 border-t border-border p-3 text-xs"
      data-testid="font-health-banner"
    >
      <div className="text-text-secondary font-medium mb-1">Mapping health</div>
      <ul className="flex flex-col gap-1">
        {identity && (
          <li className="px-2 py-1 rounded bg-error/10 text-error">
            Identity encoding with no ToUnicode CMap -- copied/extracted text yields gibberish.
          </li>
        )}
        {missing && !identity && (
          <li className="px-2 py-1 rounded bg-warning/10 text-warning">
            No usable ToUnicode CMap -- text extraction has no code-to-Unicode coverage.
          </li>
        )}
        {withoutCount > 0 && (
          <li className="px-2 py-1 rounded bg-warning/10 text-warning">
            {withoutCount} encoding code{withoutCount === 1 ? '' : 's'} map to no Unicode -- extraction will fail for {withoutCount === 1 ? 'it' : 'them'}.
          </li>
        )}
      </ul>
    </section>
  );
}

/** Approximate per-row pixel height for the joined mapping table (matches the
 *  py-1 text-xs row height). Used by the windowing math. */
const MAPPING_ROW_HEIGHT = 26;
/** Rows rendered above/below the viewport for smooth scrolling. */
const MAPPING_OVERSCAN = 12;

/** Joined per-code mapping table (Story 13.3): one row per declared code
 *  carrying code (hex), glyph name, Unicode, and literal text -- the JOIN of
 *  the Differences and ToUnicode sources. Viewport-virtualized (NFR5): only the
 *  visible window of rows is committed to the DOM, so thousands of CID codes do
 *  not freeze the panel. Mirrors the PlainTextView windowing approach. */
function FontMappingTable({ rows }: { rows: FontMappingRowData[] }) {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(0);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    setViewportHeight(el.clientHeight);
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(() => setViewportHeight(el.clientHeight));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const handleScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  }, []);

  // Reset the window when the row set changes (a new font is selected).
  // Without this, a stale large scrollTop from a previous long CID font would
  // slice past the end of a short row set (rows.slice(firstVisible, ...) with
  // firstVisible >> totalRows yields []), rendering an empty table until the
  // user manually scrolls up. Mirrors PlainTextView's scroll-to-top-on-change.
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = 0;
    }
    setScrollTop(0);
  }, [rows]);

  // Empty state: a built-in-encoding font (no /Differences, no /ToUnicode) has
  // zero declared codes. Render a compact note instead of a flex-grown empty
  // table so the common no-mapping font (e.g. Helvetica) does not get a large
  // dead panel pushing the other sections down.
  if (rows.length === 0) {
    return (
      <section className="shrink-0 border-t border-border p-3 text-xs" data-testid="font-mapping-section">
        <div className="text-text-secondary font-medium mb-1">Code mapping</div>
        <div className="text-text-muted" data-testid="font-mapping-empty">
          No declared code mappings (no /Differences overrides and no /ToUnicode CMap). Code-to-glyph mapping is implicit in the encoding above.
        </div>
      </section>
    );
  }

  const totalRows = rows.length;
  const firstVisible = Math.max(0, Math.floor(scrollTop / MAPPING_ROW_HEIGHT) - MAPPING_OVERSCAN);
  // A zero clientHeight (jsdom / pre-measure) falls back to a bounded default so
  // the window stays small rather than rendering every row.
  const visibleCount =
    Math.ceil((viewportHeight || 320) / MAPPING_ROW_HEIGHT) + MAPPING_OVERSCAN * 2;
  const lastVisible = Math.min(totalRows, firstVisible + visibleCount);
  const rowsToRender = rows.slice(firstVisible, lastVisible);

  return (
    <section
      className="flex-1 min-h-[8rem] flex flex-col border-t border-border p-3 text-xs"
      data-testid="font-mapping-section"
    >
      <div className="text-text-secondary font-medium mb-1 shrink-0">
        Code mapping ({totalRows} declared code{totalRows === 1 ? '' : 's'})
      </div>
      <div
        className="flex-1 min-h-0 overflow-auto border border-border rounded"
        ref={scrollRef}
        onScroll={handleScroll}
      >
        <table className="w-full text-xs" data-testid="font-mapping-table">
          <thead className="sticky top-0 z-10">
            <tr>
              <th className="text-left px-2 py-1 text-text-secondary font-medium bg-surface-hover">Code</th>
              <th className="text-left px-2 py-1 text-text-secondary font-medium bg-surface-hover">Glyph</th>
              <th className="text-left px-2 py-1 text-text-secondary font-medium bg-surface-hover">Unicode</th>
              <th className="text-left px-2 py-1 text-text-secondary font-medium bg-surface-hover">Text</th>
            </tr>
          </thead>
          <tbody>
            {/* Top spacer: reserves the scroll height of the rows above the
                window so the scrollbar geometry matches the full data set. */}
            {firstVisible > 0 && (
              <tr aria-hidden="true" style={{ height: firstVisible * MAPPING_ROW_HEIGHT }}>
                <td colSpan={4} />
              </tr>
            )}
            {rowsToRender.map((r) => (
              <tr key={r.code} className="border-t border-border" style={{ height: MAPPING_ROW_HEIGHT }}>
                <td className="px-2 py-1 font-mono text-text-muted">{r.codeHex}</td>
                <td className="px-2 py-1 font-mono text-type-name">{r.glyphName || '-'}</td>
                <td className="px-2 py-1 font-mono">{r.unicode || '-'}</td>
                <td className="px-2 py-1 font-mono">{r.unicodeText || '-'}</td>
              </tr>
            ))}
            {/* Bottom spacer: reserves the scroll height of the rows below. */}
            {lastVisible < totalRows && (
              <tr aria-hidden="true" style={{ height: (totalRows - lastVisible) * MAPPING_ROW_HEIGHT }}>
                <td colSpan={4} />
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

/** Props for the FontPreview component. */
export interface FontPreviewProps {
  detail: FontDetailData;
  onReferenceClick: (refTarget: string) => void;
}

/** Renders the consolidated font inspection view for a /Type /Font dict node.
 *  Layout: fixed-height container with overflow-hidden; ToUnicodeSection
 *  flex-grows to fill remaining space so the CMap table consumes the dead
 *  panel space on tall windows. When ToUnicode is absent the panel content
 *  hugs the top -- no flex-1 substitute needed because there's no working
 *  surface that demands the height. */
export function FontPreview({ detail, onReferenceClick }: FontPreviewProps) {
  return (
    <div className="flex-1 flex flex-col h-full min-h-0 overflow-hidden" data-testid="font-preview">
      <MetadataHeader detail={detail} />
      {detail.health && <FontHealthBanner health={detail.health} />}
      <FontMappingTable rows={detail.mappingRows ?? []} />
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
