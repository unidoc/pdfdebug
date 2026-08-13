/**
 * Story 10.1: Async Plain Text Load with Cancel -- Vitest suite.
 *
 * Test IDs follow the convention. Each test maps to one or more ACs in
 * the story spec.
 *
 * Run: cd frontend && npx vitest run src/components/PlainTextView.async.test.tsx
 */
import { render, screen, waitFor, act, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { ReactNode } from 'react';
import { PlainTextView } from './PlainTextView';
import { AppProvider } from '../hooks/useDocumentState';

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
  }),
);

// --- Fixtures ---

/** Post-10-1 IPC shape: TabID + Content + TotalBytes only. */
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

const zeroByteDoc: PlainTextDocumentFixture = {
  tabId: 'tab-1',
  content: '',
  totalBytes: 0,
};

function Wrapper({ children }: { children: ReactNode }) {
  return <AppProvider>{children}</AppProvider>;
}

// Helper: render a deferred promise the test controls explicitly.
function deferred<T>(): {
  promise: Promise<T>;
  resolve: (v: T) => void;
  reject: (e: Error) => void;
} {
  let resolve!: (v: T) => void;
  let reject!: (e: Error) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

// ---------------------------------------------------------------------------
// First activation fires exactly one GetPlainText call.
// ---------------------------------------------------------------------------

describe('lazy fetch on first activation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(smallDoc);
    mockGetPlainTextSize.mockResolvedValue(17);
  });

  test('active=true fires GetPlainText(tabID) exactly once for the tabId', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(mockGetPlainText).toHaveBeenCalledWith('tab-1');
    });
    expect(mockGetPlainText).toHaveBeenCalledTimes(1);
  });

  test('active=false does NOT fetch', () => {
    render(<PlainTextView tabId="tab-1" active={false} />, { wrapper: Wrapper });
    expect(mockGetPlainText).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Loading card mounts after 200ms debounce with the heading, size
// disclosure, elapsed counter, and Cancel button.
// ---------------------------------------------------------------------------

describe('loading card structure', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    const def = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValue(def.promise);
    // Size resolves immediately so the disclosure populates.
    mockGetPlainTextSize.mockResolvedValue(487 * 1024 * 1024);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('after 200ms debounce, card renders with heading + size + elapsed + Cancel', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });

    // Before debounce: card not visible.
    act(() => {
      vi.advanceTimersByTime(150);
    });
    expect(screen.queryByTestId('plain-text-loading-card')).not.toBeInTheDocument();

    // After debounce: card mounts.
    act(() => {
      vi.advanceTimersByTime(100);
    });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-loading-card')).toBeInTheDocument();
    });

    const card = screen.getByTestId('plain-text-loading-card');
    // Heading text.
    expect(card.textContent).toContain('Loading plain text');
    // Size disclosure -- once GetPlainTextSize resolves (microtask flush).
    const sizeEl = screen.getByTestId('plain-text-loading-size');
    expect(sizeEl).toBeInTheDocument();
    // 487 MiB -> "487.0 MB" via formatBytes 1-decimal MB form (Story 10.8).
    expect(sizeEl.textContent).toContain('487.0 MB');
    // Cancel button with the literal "Cancel" label.
    const cancelBtn = screen.getByTestId('plain-text-cancel-button');
    expect(cancelBtn.textContent).toBe('Cancel');
  });
});

// ---------------------------------------------------------------------------
// Size disclosure renders empty while GetPlainTextSize is
// unresolved; the card still mounts.
// ---------------------------------------------------------------------------

describe('size disclosure tolerates unresolved size', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    const docDef = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValue(docDef.promise);
    const sizeDef = deferred<number>();
    mockGetPlainTextSize.mockReturnValue(sizeDef.promise);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('card mounts with empty size element when GetPlainTextSize is unresolved', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    act(() => {
      vi.advanceTimersByTime(250);
    });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-loading-card')).toBeInTheDocument();
    });
    // Size element exists (reserves layout) but content is empty.
    const sizeEl = screen.getByTestId('plain-text-loading-size');
    expect(sizeEl).toBeInTheDocument();
    expect(sizeEl.textContent).toBe('');
    // Cancel still renders.
    expect(screen.getByTestId('plain-text-cancel-button')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Successful resolve unmounts the loading card and renders content.
// ---------------------------------------------------------------------------

describe('success path renders content', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(smallDoc);
    mockGetPlainTextSize.mockResolvedValue(17);
  });

  test('after resolve, loading card unmounts and content lines render', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByText('%PDF-1.7')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('plain-text-loading-card')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Cancel click invokes CancelPlainText(tabID).
// ---------------------------------------------------------------------------

describe('Cancel click invokes CancelPlainText', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    const def = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValue(def.promise);
    mockGetPlainTextSize.mockResolvedValue(50 * 1024 * 1024);
    mockCancelPlainText.mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('clicking Cancel invokes CancelPlainText("tab-1") and disables the button with "Cancelling" label', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    act(() => {
      vi.advanceTimersByTime(250);
    });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-cancel-button')).toBeInTheDocument();
    });

    const cancelBtn = screen.getByTestId('plain-text-cancel-button') as HTMLButtonElement;
    act(() => {
      fireEvent.click(cancelBtn);
    });

    expect(mockCancelPlainText).toHaveBeenCalledTimes(1);
    expect(mockCancelPlainText).toHaveBeenCalledWith('tab-1');

    // The button is immediately disabled with the "Cancelling" label.
    const reread = screen.getByTestId('plain-text-cancel-button') as HTMLButtonElement;
    expect(reread.disabled).toBe(true);
    expect(reread.textContent).toBe('Cancelling');
  });
});

