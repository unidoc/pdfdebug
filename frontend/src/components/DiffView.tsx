/**
 * @file Structural diff side-by-side view. Given two open document
 * tab IDs it fetches the path-aligned DiffDocuments delta and renders it as two
 * synchronized tree panes (left/right) with added/removed/changed nodes
 * color-coded, a summary header, and next/prev-change navigation. Unchanged
 * subtrees collapse by default; the path to each change is auto-expanded so
 * changes are visible without interaction. Selecting a change shows the
 * per-key/value detail. Structure only - not a byte or pixel diff.
 */
import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { DiffDocuments } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';

/** One node in the structural delta tree, mirroring `pdfcore.DiffNode`. */
export interface DiffNodeData {
  path: string;
  status: 'added' | 'removed' | 'changed' | 'unchanged';
  kind: string;
  changedKeys?: string[];
  leftSummary: string;
  rightSummary: string;
  children?: DiffNodeData[];
  /** True when this ref was left unwalked at the depth cap. */
  truncated?: boolean;
}

/** The document-level tally, mirroring `pdfcore.DiffSummary`. */
export interface DiffSummaryData {
  added: number;
  removed: number;
  changed: number;
  pageCountLeft: number;
  pageCountRight: number;
  versionChanged: boolean;
  encryptionChanged: boolean;
  infoChanged: boolean;
  xmpChanged: boolean;
  /** Count of subtrees compared only to the depth cap; when
   *  > 0 the walk was bounded and identity cannot be claimed. */
  truncatedSubtrees: number;
}

/** The diff outcome, mirroring `pdfcore.DiffResult`. */
export interface DiffResultData {
  root: DiffNodeData | null;
  summary: DiffSummaryData;
}

/** Props for {@link DiffView}. */
export interface DiffViewProps {
  /** Left (baseline) document tab ID. */
  leftTabId: string;
  /** Right (comparison) document tab ID. */
  rightTabId: string;
  /** True when the diff view is active; the fetch runs on mount when active. */
  active: boolean;
}

/** Reports whether a node or any descendant is not "unchanged", or is a
 *  depth-cap truncated ref. A truncated node reports status "unchanged" but must
 *  still route auto-expansion so its [truncated: depth cap] marker is reachable
 *  without hand-expanding every level (mirrors Go's diffNodeHasDelta). */
function hasDelta(node: DiffNodeData): boolean {
  if (node.status !== 'unchanged' || node.truncated) return true;
  return (node.children ?? []).some(hasDelta);
}

/** Collects the paths that should be expanded by default: every node on the
 *  route to a change (i.e. carrying a delta descendant/self). */
function autoExpandedPaths(node: DiffNodeData, out: Set<string>): void {
  if (hasDelta(node) && (node.children?.length ?? 0) > 0) {
    out.add(node.path);
  }
  for (const c of node.children ?? []) autoExpandedPaths(c, out);
}

/** Pre-order list of the concrete, navigable changes: added/removed objects and
 *  changed leaves (value-level changes). Container "changed" nodes are the route
 *  to these, not navigation targets themselves. */
function collectChanges(node: DiffNodeData, out: DiffNodeData[]): void {
  const isLeaf = (node.children?.length ?? 0) === 0;
  if (node.status === 'added' || node.status === 'removed' || (node.status === 'changed' && isLeaf)) {
    out.push(node);
  }
  for (const c of node.children ?? []) collectChanges(c, out);
}

/** Finds a node by its path anywhere in the tree. */
function findByPath(node: DiffNodeData, path: string): DiffNodeData | null {
  if (node.path === path) return node;
  for (const c of node.children ?? []) {
    const hit = findByPath(c, path);
    if (hit) return hit;
  }
  return null;
}

/** Returns the paths on the route from the root to targetPath (inclusive), or
 *  null when the target is not in the tree. Used to expand the ancestors of a
 *  navigated change so it is visible even if the user collapsed a parent. */
function pathChain(node: DiffNodeData, targetPath: string, acc: string[]): string[] | null {
  const next = [...acc, node.path];
  if (node.path === targetPath) return next;
  for (const c of node.children ?? []) {
    const hit = pathChain(c, targetPath, next);
    if (hit) return hit;
  }
  return null;
}

/** Tailwind classes keyed by status for the row color-coding. */
const STATUS_CLASS: Record<string, string> = {
  added: 'text-success',
  removed: 'text-error',
  changed: 'text-warning',
  unchanged: 'text-text-muted',
};

/** One visible row: a node plus its indent depth. */
interface FlatRow {
  node: DiffNodeData;
  depth: number;
}

/** Flattens the tree into the visible rows given the expanded-path set. */
function flatten(node: DiffNodeData, depth: number, expanded: Set<string>, out: FlatRow[]): void {
  out.push({ node, depth });
  if (expanded.has(node.path)) {
    for (const c of node.children ?? []) flatten(c, depth + 1, expanded, out);
  }
}

