/**
 * @file Hooks shared by the left-rail navigator panels: the pane size their
 * Tree is drawn at, and the triggers that open the row context menu.
 */
import { useCallback, useEffect, useState, type KeyboardEvent, type MouseEvent, type RefObject } from 'react';
import type { TreeApi } from 'react-arborist';
import { useLatest } from '../hooks/useLatest';
import type { RowMenuTarget } from './RowContextMenu';

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
