/**
 * @file Embedded Data view -- document-level list of embedded/associated files
 * (attachments, ZUGFeRD/Factur-X invoice XML). Lists every embedded
 * file from the catalog /AF array and the /Names/EmbeddedFiles name tree
 * (merged + deduped server-side). Selecting a row shows its /Filespec detail
 * with "Reveal in tree" (reuses NAVIGATE_TO_REF wiring via onNavigate) and a
 * "Save..." action. XML/text payloads get an inline read-only preview; binary
 * payloads are save-only.
 */
import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import {
  GetEmbeddedFiles,
  GetEmbeddedFileBytes,
  SaveBytesToFile,
} from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';

/** One embedded file, mirroring `pdfcore.EmbeddedFile`. */
interface EmbeddedFileData {
  name: string;
  filespecRef: string;
  embeddedFileRef: string;
  embeddedFileNodeId: string;
  afRelationship: string;
  subtype: string;
  size: number;
  checkSum?: string;
  modDate?: string;
  warning?: string;
}

/** Props for {@link EmbeddedDataView}. */
export interface EmbeddedDataViewProps {
  /** Active document tab ID. Empty string renders the no-document empty state. */
  tabId: string;
  /** True when the Embedded tab is active; the fetch is eager regardless. */
  active: boolean;
  /** Dispatches NAVIGATE_TO_REF to jump TreePanel to the /EmbeddedFile stream. */
  onNavigate: (nodeId: string) => void;
  /** Fires with the file count once the fetch resolves (for the "(N)" tab label). */
  onLoaded?: (count: number) => void;
}

