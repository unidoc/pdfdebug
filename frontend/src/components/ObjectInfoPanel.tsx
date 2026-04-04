import { useState, useEffect } from 'react';
import { GetObjectDetail } from '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppState } from '../hooks/useDocumentState';
import {
  type ObjectDetailData,
  DictView,
  ArrayView,
  ScalarView,
  StreamMetadata,
} from './DetailShared';

export function ObjectInfoPanel() {
  const { tabs, activeTabId } = useAppState();
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
          {detail.type === 'dict' && <DictView properties={detail.properties} />}
          {detail.type === 'array' && <ArrayView elements={detail.elements} />}
          {detail.type === 'scalar' && (detail.scalarValue
            ? <ScalarView value={detail.scalarValue} />
            : <div className="text-text-muted text-sm p-3">No value</div>
          )}
          {detail.type === 'stream' && (
            <>
              <DictView properties={detail.properties} />
              {detail.streamInfo && <StreamMetadata info={detail.streamInfo} />}
            </>
          )}
        </>
      )}
    </div>
  );
}
