import { useState, useEffect } from 'react';
import { GetObjectDetail } from '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppState } from '../hooks/useDocumentState';

interface ValueEntryData {
  type: string;
  display: string;
  raw: string;
  refTarget: string;
}

interface PropertyEntryData {
  key: string;
  value: ValueEntryData;
}

interface StreamInfoData {
  length: number;
  filters: string[];
}

interface ObjectDetailData {
  nodeId: string;
  objectRef: string;
  type: string;
  properties: PropertyEntryData[] | null;
  elements: ValueEntryData[] | null;
  scalarValue: ValueEntryData | null;
  streamInfo: StreamInfoData | null;
}

const TYPE_CLASS_MAP: Record<string, string> = {
  name: 'text-type-name',
  string: 'text-type-string',
  number: 'text-type-number',
  reference: 'text-type-reference underline cursor-pointer',
  boolean: 'text-type-boolean',
  null: 'text-type-null',
};

function ValueDisplay({ value }: { value: ValueEntryData }) {
  const colorClass = TYPE_CLASS_MAP[value.type] ?? 'text-text';

  if (value.type === 'reference') {
    return (
      <span
        role="button"
        className={`font-mono text-xs ${colorClass}`}
        data-ref-target={value.refTarget}
        onClick={() => {}}
      >
        {value.display}
      </span>
    );
  }

  return (
    <span className={`font-mono text-xs ${colorClass}`}>
      {value.display}
    </span>
  );
}

function DictView({ properties }: { properties: PropertyEntryData[] | null }) {
  if (!properties || properties.length === 0) {
    return <div className="text-text-muted text-sm p-3">Empty dictionary</div>;
  }
  return (
    <div className="overflow-auto flex-1 min-h-0">
      <table className="w-full text-xs">
        <tbody>
          {properties.map((prop) => (
            <tr key={prop.key} className="border-b border-border">
              <td className="font-mono text-text-muted text-xs py-1 px-3 whitespace-nowrap align-top">
                {prop.key}
              </td>
              <td className="py-1 px-3">
                <ValueDisplay value={prop.value} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ArrayView({ elements }: { elements: ValueEntryData[] | null }) {
  if (!elements || elements.length === 0) {
    return <div className="text-text-muted text-sm p-3">Empty array</div>;
  }
  return (
    <div className="overflow-auto flex-1 min-h-0">
      <table className="w-full text-xs">
        <tbody>
          {elements.map((elem, i) => (
            <tr key={i} className="border-b border-border">
              <td className="font-mono text-text-muted text-xs py-1 px-3 whitespace-nowrap align-top">
                [{i}]
              </td>
              <td className="py-1 px-3">
                <ValueDisplay value={elem} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ScalarView({ value }: { value: ValueEntryData }) {
  return (
    <div className="p-3">
      <div className="text-text-muted text-xs mb-1">Type: {value.type}</div>
      <ValueDisplay value={value} />
    </div>
  );
}

function StreamMetadata({ info }: { info: StreamInfoData }) {
  return (
    <div className="border-t border-border p-3 text-xs flex-shrink-0" data-testid="stream-metadata">
      <div className="text-text-secondary font-medium mb-1">Stream Metadata</div>
      <div className="flex flex-col gap-1 text-text-muted font-mono">
        <span>{`Length: ${info.length} bytes`}</span>
        <span>{`Filters: ${info.filters.length === 0 ? 'None' : info.filters.join(', ')}`}</span>
      </div>
    </div>
  );
}

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
          {detail.type === 'scalar' && detail.scalarValue && <ScalarView value={detail.scalarValue} />}
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
