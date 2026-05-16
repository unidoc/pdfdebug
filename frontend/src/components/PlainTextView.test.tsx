/**
 * Story 9.11: Plain Text View Tests
 *
 * TDD RED PHASE: Tests MUST fail until PlainTextView.tsx is created and
 * GetPlainText is wired through the Wails bindings.
 *
 * Covers AC#6 (Latin-1 round-trip, line-break regex, gutter line numbers),
 * AC#7 (truncation banner reads capBytes + totalBytes from payload),
 * AC#10 (200ms loading debounce),
 * AC#11 (scroll-to-top on tab activation),
 * AC#13 (error rendering with mapped message).
 *
 * Run: cd frontend && npx vitest run src/components/PlainTextView.test.tsx
 */
import { render, screen, waitFor, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
// RED PHASE: this import fails until PlainTextView.tsx exists.
import { PlainTextView } from './PlainTextView';

// --- Mocks ---

const mockGetPlainText = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    GetPlainText: (...args: unknown[]) => mockGetPlainText(...args),
  })
);

// --- Fixtures ---

type PlainTextDocumentFixture = {
  tabId: string;
  content: string;
  totalBytes: number;
  truncated: boolean;
  capBytes: number;
};

const smallDoc: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  content: '%PDF-1.7\nfoo\nbar\n',
  totalBytes: 17,
  truncated: false,
  capBytes: 5242880,
};

const truncatedDoc: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  content: 'A'.repeat(5242880),
  totalBytes: 7340032, // 7 MiB
  truncated: true,
  capBytes: 5242880,
};

const crlfDoc: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  content: 'line1\r\nline2\rline3\nline4',
  totalBytes: 23,
  truncated: false,
  capBytes: 5242880,
};

const latin1Doc: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  // U+00E9 = e-acute (Latin-1 0xE9), U+00FF = y-dieresis (Latin-1 0xFF).
  content: 'cafeé byteÿ',
  totalBytes: 12,
  truncated: false,
  capBytes: 5242880,
};

// ---------------------------------------------------------------------------
// 9.11-UNIT-101 [P0] AC#6: renders 1-based line-number gutter + content lines.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-101: line-number gutter + content lines', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(smallDoc);
  });

  test('gutter shows 1-based line numbers and content shows each line', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />);
    await waitFor(() => {
      expect(screen.getByText('%PDF-1.7')).toBeInTheDocument();
    });
    expect(screen.getByText('foo')).toBeInTheDocument();
    expect(screen.getByText('bar')).toBeInTheDocument();
    // Line numbers 1, 2, 3 visible in the gutter.
    const gutter = screen.getByTestId('plain-text-gutter');
    expect(gutter.textContent).toContain('1');
    expect(gutter.textContent).toContain('2');
    expect(gutter.textContent).toContain('3');
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-102 [P0] AC#6: CRLF / lone CR / lone LF all collapse to ONE
// logical line break each (no empty intervening row).
// ---------------------------------------------------------------------------

