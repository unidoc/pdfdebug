/**
 * Story 3.2: Content Stream Viewer -- Raw Text Display (Prototype)
 *
 * TDD RED PHASE: Tests MUST fail until ContentStreamViewer.tsx is implemented.
 *
 * Test IDs: 3.2-UNIT-001 through 3.2-UNIT-005 (Vitest)
 * Run: cd frontend && npx vitest run src/components/ContentStreamViewer.test.tsx
 */
import { render, screen } from '@testing-library/react';
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
