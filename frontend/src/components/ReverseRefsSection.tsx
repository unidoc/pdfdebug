/**
 * @file "Referenced by" section. Renders inbound dict-graph edges for the
 * selected indirect object below the DetailPanel's parsed view. Default-
 * expanded for <=5 entries, collapsed otherwise. Toggle resets on selection
 * change via React's key-based remount (DetailPanel passes key={selectedNodeId}).
 *
 * Story 9-10 AC#7-#10 + AC#6 failure-mode banner (case 1).
 */
import { useState } from 'react';
import { useAppDispatch } from '../hooks/useDocumentState';

/** One inbound reference. Matches Go's pdfcore.ReverseRef. */
export interface ReverseRefEntry {
  parentNodeId: string;
  parentRef: string;
  parentType: string | null;
  path: string;
  /** Canonical root-relative path of the parent (BFS discovery). "" for the catalog. */
  parentPath: string;
}

/** Props for the Referenced by section. */
export interface ReverseRefsSectionProps {
  /** Backend-returned inbound edges for the current indirect-object selection. */
  entries: ReverseRefEntry[];
  /** iconHint of the selected node (only 'catalog' triggers the special empty copy). */
  selectedIconHint: string | null;
  /** When true, the index could not be built; render the unavailable banner only. */
  indexUnavailable: boolean;
}

const COLLAPSE_THRESHOLD = 5;

/**
 * Joins the parent's root-relative path with the within-parent path so each
 * row shows a single canonical location. Array indices stay attached to the
 * preceding segment ("/Kids" + "[3]" -> "/Kids[3]"); dict keys are space-
 * separated to match the backend's joinPath rules.
 */
function joinGlobalPath(parentPath: string, path: string): string {
  if (!parentPath) return path;
  if (!path) return parentPath;
  if (path.startsWith('[')) return parentPath + path;
  return parentPath + ' ' + path;
}

/**
 * Renders the Referenced by section. Empty-state priority order (Task 6.5):
 *   1. indexUnavailable=true  -> banner ("Reverse-ref index unavailable...")
 *   2. entries empty + catalog -> "Document root (no incoming references)."
 *   3. entries empty (other)   -> orphan copy with dict-graph qualifier.
 *   4. entries non-empty       -> header with count + collapsible row list.
 */
export function ReverseRefsSection({
  entries,
  selectedIconHint,
  indexUnavailable,
}: ReverseRefsSectionProps) {
  const dispatch = useAppDispatch();
  // Initial collapsed state derives from entries.length at mount time.
  // Re-mount-on-key (driven by parent) resets it for a new selection.
  const [expanded, setExpanded] = useState<boolean>(
    entries.length > 0 && entries.length <= COLLAPSE_THRESHOLD,
  );

  if (indexUnavailable) {
    return (
      <div
        className="border-t border-border p-3 text-xs text-text-muted"
        data-testid="reverse-refs-unavailable"
      >
        Reverse-ref index unavailable for this document.
      </div>
    );
  }

  if (entries.length === 0) {
    const copy = selectedIconHint === 'catalog'
      ? 'Document root (no incoming references).'
      : 'No incoming dict-graph references (possible orphan).';
    return (
      <div
        className="border-t border-border p-3 text-xs text-text-muted"
        data-testid="reverse-refs-empty"
      >
        {copy}
      </div>
    );
  }

  const handleRowActivate = (parentNodeId: string) => {
    dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: parentNodeId } });
  };

  return (
    <div
      className="border-t border-border flex-shrink-0"
      data-testid="reverse-refs-section"
    >
      <button
        type="button"
        onClick={() => setExpanded((e) => !e)}
        className="w-full px-3 py-1.5 text-xs font-medium text-text-secondary hover:bg-hover flex items-center gap-2 text-left"
        aria-expanded={expanded}
      >
        <span
          aria-hidden="true"
          className={`inline-block transition-transform ${expanded ? 'rotate-90' : ''}`}
        >
          {'▶'}
        </span>
        Referenced by ({entries.length})
      </button>
      {expanded && (
        <ul className="overflow-auto max-h-64">
          {entries.map((entry, i) => (
            <li
              key={`${entry.parentNodeId}-${entry.path}-${i}`}
              role="button"
              tabIndex={0}
              onClick={() => handleRowActivate(entry.parentNodeId)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  handleRowActivate(entry.parentNodeId);
                }
              }}
              className="px-3 py-1 border-t border-border flex items-baseline gap-3 cursor-pointer hover:bg-hover focus:outline-none focus:ring-2 focus:ring-focus"
              data-parent-node-id={entry.parentNodeId}
            >
              <span className="font-mono text-xs text-type-reference whitespace-nowrap">
                {entry.parentRef}
              </span>
              {entry.parentType !== null && (
                <span className="font-mono text-xs text-text-muted whitespace-nowrap">
                  {entry.parentType}
                </span>
              )}
              <span
                className="font-mono text-xs text-text-muted flex-1 truncate text-right"
                dir="rtl"
                title={joinGlobalPath(entry.parentPath, entry.path)}
              >
                <bdi>{joinGlobalPath(entry.parentPath, entry.path)}</bdi>
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
