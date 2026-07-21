/**
 * @file XREF Table view -- document-level cross-reference table. Story 9-11.
 * Renders one row per xref entry sorted by object number. In-use and
 * in-objstm rows navigate via `onNavigate` when clicked or Entered; free
 * rows are focusable but not clickable.
 */
import { useEffect, useRef, useState, useCallback } from 'react';
import { GetXRefTable } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';

/** One row in the XREF table, mirroring `pdfcore.XRefEntry`. */
interface XRefEntryData {
  objNum: number;
  gen: number;
  status: 'in-use' | 'free' | 'in-objstm';
  offset: number;
  hostObjStm: number;
  nodeID: string;
}

/** Full XREF table payload, mirroring `pdfcore.XRefTable`. */
interface XRefTableData {
  tabId: string;
  entries: XRefEntryData[];
}

/** Props for {@link XRefTableView}. */
export interface XRefTableViewProps {
  /** Active document tab ID. Empty string renders the no-document empty state. */
  tabId: string;
  /** True when the XREF tab is active; gates the lazy fetch. */
  active: boolean;
  /** Dispatches NAVIGATE_TO_REF + flips the active detail tab to Object. */
  onNavigate: (nodeId: string) => void;
  /** Fires with the row count once the fetch resolves successfully. */
  onLoaded: (count: number) => void;
}

/** Fixed row height (px) for windowing math; each rendered row is pinned to this
 *  height so the spacer geometry matches the true scroll height. */
const XREF_ROW_HEIGHT = 28;
/** Rows rendered above/below the viewport for smooth scrolling. */
const XREF_OVERSCAN = 12;

/**
 * Document-level XREF table view. Lazy-fetches on first activation; data is
 * cached in component state for the lifetime of the document (tabId change
 * resets state). The row list is viewport-virtualized (only the visible slice
 * is committed to the DOM) so a large xref -- a 750-page PDF can carry ~129k
 * entries -- does not freeze the UI on render. Mirrors the FontMappingTable /
 * PlainTextView windowing pattern.
 */
