/**
 * The Pages navigator, driven through the main layout's rail: rows built from
 * GetPageIndex, the shared selection, lazy expansion, the header count and the
 * /Count disagreement note, the jump field, virtualization, the per-tab cache
 * and the "Show node in tree" menu.
 *
 * Run: cd frontend && npx vitest run src/components/PagesPanel.test.tsx
 */
import { render, screen, act, fireEvent, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { Dispatch, ReactNode } from 'react';
import { AppProvider, useAppDispatch, useAppState, type AppAction, type AppState } from '../hooks/useDocumentState';
import { MainLayout } from './MainLayout';

const RAIL_KEY = 'unidoc-pdf-debugger:left-rail';

vi.mock('allotment', () => {
  function Pane({ children }: { children: ReactNode }) {
    return <div>{children}</div>;
  }
  function Allotment({ children }: { children: ReactNode }) {
    return <div>{children}</div>;
  }
  Allotment.Pane = Pane;
  return { Allotment };
});

vi.mock('allotment/dist/style.css', () => ({}));

const mockGetPageIndex = vi.fn();
const mockGetChildren = vi.fn();
const mockGetAncestorPath = vi.fn();
const mockGetObjectSource = vi.fn();
const mockGetObjectDetail = vi.fn();

vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: vi.fn(),
  GetTreeRoot: vi.fn(),
  GetChildren: (...args: unknown[]) => mockGetChildren(...args),
  CloseDocument: vi.fn(),
  OpenFileDialog: vi.fn(),
  GetObjectDetail: (...args: unknown[]) => mockGetObjectDetail(...args),
  GetObjectSource: (...args: unknown[]) => mockGetObjectSource(...args),
  GetReverseRefs: vi.fn().mockResolvedValue([]),
  GetAncestorPath: (...args: unknown[]) => mockGetAncestorPath(...args),
  GetXRefTable: vi.fn().mockResolvedValue({ tabId: '', entries: [] }),
  GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
  GetSignatures: vi.fn().mockResolvedValue([]),
  GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
  GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
  SaveBytesToFile: vi.fn().mockResolvedValue(''),
  DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  GetPageIndex: (...args: unknown[]) => mockGetPageIndex(...args),
  GetImageIndex: vi.fn(),
  GetImagePages: vi.fn(),
  GetImagePageGroups: vi.fn(),
}));

class MockResizeObserver {
  callback: ResizeObserverCallback;
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }
  observe(target: Element) {
    this.callback(
      [{ target, contentRect: { width: 300, height: 600 } as DOMRectReadOnly } as ResizeObserverEntry],
      this as unknown as ResizeObserver,
    );
  }
  unobserve() {}
  disconnect() {}
}

interface Entry {
  pageNum: number;
  objNum: number;
  gen: number;
  nodeId: string;
  contentNodeId: string;
  mediaBox: number[];
  rotate: number;
  inherited: number;
  annotCount: number;
  contentLen: number;
  error: string;
}

function page(pageNum: number, objNum: number, error = ''): Entry {
  return {
    pageNum,
    objNum,
    gen: 0,
    nodeId: `obj:0:${objNum}`,
    contentNodeId: `obj:0:${objNum + 1}`,
    mediaBox: [0, 0, 612, 792],
    rotate: 0,
    inherited: 0,
    annotCount: 0,
    contentLen: 15,
    error,
  };
}

const nullKid: Entry = {
  pageNum: 0,
  objNum: 0,
  gen: 0,
  nodeId: '',
  contentNodeId: '',
  mediaBox: [0, 0, 0, 0],
  rotate: 0,
  inherited: 0,
  annotCount: 0,
  contentLen: 0,
  error: 'Kids entry 1 is null',
};

const entries: Entry[] = [page(1, 3), nullKid, page(2, 8, 'no MediaBox, own or inherited'), page(3, 14)];

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 1,
  iconHint: 'catalog',
  error: '',
};

const rootChildren = [
  {
    id: 'obj:0:2',
    label: 'Pages',
    rawKey: '/Pages',
    nodeType: 'dict',
    valueType: 'reference',
    hasChildren: true,
    childCount: 1,
    iconHint: 'pages',
    error: '',
    objectRef: '2 0 R',
    typeName: 'Pages',
  },
];