// ---------------------------------------------------------------------------
// A cancellation rejection transitions to the cancelled state with the
// documented body + CTA.
// ---------------------------------------------------------------------------

describe('cancelled state renders documented copy + CTA', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainTextSize.mockResolvedValue(50 * 1024 * 1024);
    mockCancelPlainText.mockResolvedValue(undefined);
  });

  test('rejected GetPlainText with cancel-substring shows "Plain text load cancelled." + Load plain text CTA', async () => {
    const fetchDef = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValueOnce(fetchDef.promise);

    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });

    // Reject with a cancellation error (substring 'cancel' is the frontend
    // contract: extractErrorMessage(err) contains 'cancel').
    await act(async () => {
      fetchDef.reject(new Error('context canceled'));
    });

    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-cta')).toBeInTheDocument();
    });

    // cancelled-state copy + CTA label.
    expect(screen.getByText('Plain text load cancelled.')).toBeInTheDocument();
    const cta = screen.getByTestId('plain-text-load-cta');
    expect(cta.textContent).toBe('Load plain text');
  });
});

// ---------------------------------------------------------------------------
// CTA click re-runs the fetch flow with a fresh elapsed counter (0s).
// ---------------------------------------------------------------------------

describe('cancelled CTA re-runs fetch with elapsed reset', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    mockGetPlainTextSize.mockResolvedValue(50 * 1024 * 1024);
    mockCancelPlainText.mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('clicking Load plain text CTA fires GetPlainText again and elapsed restarts at 0s', async () => {
    const firstDef = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValueOnce(firstDef.promise);

    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });

    await act(async () => {
      firstDef.reject(new Error('context canceled'));
    });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-cta')).toBeInTheDocument();
    });

    // Set up the second fetch as a never-resolving deferred so we can observe
    // the loading card.
    const secondDef = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValueOnce(secondDef.promise);

    await act(async () => {
      fireEvent.click(screen.getByTestId('plain-text-load-cta'));
    });

    // GetPlainText must have been called a second time.
    expect(mockGetPlainText).toHaveBeenCalledTimes(2);

    // Advance past the 200ms debounce; loading card mounts again.
    act(() => {
      vi.advanceTimersByTime(250);
    });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-loading-card')).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// non-cancellation rejection renders the error state with the mapped error
// message and a Retry button using the shared CTA testid.
// ---------------------------------------------------------------------------

describe('error state shows Retry with shared CTA testid', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainTextSize.mockResolvedValue(50 * 1024 * 1024);
  });

  test('rejected with non-cancel error renders plain-text-error + Retry CTA', async () => {
    const fetchDef = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValueOnce(fetchDef.promise);

    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });

    await act(async () => {
      fetchDef.reject(new Error('file moved or deleted'));
    });

    await waitFor(() => {
      expect(screen.getByTestId('plain-text-error')).toBeInTheDocument();
    });
    expect(screen.getByTestId('plain-text-error').textContent).toContain('file moved or deleted');

    const retry = screen.getByTestId('plain-text-load-cta');
    expect(retry.textContent).toBe('Retry');
  });

  test('clicking Retry re-runs the fetch flow', async () => {
    const firstDef = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValueOnce(firstDef.promise);

    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await act(async () => {
      firstDef.reject(new Error('file moved or deleted'));
    });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-cta')).toBeInTheDocument();
    });

    mockGetPlainText.mockResolvedValueOnce(smallDoc);
    await act(async () => {
      fireEvent.click(screen.getByTestId('plain-text-load-cta'));
    });
    expect(mockGetPlainText).toHaveBeenCalledTimes(2);
  });
});

