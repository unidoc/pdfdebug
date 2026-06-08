/**
 * Story 10.2: Find Bar in Plain Text View -- integration red-phase suite.
 *
 * TDD RED PHASE: every test below fails until Story 10-2 is implemented
 * (PlainTextView wires useFindBar + FindBar, per-row <mark> overlay,
 * gutter density markers, auto-scroll on active match change).
 *
 * Scope:
 * - AC1 mount: Cmd+F + active=true + data ready -> FindBar appears.
 * - AC5 per-row <mark>: matched substrings render as <mark> spans with the
 *   active match tagged data-testid="plain-text-find-active-match" and the
 *   others data-testid="plain-text-find-match".
 * - AC5 row textContent invariant: each row's textContent stays byte-identical
 *   to the raw line.
 * - AC6 gutter density markers: lines with matches get
 *   data-testid="plain-text-find-gutter-marker-{lineNo}".
 * - AC7 auto-scroll on next/prev: the scroll container's scrollTop is
 *   adjusted to bring the active match's line into view.
 * - AC11 inner-tab persistence: query + matches survive an active=false ->
 *   active=true toggle on the same tabId.
 * - AC11 document-tab reset: query clears + bar closes when tabId changes.
 * - AC13 Cmd+F gate on data===null: keystroke is consumed (preventDefault)
 *   but bar does not open.
 * - AC22 Esc scope: a sibling window-level keydown listener does NOT fire
 *   when Esc closes the FindBar (the handler is scoped to the FindBar root).
 *
 * Test IDs follow the 10-2-INTG-NNN convention.
 *
 * Run: cd frontend && npx vitest run src/components/PlainTextView.find.test.tsx
 */
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { ReactNode } from 'react';
import { PlainTextView } from './PlainTextView';
import { AppProvider } from '../hooks/useDocumentState';

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

function Wrapper({ children }: { children: ReactNode }) {
  return <AppProvider>{children}</AppProvider>;
}

// Force the Cmd-key platform under jsdom so the Cmd+F keystroke binds.
function forceMacPlatform() {
  const original = Object.getOwnPropertyDescriptor(window.navigator, 'platform');
  Object.defineProperty(window.navigator, 'platform', {
    configurable: true,
    get: () => 'MacIntel',
  });
  return () => {
    if (original) {
      Object.defineProperty(window.navigator, 'platform', original);
    }
  };
}

function dispatchKey(opts: {
  key: string;
  metaKey?: boolean;
  ctrlKey?: boolean;
  shiftKey?: boolean;
}) {
  const ev = new KeyboardEvent('keydown', {
    key: opts.key,
    metaKey: opts.metaKey ?? false,
    ctrlKey: opts.ctrlKey ?? false,
    shiftKey: opts.shiftKey ?? false,
    bubbles: true,
    cancelable: true,
  });
  window.dispatchEvent(ev);
  return ev;
}

// Fixtures shaped per the post-10-1 IPC contract.
const helveticaCorpus = {
  tabId: 'tab-1',
  content: 'first /Helvetica line\nsecond plain line\nthird /Helvetica again\nfourth /helvetica lowercase',
  totalBytes: 90,
};

const noMatchCorpus = {
  tabId: 'tab-1',
  content: 'first line\nsecond line\nthird line',
  totalBytes: 30,
};

// ---------------------------------------------------------------------------
// 10-2-INTG-001 [P0] AC#1: Cmd+F on active Plain Text tab + data ready
// mounts the FindBar.
// ---------------------------------------------------------------------------

describe('10-2-INTG-001: Cmd+F mounts the FindBar', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Cmd+F renders plain-text-find-bar', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.queryByText('first /Helvetica line')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('plain-text-find-bar')).toBeNull();
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    expect(screen.getByTestId('plain-text-find-bar')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-002 [P0] AC#5: per-row <mark> elements wrap matched substrings.
// The active match carries data-testid="plain-text-find-active-match"; the
// others carry data-testid="plain-text-find-match".
// ---------------------------------------------------------------------------

describe('10-2-INTG-002: per-row <mark> rendering', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('typing "Helvetica" produces marks across all matching lines', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.queryByText('first /Helvetica line')).toBeInTheDocument();
    });
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    const input = screen.getByTestId('plain-text-find-input') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'Helvetica' } });
    await waitFor(() => {
      // 3 lowercase-or-mixed-case matches under default case-insensitive flag.
      const active = screen.queryAllByTestId('plain-text-find-active-match');
      const inactive = screen.queryAllByTestId('plain-text-find-match');
      expect(active.length + inactive.length).toBeGreaterThanOrEqual(3);
      // Exactly one active match.
      expect(active.length).toBe(1);
    });
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-003 [P0] AC#5 row textContent invariant: each row's textContent
// stays byte-identical to the raw line.
// ---------------------------------------------------------------------------

