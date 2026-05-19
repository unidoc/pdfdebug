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
import { render, screen, waitFor, act, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { ReactNode } from 'react';
// RED PHASE: this import fails until PlainTextView.tsx exists.
import { PlainTextView } from './PlainTextView';
import { AppProvider, useAppState } from '../hooks/useDocumentState';

// Story 9-12 wraps existing 9-11 tests in AppProvider because PlainTextView
// now consumes useAppDispatch() for the global SET_DOCUMENT_ERROR path.
function ProviderWrapper({ children }: { children: ReactNode }) {
  return <AppProvider>{children}</AppProvider>;
}

// --- Mocks ---

const mockGetPlainText = vi.fn();
const mockGetPlainTextFull = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    GetPlainText: (...args: unknown[]) => mockGetPlainText(...args),
    // RED PHASE: GetPlainTextFull export does not exist on the binding yet.
    // The mock satisfies the import; the production code wiring is what is
    // missing.
    GetPlainTextFull: (...args: unknown[]) => mockGetPlainTextFull(...args),
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
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
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
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
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
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
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

  test('banner shows formatBytes-formatted size text (post 9-12 contract)', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-truncated-banner')).toBeInTheDocument();
    });
    const banner = screen.getByTestId('plain-text-truncated-banner');
    // 9-12 AC2: formatBytes integer-MB form. 5242880 = 5 MiB -> "5 MB";
    // 7340032 = 7 MiB -> "7 MB". The word "truncated" no longer appears.
    expect(banner.textContent).toContain('Showing first 5 MB of 7 MB.');
    expect(banner.textContent).not.toContain('truncated');
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
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
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
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
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
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
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
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
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
    const { rerender } = render(<PlainTextView tabId="tab-1" active={false} />, { wrapper: ProviderWrapper });
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
// 9.11-UNIT-110 [P0] Task 7.8: empty state when no tabId / no document.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-110: empty state when no document', () => {
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
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });
    await waitFor(() => {
      // Wait for at least one row to render so we know the fetch resolved.
      expect(screen.getAllByTestId(/^plain-text-row-/).length).toBeGreaterThan(0);
    });
    const rows = screen.getAllByTestId(/^plain-text-row-/);
    // Virtualization upper bound -- viewport + overscan. The story pins <200.
    expect(rows.length).toBeLessThan(200);
  });
});

// ===========================================================================
// Story 9-12: "Load all" button + banner refactor.
//
// RED PHASE: every test below fails until PlainTextView is refactored per
// story 9-12 (banner layout, formatBytes copy, Load all button, loading +
// retry state machine, stale-fetch guard). The production code is unchanged
// at this commit; tests assert the *expected* post-9-12 contract.
//
// Test IDs follow the 9.11-UNIT-NNN convention extended into the -120s.
// ===========================================================================

const SIZE_LABEL_THRESHOLD = 100 * 1024 * 1024; // 104857600

const truncated25of35: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  // Content shape does not matter for banner/button assertions; the production
  // code reads only capBytes + totalBytes for the banner/button copy.
  content: 'A',
  totalBytes: 36615540, // ~35 MB
  truncated: true,
  capBytes: 26214400, // 25 MiB (post-cap-bump)
};

const truncated50MB: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  content: 'A',
  totalBytes: 50 * 1024 * 1024,
  truncated: true,
  capBytes: 26214400,
};

const truncated487MB: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  content: 'A',
  totalBytes: 487 * 1024 * 1024,
  truncated: true,
  capBytes: 26214400,
};

const truncatedJustBelowThreshold: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  content: 'A',
  totalBytes: SIZE_LABEL_THRESHOLD - 1, // 104857599
  truncated: true,
  capBytes: 26214400,
};

const truncatedAtThreshold: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  content: 'A',
  totalBytes: SIZE_LABEL_THRESHOLD, // 104857600
  truncated: true,
  capBytes: 26214400,
};

const fullPayloadFromTabA: PlainTextDocumentFixture = {
  tabId: 'tab-A',
  content: 'A'.repeat(64),
  totalBytes: 36615540,
  truncated: false,
  capBytes: 0,
};

/** Capture-dispatch helper. Renders a sibling that exposes the live dispatch. */
function Wrapper({ children }: { children: ReactNode }) {
  return <AppProvider>{children}</AppProvider>;
}

