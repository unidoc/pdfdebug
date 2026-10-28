/**
 * The shared row menu on its own: placement, focus on open, which pointer and
 * key events close it, what activating its default item dispatches, and
 * arrow-key focus and activation with caller-supplied items.
 *
 * Run: cd frontend && npx vitest run src/components/RowContextMenu.test.tsx
 */
import { render, screen, act, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi } from 'vitest';
import type { Dispatch } from 'react';
import { AppProvider, selectActiveLeftView, useAppDispatch, useAppState, type AppAction, type AppState } from '../hooks/useDocumentState';
import { RowContextMenu, type RowMenuItem, type RowMenuTarget } from './RowContextMenu';

let state: AppState;
let dispatch: Dispatch<AppAction>;

function Probe() {
  state = useAppState();
  dispatch = useAppDispatch();
  return null;
}

const rootNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: false,
  childCount: 0,
  iconHint: 'catalog',
  error: '',
};

function renderMenu(over: Partial<RowMenuTarget> = {}) {
  const row = document.createElement('button');
  row.textContent = 'row';
  document.body.appendChild(row);
  const onClose = vi.fn();
  const target: RowMenuTarget = { x: 40, y: 120, nodeId: 'obj:0:12', returnFocus: row, ...over };
  render(
    <AppProvider>
      <Probe />
      <RowContextMenu target={target} label="Test row actions" onClose={onClose} />
    </AppProvider>,
  );
  act(() =>
    dispatch({
      type: 'OPEN_DOCUMENT',
      payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/tmp/a.pdf', pageCount: 1, rootNode, rootChildren: [] },
    }),
  );
  return { row, onClose };
}

describe('RowContextMenu', () => {
  test('sits at the target point, carries its label and focuses its item', () => {
    const { row } = renderMenu();
    const menu = screen.getByRole('menu', { name: 'Test row actions' });
    expect(menu.style.left).toBe('40px');
    expect(menu.style.top).toBe('120px');
    expect(document.activeElement).toBe(screen.getByRole('menuitem', { name: 'Show node in tree' }));
    row.remove();
  });

  test('a pointer-down inside the menu keeps it open; one outside closes it', () => {
    const { row, onClose } = renderMenu();
    fireEvent.pointerDown(screen.getByRole('menuitem'));
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.pointerDown(document.body);
    expect(onClose).toHaveBeenCalledTimes(1);
    row.remove();
  });

  test('keys other than Escape leave it open', () => {
    const { row, onClose } = renderMenu();
    fireEvent.keyDown(document, { key: 'ArrowDown' });
    fireEvent.keyDown(document, { key: 'Enter' });
    expect(onClose).not.toHaveBeenCalled();
    row.remove();
  });

  test('Escape closes it and returns focus to the row', () => {
    const { row, onClose } = renderMenu();
    const ev = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
    act(() => {
      document.dispatchEvent(ev);
    });
    expect(ev.defaultPrevented).toBe(true);
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(row);
    row.remove();
  });

  test('Escape with no element to refocus still closes it', () => {
    const { row, onClose } = renderMenu({ returnFocus: null });
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
    row.remove();
  });

  test('activating the item closes the menu and asks the Structure tree to reveal the node', async () => {
    const user = userEvent.setup();
    const { row, onClose } = renderMenu();
    act(() => dispatch({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } }));
    await user.click(screen.getByRole('menuitem', { name: 'Show node in tree' }));
    expect(onClose).toHaveBeenCalled();
    expect(selectActiveLeftView(state)).toBe('structure');
    expect(state.tabs.find((t) => t.tabId === 'tab-1')?.pendingNavTarget).toBe('obj:0:12');
    row.remove();
  });

  test('arrow keys move focus between caller-supplied items, wrapping at the ends', () => {
    const row = document.createElement('button');
    document.body.appendChild(row);
    const items: RowMenuItem[] = [
      { label: 'First', onSelect: vi.fn() },
      { label: 'Second', onSelect: vi.fn() },
    ];
    render(
      <AppProvider>
        <RowContextMenu target={{ x: 0, y: 0, nodeId: 'obj:0:1', returnFocus: row }} label="Items" onClose={vi.fn()} items={items} />
      </AppProvider>,
    );
    const first = screen.getByRole('menuitem', { name: 'First' });
    const second = screen.getByRole('menuitem', { name: 'Second' });
    expect(document.activeElement).toBe(first);
    fireEvent.keyDown(document, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(second);
    fireEvent.keyDown(document, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(first);
    fireEvent.keyDown(document, { key: 'ArrowUp' });
    expect(document.activeElement).toBe(second);
    row.remove();
  });

  test('an arrow key with focus outside the menu brings focus back to its first item without scrolling', () => {
    const row = document.createElement('button');
    document.body.appendChild(row);
    const items: RowMenuItem[] = [
      { label: 'First', onSelect: vi.fn() },
      { label: 'Second', onSelect: vi.fn() },
    ];
    render(
      <AppProvider>
        <RowContextMenu target={{ x: 0, y: 0, nodeId: 'obj:0:1', returnFocus: row }} label="Items" onClose={vi.fn()} items={items} />
      </AppProvider>,
    );
    row.focus();
    const ev = new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true });
    act(() => {
      document.dispatchEvent(ev);
    });
    expect(ev.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(screen.getByRole('menuitem', { name: 'First' }));
    row.remove();
  });

  test('activating a caller-supplied item closes the menu, returns focus to the row and runs it', () => {
    const row = document.createElement('button');
    document.body.appendChild(row);
    const onClose = vi.fn();
    const onSelect = vi.fn(() => {
      expect(document.activeElement).toBe(row);
    });
    render(
      <AppProvider>
        <RowContextMenu target={{ x: 0, y: 0, nodeId: 'obj:0:1', returnFocus: row }} label="Items" onClose={onClose} items={[{ label: 'Copy path', onSelect }]} />
      </AppProvider>,
    );
    fireEvent.click(screen.getByRole('menuitem', { name: 'Copy path' }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(row);
    expect(screen.queryByRole('menuitem', { name: 'Show node in tree' })).not.toBeInTheDocument();
    row.remove();
  });
});
