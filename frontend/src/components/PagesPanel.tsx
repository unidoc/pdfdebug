/**
 * @file Pages navigator: every page of the document as an expandable /Page
 * row in document order, built from GetPageIndex. Rows render through the
 * tree's NodeRenderer, expand lazily through GetChildren, and write the shared
 * selection, so the object-source pane and the detail panel follow a page
 * click exactly as they follow a tree click.
 */
import { useCallback, useEffect, useMemo, useReducer, useRef, useState, type KeyboardEvent, type MouseEvent } from 'react';
import { Tree, type NodeApi, type NodeRendererProps, type TreeApi } from 'react-arborist';
import { GetChildren, GetPageIndex } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppDispatch, useAppState, type TreeNode } from '../hooks/useDocumentState';
import { useLatest } from '../hooks/useLatest';
import type { LeftRailPanelProps } from './leftRailDestinations';
import {
  NodeRenderer,
  RowStateContext,
  deriveOpenState,
  findDisplayId,
  toTreeNodeData,
  updateNodeChildren,
  type TreeNodeData,
} from './treeRows';

/** One row of the backend page index (pdfcore.PageIndexEntry). */
interface PageIndexEntry {
  pageNum: number;
  objNum: number;
  gen: number;
  nodeId: string;
  contentNodeId: string;
  mediaBox: number[];
  rotate: number;
  inherited: number;
  annotCount: number;
  contentLen: number;
  error: string;
}

/** Per-tab Pages state: the fetched index and its arborist data. */
interface PagesCache {
  entries: PageIndexEntry[];
  data: TreeNodeData[];
  openState: Record<string, boolean>;
}

/** Open context menu: where it sits, the node it acts on, and the row to refocus. */
interface MenuState {
  x: number;
  y: number;
  backendId: string;
  returnFocus: HTMLElement | null;
}

// Builds the top-level rows. A numbered page with a node id is an expandable
// /Page row; anything else is an unnumbered leaf that carries its error as the
// row value (NodeRenderer shows only a marker for `error`). Display ids are
// prefixed by the walk index because a page listed twice repeats its node id.
function buildRows(entries: PageIndexEntry[]): TreeNodeData[] {
  return entries.map((e, i) => {
    const page = e.pageNum > 0 && e.nodeId !== '';
    const ref = e.nodeId !== '' ? `${e.objNum} ${e.gen} R` : '';
    let name: string;
    if (e.pageNum > 0) name = `${e.pageNum}: Page`;
    else name = e.nodeId !== '' ? '-: Page tree node' : '-: Kids entry';
    return {
      id: `p${i}`,
      backendId: page ? e.nodeId : '',
      name,
      children: page ? [] : null,
      rawKey: '',
      nodeType: 'dict',
      valueType: '',
      hasChildren: page,
      childCount: 0,
      iconHint: e.pageNum > 0 ? 'page' : 'default',
      error: e.error,
      objectRef: ref,
      typeName: '',
      value: e.error,
    };
  });
}

function findNode(data: TreeNodeData[], id: string): TreeNodeData | null {
  for (const n of data) {
    if (n.id === id) return n;
    if (n.children) {
      const found = findNode(n.children, id);
      if (found) return found;
    }
  }
  return null;
}

/**
 * Left-rail Pages destination. Fetches the page index for a tab the first
 * time the panel is active for it and keeps it, with the rows' expansion,
 * in a per-tab cache evicted when the tab closes.
 */