describe('9.11-UNIT-102: line-break regex collapses CRLF/CR/LF', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(crlfDoc);
  });

  test('"line1\\r\\nline2\\rline3\\nline4" produces exactly 4 logical lines', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />);
    await waitFor(() => {
      expect(screen.getByText('line1')).toBeInTheDocument();
    });
    expect(screen.getByText('line2')).toBeInTheDocument();
    expect(screen.getByText('line3')).toBeInTheDocument();
    expect(screen.getByText('line4')).toBeInTheDocument();
    // No "empty" intervening row -- exactly 4 content rows.
    const rows = screen.getAllByTestId(/^plain-text-row-/);
    expect(rows.length).toBe(4);
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-103 [P0] AC#6: Latin-1 high bytes (0x80-0xFF) render verbatim
// (already decoded by the backend). The frontend MUST NOT re-decode.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-103: Latin-1 high bytes render verbatim', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(latin1Doc);
  });

  test('U+00E9 and U+00FF survive end-to-end render', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />);
    await waitFor(() => {
      // Both substrings appear in the rendered DOM.
      expect(screen.getByText(/cafeé/)).toBeInTheDocument();
    });
    expect(screen.getByText(/byteÿ/)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-104 [P0] AC#7: truncation banner appears with capBytes +
// totalBytes formatted via toLocaleString.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-104: truncation banner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncatedDoc);
  });

  test('banner shows "Showing first 5,242,880 of 7,340,032 bytes (truncated)."', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />);
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-truncated-banner')).toBeInTheDocument();
    });
    const banner = screen.getByTestId('plain-text-truncated-banner');
    expect(banner.textContent).toContain('5,242,880');
    expect(banner.textContent).toContain('7,340,032');
    expect(banner.textContent).toContain('truncated');
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-105 [P0] AC#7: banner does NOT appear when truncated=false.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-105: no banner when not truncated', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(smallDoc);
  });

  test('banner is absent for a small document', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />);
    await waitFor(() => {
      expect(screen.getByText('%PDF-1.7')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('plain-text-truncated-banner')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-106 [P0] AC#10: 200ms loading debounce.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-106: 200ms loading debounce', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  test('loading indicator hidden before 200ms', () => {
    let resolveFn: ((val: PlainTextDocumentFixture) => void) | null = null;
    mockGetPlainText.mockReturnValueOnce(
      new Promise<PlainTextDocumentFixture>((resolve) => {
        resolveFn = resolve;
      })
    );
    render(<PlainTextView tabId="tab-1" active={true} />);
    act(() => {
      vi.advanceTimersByTime(150);
    });
    expect(screen.queryByTestId('plain-text-loading')).not.toBeInTheDocument();
    act(() => {
      resolveFn!(smallDoc);
    });
  });

  test('loading indicator visible after 200ms while in flight', () => {
    let resolveFn: ((val: PlainTextDocumentFixture) => void) | null = null;
    mockGetPlainText.mockReturnValueOnce(
      new Promise<PlainTextDocumentFixture>((resolve) => {
        resolveFn = resolve;
      })
    );
    render(<PlainTextView tabId="tab-1" active={true} />);
    act(() => {
      vi.advanceTimersByTime(250);
    });
    expect(screen.getByTestId('plain-text-loading')).toBeInTheDocument();
    act(() => {
      resolveFn!(smallDoc);
    });
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-107 [P0] AC#13: error rendering.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-107: error rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('rejected fetch renders plain-text-error with mapped message', async () => {
    mockGetPlainText.mockRejectedValueOnce(new Error('file moved or deleted'));
    render(<PlainTextView tabId="tab-1" active={true} />);
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-error')).toBeInTheDocument();
    });
    expect(screen.getByTestId('plain-text-error').textContent).toContain('file moved or deleted');
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-108 [P0] Task 7.6 / AC#6: scroll-to-top on every tab activation.
// When `active` transitions false -> true, the scroll container's scrollTop
// is set to 0.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-108: scroll-to-top on tab activation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(smallDoc);
  });

  test('scroll container scrollTop set to 0 when active flips false -> true', async () => {
    const { rerender } = render(<PlainTextView tabId="tab-1" active={false} />);
    rerender(<PlainTextView tabId="tab-1" active={true} />);
    await waitFor(() => {
      expect(screen.getByText('%PDF-1.7')).toBeInTheDocument();
    });
    const container = screen.getByTestId('plain-text-scroll');
    // Simulate that the user scrolled, then activation flip-flops should
    // reset scrollTop. We assert scrollTop is 0 immediately after activation.
    expect(container.scrollTop).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-109 [P1] AC#10 + Task 7.2: lazy fetch gated by active prop.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-109: lazy fetch gated by active prop', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(smallDoc);
  });

  test('active=false does NOT fetch', () => {
    render(<PlainTextView tabId="tab-1" active={false} />);
    expect(mockGetPlainText).not.toHaveBeenCalled();
  });

  test('active=true fetches exactly once for a given tabId', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />);
    await waitFor(() => {
      expect(mockGetPlainText).toHaveBeenCalledWith('tab-1');
    });
    expect(mockGetPlainText).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-110 [P0] Task 7.8: empty state when no tabId / no document.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-110: empty state when no document', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('plain-text-empty visible when tabId is empty', () => {
    render(<PlainTextView tabId="" active={true} />);
    expect(screen.getByTestId('plain-text-empty')).toBeInTheDocument();
    expect(mockGetPlainText).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-111 [P1] Task 7.5: virtualization performance smoke test.
// A 10,000-line payload must render fewer than 200 row DOM nodes at once
// (proves viewport virtualization is active, not whole-list render).
// ---------------------------------------------------------------------------

describe('9.11-UNIT-111: virtualization keeps DOM small', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('10,000-line payload renders fewer than 200 row DOM nodes', async () => {
    const lines = Array.from({ length: 10000 }, (_, i) => `line ${i + 1}`).join('\n');
    const bigDoc: PlainTextDocumentFixture = {
      tabId: 'tab-1',
      content: lines,
      totalBytes: lines.length,
      truncated: false,
      capBytes: 5242880,
    };
    mockGetPlainText.mockResolvedValue(bigDoc);
    render(<PlainTextView tabId="tab-1" active={true} />);
    await waitFor(() => {
      // Wait for at least one row to render so we know the fetch resolved.
      expect(screen.getAllByTestId(/^plain-text-row-/).length).toBeGreaterThan(0);
    });
    const rows = screen.getAllByTestId(/^plain-text-row-/);
    // Virtualization upper bound -- viewport + overscan. The story pins <200.
    expect(rows.length).toBeLessThan(200);
  });
});