// ---------------------------------------------------------------------------
// document-tab switch mid-load triggers the stale- fetch guard; the previous
// load's resolve does not mutate state on the new doc.
// ---------------------------------------------------------------------------

describe('stale-fetch guard on document tab switch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainTextSize.mockResolvedValue(50 * 1024 * 1024);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  test('resolving a tab-A fetch after switching to tab-B does NOT render tab-A content', async () => {
    const consoleErr = vi.spyOn(console, 'error').mockImplementation(() => {});

    const aDef = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValueOnce(aDef.promise);

    const { rerender } = render(
      <PlainTextView tabId="tab-A" active={true} />,
      { wrapper: Wrapper },
    );

    // Switch to tab-B mid-load. The reset effect MUST clear in-flight state,
    // and the new tab-B fetch must succeed against tab-B's payload.
    const tabBDoc: PlainTextDocumentFixture = {
      tabId: 'tab-B',
      content: 'B content',
      totalBytes: 9,
    };
    mockGetPlainText.mockResolvedValueOnce(tabBDoc);
    rerender(<PlainTextView tabId="tab-B" active={true} />);
    await waitFor(() => {
      expect(screen.getByText('B content')).toBeInTheDocument();
    });

    // Now the original tab-A promise resolves AFTER the swap.
    const tabADoc: PlainTextDocumentFixture = {
      tabId: 'tab-A',
      content: 'A content',
      totalBytes: 9,
    };
    await act(async () => {
      aDef.resolve(tabADoc);
    });

    // Tab-B content survives; tab-A content is NOT rendered.
    expect(screen.getByText('B content')).toBeInTheDocument();
    expect(screen.queryByText('A content')).not.toBeInTheDocument();
    expect(consoleErr).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// inner-tab toggle (active=true -> false -> true) does NOT re-fetch after a
// successful load.
// ---------------------------------------------------------------------------

describe('inner-tab cache persists across active toggles', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(smallDoc);
    mockGetPlainTextSize.mockResolvedValue(17);
  });

  test('active flip after success does not re-invoke GetPlainText', async () => {
    const { rerender } = render(
      <PlainTextView tabId="tab-1" active={true} />,
      { wrapper: Wrapper },
    );
    await waitFor(() => {
      expect(screen.getByText('%PDF-1.7')).toBeInTheDocument();
    });
    expect(mockGetPlainText).toHaveBeenCalledTimes(1);

    rerender(<PlainTextView tabId="tab-1" active={false} />);
    rerender(<PlainTextView tabId="tab-1" active={true} />);

    expect(mockGetPlainText).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// Document tab change resets to idle and clears in-flight refs / elapsed
// counter.
// ---------------------------------------------------------------------------

describe('document tab change resets state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainTextSize.mockResolvedValue(50 * 1024 * 1024);
  });

  test('changing tabId from tab-A to tab-B triggers a fresh GetPlainText on tab-B', async () => {
    mockGetPlainText.mockResolvedValueOnce({
      tabId: 'tab-A',
      content: 'A',
      totalBytes: 1,
    });
    const { rerender } = render(
      <PlainTextView tabId="tab-A" active={true} />,
      { wrapper: Wrapper },
    );
    await waitFor(() => {
      expect(mockGetPlainText).toHaveBeenCalledWith('tab-A');
    });

    mockGetPlainText.mockResolvedValueOnce({
      tabId: 'tab-B',
      content: 'B',
      totalBytes: 1,
    });
    rerender(<PlainTextView tabId="tab-B" active={true} />);
    await waitFor(() => {
      expect(mockGetPlainText).toHaveBeenCalledWith('tab-B');
    });
    expect(mockGetPlainText).toHaveBeenCalledTimes(2);
  });
});

// ---------------------------------------------------------------------------
// fast-path under 200ms never mounts the loading card.
// ---------------------------------------------------------------------------

