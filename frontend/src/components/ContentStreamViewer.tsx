/**
 * @file Presentational component that renders decoded PDF content stream
 * text with line numbers in a scrollable, monospace view. When tokenized
 * data is available, renders syntax-highlighted tokens instead of plain text.
 * Supports toggling between "formatted" (syntax-highlighted) and "raw" (plain text) views.
 */
import * as Tooltip from '@radix-ui/react-tooltip';

/** Shape of a single content stream token from the backend. */
interface TokenData {
  type: string;
  value: string;
  line: number;
  col: number;
}

/**
 * Shape of one formatted row from the backend (Go-side `FormattedLine`).
 * Story 9-6: the Go formatter groups operands with their operator and emits
 * one row per logical PDF operation, so the frontend just iterates and renders.
 */
interface FormattedLineData {
  tokens: TokenData[];
  indent: number;
  operator: string;
  srcLineStart: number;
  srcLineEnd: number;
}

export type StreamViewMode = 'formatted' | 'raw';

interface ContentStreamViewerProps {
  raw: string;
  formatted?: FormattedLineData[] | null;
  error?: string;
  viewMode?: StreamViewMode;
  onViewModeChange?: (mode: StreamViewMode) => void;
}

/** PDF operator descriptions for tooltip display. */
const OPERATOR_DESCRIPTIONS: Record<string, string> = {
  BT: 'Begin text object',
  ET: 'End text object',
  Tf: 'Set text font and size',
  Td: 'Move text position',
  TD: 'Move text position and set leading',
  Tm: 'Set text matrix',
  Tj: 'Show text string',
  TJ: 'Show text with individual glyph positioning',
  'T*': 'Move to start of next line',
  Tc: 'Set character spacing',
  Tw: 'Set word spacing',
  Tz: 'Set horizontal text scaling',
  TL: 'Set text leading',
  Tr: 'Set text rendering mode',
  Ts: 'Set text rise',
  q: 'Save graphics state',
  Q: 'Restore graphics state',
  cm: 'Concatenate matrix to CTM',
  m: 'Begin new subpath (moveto)',
  l: 'Append line segment',
  c: 'Append cubic Bezier curve',
  v: 'Append cubic Bezier curve (initial point replicated)',
  y: 'Append cubic Bezier curve (final point replicated)',
  h: 'Close subpath',
  re: 'Append rectangle',
  S: 'Stroke path',
  s: 'Close and stroke path',
  f: 'Fill path (nonzero winding)',
  F: 'Fill path (nonzero winding, PDF 1.0 compat)',
  'f*': 'Fill path (even-odd rule)',
  B: 'Fill then stroke path',
  'B*': 'Fill then stroke path (even-odd)',
  b: 'Close, fill, then stroke path',
  'b*': 'Close, fill, then stroke (even-odd)',
  n: 'End path without fill or stroke',
  W: 'Set clipping path (nonzero winding)',
  'W*': 'Set clipping path (even-odd)',
  Do: 'Invoke named XObject',
  gs: 'Set graphics state from ExtGState',
  CS: 'Set color space (stroking)',
  cs: 'Set color space (non-stroking)',
  SC: 'Set color (stroking)',
  SCN: 'Set color (stroking, extended)',
  sc: 'Set color (non-stroking)',
  scn: 'Set color (non-stroking, extended)',
  G: 'Set gray level (stroking)',
  g: 'Set gray level (non-stroking)',
  RG: 'Set RGB color (stroking)',
  rg: 'Set RGB color (non-stroking)',
  K: 'Set CMYK color (stroking)',
  k: 'Set CMYK color (non-stroking)',
  w: 'Set line width',
  J: 'Set line cap style',
  j: 'Set line join style',
  M: 'Set miter limit',
  d: 'Set line dash pattern',
  ri: 'Set color rendering intent',
  i: 'Set flatness tolerance',
  BMC: 'Begin marked-content sequence',
  BDC: 'Begin marked-content sequence with property list',
  EMC: 'End marked-content sequence',
  MP: 'Define marked-content point',
  DP: 'Define marked-content point with property list',
  BI: 'Begin inline image',
  ID: 'Begin inline image data',
  EI: 'End inline image',
  d0: 'Set glyph width in Type 3 font',
  d1: 'Set glyph width and bounding box in Type 3 font',
  sh: 'Paint with shading pattern',
};

/** CSS class for a token type. */
function tokenClass(type: string): string {
  switch (type) {
    case 'operator':
      return 'text-token-operator font-semibold';
    case 'number':
      return 'text-token-number';
    case 'string':
      return 'text-token-string';
    case 'name':
      return 'text-token-name';
    case 'comment':
      return 'text-token-comment italic';
    default:
      return 'text-text';
  }
}

/** Renders a single operator token, optionally wrapped in a tooltip. */
function OperatorToken({ token }: { token: TokenData }) {
  const desc = OPERATOR_DESCRIPTIONS[token.value];
  const span = <span className={`${tokenClass('operator')} cursor-default`}>{token.value}</span>;

  if (!desc) return span;

  return (
    <Tooltip.Root>
      <Tooltip.Trigger asChild>{span}</Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content
          className="bg-surface border border-border rounded px-2 py-1 text-xs text-text shadow-md z-50"
          data-testid="operator-tooltip"
          sideOffset={5}
        >
          {desc}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  );
}

