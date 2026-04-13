/**
 * @file Right-hand detail panel. Shows the full object detail for the
 * selected tree node, with a contextual header label.
 */
import { useState, useEffect, useCallback, useMemo, memo } from 'react';
import { GetObjectDetail, GetContentStream } from '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js';
import { ContentStreamData } from '../../bindings/unipdf-debugger/internal/pdfcore/models.js';
import { useAppState, useAppDispatch } from '../hooks/useDocumentState';
import {
  type ObjectDetailData,
  DictView,
  ArrayView,
  ScalarView,
} from './DetailShared';
import { ContentStreamViewer, type StreamViewMode } from './ContentStreamViewer';

/** Maps PDF object type to a human-readable header label. */
const TYPE_LABEL_MAP: Record<string, string> = {
  dict: 'Properties',
  array: 'Array',
  stream: 'Content Stream',
  scalar: 'Value',
};

/** Inner (un-memoized) detail panel that fetches and renders object detail. */
function DetailPanelInner() {
  const { tabs, activeTabId } = useAppState();
  const dispatch = useAppDispatch();
  const activeTab = tabs.find((t) => t.tabId === activeTabId);
  const selectedNodeId = activeTab?.selectedNodeId ?? null;
  const selectedNodeLabel = activeTab?.selectedNodeLabel ?? null;
  const selectedNodeRawKey = activeTab?.selectedNodeRawKey ?? null;
  const navHistory = activeTab?.navHistory ?? [];
  const navHistoryIndex = activeTab?.navHistoryIndex ?? -1;
  const canGoBack = navHistoryIndex > 0;
  const canGoForward = navHistoryIndex < navHistory.length - 1;
  const isMac = useMemo(() => navigator.platform.startsWith('Mac'), []);

  const [detail, setDetail] = useState<ObjectDetailData | null>(null);
  const [detailTabId, setDetailTabId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [contentStream, setContentStream] = useState<ContentStreamData | null>(null);
  const [contentStreamLoading, setContentStreamLoading] = useState(false);
  const [showContentStreamLoading, setShowContentStreamLoading] = useState(false);
  const [streamViewMode, setStreamViewMode] = useState<StreamViewMode>('formatted');

  useEffect(() => {
    if (!activeTabId || !selectedNodeId) {
      setDetail(null);
      setError(null);
      setLoading(false);
      setContentStream(null);
      setContentStreamLoading(false);
      setShowContentStreamLoading(false);
      return;
    }
    // Keep previous detail/contentStream visible until the new fetch resolves
    // to avoid a flash of empty or error state during tab switches.
    setError(null);
    setLoading(true);
    // Stale-fetch guard: discard response if selection changed before resolve
    let cancelled = false;
    GetObjectDetail(activeTabId, selectedNodeId)
      .then((result: unknown) => {
        if (!cancelled) {
          setDetail(result as ObjectDetailData);
          setDetailTabId(activeTabId);
          setLoading(false);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(String(err));
          setLoading(false);
        }
      });
    return () => { cancelled = true; };
  }, [activeTabId, selectedNodeId]);

  // Fetch content stream when detail resolves to a stream node.
  // Uses detailTabId (the tab that produced this detail) to avoid a
  // mismatched fetch when activeTabId changes before detail updates.
  useEffect(() => {
    if (!detail || !detailTabId) return;
    if (detail.type !== 'stream') {
      setContentStream(null);
      return;
    }
    setContentStreamLoading(true);
    let cancelled = false;
    GetContentStream(detailTabId, detail.nodeId)
      .then((result: unknown) => {
        if (!cancelled) {
          setContentStream(result as ContentStreamData);
          setContentStreamLoading(false);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setContentStream(new ContentStreamData({ nodeId: detail.nodeId, raw: '', tokenized: [], error: String(err) }));
          setContentStreamLoading(false);
        }
      });
    return () => { cancelled = true; };
  }, [detail, detailTabId]);

  // Debounce loading indicator by 200ms
  useEffect(() => {
    if (!contentStreamLoading) {
      setShowContentStreamLoading(false);
      return;
    }
    const timer = setTimeout(() => setShowContentStreamLoading(true), 200);
    return () => clearTimeout(timer);
  }, [contentStreamLoading]);

  // Keyboard shortcuts: Cmd+[ / Ctrl+[ for back, Cmd+] / Ctrl+] for forward
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod) return;
      if (e.key === '[') {
        e.preventDefault();
        if (canGoBack) dispatch({ type: 'NAVIGATE_BACK' });
      } else if (e.key === ']') {
        e.preventDefault();
        if (canGoForward) dispatch({ type: 'NAVIGATE_FORWARD' });
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [canGoBack, canGoForward, dispatch]);

  /** Navigate the tree to the referenced PDF object. */
  const handleReferenceClick = useCallback((refTarget: string) => {
    if (refTarget) {
      dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: refTarget } });
    }
  }, [dispatch]);

  const typeLabel = detail ? (TYPE_LABEL_MAP[detail.type] ?? 'Details') : null;
  const contextSuffix = selectedNodeRawKey || selectedNodeLabel;
  const headerLabel = typeLabel && contextSuffix
    ? `${typeLabel} - ${contextSuffix}`
    : typeLabel;

  return (
    <div className="h-full flex flex-col" data-testid="detail-panel">
      <div className="h-full" aria-live="polite" data-testid="detail-panel-content">
        {!selectedNodeId && (
          <div
            className="h-full flex items-center justify-center text-text-muted text-sm"
            data-testid="detail-panel-empty"
          >
            Select a node in the tree to view details
          </div>
        )}
        {selectedNodeId && error && (
          <div
            className="p-3 text-error text-sm"
            data-testid="detail-panel-error"
          >
            {error}
          </div>
        )}
        {selectedNodeId && detail && (
          <div className="h-full flex flex-col">
            <div
              className="px-3 py-1.5 border-b border-border flex-shrink-0 flex items-center justify-between"
              data-testid="detail-panel-header"
            >
              <div className="flex items-center gap-2">
                <div className="flex items-center gap-0.5">
                  <button
                    onClick={() => dispatch({ type: 'NAVIGATE_BACK' })}
                    disabled={!canGoBack}
                    title={`Back (${isMac ? 'Cmd+[' : 'Ctrl+['})`}
                    className={`p-0.5 rounded text-sm ${canGoBack ? 'text-text-secondary hover:bg-hover cursor-pointer' : 'text-text-muted/40 cursor-not-allowed'}`}
                    data-testid="nav-back-button"
                    aria-label="Navigate back"
                  >
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M10 3L5 8l5 5"/></svg>
                  </button>
                  <button
                    onClick={() => dispatch({ type: 'NAVIGATE_FORWARD' })}
                    disabled={!canGoForward}
                    title={`Forward (${isMac ? 'Cmd+]' : 'Ctrl+]'})`}
                    className={`p-0.5 rounded text-sm ${canGoForward ? 'text-text-secondary hover:bg-hover cursor-pointer' : 'text-text-muted/40 cursor-not-allowed'}`}
                    data-testid="nav-forward-button"
                    aria-label="Navigate forward"
                  >
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M6 3l5 5-5 5"/></svg>
                  </button>
                </div>
                <span className="text-sm font-medium text-text-secondary">{headerLabel}</span>
              </div>
              {detail.objectRef && (
                <span className="text-xs text-text-muted font-mono">{detail.objectRef}</span>
              )}
            </div>
            {detail.type === 'dict' && <DictView properties={detail.properties} onReferenceClick={handleReferenceClick} />}
            {detail.type === 'array' && <ArrayView elements={detail.elements} onReferenceClick={handleReferenceClick} />}
            {detail.type === 'scalar' && (detail.scalarValue
              ? <ScalarView value={detail.scalarValue} onReferenceClick={handleReferenceClick} />
              : <div className="text-text-muted text-sm p-3">No value</div>
            )}
            {detail.type === 'stream' && (
              <>
                {contentStream && (
                  <ContentStreamViewer
                    raw={contentStream.raw}
                    tokenized={contentStream.tokenized}
                    error={contentStream.error}
                    viewMode={streamViewMode}
                    onViewModeChange={setStreamViewMode}
                  />
                )}
                {showContentStreamLoading && !contentStream && (
                  <div className="p-3 text-text-muted text-sm" data-testid="content-stream-loading">
                    Decoding stream...
                  </div>
                )}
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

/**
 * Memoized detail panel. Re-renders only when props change (none currently),
 * relying on internal hooks for state updates.
 */
export const DetailPanel = memo(DetailPanelInner);