/**
 * Renders the current state.documentError into the DOM so tests can observe
 * SET_DOCUMENT_ERROR dispatches by reading rendered text. This is the only
 * reliable way to verify that the production PlainTextView's useAppDispatch
 * call actually fed the reducer -- spying on a sibling's dispatch handle does
 * NOT intercept the production-side useAppDispatch call.
 */
function DocumentErrorProbe() {
  const state = useAppState();
  return (
    <div data-testid="document-error-probe">{state.documentError ?? ''}</div>
  );
}

// ---------------------------------------------------------------------------
// 9.12-UNIT-121 [P0] AC#1 + AC#2: banner copy uses integer-MB formatBytes.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-121: banner copy uses formatBytes (integer-MB)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated25of35);
  });

  test('banner reads exactly "Showing first 25 MB of 35 MB."', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-truncated-banner')).toBeInTheDocument();
    });
    const banner = screen.getByTestId('plain-text-truncated-banner');
    expect(banner.textContent).toContain('Showing first 25 MB of 35 MB.');
    // AC2 explicit: the literal word "truncated" MUST NOT appear.
    expect(banner.textContent).not.toContain('truncated');
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-122 [P0] AC#1: banner layout is flex justify-between items-center.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-122: banner uses flex justify-between layout', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated25of35);
  });

  test('banner root carries flex, justify-between, items-center classes', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-truncated-banner')).toBeInTheDocument();
    });
    const banner = screen.getByTestId('plain-text-truncated-banner');
    expect(banner.className).toContain('flex');
    expect(banner.className).toContain('justify-between');
    expect(banner.className).toContain('items-center');
    // The existing token classes from 9-11 must still be present.
    expect(banner.className).toContain('text-warning');
    expect(banner.className).toContain('bg-surface-hover');
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-123 [P0] AC#3: button label is "Load all" under threshold.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-123: button label "Load all" under threshold', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated50MB);
  });

  test('totalBytes=50 MiB renders neutral label "Load all"', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const button = screen.getByTestId('plain-text-load-full-button');
    expect(button.textContent).toBe('Load all');
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-124 [P0] AC#4: button label gains size suffix at/above threshold.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-124: button label "Load all (487 MB)" above threshold', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated487MB);
  });

  test('totalBytes=487 MiB renders "Load all (487 MB)" (no decimal)', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const button = screen.getByTestId('plain-text-load-full-button');
    expect(button.textContent).toBe('Load all (487 MB)');
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-125 [P0] AC#3 + AC#4: SIZE_LABEL_THRESHOLD boundary.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-125: threshold boundary (104857599 vs 104857600)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('totalBytes = THRESHOLD-1 -> neutral label + border-border', async () => {
    mockGetPlainText.mockResolvedValue(truncatedJustBelowThreshold);
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const button = screen.getByTestId('plain-text-load-full-button');
    expect(button.textContent).toBe('Load all');
    expect(button.className).toContain('border-border');
    expect(button.className).not.toContain('border-warning');
  });

  test('totalBytes = THRESHOLD -> "Load all (100 MB)" + border-warning', async () => {
    mockGetPlainText.mockResolvedValue(truncatedAtThreshold);
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const button = screen.getByTestId('plain-text-load-full-button');
    expect(button.textContent).toBe('Load all (100 MB)');
    expect(button.className).toContain('border-warning');
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-126 [P0] AC#4 + AC#5: button color tokens flip under/above.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-126: button color tokens flip across threshold', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('under threshold carries text-text-primary + border-border', async () => {
    mockGetPlainText.mockResolvedValue(truncated50MB);
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const button = screen.getByTestId('plain-text-load-full-button');
    expect(button.className).toContain('text-text-primary');
    expect(button.className).toContain('border-border');
    expect(button.className).not.toContain('text-warning');
    expect(button.className).not.toContain('border-warning');
  });

  test('above threshold carries text-warning + border-warning', async () => {
    mockGetPlainText.mockResolvedValue(truncated487MB);
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const button = screen.getByTestId('plain-text-load-full-button');
    expect(button.className).toContain('text-warning');
    expect(button.className).toContain('border-warning');
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-127 [P0] AC#6: click invokes GetPlainTextFull with the tabId.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-127: click invokes GetPlainTextFull(tabId)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated25of35);
  });

  test('clicking "Load all" calls GetPlainTextFull exactly once with the tabId', async () => {
    // Never-resolving promise so the loading state sticks and we can still
    // assert the call.
    mockGetPlainTextFull.mockReturnValueOnce(new Promise(() => {}));
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const button = screen.getByTestId('plain-text-load-full-button');
    act(() => {
      fireEvent.click(button);
    });
    expect(mockGetPlainTextFull).toHaveBeenCalledTimes(1);
    expect(mockGetPlainTextFull).toHaveBeenCalledWith('tab-1');
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-128 [P0] AC#6: loading state is disabled + aria-busy + "Loading..."
// ---------------------------------------------------------------------------

describe('9.12-UNIT-128: loading state attributes', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated25of35);
  });

  test('button while in flight: disabled, aria-busy="true", text "Loading..."', async () => {
    mockGetPlainTextFull.mockReturnValueOnce(new Promise(() => {}));
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const button = screen.getByTestId('plain-text-load-full-button') as HTMLButtonElement;
    act(() => {
      fireEvent.click(button);
    });
    const reread = screen.getByTestId('plain-text-load-full-button') as HTMLButtonElement;
    expect(reread.disabled).toBe(true);
    expect(reread.getAttribute('aria-busy')).toBe('true');
    expect(reread.textContent).toBe('Loading...');
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-129 [P0] AC#6 re-entrancy guard: double-click fires once.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-129: re-entrancy guard (fullInFlightRef)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated25of35);
  });

  test('two clicks in quick succession invoke GetPlainTextFull once', async () => {
    mockGetPlainTextFull.mockReturnValueOnce(new Promise(() => {}));
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const button = screen.getByTestId('plain-text-load-full-button');
    act(() => {
      fireEvent.click(button);
      fireEvent.click(button);
    });
    expect(mockGetPlainTextFull).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-130 [P0] AC#7: successful resolve unmounts the banner.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-130: success path unmounts the banner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated25of35);
  });

  test('resolved GetPlainTextFull with truncated=false removes the banner', async () => {
    mockGetPlainTextFull.mockResolvedValueOnce(fullPayloadFromTabA);
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const button = screen.getByTestId('plain-text-load-full-button');
    await act(async () => {
      fireEvent.click(button);
    });
    await waitFor(() => {
      expect(screen.queryByTestId('plain-text-truncated-banner')).not.toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-131 [P0] AC#8: error path swaps button to Retry + dispatches
// SET_DOCUMENT_ERROR; clicking Retry re-invokes.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-131: error path swaps to Retry + dispatches SET_DOCUMENT_ERROR', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated25of35);
  });

  test('rejected fetch keeps the banner, swaps button to Retry, dispatches SET_DOCUMENT_ERROR; click Retry re-invokes', async () => {
    mockGetPlainTextFull.mockRejectedValueOnce(new Error('boom: cannot read full payload'));
    render(
      <Wrapper>
        <DocumentErrorProbe />
        <PlainTextView tabId="tab-1" active={true} />
      </Wrapper>,
    );
    // Wait for the Load-all button to render.
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });

    // documentError starts empty.
    expect(screen.getByTestId('document-error-probe').textContent).toBe('');

    const initialBtn = screen.getByTestId('plain-text-load-full-button');
    await act(async () => {
      fireEvent.click(initialBtn);
    });

    // Banner is still rendered (AC8: data unchanged on reject).
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-retry')).toBeInTheDocument();
    });
    expect(screen.getByTestId('plain-text-truncated-banner')).toBeInTheDocument();
    const retry = screen.getByTestId('plain-text-load-full-retry');
    expect(retry.textContent).toBe('Retry');

    // AC8: the production code must have dispatched SET_DOCUMENT_ERROR with
    // the extracted error message. We observe via state.documentError because
    // useAppDispatch() inside the component is called against the live
    // AppProvider context -- a sibling DispatchCapture's wrapped handle is
    // NOT on that path, so a spy on a sibling can't intercept the real call.
    await waitFor(() => {
      expect(screen.getByTestId('document-error-probe').textContent).toBe(
        'boom: cannot read full payload',
      );
    });

    // Second click re-invokes the same fetch flow.
    mockGetPlainTextFull.mockReturnValueOnce(new Promise(() => {}));
    await act(async () => {
      fireEvent.click(retry);
    });
    expect(mockGetPlainTextFull).toHaveBeenCalledTimes(2);
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-132 [P0] AC#8: Retry-in-flight keeps testid + shows "Loading...".
// ---------------------------------------------------------------------------

