/**
 * Story 10.7: Frontend Hook and Render-Path Correctness
 * AC7 (finding #5) -- raw-mode line splitting handles CR / CRLF / mixed.
 *
 * TDD RED PHASE: emitted as `test()`. Asserts the POST-FIX split at
 * ContentStreamViewer.tsx:222 -- `raw.split(/\r\n?|\n/)` instead of
 * `raw.split('\n')`. Against the current code, CR-only input
 * ("line1\rline2\rline3") splits into a SINGLE element, so the gutter shows 1
 * line number and the content column shows 1 row -- these assertions fail. A
 * developer activates them by removing `.skip` after Task 5.
 *
 * Raw mode is forced by passing no `formatted` prop (the component falls back
 * to raw rendering when formatted is empty). Each raw line renders as a
 * <div> inside data-testid="content-stream-content"; the gutter renders one
 * <div>{i+1}</div> per line inside data-testid="content-stream-gutter".
 *
 * Test IDs follow the 10-7-UNIT-NNN convention.
 *
 * Run: cd frontend && npx vitest run src/components/ContentStreamViewer.line-endings.test.tsx
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { ContentStreamViewer } from './ContentStreamViewer';

// Count rendered rows by reading the immediate child <div> count of the
// content column. The gutter line count must match.
function rowCount(): number {
  const content = screen.getByTestId('content-stream-content');
  return content.children.length;
}

function gutterLineNumbers(): string[] {
  const gutter = screen.getByTestId('content-stream-gutter');
  return Array.from(gutter.children).map((el) => el.textContent ?? '');
}

// ---------------------------------------------------------------------------
// 10-7-UNIT-004 [P1] AC7: LF-only (baseline that already works).
// ---------------------------------------------------------------------------

describe('LF-only line endings render N rows', () => {
  test('"line1\\nline2\\nline3" renders 3 rows and gutter 1,2,3', () => {
    render(<ContentStreamViewer raw={'line1\nline2\nline3'} />);
    expect(rowCount()).toBe(3);
    expect(gutterLineNumbers()).toEqual(['1', '2', '3']);
  });
});

// ---------------------------------------------------------------------------
// 10-7-UNIT-005 [P1] AC7: CR-only line endings -- the core fix.
// ---------------------------------------------------------------------------

describe('CR-only line endings render N rows', () => {
  test('"line1\\rline2\\rline3" renders 3 rows and gutter 1,2,3', () => {
    render(<ContentStreamViewer raw={'line1\rline2\rline3'} />);
    expect(rowCount()).toBe(3);
    expect(gutterLineNumbers()).toEqual(['1', '2', '3']);
  });

  test('each CR-split row shows the correct text', () => {
    render(<ContentStreamViewer raw={'line1\rline2\rline3'} />);
    const content = screen.getByTestId('content-stream-content');
    expect(content).toHaveTextContent('line1');
    expect(content).toHaveTextContent('line2');
    expect(content).toHaveTextContent('line3');
  });
});

// ---------------------------------------------------------------------------
// 10-7-UNIT-006 [P1] AC7: CRLF line endings count once (not as two breaks).
// ---------------------------------------------------------------------------

describe('CRLF line endings count once', () => {
  test('"line1\\r\\nline2" renders 2 rows (CRLF is a single break)', () => {
    render(<ContentStreamViewer raw={'line1\r\nline2'} />);
    expect(rowCount()).toBe(2);
    expect(gutterLineNumbers()).toEqual(['1', '2']);
  });
});

// ---------------------------------------------------------------------------
// 10-7-UNIT-007 [P1] AC7: mixed line endings in one corpus.
// ---------------------------------------------------------------------------

describe('mixed CR / LF / CRLF', () => {
  test('"a\\rb\\nc\\r\\nd" renders 4 rows', () => {
    // 3 breaks (CR, LF, CRLF) -> 4 logical lines: a, b, c, d.
    render(<ContentStreamViewer raw={'a\rb\nc\r\nd'} />);
    expect(rowCount()).toBe(4);
    expect(gutterLineNumbers()).toEqual(['1', '2', '3', '4']);
  });
});
