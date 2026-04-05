import { useState, useEffect, useCallback, memo } from 'react';
import { GetObjectDetail } from '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppState, useAppDispatch } from '../hooks/useDocumentState';
import {
  type ObjectDetailData,
  DictView,
  ArrayView,
  ScalarView,
  StreamMetadata,
} from './DetailShared';

const TYPE_LABEL_MAP: Record<string, string> = {
  dict: 'Properties',
  array: 'Array',
  stream: 'Stream',
  scalar: 'Value',
};

function DetailPanelInner() {
  const { tabs, activeTabId } = useAppState();
  const dispatch = useAppDispatch();
  const activeTab = tabs.find((t) => t.tabId === activeTabId);
  const selectedNodeId = activeTab?.selectedNodeId ?? null;
  const selectedNodeLabel = activeTab?.selectedNodeLabel ?? null;
  const selectedNodeRawKey = activeTab?.selectedNodeRawKey ?? null;

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
              <span className="text-sm font-medium text-text-secondary">{headerLabel}</span>
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
          </div>
        )}
      </div>
    </div>
  );
}

export const DetailPanel = memo(DetailPanelInner);
