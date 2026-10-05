/**
 * @file Images navigator: every image XObject referenced from the document's
 * page resources, one text row per image object built from GetImageIndex,
 * with a flat view (rows expand to the pages that use the image) and a
 * by-page view (GetImagePageGroups). Rows write the shared selection with the
 * image icon hint, so the existing image preview renders a clicked image.
 */
import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { Tree, type NodeApi, type NodeRendererProps, type TreeApi } from 'react-arborist';
import {
  GetImageIndex,
  GetImagePageGroups,
  GetImagePages,
} from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppDispatch, useAppState } from '../hooks/useDocumentState';
import { useLatest } from '../hooks/useLatest';
import { clampDisplayValue, TREE_VALUE_RENDER_CAP } from '../lib/escapeDisplayValue';
import { extractErrorMessage } from '../lib/extractErrorMessage';
import type { LeftRailPanelProps } from './leftRailDestinations';
import { useContainerSize, useRowContextMenu, useTabCache } from './navigatorHooks';
import { RowContextMenu } from './RowContextMenu';
import { ROW_IDLE, ROW_SELECTED } from './rowState';
import { findById, RowStateContext } from './treeRows';

/** One row of the backend image index (pdfcore.ImageIndexEntry). */
interface ImageEntry {
  objNum: number;
  gen: number;
  nodeId: string;
  width: number;
  height: number;
  bitsPerComponent: number;
  colorSpace: string;
  filters: string[];
  imageMask: boolean;
  smask: string | null;
  decodeNonDefault: boolean;
  sampleInterpretation: string;
  adobeMarker: string;
  adobeTransform: number | null;
  estimatedBytes: number;
  firstPage: number;
  pageCount: number;
  firstPages: number[];
  /** /Page node id of each firstPages entry; '' for a page without one. */
  firstPageNodeIds: string[];
  warning: string;
  error: string;
}

/** One page of GetImagePages (pdfcore.ImagePageRef). */
interface PageRef {
  pageNum: number;
  /** '' when the page has no /Page node id. */
  nodeId: string;
}

/** One page of GetImagePageGroups (pdfcore.ImagePageGroup). */
interface PageGroup {
  pageNum: number;
  /** '' when the page has no /Page node id. */
  nodeId: string;
  images: { objNum: number; gen: number; path: string[] }[];
  /** The walk did not finish this page; its images are only those reached. */
  incomplete?: boolean;
}

type View = 'flat' | 'byPage';
type Sort = 'firstUse' | 'objNum';

/** One arborist row: an image, a page, or an error row. */
interface ImageRowData {
  id: string;
  kind: 'image' | 'page' | 'error';
  /** Node id the row selects; '' for a row that cannot be selected. */
  nodeId: string;
  entry: ImageEntry | null;
  pageNum: number;
  /** Resource-name path of a by-page image row. */
  path: string[] | null;
  children: ImageRowData[] | null;
  /** A by-page row whose page the walk did not finish. */
  incomplete?: boolean;
  /** Text of an error row that has no entry. */
  message?: string;
}

/** Per-tab Images state: the fetched indexes, the view settings and each view's rows and expansion. */
interface ImagesCache {
  entries: ImageEntry[];
  /** Full page lists fetched for entries whose firstPages are capped, by entry index. */
  fullPages: Record<number, PageRef[]>;
  /** Failures of those fetches, by entry index. */
  pagesErrors: Record<number, string>;
  groups: PageGroup[] | null;
  groupsError: string | null;
  groupsLoading: boolean;
  view: View;
  sort: Sort;
  flatData: ImageRowData[];
  groupData: ImageRowData[] | null;
  openState: Record<View, Record<string, boolean>>;
}

const EMPTY_COPY =
  "No image XObjects are referenced from this document's page resources. Inline images (BI/ID/EI) are not listed.";

function refText(objNum: number, gen: number) {
  return `${objNum} ${gen} R`;
}

function pageRow(id: string, pageNum: number, nodeId: string, children: ImageRowData[] | null): ImageRowData {
  return { id, kind: 'page', nodeId, entry: null, pageNum, path: null, children };
}

