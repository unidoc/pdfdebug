/**
 * @file Plain Text view -- document-level Latin-1-decoded file bytes with a
 * 1-based line-number gutter and viewport virtualization. Story 9-11
 * (initial); Story 10-1 (single uncapped lazy load + cancellable read +
 * loading card with size disclosure and Cancel button).
 *
 * Lines are split on /\r\n?|\n/ so CRLF / lone CR / lone LF all collapse to
 * one logical row.
 *
 * Hand-rolled viewport virtualization: a tall spacer fixes the total scroll
 * height, but only the visible slice of rows is rendered. Keeps the DOM under
 * 200 rows even for multi-GB payloads.
 */
import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { useLatest } from '../hooks/useLatest';
import { flushSync } from 'react-dom';
import {
  GetPlainText,
  GetPlainTextSize,
  CancelPlainText,
} from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';
import { useAppDispatch, useAppState } from '../hooks/useDocumentState';
import { useFindBar } from '../hooks/useFindBar';
import { FindBar } from './FindBar';
import type { Match } from '../lib/findMatches';

/** Plain Text payload mirroring `pdfcore.PlainTextDocument`. */
interface PlainTextDocumentData {
  tabId: string;
  content: string;
  totalBytes: number;
}

/** Props for {@link PlainTextView}. */
export interface PlainTextViewProps {
  /** Active document tab ID. Empty string renders the no-document empty state. */
  tabId: string;
  /** True when the Plain Text tab is active; gates the lazy fetch + scroll-to-top. */
  active: boolean;
}

/** Approximate per-row pixel height (font-mono text-sm line-height in style.css). */
const ROW_HEIGHT = 20;
/** Number of rows to render above/below the viewport for smooth scrolling. */
const OVERSCAN = 20;

/** Per-component load lifecycle. Story 10-1. */
type LoadState = 'idle' | 'loading' | 'ready' | 'cancelled' | 'error';