describe('9.12-UNIT-132: Retry-in-flight keeps retry testid + loading label', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated25of35);
  });

  test('after first failure, clicking Retry against a never-resolving promise leaves testid="plain-text-load-full-retry" + disabled + aria-busy + "Loading..."', async () => {
    mockGetPlainTextFull.mockRejectedValueOnce(new Error('first fail'));
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    await act(async () => {
      fireEvent.click(screen.getByTestId('plain-text-load-full-button'));
    });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-retry')).toBeInTheDocument();
    });
    mockGetPlainTextFull.mockReturnValueOnce(new Promise(() => {}));
    await act(async () => {
      fireEvent.click(screen.getByTestId('plain-text-load-full-retry'));
    });
    const stillRetry = screen.getByTestId('plain-text-load-full-retry') as HTMLButtonElement;
    expect(stillRetry.disabled).toBe(true);
    expect(stillRetry.getAttribute('aria-busy')).toBe('true');
    expect(stillRetry.textContent).toBe('Loading...');
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-133 [P0] AC#9: stale-fetch guard (RESOLVE path).
// ---------------------------------------------------------------------------

describe('9.12-UNIT-133: stale-fetch guard (resolve path)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  test('resolving a doc-A fetch after switching to doc-B does NOT update doc-B state', async () => {
    // Initial truncated fetch for tab-A.
    mockGetPlainText.mockResolvedValueOnce({ ...truncated25of35, tabId: 'tab-A' });
    const consoleErrSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    // Click on tab-A creates an in-flight GetPlainTextFull that we control via
    // deferred resolve.
    let resolveFull: ((v: PlainTextDocumentFixture) => void) | null = null;
    mockGetPlainTextFull.mockReturnValueOnce(
      new Promise<PlainTextDocumentFixture>((r) => {
        resolveFull = r;
      }),
    );

    const { rerender } = render(
      <PlainTextView tabId="tab-A" active={true} />,
      { wrapper: Wrapper },
    );
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    await act(async () => {
      fireEvent.click(screen.getByTestId('plain-text-load-full-button'));
    });

    // Now the user switches the document tab to tab-B. The reset effect kicks
    // in and re-fetches truncated for B.
    const truncatedTabB: PlainTextDocumentFixture = {
      tabId: 'tab-B',
      content: 'B',
      totalBytes: 36615540,
      truncated: true,
      capBytes: 26214400,
    };
    mockGetPlainText.mockResolvedValueOnce(truncatedTabB);
    rerender(<PlainTextView tabId="tab-B" active={true} />);
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-truncated-banner')).toBeInTheDocument();
    });

    // The original tab-A fetch resolves AFTER the tabId swap.
    await act(async () => {
      resolveFull!(fullPayloadFromTabA);
    });

    // The banner for tab-B is still present (the stale resolve did NOT call
    // setData against tab-B).
    expect(screen.getByTestId('plain-text-truncated-banner')).toBeInTheDocument();
    // The button is back to neutral "Load all" (loadingFull was reset on
    // tabId change).
    const buttonForB = screen.getByTestId('plain-text-load-full-button') as HTMLButtonElement;
    expect(buttonForB.disabled).toBe(false);
    expect(buttonForB.textContent).toBe('Load all');
    // No React act() warnings on unmounted-state writes.
    expect(consoleErrSpy).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-135 [P1] AC#5: shared base styling tokens on the action button.
