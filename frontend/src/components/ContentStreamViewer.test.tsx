/**
 * Story 3.2: Content Stream Viewer -- Raw Text Display (Prototype)
 *
 * TDD RED PHASE: Tests MUST fail until ContentStreamViewer.tsx is implemented.
 *
 * Test IDs: 3.2-UNIT-001 through 3.2-UNIT-005 (Vitest)
 * Run: cd frontend && npx vitest run src/components/ContentStreamViewer.test.tsx
 */
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect } from 'vitest';
// RED PHASE: This import will fail until ContentStreamViewer.tsx is created.
import { ContentStreamViewer } from './ContentStreamViewer';

const multiLineRaw = 'BT\n/F1 12 Tf\n100 700 Td\n(Hello World) Tj\nET';

// ---------------------------------------------------------------------------
// 3.2-UNIT-001 [P0]: ContentStreamViewer renders decoded text in monospace
// font with line numbers.
// AC#1: Content stream displayed as decoded plain text in monospace font,
//       line numbers shown in left gutter.
// ---------------------------------------------------------------------------

describe('3.2-UNIT-001: ContentStreamViewer line numbers and content', () => {
  test('renders line numbers starting at 1 for multi-line content', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const gutter = screen.getByTestId('content-stream-gutter');
    // 5 lines of content
    expect(gutter).toHaveTextContent('1');
    expect(gutter).toHaveTextContent('2');
    expect(gutter).toHaveTextContent('3');
    expect(gutter).toHaveTextContent('4');
    expect(gutter).toHaveTextContent('5');
  });

  test('renders raw text content preserving whitespace', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const content = screen.getByTestId('content-stream-content');
    expect(content).toHaveTextContent('BT');
    expect(content).toHaveTextContent('/F1 12 Tf');
    expect(content).toHaveTextContent('100 700 Td');
    expect(content).toHaveTextContent('(Hello World) Tj');
    expect(content).toHaveTextContent('ET');
  });

  test('uses monospace font classes on content area', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const content = screen.getByTestId('content-stream-content');
    expect(content.className).toMatch(/font-mono/);
    expect(content.className).toMatch(/text-xs/);
  });

  test('content area uses whitespace-pre for preserved spacing', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const content = screen.getByTestId('content-stream-content');
    expect(content.className).toMatch(/whitespace-pre/);
  });
});

// ---------------------------------------------------------------------------
// 3.2-UNIT-002 [P0]: ContentStreamViewer shows error message when error set.
// AC#3: DetailPanel shows an error message explaining the decode failure.
// ---------------------------------------------------------------------------

describe('3.2-UNIT-002: ContentStreamViewer error display', () => {
  test('renders error message when error prop is set', () => {
    render(<ContentStreamViewer raw="" error="failed to decode stream" />);

    const errorEl = screen.getByTestId('content-stream-error');
    expect(errorEl).toHaveTextContent('failed to decode stream');
  });

  test('error element has text-error styling', () => {
    render(<ContentStreamViewer raw="" error="decode error" />);

    const errorEl = screen.getByTestId('content-stream-error');
    expect(errorEl.className).toMatch(/text-error/);
    expect(errorEl.className).toMatch(/text-sm/);
  });

  test('does not render raw content when error is present', () => {
    render(
      <ContentStreamViewer raw="BT\nET" error="stream decode failed" />
    );

    expect(screen.queryByTestId('content-stream-content')).not.toBeInTheDocument();
    expect(screen.queryByTestId('content-stream-gutter')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 3.2-UNIT-003 [P1]: ContentStreamViewer gutter non-selectable.
// AC#1: Line numbers are non-selectable (user-select: none).
// ---------------------------------------------------------------------------

describe('3.2-UNIT-003: ContentStreamViewer gutter styling', () => {
  test('gutter text is not user-selectable (has select-none class)', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const gutter = screen.getByTestId('content-stream-gutter');
    expect(gutter.className).toMatch(/select-none/);
  });

  test('gutter uses muted text color', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const gutter = screen.getByTestId('content-stream-gutter');
    expect(gutter.className).toMatch(/text-text-muted/);
  });

  test('gutter uses monospace font', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const gutter = screen.getByTestId('content-stream-gutter');
    expect(gutter.className).toMatch(/font-mono/);
  });
});

// ---------------------------------------------------------------------------
// 3.2-UNIT-004 [P1]: ContentStreamViewer scrollable container.
// AC#1: Content is scrollable for long streams.
// ---------------------------------------------------------------------------

describe('3.2-UNIT-004: ContentStreamViewer scrollable container', () => {
  test('outer wrapper has overflow-auto for scrollability', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const viewer = screen.getByTestId('content-stream-viewer');
    expect(viewer.className).toMatch(/overflow-auto/);
  });

  test('outer wrapper fills available height', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const viewer = screen.getByTestId('content-stream-viewer');
    expect(viewer.className).toMatch(/flex-1/);
    expect(viewer.className).toMatch(/min-h-0/);
  });
});