export function PagesPanel({ active }: LeftRailPanelProps) {
  const { tabs, activeTabId, pagesJumpFocusVersion } = useAppState();
  const dispatch = useAppDispatch();
  const activeTab = tabs.find((t) => t.tabId === activeTabId);
  const selectedNodeId = activeTab?.selectedNodeId ?? null;
  const selectedNodeIdRef = useLatest(selectedNodeId);

  const cache = useRef<Record<string, PagesCache>>({});
  const errors = useRef<Record<string, string>>({});
  const inflight = useRef<Set<string>>(new Set());
  // Bumped whenever the cache changes, so the render reads the new entry.
  const [, bump] = useReducer((n: number) => n + 1, 0);

  const entry = activeTabId ? cache.current[activeTabId] : undefined;
  const fetchError = activeTabId ? errors.current[activeTabId] : undefined;
  const data = entry?.data;
  const dataRef = useLatest(data);

  const treeRef = useRef<TreeApi<TreeNodeData> | undefined>(undefined);
  const containerRef = useRef<HTMLDivElement>(null);
  const [dimensions, setDimensions] = useState({ width: 0, height: 0 });
  const [loadingNodeId, setLoadingNodeId] = useState<string | null>(null);
  const [flashNodeId, setFlashNodeId] = useState<string | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const requestRef = useRef(0);

  const [jumpText, setJumpText] = useState('');
  const [jumpError, setJumpError] = useState<string | null>(null);
  const jumpRef = useRef<HTMLInputElement>(null);
  const seenFocusVersion = useRef(pagesJumpFocusVersion);

  const [menu, setMenu] = useState<MenuState | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuItemRef = useRef<HTMLButtonElement>(null);

  const hasTab = activeTab !== undefined;
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver((items) => {
      const item = items[0];
      if (item) setDimensions({ width: item.contentRect.width, height: item.contentRect.height });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [hasTab]);

  const liveTabIdsRef = useLatest(tabs.map((t) => t.tabId));

  // First activation for a tab fetches its index; later activations reuse it.
  // A failed fetch is retried the next time the panel is shown for the tab.
  useEffect(() => {
    if (!active || !activeTabId) return;
    const tabId = activeTabId;
    if (cache.current[tabId] || inflight.current.has(tabId)) return;
    inflight.current.add(tabId);
    if (errors.current[tabId] !== undefined) {
      delete errors.current[tabId];
      bump();
    }
    GetPageIndex(tabId)
      .then((result) => {
        if (!liveTabIdsRef.current.includes(tabId)) return;
        const entries = (result ?? []).filter((e): e is PageIndexEntry => e !== null);
        cache.current[tabId] = { entries, data: buildRows(entries), openState: {} };
      })
      .catch((err: unknown) => {
        if (!liveTabIdsRef.current.includes(tabId)) return;
        errors.current[tabId] = err instanceof Error ? err.message : String(err);
      })
      .finally(() => {
        inflight.current.delete(tabId);
        bump();
      });
  }, [active, activeTabId, liveTabIdsRef]);

  // Evict closed tabs. Keyed on the tab id list so it runs only when it changes.
  const tabIdKey = tabs.map((t) => t.tabId).join(',');
  useEffect(() => {
    const live = new Set(tabIdKey.split(','));
    for (const id of Object.keys(cache.current)) if (!live.has(id)) delete cache.current[id];
    for (const id of Object.keys(errors.current)) if (!live.has(id)) delete errors.current[id];
  }, [tabIdKey]);

  useEffect(() => () => {
    if (timerRef.current) clearTimeout(timerRef.current);
  }, []);

  // A focus request (Cmd+G, Navigate > Go to Page) focuses the jump field
  // with its text selected.
  useEffect(() => {
    if (pagesJumpFocusVersion === seenFocusVersion.current) return;
    seenFocusVersion.current = pagesJumpFocusVersion;
    const el = jumpRef.current;
    if (!el) return;
    el.focus();
    el.select();
  }, [pagesJumpFocusVersion]);

  const handleToggle = useCallback(async (id: string) => {
    const tabId = activeTabId;
    if (!tabId) return;
    const current = cache.current[tabId];
    const node = current ? findNode(current.data, id) : null;
    if (!node || !Array.isArray(node.children) || node.children.length > 0) return;

    if (timerRef.current) clearTimeout(timerRef.current);
    setLoadingNodeId(null);
    const generation = ++requestRef.current;
    timerRef.current = setTimeout(() => setLoadingNodeId(id), 200);
    try {
      const children = await GetChildren(tabId, node.backendId);
      if (requestRef.current !== generation) return;
      const target = cache.current[tabId];
      if (!target) return;
      const mapped = (children || []).filter((c): c is TreeNode => c !== null).map((c) => toTreeNodeData(c, node.id));
      target.data = updateNodeChildren(target.data, id, mapped);
      target.openState = deriveOpenState(target.data);
      bump();
    } catch {
      // Keep children [] so the row stays expandable and a retry refetches.
    } finally {
      if (requestRef.current === generation) {
        if (timerRef.current) clearTimeout(timerRef.current);
        timerRef.current = null;
        setLoadingNodeId(null);
      }
    }
  }, [activeTabId]);

  // The row last selected here, so a page listed twice keeps the highlight
  // on the listing the user picked rather than the first one.
  const [pickedDisplayId, setPickedDisplayId] = useState<string | null>(null);
  const selectionDisplayId = useMemo(() => {
    if (!selectedNodeId || !data) return undefined;
    if (pickedDisplayId && findNode(data, pickedDisplayId)?.backendId === selectedNodeId) return pickedDisplayId;
    return findDisplayId(data, selectedNodeId);
  }, [selectedNodeId, data, pickedDisplayId]);
  const selectionRef = useLatest(selectionDisplayId);

  // Dispatches what a tree click dispatches. A selection pushed in through the
  // `selection` prop also fires onSelect, so an id already selected is skipped.
  // Unnumbered rows are not targets: the previous selection is put back
  // without moving focus or scrolling, so arrow keys can walk past the row.
  const handleSelect = useCallback((nodes: NodeApi<TreeNodeData>[]) => {
    if (nodes.length !== 1) return;
    const node = nodes[0].data;
    if (node.backendId === '') {
      const previous = selectionRef.current ?? null;
      treeRef.current?.setSelection({ ids: previous ? [previous] : [], anchor: previous, mostRecent: previous });
      return;
    }
    setPickedDisplayId(nodes[0].id);
    if (node.backendId === selectedNodeIdRef.current) return;
    selectedNodeIdRef.current = node.backendId;
    dispatch({
      type: 'SELECT_NODE',
      payload: { nodeId: node.backendId, label: node.name, rawKey: node.rawKey, iconHint: node.iconHint },
    });
  }, [dispatch, selectedNodeIdRef, selectionRef]);

  const renderNode = useCallback(
    (props: NodeRendererProps<TreeNodeData>) => <NodeRenderer {...props} />,
    [],
  );
  const rowState = useMemo(() => ({ loadingNodeId, flashNodeId }), [loadingNodeId, flashNodeId]);

  const numbered = useMemo(() => (entry ? entry.entries.filter((e) => e.pageNum > 0) : []), [entry]);
  const pageTotal = numbered.length;

  function flash(displayId: string) {
    setFlashNodeId(displayId);
    setTimeout(() => setFlashNodeId(null), 100);
  }

  function handleJumpKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    const text = jumpText.trim();
    if (!entry) {
      setJumpError(fetchError !== undefined ? 'The page index could not be loaded.' : 'The page index is still loading.');
      return;
    }
    if (pageTotal === 0) {
      setJumpError('This document has no pages to go to.');
      return;
    }
    if (text === '') {
      setJumpError('Enter a page number.');
      return;
    }
    if (!/^-?\d+$/.test(text)) {
      setJumpError('Page number must be an integer.');
      return;
    }
    const n = Number(text);
    if (n < 1 || n > pageTotal) {
      setJumpError(`Page number out of range (1-${pageTotal}).`);
      return;
    }
    setJumpError(null);
    const rows = dataRef.current ?? [];
    const index = entry?.entries.findIndex((en) => en.pageNum === n) ?? -1;
    const row = index >= 0 ? rows[index] : undefined;
    if (!row || row.backendId === '') {
      setJumpError(`Page ${n} has no object reference to select.`);
      return;
    }
    setPickedDisplayId(row.id);
    if (row.backendId !== selectedNodeIdRef.current) {
      selectedNodeIdRef.current = row.backendId;
      dispatch({
        type: 'SELECT_NODE',
        payload: { nodeId: row.backendId, label: row.name, rawKey: row.rawKey, iconHint: row.iconHint },
      });
    }
    void treeRef.current?.scrollTo(row.id);
    flash(row.id);
  }

  const openMenu = useCallback((displayId: string, x: number, y: number, returnFocus: HTMLElement | null) => {
    const node = dataRef.current ? findNode(dataRef.current, displayId) : null;
    if (!node || node.backendId === '') return;
    treeRef.current?.select(displayId);
    setMenu({ x, y, backendId: node.backendId, returnFocus });
  }, [dataRef]);

  function handleContextMenu(e: MouseEvent<HTMLDivElement>) {
    const row = (e.target as HTMLElement).closest('[data-testid="tree-node"]');
    if (!row) return;
    e.preventDefault();
    const id = row.getAttribute('data-node-id');
    if (!id) return;
    const item = row.closest<HTMLElement>('[role="treeitem"]') ?? (row as HTMLElement);
    openMenu(id, e.clientX, e.clientY, item);
  }

  // Shift+F10 and the ContextMenu key open the menu for the focused row.
  function handleTreeKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (!((e.key === 'F10' && e.shiftKey) || e.key === 'ContextMenu')) return;
    const item = (e.target as HTMLElement).closest<HTMLElement>('[role="treeitem"]');
    const row = item?.querySelector('[data-testid="tree-node"]');
    const id = row?.getAttribute('data-node-id') ?? treeRef.current?.focusedNode?.id;
    if (!id) return;
    e.preventDefault();
    const rect = (item ?? row)?.getBoundingClientRect();
    openMenu(id, rect ? rect.left + 16 : 0, rect ? rect.bottom : 0, item ?? null);
  }

  const menuStateRef = useLatest(menu);
  const closeMenu = useCallback((restoreFocus: boolean) => {
    const returnFocus = menuStateRef.current?.returnFocus;
    setMenu(null);
    if (restoreFocus) returnFocus?.focus();
  }, [menuStateRef]);

  useEffect(() => {
    if (!menu) return;
    menuItemRef.current?.focus();
    function onKey(e: globalThis.KeyboardEvent) {
      if (e.key === 'Escape') {
        e.preventDefault();
        closeMenu(true);
      }
    }
    function onPointerDown(e: PointerEvent) {
      if (menuRef.current && e.target instanceof Node && menuRef.current.contains(e.target)) return;
      closeMenu(false);
    }
    document.addEventListener('keydown', onKey);
    document.addEventListener('pointerdown', onPointerDown);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('pointerdown', onPointerDown);
    };
  }, [menu, closeMenu]);

  // The menu acts on a row of the tab and view it was opened in, so a tab
  // switch (Cmd+Left/Right, Cmd+W) or a view change closes it.
  useEffect(() => {
    setMenu(null);
  }, [active, activeTabId]);

  // A jump error names the previous tab's page range; drop it on a tab switch.
  useEffect(() => {
    setJumpError(null);
  }, [activeTabId]);

  function showInTree() {
    if (!menu) return;
    const target = menu.backendId;
    setMenu(null);
    dispatch({ type: 'SELECT_LEFT_VIEW', payload: { view: 'structure' } });
    dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: target } });
  }

  if (!activeTab) {
    return (
      <div className="h-full flex flex-col" data-testid="pages-panel">
        <div className="px-3 py-1.5 text-sm font-medium text-text-secondary border-b border-border flex-shrink-0">Pages</div>
        <div className="px-3 py-2 text-sm text-text-muted">No document open.</div>
      </div>
    );
  }

  const countNote = (() => {
    if (!entry || pageTotal === activeTab.pageCount) return null;
    const count = activeTab.pageCount === 0 ? '/Count is 0 or unreadable' : `/Count says ${activeTab.pageCount}`;
    return `The page tree has ${pageTotal} page leaves, but ${count}.`;
  })();

  return (
    <div className="h-full flex flex-col" data-testid="pages-panel">
      <div className="px-3 py-1.5 text-sm font-medium text-text-secondary border-b border-border flex-shrink-0 flex items-baseline gap-[8px]">
        <span>Pages</span>
        {entry && (
          <span className="text-xs font-normal text-text-muted">{pageTotal === 1 ? '1 page' : `${pageTotal} pages`}</span>
        )}
      </div>
      {countNote && (
        <div className="px-3 py-1 text-xs text-warning border-b border-border flex-shrink-0">{countNote}</div>
      )}
      <div className="px-3 py-1.5 border-b border-border flex-shrink-0">
        <div className="flex items-center gap-[8px]">
          <label htmlFor="pages-jump-field" className="text-xs text-text-secondary whitespace-nowrap">Go to page</label>
          <input
            id="pages-jump-field"
            ref={jumpRef}
            type="text"
            inputMode="numeric"
            autoComplete="off"
            value={jumpText}
            placeholder={pageTotal > 0 ? `1-${pageTotal}` : undefined}
            onChange={(e) => {
              setJumpText(e.target.value);
              setJumpError(null);
            }}
            onKeyDown={handleJumpKeyDown}
            aria-invalid={jumpError !== null}
            aria-describedby={jumpError ? 'pages-jump-error' : undefined}
            className="min-w-0 flex-1 px-2 py-0.5 text-sm bg-bg text-text border border-border rounded focus:outline-none focus-visible:ring-2 focus-visible:ring-border-focus"
          />
        </div>
        {jumpError && (
          <div id="pages-jump-error" role="alert" className="mt-1 text-xs text-error">{jumpError}</div>
        )}
      </div>
      <div
        ref={containerRef}
        className="h-full w-full relative flex-1 min-h-0"
        onContextMenu={handleContextMenu}
        onKeyDown={handleTreeKeyDown}
      >
        {!entry && !fetchError && <div className="px-3 py-2 text-sm text-text-muted">Loading pages...</div>}
        {fetchError !== undefined && (
          <div className="px-3 py-2 text-sm text-error">Could not load the page index: {fetchError}</div>
        )}
        {entry && data && dimensions.width > 0 && dimensions.height > 0 && data.length > 0 && (
          <RowStateContext.Provider value={rowState}>
            <Tree<TreeNodeData>
              key={activeTabId ?? ''}
              ref={treeRef}
              data={data}
              selection={selectionDisplayId}
              onSelect={handleSelect}
              onToggle={handleToggle}
              selectionFollowsFocus={true}
              openByDefault={false}
              initialOpenState={entry.openState}
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
        {entry && entry.entries.length === 0 && (
          <div className="px-3 py-2 text-sm text-text-muted">This document has no pages.</div>
        )}
      </div>
      {menu && (
        <div
          ref={menuRef}
          role="menu"
          aria-label="Page row actions"
          className="fixed z-50 min-w-[160px] py-1 bg-surface border border-border rounded shadow-md text-sm"
          style={{ left: menu.x, top: menu.y }}
        >
          <button
            ref={menuItemRef}
            type="button"
            role="menuitem"
            onClick={showInTree}
            className="block w-full text-left px-3 py-1 text-text hover:bg-surface-hover focus:outline-none focus-visible:bg-surface-hover cursor-pointer"
          >
            Show node in tree
          </button>
        </div>
      )}
    </div>
  );
}
