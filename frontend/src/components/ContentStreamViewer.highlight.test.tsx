/**
 * Content Stream Tokenizer with Syntax Highlighting (V1)
 *
 * Run: cd frontend && npx vitest run src/components/ContentStreamViewer.highlight.test.tsx
 */
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect } from 'vitest';
import { ContentStreamViewer } from './ContentStreamViewer';

// See ContentStreamViewer.test.tsx for the rationale; this helper wraps
// a flat token fixture into a single FormattedLine so per-token highlight
// assertions survive the prop rename without re-deriving operator boundaries.
type TokFixture = ReadonlyArray<{ type: string; value: string; line: number; col: number }>;
function toFormatted(toks: TokFixture | null | undefined) {
  if (!toks || toks.length === 0) return [];
  const lines = toks.map(t => t.line);
  const last = toks[toks.length - 1];
  return [{
    tokens: [...toks],
    indent: 0,
    operator: last.type === 'operator' ? last.value : '',
    srcLineStart: Math.min(...lines),
    srcLineEnd: Math.max(...lines),
  }];
}

// Token fixture matching the Token binding shape from model.go
const sampleTokens = [
  { type: 'operator', value: 'BT', line: 1, col: 1 },
  { type: 'name', value: '/F1', line: 2, col: 1 },
  { type: 'number', value: '12', line: 2, col: 5 },
  { type: 'operator', value: 'Tf', line: 2, col: 8 },
  { type: 'number', value: '100', line: 3, col: 1 },
  { type: 'number', value: '700', line: 3, col: 5 },
  { type: 'operator', value: 'Td', line: 3, col: 9 },
  { type: 'string', value: '(Hello World)', line: 4, col: 1 },
  { type: 'operator', value: 'Tj', line: 4, col: 15 },
  { type: 'operator', value: 'ET', line: 5, col: 1 },
];

const multiLineRaw = 'BT\n/F1 12 Tf\n100 700 Td\n(Hello World) Tj\nET';

// ---------------------------------------------------------------------------
// Syntax highlighting applies distinct CSS classes for operator, number,
// string, name, and comment token types.
// Operators visually distinct from operands.
// ---------------------------------------------------------------------------

describe('Token type CSS classes', () => {
  test('operator tokens have text-token-operator class', () => {
    render(
      <ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(sampleTokens)} />
    );

    // BT is an operator on line 1
    const btSpan = screen.getByText('BT');
    expect(btSpan.className).toMatch(/text-token-operator/);
  });

  test('number tokens have text-token-number class', () => {
    render(
      <ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(sampleTokens)} />
    );

    const numSpan = screen.getByText('12');
    expect(numSpan.className).toMatch(/text-token-number/);
  });

  test('string tokens have text-token-string class', () => {
    render(
      <ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(sampleTokens)} />
    );

    const strSpan = screen.getByText('(Hello World)');
    expect(strSpan.className).toMatch(/text-token-string/);
  });

  test('name tokens have text-token-name class', () => {
    render(
      <ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(sampleTokens)} />
    );

    const nameSpan = screen.getByText('/F1');
    expect(nameSpan.className).toMatch(/text-token-name/);
  });

  test('comment tokens have text-token-comment class', () => {
    const commentTokens = [
      { type: 'comment', value: '% a comment', line: 1, col: 1 },
      { type: 'operator', value: 'BT', line: 2, col: 1 },
    ];
    render(
      <ContentStreamViewer
        raw="% a comment\nBT"
        formatted={toFormatted(commentTokens)}
      />
    );

    const commentSpan = screen.getByText('% a comment');
    expect(commentSpan.className).toMatch(/text-token-comment/);
  });
});

// ---------------------------------------------------------------------------
// Operator highlighting does not rely solely on color.
// Font weight or style also differentiates operators from operands.
// ---------------------------------------------------------------------------

describe('Non-color differentiation (accessibility)', () => {
  test('operator tokens have font-semibold class', () => {
    render(
      <ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(sampleTokens)} />
    );

    const btSpan = screen.getByText('BT');
    expect(btSpan.className).toMatch(/font-semibold/);
  });

  test('comment tokens have italic class', () => {
    const commentTokens = [
      { type: 'comment', value: '% a comment', line: 1, col: 1 },
    ];
    render(
      <ContentStreamViewer raw="% a comment" formatted={toFormatted(commentTokens)} />
    );

    const commentSpan = screen.getByText('% a comment');
    expect(commentSpan.className).toMatch(/italic/);
  });
});

// ---------------------------------------------------------------------------
// Operator tooltip shows description on hover using Radix UI Tooltip.
// Hovering over operator keyword shows brief description.
// ---------------------------------------------------------------------------

