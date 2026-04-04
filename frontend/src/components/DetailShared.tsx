export interface ValueEntryData {
  type: string;
  display: string;
  raw: string;
  refTarget: string;
}

export interface PropertyEntryData {
  key: string;
  value: ValueEntryData;
}

export interface StreamInfoData {
  length: number;
  filters: string[];
}

export interface ObjectDetailData {
  nodeId: string;
  objectRef: string;
  type: string;
  properties: PropertyEntryData[] | null;
  elements: ValueEntryData[] | null;
  scalarValue: ValueEntryData | null;
  streamInfo: StreamInfoData | null;
}

export const TYPE_CLASS_MAP: Record<string, string> = {
  name: 'text-type-name',
  string: 'text-type-string',
  number: 'text-type-number',
  reference: 'text-type-reference underline cursor-pointer',
  boolean: 'text-type-boolean',
  null: 'text-type-null',
};

export function ValueDisplay({ value }: { value: ValueEntryData }) {
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

export function DictView({ properties }: { properties: PropertyEntryData[] | null }) {
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

export function ArrayView({ elements }: { elements: ValueEntryData[] | null }) {
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

export function ScalarView({ value }: { value: ValueEntryData }) {
  return (
    <div className="p-3">
      <div className="text-text-muted text-xs mb-1">Type: {value.type}</div>
      <ValueDisplay value={value} />
    </div>
  );
}

export function StreamMetadata({ info }: { info: StreamInfoData }) {
  const filters = info.filters ?? [];
  return (
    <div className="border-t border-border p-3 text-xs flex-shrink-0" data-testid="stream-metadata">
      <div className="text-text-secondary font-medium mb-1">Stream Metadata</div>
      <div className="flex flex-col gap-1 text-text-muted font-mono">
        <span>{`Length: ${info.length} bytes`}</span>
        <span>{`Filters: ${filters.length === 0 ? 'None' : filters.join(', ')}`}</span>
      </div>
    </div>
  );
}
