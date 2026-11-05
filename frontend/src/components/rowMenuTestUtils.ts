/**
 * @file Test helpers for the native row menu: read which menu a contextmenu
 * event armed, and deliver a menu item click the way the Go side sends it.
 */
import { act, fireEvent } from '@testing-library/react';
import { RowMenuAction } from '../../bindings/events/index.js';

/** The native menu armed on an element's ancestors, or null when none is. */
export interface ArmedRowMenu {
  menu: string;
  token: string;
}

/** Reads the --custom-contextmenu properties the nearest ancestor of el carries. */
export function armedRowMenu(el: HTMLElement): ArmedRowMenu | null {
  for (let n: HTMLElement | null = el; n; n = n.parentElement) {
    const menu = n.style.getPropertyValue('--custom-contextmenu');
    if (menu) return { menu, token: n.style.getPropertyValue('--custom-contextmenu-data') };
  }
  return null;
}

/** Right-clicks el and returns the native menu that would open, or null. */
export function rightClickRow(el: HTMLElement): ArmedRowMenu | null {
  fireEvent.contextMenu(el);
  return armedRowMenu(el);
}

/** Delivers a row menu item click from the backend. */
export function clickRowMenuItem(action: string, token: string) {
  const w = window as unknown as { _wails: { dispatchWailsEvent: (e: { name: string; data: unknown }) => void } };
  act(() => {
    w._wails.dispatchWailsEvent({ name: RowMenuAction, data: { action, token } });
  });
}