const pageChildren = [
  {
    id: 'dict:obj:0:3:Contents',
    label: 'Contents',
    rawKey: '/Contents',
    nodeType: 'stream',
    valueType: 'reference',
    hasChildren: false,
    childCount: 0,
    iconHint: 'stream',
    error: '',
    objectRef: '4 0 R',
  },
  {
    id: 'dict:obj:0:3:MediaBox',
    label: 'MediaBox',
    rawKey: '/MediaBox',
    nodeType: 'array',
    valueType: '',
    hasChildren: true,
    childCount: 4,
    iconHint: 'default',
    error: '',
  },
];

let state: AppState;
let dispatch: Dispatch<AppAction>;

function Probe() {
  state = useAppState();
  dispatch = useAppDispatch();
  return null;
}

function renderLayout() {
  return render(
    <AppProvider>
      <Probe />
      <MainLayout />
    </AppProvider>,
  );
}

function openTab(tabId = 'tab-1', pageCount = 3) {
  act(() =>
    dispatch({
      type: 'OPEN_DOCUMENT',
      payload: { tabId, fileName: `${tabId}.pdf`, filePath: `/tmp/${tabId}.pdf`, pageCount, rootNode: catalogNode, rootChildren },
    }),
  );
}

function rail() {
  const lists = screen.getAllByRole('tablist').filter((l) => l.getAttribute('aria-orientation') === 'vertical');
  expect(lists).toHaveLength(1);
  return lists[0];
}

function tab(name: string) {
  return within(rail()).getByRole('tab', { name });
}

function panelFor(name: string) {
  const panel = document.getElementById(tab(name).getAttribute('aria-controls') ?? '');
  expect(panel).not.toBeNull();
  return panel!;
}

async function showPages(user: ReturnType<typeof userEvent.setup>) {
  await user.click(tab('Pages'));
  const panel = panelFor('Pages');
  await waitFor(() => expect(within(panel).getAllByTestId('tree-node').length).toBeGreaterThan(0));
  return panel;
}

function rowByText(panel: HTMLElement, text: string | RegExp) {
  const row = within(panel).getAllByTestId('tree-node').find((r) =>
    typeof text === 'string' ? (r.textContent ?? '').includes(text) : text.test(r.textContent ?? ''),
  );
  expect(row, `no row matching ${String(text)}`).toBeDefined();
  return row!;
}

function jumpField(panel: HTMLElement) {
  return within(panel).getByRole('textbox', { name: /go to page/i });
}

beforeEach(() => {
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
  window.localStorage.removeItem(RAIL_KEY);
  mockGetPageIndex.mockReset().mockResolvedValue(entries);
  mockGetChildren.mockReset().mockResolvedValue(pageChildren);
  mockGetAncestorPath.mockReset().mockResolvedValue(['root', 'obj:0:2', 'obj:0:3']);
  mockGetObjectSource.mockReset().mockResolvedValue('');
  mockGetObjectDetail.mockReset().mockResolvedValue(null);
});

afterEach(() => {
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

describe('page rows', () => {
  test('each page row is prefixed by its page number and shows the /Page reference', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    expect(mockGetPageIndex).toHaveBeenCalledWith('tab-1');
    expect(rowByText(panel, '1: Page').textContent).toContain('[3 0 R]');
    expect(rowByText(panel, '2: Page').textContent).toContain('[8 0 R]');
    expect(rowByText(panel, '3: Page').textContent).toContain('[14 0 R]');
  });

  test('page rows carry no /T:Page suffix', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    expect(panel.textContent).not.toContain('/T:');
  });

  test('a row with an error shows the message text, not only a marker', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    expect(rowByText(panel, '2: Page').textContent).toContain('no MediaBox, own or inherited');
  });

  test('a row that is not a page is unnumbered and shows its error', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    const rows = within(panel).getAllByTestId('tree-node');
    expect(rows).toHaveLength(4);
    expect(rows[1].textContent).toMatch(/^\s*-:/);
    expect(rows[1].textContent).toContain('Kids entry 1 is null');
  });

  test('page rows keep document order', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    const text = within(panel).getAllByTestId('tree-node').map((r) => r.textContent ?? '');
    expect(text[0]).toContain('1: Page');
    expect(text[2]).toContain('2: Page');
    expect(text[3]).toContain('3: Page');
  });
});

