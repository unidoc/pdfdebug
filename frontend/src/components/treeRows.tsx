/**
 * @file Tree row pieces shared by the Structure tree and the Pages navigator:
 * the react-arborist data shape, its builders, lookups and updaters, the
 * lazy child loader, and the row renderer. Both navigators render rows through NodeRenderer so they read
 * identically.
 */
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import type { NodeRendererProps } from 'react-arborist';
import { GetChildren } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { BookOpen, FolderTree, FileText, FileCode, Image as ImageIcon, Type, type LucideIcon } from 'lucide-react';
import type { TreeNode } from '../hooks/useDocumentState';
import { useLatest } from '../hooks/useLatest';
import { clampDisplayValue, TREE_VALUE_RENDER_CAP } from '../lib/escapeDisplayValue';

/**
 * Per-row transient state (which node is mid-load, which is flashing) delivered
 * to NodeRenderer via context rather than props. react-arborist memoizes its
 * rows and rebuilds its entire O(N) node model whenever any Tree prop identity
 * changes; passing these through the render-prop would force that rebuild on
 * every load/flash toggle. A context change instead re-renders only the row
 * consumers, leaving the Tree props (and the model) untouched.
 */
export const RowStateContext = createContext<{ loadingNodeId: string | null; flashNodeId: string | null }>({
  loadingNodeId: null,
  flashNodeId: null,
});

/**
 * Map a backend iconHint to a lucide-react icon component. Returns null for
 * "default" and unknown hints so untyped scalars/arrays render without
 * decoration and the tree stays visually quiet.
 */
export function iconForHint(hint: string): LucideIcon | null {
  switch (hint) {
    case 'catalog': return BookOpen;
    case 'pages':   return FolderTree;
    case 'page':    return FileText;
    case 'stream':  return FileCode;
    case 'image':   return ImageIcon;
    case 'font':    return Type;
    default:        return null;
  }
}

/** Shape consumed by react-arborist. Mapped from backend TreeNode. */
export interface TreeNodeData {
  id: string;         // path-unique display ID for react-arborist
  backendId: string;  // original backend ID for API calls
  name: string;
  children: TreeNodeData[] | null;
  rawKey: string;
  nodeType: string;
  valueType: string;
  hasChildren: boolean;
  childCount: number;
  iconHint: string;
  error: string;
  objectRef: string;
  typeName: string;
  value: string;
}

/**
 * Convert a backend TreeNode to the react-arborist data shape.
 * parentTreeId is prepended to create a path-unique display ID so that
 * the same PDF object appearing in multiple places (e.g., /Parent back-ref)
 * gets distinct IDs and react-arborist doesn't collapse the wrong node.
 */
export function toTreeNodeData(node: TreeNode, parentTreeId?: string): TreeNodeData {
  const displayId = parentTreeId ? `${parentTreeId}>${node.id}` : node.id;
  return {
    id: displayId,
    backendId: node.id,
    name: node.label,
    rawKey: node.rawKey,
    nodeType: node.nodeType,
    valueType: node.valueType,
    hasChildren: node.hasChildren,
    childCount: node.childCount,
    iconHint: node.iconHint,
    error: node.error,
    objectRef: node.objectRef ?? '',
    typeName: node.typeName ?? '',
    value: node.value ?? '',
    children: node.hasChildren ? [] : null,
  };
}

/**
 * Decide whether the /T:<typeName> suffix should be rendered for a tree row.
 * Dedup: suppress when the semantic label already encodes the type.
 *   - exact case-insensitive match (e.g. label "Pages" vs typeName "Pages")
 *   - "Font:" prefix label (e.g. "Font: Helvetica" vs typeName "Font")
 * Otherwise render. Empty typeName -> never render.
 */
function shouldRenderTypeSuffix(label: string, typeName: string): boolean {
  if (typeName === '') return false;
  const lbl = label.toLowerCase();
  const tn = typeName.toLowerCase();
  if (lbl === tn) return false;
  if (tn === 'font' && lbl.startsWith('font:')) return false;
  return true;
}

/**
 * Derive a react-arborist openState map from tree data. A node is "open"
 * if it has children and its children array is non-empty (i.e., children
 * were fetched and the node was expanded).
 */
export function deriveOpenState(data: TreeNodeData[]): Record<string, boolean> {
  const state: Record<string, boolean> = {};
  function walk(nodes: TreeNodeData[]) {
    for (const n of nodes) {
      if (n.hasChildren && Array.isArray(n.children) && n.children.length > 0) {
        state[n.id] = true;
        walk(n.children);
      }
    }
  }
  walk(data);
  return state;
}