// Flat rows in the chosen order; error rows (no node id) stay last in walk
// order. Row ids are the entry's walk index, so expansion survives a re-sort.
// An uncapped entry carries its page children; a capped one starts empty and
// is filled from GetImagePages on expansion, or holds one error row when that
// fetch failed.
function buildFlat(entries: ImageEntry[], sort: Sort, fullPages: Record<number, PageRef[]>, pagesErrors: Record<number, string>): ImageRowData[] {
  const indexed = entries.map((entry, i) => ({ entry, i }));
  const images = indexed.filter((x) => x.entry.nodeId !== '');
  const errors = indexed.filter((x) => x.entry.nodeId === '');
  if (sort === 'objNum') images.sort((a, b) => a.entry.objNum - b.entry.objNum || a.entry.gen - b.entry.gen);
  return [...images, ...errors].map(({ entry, i }) => {
    const id = `i${i}`;
    if (entry.nodeId === '') {
      return { id, kind: 'error', nodeId: '', entry, pageNum: 0, path: null, children: null };
    }
    let children: ImageRowData[] | null = null;
    if (pagesErrors[i] !== undefined) {
      children = [{ id: `${id}/err`, kind: 'error', nodeId: '', entry: null, pageNum: 0, path: null, children: null, message: `Could not load the pages: ${pagesErrors[i]}` }];
    } else if (entry.pageCount > 0) {
      const first = entry.firstPages ?? [];
      const ids = entry.firstPageNodeIds ?? [];
      const pages = fullPages[i] ?? (entry.pageCount > first.length ? [] : first.map((n, k) => ({ pageNum: n, nodeId: ids[k] ?? '' })));
      children = pages.map((p) => pageRow(`${id}/p${p.pageNum}`, p.pageNum, p.nodeId, null));
    }
    return { id, kind: 'image', nodeId: entry.nodeId, entry, pageNum: 0, path: null, children };
  });
}

// One row per numbered page that uses an image; its children are the page's
// image uses in walk order. The backend already leaves out pages with no
// images that it walked to the end, and pages after the one the walk stopped
// at; the filter here is a guard. The index's error rows follow, so a walk
// that stopped early is not read as pages with no images. Uses match entries
// by object number alone, as the backend deduplicates them, so a reference
// with another generation still finds its entry.
function buildGroups(groups: PageGroup[], entries: ImageEntry[]): ImageRowData[] {
  const byObjNum = new Map<number, ImageEntry>();
  for (const e of entries) if (e.nodeId !== '') byObjNum.set(e.objNum, e);
  const rows = groups.filter((g) => (g.images ?? []).length > 0 || g.incomplete === true).map((g) => {
    const id = `g${g.pageNum}`;
    const uses = g.images ?? [];
    const children = uses.length === 0 ? null : uses.map((u, k): ImageRowData => {
      const entry = byObjNum.get(u.objNum) ?? null;
      return {
        id: `${id}/${k}`,
        kind: 'image',
        nodeId: entry?.nodeId ?? `obj:${u.gen}:${u.objNum}`,
        entry,
        pageNum: g.pageNum,
        path: u.path ?? [],
        children: null,
      };
    });
    return { ...pageRow(id, g.pageNum, g.nodeId ?? '', children), incomplete: g.incomplete === true };
  });
  const errorRows = entries.flatMap((entry, i): ImageRowData[] =>
    entry.nodeId === '' ? [{ id: `e${i}`, kind: 'error', nodeId: '', entry, pageNum: 0, path: null, children: null }] : []);
  return [...rows, ...errorRows];
}

// Searches top-level rows and the children of open rows only, so a selection
// pushed in from elsewhere never expands a row here.
function findVisibleRow(data: ImageRowData[], open: Record<string, boolean>, match: (row: ImageRowData) => boolean): ImageRowData | null {
  for (const r of data) {
    if (match(r)) return r;
    if (r.children && open[r.id]) {
      const found = findVisibleRow(r.children, open, match);
      if (found) return found;
    }
  }
  return null;
}

function badges(e: ImageEntry): string[] {
  const out: string[] = [];
  if (e.imageMask) out.push('mask');
  if (e.smask !== null) out.push('SMask');
  if (e.decodeNonDefault) out.push('Decode');
  if (e.adobeMarker === 'present') out.push(e.adobeTransform !== null ? `APP14 t=${e.adobeTransform}` : 'APP14');
  if (e.warning !== '') out.push('warn');
  return out;
}

const clamp = (s: string) => clampDisplayValue(s, TREE_VALUE_RENDER_CAP);