describe('header', () => {
  test('shows the page count and no /Count note when they agree', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1', 3);
    const panel = await showPages(user);
    expect(within(panel).getByText(/\b3 pages\b/i)).toBeInTheDocument();
    expect(panel.textContent).not.toContain('/Count');
  });

  test('notes a disagreement with /Count, stating both numbers', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1', 5);
    const panel = await showPages(user);
    const note = within(panel).getByText(/\/Count/);
    expect(note.textContent).toMatch(/\b3\b/);
    expect(note.textContent).toMatch(/\b5\b/);
  });

  test('a page count of 0 is described as 0 or unreadable', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1', 0);
    const panel = await showPages(user);
    expect(within(panel).getByText(/\/Count is 0 or unreadable/)).toBeInTheDocument();
  });

  test('with no document open the panel renders an empty state and does not fetch', async () => {
    const user = userEvent.setup();
    renderLayout();
    await user.click(tab('Pages'));
    expect(within(panelFor('Pages')).queryAllByTestId('tree-node')).toHaveLength(0);
    expect(mockGetPageIndex).not.toHaveBeenCalled();
  });

  test('a fetch that fails while the panel is hidden is retried when it is shown again', async () => {
    let reject: (err: Error) => void = () => {};
    mockGetPageIndex.mockReset()
      .mockReturnValueOnce(new Promise((_, r) => { reject = r; }))
      .mockResolvedValue(entries);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Pages'));
    await user.click(tab('Structure'));
    await act(async () => reject(new Error('page tree unreadable')));
    await user.click(tab('Pages'));
    await waitFor(() => expect(within(panelFor('Pages')).getAllByTestId('tree-node').length).toBeGreaterThan(0));
    expect(mockGetPageIndex).toHaveBeenCalledTimes(2);
    expect(panelFor('Pages').textContent).not.toContain('page tree unreadable');
  });

  test('a failed fetch shows the error inline', async () => {
    mockGetPageIndex.mockReset().mockRejectedValue(new Error('page tree unreadable'));
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Pages'));
    await waitFor(() => expect(panelFor('Pages').textContent).toContain('page tree unreadable'));
  });

  test('a failure with an empty message shows the error banner without the loading line', async () => {
    mockGetPageIndex.mockReset().mockRejectedValue(new Error(''));
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Pages'));
    const panel = panelFor('Pages');
    await waitFor(() => expect(panel.textContent).toContain('Could not load the page index:'));
    expect(panel.textContent).not.toContain('Loading pages...');
  });
});

describe('selection', () => {
  test('clicking a page row sets the shared selection, so the source and detail panes follow', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    await user.click(rowByText(panel, '1: Page'));
    expect(state.tabs[0].selectedNodeId).toBe('obj:0:3');
    await waitFor(() => expect(mockGetObjectSource).toHaveBeenCalledWith('tab-1', 'obj:0:3'));
    await waitFor(() => expect(mockGetObjectDetail).toHaveBeenCalledWith('tab-1', 'obj:0:3'));
  });

  test('clicking a page row pushes one history entry, so Back returns to the previous selection', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    act(() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:2', label: 'Pages' } }));
    const panel = await showPages(user);
    await user.click(rowByText(panel, '1: Page'));
    act(() => dispatch({ type: 'NAVIGATE_BACK' }));
    expect(state.tabs[0].selectedNodeId).toBe('obj:0:2');
  });

  test('clicking an unnumbered row does not change the selection', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    await user.click(rowByText(panel, 'Kids entry 1 is null'));
    expect(state.tabs[0].selectedNodeId).toBeNull();
  });

  test('arrow keys walk past an unnumbered row to the next page', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    await user.click(rowByText(panel, '1: Page'));
    expect(state.tabs[0].selectedNodeId).toBe('obj:0:3');
    await user.keyboard('{ArrowDown}');
    expect(state.tabs[0].selectedNodeId).toBe('obj:0:3');
    await user.keyboard('{ArrowDown}');
    await waitFor(() => expect(state.tabs[0].selectedNodeId).toBe('obj:0:8'));
  });

  test('a selection made elsewhere is not re-dispatched by the Pages panel', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await showPages(user);
    const before = state.tabs[0].navHistory.length;
    act(() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:3', label: 'Kids[0]', rawKey: '[0]' } }));
    await waitFor(() => expect(state.tabs[0].selectedNodeId).toBe('obj:0:3'));
    await new Promise((r) => setTimeout(r, 20));
    expect(state.tabs[0].selectedNodeLabel).toBe('Kids[0]');
    expect(state.tabs[0].navHistory.length).toBe(before + 1);
  });
});

