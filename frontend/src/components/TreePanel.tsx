/**
 * @file PDF object-tree panel. Renders the hierarchical document structure
 * using react-arborist with lazy child loading and cross-reference navigation.
 */
import { useState, useRef, useEffect, useCallback, useMemo, createContext, useContext } from 'react';
import { useLatest } from '../hooks/useLatest';
import { Tree, type TreeApi, type NodeRendererProps } from 'react-arborist';
import { BookOpen, FolderTree, FileText, FileCode, Image as ImageIcon, Type, type LucideIcon } from 'lucide-react';
import { GetChildren, GetAncestorPath } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppState, useAppDispatch, type TreeNode } from '../hooks/useDocumentState';

/**
 * Per-row transient state (which node is mid-load, which is flashing) delivered
 * to NodeRenderer via context rather than props. react-arborist memoizes its
 * rows and rebuilds its entire O(N) node model whenever any Tree prop identity
 * changes; passing these through the render-prop would force that rebuild on
 * every load/flash toggle. A context change instead re-renders only the row
 * consumers, leaving the Tree props (and the model) untouched.
 */
const RowStateContext = createContext<{ loadingNodeId: string | null; flashNodeId: string | null }>({
  loadingNodeId: null,
  flashNodeId: null,
});

/**
 * Map a backend iconHint to a lucide-react icon component. Returns null for
 * "default" and unknown hints so untyped scalars/arrays render without
 * decoration and the tree stays visually quiet.
 */
