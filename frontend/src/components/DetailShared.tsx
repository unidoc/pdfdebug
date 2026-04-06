/**
 * @file Shared data types and presentational components for rendering
 * PDF object details (dict, array, scalar, stream) in both panels.
 */

/** A single value in a PDF object (name, string, number, reference, etc.). */
export interface ValueEntryData {
  type: string;
  display: string;
  raw: string;
  refTarget: string;
}

/** A key-value pair in a PDF dictionary. */
export interface PropertyEntryData {
  key: string;
  value: ValueEntryData;
}

/** Metadata for a PDF stream object (byte length + compression filters). */
export interface StreamInfoData {
  length: number;
  filters: string[];
}

/** Full detail payload returned by GetObjectDetail for a single PDF object. */
export interface ObjectDetailData {
  nodeId: string;
  objectRef: string;
  type: string;
  properties: PropertyEntryData[] | null;
  elements: ValueEntryData[] | null;
  scalarValue: ValueEntryData | null;
  streamInfo: StreamInfoData | null;
}

/** Maps PDF value types to Tailwind color classes for syntax highlighting. */
export const TYPE_CLASS_MAP: Record<string, string> = {
  name: 'text-type-name',
  string: 'text-type-string',
  number: 'text-type-number',
  reference: 'text-type-reference underline cursor-pointer',
  boolean: 'text-type-boolean',
  null: 'text-type-null',
};

/** Renders a single PDF value with type-based coloring. References are clickable. */
export function ValueDisplay({ value, onReferenceClick }: {
  value: ValueEntryData;
  onReferenceClick?: (refTarget: string) => void;
}) {
  const colorClass = TYPE_CLASS_MAP[value.type] ?? 'text-text';

  if (value.type === 'reference') {
    return (
      <span
        role="button"
        tabIndex={0}
        className={`font-mono text-xs ${colorClass}`}
        data-ref-target={value.refTarget}
        onClick={() => { onReferenceClick?.(value.refTarget); }}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            onReferenceClick?.(value.refTarget);
          }
        }}
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

/** Table view of a PDF dictionary's key-value pairs. */
export function DictView({ properties, onReferenceClick }: {
  properties: PropertyEntryData[] | null;
  onReferenceClick?: (refTarget: string) => void;
}) {
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
                <ValueDisplay value={prop.value} onReferenceClick={onReferenceClick} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** Table view of a PDF array's indexed elements. */
export function ArrayView({ elements, onReferenceClick }: {
  elements: ValueEntryData[] | null;
  onReferenceClick?: (refTarget: string) => void;
}) {
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
                <ValueDisplay value={elem} onReferenceClick={onReferenceClick} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** Displays a single scalar PDF value with its type label. */
export function ScalarView({ value, onReferenceClick }: {
  value: ValueEntryData;
  onReferenceClick?: (refTarget: string) => void;
}) {
  return (
    <div className="p-3">
      <div className="text-text-muted text-xs mb-1">Type: {value.type}</div>
      <ValueDisplay value={value} onReferenceClick={onReferenceClick} />
    </div>
  );
}

/** Renders stream byte-length and applied compression filters. */
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