/** Row renderer for image, page and error rows. */
function ImageRow({ node, style, dragHandle }: NodeRendererProps<ImageRowData>) {
  const { loadingNodeId: loadingId } = useContext(RowStateContext);
  const row = node.data;
  const rowClasses = `flex items-center h-[28px] text-sm font-ui cursor-pointer whitespace-nowrap ${node.isSelected ? ROW_SELECTED : ROW_IDLE}`;
  const chevron = (
    <span
      className={`w-4 text-center text-text-muted flex-shrink-0 ${row.id === loadingId ? 'animate-pulse' : ''}`}
      onClick={(e) => {
        if (node.isInternal) {
          e.stopPropagation();
          node.toggle();
        }
      }}
    >
      {node.isInternal ? (node.isOpen ? 'v' : '>') : ''}
    </span>
  );

  if (row.kind === 'page') {
    return (
      <div style={style} ref={dragHandle} data-testid="image-page-row" data-row-id={row.id} className={rowClasses}>
        {chevron}
        <span className="text-text">Page {row.pageNum}</span>
        {row.incomplete && (
          <>
            {' '}
            <span className="ml-1.5 text-warning" title="The image walk stopped before finishing this page; the error rows at the end say why.">
              (walk incomplete)
            </span>
          </>
        )}
      </div>
    );
  }

  const e = row.entry;
  if (row.kind === 'error' || e === null) {
    const msg = clamp(row.message ?? e?.error ?? '');
    return (
      <div style={style} ref={dragHandle} data-testid="image-row" data-row-id={row.id} className={rowClasses} title={msg}>
        {chevron}
        <span className="text-text-muted">-</span>{' '}
        <span className="ml-1.5 min-w-0 truncate text-error">{row.kind === 'error' ? msg : row.nodeId}</span>
      </div>
    );
  }

  const filters = e.filters && e.filters.length > 0 ? clamp(e.filters.join(',')) : '-';
  const usage = row.path !== null
    ? clamp(row.path.join(' > '))
    : `used on ${e.pageCount} ${e.pageCount === 1 ? 'page' : 'pages'}`;
  const title = [e.sampleInterpretation, e.warning].filter((s) => s !== '').map(clamp).join('\n');
  return (
    <div
      style={style}
      ref={dragHandle}
      data-testid="image-row"
      data-row-id={row.id}
      className={rowClasses}
      title={title || undefined}
    >
      {chevron}
      <span className="w-[88px] flex-shrink-0 text-text">{refText(e.objNum, e.gen)}</span>{' '}
      <span className="w-[80px] flex-shrink-0 text-text-secondary">{e.width}x{e.height}</span>{' '}
      <span className="w-[96px] flex-shrink-0 truncate text-text-secondary">{filters}</span>{' '}
      <span className="w-[88px] flex-shrink-0 truncate text-text-secondary">{e.colorSpace !== '' ? clamp(e.colorSpace) : '-'}</span>{' '}
      <span className="flex-shrink-0 text-text-muted">{usage}</span>
      {badges(e).map((b) => (
        <span key={b}>
          {' '}
          <span className="ml-1.5 px-1 text-xs rounded border border-border text-text-secondary">{b}</span>
        </span>
      ))}
      {e.error !== '' && (
        <>
          {' '}
          <span className="ml-1.5 min-w-0 truncate text-error">{clamp(e.error)}</span>
        </>
      )}
    </div>
  );
}

/**
 * Left-rail Images destination. Fetches the image index for a tab the first
 * time the panel is active for it, and keeps it, the view, the sort and each
 * view's expansion in a per-tab cache evicted when the tab closes.
 */
