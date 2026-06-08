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

/**
 * Document-level XREF table view. Lazy-fetches on first activation; data is
 * cached in component state for the lifetime of the document (tabId change
 * resets state).
 */
export function XRefTableView({ tabId, active: _active, onNavigate, onLoaded }: XRefTableViewProps) {
  const [data, setData] = useState<XRefTableData | null>(null);
  const [loading, setLoading] = useState(false);
  const [showLoading, setShowLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Cache flags as refs so the fetch effect only re-runs when tabId / active
  // change. Including data/loading in deps creates a stale-fetch race where
  // the cleanup from the loading-state re-render cancels the in-flight call.
  const dataRef = useRef<XRefTableData | null>(null);
  const inFlightRef = useRef(false);
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

  // Eager fetch on tabId change so the parent can render the "XREF (N)"
  // tab label without waiting for first activation. Stale-fetch guard via
  // `cancelled`. The `active` prop is intentionally NOT a gate -- the xref is
  // cheap to extract from pdfcpu's already-parsed state.
  useEffect(() => {
    if (!tabId) return;
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
      // Clear inFlightRef in the cleanup too: the .then/.catch branches return
      // early when cancelled, so without this the slot stays "in flight"
      // forever and a re-activation under the same tabId would be blocked.
      cancelled = true;
      inFlightRef.current = false;
    };
  }, [tabId]);

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
   * Enter / Space activate non-free rows.
   */
  const handleRowKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTableRowElement>, entry: XRefEntryData) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        const next = e.currentTarget.nextElementSibling as HTMLTableRowElement | null;
        if (next) next.focus();
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        const prev = e.currentTarget.previousElementSibling as HTMLTableRowElement | null;
        if (prev) prev.focus();
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

  return (
    <div className="h-full overflow-auto" data-testid="xref-table-container">
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
          {(data.entries ?? []).map((entry) => {
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
                tabIndex={0}
                aria-disabled={isFree ? 'true' : undefined}
                data-testid={`xref-row-${entry.objNum}`}
                onClick={() => handleRowClick(entry)}
                onKeyDown={(e) => handleRowKeyDown(e, entry)}
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
        </tbody>
      </table>
    </div>
  );
}

export default XRefTableView;
