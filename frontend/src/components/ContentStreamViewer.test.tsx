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
import { describe, test, expect, vi } from 'vitest';
// RED PHASE: This import will fail until ContentStreamViewer.tsx is created.
import { ContentStreamViewer, type StreamViewMode } from './ContentStreamViewer';

// 9-6: tests previously passed flat token arrays via the old `tokenized` prop;
// the component now consumes pre-grouped FormattedLine[] from the Go formatter.
// toFormatted wraps a token fixture into a single formatted row so the
// per-token highlighting/tooltip assertions still hold without re-deriving
// operator boundaries in the test layer.
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
  test('content area has overflow-auto for scrollability', () => {
    render(<ContentStreamViewer raw={multiLineRaw} />);

    const viewer = screen.getByTestId('content-stream-viewer');
    // overflow-auto is on the inner scroll container (child of the outer wrapper)
    const scrollArea = viewer.querySelector('.overflow-auto');
    expect(scrollArea).not.toBeNull();
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
    render(<ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(tokenizedFixture)} />);

    const btEl = screen.getByText('BT');
    expect(btEl.className).toMatch(/text-token-operator/);
  });

  test('number tokens have text-token-number class', () => {
    render(<ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(tokenizedFixture)} />);

    const numEl = screen.getByText('12');
    expect(numEl.className).toMatch(/text-token-number/);
  });

  test('string tokens have text-token-string class', () => {
    render(<ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(tokenizedFixture)} />);

    const strEl = screen.getByText('(Hello World)');
    expect(strEl.className).toMatch(/text-token-string/);
  });

  test('name tokens have text-token-name class', () => {
    render(<ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(tokenizedFixture)} />);

    const nameEl = screen.getByText('/F1');
    expect(nameEl.className).toMatch(/text-token-name/);
  });

  test('comment tokens have text-token-comment class', () => {
    render(
      <ContentStreamViewer
        raw="% a comment\nBT"
        formatted={toFormatted(commentTokenFixture)}
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
    render(<ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(tokenizedFixture)} />);

    const btEl = screen.getByText('BT');
    expect(btEl.className).toMatch(/font-semibold/);
  });

  test('comment tokens have italic class', () => {
    render(
      <ContentStreamViewer
        raw="% a comment\nBT"
        formatted={toFormatted(commentTokenFixture)}
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
    render(<ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(tokenizedFixture)} />);

    // BT has a description -- its span should be wrapped in a tooltip trigger
    const btEl = screen.getByText('BT');
    expect(btEl).toBeInTheDocument();
    // Tooltip.Trigger with asChild sets data attributes on the child
    expect(btEl.closest('[data-state]')).not.toBeNull();
  });

  test('hovering over operator shows tooltip with description', async () => {
    const user = userEvent.setup();
    render(<ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(tokenizedFixture)} />);

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
    render(<ContentStreamViewer raw="ZZ" formatted={toFormatted(unknownOpTokens)} />);

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
    render(<ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(null)} />);

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
    render(<ContentStreamViewer raw={multiLineRaw} formatted={[]} />);

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
  test('renders one gutter row per formatted line (story 9-6 semantics)', () => {
    // 5-row formatted fixture: BT, Tf, Td, Tj, ET -- one logical operation each.
    const fiveRows = [
      { tokens: [{ type: 'operator', value: 'BT', line: 1, col: 1 }], indent: 0, operator: 'BT', srcLineStart: 1, srcLineEnd: 1 },
      { tokens: [{ type: 'operator', value: 'Tf', line: 2, col: 8 }], indent: 0, operator: 'Tf', srcLineStart: 2, srcLineEnd: 2 },
      { tokens: [{ type: 'operator', value: 'Td', line: 3, col: 9 }], indent: 0, operator: 'Td', srcLineStart: 3, srcLineEnd: 3 },
      { tokens: [{ type: 'operator', value: 'Tj', line: 4, col: 15 }], indent: 0, operator: 'Tj', srcLineStart: 4, srcLineEnd: 4 },
      { tokens: [{ type: 'operator', value: 'ET', line: 5, col: 1 }], indent: 0, operator: 'ET', srcLineStart: 5, srcLineEnd: 5 },
    ];
    render(<ContentStreamViewer raw={multiLineRaw} formatted={fiveRows} />);

    const gutter = screen.getByTestId('content-stream-gutter');
    // Gutter is keyed by formatted-row index, not source line, so 1..5.
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
        <ContentStreamViewer raw={op} formatted={toFormatted(tokens)} />
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
  // Story 9-6: in formatted view the gutter is keyed by formatted-row index,
  // not source line, so blank source lines no longer map to a gutter row.
  // The Raw view (which is byte-faithful) preserves source-line semantics
  // including blanks; assert there.
  test('blank source lines are preserved in raw view', () => {
    const rawWithBlank = 'BT\n\nET';
    render(<ContentStreamViewer raw={rawWithBlank} />);

    const gutter = screen.getByTestId('content-stream-gutter');
    // raw.split('\n') -> ['BT','','ET'] -> gutter shows 1, 2, 3
    expect(gutter).toHaveTextContent('3');
  });

  test('trailing blank lines from raw are preserved in raw view', () => {
    const rawTrailing = 'BT\nET\n';
    render(<ContentStreamViewer raw={rawTrailing} />);

    const gutter = screen.getByTestId('content-stream-gutter');
    // raw.split('\n') -> ['BT','ET',''] -> 3 gutter rows
    expect(gutter).toHaveTextContent('3');
  });
});