describe('Operator tooltip', () => {
  test('operator token with description renders a tooltip trigger', () => {
    render(
      <ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(sampleTokens)} />
    );

    // BT has a known description: "Begin text object"
    // The trigger element should wrap the BT text
    const btEl = screen.getByText('BT');
    // Radix Tooltip.Trigger with asChild merges data-state onto the child span
    expect(btEl.getAttribute('data-state')).toBeTruthy();
  });

  test('hovering over operator shows tooltip with description text', async () => {
    const user = userEvent.setup();
    render(
      <ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(sampleTokens)} />
    );

    const btEl = screen.getByText('BT');
    await user.hover(btEl);

    // Tooltip content should appear with the operator description
    const tooltip = await screen.findByTestId('operator-tooltip');
    expect(tooltip).toHaveTextContent('Begin text object');
  });

  test('operators without descriptions do not render tooltip', () => {
    // Use a made-up operator that is not in the description map
    const unknownOpTokens = [
      { type: 'operator', value: 'ZZ', line: 1, col: 1 },
    ];
    render(
      <ContentStreamViewer raw="ZZ" formatted={toFormatted(unknownOpTokens)} />
    );

    const zzEl = screen.getByText('ZZ');
    // Should NOT be wrapped in a tooltip trigger
    expect(zzEl.closest('[data-radix-tooltip-trigger]')).toBeNull();
    expect(zzEl.closest('button')).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Falls back to plain text when tokenized is null/undefined/empty.
// When tokenized is not provided, falls back to raw text rendering.
// ---------------------------------------------------------------------------

describe('Tokenized fallback to plain text', () => {
  test('renders plain text when tokenized is undefined', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    // Falls back to raw text -- content area should have the text
    const content = screen.getByTestId('content-stream-content');
    expect(content).toHaveTextContent('BT');
    expect(content).toHaveTextContent('ET');
    // No token-specific classes on the text
    const btText = screen.getByText('BT');
    expect(btText.className).not.toMatch(/text-token-operator/);
  });

  test('renders plain text when tokenized is null', () => {
    render(
      <ContentStreamViewer raw={multiLineRaw} formatted={null as any} />
    );

    const content = screen.getByTestId('content-stream-content');
    expect(content).toHaveTextContent('/F1 12 Tf');
  });

  test('renders plain text when tokenized is empty array', () => {
    render(<ContentStreamViewer raw={multiLineRaw} formatted={[]} />);

    const content = screen.getByTestId('content-stream-content');
    expect(content).toHaveTextContent('100 700 Td');
  });
});

// ---------------------------------------------------------------------------
// Line numbers still rendered correctly with tokenized data.
// ---------------------------------------------------------------------------

describe('Line numbers with tokenized data', () => {
  // Gutter is keyed by formatted-row index in formatted mode.
  // Build a 5-row fixture (one per logical operation) so the gutter shows 1..5.
  test('renders one gutter row per formatted line', () => {
    const fiveRows = [
      { tokens: [{ type: 'operator', value: 'BT', line: 1, col: 1 }], indent: 0, operator: 'BT', srcLineStart: 1, srcLineEnd: 1 },
      { tokens: [{ type: 'operator', value: 'Tf', line: 2, col: 8 }], indent: 0, operator: 'Tf', srcLineStart: 2, srcLineEnd: 2 },
      { tokens: [{ type: 'operator', value: 'Td', line: 3, col: 9 }], indent: 0, operator: 'Td', srcLineStart: 3, srcLineEnd: 3 },
      { tokens: [{ type: 'operator', value: 'Tj', line: 4, col: 15 }], indent: 0, operator: 'Tj', srcLineStart: 4, srcLineEnd: 4 },
      { tokens: [{ type: 'operator', value: 'ET', line: 5, col: 1 }], indent: 0, operator: 'ET', srcLineStart: 5, srcLineEnd: 5 },
    ];
    render(
      <ContentStreamViewer raw={multiLineRaw} formatted={fiveRows} />
    );

    const gutter = screen.getByTestId('content-stream-gutter');
    expect(gutter).toHaveTextContent('1');
    expect(gutter).toHaveTextContent('2');
    expect(gutter).toHaveTextContent('3');
    expect(gutter).toHaveTextContent('4');
    expect(gutter).toHaveTextContent('5');
  });

  test('outer wrapper retains data-testid and layout classes', () => {
    render(
      <ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(sampleTokens)} />
    );

    const viewer = screen.getByTestId('content-stream-viewer');
    expect(viewer).toBeInTheDocument();
    // overflow-auto is on the inner scroll container
    const scrollArea = viewer.querySelector('.overflow-auto');
    expect(scrollArea).not.toBeNull();
  });
});