/** Depth-first search for the first node matching `match`. */
export function findNode(data: TreeNodeData[], match: (node: TreeNodeData) => boolean): TreeNodeData | null {
  for (const n of data) {
    if (match(n)) return n;
    if (n.children) {
      const found = findNode(n.children, match);
      if (found) return found;
    }
  }
  return null;
}

/** Map a backend node id to its react-arborist display id by walking the tree. */
export function findDisplayId(data: TreeNodeData[], backendId: string): string | undefined {
  return findNode(data, (n) => n.backendId === backendId)?.id;
}

/**
 * Lazy-loads a row's children the first time it is expanded. Each row keeps
 * its own request generation, so expanding a second row before the first
 * returns does not discard the first row's children; re-expanding the same
 * row supersedes its earlier request. The spinner shows after 200ms for the
 * most recent request only. `getNode` finds a row by display id in the
 * current tab's data; `applyChildren` stores the fetched children for the tab
 * the request was made in.
 */
export function useLazyChildren(
  tabId: string | null,
  getNode: (id: string) => TreeNodeData | null,
  applyChildren: (tabId: string, id: string, children: TreeNodeData[]) => void,
): { loadingNodeId: string | null; toggle: (id: string) => Promise<void> } {
  const [loadingNodeId, setLoadingNodeId] = useState<string | null>(null);
  // Delays the spinner by 200ms to avoid flicker on fast loads.
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const spinnerOwner = useRef<object | null>(null);
  const generations = useRef(new Map<string, number>());
  const getNodeRef = useLatest(getNode);
  const applyRef = useLatest(applyChildren);

  useEffect(() => () => {
    if (timerRef.current) clearTimeout(timerRef.current);
  }, []);

  const toggle = useCallback(async (id: string) => {
    if (!tabId) return;
    const node = getNodeRef.current(id);
    if (!node || !Array.isArray(node.children) || node.children.length > 0) return;

    const key = `${tabId}\n${id}`;
    const generation = (generations.current.get(key) ?? 0) + 1;
    generations.current.set(key, generation);
    const owner = {};
    spinnerOwner.current = owner;
    if (timerRef.current) clearTimeout(timerRef.current);
    setLoadingNodeId(null);
    timerRef.current = setTimeout(() => setLoadingNodeId(id), 200);
    try {
      const children = await GetChildren(tabId, node.backendId);
      if (generations.current.get(key) !== generation) return;
      const mapped = (children || []).filter((c): c is TreeNode => c !== null).map((c) => toTreeNodeData(c, node.id));
      applyRef.current(tabId, id, mapped);
    } catch {
      // Keep children [] so the row stays expandable and a retry refetches.
    } finally {
      if (spinnerOwner.current === owner) {
        if (timerRef.current) clearTimeout(timerRef.current);
        timerRef.current = null;
        spinnerOwner.current = null;
        setLoadingNodeId(null);
      }
    }
  }, [tabId, getNodeRef, applyRef]);

  return { loadingNodeId, toggle };
}

/** Recursively find a node by ID and replace its children immutably. */
export function updateNodeChildren(
  data: TreeNodeData[],
  parentId: string,
  newChildren: TreeNodeData[],
): TreeNodeData[] {
  return data.map((node) => {
    if (node.id === parentId) {
      return { ...node, children: newChildren };
    }
    if (Array.isArray(node.children) && node.children.length > 0) {
      return { ...node, children: updateNodeChildren(node.children, parentId, newChildren) };
    }
    return node;
  });
}

