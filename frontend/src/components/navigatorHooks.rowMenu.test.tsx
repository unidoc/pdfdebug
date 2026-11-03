/**
 * useRowContextMenu: one Shift+F10 / ContextMenu key press opens one native
 * menu even when the WebView sends its own contextmenu for the same press,
 * and a clicked item hands back the hit the row resolved to.
 */
import { useRef } from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { useRowContextMenu, type RowMenuHit } from './navigatorHooks';
import { armedRowMenu, clickRowMenuItem } from './rowMenuTestUtils';

function Panel({ acted }: { acted: { action: string; hit: RowMenuHit }[] }) {
  const treeRef = useRef(undefined);
  const { onContextMenu, onKeyDown, onMouseDownCapture } = useRowContextMenu(
    treeRef,
    '[data-row-id]',
    'data-row-id',
    (rowId) => ({ menu: 'tree-row-ref', nodeId: `obj:${rowId}:0`, objectRef: `${rowId} 0 R` }),
    (action, hit) => acted.push({ action, hit }),
  );
  return (
    <div onContextMenu={onContextMenu} onKeyDown={onKeyDown} onMouseDownCapture={onMouseDownCapture}>
      <div role="treeitem" tabIndex={0} data-testid="item">
        <span data-row-id="7">row 7</span>
      </div>
    </div>
  );
}

describe('row menu opened from the keyboard', () => {
  test('the WebView contextmenu for the same key press is cancelled and the first menu still acts', () => {
    const acted: { action: string; hit: RowMenuHit }[] = [];
    render(<Panel acted={acted} />);
    const row = screen.getByText('row 7');

    fireEvent.keyDown(screen.getByTestId('item'), { key: 'ContextMenu' });
    const first = armedRowMenu(row)?.token ?? '';
    const notPrevented = fireEvent.contextMenu(row);

    expect(notPrevented).toBe(false);
    expect(armedRowMenu(row)?.token).toBe(first);
    clickRowMenuItem('copy-ref', first);
    expect(acted).toEqual([{ action: 'copy-ref', hit: { menu: 'tree-row-ref', nodeId: 'obj:7:0', objectRef: '7 0 R' } }]);
  });

  test('a right-click after a key press that drew no WebView event opens a new menu', () => {
    render(<Panel acted={[]} />);
    const row = screen.getByText('row 7');

    fireEvent.keyDown(screen.getByTestId('item'), { key: 'F10', shiftKey: true });
    const first = armedRowMenu(row)?.token;
    fireEvent.mouseDown(row, { button: 2 });
    fireEvent.contextMenu(row);

    const second = armedRowMenu(row)?.token;
    expect(second).toBeTruthy();
    expect(second).not.toBe(first);
  });

  test('a second key press opens a menu again', () => {
    render(<Panel acted={[]} />);
    const row = screen.getByText('row 7');
    const item = screen.getByTestId('item');

    fireEvent.keyDown(item, { key: 'ContextMenu' });
    const first = armedRowMenu(row)?.token;
    fireEvent.keyDown(item, { key: 'ContextMenu' });

    expect(armedRowMenu(row)?.token).not.toBe(first);
  });
});

describe('row menu opened with the mouse', () => {
  test('consecutive right-clicks each open a menu', () => {
    render(<Panel acted={[]} />);
    const row = screen.getByText('row 7');

    fireEvent.mouseDown(row, { button: 2 });
    fireEvent.contextMenu(row);
    const first = armedRowMenu(row)?.token;
    fireEvent.mouseDown(row, { button: 2 });
    fireEvent.contextMenu(row);

    expect(first).toBeTruthy();
    expect(armedRowMenu(row)?.token).not.toBe(first);
  });
});