// ---------------------------------------------------------------------------
// Story 3.4: View mode toggle unit tests
// ---------------------------------------------------------------------------

// Helper to render with controlled view mode. Accepts a flat token fixture
// for ergonomic test authoring; the helper wraps it via toFormatted().
function renderWithToggle(
  props: {
    raw: string;
    tokenized?: TokFixture | null;
    error?: string;
    viewMode?: StreamViewMode;
  },
) {
  const onViewModeChange = vi.fn();
  const formatted = props.tokenized ? toFormatted(props.tokenized) : null;
  const result = render(
    <ContentStreamViewer
      raw={props.raw}
      formatted={formatted}
      error={props.error}
      viewMode={props.viewMode ?? 'formatted'}
      onViewModeChange={onViewModeChange}
    />
  );
  return { ...result, onViewModeChange };
}

// ---------------------------------------------------------------------------
// 3.4-UNIT-001 [P0]: Segmented control renders with Formatted and Raw options
// AC#1: Segmented control appears above stream content.
// ---------------------------------------------------------------------------

describe('3.4-UNIT-001: View mode segmented control rendering', () => {
  test('renders Formatted and Raw buttons when onViewModeChange is provided', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: tokenizedFixture as TokFixture });

    expect(screen.getByTestId('view-mode-control')).toBeInTheDocument();
    expect(screen.getByTestId('view-mode-formatted')).toHaveTextContent('Formatted');
    expect(screen.getByTestId('view-mode-raw')).toHaveTextContent('Raw');
  });

  test('does not render segmented control when onViewModeChange is not provided', () => {
    render(<ContentStreamViewer raw={multiLineRaw} formatted={toFormatted(tokenizedFixture)} />);

    expect(screen.queryByTestId('view-mode-control')).not.toBeInTheDocument();
  });

  test('segmented control has tablist role', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: tokenizedFixture as TokFixture });

    expect(screen.getByTestId('view-mode-control')).toHaveAttribute('role', 'tablist');
  });

  test('buttons have tab role with aria-selected', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: tokenizedFixture, viewMode: 'formatted' });

    expect(screen.getByTestId('view-mode-formatted')).toHaveAttribute('role', 'tab');
    expect(screen.getByTestId('view-mode-formatted')).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByTestId('view-mode-raw')).toHaveAttribute('role', 'tab');
    expect(screen.getByTestId('view-mode-raw')).toHaveAttribute('aria-selected', 'false');
  });
});

// ---------------------------------------------------------------------------
// 3.4-UNIT-002 [P0]: Default view mode is Formatted
// AC#1: Default selection is "Formatted" when tokenized data is available.
// ---------------------------------------------------------------------------

describe('3.4-UNIT-002: Default view mode is Formatted', () => {
  test('Formatted is selected by default when tokens available', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: tokenizedFixture as TokFixture });

    expect(screen.getByTestId('view-mode-formatted')).toHaveAttribute('aria-selected', 'true');
    // Should render syntax-highlighted tokens
    const content = screen.getByTestId('content-stream-content');
    expect(content.querySelector('.text-token-operator')).not.toBeNull();
  });
});

