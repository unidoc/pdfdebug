/**
 * @file Plain Text view -- document-level Latin-1-decoded file bytes with a
 * 1-based line-number gutter. Story 9-11. Lines are split on /\r\n?|\n/ so
 * CRLF / lone CR / lone LF all collapse to one logical row.
 *
 * The view uses hand-rolled viewport virtualization: a tall spacer fixes the
 * total scroll height, but only the visible slice of rows is rendered. This
 * keeps the DOM small (under 200 rows) even for the 5MB truncation cap
 * (~50,000-100,000 logical lines).
 */
import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { GetPlainText } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';

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
  const scrollRef = useRef<HTMLDivElement | null>(null);
  // Cache flags as refs so the fetch effect only re-runs when tabId / active
  // change. See XRefTableView for the rationale.
  const dataRef = useRef<PlainTextDocumentData | null>(null);
  const inFlightRef = useRef(false);

  // Reset state on document change (AC17 / Task 7.x parallel to XRefTableView).
  useEffect(() => {
    setData(null);
    setError(null);
    setLoading(false);
    setShowLoading(false);
    setScrollTop(0);
    dataRef.current = null;
    inFlightRef.current = false;
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

  return (
    <div
      className="h-full overflow-auto font-mono text-sm bg-bg"
      data-testid="plain-text-scroll"
      ref={scrollRef}
      onScroll={handleScroll}
    >
      {data.truncated && (
        <div
          className="sticky top-0 z-10 px-3 py-1.5 text-warning bg-surface-hover border-b border-border text-xs"
          data-testid="plain-text-truncated-banner"
        >
          Showing first {data.capBytes.toLocaleString()} of {data.totalBytes.toLocaleString()} bytes (truncated).
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
