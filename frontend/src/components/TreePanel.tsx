/**
 * @file PDF object-tree panel. Renders the hierarchical document structure
 * using react-arborist with lazy child loading and cross-reference navigation.
 */
import { useState, useRef, useEffect, useCallback } from 'react';
import { Tree, type TreeApi, type NodeRendererProps } from 'react-arborist';
import { GetChildren, GetAncestorPath } from '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppState, useAppDispatch, type TreeNode } from '../hooks/useDocumentState';

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
    children: node.hasChildren ? [] : null,
  };
}

/** Build the top-level tree data array from the root node and its pre-fetched children. */
function buildInitialData(rootNode: TreeNode, rootChildren: TreeNode[] | null): TreeNodeData[] {
  const root = toTreeNodeData(rootNode);
  root.children = rootChildren ? rootChildren.map((c) => toTreeNodeData(c, root.id)) : [];
  return [root];
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
function NodeRenderer({ node, style, dragHandle, isLoading, flashNodeIdRef }: NodeRendererProps<TreeNodeData> & { isLoading?: boolean; flashNodeIdRef?: React.RefObject<string | null> }) {
  const data = node.data;
  const isError = data.error !== '';
  const isSelected = node.isSelected;
  const isInternal = node.isInternal;
  const isFlashing = data.id === flashNodeIdRef?.current;

  const showRawKey = data.rawKey !== '' && data.rawKey !== data.name;

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

      {/* Label */}
      <span className={isError ? 'text-text-muted' : 'text-text'}>{data.name}</span>

      {/* Raw key */}
      {showRawKey && (
        <span className="text-text-muted ml-1.5 text-xs">{data.rawKey}</span>
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

  const [treeData, setTreeData] = useState<TreeNodeData[]>([]);
  const [initialOpenState] = useState<Record<string, boolean>>(() => ({ root: true }));
  const [loadingNodeId, setLoadingNodeId] = useState<string | null>(null);
  // timerRef delays the loading spinner by 200ms to avoid flicker on fast loads
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // requestRef is a generation counter to cancel stale child-fetch responses
  const requestRef = useRef<number>(0);
  // Refs mirror state for use inside async callbacks without stale closures
  const selectedNodeIdRef = useRef<string | null>(null);
  const treeDataRef = useRef<TreeNodeData[]>([]);
  const treeRef = useRef<TreeApi<TreeNodeData> | undefined>(undefined);
  const [flashNodeId, setFlashNodeId] = useState<string | null>(null);
  const flashNodeIdRef = useRef<string | null>(null);

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

  // Build initial tree data from rootNode + rootChildren
  useEffect(() => {
    if (rootNode) {
      const data = buildInitialData(rootNode, rootChildren);
      setTreeData(data);
      treeDataRef.current = data;
    } else {
      setTreeData([]);
      treeDataRef.current = [];
    }
  }, [rootNode, rootChildren]);

  // Keep ref in sync
  useEffect(() => {
    selectedNodeIdRef.current = selectedNodeId;
  }, [selectedNodeId]);

  // Cleanup pending timer on unmount
  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  // Navigate to a cross-reference target: fetch ancestor path, expand each
  // ancestor, scroll to the node, select it, and flash-highlight briefly.
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

        for (let i = 0; i < ancestorPath.length - 1; i++) {
          const ancestorBackendId = ancestorPath[i];
          const node = findByBackendId(treeDataRef.current, ancestorBackendId);
          if (node && Array.isArray(node.children) && node.children.length === 0) {
            const children = await GetChildren(activeTabId, ancestorBackendId);
            if (cancelled) return;
            const mapped = (children || []).filter((c: TreeNode | null): c is TreeNode => c !== null).map((c) => toTreeNodeData(c, node.id));
            setTreeData((prev) => {
              const updated = updateNodeChildren(prev, node.id, mapped);
              treeDataRef.current = updated;
              return updated;
            });
          }
          // Open by display id (react-arborist uses display ids)
          if (node) treeRef.current?.open(node.id);
        }

        // Find target node by backendId
        const targetNode = findByBackendId(treeDataRef.current, pendingNavTarget);
        if (!targetNode) {
          dispatch({ type: 'NAV_ERROR', payload: { message: `Target node ${pendingNavTarget} not found in tree` } });
          return;
        }

        // Scroll and select by display id
        await treeRef.current?.scrollTo(targetNode.id);
        if (cancelled) return;

        dispatch({
          type: 'SELECT_NODE',
          payload: { nodeId: targetNode.backendId, label: targetNode.name, rawKey: targetNode.rawKey },
        });

        // Flash effect
        flashNodeIdRef.current = targetNode.id;
        setFlashNodeId(targetNode.id);
        setTimeout(() => {
          flashNodeIdRef.current = null;
          setFlashNodeId(null);
        }, 100);

        dispatch({ type: 'CLEAR_NAV_TARGET' });
      } catch (err: unknown) {
        if (cancelled) return;
        dispatch({ type: 'NAV_ERROR', payload: { message: String(err) } });
      }
    })();

    return () => { cancelled = true; };
  }, [pendingNavTarget, activeTabId, dispatch]);

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
  }, [activeTabId]);

  /** Dispatch SELECT_NODE on single-node selection. Uses backendId for API calls. */
  const handleSelect = useCallback((nodes: { id: string; data: TreeNodeData }[]) => {
    if (nodes.length !== 1) return;
    const node = nodes[0];
    if (node.data.backendId === selectedNodeIdRef.current) return;
    selectedNodeIdRef.current = node.data.backendId;
    dispatch({
      type: 'SELECT_NODE',
      payload: { nodeId: node.data.backendId, label: node.data.name, rawKey: node.data.rawKey },
    });
  }, [dispatch]);

  // Map backendId to display id for react-arborist's selection prop
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
  const selectionDisplayId = selectedNodeId ? findDisplayId(treeData, selectedNodeId) : undefined;

  return (
    <div className="h-full flex flex-col" data-testid="tree-panel">
      <div className="px-3 py-1.5 text-sm font-medium text-text-secondary border-b border-border flex-shrink-0">
        Document Structure
      </div>
      <div ref={containerRef} className="h-full w-full relative flex-1 min-h-0">
        {dimensions.width > 0 && dimensions.height > 0 && treeData.length > 0 && (
          <Tree<TreeNodeData>
            key={activeTabId ?? ''}
            ref={treeRef}
            data={treeData}
            selection={selectionDisplayId}
            onSelect={handleSelect}
            onToggle={handleToggle}
            selectionFollowsFocus={true}
            openByDefault={false}
            initialOpenState={initialOpenState}
            disableMultiSelection={true}
            disableDrag={true}
            disableDrop={true}
            disableEdit={true}
            rowHeight={28}
            indent={16}
            width={dimensions.width}
            height={dimensions.height}
          >
            {(props: NodeRendererProps<TreeNodeData>) => (
              <NodeRenderer {...props} isLoading={loadingNodeId === props.node.id} flashNodeIdRef={flashNodeIdRef} />
            )}
          </Tree>
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