/**
 * Side-by-side structural diff view. Self-contained: fetches the delta for the
 * two tab IDs and renders it. The LEFT pane carries the interactive rows
 * (selection + status data attributes); the RIGHT pane mirrors the same visible
 * nodes showing the right-hand value.
 */
export function DiffView({ leftTabId, rightTabId, active }: DiffViewProps) {
  const [result, setResult] = useState<DiffResultData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  // The Diff tab is forceMounted, so this component stays mounted across tab
  // switches and `active` toggles on every switch. Track the last SUCCESSFULLY
  // fetched (left,right) pair so a mere re-activation does not re-run the
  // expensive two-graph walk or reset the user's selection/expansion. Only set
  // on success so a fetch cancelled by a quick tab switch (or a failed one) can
  // retry on the next activation.
  const fetchedPairRef = useRef<string | null>(null);

  useEffect(() => {
    if (!active || !leftTabId || !rightTabId) return;
    const pairKey = `${leftTabId}::${rightTabId}`;
    if (fetchedPairRef.current === pairKey) return;
    let cancelled = false;
    DiffDocuments(leftTabId, rightTabId)
      .then((res: unknown) => {
        if (cancelled) return;
        fetchedPairRef.current = pairKey;
        const r = res as DiffResultData;
        setResult(r);
        setError(null);
        // Auto-expand the route to every change; collapse everything else.
        const exp = new Set<string>();
        if (r.root) autoExpandedPaths(r.root, exp);
        setExpanded(exp);
        setSelectedPath(null);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(extractErrorMessage(err));
      });
    return () => {
      cancelled = true;
    };
  }, [active, leftTabId, rightTabId]);

  const changes = useMemo(() => {
    if (!result?.root) return [];
    const out: DiffNodeData[] = [];
    collectChanges(result.root, out);
    return out;
  }, [result]);

  const rows = useMemo(() => {
    if (!result?.root) return [];
    const out: FlatRow[] = [];
    flatten(result.root, 0, expanded, out);
    return out;
  }, [result, expanded]);

  const selectedNode = useMemo(
    () => (result?.root && selectedPath ? findByPath(result.root, selectedPath) : null),
    [result, selectedPath]
  );

  const navChange = useCallback(
    (dir: 1 | -1) => {
      if (changes.length === 0) return;
      const idx = changes.findIndex((c) => c.path === selectedPath);
      let next: number;
      if (idx === -1) {
        next = dir === 1 ? 0 : changes.length - 1;
      } else {
        next = (idx + dir + changes.length) % changes.length;
      }
      const target = changes[next].path;
      setSelectedPath(target);
      // Reveal the navigated change even if the user collapsed an ancestor:
      // expand every node on the route to it.
      if (result?.root) {
        const chain = pathChain(result.root, target, []);
        if (chain) setExpanded((prev) => new Set([...prev, ...chain]));
      }
    },
    [changes, selectedPath, result]
  );

  const toggle = useCallback((path: string) => {
    setExpanded((prev) => {
      const nextSet = new Set(prev);
      if (nextSet.has(path)) nextSet.delete(path);
      else nextSet.add(path);
      return nextSet;
    });
  }, []);

  if (error) {
    return (
      <div className="p-3 text-error text-sm" data-testid="diff-error">
        {error}
      </div>
    );
  }

  if (!result) {
    return (
      <div className="p-3 text-text-muted text-sm" data-testid="diff-loading">
        Computing structural diff...
      </div>
    );
  }

  const s = result.summary;
  // Mirror Go's diffIsIdentical: node counts alone miss document-level deltas
  // (encryption, /Version, /Info, XMP) that live off the catalog walk, so a
  // flags-only change must not report "No structural differences". A
  // depth-capped walk (truncatedSubtrees > 0) left a subtree unexplored, so
  // identity cannot be claimed even with zero visible deltas,
  // mirroring Go's diffIsIdentical.
  const identical =
    s.added === 0 &&
    s.removed === 0 &&
    s.changed === 0 &&
    !s.versionChanged &&
    !s.encryptionChanged &&
    !s.infoChanged &&
    !s.xmpChanged &&
    s.truncatedSubtrees === 0;

  return (
    <div className="h-full flex flex-col" data-testid="diff-view">
      <div
        className="px-3 py-2 border-b border-border bg-surface flex-shrink-0 text-xs text-text-secondary"
        data-testid="diff-summary"
      >
        <div>
          {identical ? 'No structural differences. ' : ''}
          {s.added} added, {s.removed} removed, {s.changed} changed
        </div>
        <div className="text-text-muted mt-0.5">
          Page count: {s.pageCountLeft} -&gt; {s.pageCountRight}
          {s.versionChanged ? ' | /Version changed' : ''}
          {s.encryptionChanged ? ' | encryption changed' : ''}
          {s.infoChanged ? ' | /Info changed' : ''}
          {s.xmpChanged ? ' | XMP changed' : ''}
        </div>
        {s.truncatedSubtrees > 0 && (
          <div className="text-warning mt-0.5" data-testid="diff-truncation-note">
            {s.truncatedSubtrees} subtree{s.truncatedSubtrees === 1 ? '' : 's'} truncated at the depth
            cap; deeper differences cannot be ruled out.
          </div>
        )}
      </div>

      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-surface flex-shrink-0">
        <span className="text-xs text-text-muted">
          {changes.length} change{changes.length === 1 ? '' : 's'}
        </span>
        <button
          type="button"
          data-testid="diff-prev-change"
          className="px-2 py-0.5 text-xs rounded border border-border text-text-secondary hover:bg-surface-hover cursor-pointer disabled:opacity-50"
          onClick={() => navChange(-1)}
          disabled={changes.length === 0}
        >
          Prev change
        </button>
        <button
          type="button"
          data-testid="diff-next-change"
          className="px-2 py-0.5 text-xs rounded border border-border text-text-secondary hover:bg-surface-hover cursor-pointer disabled:opacity-50"
          onClick={() => navChange(1)}
          disabled={changes.length === 0}
        >
          Next change
        </button>
      </div>

      <div className="flex-1 min-h-0 flex overflow-hidden">
        {/* Left pane: interactive rows carrying status + selection. */}
        <div
          className="flex-1 min-w-0 overflow-auto border-r border-border font-mono text-xs"
          data-testid="diff-tree-left"
        >
          {rows.map(({ node, depth }) => {
            const isLeaf = (node.children?.length ?? 0) === 0;
            const isExpanded = expanded.has(node.path);
            return (
              <div
                key={node.path}
                data-testid="diff-node"
                data-status={node.status}
                data-selected={node.path === selectedPath ? 'true' : 'false'}
                onClick={() => setSelectedPath(node.path)}
                className={
                  'px-2 py-0.5 cursor-pointer whitespace-nowrap hover:bg-surface-hover ' +
                  (node.path === selectedPath ? 'bg-surface-hover ' : '') +
                  (STATUS_CLASS[node.status] ?? '')
                }
                style={{ paddingLeft: `${depth * 12 + 8}px` }}
              >
                {!isLeaf && (
                  <button
                    type="button"
                    className="mr-1 text-text-muted"
                    aria-label={isExpanded ? 'Collapse' : 'Expand'}
                    onClick={(e) => {
                      e.stopPropagation();
                      toggle(node.path);
                    }}
                  >
                    {isExpanded ? '-' : '+'}
                  </button>
                )}
                <span>{diffMarker(node.status)} </span>
                <span>{node.path}</span>
                {node.leftSummary ? <span className="text-text-muted"> {node.leftSummary}</span> : null}
                {node.truncated ? <span className="text-warning"> [truncated: depth cap]</span> : null}
              </div>
            );
          })}
        </div>

        {/* Right pane: the same visible nodes showing the right-hand value. */}
        <div
          className="flex-1 min-w-0 overflow-auto font-mono text-xs"
          data-testid="diff-tree-right"
        >
          {rows.map(({ node, depth }) => (
            <div
              key={node.path}
              data-status={node.status}
              className={'px-2 py-0.5 whitespace-nowrap ' + (STATUS_CLASS[node.status] ?? '')}
              style={{ paddingLeft: `${depth * 12 + 8}px` }}
            >
              <span>{diffMarker(node.status)} </span>
              <span>{node.path}</span>
              {node.rightSummary ? <span className="text-text-muted"> {node.rightSummary}</span> : null}
            </div>
          ))}
        </div>
      </div>

      {selectedNode && (
        <div
          className="border-t border-border bg-surface flex-shrink-0 p-3 text-xs font-mono max-h-40 overflow-auto"
          data-testid="diff-detail"
        >
          <div className="text-text-secondary mb-1 break-all">{selectedNode.path}</div>
          <div className="text-text-muted uppercase tracking-wide mb-1">{selectedNode.status}</div>
          {selectedNode.changedKeys && selectedNode.changedKeys.length > 0 && (
            <div className="mb-1">changed keys: {selectedNode.changedKeys.join(', ')}</div>
          )}
          {(selectedNode.leftSummary || selectedNode.rightSummary) && (
            <div className="flex flex-col gap-0.5">
              <div>
                <span className="text-text-muted">left: </span>
                <span className="text-error break-all">{selectedNode.leftSummary || '(absent)'}</span>
              </div>
              <div>
                <span className="text-text-muted">right: </span>
                <span className="text-success break-all">{selectedNode.rightSummary || '(absent)'}</span>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/** Maps a status to its plain-text delta marker for the row prefix. */
function diffMarker(status: string): string {
  switch (status) {
    case 'added':
      return '+';
    case 'removed':
      return '-';
    case 'changed':
      return '~';
    default:
      return ' ';
  }
}

export default DiffView;