describe('expansion', () => {
  test('expanding a page row loads its children lazily through GetChildren', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    expect(mockGetChildren).not.toHaveBeenCalled();
    const row = rowByText(panel, '1: Page');
    await user.click(within(row).getByText('>'));
    await waitFor(() => expect(mockGetChildren).toHaveBeenCalledWith('tab-1', 'obj:0:3'));
    await waitFor(() => expect(rowByText(panel, 'Contents').textContent).toContain('[4 0 R]'));
    expect(rowByText(panel, 'MediaBox')).toBeDefined();
  });

  test('unnumbered rows are not expandable', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    expect(within(rowByText(panel, 'Kids entry 1 is null')).queryByText('>')).toBeNull();
  });
});

describe('virtualization', () => {
  test('500 pages render a window of rows, not all of them', async () => {
    mockGetPageIndex.mockReset().mockResolvedValue(Array.from({ length: 500 }, (_, i) => page(i + 1, 10 + i * 2)));
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1', 500);
    const panel = await showPages(user);
    const rendered = within(panel).getAllByTestId('tree-node').length;
    expect(rendered).toBeGreaterThan(0);
    expect(rendered).toBeLessThan(100);
  });
});

describe('per-tab cache', () => {
  test('switching back to a tab does not refetch; closing it evicts', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    await showPages(user);
    // Opening a file lands on Structure, so each open is followed by showing Pages.
    openTab('tab-2');
    await showPages(user);
    expect(mockGetPageIndex).toHaveBeenCalledWith('tab-2');
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } }));
    await waitFor(() => expect(within(panelFor('Pages')).getAllByTestId('tree-node').length).toBeGreaterThan(0));
    expect(mockGetPageIndex.mock.calls.filter((c) => c[0] === 'tab-1')).toHaveLength(1);

    act(() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } }));
    openTab('tab-1');
    await showPages(user);
    expect(mockGetPageIndex.mock.calls.filter((c) => c[0] === 'tab-1')).toHaveLength(2);
  });

  test('an index fetch still pending when its tab closes neither blocks nor overwrites the tab reopened under that id', async () => {
    let resolveFirst: (v: Entry[]) => void = () => {};
    mockGetPageIndex.mockReset().mockReturnValueOnce(new Promise<Entry[]>((r) => {
      resolveFirst = r;
    })).mockResolvedValue(entries);
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    await user.click(tab('Pages'));
    await waitFor(() => expect(mockGetPageIndex).toHaveBeenCalledTimes(1));

    act(() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } }));
    openTab('tab-1');
    const panel = await showPages(user);
    expect(mockGetPageIndex).toHaveBeenCalledTimes(2);
    await act(async () => resolveFirst([page(1, 3)]));
    expect(rowByText(panel, '2: Page')).toBeDefined();
  });

  test('a tab opened while Structure is active is not fetched until Pages is shown', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    await showPages(user);
    await user.click(tab('Structure'));
    openTab('tab-2');
    expect(mockGetPageIndex.mock.calls.filter((c) => c[0] === 'tab-2')).toHaveLength(0);
  });
});