/**
 * Binary-base size formatter for user-facing copy. JEDEC-style labels
 * (KB/MB/GB, not KiB/MiB/GiB). Non-finite or negative inputs collapse to
 * "0 B".
 */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) {
    return '0 B';
  }
  if (n < 1024) {
    return `${n} B`;
  }
  // Promote across a unit boundary when 1-decimal rounding would otherwise
  // render the full count of the lower unit (e.g. "1024.0 KB" -> "1.0 MB").
  if (n < 1024 * 1024 && n / 1024 < 1023.95) {
    return `${(n / 1024).toFixed(1)} KB`;
  }
  if (n < 1024 * 1024 * 1024 && n / (1024 * 1024) < 1023.95) {
    return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  }
  return `${(n / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

/**
 * Document-level Plain Text view. Lazy-fetches on first activation; renders
 * a virtualized scroll container once the payload is ready. Story 10-1.
 */
export function PlainTextView({ tabId, active }: PlainTextViewProps) {
  const [data, setData] = useState<PlainTextDocumentData | null>(null);
  const [loadState, setLoadState] = useState<LoadState>('idle');
  const [error, setError] = useState<string | null>(null);
  const [totalBytes, setTotalBytes] = useState<number | null>(null);
  const [showLoadingCard, setShowLoadingCard] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(0);

  // Per-tab case-sensitivity toggle on TabState (Story 10-2).
  const appState = useAppState();
  const dispatch = useAppDispatch();
  const findCaseSensitive =
    appState.tabs.find((t) => t.tabId === tabId)?.findCaseSensitive ?? false;

  const scrollRef = useRef<HTMLDivElement | null>(null);
  // Cache data as a ref so the fetch effect doesn't re-fire on data change.
  // useLatest mirrors `data` during render (#28); the imperative dataRef writes
  // in the reset/resolve paths below still apply (useLatest returns a stable
  // ref and each write is paired with the matching setData, so render-phase
  // mirroring never disagrees with a fresher imperative write).
  const dataRef = useLatest(data);
  const inFlightRef = useRef(false);
  // tabId mirrored during render via useLatest (#28); the resolve/reject
  // branches capture tabId at call time and compare against this ref before
  // mutating state on a stale fetch. The imperative write in the reset
  // effect is retained for clarity but is now redundant with useLatest.
  const tabIdRef = useLatest(tabId);
  // Mirrors loadState for the lazy-fetch effect's guard so a terminal state
  // (cancelled / error) on the previous activation does not silently restart
  // when the user toggles back to the Plain Text inner tab. The fetch effect
  // fires before React commits the reset effect's setLoadState('idle'), so
  // reading loadState directly would observe stale 'ready' / 'error' /
  // 'cancelled' values; the ref is reset synchronously inside the reset
  // effect to keep the two in sync.
  const loadStateRef = useRef<LoadState>('idle');

  // Reset state on document change.
  useEffect(() => {
    tabIdRef.current = tabId;
    setData(null);
    setError(null);
    setLoadState('idle');
    setTotalBytes(null);
    setShowLoadingCard(false);
    setCancelling(false);
    setScrollTop(0);
    dataRef.current = null;
    inFlightRef.current = false;
    loadStateRef.current = 'idle';
    // dataRef / tabIdRef are stable useLatest refs (identity never changes);
    // listed to satisfy exhaustive-deps without a disable.
  }, [tabId, dataRef, tabIdRef]);

  /** Kicks the GetPlainText + GetPlainTextSize pair. Story 10-1. */
  const handleLoad = useCallback(() => {
    if (!tabId) return;
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    loadStateRef.current = 'loading';
    setLoadState('loading');
    setError(null);
    setCancelling(false);
    const tabIdAtFetch = tabId;
    // Size disclosure -- independent of the main fetch. flushSync forces
    // an immediate commit so a settled GetPlainTextSize resolution paints
    // before the next scheduler tick; without it, the size cell can lag
    // behind the loading card mount under fake-timer tests (Vitest
    // schedulers idle while fake clocks are frozen). In production the
    // visible difference is at most one frame.
    const sizePromise = GetPlainTextSize(tabIdAtFetch);
    if (sizePromise && typeof sizePromise.then === 'function') {
      sizePromise
        .then((size: unknown) => {
          if (tabIdAtFetch !== tabIdRef.current) return;
          flushSync(() => {
            setTotalBytes(typeof size === 'number' ? size : null);
          });
        })
        .catch(() => {
          // Size disclosure is best-effort; failure leaves the element empty.
          if (tabIdAtFetch !== tabIdRef.current) return;
        });
    }
    GetPlainText(tabIdAtFetch)
      .then((result: unknown) => {
        if (tabIdAtFetch !== tabIdRef.current) return;
        inFlightRef.current = false;
        const doc = result as PlainTextDocumentData;
        dataRef.current = doc;
        loadStateRef.current = 'ready';
        setData(doc);
        setLoadState('ready');
      })
      .catch((err: unknown) => {
        if (tabIdAtFetch !== tabIdRef.current) return;
        inFlightRef.current = false;
        const msg = extractErrorMessage(err);
        // Cancellation contract: extractErrorMessage substring 'cancel'
        // (case-insensitive) routes to the cancelled state. The backend
        // errors.Is(err, context.Canceled) identity is the authoritative
        // Go-side contract; this is the frontend matching path.
        if (/cancel/i.test(msg)) {
          loadStateRef.current = 'cancelled';
          setLoadState('cancelled');
          setCancelling(false);
        } else {
          loadStateRef.current = 'error';
          setError(msg);
          setLoadState('error');
        }
      });
    // dataRef / tabIdRef are stable useLatest refs; listed to satisfy
    // exhaustive-deps without a disable.
  }, [tabId, dataRef, tabIdRef]);

  // Lazy fetch gated on `active`. Story 10-1. handleLoad is omitted from deps
  // (and loadState is too via the ref) because handleLoad already guards via
  // inFlightRef; re-running this effect on every loadState transition would
  // race the trigger conditions.
  //
  // loadStateRef gates auto-fetch to the first activation of an idle component
  // only. Once the load has produced a terminal state (`cancelled` or
  // `error`), the user must click the explicit CTA -- toggling the Plain Text
  // inner tab away and back must NOT silently re-fetch.
  useEffect(() => {
    if (!tabId) return;
    if (!active) return;
    if (dataRef.current !== null) return;
    if (inFlightRef.current) return;
    if (loadStateRef.current !== 'idle') return;
    handleLoad();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tabId, active]);

  // 200ms loading-card debounce. Avoids flash-of-loading for the fast path
  // (OS page-cache-warm reads complete under 200ms). Story 10-1.
  useEffect(() => {
    if (loadState !== 'loading') {
      setShowLoadingCard(false);
      return;
    }
    const timer = setTimeout(() => setShowLoadingCard(true), 200);
    return () => clearTimeout(timer);
  }, [loadState]);

  // Scroll to top whenever `active` transitions false -> true.
  useEffect(() => {
    if (!active) return;
    if (scrollRef.current) {
      scrollRef.current.scrollTop = 0;
    }
    setScrollTop(0);
  }, [active]);

  // Track viewport height for virtualization.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    setViewportHeight(el.clientHeight);
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(() => {
      setViewportHeight(el.clientHeight);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [data]);

  /** Memoized line split: CRLF / CR / LF all collapse to one row each.
   * zero-byte case renders an empty list (no synthetic single empty row). */
  const lines = useMemo(() => {
    if (!data) return [] as string[];
    const content = data.content ?? '';
    if (content === '') return [] as string[];
    return content.split(/\r\n?|\n/);
  }, [data]);

  // Story 10-2: find-bar hook. content is the raw payload when load is ready;
  // Null otherwise so Cmd+F preventDefault-only.
  const findBar = useFindBar({
    tabId,
    content: data ? data.content : null,
    caseSensitive: findCaseSensitive,
    active,
  });
  const {
    open: findOpen,
    query: findQuery,
    matches: findMatchesList,
    activeIndex: findActiveIndex,
    wrapped: findWrapped,
    nonLatin1: findNonLatin1,
    lineStartOffsets: findLineStarts,
    focusVersion: findFocusVersion,
    setQuery: setFindQuery,
    next: findNext,
    prev: findPrev,
    closeBar: closeFindBar,
  } = findBar;

  const handleCaseToggle = useCallback(() => {
    dispatch({
      type: 'SET_FIND_CASE_SENSITIVE',
      payload: { tabId, value: !findCaseSensitive },
    });
  }, [dispatch, tabId, findCaseSensitive]);

  // On Esc close, restore focus to the scroll container so subsequent F3 /
  // Shift+F3 keystrokes still reach the window-level navigation handler (and so
  // the input-focus check in App.jsx's Cmd+G handler does not erroneously see a
  // stale FindBar input as the active text field).
  const handleFindClose = useCallback(() => {
    closeFindBar();
    const el = scrollRef.current;
    if (el) {
      // tabIndex=-1 lets a div accept programmatic focus without entering the
      // tab order. Set it lazily so we don't bake the attribute into the
      // markup until needed.
      if (!el.hasAttribute('tabindex')) {
        el.setAttribute('tabindex', '-1');
      }
      el.focus({ preventScroll: true });
    }
  }, [closeFindBar]);

  /** Lines that contain at least one match, for the gutter density marker. */
  const matchedLineSet = useMemo(() => {
    const s = new Set<number>();
    for (const m of findMatchesList) s.add(m.line);
    return s;
  }, [findMatchesList]);

  /**
   * Per-row matches table. Index by 1-based line number. Each entry is the
   * subset of findMatchesList whose start offset falls in that row's byte
   * range. O(M) bucketing, single pass, computed once per matches list.
   */
  const matchesByLine = useMemo(() => {
    const map = new Map<number, Match[]>();
    for (const m of findMatchesList) {
      const arr = map.get(m.line);
      if (arr) arr.push(m);
      else map.set(m.line, [m]);
    }
    return map;
  }, [findMatchesList]);

  // Cache the prefers-reduced-motion media query and subscribe to OS-level
  // changes so a mid-session toggle is honored. Read on the scroll path.
  const reducedMotionRef = useRef<boolean>(false);
  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return;
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)');
    reducedMotionRef.current = mq.matches;
    const onChange = (e: MediaQueryListEvent) => {
      reducedMotionRef.current = e.matches;
    };
    if (typeof mq.addEventListener === 'function') {
      mq.addEventListener('change', onChange);
      return () => mq.removeEventListener('change', onChange);
    }
  }, []);

  const totalRows = lines.length;
  const totalHeight = totalRows * ROW_HEIGHT;
  const firstVisible = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const visibleCount = Math.ceil((viewportHeight || 400) / ROW_HEIGHT) + OVERSCAN * 2;
  const lastVisible = Math.min(totalRows, firstVisible + visibleCount);
  const rowsToRender = lines.slice(firstVisible, lastVisible);

  const handleScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  }, []);

  // Auto-scroll on active-match change. Centers the line on change when not
  // already visible; uses smooth scroll unless prefers-reduced-motion is set.
  // Also best-effort adjusts scrollLeft so the match's start sits at least 8
  // columns inside the visible horizontal viewport (horizontal scroll
  // requirement).
  useEffect(() => {
    if (!findOpen && findMatchesList.length === 0) return;
    const el = scrollRef.current;
    if (!el) return;
    const m = findMatchesList[findActiveIndex];
    if (!m) return;

    // Vertical: center the active line when not already visible.
    const lineTop = m.line * ROW_HEIGHT - ROW_HEIGHT;
    const viewportTop = el.scrollTop;
    const viewportBottom = viewportTop + el.clientHeight;
    const verticallyVisible =
      lineTop >= viewportTop && lineTop + ROW_HEIGHT <= viewportBottom;
    const verticalTarget = verticallyVisible
      ? el.scrollTop
      : Math.max(0, Math.min(lineTop - el.clientHeight / 2, el.scrollHeight - el.clientHeight));

    // Horizontal: best-effort column-to-pixel mapping via the mono-font width
    // of a single character. Measured once per effect run from the live DOM
    // (the gutter cell contains a digit at the right font/size). If
    // measurement fails (zero/NaN width), skip the horizontal adjustment.
    let horizontalTarget = el.scrollLeft;
    const rowStart = findLineStarts[m.line - 1] ?? 0;
    const col = Math.max(0, m.start - rowStart);
    // Read the char width from an existing rendered row's textContent. The
    // gutter cell is sticky and present for every visible row. Width per
    // character = element.scrollWidth / textContent.length when textContent
    // is non-empty and single-line.
    const probe = el.querySelector<HTMLElement>('[data-testid^="plain-text-row-"]');
    if (probe && probe.textContent && probe.textContent.length > 0) {
      const charWidth = probe.scrollWidth / probe.textContent.length;
      if (charWidth > 0 && Number.isFinite(charWidth)) {
        const matchLeftPx = col * charWidth;
        const visibleLeft = el.scrollLeft;
        const visibleRight = visibleLeft + el.clientWidth;
        const inset = 8 * charWidth; // "at least 8 columns inside"
        if (matchLeftPx < visibleLeft + inset) {
          horizontalTarget = Math.max(0, matchLeftPx - inset);
        } else if (matchLeftPx > visibleRight - inset) {
          horizontalTarget = Math.max(
            0,
            Math.min(matchLeftPx - el.clientWidth + inset, el.scrollWidth - el.clientWidth),
          );
        }
      }
    }

    const verticalNeedsScroll = !verticallyVisible;
    const horizontalNeedsScroll = horizontalTarget !== el.scrollLeft;
    if (!verticalNeedsScroll && !horizontalNeedsScroll) return;

    if (reducedMotionRef.current) {
      if (verticalNeedsScroll) el.scrollTop = verticalTarget;
      if (horizontalNeedsScroll) el.scrollLeft = horizontalTarget;
    } else {
      try {
        el.scrollTo({ top: verticalTarget, left: horizontalTarget, behavior: 'smooth' });
      } catch {
        if (verticalNeedsScroll) el.scrollTop = verticalTarget;
        if (horizontalNeedsScroll) el.scrollLeft = horizontalTarget;
      }
    }
  }, [findActiveIndex, findMatchesList, findOpen, findLineStarts]);

  /** Fire-and-forget Cancel. The original GetPlainText promise rejects with
   * context.Canceled; the .catch branch flips to 'cancelled'. Story 10-1. */
  const handleCancel = useCallback(() => {
    if (cancelling) return;
    setCancelling(true);
    // Fire-and-forget; we do NOT await. The reject branch of GetPlainText
    // handles state transition. Attach an empty catch so a rejection on the
    // cancel call itself (e.g. ErrDocumentNotFound during a close race) does
    // not surface as an unhandled promise rejection.
    CancelPlainText(tabId).catch(() => {});
  }, [cancelling, tabId]);

  // Empty / no-document state.
  if (!tabId) {
    return (
      <div
        className="h-full flex items-center justify-center text-text-muted text-sm"
        data-testid="plain-text-empty"
      >
        No document open
      </div>
    );
  }

  if (loadState === 'error') {
    return (
      <div className="h-full flex flex-col items-center justify-center gap-3 p-6 text-sm">
        <div className="text-error" data-testid="plain-text-error">
          {error}
        </div>
        <button
          type="button"
          data-testid="plain-text-load-cta"
          className="bg-bg border border-border rounded px-3 py-1 text-sm text-text-primary hover:bg-surface-hover cursor-pointer disabled:cursor-not-allowed disabled:opacity-60"
          onClick={handleLoad}
        >
          Retry
        </button>
      </div>
    );
  }

  if (loadState === 'cancelled') {
    return (
      <div className="h-full flex flex-col items-center justify-center gap-3 p-6 text-sm">
        <div className="text-text-muted">Plain text load cancelled.</div>
        <button
          type="button"
          data-testid="plain-text-load-cta"
          className="bg-bg border border-border rounded px-3 py-1 text-sm text-text-primary hover:bg-surface-hover cursor-pointer disabled:cursor-not-allowed disabled:opacity-60"
          onClick={handleLoad}
        >
          Load plain text
        </button>
      </div>
    );
  }

  // Loading card (only after 200ms debounce; fast-path skips this entirely).
  if (loadState === 'loading' && showLoadingCard && !data) {
    return (
      <div
        className="h-full flex items-center justify-center p-6"
        data-testid="plain-text-loading-card"
      >
        <div className="flex flex-col items-center gap-2 text-sm text-text-muted">
          <div
            data-testid="plain-text-loading-spinner"
            aria-hidden="true"
            className="animate-spin w-8 h-8 rounded-full border-2 border-text-muted border-t-transparent mb-1"
          />
          <div className="font-medium text-text-primary">Loading plain text</div>
          <div data-testid="plain-text-loading-size" className="min-h-[1em]">
            {totalBytes !== null ? formatBytes(totalBytes) : ''}
          </div>
          <button
            type="button"
            data-testid="plain-text-cancel-button"
            className="bg-bg border border-border rounded px-3 py-1 text-sm text-text-primary hover:bg-surface-hover cursor-pointer disabled:cursor-not-allowed disabled:opacity-60"
            disabled={cancelling}
            onClick={handleCancel}
          >
            {cancelling ? 'Cancelling' : 'Cancel'}
          </button>
        </div>
      </div>
    );
  }

  // Pre-debounce loading -- empty placeholder keeps the panel from flashing.
  if (!data) {
    return <div className="h-full" data-testid="plain-text-empty-initial" />;
  }

  return (
    <div className="h-full flex flex-col bg-bg">
      {findOpen && (
        <FindBar
          matches={findMatchesList}
          activeIndex={findActiveIndex}
          query={findQuery}
          caseSensitive={findCaseSensitive}
          wrapped={findWrapped}
          nonLatin1={findNonLatin1}
          focusVersion={findFocusVersion}
          onQueryChange={setFindQuery}
          onNext={findNext}
          onPrev={findPrev}
          onCaseToggle={handleCaseToggle}
          onClose={handleFindClose}
        />
      )}
      <div
        className="flex-1 overflow-auto font-mono text-sm"
        data-testid="plain-text-scroll"
        ref={scrollRef}
        onScroll={handleScroll}
      >
        <div style={{ position: 'relative', height: totalHeight }}>
          <div
            className="flex pr-4"
            style={{
              position: 'absolute',
              top: firstVisible * ROW_HEIGHT,
              left: 0,
              minWidth: '100%',
              width: 'max-content',
            }}
          >
            {/* Gutter column: sticky-left so line numbers stay visible during
                horizontal scroll on lines longer than the viewport. */}
            <div
              className="flex-shrink-0 select-none text-right text-text-muted px-2 border-l border-r border-border sticky left-0 bg-bg z-10"
              data-testid="plain-text-gutter"
              style={{ minWidth: '4ch' }}
            >
              {rowsToRender.map((_, i) => {
                const lineNo = firstVisible + i + 1;
                const cell = (
                  <div style={{ height: ROW_HEIGHT, lineHeight: `${ROW_HEIGHT}px` }}>{lineNo}</div>
                );
                // Gutter density marker on lines with at least one match.
                if (matchedLineSet.has(lineNo)) {
                  return (
                    <div
                      key={lineNo}
                      data-testid={`plain-text-find-gutter-marker-${lineNo}`}
                      className="border-l-2 border-find-gutter"
                    >
                      {cell}
                    </div>
                  );
                }
                return <div key={lineNo}>{cell}</div>;
              })}
            </div>
            {/* Content column */}
            <div className="flex-1 pl-2 text-text whitespace-pre">
              {rowsToRender.map((line, i) => {
                const lineNo = firstVisible + i + 1;
                // Per-row mark slicing. The row's offset range is
                // [findLineStarts[lineNo - 1], findLineStarts[lineNo - 1] + line.length).
                const rowMatches = matchesByLine.get(lineNo);
                let content: React.ReactNode = line;
                if (rowMatches && rowMatches.length > 0) {
                  const rowStart = findLineStarts[lineNo - 1] ?? 0;
                  content = renderLineWithMarks(
                    line,
                    rowStart,
                    rowMatches,
                    findMatchesList,
                    findActiveIndex,
                  );
                }
                return (
                  <div
                    key={lineNo}
                    data-testid={`plain-text-row-${lineNo}`}
                    style={{ height: ROW_HEIGHT, lineHeight: `${ROW_HEIGHT}px` }}
                  >
                    {content}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

/**
 * Render a single content row, wrapping matched substrings in `<mark>` tags.
 * The concatenated text content of the returned fragment equals the raw
 * line. The active match (matches[activeIndex]) gets the
 * `plain-text-find-active-match` testid + bg-find-active class; non-active
 * matches get `plain-text-find-match` + bg-find-match.
 */
function renderLineWithMarks(
  line: string,
  rowStart: number,
  rowMatches: Match[],
  allMatches: Match[],
  activeIndex: number,
): React.ReactNode[] {
  const activeMatch = allMatches[activeIndex];
  const nodes: React.ReactNode[] = [];
  let cursor = 0;
  const lineLen = line.length;
  for (const m of rowMatches) {
    const localStart = m.start - rowStart;
    const localEnd = Math.min(lineLen, m.end - rowStart);
    if (localStart < 0 || localStart >= lineLen) continue;
    if (cursor < localStart) {
      nodes.push(
        <span key={`s-${cursor}`}>{line.slice(cursor, localStart)}</span>,
      );
    }
    const isActive = activeMatch !== undefined && m.start === activeMatch.start;
    nodes.push(
      <mark
        key={`m-${m.start}`}
        data-testid={isActive ? 'plain-text-find-active-match' : 'plain-text-find-match'}
        className={
          isActive
            ? 'bg-find-active text-find-active-fg'
            : 'bg-find-match text-find-match-fg'
        }
      >
        {line.slice(localStart, localEnd)}
      </mark>,
    );
    cursor = localEnd;
  }
  if (cursor < lineLen) {
    nodes.push(<span key={`s-end-${cursor}`}>{line.slice(cursor)}</span>);
  }
  return nodes;
}

export default PlainTextView;
