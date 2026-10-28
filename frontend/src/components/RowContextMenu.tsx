/**
 * @file The row context menu shared by the left-rail navigators and the
 * structure tree. Each panel finds the row and resolves its node id; this menu
 * positions itself, focuses its first item, moves focus between items with the
 * arrow keys, and closes on Escape (returning focus to the row) or a
 * pointer-down outside it. The navigators show "Show node in tree"; the
 * structure tree passes its own items.
 */
import { useEffect, useRef } from 'react';
import { useAppDispatch } from '../hooks/useDocumentState';
import { useLatest } from '../hooks/useLatest';

/** An open row menu: where it sits, the node it reveals, and the element to refocus. */
export interface RowMenuTarget {
  x: number;
  y: number;
  nodeId: string;
  returnFocus: HTMLElement | null;
}

/** One menu entry; onSelect runs after the menu has closed and focus is back on the row. */
export interface RowMenuItem {
  label: string;
  onSelect: () => void;
}

interface RowContextMenuProps {
  target: RowMenuTarget;
  /** Accessible name of the menu, e.g. "Page row actions". */
  label: string;
  /** Called whenever the menu closes; the panel drops its target. */
  onClose: () => void;
  /** Entries to show. Defaults to the single "Show node in tree" item. */
  items?: RowMenuItem[];
}

/**
 * Row menu. The default item "Show node in tree" dispatches NAVIGATE_TO_REF,
 * which also switches the rail to Structure and un-collapses the left panel.
 */
export function RowContextMenu({ target, label, onClose, items }: RowContextMenuProps) {
  const dispatch = useAppDispatch();
  const menuRef = useRef<HTMLDivElement>(null);
  const targetRef = useLatest(target);
  const onCloseRef = useLatest(onClose);

  useEffect(() => {
    const menuItems = () => Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? []);
    menuItems()[0]?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        const all = menuItems();
        if (all.length === 0) return;
        e.preventDefault();
        const at = all.indexOf(document.activeElement as HTMLButtonElement);
        const step = e.key === 'ArrowDown' ? 1 : -1;
        all[at < 0 ? 0 : (at + step + all.length) % all.length].focus();
        return;
      }
      if (e.key !== 'Escape') return;
      e.preventDefault();
      const returnFocus = targetRef.current.returnFocus;
      onCloseRef.current();
      returnFocus?.focus();
    }
    function onPointerDown(e: PointerEvent) {
      if (menuRef.current && e.target instanceof Node && menuRef.current.contains(e.target)) return;
      onCloseRef.current();
    }
    document.addEventListener('keydown', onKey);
    document.addEventListener('pointerdown', onPointerDown);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('pointerdown', onPointerDown);
    };
  }, [target, targetRef, onCloseRef]);

  const entries: RowMenuItem[] = items ?? [{
    label: 'Show node in tree',
    onSelect: () => dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: target.nodeId } }),
  }];

  // Focus goes back to the row first, so an item that does not move focus
  // itself (a copy) leaves the keyboard user on the row they acted on.
  function activate(item: RowMenuItem) {
    const returnFocus = target.returnFocus;
    onClose();
    returnFocus?.focus();
    item.onSelect();
  }

  return (
    <div
      ref={menuRef}
      role="menu"
      aria-label={label}
      className="fixed z-50 min-w-[160px] py-1 bg-surface border border-border rounded shadow-md text-sm"
      style={{ left: target.x, top: target.y }}
    >
      {entries.map((item) => (
        <button
          key={item.label}
          type="button"
          role="menuitem"
          onClick={() => activate(item)}
          className="block w-full text-left px-3 py-1 text-text hover:bg-surface-hover focus:outline-none focus-visible:bg-surface-hover cursor-pointer"
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}
