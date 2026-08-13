/**
 * Plain Text View tests -- retained tests from Story 9.11 that survive the
 * Story 10-1 collapse (truncation banner / Load all flow removed; see
 * PlainTextView.async.test.tsx for the new async loading-card behavior).
 *
 * Run: cd frontend && npx vitest run src/components/PlainTextView.test.tsx
 */
import { render, screen, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { PlainTextView } from './PlainTextView';
import { AppProvider } from '../hooks/useDocumentState';

function ProviderWrapper({ children }: { children: ReactNode }) {
  return <AppProvider>{children}</AppProvider>;
}

// --- Mocks ---

const mockGetPlainText = vi.fn();
const mockGetPlainTextSize = vi.fn();
const mockCancelPlainText = vi.fn();

vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    GetPlainText: (...args: unknown[]) => mockGetPlainText(...args),
    GetPlainTextSize: (...args: unknown[]) => mockGetPlainTextSize(...args),
    CancelPlainText: (...args: unknown[]) => mockCancelPlainText(...args),
  })
);

// --- Fixtures ---

type PlainTextDocumentFixture = {
  tabId: string;
  content: string;
  totalBytes: number;
};

const smallDoc: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  content: '%PDF-1.7\nfoo\nbar\n',
  totalBytes: 17,
};

const crlfDoc: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  content: 'line1\r\nline2\rline3\nline4',
  totalBytes: 23,
};

const latin1Doc: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  // U+00E9 = e-acute (Latin-1 0xE9), U+00FF = y-dieresis (Latin-1 0xFF).
  content: 'cafeé byteÿ',
  totalBytes: 12,
};

// ---------------------------------------------------------------------------
// Renders 1-based line-number gutter + content lines.
// ---------------------------------------------------------------------------

describe('line-number gutter + content lines', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(smallDoc);
    mockGetPlainTextSize.mockResolvedValue(17);
  });

  test('gutter shows 1-based line numbers and content shows each line', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
    await waitFor(() => {
      expect(screen.getByText('%PDF-1.7')).toBeInTheDocument();
    });
    expect(screen.getByText('foo')).toBeInTheDocument();
    expect(screen.getByText('bar')).toBeInTheDocument();
    const gutter = screen.getByTestId('plain-text-gutter');
    expect(gutter.textContent).toContain('1');
    expect(gutter.textContent).toContain('2');
    expect(gutter.textContent).toContain('3');
  });
});

// ---------------------------------------------------------------------------
// CRLF / lone CR / lone LF all collapse to ONE logical line break each
// (no empty intervening row).
// ---------------------------------------------------------------------------

describe('line-break regex collapses CRLF/CR/LF', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(crlfDoc);
    mockGetPlainTextSize.mockResolvedValue(23);
  });

  test('"line1\\r\\nline2\\rline3\\nline4" produces exactly 4 logical lines', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
    await waitFor(() => {
      expect(screen.getByText('line1')).toBeInTheDocument();
    });
    expect(screen.getByText('line2')).toBeInTheDocument();
    expect(screen.getByText('line3')).toBeInTheDocument();
    expect(screen.getByText('line4')).toBeInTheDocument();
    const rows = screen.getAllByTestId(/^plain-text-row-/);
    expect(rows.length).toBe(4);
  });
});

// ---------------------------------------------------------------------------
// Latin-1 high bytes (0x80-0xFF) render verbatim.
// ---------------------------------------------------------------------------

describe('Latin-1 high bytes render verbatim', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(latin1Doc);
    mockGetPlainTextSize.mockResolvedValue(12);
  });

  test('U+00E9 and U+00FF survive end-to-end render', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
    await waitFor(() => {
      expect(screen.getByText(/cafeé/)).toBeInTheDocument();
    });
    expect(screen.getByText(/byteÿ/)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Lazy fetch gated by active prop.
// ---------------------------------------------------------------------------

describe('lazy fetch gated by active prop', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(smallDoc);
    mockGetPlainTextSize.mockResolvedValue(17);
  });

  test('active=false does NOT fetch', () => {
    render(<PlainTextView tabId="tab-1" active={false} />, { wrapper: ProviderWrapper });
    expect(mockGetPlainText).not.toHaveBeenCalled();
  });

  test('active=true fetches exactly once for a given tabId', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
    await waitFor(() => {
      expect(mockGetPlainText).toHaveBeenCalledWith('tab-1');
    });
    expect(mockGetPlainText).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// Task 7.8: empty state when no tabId / no document.
// ---------------------------------------------------------------------------

describe('empty state when no document', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('plain-text-empty visible when tabId is empty', () => {
    render(<PlainTextView tabId="" active={true} />, { wrapper: ProviderWrapper });
    expect(screen.getByTestId('plain-text-empty')).toBeInTheDocument();
    expect(mockGetPlainText).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Task 7.5: virtualization performance smoke test.
// ---------------------------------------------------------------------------

describe('virtualization keeps DOM small', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('10,000-line payload renders fewer than 200 row DOM nodes', async () => {
    const lines = Array.from({ length: 10000 }, (_, i) => `line ${i + 1}`).join('\n');
    const bigDoc: PlainTextDocumentFixture = {
      tabId: 'tab-1',
      content: lines,
      totalBytes: lines.length,
    };
    mockGetPlainText.mockResolvedValue(bigDoc);
    mockGetPlainTextSize.mockResolvedValue(lines.length);
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
    await waitFor(() => {
      expect(screen.getAllByTestId(/^plain-text-row-/).length).toBeGreaterThan(0);
    });
    const rows = screen.getAllByTestId(/^plain-text-row-/);
    expect(rows.length).toBeLessThan(200);
  });
});