describe('10-2-INTG-003: row textContent invariant', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('plain-text-row-1 textContent equals the raw first line', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.queryByTestId('plain-text-row-1')).toBeInTheDocument();
    });
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    const input = screen.getByTestId('plain-text-find-input') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'Helvetica' } });
    await waitFor(() => {
      expect(screen.getAllByTestId('plain-text-find-match').length).toBeGreaterThan(0);
    });
    const row1 = screen.getByTestId('plain-text-row-1');
    expect(row1.textContent).toBe('first /Helvetica line');
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-004 [P0] AC#6: gutter density markers on lines with matches.
// ---------------------------------------------------------------------------

describe('10-2-INTG-004: gutter density markers', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('lines 1, 3, 4 with /helvetica matches have gutter markers; line 2 does not', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.queryByText('first /Helvetica line')).toBeInTheDocument();
    });
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    const input = screen.getByTestId('plain-text-find-input') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'helvetica' } });
    await waitFor(() => {
      expect(screen.getAllByTestId('plain-text-find-match').length).toBeGreaterThan(0);
    });
    expect(screen.queryByTestId('plain-text-find-gutter-marker-1')).toBeInTheDocument();
    expect(screen.queryByTestId('plain-text-find-gutter-marker-2')).toBeNull();
    expect(screen.queryByTestId('plain-text-find-gutter-marker-3')).toBeInTheDocument();
    expect(screen.queryByTestId('plain-text-find-gutter-marker-4')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-005 [P0] AC#7: clicking Next advances the active match.
// ---------------------------------------------------------------------------

describe('10-2-INTG-005: Next button advances active match', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Next moves the active testid forward and updates the count', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.queryByText('first /Helvetica line')).toBeInTheDocument();
    });
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    const input = screen.getByTestId('plain-text-find-input') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'helvetica' } });
    await waitFor(() => {
      expect(screen.getAllByTestId('plain-text-find-match').length).toBeGreaterThan(0);
    });
    const initialCount = screen.getByTestId('plain-text-find-count').textContent ?? '';
    expect(initialCount.startsWith('1 of ')).toBe(true);
    fireEvent.click(screen.getByTestId('plain-text-find-next'));
    expect(screen.getByTestId('plain-text-find-count').textContent?.startsWith('2 of ')).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-006 [P0] AC#18: no-match query renders no marks and "0 of 0".
// ---------------------------------------------------------------------------

describe('10-2-INTG-006: no-match query', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(noMatchCorpus);
    mockGetPlainTextSize.mockResolvedValue(noMatchCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('typing a query that yields zero matches keeps count "0 of 0" and renders no marks', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.queryByText('first line')).toBeInTheDocument();
    });
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    const input = screen.getByTestId('plain-text-find-input') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'xyzzy' } });
    expect(screen.getByTestId('plain-text-find-count').textContent).toBe('0 of 0');
    expect(screen.queryAllByTestId('plain-text-find-match')).toHaveLength(0);
    expect(screen.queryAllByTestId('plain-text-find-active-match')).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-007 [P0] AC#11: inner-tab persistence -- toggling active=false ->
// active=true on the same tabId preserves query + matches (PlainTextView
// stays mounted; only the `active` prop toggles).
// ---------------------------------------------------------------------------

