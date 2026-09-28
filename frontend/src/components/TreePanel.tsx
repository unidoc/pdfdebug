/**
 * @file PDF object-tree panel. Renders the hierarchical document structure
 * using react-arborist with lazy child loading and cross-reference navigation.
 */
import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { useLatest } from '../hooks/useLatest';
import { Tree, type TreeApi, type NodeRendererProps } from 'react-arborist';
import { GetChildren, GetAncestorPath } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppState, useAppDispatch, type TreeNode } from '../hooks/useDocumentState';
import {
  RowStateContext,
  NodeRenderer,
  toTreeNodeData,
  deriveOpenState,
  findDisplayId,
  updateNodeChildren,
  type TreeNodeData,
} from './treeRows';

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

  // The arborist Tree renders only while the container has a size and there
  // is data; a reveal waits for it so its scrollTo and open calls land.
  const treeReady = dimensions.width > 0 && dimensions.height > 0 && treeData.length > 0;

  // Navigate to a cross-reference target: fetch ancestor path, expand each
  // ancestor (including intermediate dict/arr nodes), scroll to the target,
  // select it, and flash-highlight briefly. Waits for treeReady, so a reveal
  // requested while the left panel is collapsed runs once it has a size.
  // Uses a `cancelled` flag so an outdated navigation is discarded on cleanup.
  useEffect(() => {
    if (!pendingNavTarget || !activeTabId || !treeReady) return;
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
  }, [pendingNavTarget, activeTabId, treeReady, dispatch, treeDataRef]);

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
        {treeReady && (
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