function iconForHint(hint: string): LucideIcon | null {
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
interface TreeNodeData {
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
}

/**
 * Convert a backend TreeNode to the react-arborist data shape.
 * parentTreeId is prepended to create a path-unique display ID so that
 * the same PDF object appearing in multiple places (e.g., /Parent back-ref)
 * gets distinct IDs and react-arborist doesn't collapse the wrong node.
 */
function toTreeNodeData(node: TreeNode, parentTreeId?: string): TreeNodeData {
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
function deriveOpenState(data: TreeNodeData[]): Record<string, boolean> {
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

/** Per-tab tree state cache entry. */
interface TabTreeCache {
  data: TreeNodeData[];
  openState: Record<string, boolean>;
}

/** Build the top-level tree data array from the root node and its pre-fetched children. */
function buildInitialData(rootNode: TreeNode, rootChildren: TreeNode[] | null): TreeNodeData[] {
  const root = toTreeNodeData(rootNode);
  root.children = rootChildren ? rootChildren.map((c) => toTreeNodeData(c, root.id)) : [];
  return [root];
}

/** Map a backend node id to its react-arborist display id by walking the tree. */
function findDisplayId(data: TreeNodeData[], backendId: string): string | undefined {
  for (const n of data) {
    if (n.backendId === backendId) return n.id;
    if (n.children) {
      const found = findDisplayId(n.children, backendId);
      if (found) return found;
    }
  }
  return undefined;
}

/** Recursively find a node by ID and replace its children immutably. */
function updateNodeChildren(
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
function NodeRenderer({ node, style, dragHandle }: NodeRendererProps<TreeNodeData>) {
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

  const rowClasses = [
    'flex items-center h-[28px] text-sm font-ui cursor-pointer',
    isFlashing ? 'bg-surface-selected ring-2 ring-border-focus border-l-2 border-l-transparent' : '',
    isSelected && !isFlashing ? 'bg-surface-selected border-l-2 border-l-border-focus' : '',
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
        <span className="text-error mr-1 flex-shrink-0" aria-hidden="true">!</span>
      )}

      {/* Type icon driven by backend iconHint */}
      {(() => {
        const Icon = iconForHint(data.iconHint);
        if (Icon === null) return null;
        return <Icon size={14} className="text-text-muted mr-1.5 flex-shrink-0" aria-hidden="true" />;
      })()}

      {/* Label */}
      <span className={`whitespace-nowrap ${isError ? 'text-text-muted' : 'text-text'}`}>{data.name}</span>

      {/* Raw key */}
      {showRawKey && (
        <span className="text-text-muted ml-1.5 text-xs">{data.rawKey}</span>
      )}

      {/* Inline object ref [N G R] (Story 9-8) */}
      {data.objectRef !== '' && (
        <span
          className="text-text-muted ml-1.5 text-xs whitespace-nowrap"
          title={`Object ${data.objectRef}${data.typeName ? ` /Type ${data.typeName}` : ''}`}
        >
          [{data.objectRef}]
        </span>
      )}

      {/* /T:<TypeName> suffix with dedup (Story 9-8) */}
      {shouldRenderTypeSuffix(data.name, data.typeName) && (
        <span className="text-text-muted ml-1.5 text-xs whitespace-nowrap">
          /T:{data.typeName}
        </span>
      )}
    </div>
  );
}

/**
 * PDF document structure tree with lazy child loading and reference navigation.
 * Children are fetched on-demand when a node is expanded. Cross-reference
 * navigation expands ancestor path, scrolls to the target, and flashes it.
 */
export function TreePanel() {
  const { tabs, activeTabId } = useAppState();
  const dispatch = useAppDispatch();
  const activeTab = tabs.find((t) => t.tabId === activeTabId);
  const rootNode = activeTab?.rootNode ?? null;
  const rootChildren = activeTab?.rootChildren ?? null;
  const selectedNodeId = activeTab?.selectedNodeId ?? null;
  const pendingNavTarget = activeTab?.pendingNavTarget ?? null;
  const navError = activeTab?.navError ?? null;

  // Per-tab cache preserves expanded tree state across tab switches.
  // Stored in a ref (not context) to avoid re-renders of every consumer.
  const treeDataCache = useRef<Record<string, TabTreeCache>>({});
  const [treeData, setTreeData] = useState<TreeNodeData[]>([]);

  // Compute openState synchronously during render so it's available when
  // react-arborist mounts (key={activeTabId} forces remount on tab switch).
  // Must be synchronous -- useEffect would fire after the first render,
  // missing the mount when initialOpenState is read.
  let openState: Record<string, boolean> = { root: true };
  if (activeTabId && treeDataCache.current[activeTabId]) {
    openState = treeDataCache.current[activeTabId].openState;
  }
  const [loadingNodeId, setLoadingNodeId] = useState<string | null>(null);
  // timerRef delays the loading spinner by 200ms to avoid flicker on fast loads
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // requestRef is a generation counter to cancel stale child-fetch responses
  const requestRef = useRef<number>(0);
  // Refs mirror state for use inside async callbacks without stale closures.
  // useLatest keeps .current synced to the latest render value (#28); the
  // imperative treeDataRef writes inside async flows below still apply because
  // useLatest returns a stable ref and each one is paired with setTreeData of
  // the same value, so render-phase mirroring never clobbers a fresher write.
  const selectedNodeIdRef = useLatest(selectedNodeId);
  const treeDataRef = useLatest(treeData);
  const treeRef = useRef<TreeApi<TreeNodeData> | undefined>(undefined);
  const [flashNodeId, setFlashNodeId] = useState<string | null>(null);

  // Container sizing for react-arborist
  const containerRef = useRef<HTMLDivElement>(null);
  const [dimensions, setDimensions] = useState({ width: 0, height: 0 });

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (entry) {
        setDimensions({
          width: entry.contentRect.width,
          height: entry.contentRect.height,
        });
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Build initial tree data from rootNode + rootChildren, or restore from cache
  useEffect(() => {
    if (!activeTabId) {
      setTreeData([]);
      treeDataRef.current = [];
      return;
    }
    const cached = treeDataCache.current[activeTabId];
    if (cached && cached.data.length > 0) {
      setTreeData(cached.data);
      treeDataRef.current = cached.data;
    } else if (rootNode) {
      const data = buildInitialData(rootNode, rootChildren);
      setTreeData(data);
      treeDataRef.current = data;
      treeDataCache.current[activeTabId] = { data, openState: { root: true } };
    } else {
      setTreeData([]);
      treeDataRef.current = [];
    }
    // treeDataRef is a stable useLatest ref; listed to satisfy exhaustive-deps.
  }, [rootNode, rootChildren, activeTabId, treeDataRef]);

  // Evict closed tabs from treeDataCache. Uses a stable string key derived
  // from tab IDs to avoid running on every reducer action (tabs is a new
  // reference on most dispatches, not just CLOSE_DOCUMENT).
  const tabIdKey = tabs.map((t) => t.tabId).join(',');
  useEffect(() => {
    const liveIds = new Set(tabs.map((t) => t.tabId));
    for (const cachedId of Object.keys(treeDataCache.current)) {
      if (!liveIds.has(cachedId)) {
        delete treeDataCache.current[cachedId];
      }
    }
  }, [tabIdKey]); // eslint-disable-line react-hooks/exhaustive-deps

  // selectedNodeIdRef is now mirrored during render via useLatest (#28); the
  // dedicated sync effect is no longer needed.

  // Cleanup pending timer on unmount
  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  // Navigate to a cross-reference target: fetch ancestor path, expand each
  // ancestor (including intermediate dict/arr nodes), scroll to the target,
  // select it, and flash-highlight briefly.
  // Uses a `cancelled` flag so an outdated navigation is discarded on cleanup.
  useEffect(() => {
    if (!pendingNavTarget || !activeTabId) return;
    let cancelled = false;

    (async () => {
      try {
        const ancestorPath = await GetAncestorPath(activeTabId, pendingNavTarget);
        if (cancelled) return;

        // Search by backendId since ancestor path contains backend IDs
        function findByBackendId(data: TreeNodeData[], backendId: string): TreeNodeData | null {
          for (const n of data) {
            if (n.backendId === backendId) return n;
            if (n.children) {
              const found = findByBackendId(n.children, backendId);
              if (found) return found;
            }
          }
          return null;
        }

        // Expand a single node: fetch children if not loaded, open in tree.
        // Updates treeDataRef directly so subsequent reads within this async
        // flow see the new children immediately (React 18 batching defers
        // the setState updater to the render phase).
        async function expandNode(node: TreeNodeData): Promise<boolean> {
          if (node.hasChildren && Array.isArray(node.children) && node.children.length === 0) {
            const children = await GetChildren(activeTabId!, node.backendId);
            if (cancelled) return false;
            const mapped = (children || []).filter((c: TreeNode | null): c is TreeNode => c !== null).map((c) => toTreeNodeData(c, node.id));
            const updated = updateNodeChildren(treeDataRef.current, node.id, mapped);
            treeDataRef.current = updated;
            if (activeTabId) {
              const os = deriveOpenState(updated);
              treeDataCache.current[activeTabId] = { data: updated, openState: os };
            }
            setTreeData(updated);
          }
          treeRef.current?.open(node.id);
          return !cancelled;
        }

        // Expand intermediate container children (dict:/arr: nodes) between
        // two obj: ancestors until nextId is reachable in the tree. The backend
        // ancestor path only contains obj: IDs but the frontend tree has
        // inline dict/arr nodes between them that also need expanding.
        async function expandIntermediates(parentBackendId: string, nextId: string): Promise<boolean> {
          if (findByBackendId(treeDataRef.current, nextId)) return true;
          const parent = findByBackendId(treeDataRef.current, parentBackendId);
          if (!parent?.children) return false;
          const queue = parent.children.filter(
            (c) => c.hasChildren && !c.backendId.startsWith('obj:')
          );
          for (const child of queue) {
            if (!(await expandNode(child))) return false;
            if (findByBackendId(treeDataRef.current, nextId)) return true;
            // Check one more level: nested inline containers
            const expanded = findByBackendId(treeDataRef.current, child.backendId);
            if (expanded?.children) {
              for (const grandchild of expanded.children) {
                if (grandchild.hasChildren && !grandchild.backendId.startsWith('obj:')) {
                  if (!(await expandNode(grandchild))) return false;
                  if (findByBackendId(treeDataRef.current, nextId)) return true;
                }
              }
            }
          }
          return !!findByBackendId(treeDataRef.current, nextId);
        }

        for (let i = 0; i < ancestorPath.length - 1; i++) {
          const ancestorBackendId = ancestorPath[i];
          const node = findByBackendId(treeDataRef.current, ancestorBackendId);
          if (!node) break;
          if (!(await expandNode(node))) return;

          // Expand intermediate dict/arr children to reach the next node
          const nextId = ancestorPath[i + 1];
          if (!findByBackendId(treeDataRef.current, nextId)) {
            await expandIntermediates(ancestorBackendId, nextId);
            if (cancelled) return;
          }
        }

        // Find target node by backendId
        const targetNode = findByBackendId(treeDataRef.current, pendingNavTarget);
        if (!targetNode) {
          // Extract object number for user-friendly message
          const parts = pendingNavTarget.split(':');
          const objNum = parts.length >= 3 ? parts[2] : pendingNavTarget;
          dispatch({ type: 'NAV_ERROR', payload: { message: `Object ${objNum} not found in the document` } });
          return;
        }

        // Scroll and select by display id
        await treeRef.current?.scrollTo(targetNode.id);
        if (cancelled) return;

        dispatch({
          type: 'SELECT_NODE',
          payload: { nodeId: targetNode.backendId, label: targetNode.name, rawKey: targetNode.rawKey, iconHint: targetNode.iconHint },
        });

        // Flash effect (delivered to rows via RowStateContext)
        setFlashNodeId(targetNode.id);
        setTimeout(() => {
          setFlashNodeId(null);
        }, 100);

        dispatch({ type: 'CLEAR_NAV_TARGET' });
      } catch (err: unknown) {
        if (cancelled) return;
        dispatch({ type: 'NAV_ERROR', payload: { message: String(err) } });
      }
    })();

    return () => { cancelled = true; };
    // treeDataRef is a stable useLatest ref; listed to satisfy exhaustive-deps.
  }, [pendingNavTarget, activeTabId, dispatch, treeDataRef]);

  // Auto-dismiss navError after 3 seconds
  useEffect(() => {
    if (!navError) return;
    const timer = setTimeout(() => {
      dispatch({ type: 'DISMISS_NAV_ERROR' });
    }, 3000);
    return () => clearTimeout(timer);
  }, [navError, dispatch]);

  /** Lazy-load children when a node is expanded for the first time. */
  const handleToggle = useCallback(async (id: string) => {
    if (!activeTabId) return;

    // Find the node in tree data to check if children need loading
    function findNode(data: TreeNodeData[], nodeId: string): TreeNodeData | null {
      for (const n of data) {
        if (n.id === nodeId) return n;
        if (n.children) {
          const found = findNode(n.children, nodeId);
          if (found) return found;
        }
      }
      return null;
    }

    const node = findNode(treeDataRef.current, id);
    if (!node) return;

    // Only fetch if opening and children haven't been loaded yet
    if (Array.isArray(node.children) && node.children.length === 0) {
      // Cancel any pending timer
      if (timerRef.current) clearTimeout(timerRef.current);
      setLoadingNodeId(null);

      const generation = ++requestRef.current;
      timerRef.current = setTimeout(() => setLoadingNodeId(id), 200);

      try {
        // Use backendId for API call, display id for tree state updates
        const children = await GetChildren(activeTabId, node.backendId);
        if (requestRef.current !== generation) return;
        const mapped = (children || []).filter((c): c is TreeNode => c !== null).map((c) => toTreeNodeData(c, node.id));
        setTreeData((prev) => {
          const updated = updateNodeChildren(prev, id, mapped);
          treeDataRef.current = updated;
          if (activeTabId) {
            const os = deriveOpenState(updated);
            treeDataCache.current[activeTabId] = { data: updated, openState: os };
          }
          return updated;
        });
      } catch {
        // Fetch failed -- keep children as [] so the node stays expandable
        // and the user can retry by toggling again.
      } finally {
        if (requestRef.current === generation) {
          if (timerRef.current) clearTimeout(timerRef.current);
          timerRef.current = null;
          setLoadingNodeId(null);
        }
      }
    }
    // treeDataRef is a stable useLatest ref; listed to satisfy exhaustive-deps.
  }, [activeTabId, treeDataRef]);

  /** Dispatch SELECT_NODE on single-node selection. Uses backendId for API calls. */
  const handleSelect = useCallback((nodes: { id: string; data: TreeNodeData }[]) => {
    if (nodes.length !== 1) return;
    const node = nodes[0];
    if (node.data.backendId === selectedNodeIdRef.current) return;
    selectedNodeIdRef.current = node.data.backendId;
    dispatch({
      type: 'SELECT_NODE',
      payload: { nodeId: node.data.backendId, label: node.data.name, rawKey: node.data.rawKey, iconHint: node.data.iconHint },
    });
    // selectedNodeIdRef is a stable useLatest ref; listed to satisfy exhaustive-deps.
  }, [dispatch, selectedNodeIdRef]);

  // Memoized so the `selection` prop keeps a stable identity across renders that
  // change neither the selection nor the tree. react-arborist rebuilds its entire
  // O(N) node model whenever ANY prop identity in treeProps changes (see its
  // provider's `api.update(treeProps)` memo), so an unstable prop forces a full
  // rebuild on every render -- pathological on large trees.
  const selectionDisplayId = useMemo(
    () => (selectedNodeId ? findDisplayId(treeData, selectedNodeId) : undefined),
    [selectedNodeId, treeData],
  );

  // Fully stable child render-prop: an inline arrow (or one keyed on loadingNodeId)
  // is a new function whenever it changes, and react-arborist rebuilds its whole
  // O(N) model on any Tree prop identity change. Per-row loading/flash now flow
  // through RowStateContext instead, so this never needs to change.
  const renderNode = useCallback(
    (props: NodeRendererProps<TreeNodeData>) => <NodeRenderer {...props} />,
    [],
  );
  const rowState = useMemo(() => ({ loadingNodeId, flashNodeId }), [loadingNodeId, flashNodeId]);

  return (
    <div className="h-full flex flex-col" data-testid="tree-panel">
      <div className="px-3 py-1.5 text-sm font-medium text-text-secondary border-b border-border flex-shrink-0">
        Document Structure
      </div>
      <div ref={containerRef} className="h-full w-full relative flex-1 min-h-0">
        {dimensions.width > 0 && dimensions.height > 0 && treeData.length > 0 && (
          <RowStateContext.Provider value={rowState}>
          <Tree<TreeNodeData>
            key={activeTabId ?? ''}
            ref={treeRef}
            data={treeData}
            selection={selectionDisplayId}
            onSelect={handleSelect}
            onToggle={handleToggle}
            selectionFollowsFocus={true}
            openByDefault={false}
            initialOpenState={openState}
            disableMultiSelection={true}
            disableDrag={true}
            disableDrop={true}
            disableEdit={true}
            rowHeight={28}
            indent={16}
            width={dimensions.width}
            height={dimensions.height}
          >
            {renderNode}
          </Tree>
          </RowStateContext.Provider>
        )}
        {navError && (
          <div
            className="absolute bottom-2 left-2 right-2 text-error text-xs bg-surface p-2 border border-error rounded"
            data-testid="nav-error-toast"
          >
            {navError}
          </div>
        )}
      </div>
    </div>
  );
}