/** Custom row renderer for tree nodes. Handles selection, flash, and error styling. */
export function NodeRenderer({ node, style, dragHandle }: NodeRendererProps<TreeNodeData>) {
  const { loadingNodeId, flashNodeId } = useContext(RowStateContext);
  const data = node.data;
  const isError = data.error !== '';
  const isSelected = node.isSelected;
  const isInternal = node.isInternal;
  const isLoading = data.id === loadingNodeId;
  const isFlashing = data.id === flashNodeId;

  // Hide rawKey when the inline objectRef suffix is rendered. PDFBox-style
  // rows show "Pages [2 0 R]" rather than "Pages /Pages [2 0 R]" -- the
  // /<bareKey> rawKey adds nothing once the ref is visible.
  const showRawKey = data.rawKey !== '' && data.rawKey !== data.name && data.objectRef === '';

  // An array element's label IS its value, so the row shows it once, in the
  // label's place. The backend clamps that label to the CLI row's ceiling, so
  // the row renders data.value at the GUI ceiling instead: one value, one pair
  // of counts between the row and its title, and as much of it as a dictionary
  // sibling shows. An element with no value of its own (a container, or an
  // indirect ref the walker did not dereference) keeps its label.
  const labelCarriesValue = data.rawKey.startsWith('[');
  const labelShowsValue = labelCarriesValue && data.value !== '';

  // Escaped once per render rather than once for the text and once for the
  // title: data.value is uncapped, and a multi-megabyte string literal would
  // otherwise be walked twice and put whole into a DOM attribute.
  const displayValue = useMemo(
    () => clampDisplayValue(data.value, TREE_VALUE_RENDER_CAP),
    [data.value],
  );

  const rowClasses = [
    'flex items-center h-[28px] text-sm font-ui cursor-pointer',
    isFlashing ? 'bg-row-selected ring-2 ring-border-focus border-l-2 border-l-transparent' : '',
    isSelected && !isFlashing ? 'bg-row-selected border-l-2 border-l-border-focus' : '',
    !isSelected && !isFlashing ? 'border-l-2 border-l-transparent' : '',
    !isSelected && !isFlashing ? 'hover:bg-surface-hover' : '',
  ].join(' ');

  return (
    <div
      style={style}
      data-testid="tree-node"
      data-node-id={data.id}
      className={rowClasses}
      ref={dragHandle}
    >
      {/* Expand/collapse arrow */}
      <span
        className={`w-4 text-center text-text-muted flex-shrink-0 ${isLoading ? 'animate-pulse' : ''} ${node.data.id && node.isInternal ? 'cursor-pointer' : ''}`}
        {...(isLoading ? { 'data-testid': 'tree-loading-indicator' } : {})}
        onClick={(e) => {
          if (isInternal) {
            e.stopPropagation();
            node.toggle();
          }
        }}
      >
        {isInternal ? (node.isOpen ? 'v' : '>') : ''}
      </span>

      {/* Error warning icon */}
      {isError && (
        <span className="text-error mr-1 flex-shrink-0 before:content-['!']" aria-hidden="true" />
      )}

      {/* Type icon driven by backend iconHint */}
      {(() => {
        const Icon = iconForHint(data.iconHint);
        if (Icon === null) return null;
        return <Icon size={14} className="text-text-muted mr-1.5 flex-shrink-0" aria-hidden="true" />;
      })()}

      {/* Label. An array element renders its value here instead, so that span
          takes the remaining width and ellipsizes the way the value span does. */}
      <span
        className={`${labelShowsValue ? 'min-w-0 truncate' : 'whitespace-nowrap flex-shrink-0'} ${isError ? 'text-text-muted' : 'text-text'}`}
        {...(labelShowsValue ? { title: displayValue } : {})}
      >
        {labelShowsValue ? displayValue : data.name}
      </span>

      {/* Scalar value: the only element that takes the remaining width and the
          only one allowed to ellipsize. The title carries the same clamped
          string, so hovering recovers what the row WIDTH elides but not what
          the render cap dropped; past the cap, the detail panel and
          `dump tree --json` hold the whole value. */}
      {data.value !== '' && !labelCarriesValue && (
        <span
          className="text-text-muted ml-1.5 min-w-0 truncate"
          title={displayValue}
        >
          {displayValue}
        </span>
      )}

      {/* Raw key */}
      {showRawKey && (
        <span className="text-text-muted ml-1.5 text-xs whitespace-nowrap flex-shrink-0">{data.rawKey}</span>
      )}

      {/* Inline object ref [N G R] */}
      {data.objectRef !== '' && (
        <span
          className="text-text-muted ml-1.5 text-xs whitespace-nowrap flex-shrink-0"
          title={`Object ${data.objectRef}${data.typeName ? ` /Type ${data.typeName}` : ''}`}
        >
          [{data.objectRef}]
        </span>
      )}

      {/* /T:<TypeName> suffix with dedup */}
      {shouldRenderTypeSuffix(data.name, data.typeName) && (
        <span className="text-text-muted ml-1.5 text-xs whitespace-nowrap flex-shrink-0">
          /T:{data.typeName}
        </span>
      )}
    </div>
  );
}