/** View mode segmented control above the stream content. */
function ViewModeControl({
  viewMode,
  onViewModeChange,
  hasTokens,
}: {
  viewMode: StreamViewMode;
  onViewModeChange: (mode: StreamViewMode) => void;
  hasTokens: boolean;
}) {
  return (
    <div
      className="flex-shrink-0 flex gap-0 border-b border-border px-3 py-1.5"
      role="tablist"
      aria-label="Content stream view mode"
      data-testid="view-mode-control"
    >
      <button
        role="tab"
        aria-selected={viewMode === 'formatted'}
        aria-disabled={!hasTokens}
        disabled={!hasTokens}
        className={`relative px-3 py-1 text-xs border rounded-l transition-colors ${
          viewMode === 'formatted'
            ? 'bg-interactive text-on-interactive border-interactive font-medium z-10'
            : hasTokens
              ? 'bg-surface text-text-secondary border-border hover:bg-hover'
              : 'bg-surface text-text-muted border-border cursor-not-allowed opacity-50'
        }`}
        onClick={() => onViewModeChange('formatted')}
        data-testid="view-mode-formatted"
      >
        Formatted
      </button>
      <button
        role="tab"
        aria-selected={viewMode === 'raw'}
        className={`relative px-3 py-1 text-xs border rounded-r transition-colors -ml-px ${
          viewMode === 'raw'
            ? 'bg-interactive text-on-interactive border-interactive font-medium z-10'
            : 'bg-surface text-text-secondary border-border hover:bg-hover'
        }`}
        onClick={() => onViewModeChange('raw')}
        data-testid="view-mode-raw"
      >
        Raw
      </button>
    </div>
  );
}

/** Renders decoded content stream with a view mode toggle and line-number gutter. */
export function ContentStreamViewer({ raw, formatted, error, viewMode: controlledMode, onViewModeChange }: ContentStreamViewerProps) {
  if (error) {
    return (
      <div
        className="text-error text-sm p-3"
        data-testid="content-stream-error"
      >
        {error}
      </div>
    );
  }

  const hasFormatted = !!(formatted && formatted.length > 0);
  // If no formatted rows are available, force raw mode regardless of controlled value
  const effectiveMode: StreamViewMode = hasFormatted ? (controlledMode ?? 'formatted') : 'raw';
  const useFormatted = hasFormatted && effectiveMode === 'formatted';
  const rawLines = raw ? raw.split('\n') : [];

  // Gutter line count: number of formatted rows in formatted mode, source
  // byte-line count in raw mode. Padded so the gutter width is stable.
  const gutterCount = useFormatted ? (formatted!.length) : rawLines.length;
  const gutterDigits = Math.max(String(gutterCount).length, 2);

  return (
    <Tooltip.Provider delayDuration={300}>
      <div
        className="flex-1 min-h-0 flex flex-col"
        data-testid="content-stream-viewer"
      >
        {onViewModeChange && (
          <ViewModeControl
            viewMode={effectiveMode}
            onViewModeChange={onViewModeChange}
            hasTokens={hasFormatted}
          />
        )}
        <div className="flex-1 min-h-0 overflow-auto">
        <div className="flex">
          <div
            className="flex-shrink-0 text-right pr-3 select-none text-text-muted text-xs font-mono border-r border-border"
            style={{ minWidth: `${gutterDigits}ch` }}
            data-testid="content-stream-gutter"
          >
            {Array.from({ length: gutterCount }, (_, i) => (
              <div key={i} className="px-1">{i + 1}</div>
            ))}
          </div>
          <div
            className="pl-3 font-mono text-xs text-text whitespace-pre"
            data-testid="content-stream-content"
          >
            {useFormatted
              ? formatted!.map((row, i) => {
                  const indentStr = row.indent > 0 ? '  '.repeat(row.indent) : '';
                  const spans: React.ReactNode[] = [];
                  if (indentStr) spans.push(indentStr);
                  for (let ti = 0; ti < row.tokens.length; ti++) {
                    const tok = row.tokens[ti];
                    if (ti > 0) spans.push(' ');
                    if (tok.type === 'operator') {
                      spans.push(<OperatorToken key={`${i}-${ti}`} token={tok} />);
                    } else {
                      spans.push(
                        <span key={`${i}-${ti}`} className={tokenClass(tok.type)}>
                          {tok.value}
                        </span>
                      );
                    }
                  }
                  // title shows the source-byte-line range so users can correlate
                  // a formatted row with its origin in the Raw view (load-bearing
                  // for cross-view scroll-sync per AC #6).
                  const rangeTitle = row.srcLineStart === row.srcLineEnd
                    ? `Source line ${row.srcLineStart}`
                    : `Source lines ${row.srcLineStart}-${row.srcLineEnd}`;
                  return <div key={i} title={rangeTitle}>{spans}</div>;
                })
              : rawLines.map((line, i) => (
                  <div key={i}>{line}</div>
                ))
            }
          </div>
        </div>
        </div>
      </div>
    </Tooltip.Provider>
  );
}
