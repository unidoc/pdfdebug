/**
 * @file Bottom-left "Object Source" panel. Renders the selected indirect
 * object as reserialized PDF syntax (e.g. `38109 0 obj\n[ 38110 0 R ]\nendobj`),
 * NOT raw file bytes. Indirect refs in the body are clickable -- click or
 * Enter/Space dispatches NAVIGATE_TO_REF.
 *
 * Component is named ObjectSourcePanel (the file keeps the legacy name to
 * minimize cross-test churn).
 */
import { useState, useEffect, Fragment } from 'react';
import { GetObjectSource } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppState, useAppDispatch } from '../hooks/useDocumentState';

/**
 * Indirect-ref scanner. Capture 1 is num, capture 2 is gen. Load-bearing
 * mapping: dispatched nodeID is `obj:${gen}:${num}`, matching
 * `inspector.go:objectRefFromNodeID` (parentID=gen, lastPart=num). Inverting
 * the mapping yields silently wrong navigation, so it is covered by tests.
 */
const REF_REGEX = /\b(\d+)\s+(\d+)\s+R\b/g;

/**
 * Line marker that the backend prepends to the stream-object byte-count
 * placeholder. The frontend strips it and skips ref scanning on that line so
 * "12,345 bytes" never becomes a fake click target.
 */
const STREAM_PLACEHOLDER_MARKER = '\u200b!';

/**
 * Bottom-left Object Source panel. Fetches reserialized PDF text for the
 * selected indirect object and renders it in monospace with clickable refs.
 */
export function ObjectSourcePanel() {
  const { tabs, activeTabId } = useAppState();
  const dispatch = useAppDispatch();
  const activeTab = tabs.find((t) => t.tabId === activeTabId);
  const selectedNodeId = activeTab?.selectedNodeId ?? null;

  const [source, setSource] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!activeTabId || !selectedNodeId) {
      setSource(null);
      setError(null);
      return;
    }
    setError(null);
    // Stale-fetch guard: discard the response if the selection changed before
    // resolve, matching the pattern used elsewhere in the app.
    let cancelled = false;
    GetObjectSource(activeTabId, selectedNodeId)
      .then((result: unknown) => {
        if (!cancelled) {
          setSource(typeof result === 'string' ? result : '');
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(String(err));
        }
      });
    return () => { cancelled = true; };
  }, [activeTabId, selectedNodeId]);

  const handleRefClick = (refTarget: string) => {
    if (refTarget) {
      dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: refTarget } });
    }
  };

  return (
    <div className="h-full flex flex-col" data-testid="object-source-panel">
      <div className="px-3 py-1.5 border-b border-border flex-shrink-0">
        <span className="text-sm font-medium text-text-secondary">Object Source</span>
      </div>
      <div className="flex-1 min-h-0 overflow-auto">
        {!selectedNodeId && (
          <div
            className="h-full flex items-center justify-center text-text-muted text-sm text-center px-4"
            data-testid="object-source-empty"
          >
            Select an object to view its source.
          </div>
        )}
        {selectedNodeId && error && (
          <div
            className="p-3 text-error text-sm"
            data-testid="object-source-error"
          >
            {error}
          </div>
        )}
        {selectedNodeId && !error && source === '' && (
          <div
            className="h-full flex items-center justify-center text-text-muted text-sm text-center px-4"
            data-testid="object-source-inline-empty"
          >
            Inline object -- no separate source. See parsed view on the right.
          </div>
        )}
        {selectedNodeId && !error && source && (
          <pre
            className="font-mono text-xs whitespace-pre-wrap p-3 text-text"
            data-testid="object-source-body"
          >
            {renderSourceWithClickableRefs(source, handleRefClick)}
          </pre>
        )}
      </div>
    </div>
  );
}

/**
 * Splits the rendered text by the indirect-ref regex and wraps each match in
 * a keyboard-accessible clickable span. Lines tagged with the stream
 * placeholder marker are passed through verbatim (with the marker stripped),
 * so the byte-count line cannot become a false-positive click target.
 */
function renderSourceWithClickableRefs(
  source: string,
  onRefClick: (target: string) => void,
): React.ReactNode {
  const lines = source.split('\n');
  return lines.map((line, lineIdx) => {
    const trailingNewline = lineIdx < lines.length - 1 ? '\n' : '';

    if (line.startsWith(STREAM_PLACEHOLDER_MARKER)) {
      // Strip the marker; suppress ref scanning so the byte-count line never
      // becomes a click target.
      return (
        <Fragment key={lineIdx}>
          {line.slice(STREAM_PLACEHOLDER_MARKER.length)}
          {trailingNewline}
        </Fragment>
      );
    }

    return (
      <Fragment key={lineIdx}>
        {tokenizeLine(line, onRefClick)}
        {trailingNewline}
      </Fragment>
    );
  });
}

/**
 * Splits one line into plain-text segments and clickable ref spans. Resets
 * the regex's `lastIndex` because exec-on-global state persists across calls.
 */
function tokenizeLine(
  line: string,
  onRefClick: (target: string) => void,
): React.ReactNode[] {
  const out: React.ReactNode[] = [];
  REF_REGEX.lastIndex = 0;
  let cursor = 0;
  let match: RegExpExecArray | null;
  let key = 0;
  while ((match = REF_REGEX.exec(line)) !== null) {
    if (match.index > cursor) {
      out.push(line.slice(cursor, match.index));
    }
    const num = match[1];
    const gen = match[2];
    // Mapping: num is capture 1, gen is capture 2. Target ID is obj:gen:num
    // (parentID=gen, lastPart=num, matching inspector.go).
    const target = `obj:${gen}:${num}`;
    out.push(
      <span
        key={`ref-${key++}-${match.index}`}
        role="button"
        tabIndex={0}
        className="font-mono text-xs cursor-pointer hover:underline text-type-reference focus:outline-none focus:ring-2 focus:ring-focus rounded-sm"
        data-ref-target={target}
        onClick={() => onRefClick(target)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            onRefClick(target);
          }
        }}
      >
        {match[0]}
      </span>,
    );
    cursor = match.index + match[0].length;
  }
  if (cursor < line.length) {
    out.push(line.slice(cursor));
  }
  return out;
}

// Legacy export kept so MainLayout/other test files don't need renaming in
// this story (Task 5.1 keeps the file path stable).
export const ObjectInfoPanel = ObjectSourcePanel;
