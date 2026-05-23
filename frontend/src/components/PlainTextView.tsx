/**
 * @file Plain Text view -- document-level Latin-1-decoded file bytes with a
 * 1-based line-number gutter and viewport virtualization. Story 9-11
 * (initial); Story 10-1 (single uncapped lazy load + cancellable read +
 * loading card with size disclosure, elapsed counter, and Cancel button).
 *
 * Lines are split on /\r\n?|\n/ so CRLF / lone CR / lone LF all collapse to
 * one logical row.
 *
 * Hand-rolled viewport virtualization: a tall spacer fixes the total scroll
 * height, but only the visible slice of rows is rendered. Keeps the DOM under
 * 200 rows even for multi-GB payloads.
 */
import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { flushSync } from 'react-dom';
import {
  GetPlainText,
  GetPlainTextSize,
  CancelPlainText,
} from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';

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
function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) {
    return '0 B';
  }
  if (n < 1024) {
    return `${n} B`;
  }
  if (n < 1024 * 1024) {
    return `${(n / 1024).toFixed(1)} KB`;
  }
  if (n < 1024 * 1024 * 1024) {
    return `${Math.round(n / (1024 * 1024))} MB`;
  }
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
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
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  const [cancelling, setCancelling] = useState(false);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(0);

  const scrollRef = useRef<HTMLDivElement | null>(null);
  // Cache data as a ref so the fetch effect doesn't re-fire on data change.
  const dataRef = useRef<PlainTextDocumentData | null>(null);
  const inFlightRef = useRef(false);
  // tabId at the latest reset-effect run; the resolve/reject branches
  // capture tabId at call time and compare against this ref before mutating
  // state on a stale fetch (AC8).
  const tabIdRef = useRef(tabId);
  // Mirrors loadState for the lazy-fetch effect's guard so a terminal state
  // (cancelled / error) on the previous activation does not silently restart
  // when the user toggles back to the Plain Text inner tab. The fetch effect
  // fires before React commits the reset effect's setLoadState('idle'), so
  // reading loadState directly would observe stale 'ready' / 'error' /
  // 'cancelled' values; the ref is reset synchronously inside the reset
  // effect to keep the two in sync.
  const loadStateRef = useRef<LoadState>('idle');

  // Reset state on document change (AC17).
  useEffect(() => {
    tabIdRef.current = tabId;
    setData(null);
    setError(null);
    setLoadState('idle');
    setTotalBytes(null);
    setShowLoadingCard(false);
    setElapsedSeconds(0);
    setCancelling(false);
    setScrollTop(0);
    dataRef.current = null;
    inFlightRef.current = false;
    loadStateRef.current = 'idle';
  }, [tabId]);

  /** Kicks the GetPlainText + GetPlainTextSize pair. Story 10-1 AC1, AC2, AC6, AC7. */
  const handleLoad = useCallback(() => {
    if (!tabId) return;
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    loadStateRef.current = 'loading';
    setLoadState('loading');
    setError(null);
    setElapsedSeconds(0);
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
        // (case-insensitive) routes to the cancelled state per AC4. The
        // backend errors.Is(err, context.Canceled) identity is the
        // authoritative Go-side contract; this is the frontend matching path.
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
  }, [tabId]);

  // Lazy fetch gated on `active`. Story 10-1 AC1. handleLoad is omitted from
  // deps (and loadState is too via the ref) because handleLoad already guards
  // via inFlightRef; re-running this effect on every loadState transition
  // would race the trigger conditions.
  //
  // loadStateRef gates auto-fetch to the first activation of an idle component
  // only. Once the load has produced a terminal state (`cancelled` or
  // `error`), the user must click the explicit CTA (AC5 / AC7) -- toggling
  // the Plain Text inner tab away and back must NOT silently re-fetch.
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
  // (OS page-cache-warm reads complete under 200ms). Story 10-1 AC20.
  useEffect(() => {
    if (loadState !== 'loading') {
      setShowLoadingCard(false);
      return;
    }
    const timer = setTimeout(() => setShowLoadingCard(true), 200);
    return () => clearTimeout(timer);
  }, [loadState]);

  // Elapsed-seconds counter -- ticks once per second while the loading card
  // is visible. Cleared on every other state transition. Story 10-1 AC2.
  useEffect(() => {
    if (!showLoadingCard) {
      return;
    }
    const interval = setInterval(() => {
      setElapsedSeconds((s) => s + 1);
    }, 1000);
    return () => clearInterval(interval);
  }, [showLoadingCard]);

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

  /** Memoized line split: CRLF / CR / LF all collapse to one row each. AC21
   * zero-byte case renders an empty list (no synthetic single empty row). */
  const lines = useMemo(() => {
    if (!data) return [] as string[];
    const content = data.content ?? '';
    if (content === '') return [] as string[];
    return content.split(/\r\n?|\n/);
  }, [data]);

  const totalRows = lines.length;
  const totalHeight = totalRows * ROW_HEIGHT;
  const firstVisible = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const visibleCount = Math.ceil((viewportHeight || 400) / ROW_HEIGHT) + OVERSCAN * 2;
  const lastVisible = Math.min(totalRows, firstVisible + visibleCount);
  const rowsToRender = lines.slice(firstVisible, lastVisible);

  const handleScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  }, []);

  /** Fire-and-forget Cancel. The original GetPlainText promise rejects with
   * context.Canceled; the .catch branch flips to 'cancelled'. Story 10-1 AC4. */
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
          <div className="font-medium text-text-primary">Loading plain text</div>
          <div data-testid="plain-text-loading-size" className="min-h-[1em]">
            {totalBytes !== null ? formatBytes(totalBytes) : ''}
          </div>
          <div data-testid="plain-text-loading-elapsed">{elapsedSeconds}s</div>
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
                return (
                  <div key={lineNo} style={{ height: ROW_HEIGHT, lineHeight: `${ROW_HEIGHT}px` }}>
                    {lineNo}
                  </div>
                );
              })}
            </div>
            {/* Content column */}
            <div className="flex-1 pl-2 text-text whitespace-pre">
              {rowsToRender.map((line, i) => {
                const lineNo = firstVisible + i + 1;
                return (
                  <div
                    key={lineNo}
                    data-testid={`plain-text-row-${lineNo}`}
                    style={{ height: ROW_HEIGHT, lineHeight: `${ROW_HEIGHT}px` }}
                  >
                    {line}
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

export default PlainTextView;