describe('jump field', () => {
  test('is labelled "Go to page" with a placeholder of the numbered range', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    expect(jumpField(panel)).toHaveAttribute('placeholder', '1-3');
  });

  test.each([
    ['', 'Enter a page number.'],
    ['abc', 'Page number must be an integer.'],
    ['1.5', 'Page number must be an integer.'],
    ['0', 'Page number out of range (1-3).'],
    ['4', 'Page number out of range (1-3).'],
  ])('input %j shows %j', async (input, message) => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    const field = jumpField(panel);
    await user.clear(field);
    if (input) await user.type(field, input);
    await user.type(field, '{Enter}');
    expect(within(panel).getByRole('alert')).toHaveTextContent(message);
    expect(state.tabs[0].selectedNodeId).toBeNull();
  });

  test('a valid page number selects that page row, skipping unnumbered rows', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    await user.type(jumpField(panel), '2{Enter}');
    expect(state.tabs[0].selectedNodeId).toBe('obj:0:8');
    expect(within(panel).queryByRole('alert')).toBeNull();
  });

  test('the jump field filters nothing', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    await user.type(jumpField(panel), '3');
    expect(within(panel).getAllByTestId('tree-node')).toHaveLength(4);
  });

  test('a focus request focuses the field with its text selected', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    const field = jumpField(panel) as HTMLInputElement;
    await user.type(field, '12');
    field.blur();
    act(() => dispatch({ type: 'FOCUS_PAGES_JUMP' } as unknown as AppAction));
    await waitFor(() => expect(field).toHaveFocus());
    expect(field.selectionStart).toBe(0);
    expect(field.selectionEnd).toBe(2);
  });
});

describe('jump field across tabs', () => {
  test('an error naming one tab\'s page range is cleared when another tab becomes active', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    openTab('tab-2');
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } }));
    const panel = await showPages(user);
    await user.type(jumpField(panel), '9{Enter}');
    expect(within(panel).getByRole('alert')).toHaveTextContent('Page number out of range (1-3).');
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } }));
    expect(within(panel).queryByRole('alert')).toBeNull();
  });
});

describe('jump field with no pages', () => {
  test('a jump in a document with no page leaves says there are no pages', async () => {
    mockGetPageIndex.mockReset().mockResolvedValue([nullKid]);
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1', 0);
    const panel = await showPages(user);
    await user.type(jumpField(panel), '1{Enter}');
    expect(within(panel).getByRole('alert')).toHaveTextContent('This document has no pages to go to.');
  });
});

describe('jump field before the index is available', () => {
  test('a jump while the index is loading says so instead of reporting a range', async () => {
    mockGetPageIndex.mockReset().mockReturnValue(new Promise(() => {}));
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Pages'));
    const panel = panelFor('Pages');
    await user.type(jumpField(panel), '2{Enter}');
    expect(within(panel).getByRole('alert')).toHaveTextContent('The page index is still loading.');
  });
});

describe('a page listed twice', () => {
  test('selecting the second listing keeps the highlight on it', async () => {
    mockGetPageIndex.mockReset().mockResolvedValue([page(1, 3), page(2, 8), page(3, 3)]);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    await user.type(jumpField(panel), '3{Enter}');
    expect(state.tabs[0].selectedNodeId).toBe('obj:0:3');
    await waitFor(() => {
      const selected = panel.querySelectorAll('[role="treeitem"][aria-selected="true"]');
      expect(selected).toHaveLength(1);
      expect(selected[0].textContent).toContain('3: Page');
    });
  });
});