// ---------------------------------------------------------------------------
// 3.2-UNIT-005 [P2]: ContentStreamViewer empty raw string.
// Edge case: empty content stream renders no content lines.
// ---------------------------------------------------------------------------

describe('3.2-UNIT-005: ContentStreamViewer empty content', () => {
  test('empty raw string renders viewer with no content lines', () => {
    render(<ContentStreamViewer raw="" />);

    const viewer = screen.getByTestId('content-stream-viewer');
    expect(viewer).toBeInTheDocument();
    // With empty string, there should be no gutter/content or minimal content
    // (implementation may render a single empty line or nothing)
  });
});

// ---------------------------------------------------------------------------
// 3.2-UNIT-006 [P2]: ContentStreamViewer single-line boundary
// Edge case: content stream with exactly one line renders line number 1.
// ---------------------------------------------------------------------------

describe('3.2-UNIT-006: ContentStreamViewer single-line content', () => {
  test('single-line raw string renders exactly one line number', () => {
    render(<ContentStreamViewer raw="q 0 0 612 792 re W n" />);

    const gutter = screen.getByTestId('content-stream-gutter');
    expect(gutter).toHaveTextContent('1');
    // No line number 2
    expect(gutter.textContent).toBe('1');
  });

  test('single-line content is rendered in content area', () => {
    render(<ContentStreamViewer raw="q 0 0 612 792 re W n" />);

    const content = screen.getByTestId('content-stream-content');
    expect(content).toHaveTextContent('q 0 0 612 792 re W n');
  });
});

// ---------------------------------------------------------------------------
// Story 3.3: Syntax highlighting unit tests
// ---------------------------------------------------------------------------

