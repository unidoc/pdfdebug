/**
 * @file Presentational component that renders decoded PDF content stream
 * text with line numbers in a scrollable, monospace view. When tokenized
 * data is available, renders syntax-highlighted tokens instead of plain text.
 */
import * as Tooltip from '@radix-ui/react-tooltip';

/** Shape of a single content stream token from the backend. */
interface TokenData {
  type: string;
  value: string;
  line: number;
  col: number;
}

interface ContentStreamViewerProps {
  raw: string;
  tokenized?: TokenData[] | null;
  error?: string;
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
          className="bg-surface border border-border rounded px-2 py-1 text-xs text-text shadow-md"
          data-testid="operator-tooltip"
          sideOffset={5}
        >
          {desc}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  );
}

/** Renders decoded content stream as plain text with a line-number gutter. */
export function ContentStreamViewer({ raw, tokenized, error }: ContentStreamViewerProps) {
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

  const useTokens = tokenized && tokenized.length > 0;
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

  return (
    <Tooltip.Provider delayDuration={300}>
      <div
        className="flex-1 min-h-0 overflow-auto"
        data-testid="content-stream-viewer"
      >
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
                  if (!lineToks || lineToks.length === 0) {
                    return <div key={i}>{''}</div>;
                  }
                  const spans: React.ReactNode[] = [];
                  let cursor = 1; // current column position
                  for (let ti = 0; ti < lineToks.length; ti++) {
                    const tok = lineToks[ti];
                    // Insert spacing
                    const gap = tok.col - cursor;
                    if (gap > 0) {
                      spans.push(' '.repeat(gap));
                    }
                    if (tok.type === 'operator') {
                      spans.push(<OperatorToken key={`${lineNum}-${ti}`} token={tok} />);
                    } else {
                      spans.push(
                        <span key={`${lineNum}-${ti}`} className={tokenClass(tok.type)}>
                          {tok.value}
                        </span>
                      );
                    }
                    cursor = tok.col + tok.value.length;
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
    </Tooltip.Provider>
  );
}
