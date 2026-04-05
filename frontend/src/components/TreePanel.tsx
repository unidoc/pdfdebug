import { useState, useRef, useEffect, useCallback } from 'react';
import { Tree, type TreeApi, type NodeRendererProps } from 'react-arborist';
import { GetChildren, GetAncestorPath } from '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppState, useAppDispatch, type TreeNode } from '../hooks/useDocumentState';

interface TreeNodeData {
  id: string;
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

function toTreeNodeData(node: TreeNode): TreeNodeData {
  return {
    id: node.id,
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

function buildInitialData(rootNode: TreeNode, rootChildren: TreeNode[] | null): TreeNodeData[] {
  const root = toTreeNodeData(rootNode);
  root.children = rootChildren ? rootChildren.map(toTreeNodeData) : [];
  return [root];
}

// Recursively find and update a node's children immutably
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
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const requestRef = useRef<number>(0);
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

  // Navigation effect: watch pendingNavTarget
  useEffect(() => {
    if (!pendingNavTarget || !activeTabId) return;
    let cancelled = false;

    (async () => {
      try {
        const ancestorPath = await GetAncestorPath(activeTabId, pendingNavTarget);
        if (cancelled) return;

        // Load children for each ancestor and expand
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

        for (let i = 0; i < ancestorPath.length - 1; i++) {
          const ancestorId = ancestorPath[i];
          const node = findNode(treeDataRef.current, ancestorId);
          if (node && Array.isArray(node.children) && node.children.length === 0) {
            const children = await GetChildren(activeTabId, ancestorId);
            if (cancelled) return;
            const mapped = (children || []).filter((c: TreeNode | null): c is TreeNode => c !== null).map(toTreeNodeData);
            setTreeData((prev) => {
              const updated = updateNodeChildren(prev, ancestorId, mapped);
              treeDataRef.current = updated;
              return updated;
            });
          }
          treeRef.current?.open(ancestorId);
        }

        // Scroll to target
        await treeRef.current?.scrollTo(pendingNavTarget);
        if (cancelled) return;

        // Find target node data for SELECT_NODE
        const targetNode = findNode(treeDataRef.current, pendingNavTarget);
        if (!targetNode) {
          dispatch({ type: 'NAV_ERROR', payload: { message: `Target node ${pendingNavTarget} not found in tree` } });
          return;
        }

        dispatch({
          type: 'SELECT_NODE',
          payload: { nodeId: targetNode.id, label: targetNode.name, rawKey: targetNode.rawKey },
        });

        // Flash effect
        flashNodeIdRef.current = pendingNavTarget;
        setFlashNodeId(pendingNavTarget);
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
        const children = await GetChildren(activeTabId, id);
        if (requestRef.current !== generation) return;
        const mapped = (children || []).filter((c): c is TreeNode => c !== null).map(toTreeNodeData);
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

  const handleSelect = useCallback((nodes: { id: string; data: TreeNodeData }[]) => {
    if (nodes.length !== 1) return;
    const node = nodes[0];
    if (node.id === selectedNodeIdRef.current) return;
    selectedNodeIdRef.current = node.id;
    dispatch({
      type: 'SELECT_NODE',
      payload: { nodeId: node.id, label: node.data.name, rawKey: node.data.rawKey },
    });
  }, [dispatch]);

  return (
    <div className="h-full flex flex-col" data-testid="tree-panel">
      <div className="px-3 py-1.5 text-sm font-medium text-text-secondary border-b border-border flex-shrink-0">
        Document Structure
      </div>
      <div ref={containerRef} className="h-full w-full relative flex-1 min-h-0">
        {dimensions.width > 0 && dimensions.height > 0 && treeData.length > 0 && (
          <Tree<TreeNodeData>
            ref={treeRef}
            data={treeData}
            selection={selectedNodeId ?? undefined}
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