describe('10-2-INTG-007: inner-tab persistence', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('query + matches survive an active toggle on the same tabId', async () => {
    const { rerender } = render(
      <PlainTextView tabId="tab-1" active={true} />,
      { wrapper: Wrapper },
    );
    await waitFor(() => {
      expect(screen.queryByText('first /Helvetica line')).toBeInTheDocument();
    });
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    fireEvent.change(screen.getByTestId('plain-text-find-input'), {
      target: { value: 'Helvetica' },
    });
    await waitFor(() => {
      expect(screen.getAllByTestId('plain-text-find-match').length).toBeGreaterThan(0);
    });
    const initialMarks =
      screen.queryAllByTestId('plain-text-find-match').length +
      screen.queryAllByTestId('plain-text-find-active-match').length;

    // Switch inner tab away (component stays mounted, active=false).
    rerender(<PlainTextView tabId="tab-1" active={false} />);
    // Switch inner tab back.
    rerender(<PlainTextView tabId="tab-1" active={true} />);

    // Query + matches preserved (FindBar still open or reopens on the saved state).
    const input = screen.queryByTestId('plain-text-find-input') as HTMLInputElement | null;
    if (input) {
      expect(input.value).toBe('Helvetica');
    }
    const stillVisible =
      screen.queryAllByTestId('plain-text-find-match').length +
      screen.queryAllByTestId('plain-text-find-active-match').length;
    expect(stillVisible).toBe(initialMarks);
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-008 [P0] AC#11: document-tab reset -- changing tabId closes the
// bar and clears the query.
// ---------------------------------------------------------------------------

describe('10-2-INTG-008: document-tab reset on tabId change', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('changing tabId closes the bar', async () => {
    const { rerender } = render(
      <PlainTextView tabId="tab-1" active={true} />,
      { wrapper: Wrapper },
    );
    await waitFor(() => {
      expect(screen.queryByText('first /Helvetica line')).toBeInTheDocument();
    });
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    fireEvent.change(screen.getByTestId('plain-text-find-input'), {
      target: { value: 'Helvetica' },
    });
    expect(screen.queryByTestId('plain-text-find-bar')).toBeInTheDocument();

    // Backend returns the same content shape for tab-2 (the test asserts only
    // that the find bar closes on tabId change, not that data swaps).
    mockGetPlainText.mockResolvedValue({ ...helveticaCorpus, tabId: 'tab-2' });
    rerender(<PlainTextView tabId="tab-2" active={true} />);

    expect(screen.queryByTestId('plain-text-find-bar')).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-009 [P0] AC#13: Cmd+F on Plain Text inner tab with data===null
// (loadState !== 'ready') is consumed but does NOT mount the bar.
// ---------------------------------------------------------------------------

describe('10-2-INTG-009: Cmd+F gated on data!==null', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Cmd+F while load is in flight does not mount the bar; preventDefault is called', async () => {
    // Hang the load indefinitely so loadState stays 'loading'.
    mockGetPlainText.mockReturnValue(new Promise(() => {}));
    mockGetPlainTextSize.mockResolvedValue(1024);
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    // Trigger the loading state without resolving.
    await waitFor(() => {
      expect(mockGetPlainText).toHaveBeenCalledWith('tab-1');
    });
    let ev: KeyboardEvent | null = null;
    act(() => {
      ev = dispatchKey({ key: 'f', metaKey: true });
    });
    expect(screen.queryByTestId('plain-text-find-bar')).toBeNull();
    expect(ev!.defaultPrevented).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-011 [P0] AC#3: Esc close moves focus to the scroll container so
// subsequent F3 / Shift+F3 keystrokes still reach the window-level navigation
// handler (the App.jsx Cmd+G focus-guard relies on focus being OFF the
// FindBar input after Esc). The scroll container is lazily given tabindex=-1
// so it can accept programmatic focus.
// ---------------------------------------------------------------------------

describe('10-2-INTG-011: Esc restores focus to the scroll container (AC3)', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Esc closes the bar and document.activeElement is the scroll container', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.queryByText('first /Helvetica line')).toBeInTheDocument();
    });
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    const input = screen.getByTestId('plain-text-find-input');
    expect(document.activeElement).toBe(input);
    fireEvent.keyDown(input, { key: 'Escape' });
    expect(screen.queryByTestId('plain-text-find-bar')).toBeNull();
    const scroll = screen.getByTestId('plain-text-scroll');
    // tabindex must be set lazily so programmatic focus works.
    expect(scroll.getAttribute('tabindex')).toBe('-1');
    expect(document.activeElement).toBe(scroll);
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-012 [P0] AC#7: auto-scroll on active-match change. When the new
// active match is outside the vertical viewport, scrollTop is set so the
// match's line is centered. jsdom does not lay out the DOM, so we stub
// clientHeight / scrollHeight on the scroll container to simulate a viewport
// smaller than the corpus. ROW_HEIGHT is 20.
// ---------------------------------------------------------------------------

describe('10-2-INTG-012: auto-scroll on Next when match is below the viewport (AC7)', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Next click on a match below the viewport bumps scrollTop', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.queryByText('first /Helvetica line')).toBeInTheDocument();
    });

    const scroll = screen.getByTestId('plain-text-scroll') as HTMLDivElement;
    // Simulate a 40px tall viewport that holds 2 rows; the 3rd and 4th
    // /Helvetica matches sit beyond it. scrollHeight = 4 lines * 20px = 80.
    Object.defineProperty(scroll, 'clientHeight', { value: 40, configurable: true });
    Object.defineProperty(scroll, 'scrollHeight', { value: 80, configurable: true });
    Object.defineProperty(scroll, 'clientWidth', { value: 400, configurable: true });
    Object.defineProperty(scroll, 'scrollWidth', { value: 400, configurable: true });
    // Stub scrollTo so the auto-scroll path can call into it without jsdom
    // throwing. The matchedTarget is then captured from the scrollTop setter.
    const scrollToSpy = vi.fn((opts: ScrollToOptions) => {
      if (typeof opts.top === 'number') {
        Object.defineProperty(scroll, 'scrollTop', { value: opts.top, configurable: true });
      }
    });
    scroll.scrollTo = scrollToSpy as unknown as typeof scroll.scrollTo;

    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    const input = screen.getByTestId('plain-text-find-input') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'Helvetica' } });
    await waitFor(() => {
      expect(screen.getAllByTestId('plain-text-find-match').length).toBeGreaterThan(0);
    });

    const scrollTopBefore = scroll.scrollTop;
    // Advance to match 3 (line 3, lineTop = 40) which sits below the 40px
    // viewport when scrollTop is 0. Spec: scrollTop = clamp(lineTop - clientHeight/2, ...)
    fireEvent.click(screen.getByTestId('plain-text-find-next'));
    fireEvent.click(screen.getByTestId('plain-text-find-next'));

    // Either scrollTo was called (smooth path) or scrollTop was set directly
    // (reduced-motion fallback / try-catch fallback). Both satisfy AC7.
    const scrolled =
      scrollToSpy.mock.calls.length > 0 || scroll.scrollTop !== scrollTopBefore;
    expect(scrolled).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-013 [P0] AC#7: when the active match is already visible, the
// auto-scroll effect must NOT touch scrollTop. This pins the visibility
// short-circuit so the scroll container doesn't jitter mid-typing.
// ---------------------------------------------------------------------------

describe('10-2-INTG-013: auto-scroll skips when the match is already visible (AC7)', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('opening the bar on a match that is already in view does NOT call scrollTo', async () => {
    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.queryByText('first /Helvetica line')).toBeInTheDocument();
    });

    const scroll = screen.getByTestId('plain-text-scroll') as HTMLDivElement;
    // 200px viewport easily contains all 4 lines (80px tall).
    Object.defineProperty(scroll, 'clientHeight', { value: 200, configurable: true });
    Object.defineProperty(scroll, 'scrollHeight', { value: 80, configurable: true });
    Object.defineProperty(scroll, 'clientWidth', { value: 400, configurable: true });
    Object.defineProperty(scroll, 'scrollWidth', { value: 400, configurable: true });
    const scrollToSpy = vi.fn();
    scroll.scrollTo = scrollToSpy as unknown as typeof scroll.scrollTo;

    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    const input = screen.getByTestId('plain-text-find-input') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'Helvetica' } });
    await waitFor(() => {
      expect(screen.getAllByTestId('plain-text-find-match').length).toBeGreaterThan(0);
    });

    // Match 1 (line 1, lineTop=0, height=20) sits inside the 200px viewport.
    // No vertical scroll required. Horizontal short-circuit needs col=0 too,
    // which the first match satisfies (it starts on the first line).
    expect(scrollToSpy).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 10-2-INTG-010 [P0] AC#22: Esc inside the FindBar closes the bar and does