// Trace step 9/10 flagged AC5 as PARTIAL -- the base classes (bg-bg, border,
// rounded, px-3 py-1 text-sm, hover:bg-surface-hover, cursor-pointer,
// disabled:cursor-not-allowed, disabled:opacity-60) were only implicitly
// asserted. Pin them explicitly so a refactor that drops one fails loud.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-135: shared base styling tokens on action button', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated50MB);
  });

  test('button carries all AC5 base classes', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    const cls = screen.getByTestId('plain-text-load-full-button').className;
    for (const token of [
      'bg-bg',
      'border',
      'rounded',
      'px-3',
      'py-1',
      'text-sm',
      'hover:bg-surface-hover',
      'cursor-pointer',
      'disabled:cursor-not-allowed',
      'disabled:opacity-60',
    ]) {
      expect(cls).toContain(token);
    }
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-136 [P0] AC#10: inner-tab persistence -- after Load all resolves,
// toggling the DetailPanel inner tab off and back (simulated via active=true
// -> false -> true) does NOT trigger a re-fetch. The full payload survives
// from in-component state. Structurally guarded by 9-11's forceMount test;
// this asserts the cache-after-load-all behavior directly.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-136: inner-tab cache persists after Load all', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(truncated25of35);
    mockGetPlainTextFull.mockResolvedValue(fullPayloadFromTabA);
  });

  test('Load all -> active=false -> active=true does NOT re-invoke GetPlainTextFull or GetPlainText', async () => {
    const { rerender } = render(
      <PlainTextView tabId="tab-1" active={true} />,
      { wrapper: Wrapper },
    );
    // Wait for the truncated fetch + button.
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    expect(mockGetPlainText).toHaveBeenCalledTimes(1);

    // Click Load all and wait for the banner to unmount (success).
    await act(async () => {
      fireEvent.click(screen.getByTestId('plain-text-load-full-button'));
    });
    await waitFor(() => {
      expect(screen.queryByTestId('plain-text-truncated-banner')).not.toBeInTheDocument();
    });
    expect(mockGetPlainTextFull).toHaveBeenCalledTimes(1);

    // Toggle inner tab off and back. Same tabId, no unmount (forceMount).
    rerender(<PlainTextView tabId="tab-1" active={false} />);
    rerender(<PlainTextView tabId="tab-1" active={true} />);

    // Neither binding was called again -- the in-component data survived.
    expect(mockGetPlainText).toHaveBeenCalledTimes(1);
    expect(mockGetPlainTextFull).toHaveBeenCalledTimes(1);
    // Banner stays unmounted; full content still rendered.
    expect(screen.queryByTestId('plain-text-truncated-banner')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 9.12-UNIT-134 [P0] AC#9: stale-fetch guard (REJECT path).
// A stale rejection must NOT dispatch SET_DOCUMENT_ERROR against doc-B.
// ---------------------------------------------------------------------------

describe('9.12-UNIT-134: stale-fetch guard (reject path)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  test('rejecting a doc-A fetch after switching to doc-B does NOT dispatch SET_DOCUMENT_ERROR and does NOT surface Retry on doc-B', async () => {
    mockGetPlainText.mockResolvedValueOnce({ ...truncated25of35, tabId: 'tab-A' });
    const consoleErrSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    let rejectFull: ((e: Error) => void) | null = null;
    mockGetPlainTextFull.mockReturnValueOnce(
      new Promise<PlainTextDocumentFixture>((_, reject) => {
        rejectFull = reject;
      }),
    );

    // We observe SET_DOCUMENT_ERROR dispatches via state.documentError through
    // the DocumentErrorProbe. The production PlainTextView calls
    // useAppDispatch() against the live AppProvider context, so a sibling's
    // wrapped dispatch CANNOT intercept that path. The probe reads state via
    // useAppState(), which DOES sit on the production path: if AC9's reject
    // branch leaks an SET_DOCUMENT_ERROR dispatch, the probe will surface the
    // message; if the stale-guard short-circuits correctly, the probe stays
    // empty.

    const { rerender } = render(
      <Wrapper>
        <DocumentErrorProbe />
        <PlainTextView tabId="tab-A" active={true} />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    });
    expect(screen.getByTestId('document-error-probe').textContent).toBe('');
    await act(async () => {
      fireEvent.click(screen.getByTestId('plain-text-load-full-button'));
    });

    // Switch to tab-B and let the truncated fetch resolve.
    const truncatedTabB: PlainTextDocumentFixture = {
      tabId: 'tab-B',
      content: 'B',
      totalBytes: 36615540,
      truncated: true,
      capBytes: 26214400,
    };
    mockGetPlainText.mockResolvedValueOnce(truncatedTabB);
    rerender(
      <Wrapper>
        <DocumentErrorProbe />
        <PlainTextView tabId="tab-B" active={true} />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-truncated-banner')).toBeInTheDocument();
    });

    // The original tab-A fetch rejects AFTER the tabId swap.
    await act(async () => {
      rejectFull!(new Error('stale doc-A failure'));
    });

    // Doc-B must not surface the Retry variant -- the stale rejection branch
    // must early-return before mutating loadFullErrored.
    expect(screen.queryByTestId('plain-text-load-full-retry')).not.toBeInTheDocument();
    // Doc-B's button is still the neutral Load-all (truncated label).
    expect(screen.getByTestId('plain-text-load-full-button')).toBeInTheDocument();
    // AC9 reject-path contract: no SET_DOCUMENT_ERROR was dispatched, so the
    // global documentError slot stays empty.
    expect(screen.getByTestId('document-error-probe').textContent).toBe('');
    expect(consoleErrSpy).not.toHaveBeenCalled();
  });
});
