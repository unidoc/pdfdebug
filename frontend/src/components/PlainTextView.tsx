/**
 * @file Plain Text view -- document-level Latin-1-decoded file bytes with a
 * 1-based line-number gutter. Story 9-11. Lines are split on /\r\n?|\n/ so
 * CRLF / lone CR / lone LF all collapse to one logical row.
 *
 * The view uses hand-rolled viewport virtualization: a tall spacer fixes the
 * total scroll height, but only the visible slice of rows is rendered. This
 * keeps the DOM small (under 200 rows) even for the 25 MiB truncation cap.
 *
 * Story 9-12: adds the "Load all" escape hatch on the truncation banner.
 * formatBytes formats sizes; SIZE_LABEL_THRESHOLD gates the in-label size
 * suffix + warning-color tokens. Errors from the Load-all fetch are
 * dispatched via SET_DOCUMENT_ERROR so the global ErrorBanner surfaces them.
 */
import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import {
  GetPlainText,
  GetPlainTextFull,
} from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';
import { useAppDispatch } from '../hooks/useDocumentState';

/** Plain Text payload mirroring `pdfcore.PlainTextDocument`. */
interface PlainTextDocumentData {
  tabId: string;
  content: string;
  totalBytes: number;
  truncated: boolean;
  capBytes: number;
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

/**
 * Threshold above which the "Load all" button surfaces the size in its label
 * and shifts to warning-color tokens. 100 MiB exact. Story 9-12 AC3/AC4.
 */
const SIZE_LABEL_THRESHOLD = 100 * 1024 * 1024;

/**
 * Binary-base size formatter for user-facing copy. JEDEC-style labels
 * (KB/MB/GB, not KiB/MiB/GiB). Story 9-12 AC2.
 *
 * Non-finite or negative inputs collapse to "0 B" (defensive: the backend
 * always returns a non-negative int64, but a malformed payload should not
 * crash the banner). Boundary artifacts at the unit edges (e.g. n=1048575
 * formatting as "1024.0 KB", n=1073741823 formatting as "1024 MB") are
 * accepted per AC2 - harmless and explicit, not hidden by a fudge factor.
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
 * a virtualized scroll container.
 */
export function PlainTextView({ tabId, active }: PlainTextViewProps) {
  const [data, setData] = useState<PlainTextDocumentData | null>(null);
  const [loading, setLoading] = useState(false);
  const [showLoading, setShowLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(0);
  // Story 9-12: separate state machine for the "Load all" click-driven fetch.
  const [loadingFull, setLoadingFull] = useState(false);
  const [loadFullErrored, setLoadFullErrored] = useState(false);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  // Cache flags as refs so the fetch effect only re-runs when tabId / active
  // change. See XRefTableView for the rationale.
  const dataRef = useRef<PlainTextDocumentData | null>(null);
  const inFlightRef = useRef(false);
  // Story 9-12: Load-all in-flight guard (re-entrancy + stale-fetch).
  const fullInFlightRef = useRef(false);
  // tabId at the latest reset-effect run; the click handler captures tabId at
  // call time and compares against this ref before mutating state on
  // resolve/reject. Per AC9 option B.
  const tabIdRef = useRef(tabId);

  const dispatch = useAppDispatch();

  // Reset state on document change (AC17 / Task 7.x parallel to XRefTableView).
  useEffect(() => {
    // Set the stale-fetch guard FIRST so any in-flight resolve that fires
    // synchronously between effect-runs sees the new tabId. Story 9-12 AC9.
    tabIdRef.current = tabId;
    setData(null);
    setError(null);
    setLoading(false);
    setShowLoading(false);
    setScrollTop(0);
    setLoadingFull(false);
    setLoadFullErrored(false);
    dataRef.current = null;
    inFlightRef.current = false;
    fullInFlightRef.current = false;
  }, [tabId]);

  // Lazy fetch gated on `active`.
  useEffect(() => {
    if (!tabId) return;
    if (!active) return;
    if (dataRef.current !== null) return;
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    setLoading(true);
    setError(null);
    let cancelled = false;
    GetPlainText(tabId)
      .then((result: unknown) => {
        if (cancelled) return;
        const doc = result as PlainTextDocumentData;
        dataRef.current = doc;
        setData(doc);
        setLoading(false);
        inFlightRef.current = false;
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(extractErrorMessage(err));
        setLoading(false);
        inFlightRef.current = false;
      });
    return () => {
      // Clear inFlightRef in the cleanup too: the .then/.catch branches return
      // early when cancelled, so without this the slot stays "in flight"
      // forever and a re-activation under the same tabId would be blocked.
      cancelled = true;
      inFlightRef.current = false;
    };
  }, [tabId, active]);

  // 200ms loading debounce.
  useEffect(() => {
    if (!loading) {
      setShowLoading(false);
      return;
    }
    const timer = setTimeout(() => setShowLoading(true), 200);
    return () => clearTimeout(timer);
  }, [loading]);

  // Scroll to top whenever `active` transitions false -> true (AC6).
  // The effect also fires on mount; the container ref might not be attached
  // yet on the first render pass, so we guard.
  useEffect(() => {
    if (!active) return;
    if (scrollRef.current) {
      scrollRef.current.scrollTop = 0;
    }
    setScrollTop(0);
  }, [active]);

  // Track viewport height for virtualization. ResizeObserver re-fires on every
  // container resize (Allotment splitter drag, window resize) so the visible
  // row count stays correct -- without it, dragging the panel wider leaves blank
  // space below the last rendered row.
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

  /** Memoized line split: CRLF / CR / LF all collapse to one row each (AC6). */
  const lines = useMemo(() => {
    if (!data) return [] as string[];
    return (data.content ?? '').split(/\r\n?|\n/);
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

  // Story 9-12: click handler for "Load all" / "Retry". Imperative path
  // (cannot use the effect-cleanup pattern); guards against re-entrancy and
  // stale-fetch via fullInFlightRef + tabIdRef.
  const handleLoadFull = useCallback(() => {
    if (fullInFlightRef.current) return;
    fullInFlightRef.current = true;
    setLoadingFull(true);
    // Do NOT clear loadFullErrored here: AC8 mandates that the Retry button
    // keeps the retry testid while a retry-click fetch is in flight. The flag
    // only clears on success (via banner unmount) or on tabId change.
    const tabIdAtFetch = tabId;
    GetPlainTextFull(tabId)
      .then((result: unknown) => {
        // Stale-fetch guard MUST fire before any state writes / dispatch.
        if (tabIdAtFetch !== tabIdRef.current) return;
        const doc = result as PlainTextDocumentData;
        fullInFlightRef.current = false;
        setLoadingFull(false);
        setLoadFullErrored(false);
        dataRef.current = doc;
        setData(doc);
      })
      .catch((err: unknown) => {
        if (tabIdAtFetch !== tabIdRef.current) return;
        fullInFlightRef.current = false;
        setLoadingFull(false);
        setLoadFullErrored(true);
        dispatch({
          type: 'SET_DOCUMENT_ERROR',
          payload: { message: extractErrorMessage(err) },
        });
      });
  }, [tabId, dispatch]);

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

  if (error) {
    return (
      <div className="p-3 text-error text-sm" data-testid="plain-text-error">
        {error}
      </div>
    );
  }

  if (showLoading && !data) {
    return (
      <div className="p-3 text-text-muted text-sm" data-testid="plain-text-loading">
        Loading plain text...
      </div>
    );
  }

  if (!data) {
    return <div className="h-full" data-testid="plain-text-empty-initial" />;
  }

  const showSizeInLabel = data.totalBytes >= SIZE_LABEL_THRESHOLD;
  const isRetry = loadFullErrored;
  // Visual label: while in flight the button shows "Loading..." regardless of
  // whether it started as Load-all or Retry. Otherwise show the appropriate
  // resting label.
  let actionLabel: string;
  if (loadingFull) {
    actionLabel = 'Loading...';
  } else if (isRetry) {
    actionLabel = 'Retry';
  } else {
    actionLabel = showSizeInLabel
      ? `Load all (${formatBytes(data.totalBytes)})`
      : 'Load all';
  }
  // Color tokens: warning at/above the threshold, neutral below.
  const colorClasses = showSizeInLabel
    ? 'text-warning border-warning'
    : 'text-text-primary border-border';
  const buttonClass = `bg-bg border rounded px-3 py-1 text-sm hover:bg-surface-hover cursor-pointer disabled:cursor-not-allowed disabled:opacity-60 ${colorClasses}`;
  // testid: retry variant when the previous fetch errored, even while a retry
  // fetch is in flight (per AC8 "it does NOT flip back to Load all mid-flight").
  const actionTestId = isRetry
    ? 'plain-text-load-full-retry'
    : 'plain-text-load-full-button';

  return (
    <div
      className="h-full overflow-auto font-mono text-sm bg-bg"
      data-testid="plain-text-scroll"
      ref={scrollRef}
      onScroll={handleScroll}
    >
      {data.truncated && (
        <div
          className="sticky top-0 z-10 px-3 py-1.5 text-warning bg-surface-hover border-b border-border text-xs flex justify-between items-center"
          data-testid="plain-text-truncated-banner"
        >
          <span>
            Showing first {formatBytes(data.capBytes)} of {formatBytes(data.totalBytes)}.
          </span>
          <button
            type="button"
            data-testid={actionTestId}
            className={buttonClass}
            disabled={loadingFull}
            aria-busy={loadingFull ? 'true' : undefined}
            aria-label={actionLabel}
            onClick={handleLoadFull}
          >
            {actionLabel}
          </button>
        </div>
      )}
      <div style={{ position: 'relative', height: totalHeight }}>
        <div
          className="flex"
          style={{
            position: 'absolute',
            top: firstVisible * ROW_HEIGHT,
            left: 0,
            right: 0,
          }}
        >
          {/* Gutter column */}
          <div
            className="flex-shrink-0 select-none text-right text-text-muted pr-2 border-r border-border"
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
  );
}

export default PlainTextView;