// NOT propagate to a sibling window-level keydown listener.
// ---------------------------------------------------------------------------

describe('10-2-INTG-010: Esc scope (does not collide with palette/window Esc)', () => {
  let restore: () => void;
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlainText.mockResolvedValue(helveticaCorpus);
    mockGetPlainTextSize.mockResolvedValue(helveticaCorpus.totalBytes);
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Esc inside FindBar input closes the bar; a window-level Esc listener does NOT fire', async () => {
    const windowEscListener = vi.fn();
    window.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        windowEscListener();
      }
    });
    try {
      render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: Wrapper });
      await waitFor(() => {
        expect(screen.queryByText('first /Helvetica line')).toBeInTheDocument();
      });
      act(() => {
        dispatchKey({ key: 'f', metaKey: true });
      });
      const input = screen.getByTestId('plain-text-find-input');
      fireEvent.keyDown(input, { key: 'Escape' });
      expect(screen.queryByTestId('plain-text-find-bar')).toBeNull();
      // The Esc handler must be scoped to the FindBar root, so a non-bubbling
      // pattern (e.g. stopPropagation) prevents the sibling from co-firing.
      expect(windowEscListener).not.toHaveBeenCalled();
    } finally {
      // RTL render cleanup teardown removes the bar; the listener cleanup is
      // best-effort -- subsequent tests use afterEach hooks via vi.clearAllMocks.
    }
  });
});
