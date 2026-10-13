/**
 * @file The "Show node in tree" context menu shared by the left-rail
 * navigators. Each panel finds the row and resolves its node id; this menu
 * only positions itself, takes focus, and closes on Escape (returning focus
 * to the row) or a pointer-down outside it.
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

interface RowContextMenuProps {
  target: RowMenuTarget;
  /** Accessible name of the menu, e.g. "Page row actions". */
  label: string;
  /** Called whenever the menu closes; the panel drops its target. */
  onClose: () => void;
}

/**
 * Single-item row menu. Activating "Show node in tree" dispatches
 * NAVIGATE_TO_REF, which also switches the rail to Structure and un-collapses
 * the left panel.
 */
export function RowContextMenu({ target, label, onClose }: RowContextMenuProps) {
  const dispatch = useAppDispatch();
  const menuRef = useRef<HTMLDivElement>(null);
  const itemRef = useRef<HTMLButtonElement>(null);
  const targetRef = useLatest(target);
  const onCloseRef = useLatest(onClose);

  useEffect(() => {
    itemRef.current?.focus();
    function onKey(e: KeyboardEvent) {
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

  function showInTree() {
    const nodeId = target.nodeId;
    onClose();
    dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: nodeId } });
  }

  return (
    <div
      ref={menuRef}
      role="menu"
      aria-label={label}
      className="fixed z-50 min-w-[160px] py-1 bg-surface border border-border rounded shadow-md text-sm"
      style={{ left: target.x, top: target.y }}
    >
      <button
        ref={itemRef}
        type="button"
        role="menuitem"
        onClick={showInTree}
        className="block w-full text-left px-3 py-1 text-text hover:bg-surface-hover focus:outline-none focus-visible:bg-surface-hover cursor-pointer"
      >
        Show node in tree
      </button>
    </div>
  );
}
