/**
 * @file Document Metadata view -- the catalog /Metadata XMP packet and the
 * trailer /Info dictionary fields. The XMP packet is rendered as
 * PLAIN TEXT only (never injected as HTML) in a read-only, scrollable, bounded
 * region; Info fields render as a key/value block.
 */
import { useEffect, useState } from 'react';
import { GetDocumentMetadata } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';

/** Document metadata payload, mirroring `pdfcore.DocumentMetadata`. */
interface DocumentMetadataData {
  info: Record<string, string>;
  xmp: string;
  warning?: string;
}

/** Props for {@link DocumentMetadataView}. */
export interface DocumentMetadataViewProps {
  /** Active document tab ID. Empty string renders the no-document empty state. */
  tabId: string;
  /** True when the Metadata tab is active; the fetch is eager regardless. */
  active: boolean;
}

/** Ordered /Info keys, mirroring the backend's stable plain-text ordering. */
const INFO_ORDER = ['Title', 'Author', 'Subject', 'Keywords', 'Creator', 'Producer', 'CreationDate', 'ModDate'];

/**
 * Document-level metadata view. Eager-fetches on tabId change; data is cached in
 * component state for the document lifetime (tabId change resets).
 */
export function DocumentMetadataView({ tabId, active: _active }: DocumentMetadataViewProps) {
  const [data, setData] = useState<DocumentMetadataData | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setData(null);
    setError(null);
  }, [tabId]);

  useEffect(() => {
    if (!tabId) return;
    let cancelled = false;
    GetDocumentMetadata(tabId)
      .then((result: unknown) => {
        if (cancelled) return;
        const md = result as DocumentMetadataData;
        setData({ info: md?.info ?? {}, xmp: md?.xmp ?? '', warning: md?.warning ?? '' });
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(extractErrorMessage(err));
      });
    return () => { cancelled = true; };
  }, [tabId]);

  if (!tabId) {
    return (
      <div className="h-full flex items-center justify-center text-text-muted text-sm" data-testid="metadata-empty-nodoc">
        No document open
      </div>
    );
  }
  if (error) {
    return <div className="p-3 text-error text-sm" data-testid="metadata-error">{error}</div>;
  }
  if (data === null) {
    return <div className="h-full" data-testid="metadata-empty-initial" />;
  }

  const infoKeys = Object.keys(data.info);
  // Render known keys first in their canonical order, then any extras.
  const orderedKeys = [
    ...INFO_ORDER.filter((k) => k in data.info),
    ...infoKeys.filter((k) => !INFO_ORDER.includes(k)),
  ];
  const hasInfo = orderedKeys.length > 0;
  const hasXMP = data.xmp !== '';

  if (!hasInfo && !hasXMP && !data.warning) {
    return (
      <div className="h-full flex items-center justify-center text-text-muted text-sm" data-testid="metadata-empty">
        No document metadata
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto p-3" data-testid="metadata-view">
      {data.warning && (
        <div className="mb-3 text-error text-xs" data-testid="metadata-warning">{data.warning}</div>
      )}

      {hasInfo && (
        <div className="mb-4" data-testid="metadata-info">
          <h3 className="text-xs font-medium text-text-secondary mb-1">Info</h3>
          <dl className="text-xs font-mono grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
            {orderedKeys.map((k) => (
              <div key={k} className="contents">
                <dt className="text-text-muted">{k}</dt>
                <dd className="text-text break-all">{data.info[k]}</dd>
              </div>
            ))}
          </dl>
        </div>
      )}

      <div data-testid="metadata-xmp-section">
        <h3 className="text-xs font-medium text-text-secondary mb-1">XMP</h3>
        {/* The XMP packet is rendered as PLAIN TEXT inside <pre>. React escapes
            the string content, so embedded markup (e.g. <script>) is shown
            verbatim and never parsed into the DOM. */}
        <pre
          className="p-2 text-xs font-mono bg-surface border border-border rounded overflow-auto max-h-96 whitespace-pre-wrap break-all"
          data-testid="metadata-xmp"
        >
          {hasXMP ? data.xmp : '(no XMP metadata)'}
        </pre>
      </div>
    </div>
  );
}

export default DocumentMetadataView;