export function XRefTableView({ tabId, active, onNavigate, onLoaded }: XRefTableViewProps) {
  const [data, setData] = useState<XRefTableData | null>(null);
  const [loading, setLoading] = useState(false);
  const [showLoading, setShowLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // latchedTabId records the tabId the user actually activated the XREF tab on.
  // The fetch gates on the DERIVED `everActive = latchedTabId === tabId` (below),
  // NOT a boolean+reset: a boolean reset in an effect is applied a render late,
  // so the fetch effect could observe a stale `everActive=true` on the first
  // render after a document switch and eagerly fetch a document the user never
  // opened XREF on. The derived value is false in the same render as the switch.
  const [latchedTabId, setLatchedTabId] = useState<string | null>(null);
  // Cache flags as refs so the fetch effect only re-runs when tabId / everActive
  // change. Including data/loading in deps creates a stale-fetch race where
  // the cleanup from the loading-state re-render cancels the in-flight call.
  const dataRef = useRef<XRefTableData | null>(null);
  const inFlightRef = useRef(false);
  // Current tabId mirrored to a ref so the activation latch can read it WITHOUT
  // taking tabId as a dependency -- otherwise the one-render stale `active=true`
  // after a document switch (the parent resets its detail tab a render late)
  // would re-latch the new tabId and trigger an eager fetch.
  const tabIdRef = useRef(tabId);
  tabIdRef.current = tabId;
  const onLoadedRef = useRef(onLoaded);
  useEffect(() => { onLoadedRef.current = onLoaded; }, [onLoaded]);

  // Reset everything when the document changes so the new document never
  // renders the previous document's rows (AC17).
  useEffect(() => {
    setData(null);
    setError(null);
    setLoading(false);
    setShowLoading(false);
    dataRef.current = null;
    inFlightRef.current = false;
  }, [tabId]);

  // Latch the current tabId on a genuine activation (active false->true). Keyed
  // on `active` only (tabId via ref): a document switch does not re-run this, so
  // the transient stale `active=true` that lingers one render after a switch
  // cannot latch the new tabId or trigger an eager fetch. A real re-activation
  // of the XREF tab on the new document latches it and lets the fetch proceed.
  useEffect(() => {
    if (active) setLatchedTabId(tabIdRef.current);
  }, [active]);

  // Derived activation gate: true only for the tabId the user opened XREF on.
  // False in the same render as a tabId change -- no stale-true window.
  const everActive = latchedTabId === tabId;

  // Fetch on FIRST activation of the XREF tab, then cache for the document's
  // lifetime. Gated on `everActive` (not `active`): the payload can be very
  // large (a 750-page, 129k-entry PDF serializes ~12 MB of JSON), and this pane
  // is force-mounted, so an unconditional fetch would JSON.parse ~12 MB + render
  // 129k rows on the main thread on EVERY document open -- freezing the UI while
  // the user is still on the Object tree. Deferring to activation keeps open
  // instant; the "XREF (N)" count label populates on first XREF open instead of
  // on load. Because `active` is not a dependency, toggling the tab off/on
  // mid-flight cannot start a duplicate fetch; `cancelled` only guards a tabId
  // (document) change.
  useEffect(() => {
    if (!tabId) return;
    if (!everActive) return;
    if (dataRef.current !== null) return;
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    setLoading(true);
    setError(null);
    let cancelled = false;
    GetXRefTable(tabId)
      .then((result: unknown) => {
        if (cancelled) return;
        const table = result as XRefTableData;
        dataRef.current = table;
        setData(table);
        setLoading(false);
        inFlightRef.current = false;
        onLoadedRef.current(table?.entries?.length ?? 0);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(extractErrorMessage(err));
        setLoading(false);
        inFlightRef.current = false;
      });
    return () => {
      // Clear inFlightRef so a StrictMode remount or a tabId reset can fetch
      // again; the .then/.catch branches return early when cancelled. An
      // active-toggle never reaches here because `active` is not a dependency.
      cancelled = true;
      inFlightRef.current = false;
    };
  }, [tabId, everActive]);

  // 200ms loading debounce -- mirrors the showContentStreamLoading pattern in
  // DetailPanel.tsx.
  useEffect(() => {
    if (!loading) {
      setShowLoading(false);
      return;
    }
    const timer = setTimeout(() => setShowLoading(true), 200);
    return () => clearTimeout(timer);
  }, [loading]);

  // --- Viewport virtualization (see class doc). The scroll container mounts
  // only once `data` is present (after the early returns below), so the measure
  // and reset effects key on `data` rather than `[]`. ---
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(0);
  // Row index to focus after the window updates. Because virtualization unmounts
  // off-window rows, arrow-key navigation cannot walk DOM siblings; instead it
  // scrolls the target index into view and focuses it here once it is rendered.
  const pendingFocusRef = useRef<number | null>(null);
  const [focusTick, setFocusTick] = useState(0);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    setViewportHeight(el.clientHeight);
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(() => setViewportHeight(el.clientHeight));
    ro.observe(el);
    return () => ro.disconnect();
  }, [data]);

  const handleScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  }, []);

  // Reset the window to the top when the document (row set) changes, so a stale
  // scrollTop never slices past the end of a shorter table.
  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
    setScrollTop(0);
  }, [data]);

  // Focus the pending arrow-key target once the window has updated to include it.
  // Runs on focusTick (a key press) and scrollTop (the window shifted); if the
  // row is not yet rendered, the pending index survives to the next run.
  useEffect(() => {
    const target = pendingFocusRef.current;
    if (target === null) return;
    const entry = dataRef.current?.entries?.[target];
    if (!entry) {
      pendingFocusRef.current = null;
      return;
    }
    const row = scrollRef.current?.querySelector<HTMLElement>(`[data-testid="xref-row-${entry.objNum}"]`);
    if (row) {
      row.focus();
      pendingFocusRef.current = null;
    }
  }, [focusTick, scrollTop]);

  /** Click handler: free rows are no-ops; in-use / in-objstm dispatch navigation. */
  const handleRowClick = useCallback(
    (entry: XRefEntryData) => {
      if (entry.status === 'free') return;
      if (!entry.nodeID) return;
      onNavigate(entry.nodeID);
    },
    [onNavigate]
  );

  /**
   * Keyboard handler per AC4: ArrowDown / ArrowUp move row focus (no wrap);
   * Enter / Space activate non-free rows. `index` is the row's absolute position
   * in the full entry list. Because the table is virtualized, moving focus walks
   * the index (not DOM siblings, which would hit spacer rows or unmounted rows):
   * it scrolls the target into view and defers the focus to the effect above.
   */
  const handleRowKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTableRowElement>, index: number, entry: XRefEntryData) => {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        const total = dataRef.current?.entries?.length ?? 0;
        const target = e.key === 'ArrowDown' ? index + 1 : index - 1;
        if (target < 0 || target >= total) return; // no wrap
        const el = scrollRef.current;
        if (el) {
          // Ensure the target row is inside the scroll viewport so it renders in
          // the next window; the effect then focuses it.
          const rowTop = target * XREF_ROW_HEIGHT;
          const rowBottom = rowTop + XREF_ROW_HEIGHT;
          if (rowTop < el.scrollTop) el.scrollTop = rowTop;
          else if (rowBottom > el.scrollTop + el.clientHeight) el.scrollTop = rowBottom - el.clientHeight;
          setScrollTop(el.scrollTop);
        }
        pendingFocusRef.current = target;
        setFocusTick((t) => t + 1);
        return;
      }
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        handleRowClick(entry);
      }
    },
    [handleRowClick]
  );

  // No-document branch (defense-in-depth -- DetailPanel does not mount when
  // activeTabId is null, but a unit test can render with empty tabId).
  if (!tabId) {
    return (
      <div
        className="h-full flex items-center justify-center text-text-muted text-sm"
        data-testid="xref-empty"
      >
        No document open
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-3 text-error text-sm" data-testid="xref-error">
        {error}
      </div>
    );
  }

  if (showLoading && !data) {
    return (
      <div className="p-3 text-text-muted text-sm" data-testid="xref-loading">
        Loading xref table...
      </div>
    );
  }

  if (!data) {
    return <div className="h-full" data-testid="xref-empty-initial" />;
  }

  const entries = data.entries ?? [];
  const totalRows = entries.length;
  const firstVisible = Math.max(0, Math.floor(scrollTop / XREF_ROW_HEIGHT) - XREF_OVERSCAN);
  // A zero clientHeight (jsdom / pre-measure) falls back to a bounded default so
  // the window stays small rather than rendering every row.
  const visibleCount = Math.ceil((viewportHeight || 320) / XREF_ROW_HEIGHT) + XREF_OVERSCAN * 2;
  const lastVisible = Math.min(totalRows, firstVisible + visibleCount);
  const rowsToRender = entries.slice(firstVisible, lastVisible);

  return (
    <div
      className="h-full overflow-auto"
      data-testid="xref-table-container"
      ref={scrollRef}
      onScroll={handleScroll}
    >
      <table className="w-full text-xs font-mono border-collapse">
        {/* `position: sticky` on <thead> is unreliable on WebKit2GTK and older
            Safari/WebKit; apply sticky on each <th> so the header sticks on every
            target. The shared border-b also lives on the cells so the underline
            sticks with the row. */}
        <thead>
          <tr>
            <th className="sticky top-0 bg-surface border-b border-border px-2 py-1 text-right text-text-secondary font-medium">Obj #</th>
            <th className="sticky top-0 bg-surface border-b border-border px-2 py-1 text-right text-text-secondary font-medium">Gen</th>
            <th className="sticky top-0 bg-surface border-b border-border px-2 py-1 text-right text-text-secondary font-medium">Offset</th>
            <th className="sticky top-0 bg-surface border-b border-border px-2 py-1 text-left text-text-secondary font-medium">Status</th>
            <th className="sticky top-0 bg-surface border-b border-border px-2 py-1 text-right text-text-secondary font-medium">Host ObjStm</th>
          </tr>
        </thead>
        <tbody>
          {/* Top spacer reserves the scroll height of the rows above the window. */}
          {firstVisible > 0 && (
            <tr aria-hidden="true" style={{ height: firstVisible * XREF_ROW_HEIGHT }}>
              <td colSpan={5} />
            </tr>
          )}
          {rowsToRender.map((entry, i) => {
            const index = firstVisible + i;
            const isFree = entry.status === 'free';
            const isInObjStm = entry.status === 'in-objstm';
            const rowClass = [
              'border-b border-border',
              isFree
                ? 'opacity-60 cursor-default'
                : 'cursor-pointer hover:bg-surface-hover',
            ].join(' ');
            const pillClass = isFree
              ? 'text-text-muted bg-surface-hover px-1.5 py-0.5 rounded'
              : isInObjStm
                ? 'text-type-name bg-surface-hover px-1.5 py-0.5 rounded'
                : 'text-text px-1.5 py-0.5';
            return (
              <tr
                key={`${entry.objNum}:${entry.gen}`}
                className={rowClass}
                style={{ height: XREF_ROW_HEIGHT }}
                tabIndex={0}
                aria-disabled={isFree ? 'true' : undefined}
                data-testid={`xref-row-${entry.objNum}`}
                onClick={() => handleRowClick(entry)}
                onKeyDown={(e) => handleRowKeyDown(e, index, entry)}
              >
                <td className="px-2 py-1 text-right text-text" data-testid={`xref-row-objnum-${entry.objNum}`}>
                  {entry.objNum}
                </td>
                <td className="px-2 py-1 text-right text-text">{entry.gen}</td>
                <td
                  className="px-2 py-1 text-right text-text"
                  data-testid={`xref-row-offset-${entry.objNum}`}
                >
                  {entry.status === 'in-use' && entry.offset >= 0 ? entry.offset : '-'}
                </td>
                <td className="px-2 py-1 text-left">
                  <span className={pillClass}>{entry.status}</span>
                </td>
                <td
                  className="px-2 py-1 text-right text-text"
                  data-testid={`xref-row-host-${entry.objNum}`}
                >
                  {isInObjStm && entry.hostObjStm > 0 ? entry.hostObjStm : '-'}
                </td>
              </tr>
            );
          })}
          {/* Bottom spacer reserves the scroll height of the rows below. */}
          {lastVisible < totalRows && (
            <tr aria-hidden="true" style={{ height: (totalRows - lastVisible) * XREF_ROW_HEIGHT }}>
              <td colSpan={5} />
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

export default XRefTableView;