describe('Show node in tree', () => {
  test('right-click opens a menu with one item', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    fireEvent.contextMenu(rowByText(panel, '1: Page'));
    const menu = screen.getByRole('menu');
    expect(within(menu).getAllByRole('menuitem').map((m) => m.textContent)).toEqual(['Show node in tree']);
  });

  test('the right-click is handled, so the WebView menu does not also open', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    const ev = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
    act(() => {
      rowByText(panel, '1: Page').dispatchEvent(ev);
    });
    expect(ev.defaultPrevented).toBe(true);
  });

  test('activating it switches to Structure and reveals the node with its ancestors', async () => {
    const kidsChildren = [
      {
        id: 'obj:0:3',
        label: 'Kids[0]',
        rawKey: '[0]',
        nodeType: 'dict',
        valueType: 'reference',
        hasChildren: true,
        childCount: 2,
        iconHint: 'page',
        error: '',
        objectRef: '3 0 R',
        typeName: 'Page',
      },
    ];
    mockGetChildren.mockImplementation((_tab: string, id: string) =>
      Promise.resolve(id === 'obj:0:2' ? kidsChildren : pageChildren),
    );
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    fireEvent.contextMenu(rowByText(panel, '1: Page'));
    await user.click(screen.getByRole('menuitem', { name: 'Show node in tree' }));
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('menu')).toBeNull();
    await waitFor(() => expect(mockGetAncestorPath).toHaveBeenCalledWith('tab-1', 'obj:0:3'));
    // The reveal expands the parent Pages node and lands on the page in the Structure tree.
    await waitFor(() => expect(state.tabs[0].pendingNavTarget).toBeNull());
    expect(state.tabs[0].navError).toBeNull();
    expect(mockGetChildren).toHaveBeenCalledWith('tab-1', 'obj:0:2');
    const structure = panelFor('Structure');
    await waitFor(() => {
      const selected = structure.querySelectorAll('[role="treeitem"][aria-selected="true"]');
      expect(selected).toHaveLength(1);
      expect(selected[0].querySelector('[data-testid="tree-node"]')?.getAttribute('data-node-id')).toBe('root>obj:0:2>obj:0:3');
    });
  });

  test('Escape closes the menu and returns focus to the row', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    const row = rowByText(panel, '1: Page');
    fireEvent.contextMenu(row);
    expect(screen.getByRole('menu')).toBeInTheDocument();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('menu')).toBeNull();
    expect(row.contains(document.activeElement) || row.closest('[role="treeitem"]')?.contains(document.activeElement)).toBe(true);
  });

  test('a pointer-down outside closes the menu', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    fireEvent.contextMenu(rowByText(panel, '1: Page'));
    expect(screen.getByRole('menu')).toBeInTheDocument();
    fireEvent.pointerDown(document.body);
    expect(screen.queryByRole('menu')).toBeNull();
  });

  test('a tab switch closes the menu, so it cannot act on the other document', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    openTab('tab-2');
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } }));
    const panel = await showPages(user);
    fireEvent.contextMenu(rowByText(panel, '1: Page'));
    expect(screen.getByRole('menu')).toBeInTheDocument();
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } }));
    expect(screen.queryByRole('menu')).toBeNull();
  });

  test('switching the rail view from the keyboard closes the menu', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    fireEvent.contextMenu(rowByText(panel, '1: Page'));
    expect(screen.getByRole('menu')).toBeInTheDocument();
    act(() => dispatch({ type: 'SELECT_LEFT_VIEW', payload: { view: 'structure' } }));
    expect(screen.queryByRole('menu')).toBeNull();
  });

  test('Shift+F10 on a focused row opens the menu without a mouse', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    await user.click(rowByText(panel, '1: Page'));
    await user.keyboard('{Shift>}{F10}{/Shift}');
    expect(screen.getByRole('menu')).toBeInTheDocument();
  });
});

describe('a numbered page without a reference', () => {
  const directPage: Entry = { ...page(2, 0, 'page is a direct dictionary, not an indirect reference'), nodeId: '', contentNodeId: '' };

  test('keeps its page number but is neither expandable nor selectable', async () => {
    mockGetPageIndex.mockReset().mockResolvedValue([page(1, 3), directPage, page(3, 14)]);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    const row = rowByText(panel, '2: Page');
    expect(within(row).queryByText('>')).toBeNull();
    await user.click(row);
    expect(state.tabs[0].selectedNodeId).toBeNull();
  });

  test('a jump to it says it has no object reference and leaves the selection alone', async () => {
    mockGetPageIndex.mockReset().mockResolvedValue([page(1, 3), directPage, page(3, 14)]);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    await user.type(jumpField(panel), '2{Enter}');
    expect(within(panel).getByRole('alert')).toHaveTextContent('Page 2 has no object reference to select.');
    expect(state.tabs[0].selectedNodeId).toBeNull();
  });
});

