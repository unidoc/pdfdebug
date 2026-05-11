/**
 * @file Right-hand detail panel. Shows the full object detail for the
 * selected tree node, with a contextual header label.
 */
import { useState, useEffect, useCallback, useMemo, memo } from 'react';
import { GetObjectDetail, GetContentStream, GetImageData, GetReverseRefs } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { ContentStreamData, ImageData as PdfImageData } from '../../bindings/unidoc-pdf-debugger/internal/pdfcore/models.js';
import { useAppState, useAppDispatch } from '../hooks/useDocumentState';
import {
  type ObjectDetailData,
  DictView,
  ArrayView,
  ScalarView,
} from './DetailShared';
import { ContentStreamViewer, type StreamViewMode } from './ContentStreamViewer';
import { ImagePreview } from './ImagePreview';
import { ReverseRefsSection, type ReverseRefEntry } from './ReverseRefsSection';

/**
 * Matches indirect-object node IDs exactly (e.g. "obj:0:5"). Inline nodes
 * carry trailing dict/arr segments after the obj prefix and must be excluded
 * from the Referenced by section per Task 7.1.
 */
const INDIRECT_NODE_RE = /^obj:\d+:\d+$/;

/**
 * Lowercase substring used to detect the index-unavailable sentinel coming
 * back from Wails error rejection. Matches the canonical
 * `ErrReverseRefIndexUnavailable` wording ("reverse-ref index unavailable").
 */
