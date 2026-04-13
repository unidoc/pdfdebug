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

export type StreamViewMode = 'formatted' | 'raw';

interface ContentStreamViewerProps {
  raw: string;
  tokenized?: TokenData[] | null;
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
export function ContentStreamViewer({ raw, tokenized, error, viewMode: controlledMode, onViewModeChange }: ContentStreamViewerProps) {
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

  const hasTokens = !!(tokenized && tokenized.length > 0);
  // If no tokens available, force raw mode regardless of controlled value
  const effectiveMode = hasTokens ? (controlledMode ?? 'formatted') : 'raw';
  const useTokens = hasTokens && effectiveMode === 'formatted';
  const rawLines = raw ? raw.split('\n') : [];

  // Group tokens by line number
  const tokensByLine: Map<number, TokenData[]> = new Map();
  if (useTokens) {
    for (const tok of tokenized) {
      const arr = tokensByLine.get(tok.line) || [];
      arr.push(tok);
      tokensByLine.set(tok.line, arr);
    }
  }

  // Avoid Math.max(...spread) -- large token arrays blow the call stack.
  let maxTokenLine = 0;
  if (useTokens) {
    for (const t of tokenized) {
      if (t.line > maxTokenLine) maxTokenLine = t.line;
    }
  }
  const lineCount = Math.max(maxTokenLine, rawLines.length);

  // Compute structural indentation per line based on block-opening/closing
  // operators (BT/ET, q/Q, BDC/BMC/EMC). This produces consistent
  // indentation regardless of how the PDF generator wrote the stream.
  const INDENT_OPEN = new Set(['BT', 'q', 'BDC', 'BMC']);
  const INDENT_CLOSE = new Set(['ET', 'Q', 'EMC']);
  const lineIndent: Map<number, number> = new Map();
  if (useTokens) {
    let depth = 0;
    for (let ln = 1; ln <= lineCount; ln++) {
      const toks = tokensByLine.get(ln);
      if (!toks || toks.length === 0) {
        lineIndent.set(ln, depth);
        continue;
      }
      // Check if this line has a closing operator -- dedent before rendering
      const hasClose = toks.some((t) => t.type === 'operator' && INDENT_CLOSE.has(t.value));
      if (hasClose) depth = Math.max(0, depth - 1);
      lineIndent.set(ln, depth);
      // Check if this line has an opening operator -- indent after rendering
      const hasOpen = toks.some((t) => t.type === 'operator' && INDENT_OPEN.has(t.value));
      if (hasOpen) depth += 1;
    }
  }

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
            hasTokens={hasTokens}
          />
        )}
        <div className="flex-1 min-h-0 overflow-auto">
        <div className="flex">
          <div
            className="flex-shrink-0 text-right pr-3 select-none text-text-muted text-xs font-mono border-r border-border"
            style={{ minWidth: `${Math.max(String(lineCount).length, 2)}ch` }}
            data-testid="content-stream-gutter"
          >
            {Array.from({ length: lineCount }, (_, i) => (
              <div key={i} className="px-1">{i + 1}</div>
            ))}
          </div>
          <div
            className="pl-3 font-mono text-xs text-text whitespace-pre"
            data-testid="content-stream-content"
          >
            {useTokens
              ? Array.from({ length: lineCount }, (_, i) => {
                  const lineNum = i + 1;
                  const lineToks = tokensByLine.get(lineNum);
                  const indent = lineIndent.get(lineNum) ?? 0;
                  const indentStr = indent > 0 ? '  '.repeat(indent) : '';
                  if (!lineToks || lineToks.length === 0) {
                    return <div key={i}>{''}</div>;
                  }
                  const spans: React.ReactNode[] = [];
                  if (indentStr) spans.push(indentStr);
                  for (let ti = 0; ti < lineToks.length; ti++) {
                    const tok = lineToks[ti];
                    // Space between tokens on the same line
                    if (ti > 0) spans.push(' ');
                    if (tok.type === 'operator') {
                      spans.push(<OperatorToken key={`${lineNum}-${ti}`} token={tok} />);
                    } else {
                      spans.push(
                        <span key={`${lineNum}-${ti}`} className={tokenClass(tok.type)}>
                          {tok.value}
                        </span>
                      );
                    }
                  }
                  return <div key={i}>{spans}</div>;
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