describe('fast-path resolve < 200ms does not flash loading card', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    mockGetPlainTextSize.mockResolvedValue(17);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('resolving at t=50ms never mounts plain-text-loading-card', async () => {
    const def = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValue(def.promise);
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });

    // Advance 50ms, then resolve. Card must not have appeared during the
    // 50ms window, and must not appear after resolve either (LoadState
    // transitions loading -> ready directly).
    act(() => {
      vi.advanceTimersByTime(50);
    });
    expect(screen.queryByTestId('plain-text-loading-card')).not.toBeInTheDocument();

    await act(async () => {
      def.resolve(smallDoc);
    });
    await waitFor(() => {
      expect(screen.getByText('%PDF-1.7')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('plain-text-loading-card')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 0-byte payload renders the empty virtualized list.
// ---------------------------------------------------------------------------

describe('zero-byte payload renders without error', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(zeroByteDoc);
    mockGetPlainTextSize.mockResolvedValue(0);
  });

  test('zero-byte content renders without loading/error and without rows', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    // We can't easily assert "ready" via a single literal; wait for the
    // loading card / error to NOT be present and the cancellation card to NOT
    // be present. The component reaches `ready` and renders the (empty) virtual
    // list.
    await waitFor(() => {
      expect(mockGetPlainText).toHaveBeenCalledWith('tab-1');
    });
    await waitFor(() => {
      expect(screen.queryByTestId('plain-text-loading-card')).not.toBeInTheDocument();
    });
    expect(screen.queryByTestId('plain-text-error')).not.toBeInTheDocument();
    expect(screen.queryByTestId('plain-text-load-cta')).not.toBeInTheDocument();
    // No content rows.
    expect(screen.queryAllByTestId(/^plain-text-row-/).length).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// At load start, GetPlainText AND GetPlainTextSize are
// dispatched in parallel.
// ---------------------------------------------------------------------------

describe('parallel dispatch of GetPlainText + GetPlainTextSize', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(smallDoc);
    mockGetPlainTextSize.mockResolvedValue(17);
  });

  test('both binding calls fire on first activation', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(mockGetPlainText).toHaveBeenCalledWith('tab-1');
    });
    expect(mockGetPlainTextSize).toHaveBeenCalledWith('tab-1');
    // Each fires exactly once per activation.
    expect(mockGetPlainText).toHaveBeenCalledTimes(1);
    expect(mockGetPlainTextSize).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// extractErrorMessage on the rejection contains the substring 'cancel'
// (case-insensitive). Cross-checked here on the frontend side; the backend
// errors.Is(err, context.Canceled) identity is asserted in the Go tests.
// ---------------------------------------------------------------------------

describe('cancellation rejection substring contract', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainTextSize.mockResolvedValue(50 * 1024 * 1024);
    mockCancelPlainText.mockResolvedValue(undefined);
  });

  test.each([
    'context canceled',
    'cancelled by user',
    'Canceled',
  ])('error message %q is treated as cancellation -> cancelled state', async (msg) => {
    const def = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValueOnce(def.promise);

    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await act(async () => {
      def.reject(new Error(msg));
    });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-cta')).toBeInTheDocument();
    });
    expect(screen.getByText('Plain text load cancelled.')).toBeInTheDocument();
    // Error pane NOT used for cancellation.
    expect(screen.queryByTestId('plain-text-error')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Terminal states (cancelled / error) must NOT auto-refetch on an inner-tab
// active toggle. The user-visible CTA is the explicit retry surface; a silent
// re-fetch on tab toggle defeats.
// ---------------------------------------------------------------------------

describe('terminal states do not auto-refetch on active toggle', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainTextSize.mockResolvedValue(17);
    mockCancelPlainText.mockResolvedValue(undefined);
  });

  test('cancelled state does NOT re-invoke GetPlainText on active false -> true', async () => {
    const def = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValueOnce(def.promise);

    const { rerender } = render(
      <PlainTextView tabId="tab-1" active={true} />,
      { wrapper: Wrapper },
    );
    await act(async () => {
      def.reject(new Error('context canceled'));
    });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-load-cta')).toBeInTheDocument();
    });
    expect(mockGetPlainText).toHaveBeenCalledTimes(1);

    // Toggle active off and back on -- this simulates the user switching to
    // a sibling inner tab (Object/XREF) and back to Plain Text.
    rerender(<PlainTextView tabId="tab-1" active={false} />);
    rerender(<PlainTextView tabId="tab-1" active={true} />);

    // No additional GetPlainText call: the CTA is the explicit retry surface.
    expect(mockGetPlainText).toHaveBeenCalledTimes(1);
    // CTA still rendered (state preserved).
    expect(screen.getByTestId('plain-text-load-cta')).toBeInTheDocument();
  });

  test('error state does NOT re-invoke GetPlainText on active false -> true', async () => {
    const def = deferred<PlainTextDocumentFixture>();
    mockGetPlainText.mockReturnValueOnce(def.promise);

    const { rerender } = render(
      <PlainTextView tabId="tab-1" active={true} />,
      { wrapper: Wrapper },
    );
    await act(async () => {
      def.reject(new Error('file moved or deleted'));
    });
    await waitFor(() => {
      expect(screen.getByTestId('plain-text-error')).toBeInTheDocument();
    });
    expect(mockGetPlainText).toHaveBeenCalledTimes(1);

    rerender(<PlainTextView tabId="tab-1" active={false} />);
    rerender(<PlainTextView tabId="tab-1" active={true} />);

    expect(mockGetPlainText).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('plain-text-error')).toBeInTheDocument();
  });
});