describe('jump field after a failed fetch', () => {
  test('a jump says the index could not be loaded', async () => {
    mockGetPageIndex.mockReset().mockRejectedValue(new Error('page tree unreadable'));
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Pages'));
    const panel = panelFor('Pages');
    await waitFor(() => expect(panel.textContent).toContain('page tree unreadable'));
    await user.type(jumpField(panel), '2{Enter}');
    expect(within(panel).getByRole('alert')).toHaveTextContent('The page index could not be loaded.');
  });
});

describe('jump field editing', () => {
  test('typing after an error clears it', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    const field = jumpField(panel);
    await user.type(field, '9{Enter}');
    expect(field).toHaveAttribute('aria-invalid', 'true');
    await user.type(field, '{Backspace}');
    expect(within(panel).queryByRole('alert')).toBeNull();
    expect(field).toHaveAttribute('aria-invalid', 'false');
  });
});

describe('Show node in tree on a row with no reference', () => {
  test('right-clicking an unnumbered row opens no menu', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    fireEvent.contextMenu(rowByText(panel, 'Kids entry 1 is null'));
    expect(screen.queryByRole('menu')).toBeNull();
  });
});

describe('header count', () => {
  test('a single page is counted in the singular', async () => {
    mockGetPageIndex.mockReset().mockResolvedValue([page(1, 3)]);
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1', 1);
    const panel = await showPages(user);
    expect(panel.textContent).toContain('1 page');
    expect(panel.textContent).not.toContain('1 pages');
  });
});

describe('expanding a second row before the first returns', () => {
  test('both rows receive their children', async () => {
    const user = userEvent.setup();
    let resolveFirst: (v: unknown) => void = () => {};
    const thirdPageChildren = [{ ...pageChildren[0], id: 'dict:obj:0:14:Contents', objectRef: '15 0 R' }];
    mockGetChildren.mockImplementation((_tab: string, id: string) =>
      id === 'obj:0:3'
        ? new Promise((resolve) => {
            resolveFirst = resolve;
          })
        : Promise.resolve(thirdPageChildren),
    );
    renderLayout();
    openTab();
    const panel = await showPages(user);
    await user.click(within(rowByText(panel, '1: Page')).getByText('>'));
    await user.click(within(rowByText(panel, '3: Page')).getByText('>'));
    await waitFor(() => expect(rowByText(panel, '[15 0 R]')).toBeDefined());

    await act(async () => resolveFirst(pageChildren));
    await waitFor(() => expect(rowByText(panel, '[4 0 R]')).toBeDefined());
  });
});

describe('Show node in tree on an error child', () => {
  test('right-clicking an error row inside an expanded page opens no menu', async () => {
    const user = userEvent.setup();
    mockGetChildren.mockResolvedValue([
      {
        id: 'error:obj:0:3:Resources',
        label: 'Resources',
        rawKey: '/Resources',
        nodeType: 'error',
        valueType: '',
        hasChildren: false,
        childCount: 0,
        iconHint: 'default',
        error: 'cannot resolve /Resources',
      },
    ]);
    renderLayout();
    openTab();
    const panel = await showPages(user);
    await user.click(within(rowByText(panel, '1: Page')).getByText('>'));
    const errorRow = await waitFor(() => rowByText(panel, 'Resources'));
    fireEvent.contextMenu(errorRow);
    expect(screen.queryByRole('menu')).toBeNull();
  });
});

describe('jump flash', () => {
  test('a second jump within the flash window keeps its own full flash', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showPages(user);
    const field = jumpField(panel);
    const flashing = (text: string) => rowByText(panel, text).className.includes('ring-2');

    vi.useFakeTimers();
    try {
      fireEvent.change(field, { target: { value: '1' } });
      fireEvent.keyDown(field, { key: 'Enter' });
      act(() => vi.advanceTimersByTime(60));
      fireEvent.change(field, { target: { value: '3' } });
      fireEvent.keyDown(field, { key: 'Enter' });
      act(() => vi.advanceTimersByTime(60));
      expect(flashing('3: Page')).toBe(true);
      act(() => vi.advanceTimersByTime(50));
      expect(flashing('3: Page')).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });
});