const REV_REFS_UNAVAILABLE_MARKER = 'reverse-ref index unavailable';

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
  const selectedNodeIconHint = activeTab?.selectedNodeIconHint ?? null;
  const navHistory = activeTab?.navHistory ?? [];
  const navHistoryIndex = activeTab?.navHistoryIndex ?? -1;
  const canGoBack = navHistoryIndex > 0;
  const canGoForward = navHistoryIndex < navHistory.length - 1;
  const isMac = useMemo(() => navigator.platform.startsWith('Mac'), []);

  const [detail, setDetail] = useState<ObjectDetailData | null>(null);
  const [detailTabId, setDetailTabId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [, setLoading] = useState(false);
  const [contentStream, setContentStream] = useState<ContentStreamData | null>(null);
  const [contentStreamLoading, setContentStreamLoading] = useState(false);
  const [showContentStreamLoading, setShowContentStreamLoading] = useState(false);
  const [streamViewMode, setStreamViewMode] = useState<StreamViewMode>('formatted');
  const [imageData, setImageData] = useState<PdfImageData | null>(null);
  const [imageLoading, setImageLoading] = useState(false);
  const [showImageLoading, setShowImageLoading] = useState(false);

  // Story 9-10: reverse-refs are fetched per-selection. The backend has the
  // document-level index so the call is O(1); no client cache is needed.
  // reverseRefsLoaded gates the section render until the fetch resolves so an
  // in-flight selection does not flash the orphan empty state for objects that
  // actually have inbound refs.
  const [reverseRefs, setReverseRefs] = useState<ReverseRefEntry[]>([]);
  const [reverseRefsUnavailable, setReverseRefsUnavailable] = useState(false);
  const [reverseRefsVisible, setReverseRefsVisible] = useState(false);
  const [reverseRefsLoaded, setReverseRefsLoaded] = useState(false);

  useEffect(() => {
    if (!activeTabId || !selectedNodeId) {
      setDetail(null);
      setError(null);
      setLoading(false);
      setContentStream(null);
      setContentStreamLoading(false);
      setShowContentStreamLoading(false);
      setImageData(null);
      setImageLoading(false);
      setShowImageLoading(false);
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

  // Fetch content stream when detail resolves to a non-image stream node.
  // Uses detailTabId (the tab that produced this detail) to avoid a
  // mismatched fetch when activeTabId changes before detail updates.
  useEffect(() => {
    if (!detail || !detailTabId) return;
    if (detail.type !== 'stream' || selectedNodeIconHint === 'image') {
      setContentStream(null);
      setContentStreamLoading(false);
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
          setContentStream(new ContentStreamData({ nodeId: detail.nodeId, raw: '', tokenized: [], formatted: [], error: String(err) }));
          setContentStreamLoading(false);
        }
      });
    return () => { cancelled = true; };
  }, [detail, detailTabId, selectedNodeIconHint]);

  // Debounce content stream loading indicator by 200ms
  useEffect(() => {
    if (!contentStreamLoading) {
      setShowContentStreamLoading(false);
      return;
    }
    const timer = setTimeout(() => setShowContentStreamLoading(true), 200);
    return () => clearTimeout(timer);
  }, [contentStreamLoading]);

  // Fetch image data when detail resolves to a stream node with image iconHint.
  useEffect(() => {
    if (selectedNodeIconHint !== 'image' || !detail || detail.type !== 'stream' || !detailTabId) {
      setImageData(null);
      setImageLoading(false);
      return;
    }
    setImageData(null);
    setImageLoading(true);
    let cancelled = false;
    GetImageData(detailTabId, detail.nodeId)
      .then((result: unknown) => {
        if (!cancelled) {
          setImageData(result as PdfImageData);
          setImageLoading(false);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          const msg = err instanceof Error ? err.message : typeof err === 'string' ? err : 'Unknown error';
          setImageData(new PdfImageData({
            nodeId: detail.nodeId,
            error: msg,
          }));
          setImageLoading(false);
        }
      });
    return () => { cancelled = true; };
  }, [detail, detailTabId, selectedNodeIconHint]);

  // Debounce image loading indicator by 200ms
  useEffect(() => {
    if (!imageLoading) {
      setShowImageLoading(false);
      return;
    }
    const timer = setTimeout(() => setShowImageLoading(true), 200);
    return () => clearTimeout(timer);
  }, [imageLoading]);

  // Story 9-10: fetch reverse refs for indirect-object selections only. The
  // catalog (nodeId='root') is also treated as indirect because in real PDFs
  // it lives in the indirect-object graph and AC#10 requires the section to
  // render the "Document root..." empty state for it. Inline-value nodes
  // never mount the section. Stale-fetch guard matches existing patterns.
  useEffect(() => {
    const isIndirect = !!selectedNodeId &&
      (INDIRECT_NODE_RE.test(selectedNodeId) || selectedNodeId === 'root');
    if (!activeTabId || !isIndirect) {
      setReverseRefs([]);
      setReverseRefsUnavailable(false);
      setReverseRefsVisible(false);
      setReverseRefsLoaded(false);
      return;
    }
    // Reset transient state on every selection change so a previous selection's
    // banner doesn't bleed into the new selection while its fetch is in flight.
    // reverseRefsLoaded stays false until the fetch resolves so the section
    // does not flash the orphan empty state for non-orphan objects.
    setReverseRefs([]);
    setReverseRefsUnavailable(false);
    setReverseRefsVisible(true);
    setReverseRefsLoaded(false);
    let cancelled = false;
    GetReverseRefs(activeTabId, selectedNodeId)
      .then((result: unknown) => {
        if (cancelled) return;
        const list = (Array.isArray(result) ? result : []) as ReverseRefEntry[];
        setReverseRefs(list);
        setReverseRefsUnavailable(false);
        setReverseRefsLoaded(true);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : typeof err === 'string' ? err : '';
        if (msg.toLowerCase().includes(REV_REFS_UNAVAILABLE_MARKER)) {
          // AC#6 failure mode: surface the unavailable banner.
          setReverseRefs([]);
          setReverseRefsUnavailable(true);
          setReverseRefsLoaded(true);
        } else {
          // Task 7.3 case (b): hide the section silently and log.
          setReverseRefs([]);
          setReverseRefsUnavailable(false);
          setReverseRefsVisible(false);
          setReverseRefsLoaded(false);
          // eslint-disable-next-line no-console
          console.warn('GetReverseRefs failed:', err);
        }
      });
    return () => { cancelled = true; };
  }, [activeTabId, selectedNodeId]);

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

  const typeLabel = detail
    ? (selectedNodeIconHint === 'image' ? 'Image Preview' : (TYPE_LABEL_MAP[detail.type] ?? 'Details'))
    : null;
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
            {detail.type === 'stream' && selectedNodeIconHint === 'image' && (
              <>
                {imageData && (
                  <ImagePreview
                    base64={imageData.base64}
                    mimeType={imageData.mimeType}
                    width={imageData.width}
                    height={imageData.height}
                    colorSpace={imageData.colorSpace}
                    bitsPerComponent={imageData.bitsPerComponent}
                    filter={imageData.filter}
                    warning={imageData.warning}
                    error={imageData.error}
                  />
                )}
                {showImageLoading && !imageData && (
                  <div className="p-3 text-text-muted text-sm" data-testid="image-loading">
                    Loading image...
                  </div>
                )}
              </>
            )}
            {detail.type === 'stream' && selectedNodeIconHint !== 'image' && (
              <>
                {contentStream && (
                  <ContentStreamViewer
                    raw={contentStream.raw}
                    formatted={contentStream.formatted}
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
            {reverseRefsVisible && reverseRefsLoaded && selectedNodeId && (
              <ReverseRefsSection
                key={selectedNodeId}
                entries={reverseRefs}
                selectedIconHint={selectedNodeIconHint}
                indexUnavailable={reverseRefsUnavailable}
              />
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
