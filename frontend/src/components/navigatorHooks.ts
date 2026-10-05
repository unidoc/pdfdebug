/**
 * @file Hooks shared by the left-rail navigator panels: the per-tab fetch
 * cache, the pane size their Tree is drawn at, and the triggers that open the
 * row context menu.
 */
import { useCallback, useEffect, useReducer, useRef, useState, type KeyboardEvent, type MouseEvent, type RefObject } from 'react';
import type { TreeApi } from 'react-arborist';
import { useLatest } from '../hooks/useLatest';
import { extractErrorMessage } from '../lib/extractErrorMessage';
import type { RowMenuTarget } from './RowContextMenu';

/**
 * Per-tab fetch cache for a navigator panel. The first time the panel is
 * active for a tab, `load` builds that tab's entry; later activations reuse
 * it. A failed load keeps its message and is retried the next time the panel
 * is shown for the tab. Entries, errors and pending loads of closed tabs are
 * dropped, and a load still pending when its tab closes is discarded. A
 * caller that changes an entry in `cache` calls `bump` to re-render.
 */
export function useTabCache<T>(active: boolean, activeTabId: string | null, tabIds: string[], load: (tabId: string) => Promise<T>) {
  const cache = useRef<Record<string, T>>({});
  const errors = useRef<Record<string, string>>({});
  // The pending load per tab, so a result lands only if its load is still current.
  const inflight = useRef(new Map<string, object>());
  const [, bump] = useReducer((n: number) => n + 1, 0);
  const loadRef = useLatest(load);

  useEffect(() => {
    if (!active || !activeTabId) return;
    const tabId = activeTabId;
    if (cache.current[tabId] || inflight.current.has(tabId)) return;
    const token = {};
    inflight.current.set(tabId, token);
    if (errors.current[tabId] !== undefined) {
      delete errors.current[tabId];
      bump();
    }
    loadRef.current(tabId)
      .then((value) => {
        if (inflight.current.get(tabId) === token) cache.current[tabId] = value;
      })
      .catch((err: unknown) => {
        if (inflight.current.get(tabId) === token) errors.current[tabId] = extractErrorMessage(err);
      })
      .finally(() => {
        if (inflight.current.get(tabId) === token) inflight.current.delete(tabId);
        bump();
      });
  }, [active, activeTabId, loadRef]);

  // Keyed on the tab id list so eviction runs only when it changes.
  const tabIdKey = tabIds.join(',');
  useEffect(() => {
    const live = new Set(tabIdKey.split(','));
    for (const id of Object.keys(cache.current)) if (!live.has(id)) delete cache.current[id];
    for (const id of Object.keys(errors.current)) if (!live.has(id)) delete errors.current[id];
    for (const id of inflight.current.keys()) if (!live.has(id)) inflight.current.delete(id);
  }, [tabIdKey]);

  return {
    cache,
    entry: activeTabId ? cache.current[activeTabId] : undefined,
    fetchError: activeTabId ? errors.current[activeTabId] : undefined,
    bump,
  };
}

/**
 * Tracks the content size of the element in `ref`, keeping the last non-zero
 * size so a collapsed pane leaves the Tree mounted. The observer is attached
 * again whenever `remountKey` changes, since the element may have been swapped.
 */
export function useContainerSize(ref: RefObject<HTMLElement | null>, remountKey: unknown) {
  const [size, setSize] = useState({ width: 0, height: 0 });
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver((items) => {
      const item = items[0];
      if (!item) return;
      const { width, height } = item.contentRect;
      if (width > 0 && height > 0) setSize({ width, height });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref, remountKey]);
  return size;
}

/**
 * Opens a RowContextMenu for a navigator row from a right-click or from
 * Shift+F10 / the ContextMenu key on the focused row. Rows are found by
 * `rowSelector` and identified by their `rowIdAttribute`, which holds the
 * arborist row id. `resolveNodeId` maps that id to the node the menu reveals,
 * or null when the row has nothing to reveal; the menu then stays closed.
 */
export function useRowContextMenu<T>(
  treeRef: RefObject<TreeApi<T> | undefined>,
  rowSelector: string,
  rowIdAttribute: string,
  resolveNodeId: (rowId: string) => string | null,
) {
  const [menu, setMenu] = useState<RowMenuTarget | null>(null);
  const resolveRef = useLatest(resolveNodeId);

  const openMenu = useCallback((rowId: string, x: number, y: number, returnFocus: HTMLElement | null) => {
    const nodeId = resolveRef.current(rowId);
    if (!nodeId) return;
    treeRef.current?.select(rowId);
    setMenu({ x, y, nodeId, returnFocus });
  }, [resolveRef, treeRef]);

  const onContextMenu = useCallback((e: MouseEvent<HTMLElement>) => {
    const row = (e.target as HTMLElement).closest(rowSelector);
    if (!row) return;
    e.preventDefault();
    const id = row.getAttribute(rowIdAttribute);
    if (!id) return;
    const item = row.closest<HTMLElement>('[role="treeitem"]') ?? (row as HTMLElement);
    openMenu(id, e.clientX, e.clientY, item);
  }, [openMenu, rowSelector, rowIdAttribute]);

  const onKeyDown = useCallback((e: KeyboardEvent<HTMLElement>) => {
    if (!((e.key === 'F10' && e.shiftKey) || e.key === 'ContextMenu')) return;
    const item = (e.target as HTMLElement).closest<HTMLElement>('[role="treeitem"]');
    const row = item?.querySelector(rowSelector);
    const id = row?.getAttribute(rowIdAttribute) ?? treeRef.current?.focusedNode?.id;
    if (!id) return;
    e.preventDefault();
    const rect = (item ?? row)?.getBoundingClientRect();
    openMenu(id, rect ? rect.left + 16 : 0, rect ? rect.bottom : 0, item ?? null);
  }, [openMenu, rowSelector, rowIdAttribute, treeRef]);

  const closeMenu = useCallback(() => setMenu(null), []);

  return { menu, closeMenu, onContextMenu, onKeyDown };
}
