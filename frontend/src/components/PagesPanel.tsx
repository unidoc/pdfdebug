/**
 * @file Pages navigator: every page of the document as an expandable /Page
 * row in document order, built from GetPageIndex. Rows render through the
 * tree's NodeRenderer, expand lazily through GetChildren, and write the shared
 * selection, so the object-source pane and the detail panel follow a page
 * click exactly as they follow a tree click.
 */
import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { Tree, type NodeApi, type NodeRendererProps, type TreeApi } from 'react-arborist';
import { GetPageIndex } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppDispatch, useAppState } from '../hooks/useDocumentState';
import { useLatest } from '../hooks/useLatest';
import type { LeftRailPanelProps } from './leftRailDestinations';
import { useContainerSize, useRowContextMenu, useTabCache } from './navigatorHooks';
import { RowContextMenu } from './RowContextMenu';
import {
  NodeRenderer,
  RowStateContext,
  deriveOpenState,
  findById,
  findDisplayId,
  updateNodeChildren,
  useLazyChildren,
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

  const { cache, entry, fetchError, bump } = useTabCache<PagesCache>(
    active,
    activeTabId,
    tabs.map((t) => t.tabId),
    async (tabId) => {
      const entries = ((await GetPageIndex(tabId)) ?? []).filter((e): e is PageIndexEntry => e !== null);
      return { entries, data: buildRows(entries), openState: {} };
    },
  );
  const data = entry?.data;
  const dataRef = useLatest(data);

  const treeRef = useRef<TreeApi<TreeNodeData> | undefined>(undefined);
  const containerRef = useRef<HTMLDivElement>(null);
  const [flashNodeId, setFlashNodeId] = useState<string | null>(null);
  const flashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [jumpText, setJumpText] = useState('');
  const [jumpError, setJumpError] = useState<string | null>(null);
  const jumpRef = useRef<HTMLInputElement>(null);
  const seenFocusVersion = useRef(pagesJumpFocusVersion);

  const dimensions = useContainerSize(containerRef, activeTab !== undefined);

  useEffect(() => () => {
    if (flashTimerRef.current) clearTimeout(flashTimerRef.current);
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

  const { loadingNodeId, toggle: handleToggle } = useLazyChildren(
    activeTabId,
    (id) => {
      const current = activeTabId ? cache.current[activeTabId] : undefined;
      return current ? findById(current.data, id) : null;
    },
    (tabId, id, mapped) => {
      const target = cache.current[tabId];
      if (!target) return;
      target.data = updateNodeChildren(target.data, id, mapped);
      target.openState = deriveOpenState(target.data);
      bump();
    },
  );

  // The row last selected here, so a page listed twice keeps the highlight
  // on the listing the user picked rather than the first one.
  const [pickedDisplayId, setPickedDisplayId] = useState<string | null>(null);
  const selectionDisplayId = useMemo(() => {
    if (!selectedNodeId || !data) return undefined;
    if (pickedDisplayId && findById(data, pickedDisplayId)?.backendId === selectedNodeId) return pickedDisplayId;
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
    if (flashTimerRef.current) clearTimeout(flashTimerRef.current);
    setFlashNodeId(displayId);
    flashTimerRef.current = setTimeout(() => {
      flashTimerRef.current = null;
      setFlashNodeId(null);
    }, 100);
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

  const { menu, closeMenu, onContextMenu, onKeyDown } = useRowContextMenu(
    treeRef,
    '[data-testid="tree-node"]',
    'data-node-id',
    (displayId) => {
      const node = dataRef.current ? findById(dataRef.current, displayId) : null;
      // Unnumbered rows and error children have nothing the Structure tree can reveal.
      if (!node || node.backendId === '' || node.backendId.startsWith('error:')) return null;
      return node.backendId;
    },
  );

  // The menu acts on a row of the tab and view it was opened in, so a tab
  // switch (Cmd+Left/Right, Cmd+W) or a view change closes it.
  useEffect(() => {
    closeMenu();
  }, [active, activeTabId, closeMenu]);

  // A jump error names the previous tab's page range; drop it on a tab switch.
  useEffect(() => {
    setJumpError(null);
  }, [activeTabId]);

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
        onContextMenu={onContextMenu}
        onKeyDown={onKeyDown}
      >
        {!entry && fetchError === undefined && <div className="px-3 py-2 text-sm text-text-muted">Loading pages...</div>}
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
      {menu && <RowContextMenu target={menu} label="Page row actions" onClose={closeMenu} />}
    </div>
  );
}