// ---------------------------------------------------------------------------
// 3.4-UNIT-003 [P0]: Clicking Raw switches to plain text rendering
// AC#2: Stream content switches to plain text with no syntax highlighting.
// ---------------------------------------------------------------------------

describe('3.4-UNIT-003: Switching to Raw mode', () => {
  test('clicking Raw calls onViewModeChange with "raw"', async () => {
    const user = userEvent.setup();
    const { onViewModeChange } = renderWithToggle({ raw: multiLineRaw, tokenized: tokenizedFixture as TokFixture });

    await user.click(screen.getByTestId('view-mode-raw'));
    expect(onViewModeChange).toHaveBeenCalledWith('raw');
  });

  test('raw mode renders plain text without token CSS classes', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: tokenizedFixture, viewMode: 'raw' });

    const content = screen.getByTestId('content-stream-content');
    expect(content.querySelector('.text-token-operator')).toBeNull();
    expect(content.querySelector('.text-token-number')).toBeNull();
    expect(content).toHaveTextContent('BT');
    expect(content).toHaveTextContent('ET');
  });

  test('raw mode still renders line numbers', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: tokenizedFixture, viewMode: 'raw' });

    const gutter = screen.getByTestId('content-stream-gutter');
    expect(gutter).toHaveTextContent('1');
    expect(gutter).toHaveTextContent('5');
  });

  test('Raw button shows as selected in raw mode', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: tokenizedFixture, viewMode: 'raw' });

    expect(screen.getByTestId('view-mode-raw')).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByTestId('view-mode-formatted')).toHaveAttribute('aria-selected', 'false');
  });
});

// ---------------------------------------------------------------------------
// 3.4-UNIT-004 [P0]: Clicking Formatted switches back to highlighted view
// AC#2: Toggling back restores syntax highlighting.
// ---------------------------------------------------------------------------

describe('3.4-UNIT-004: Switching back to Formatted mode', () => {
  test('clicking Formatted calls onViewModeChange with "formatted"', async () => {
    const user = userEvent.setup();
    const { onViewModeChange } = renderWithToggle({ raw: multiLineRaw, tokenized: tokenizedFixture, viewMode: 'raw' });

    await user.click(screen.getByTestId('view-mode-formatted'));
    expect(onViewModeChange).toHaveBeenCalledWith('formatted');
  });

  test('formatted mode renders syntax-highlighted tokens', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: tokenizedFixture, viewMode: 'formatted' });

    const content = screen.getByTestId('content-stream-content');
    expect(content.querySelector('.text-token-operator')).not.toBeNull();
  });
});

// ---------------------------------------------------------------------------
// 3.4-UNIT-005 [P1]: Formatted disabled when tokenized data is unavailable
// AC#4: Formatted option is disabled, defaults to Raw.
// ---------------------------------------------------------------------------

describe('3.4-UNIT-005: Formatted disabled when no tokens', () => {
  test('Formatted button is disabled when tokenized is null', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: null });

    const btn = screen.getByTestId('view-mode-formatted');
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute('aria-disabled', 'true');
  });

  test('Formatted button is disabled when tokenized is empty', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: [] });

    expect(screen.getByTestId('view-mode-formatted')).toBeDisabled();
  });

  test('forces raw mode when no tokens even if viewMode is formatted', () => {
    renderWithToggle({ raw: multiLineRaw, tokenized: null, viewMode: 'formatted' });

    // Should render raw despite viewMode prop
    const content = screen.getByTestId('content-stream-content');
    expect(content.querySelector('.text-token-operator')).toBeNull();
    expect(screen.getByTestId('view-mode-raw')).toHaveAttribute('aria-selected', 'true');
  });
});

// ---------------------------------------------------------------------------
// 3.4-UNIT-006 [P1]: Error state takes priority over view mode
// AC#5: Error display shown regardless of view mode.
// ---------------------------------------------------------------------------

describe('3.4-UNIT-006: Error state priority over view mode', () => {
  test('error renders error display, no segmented control', () => {
    renderWithToggle({ raw: '', error: 'decode failed' });

    expect(screen.getByTestId('content-stream-error')).toHaveTextContent('decode failed');
    expect(screen.queryByTestId('view-mode-control')).not.toBeInTheDocument();
  });
});
