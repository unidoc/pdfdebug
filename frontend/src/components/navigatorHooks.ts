/**
 * @file Hooks shared by the left-rail navigator panels: the per-tab fetch
 * cache, the pane size their Tree is drawn at, and the native row context
 * menu.
 */
import { useCallback, useEffect, useReducer, useRef, useState, type KeyboardEvent, type MouseEvent, type RefObject } from 'react';
import type { TreeApi } from 'react-arborist';
import { useLatest } from '../hooks/useLatest';
import { extractErrorMessage } from '../lib/extractErrorMessage';
import { Events } from '@wailsio/runtime';
import { RowMenuAction } from '../../bindings/events/index.js';

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

/** Native row menus registered by the Go side (rowmenus.go). */
export const ROW_MENU_REVEAL = 'row-reveal';
export const ROW_MENU_TREE = 'tree-row';
export const ROW_MENU_TREE_WITH_REF = 'tree-row-ref';

/** Actions the native row menu items send back. */
export type RowMenuActionName = 'reveal' | 'copy-ref' | 'copy-path';

/** The menu a row opens and the node its actions apply to. */
export interface RowMenuHit {
  menu: string;
  nodeId: string;
}

// Shared by every panel, so a menu action names exactly one opened row.
let nextRowMenuToken = 0;

/**
 * Opens the native row menu for a navigator row from a right-click or from
 * Shift+F10 / the ContextMenu key on the focused row. Rows are found by
 * `rowSelector` and identified by their `rowIdAttribute`, which holds the
 * arborist row id. `resolve` maps that id to the menu and the node it acts on,
 * or null when the row has no menu.
 *
 * The Wails runtime opens a menu on the DOM contextmenu event from the
 * --custom-contextmenu CSS property of the event target, so the handler sets
 * that property on the container, with a fresh token as the menu data. The
 * keyboard triggers dispatch a contextmenu event at the row. A clicked item
 * comes back as a row-menu:action event and calls `onAction` when its token
 * is the one this panel opened last. `closeMenu` forgets that row.
 */
export function useRowContextMenu<T>(
  treeRef: RefObject<TreeApi<T> | undefined>,
  rowSelector: string,
  rowIdAttribute: string,
  resolve: (rowId: string) => RowMenuHit | null,
  onAction: (action: RowMenuActionName, nodeId: string) => void,
) {
  const pending = useRef<{ token: string; nodeId: string } | null>(null);
  const resolveRef = useLatest(resolve);
  const onActionRef = useLatest(onAction);

  useEffect(() => Events.On(RowMenuAction, (ev) => {
    const { action, token } = ev.data;
    const p = pending.current;
    if (!p || p.token !== token.trim()) return;
    pending.current = null;
    onActionRef.current(action as RowMenuActionName, p.nodeId);
  }), [onActionRef]);

  const onContextMenu = useCallback((e: MouseEvent<HTMLElement>) => {
    const container = e.currentTarget;
    container.style.removeProperty('--custom-contextmenu');
    container.style.removeProperty('--custom-contextmenu-data');
    const id = (e.target as HTMLElement).closest(rowSelector)?.getAttribute(rowIdAttribute);
    const hit = id ? resolveRef.current(id) : null;
    if (!id || !hit) return;
    treeRef.current?.select(id);
    const token = String(++nextRowMenuToken);
    pending.current = { token, nodeId: hit.nodeId };
    container.style.setProperty('--custom-contextmenu', hit.menu);
    container.style.setProperty('--custom-contextmenu-data', token);
  }, [resolveRef, treeRef, rowSelector, rowIdAttribute]);

  const onKeyDown = useCallback((e: KeyboardEvent<HTMLElement>) => {
    if (!((e.key === 'F10' && e.shiftKey) || e.key === 'ContextMenu')) return;
    const item = (e.target as HTMLElement).closest<HTMLElement>('[role="treeitem"]');
    const row = item?.querySelector<HTMLElement>(rowSelector);
    if (!item || !row) return;
    e.preventDefault();
    const rect = item.getBoundingClientRect();
    row.dispatchEvent(new window.MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      clientX: rect.left + 16,
      clientY: rect.bottom,
    }));
  }, [rowSelector]);

  const closeMenu = useCallback(() => {
    pending.current = null;
  }, []);

  return { closeMenu, onContextMenu, onKeyDown };
}
