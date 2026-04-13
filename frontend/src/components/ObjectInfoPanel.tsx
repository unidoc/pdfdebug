/**
 * @file Bottom-left panel showing properties of the currently selected
 * PDF object (dict entries, array elements, scalar value, or stream metadata).
 */
import { useState, useEffect, useCallback } from 'react';
import { GetObjectDetail } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppState, useAppDispatch } from '../hooks/useDocumentState';
import {
  type ObjectDetailData,
  DictView,
  ArrayView,
  ScalarView,
  StreamMetadata,
} from './DetailShared';

/**
 * Compact property inspector for the selected tree node.
 * Fetches object detail from the backend whenever the selection changes.
 */
export function ObjectInfoPanel() {
  const { tabs, activeTabId } = useAppState();
  const dispatch = useAppDispatch();
  const activeTab = tabs.find((t) => t.tabId === activeTabId);
  const selectedNodeId = activeTab?.selectedNodeId ?? null;

  const [detail, setDetail] = useState<ObjectDetailData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!activeTabId || !selectedNodeId) {
      setDetail(null);
      setError(null);
      setLoading(false);
      return;
    }
    setDetail(null);
    setError(null);
    setLoading(true);
    // Stale-fetch guard: if the selection changes before the promise
    // resolves, the old response is discarded.
    let cancelled = false;
    GetObjectDetail(activeTabId, selectedNodeId)
      .then((result: unknown) => {
        if (!cancelled) {
          setDetail(result as ObjectDetailData);
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

  /** Navigate the tree to the referenced PDF object. */
  const handleReferenceClick = useCallback((refTarget: string) => {
    if (refTarget) {
      dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: refTarget } });
    }
  }, [dispatch]);

  return (
    <div className="h-full flex flex-col" data-testid="object-info-panel">
      {!selectedNodeId && (
        <div
          className="h-full flex items-center justify-center text-text-muted text-sm"
          data-testid="object-info-empty"
        >
          Select a node to view properties
        </div>
      )}
      {selectedNodeId && loading && !detail && !error && (
        <div
          className="h-full flex items-center justify-center text-text-muted text-sm"
          data-testid="object-info-loading"
        >
          Loading...
        </div>
      )}
      {selectedNodeId && error && (
        <div
          className="p-3 text-error text-sm"
          data-testid="object-info-error"
        >
          {error}
        </div>
      )}
      {selectedNodeId && detail && (
        <>
          <div className="px-3 py-1.5 border-b border-border flex-shrink-0 flex items-center justify-between">
            <span className="text-sm font-medium text-text-secondary">Object Properties</span>
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
              <DictView properties={detail.properties} onReferenceClick={handleReferenceClick} />
              {detail.streamInfo && <StreamMetadata info={detail.streamInfo} />}
            </>
          )}
        </>
      )}
    </div>
  );
}
