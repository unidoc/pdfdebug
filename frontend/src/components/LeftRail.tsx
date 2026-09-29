/**
 * @file The left navigator rail: a vertical tablist of labelled destinations
 * that switches what fills the left panel, and collapses the panel when the
 * active item is clicked with a pointer.
 */
import { useEffect, useRef, type KeyboardEvent, type MouseEvent } from 'react';
import { useAppDispatch, useAppState } from '../hooks/useDocumentState';
import { useLatest } from '../hooks/useLatest';
import { getPlatformModifier } from '../lib/platform';
import { LEFT_RAIL_DESTINATIONS } from './leftRailDestinations';

/** DOM id of a destination's rail item. */
export function leftRailTabId(id: string): string {
  return `left-rail-tab-${id}`;
}

/** DOM id of a destination's panel. */
export function leftRailPanelId(id: string): string {
  return `left-rail-panel-${id}`;
}

/**
 * Index of the destination to show for a view id; an unknown id resolves to
 * the first destination.
 */
export function resolveLeftViewIndex(view: string): number {
  const i = LEFT_RAIL_DESTINATIONS.findIndex((d) => d.id === view);
  return i < 0 ? 0 : i;
}

/**
 * Vertical tablist rendered from LEFT_RAIL_DESTINATIONS. Up/Down (wrapping),
 * Home and End move focus and select; a pointer click on the active item
 * toggles the left panel's collapse; keyboard activation and Cmd/Ctrl+digit
 * select and un-collapse, never collapse.
 */
export function LeftRail() {
  const { leftView, leftPanelCollapsed } = useAppState();
  const dispatch = useAppDispatch();
  const activeIndex = resolveLeftViewIndex(leftView);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const count = LEFT_RAIL_DESTINATIONS.length;

  function selectAndFocus(index: number) {
    dispatch({ type: 'SELECT_LEFT_VIEW', payload: { view: LEFT_RAIL_DESTINATIONS[index].id } });
    itemRefs.current[index]?.focus();
  }
  const selectAndFocusRef = useLatest(selectAndFocus);

  // Cmd/Ctrl+1..N selects destination N. Fires from text fields too (the
  // chord has no editing meaning) and with no document open.
  useEffect(() => {
    const wantsMeta = getPlatformModifier() === 'Cmd';
    function handler(e: globalThis.KeyboardEvent) {
      const mod = wantsMeta ? e.metaKey : e.ctrlKey;
      if (!mod || e.shiftKey || e.altKey) return;
      if (!/^[1-9]$/.test(e.key)) return;
      const index = Number(e.key) - 1;
      if (index >= count) return;
      e.preventDefault();
      selectAndFocusRef.current(index);
    }
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [count, selectAndFocusRef]);

  function handleKeyDown(e: KeyboardEvent<HTMLButtonElement>, index: number) {
    let next: number;
    switch (e.key) {
      case 'ArrowDown': next = (index + 1) % count; break;
      case 'ArrowUp': next = (index - 1 + count) % count; break;
      case 'Home': next = 0; break;
      case 'End': next = count - 1; break;
      default: return;
    }
    e.preventDefault();
    selectAndFocus(next);
  }

  function handleClick(e: MouseEvent<HTMLButtonElement>, index: number) {
    // detail 0 is a keyboard activation (Enter/Space): select, never toggle.
    if (e.detail === 0) {
      selectAndFocus(index);
      return;
    }
    if (index === activeIndex) {
      dispatch({ type: 'TOGGLE_LEFT_PANEL' });
    } else {
      dispatch({ type: 'SELECT_LEFT_VIEW', payload: { view: LEFT_RAIL_DESTINATIONS[index].id } });
    }
  }

  return (
    <div className="w-[76px] flex-shrink-0 h-full bg-surface border-r border-border py-[8px]">
      <div role="tablist" aria-orientation="vertical" aria-label="Navigators" className="flex flex-col">
        {LEFT_RAIL_DESTINATIONS.map((dest, index) => {
          const active = index === activeIndex;
          const Icon = dest.icon;
          const fill = active ? (leftPanelCollapsed ? 'bg-border-focus/5' : 'bg-border-focus/15') : '';
          const bar = active ? (leftPanelCollapsed ? 'bg-border-focus/40' : 'bg-border-focus') : 'bg-transparent';
          return (
            <button
              key={dest.id}
              ref={(el) => { itemRefs.current[index] = el; }}
              type="button"
              role="tab"
              id={leftRailTabId(dest.id)}
              aria-label={dest.label}
              aria-selected={active}
              aria-controls={leftRailPanelId(dest.id)}
              aria-expanded={active ? !leftPanelCollapsed : undefined}
              tabIndex={active ? 0 : -1}
              onClick={(e) => handleClick(e, index)}
              onKeyDown={(e) => handleKeyDown(e, index)}
              className="relative flex flex-col items-center justify-center min-h-[54px] w-full px-[4px] cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-border-focus"
            >
              <span aria-hidden="true" className={`absolute left-0 top-[8px] bottom-[8px] w-[3px] rounded-r ${bar}`} />
              <span aria-hidden="true" className={`flex items-center justify-center w-[32px] h-[28px] rounded-[6px] ${fill}`}>
                <Icon size={20} className={active ? 'text-text' : 'text-text-secondary'} />
              </span>
              <span className={`mt-[6px] text-[11px] leading-tight text-center break-words ${active ? 'text-text font-medium' : 'text-text/70'}`}>
                {dest.label}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