export function ImagesPanel({ active }: LeftRailPanelProps) {
  const { tabs, activeTabId } = useAppState();
  const dispatch = useAppDispatch();
  const activeTab = tabs.find((t) => t.tabId === activeTabId);
  const selectedNodeId = activeTab?.selectedNodeId ?? null;
  const selectedNodeIdRef = useLatest(selectedNodeId);

  const { cache, entry, fetchError, bump, tabIdKey } = useTabCache<ImagesCache>(
    active,
    activeTabId,
    tabs.map((t) => t.tabId),
    async (tabId) => {
      const entries: ImageEntry[] = ((await GetImageIndex(tabId)) ?? []).filter((e) => e !== null);
      return {
        entries,
        fullPages: {},
        pagesErrors: {},
        groups: null,
        groupsError: null,
        groupsLoading: false,
        view: 'flat',
        sort: 'firstUse',
        flatData: buildFlat(entries, 'firstUse', {}, {}),
        groupData: null,
        openState: { flat: {}, byPage: {} },
      };
    },
  );
  const view: View = entry?.view ?? 'flat';
  const data = entry ? (view === 'flat' ? entry.flatData : entry.groupData) : null;
  const dataRef = useLatest(data);

  const treeRef = useRef<TreeApi<ImageRowData> | undefined>(undefined);
  const containerRef = useRef<HTMLDivElement>(null);
  const dimensions = useContainerSize(containerRef, activeTab !== undefined);

  // Page lists past the first-pages cap: the latest request number per
  // `${tabId}\n${rowId}`, so a late answer lands on the tab that asked and only
  // the latest request applies. Numbers come from one counter, so they never
  // repeat even after a key is evicted.
  const generations = useRef(new Map<string, number>());
  const lastGeneration = useRef(0);

  // Drop the page-list requests of closed tabs.
  useEffect(() => {
    const live = new Set(tabIdKey.split(','));
    for (const key of generations.current.keys()) {
      if (!live.has(key.slice(0, key.indexOf('\n')))) generations.current.delete(key);
    }
  }, [tabIdKey]);

  // The spinner shows after 200ms for the most recent request.
  const [loadingRow, setLoadingRow] = useState<{ tabId: string; rowId: string } | null>(null);
  const spinnerTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const spinnerOwner = useRef<object | null>(null);
  useEffect(() => () => {
    if (spinnerTimer.current) clearTimeout(spinnerTimer.current);
  }, []);

  const loadPages = useCallback(async (tabId: string, rowId: string, index: number, objNum: number) => {
    const key = `${tabId}\n${rowId}`;
    const generation = ++lastGeneration.current;
    generations.current.set(key, generation);
    const owner = {};
    spinnerOwner.current = owner;
    if (spinnerTimer.current) clearTimeout(spinnerTimer.current);
    setLoadingRow(null);
    spinnerTimer.current = setTimeout(() => setLoadingRow({ tabId, rowId }), 200);
    try {
      const pages: PageRef[] = ((await GetImagePages(tabId, objNum)) ?? []).filter((p) => p !== null);
      if (generations.current.get(key) !== generation) return;
      const target = cache.current[tabId];
      if (!target) return;
      target.fullPages = { ...target.fullPages, [index]: pages };
      target.flatData = buildFlat(target.entries, target.sort, target.fullPages, target.pagesErrors);
      bump();
    } catch (err: unknown) {
      // The row shows the failure as its child; expanding it again refetches.
      if (generations.current.get(key) !== generation) return;
      const target = cache.current[tabId];
      if (!target) return;
      target.pagesErrors = { ...target.pagesErrors, [index]: extractErrorMessage(err) };
      target.flatData = buildFlat(target.entries, target.sort, target.fullPages, target.pagesErrors);
      bump();
    } finally {
      if (spinnerOwner.current === owner) {
        if (spinnerTimer.current) clearTimeout(spinnerTimer.current);
        spinnerTimer.current = null;
        spinnerOwner.current = null;
        setLoadingRow(null);
      }
    }
  }, [cache, bump]);

  const handleToggle = useCallback((id: string) => {
    if (!activeTabId) return;
    const current = cache.current[activeTabId];
    const api = treeRef.current;
    if (!current || !api) return;
    current.openState[current.view] = { ...api.openState };
    // Re-render so the selection lookup sees the rows this toggle revealed or hid.
    bump();
    if (current.view !== 'flat' || !api.isOpen(id)) return;
    const row = findById(current.flatData, id);
    const e = row?.entry;
    if (!row || !e || row.kind !== 'image' || !row.children) return;
    const index = Number(id.slice(1));
    // Expanding a row whose fetch failed clears the failure and fetches again.
    if (current.pagesErrors[index] !== undefined) {
      const { [index]: _failed, ...rest } = current.pagesErrors;
      current.pagesErrors = rest;
      current.flatData = buildFlat(current.entries, current.sort, current.fullPages, rest);
    } else if (row.children.length > 0) {
      return;
    }
    void loadPages(activeTabId, id, index, e.objNum);
  }, [activeTabId, loadPages, cache, bump]);

  // The row last selected here, so an image listed under two pages keeps the
  // highlight on the listing the user picked rather than the first one. Only a
  // row already visible is highlighted, the picked one included: arborist
  // opens the ancestors of a selected row, so matching a collapsed child would
  // expand it.
  const [pickedDisplayId, setPickedDisplayId] = useState<string | null>(null);
  const openState = entry?.openState[view];
  const selectionDisplayId = useMemo(() => {
    if (!selectedNodeId || !data) return undefined;
    const open = openState ?? {};
    if (pickedDisplayId && findVisibleRow(data, open, (r) => r.id === pickedDisplayId)?.nodeId === selectedNodeId) return pickedDisplayId;
    return findVisibleRow(data, open, (r) => r.nodeId === selectedNodeId)?.id;
  }, [selectedNodeId, data, pickedDisplayId, openState]);
  const selectionRef = useLatest(selectionDisplayId);

  // Dispatches what a tree click dispatches. A selection pushed in through the
  // `selection` prop also fires onSelect, so an id already selected is skipped.
  // A row with no node id is not a target: the previous selection is put back
  // without moving focus, so arrow keys can walk past the row.
  const handleSelect = useCallback((nodes: NodeApi<ImageRowData>[]) => {
    if (nodes.length !== 1) return;
    const row = nodes[0].data;
    if (row.nodeId === '') {
      const previous = selectionRef.current ?? null;
      treeRef.current?.setSelection({ ids: previous ? [previous] : [], anchor: previous, mostRecent: previous });
      return;
    }
    setPickedDisplayId(row.id);
    if (row.nodeId === selectedNodeIdRef.current) return;
    selectedNodeIdRef.current = row.nodeId;
    const payload = row.kind === 'page'
      ? { nodeId: row.nodeId, label: `Page ${row.pageNum}`, rawKey: '', iconHint: 'page' }
      : { nodeId: row.nodeId, label: `Image ${row.entry ? refText(row.entry.objNum, row.entry.gen) : row.nodeId}`, rawKey: '', iconHint: 'image' };
    dispatch({ type: 'SELECT_NODE', payload });
  }, [dispatch, selectedNodeIdRef, selectionRef]);

  const renderRow = useCallback((props: NodeRendererProps<ImageRowData>) => <ImageRow {...props} />, []);
  // The spinner belongs to the tab whose request is pending.
  const loadingNodeId = loadingRow && loadingRow.tabId === activeTabId ? loadingRow.rowId : null;
  const rowState = useMemo(() => ({ loadingNodeId, flashNodeId: null }), [loadingNodeId]);

  function setView(next: View) {
    if (!activeTabId) return;
    const tabId = activeTabId;
    const current = cache.current[tabId];
    if (!current || current.view === next) return;
    current.view = next;
    bump();
    if (next !== 'byPage' || current.groups || current.groupsLoading) return;
    current.groupsLoading = true;
    current.groupsError = null;
    GetImagePageGroups(tabId)
      .then((groups) => {
        const target = cache.current[tabId];
        if (!target) return;
        target.groups = (groups ?? []).filter((g) => g !== null);
        target.groupData = buildGroups(target.groups, target.entries);
      })
      .catch((err: unknown) => {
        const target = cache.current[tabId];
        if (target) target.groupsError = extractErrorMessage(err);
      })
      .finally(() => {
        const target = cache.current[tabId];
        if (target) target.groupsLoading = false;
        bump();
      });
  }

  function setSort(next: Sort) {
    const current = activeTabId ? cache.current[activeTabId] : undefined;
    if (!current || current.sort === next) return;
    current.sort = next;
    current.flatData = buildFlat(current.entries, next, current.fullPages, current.pagesErrors);
    bump();
  }

  const { menu, closeMenu, onContextMenu, onKeyDown } = useRowContextMenu(
    treeRef,
    '[data-row-id]',
    'data-row-id',
    (rowId) => {
      const row = dataRef.current ? findById(dataRef.current, rowId) : null;
      // Error rows and pages without a node id have nothing to reveal.
      return row && row.nodeId !== '' ? row.nodeId : null;
    },
  );

  // The menu acts on a row of the tab and view it was opened in, so a tab
  // switch, a rail switch or a view change closes it.
  useEffect(() => {
    closeMenu();
  }, [active, activeTabId, view, closeMenu]);

  if (!activeTab) {
    return (
      <div className="h-full flex flex-col" data-testid="images-panel">
        <div className="px-3 py-1.5 text-sm font-medium text-text-secondary border-b border-border flex-shrink-0">Images</div>
        <div className="px-3 py-2 text-sm text-text-muted">No document open.</div>
      </div>
    );
  }

  const imageCount = entry ? entry.entries.filter((e) => e.nodeId !== '').length : 0;
  // A pressed toggle uses the armed style so it reads differently from hover.
  const toggleClass = (on: boolean) =>
    `text-xs rounded border cursor-pointer px-2 py-0.5 ${on ? 'bg-surface-armed border-border-focus text-text' : 'border-border text-text-secondary hover:bg-surface-hover'}`;

  return (
    <div className="h-full flex flex-col" data-testid="images-panel">
      <div className="px-3 py-1.5 text-sm font-medium text-text-secondary border-b border-border flex-shrink-0 flex items-baseline gap-[8px]">
        <span>Images</span>
        {entry && (
          <>
            {' '}
            <span className="text-xs font-normal text-text-muted">{imageCount === 1 ? '1 image' : `${imageCount} images`}</span>{' '}
          </>
        )}
      </div>
      {entry && (
        <div className="px-3 py-1.5 border-b border-border flex-shrink-0 flex flex-wrap items-center gap-x-[16px] gap-y-[6px]">
          <div className="flex items-center gap-[4px]" role="group" aria-label="View">
            <span className="text-xs text-text-muted mr-[2px]" aria-hidden="true">View</span>
            <button type="button" aria-pressed={view === 'flat'} className={toggleClass(view === 'flat')} onClick={() => setView('flat')}>Flat</button>
            <button type="button" aria-pressed={view === 'byPage'} className={toggleClass(view === 'byPage')} onClick={() => setView('byPage')}>By page</button>
          </div>
          {view === 'flat' && (
            <div className="flex items-center gap-[4px]" role="group" aria-label="Sort">
              <span className="text-xs text-text-muted mr-[2px]" aria-hidden="true">Sort</span>
              <button type="button" aria-pressed={entry.sort === 'firstUse'} className={toggleClass(entry.sort === 'firstUse')} title="By the first page that uses each image, then object number" onClick={() => setSort('firstUse')}>First use</button>
              <button type="button" aria-pressed={entry.sort === 'objNum'} className={toggleClass(entry.sort === 'objNum')} title="By object number" onClick={() => setSort('objNum')}>Object number</button>
            </div>
          )}
        </div>
      )}
      <div
        ref={containerRef}
        className="h-full w-full relative flex-1 min-h-0"
        onContextMenu={onContextMenu}
        onKeyDown={onKeyDown}
      >
        {!entry && fetchError === undefined && <div className="px-3 py-2 text-sm text-text-muted">Loading images...</div>}
        {fetchError !== undefined && (
          <div className="px-3 py-2 text-sm text-error">Could not load the image index: {fetchError}</div>
        )}
        {entry && data !== null && data.length === 0 && (
          <div className="px-3 py-2 text-sm text-text-muted">{EMPTY_COPY}</div>
        )}
        {entry && view === 'byPage' && entry.groupsLoading && (
          <div className="px-3 py-2 text-sm text-text-muted">Loading pages...</div>
        )}
        {entry && view === 'byPage' && entry.groupsError !== null && !entry.groupsLoading && (
          <div className="px-3 py-2 text-sm text-error">Could not load the images by page: {entry.groupsError}</div>
        )}
        {entry && data && data.length > 0 && dimensions.width > 0 && dimensions.height > 0 && (
          <RowStateContext.Provider value={rowState}>
            <Tree<ImageRowData>
              key={`${activeTabId}:${view}`}
              ref={treeRef}
              data={data}
              selection={selectionDisplayId}
              onSelect={handleSelect}
              onToggle={handleToggle}
              selectionFollowsFocus={true}
              openByDefault={false}
              initialOpenState={entry.openState[view]}
              disableMultiSelection={true}
              disableDrag={true}
              disableDrop={true}
              disableEdit={true}
              rowHeight={28}
              indent={16}
              width={dimensions.width}
              height={dimensions.height}
            >
              {renderRow}
            </Tree>
          </RowStateContext.Provider>
        )}
      </div>
      {menu && <RowContextMenu target={menu} label="Image row actions" onClose={closeMenu} />}
    </div>
  );
}
