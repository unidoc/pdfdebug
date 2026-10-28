/**
 * Structure-tree row context menu: opened by right-click or Shift+F10 on a
 * row, it offers "Copy object reference" for indirect objects and "Copy path"
 * for every node, both copying through the Wails clipboard.
 *
 * Run: cd frontend && npx vitest run src/components/TreePanel.contextMenu.test.tsx
 */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { AppProvider, useAppDispatch, type AppAction } from '../hooks/useDocumentState';
import { TreePanel, formatNodePath, treeRowMenuItems } from './TreePanel';

const mockSetText = vi.fn();
vi.mock('@wailsio/runtime', () => ({
  Clipboard: { SetText: (...a: unknown[]) => mockSetText(...a) },
}));

const mockGetAncestorPath = vi.fn();
vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  GetChildren: vi.fn().mockResolvedValue([]),
  GetAncestorPath: (...a: unknown[]) => mockGetAncestorPath(...a),
}));

class MockResizeObserver {
  callback: ResizeObserverCallback;
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }
  observe(target: Element) {
    this.callback(
      [{ target, contentRect: { width: 300, height: 600 } as DOMRectReadOnly } as ResizeObserverEntry],
      this,
    );
  }
  unobserve() {}
  disconnect() {}
}

const node = (id: string, label: string, objectRef: string) => ({
  id,
  label,
  rawKey: `/${label}`,
  nodeType: 'dict',
  valueType: objectRef ? 'reference' : '',
  hasChildren: false,
  childCount: 0,
  iconHint: 'default',
  error: '',
  objectRef,
});

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: {
    tabId: 'tab-1',
    fileName: 'test.pdf',
    filePath: '/path/to/test.pdf',
    pageCount: 1,
    rootNode: { ...node('root', 'Catalog', ''), hasChildren: true, childCount: 2 },
    rootChildren: [
      node('obj:0:2', 'Pages', '2 0 R'),
      node('dict:root:Lang', 'Lang', ''),
      { ...node('error:root:deref', 'Broken', ''), error: 'dangling indirect reference obj:9:0' },
    ],
  },
};

function Opener() {
  const dispatch = useAppDispatch();
  return (
    <>
      <button data-testid="open" onClick={() => dispatch(openAction)} />
      <button
        data-testid="open-second"
        onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { ...openAction.payload, tabId: 'tab-2', fileName: 'b.pdf', filePath: '/path/to/b.pdf' } } as AppAction)}
      />
    </>
  );
}

function renderTree() {
  render(
    <AppProvider>
      <Opener />
      <TreePanel />
    </AppProvider>,
  );
  fireEvent.click(screen.getByTestId('open'));
}

async function rowFor(label: string): Promise<HTMLElement> {
  const text = await screen.findByText(label);
  const row = text.closest<HTMLElement>('[data-testid="tree-node"]');
  if (!row) throw new Error(`no tree row for ${label}`);
  return row;
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('ResizeObserver', MockResizeObserver);
  mockSetText.mockResolvedValue(true);
});

describe('structure tree row context menu', () => {
  test('right-click on an indirect object offers both copy items', async () => {
    renderTree();
    fireEvent.contextMenu(await rowFor('Pages'));

    const menu = await screen.findByRole('menu', { name: 'Tree row actions' });
    const items = Array.from(menu.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent);
    expect(items).toEqual(['Copy object reference', 'Copy path']);
  });

  test('a direct node offers only Copy path', async () => {
    renderTree();
    fireEvent.contextMenu(await rowFor('Lang'));

    const menu = await screen.findByRole('menu', { name: 'Tree row actions' });
    const items = Array.from(menu.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent);
    expect(items).toEqual(['Copy path']);
  });

  test('Copy object reference copies the objectRef and closes the menu', async () => {
    renderTree();
    fireEvent.contextMenu(await rowFor('Pages'));

    fireEvent.click(await screen.findByRole('menuitem', { name: 'Copy object reference' }));

    expect(mockSetText).toHaveBeenCalledWith('2 0 R');
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  test('Copy path copies the ancestor path of the node', async () => {
    mockGetAncestorPath.mockResolvedValue(['root', 'obj:0:2']);
    renderTree();
    fireEvent.contextMenu(await rowFor('Pages'));

    fireEvent.click(await screen.findByRole('menuitem', { name: 'Copy path' }));

    expect(mockGetAncestorPath).toHaveBeenCalledWith('tab-1', 'obj:0:2');
    await waitFor(() => expect(mockSetText).toHaveBeenCalledWith('root > obj:0:2'));
  });

  test('Shift+F10 on a focused row opens the menu', async () => {
    renderTree();
    const row = await rowFor('Pages');
    const item = row.closest<HTMLElement>('[role="treeitem"]') ?? row;

    fireEvent.keyDown(item, { key: 'F10', shiftKey: true });

    expect(await screen.findByRole('menu', { name: 'Tree row actions' })).toBeInTheDocument();
  });

  test('an error row opens no menu', async () => {
    renderTree();
    fireEvent.contextMenu(await rowFor('Broken'));

    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  test('a failed ancestor lookup copies nothing', async () => {
    mockGetAncestorPath.mockRejectedValue(new Error('document not found'));
    renderTree();
    fireEvent.contextMenu(await rowFor('Pages'));

    fireEvent.click(await screen.findByRole('menuitem', { name: 'Copy path' }));

    await waitFor(() => expect(mockGetAncestorPath).toHaveBeenCalled());
    expect(mockSetText).not.toHaveBeenCalled();
  });
});

describe('structure tree row menu across tabs', () => {
  test('switching to another tab closes an open menu', async () => {
    renderTree();
    fireEvent.contextMenu(await rowFor('Pages'));
    expect(await screen.findByRole('menu', { name: 'Tree row actions' })).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('open-second'));

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument());
  });
});

describe('treeRowMenuItems', () => {
  test('a rejected clipboard write is swallowed', async () => {
    mockSetText.mockRejectedValue(new Error('clipboard unavailable'));
    const [copyRef] = treeRowMenuItems('tab-1', 'obj:0:2', '2 0 R');

    expect(() => copyRef.onSelect()).not.toThrow();
    await waitFor(() => expect(mockSetText).toHaveBeenCalledWith('2 0 R'));
  });

  test('Copy path copies an empty path when the backend returns none', async () => {
    mockGetAncestorPath.mockResolvedValue(null);
    const items = treeRowMenuItems('tab-1', 'dict:root:Lang', '');

    expect(items.map((i) => i.label)).toEqual(['Copy path']);
    items[0].onSelect();
    await waitFor(() => expect(mockSetText).toHaveBeenCalledWith(''));
  });
});

describe('formatNodePath', () => {
  test('joins node ids the way the command palette breadcrumb does', () => {
    expect(formatNodePath(['root', 'obj:0:2', 'obj:0:4'])).toBe('root > obj:0:2 > obj:0:4');
    expect(formatNodePath([])).toBe('');
  });
});