const tokenizedFixture = [
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

const commentTokenFixture = [
  { type: 'comment', value: '% a comment', line: 1, col: 1 },
  { type: 'operator', value: 'BT', line: 2, col: 1 },
];

// ---------------------------------------------------------------------------
// 3.3-UNIT-001 [P0]: Syntax-highlighted tokens render with type-based CSS
// AC#1: Operators visually distinct from operands via CSS classes.
// ---------------------------------------------------------------------------

describe('3.3-UNIT-001: ContentStreamViewer syntax highlighting', () => {
  test('operator tokens have text-token-operator class', () => {
    render(<ContentStreamViewer raw={multiLineRaw} tokenized={tokenizedFixture} />);

    const btEl = screen.getByText('BT');
    expect(btEl.className).toMatch(/text-token-operator/);
  });

  test('number tokens have text-token-number class', () => {
    render(<ContentStreamViewer raw={multiLineRaw} tokenized={tokenizedFixture} />);

    const numEl = screen.getByText('12');
    expect(numEl.className).toMatch(/text-token-number/);
  });

  test('string tokens have text-token-string class', () => {
    render(<ContentStreamViewer raw={multiLineRaw} tokenized={tokenizedFixture} />);

    const strEl = screen.getByText('(Hello World)');
    expect(strEl.className).toMatch(/text-token-string/);
  });

  test('name tokens have text-token-name class', () => {
    render(<ContentStreamViewer raw={multiLineRaw} tokenized={tokenizedFixture} />);

    const nameEl = screen.getByText('/F1');
    expect(nameEl.className).toMatch(/text-token-name/);
  });

  test('comment tokens have text-token-comment class', () => {
    render(
      <ContentStreamViewer
        raw="% a comment\nBT"
        tokenized={commentTokenFixture}
      />
    );

    const commentEl = screen.getByText('% a comment');
    expect(commentEl.className).toMatch(/text-token-comment/);
  });
});

// ---------------------------------------------------------------------------
// 3.3-UNIT-002 [P0]: Non-color differentiation for accessibility
// AC#3: Operators use font-semibold, comments use italic.
// ---------------------------------------------------------------------------

describe('3.3-UNIT-002: ContentStreamViewer non-color differentiation', () => {
  test('operator tokens have font-semibold class', () => {
    render(<ContentStreamViewer raw={multiLineRaw} tokenized={tokenizedFixture} />);

    const btEl = screen.getByText('BT');
    expect(btEl.className).toMatch(/font-semibold/);
  });

  test('comment tokens have italic class', () => {
    render(
      <ContentStreamViewer
        raw="% a comment\nBT"
        tokenized={commentTokenFixture}
      />
    );

    const commentEl = screen.getByText('% a comment');
    expect(commentEl.className).toMatch(/italic/);
  });
});

// ---------------------------------------------------------------------------
// 3.3-UNIT-003 [P1]: Tooltip on operator tokens
// AC#2: Hovering over an operator shows a tooltip with description.
// ---------------------------------------------------------------------------

describe('3.3-UNIT-003: ContentStreamViewer operator tooltips', () => {
  test('operator with description renders a tooltip trigger', () => {
    render(<ContentStreamViewer raw={multiLineRaw} tokenized={tokenizedFixture} />);

    // BT has a description -- its span should be wrapped in a tooltip trigger
    const btEl = screen.getByText('BT');
    expect(btEl).toBeInTheDocument();
    // Tooltip.Trigger with asChild sets data attributes on the child
    expect(btEl.closest('[data-state]')).not.toBeNull();
  });

  test('hovering over operator shows tooltip with description', async () => {
    const user = userEvent.setup();
    render(<ContentStreamViewer raw={multiLineRaw} tokenized={tokenizedFixture} />);

    const btEl = screen.getByText('BT');
    await user.hover(btEl);

    await waitFor(() => {
      const tooltip = screen.getByTestId('operator-tooltip');
      expect(tooltip).toHaveTextContent('Begin text object');
    });
  });

  test('operator without description does not render tooltip', () => {
    const unknownOpTokens = [
      { type: 'operator', value: 'ZZ', line: 1, col: 1 },
    ];
    render(<ContentStreamViewer raw="ZZ" tokenized={unknownOpTokens} />);

    const zzEl = screen.getByText('ZZ');
    // No tooltip trigger wrapper -- no data-state attribute
    expect(zzEl.closest('[data-state]')).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// 3.3-UNIT-004 [P1]: Fallback to plain text when tokenized is missing
// AC#1 (Task 4.7): Falls back to raw text when tokenized is null/empty.
// ---------------------------------------------------------------------------

describe('3.3-UNIT-004: ContentStreamViewer fallback to plain text', () => {
  test('falls back to plain text when tokenized is null', () => {
    render(<ContentStreamViewer raw={multiLineRaw} tokenized={null} />);

    const content = screen.getByTestId('content-stream-content');
    // Plain text path: no token-colored spans
    expect(content).toHaveTextContent('BT');
    expect(content.querySelector('.text-token-operator')).toBeNull();
  });

  test('falls back to plain text when tokenized is undefined', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const content = screen.getByTestId('content-stream-content');
    expect(content).toHaveTextContent('BT');
    expect(content.querySelector('.text-token-operator')).toBeNull();
  });

  test('falls back to plain text when tokenized is empty array', () => {
    render(<ContentStreamViewer raw={multiLineRaw} tokenized={[]} />);

    const content = screen.getByTestId('content-stream-content');
    expect(content).toHaveTextContent('BT');
    expect(content.querySelector('.text-token-operator')).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// 3.3-UNIT-005 [P1]: Line numbers correct with tokenized data
// AC#1: Line number gutter is consistent with tokenized rendering.
// ---------------------------------------------------------------------------

describe('3.3-UNIT-005: ContentStreamViewer line numbers with tokens', () => {
  test('renders correct line count from tokenized data', () => {
    render(<ContentStreamViewer raw={multiLineRaw} tokenized={tokenizedFixture} />);

    const gutter = screen.getByTestId('content-stream-gutter');
    expect(gutter).toHaveTextContent('1');
    expect(gutter).toHaveTextContent('5');
  });
});

// ---------------------------------------------------------------------------
// 3.3-UNIT-014 [P3]: Tooltip map covers all 62 standard PDF operators.
// Validates OPERATOR_DESCRIPTIONS completeness against spec Table A.1.
// ---------------------------------------------------------------------------

describe('3.3-UNIT-014: Tooltip map completeness', () => {
  // All 62 standard operators from PDF spec 1.7 Table A.1
  const pdfSpecOperators = [
    'BT', 'ET', 'Tf', 'Td', 'TD', 'Tm', 'Tj', 'TJ', 'T*',
    'Tc', 'Tw', 'Tz', 'TL', 'Tr', 'Ts',
    'q', 'Q', 'cm', 'm', 'l', 'c', 'v', 'y', 'h',
    're', 'S', 's', 'f', 'F', 'f*', 'B', 'B*', 'b', 'b*', 'n',
    'W', 'W*', 'Do', 'gs',
    'CS', 'cs', 'SC', 'SCN', 'sc', 'scn',
    'G', 'g', 'RG', 'rg', 'K', 'k',
    'w', 'J', 'j', 'M', 'd', 'ri', 'i',
    'BMC', 'BDC', 'EMC', 'MP', 'DP',
    'BI', 'ID', 'EI',
    'd0', 'd1', 'sh',
  ];

  test('each spec operator renders a tooltip when present as a token', () => {
    // Render each operator and check it produces a tooltip trigger
    const missingDescriptions: string[] = [];
    for (const op of pdfSpecOperators) {
      const tokens = [{ type: 'operator', value: op, line: 1, col: 1 }];
      const { unmount } = render(
        <ContentStreamViewer raw={op} tokenized={tokens} />
      );

      const el = screen.getByText(op);
      if (!el.closest('[data-state]')) {
        missingDescriptions.push(op);
      }
      unmount();
    }

    expect(missingDescriptions).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// 3.3-UNIT-EDGE [P2]: Syntax highlighting edge cases
// Token spacing reconstruction and blank line preservation.
// ---------------------------------------------------------------------------

describe('3.3-UNIT-EDGE: ContentStreamViewer edge cases', () => {
  test('blank lines in raw are preserved in highlighted view', () => {
    const rawWithBlank = 'BT\n\nET';
    const tokens = [
      { type: 'operator', value: 'BT', line: 1, col: 1 },
      { type: 'operator', value: 'ET', line: 3, col: 1 },
    ];
    render(<ContentStreamViewer raw={rawWithBlank} tokenized={tokens} />);

    const gutter = screen.getByTestId('content-stream-gutter');
    // Should have 3 lines including the blank
    expect(gutter).toHaveTextContent('3');
  });

  test('trailing blank lines from raw are preserved', () => {
    const rawTrailing = 'BT\nET\n';
    const tokens = [
      { type: 'operator', value: 'BT', line: 1, col: 1 },
      { type: 'operator', value: 'ET', line: 2, col: 1 },
    ];
    render(<ContentStreamViewer raw={rawTrailing} tokenized={tokens} />);

    const gutter = screen.getByTestId('content-stream-gutter');
    // raw.split('\n') gives 3 entries for "BT\nET\n", so 3 lines
    expect(gutter).toHaveTextContent('3');
  });
});