/** Renders a byte count as a compact human-readable size (e.g. "1.5 KB"). */
function humanizeBytes(n: number): string {
  if (n < 0) return '-';
  if (n < 1024) return `${n} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let val = n / 1024;
  let i = 0;
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024;
    i += 1;
  }
  return `${val.toFixed(1)} ${units[i]}`;
}

/** True when a /Subtype MIME is text-like and safe to preview inline. */
function isTextLike(subtype: string): boolean {
  const s = subtype.toLowerCase();
  return s.startsWith('text/') || s.includes('xml') || s.includes('json');
}

/** Decodes a base64 payload (as returned by the Wails []byte binding) to a
 *  UTF-8 string. Returns null on malformed input. */
function decodeBase64ToText(b64: string): string | null {
  try {
    const binary = atob(b64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
    return new TextDecoder('utf-8', { fatal: false }).decode(bytes);
  } catch {
    return null;
  }
}

/** Stable row identity: the /EmbeddedFile ref when present, else the /Filespec
 *  ref. A direct filespec with no stream has both refs empty, so fall back to
 *  the row index to keep keys unique (avoids duplicate empty React keys). */
function rowKey(f: EmbeddedFileData, index: number): string {
  return f.embeddedFileRef || f.filespecRef || `idx:${index}`;
}

/**
 * Document-level Embedded Data view. Eager-fetches on tabId change; data is
 * cached in component state for the document lifetime (tabId change resets).
 */
export function EmbeddedDataView({ tabId, active: _active, onNavigate, onLoaded }: EmbeddedDataViewProps) {
  const [files, setFiles] = useState<EmbeddedFileData[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [preview, setPreview] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  const onLoadedRef = useRef(onLoaded);
  useEffect(() => { onLoadedRef.current = onLoaded; }, [onLoaded]);

  // Reset everything when the document changes.
  useEffect(() => {
    setFiles(null);
    setError(null);
    setSelectedKey(null);
    setPreview(null);
    setSaveError(null);
  }, [tabId]);

  // Eager fetch on tabId change (cheap: walks already-parsed pdfcpu state).
  useEffect(() => {
    if (!tabId) return;
    let cancelled = false;
    GetEmbeddedFiles(tabId)
      .then((result) => {
        if (cancelled) return;
        const list = ((result?.files ?? []) as unknown) as EmbeddedFileData[];
        setFiles(list);
        onLoadedRef.current?.(list.length);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(extractErrorMessage(err));
      });
    return () => { cancelled = true; };
  }, [tabId]);

  const selected = useMemo(
    () => (files ?? []).find((f, i) => rowKey(f, i) === selectedKey) ?? null,
    [files, selectedKey]
  );

  // Fetch + decode the inline preview when a text-like row with a stream is
  // selected. Binary rows and degraded (no-stream) rows skip the preview.
  useEffect(() => {
    setPreview(null);
    setSaveError(null);
    if (!selected || !selected.embeddedFileNodeId) return;
    if (!isTextLike(selected.subtype)) return;
    let cancelled = false;
    // Promise.resolve wrap: the real binding returns a CancellablePromise, but
    // guarding here keeps the effect safe if the call returns undefined.
    Promise.resolve(GetEmbeddedFileBytes(tabId, selected.embeddedFileNodeId))
      .then((b64) => {
        if (cancelled) return;
        const text = decodeBase64ToText(b64 ?? '');
        // null = undecodable bytes; show a marker so it is not mistaken for an
        // empty file (which would render an identical blank <pre>).
        setPreview(text ?? '(binary or undecodable content)');
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setSaveError(extractErrorMessage(err));
      });
    return () => { cancelled = true; };
  }, [selected, tabId]);

  const handleReveal = useCallback(() => {
    if (selected?.embeddedFileNodeId) onNavigate(selected.embeddedFileNodeId);
  }, [selected, onNavigate]);

  const handleSave = useCallback(async () => {
    if (!selected?.embeddedFileNodeId) return;
    setSaveError(null);
    try {
      // GetEmbeddedFileBytes returns the []byte as a base64 string (Wails
      // marshalling); SaveBytesToFile's data param is the same base64 string, so
      // the payload round-trips verbatim.
      const b64 = await GetEmbeddedFileBytes(tabId, selected.embeddedFileNodeId);
      await SaveBytesToFile(selected.name, b64);
    } catch (err) {
      setSaveError(extractErrorMessage(err));
    }
  }, [selected, tabId]);

  if (!tabId) {
    return (
      <div className="h-full flex items-center justify-center text-text-muted text-sm" data-testid="embedded-empty-nodoc">
        No document open
      </div>
    );
  }
  if (error) {
    return <div className="p-3 text-error text-sm" data-testid="embedded-error">{error}</div>;
  }
  if (files === null) {
    return <div className="h-full" data-testid="embedded-empty-initial" />;
  }
  if (files.length === 0) {
    return (
      <div className="h-full flex items-center justify-center text-text-muted text-sm" data-testid="embedded-empty">
        No embedded files
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col overflow-hidden" data-testid="embedded-view">
      <div className="overflow-auto flex-shrink-0 max-h-72">
        <table className="w-full text-xs font-mono border-collapse">
          <thead>
            <tr>
              <th className="sticky top-0 bg-surface border-b border-border px-2 py-1 text-left text-text-secondary font-medium">Name</th>
              <th className="sticky top-0 bg-surface border-b border-border px-2 py-1 text-left text-text-secondary font-medium">Relationship</th>
              <th className="sticky top-0 bg-surface border-b border-border px-2 py-1 text-left text-text-secondary font-medium">MIME</th>
              <th className="sticky top-0 bg-surface border-b border-border px-2 py-1 text-right text-text-secondary font-medium">Size</th>
            </tr>
          </thead>
          <tbody>
            {files.map((f, i) => {
              const key = rowKey(f, i);
              const isSel = key === selectedKey;
              return (
                <tr
                  key={key}
                  className={`border-b border-border cursor-pointer hover:bg-surface-hover ${isSel ? 'bg-surface-hover' : ''}`}
                  tabIndex={0}
                  data-testid={`embedded-row-${key}`}
                  onClick={() => setSelectedKey(key)}
                  onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setSelectedKey(key); } }}
                >
                  <td className="px-2 py-1 text-left text-text">{f.name || '-'}</td>
                  <td className="px-2 py-1 text-left text-text">{f.afRelationship || '-'}</td>
                  <td className="px-2 py-1 text-left text-text">{f.subtype || '-'}</td>
                  <td className="px-2 py-1 text-right text-text">{humanizeBytes(f.size)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {selected && (
        <div className="border-t border-border p-3 overflow-auto flex-1 min-h-0" data-testid="embedded-detail">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm font-medium text-text-secondary">{selected.name}</span>
            <span className="text-xs text-text-muted font-mono">{selected.filespecRef || '(direct filespec)'}</span>
          </div>

          <dl className="text-xs font-mono grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
            <dt className="text-text-muted">Relationship</dt><dd className="text-text">{selected.afRelationship || '-'}</dd>
            <dt className="text-text-muted">MIME</dt><dd className="text-text">{selected.subtype || '-'}</dd>
            <dt className="text-text-muted">Size</dt><dd className="text-text">{humanizeBytes(selected.size)}</dd>
            <dt className="text-text-muted">Filespec</dt><dd className="text-text">{selected.filespecRef || '-'}</dd>
            <dt className="text-text-muted">EmbeddedFile</dt><dd className="text-text">{selected.embeddedFileRef || '-'}</dd>
            {selected.checkSum && (<><dt className="text-text-muted">CheckSum</dt><dd className="text-text break-all">{selected.checkSum}</dd></>)}
            {selected.modDate && (<><dt className="text-text-muted">ModDate</dt><dd className="text-text">{selected.modDate}</dd></>)}
          </dl>

          {!selected.embeddedFileRef && (
            <div className="mt-2 text-error text-xs" data-testid="embedded-entry-warning">
              {selected.warning || 'This filespec has no embedded-file stream.'}
            </div>
          )}

          <div className="flex gap-2 mt-3">
            <button
              type="button"
              className="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:bg-surface-hover disabled:opacity-40 disabled:cursor-not-allowed"
              data-testid="embedded-reveal-in-tree"
              onClick={handleReveal}
              disabled={!selected.embeddedFileNodeId}
            >
              Reveal in tree
            </button>
            <button
              type="button"
              className="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:bg-surface-hover disabled:opacity-40 disabled:cursor-not-allowed"
              data-testid="embedded-save"
              onClick={handleSave}
              disabled={!selected.embeddedFileNodeId}
            >
              Save...
            </button>
          </div>

          {saveError && <div className="mt-2 text-error text-xs" data-testid="embedded-save-error">{saveError}</div>}

          {selected.embeddedFileNodeId && isTextLike(selected.subtype) && preview !== null && (
            <pre
              className="mt-3 p-2 text-xs font-mono bg-surface border border-border rounded overflow-auto max-h-64 whitespace-pre-wrap break-all"
              data-testid="embedded-text-preview"
            >
              {preview}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}

export default EmbeddedDataView;
