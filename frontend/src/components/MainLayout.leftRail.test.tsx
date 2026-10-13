/**
 * The left navigator rail in the main layout: rendered outside the resizable
 * split, labelled items, tab semantics and keyboard traversal, collapse on a
 * pointer click of the active item, Cmd/Ctrl+digit shortcuts, the panels it
 * switches between, the per-tab view across tab switches, and panel-size
 * persistence with the rail present.
 *
 * Run: cd frontend && npx vitest run src/components/MainLayout.leftRail.test.tsx
 */
import { render, screen, act, fireEvent, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { Dispatch, ReactNode } from 'react';
import { AppProvider, useAppDispatch, useAppState, type AppAction, type AppState } from '../hooks/useDocumentState';
import { MainLayout } from './MainLayout';

const RAIL_KEY = 'unidoc-pdf-debugger:left-rail';
const WINDOW_KEY = 'unidoc-pdf-debugger:window-state';

const allotment = vi.hoisted(() => ({
  horizontal: undefined as undefined | ((sizes: number[]) => void),
  vertical: undefined as undefined | ((sizes: number[]) => void),
}));

vi.mock('allotment', () => {
  function Pane({ children, visible, preferredSize }: { children: ReactNode; visible?: boolean; preferredSize?: number | string }) {
    return (
      <div
        data-testid="allotment-pane"
        data-visible={visible === false ? 'false' : 'true'}
        data-preferred-size={preferredSize === undefined ? '' : String(preferredSize)}
      >
        {children}
      </div>
    );
  }
  function Allotment({ children, onChange, vertical }: { children: ReactNode; onChange?: (sizes: number[]) => void; vertical?: boolean }) {
    if (vertical) allotment.vertical = onChange;
    else allotment.horizontal = onChange;
    return <div data-testid={vertical ? 'allotment-vertical' : 'allotment-horizontal'}>{children}</div>;
  }
  Allotment.Pane = Pane;
  return { Allotment };
});

vi.mock('allotment/dist/style.css', () => ({}));

const mockGetPageIndex = vi.fn();
const mockGetAncestorPath = vi.fn();
const mockGetChildren = vi.fn();

vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: vi.fn(),
  GetTreeRoot: vi.fn(),
  GetChildren: (...args: unknown[]) => mockGetChildren(...args),
  CloseDocument: vi.fn(),
  OpenFileDialog: vi.fn(),
  GetObjectDetail: vi.fn().mockResolvedValue(null),
  GetObjectSource: vi.fn().mockResolvedValue(''),
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

type RailState = AppState & { leftPanelCollapsed?: boolean };
let state: RailState;
let dispatch: Dispatch<AppAction>;

function Probe() {
  state = useAppState() as RailState;
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
    childCount: 2,
    iconHint: 'pages',
    error: '',
  },
];

function openTab(tabId = 'tab-1') {
  act(() =>
    dispatch({
      type: 'OPEN_DOCUMENT',
      payload: { tabId, fileName: 'a.pdf', filePath: `/tmp/${tabId}.pdf`, pageCount: 1, rootNode: catalogNode, rootChildren },
    }),
  );
}

// The detail panel has its own horizontal tablist; the rail is the vertical one.
function rail() {
  const lists = screen.getAllByRole('tablist').filter((l) => l.getAttribute('aria-orientation') === 'vertical');
  expect(lists).toHaveLength(1);
  return lists[0];
}

function tab(name: string) {
  return within(rail()).getByRole('tab', { name });
}

function leftPane() {
  // The first pane of the horizontal split holds the left panel.
  const horizontal = screen.getByTestId('allotment-horizontal');
  return within(horizontal).getAllByTestId('allotment-pane')[0];
}

function isCollapsed() {
  return leftPane().getAttribute('data-visible') === 'false';
}

function panelFor(tabEl: HTMLElement) {
  const id = tabEl.getAttribute('aria-controls');
  expect(id).toBeTruthy();
  const panel = document.getElementById(id!);
  expect(panel).not.toBeNull();
  return panel!;
}

function pressDigit(key: string, extra: Partial<KeyboardEventInit> = {}) {
  fireEvent.keyDown(window, { key, metaKey: true, ctrlKey: true, ...extra });
}

beforeEach(() => {
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
  window.localStorage.removeItem(RAIL_KEY);
  window.localStorage.removeItem(WINDOW_KEY);
  allotment.horizontal = undefined;
  allotment.vertical = undefined;
  mockGetPageIndex.mockReset().mockResolvedValue([]);
  mockGetAncestorPath.mockReset().mockResolvedValue(['root']);
  mockGetChildren.mockReset().mockResolvedValue([]);
});

afterEach(() => {
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

describe('rail placement', () => {
  test('the rail sits inside main-layout but outside the resizable split', () => {
    renderLayout();
    const list = rail();
    expect(screen.getByTestId('main-layout')).toContainElement(list);
    expect(screen.getByTestId('allotment-horizontal')).not.toContainElement(list);
    expect(screen.getByTestId('left-panel')).not.toContainElement(list);
  });

  test('the horizontal split still has two panes, so sizes[0] is the tree width', () => {
    renderLayout();
    const horizontal = screen.getByTestId('allotment-horizontal');
    const direct = Array.from(horizontal.children).filter((c) => c.getAttribute('data-testid') === 'allotment-pane');
    expect(direct).toHaveLength(2);
  });
});

describe('rail items', () => {
  test('renders Structure, Pages then Images, each with a visible text label', () => {
    renderLayout();
    const tabs = within(rail()).getAllByRole('tab');
    expect(tabs.map((t) => t.getAttribute('aria-label'))).toEqual(['Structure', 'Pages', 'Images']);
    for (const t of tabs) {
      const label = t.getAttribute('aria-label')!;
      const text = within(t).getByText(label);
      expect(text).toBeVisible();
      expect(text.closest('.sr-only')).toBeNull();
      expect(t.querySelector('svg')).not.toBeNull();
    }
  });

  test('labels stay rendered while the panel is collapsed', async () => {
    const user = userEvent.setup();
    renderLayout();
    await user.click(tab('Structure'));
    expect(isCollapsed()).toBe(true);
    expect(within(tab('Structure')).getByText('Structure')).toBeVisible();
    expect(within(tab('Pages')).getByText('Pages')).toBeVisible();
  });
});

describe('rail accessibility', () => {
  test('is a vertical tablist with roving tabindex and aria-selected on the active item', () => {
    renderLayout();
    rail();
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    expect(tab('Structure')).toHaveAttribute('tabindex', '0');
    expect(tab('Pages')).toHaveAttribute('aria-selected', 'false');
    expect(tab('Pages')).toHaveAttribute('tabindex', '-1');
  });

  test('each item controls a tabpanel labelled by that item, inside the left panel', () => {
    renderLayout();
    for (const t of within(rail()).getAllByRole('tab')) {
      const panel = panelFor(t);
      expect(panel).toHaveAttribute('role', 'tabpanel');
      expect(panel).toHaveAttribute('aria-labelledby', t.id);
      expect(screen.getByTestId('left-panel')).toContainElement(panel);
    }
  });

  test('Down and Up move focus and selection, wrapping at the ends', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    tab('Structure').focus();
    await user.keyboard('{ArrowDown}');
    expect(tab('Pages')).toHaveFocus();
    expect(tab('Pages')).toHaveAttribute('aria-selected', 'true');
    expect(tab('Pages')).toHaveAttribute('tabindex', '0');
    expect(tab('Structure')).toHaveAttribute('tabindex', '-1');
    await user.keyboard('{ArrowDown}');
    expect(tab('Images')).toHaveFocus();
    await user.keyboard('{ArrowDown}');
    expect(tab('Structure')).toHaveFocus();
    await user.keyboard('{ArrowUp}');
    expect(tab('Images')).toHaveFocus();
  });

  test('Home and End jump to the first and last item', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    tab('Structure').focus();
    await user.keyboard('{End}');
    expect(tab('Images')).toHaveFocus();
    expect(tab('Images')).toHaveAttribute('aria-selected', 'true');
    await user.keyboard('{Home}');
    expect(tab('Structure')).toHaveFocus();
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
  });

  test('arrow keys onto the active view never collapse the panel', async () => {
    const user = userEvent.setup();
    renderLayout();
    tab('Structure').focus();
    await user.keyboard('{ArrowDown}{ArrowUp}{Home}{End}');
    expect(isCollapsed()).toBe(false);
  });

  test('the active item exposes whether the panel is expanded', async () => {
    const user = userEvent.setup();
    renderLayout();
    expect(tab('Structure')).toHaveAttribute('aria-expanded', 'true');
    await user.click(tab('Structure'));
    expect(tab('Structure')).toHaveAttribute('aria-expanded', 'false');
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
  });
});

describe('rail active and focus styling', () => {
  // The fill span is the rounded square behind the icon; the bar span is the edge marker.
  function fillOf(t: HTMLElement) {
    return t.querySelector('svg')!.parentElement!.className;
  }
  function barOf(t: HTMLElement) {
    return (t.firstElementChild as HTMLElement).className;
  }

  test('every item has a focus-visible ring that is separate from the active fill', () => {
    renderLayout();
    for (const t of within(rail()).getAllByRole('tab')) {
      expect(t.className).toMatch(/focus-visible:ring-2/);
      expect(fillOf(t)).not.toMatch(/ring/);
    }
    expect(fillOf(tab('Structure'))).not.toBe(fillOf(tab('Pages')));
  });

  test('the active item renders dimmed while collapsed, distinct from expanded and inactive', async () => {
    const user = userEvent.setup();
    renderLayout();
    const expandedFill = fillOf(tab('Structure'));
    const expandedBar = barOf(tab('Structure'));
    const inactiveFill = fillOf(tab('Pages'));
    await user.click(tab('Structure'));
    expect(isCollapsed()).toBe(true);
    const collapsedFill = fillOf(tab('Structure'));
    expect(collapsedFill).not.toBe(expandedFill);
    expect(collapsedFill).not.toBe(inactiveFill);
    expect(barOf(tab('Structure'))).not.toBe(expandedBar);
  });
});

describe('panels', () => {
  test('both panels stay mounted; the inactive one is invisible, not unmounted or display:none', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const structurePanel = panelFor(tab('Structure'));
    const pagesPanel = panelFor(tab('Pages'));
    expect(within(structurePanel).getByTestId('tree-panel')).toBeInTheDocument();
    expect(pagesPanel).toHaveClass('invisible');
    expect(structurePanel).not.toHaveClass('invisible');

    await user.click(tab('Pages'));
    expect(structurePanel).toHaveClass('invisible');
    expect(structurePanel).not.toHaveClass('hidden');
    expect(pagesPanel).not.toHaveClass('invisible');
    expect(within(structurePanel).getByTestId('tree-panel')).toBeInTheDocument();
    expect(within(structurePanel).getAllByTestId('tree-node').length).toBeGreaterThan(0);
  });

  test('the object source pane stays below whichever navigator is active', async () => {
    const user = userEvent.setup();
    renderLayout();
    const vertical = screen.getByTestId('allotment-vertical');
    const bottom = within(vertical).getAllByTestId('allotment-pane')[1];
    const sourceBefore = bottom.innerHTML;
    await user.click(tab('Pages'));
    expect(within(vertical).getAllByTestId('allotment-pane')[1].innerHTML).toBe(sourceBefore);
    expect(bottom).not.toContainElement(panelFor(tab('Pages')));
  });

  test('Pages does not fetch until it is first shown', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    expect(mockGetPageIndex).not.toHaveBeenCalled();
    await user.click(tab('Pages'));
    await waitFor(() => expect(mockGetPageIndex).toHaveBeenCalledWith('tab-1'));
  });
});

describe('collapse', () => {
  test('a pointer click on the active item collapses the panel and a second click restores it', async () => {
    const user = userEvent.setup();
    renderLayout();
    await user.click(tab('Structure'));
    expect(isCollapsed()).toBe(true);
    await user.click(tab('Structure'));
    expect(isCollapsed()).toBe(false);
  });

  test('a pointer click on an inactive item switches view and never collapses', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Pages'));
    expect(tab('Pages')).toHaveAttribute('aria-selected', 'true');
    expect(isCollapsed()).toBe(false);
  });

  test('clicking an inactive item while collapsed switches view and restores the panel', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Structure'));
    await user.click(tab('Pages'));
    expect(tab('Pages')).toHaveAttribute('aria-selected', 'true');
    expect(isCollapsed()).toBe(false);
  });

  test('Enter and Space on the active item do not collapse', async () => {
    const user = userEvent.setup();
    renderLayout();
    tab('Structure').focus();
    await user.keyboard('{Enter}');
    expect(isCollapsed()).toBe(false);
    await user.keyboard(' ');
    expect(isCollapsed()).toBe(false);
  });

  test('a click with detail 0 (keyboard activation) does not collapse', () => {
    renderLayout();
    fireEvent.click(tab('Structure'), { detail: 0 });
    expect(isCollapsed()).toBe(false);
  });

  test('Enter on the active item while collapsed restores the panel', async () => {
    const user = userEvent.setup();
    renderLayout();
    await user.click(tab('Structure'));
    expect(isCollapsed()).toBe(true);
    tab('Structure').focus();
    await user.keyboard('{Enter}');
    expect(isCollapsed()).toBe(false);
  });

  test('the collapse state is written to its own key, not to the panel sizes', async () => {
    const user = userEvent.setup();
    const view = renderLayout();
    await user.click(tab('Structure'));
    expect(JSON.parse(window.localStorage.getItem(RAIL_KEY) ?? 'null')).toEqual({ collapsed: true });
    view.unmount();
    const stored = JSON.parse(window.localStorage.getItem(WINDOW_KEY) ?? '{}');
    expect(stored.panelSizes?.collapsed).toBeUndefined();
  });

  test('a persisted collapsed state starts collapsed, on Structure', () => {
    window.localStorage.setItem(RAIL_KEY, JSON.stringify({ view: 'pages', collapsed: true }));
    renderLayout();
    expect(isCollapsed()).toBe(true);
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
  });

  test('an unknown view id resolves to the first destination', () => {
    renderLayout();
    act(() => dispatch({ type: 'SELECT_LEFT_VIEW', payload: { view: 'bookmarks' } }));
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    expect(panelFor(tab('Structure'))).not.toHaveClass('invisible');
  });
});

describe('Cmd/Ctrl+digit shortcuts', () => {
  test('Cmd+2 selects Pages and focuses it; Cmd+1 goes back to Structure', () => {
    renderLayout();
    openTab();
    pressDigit('2');
    expect(tab('Pages')).toHaveAttribute('aria-selected', 'true');
    expect(tab('Pages')).toHaveFocus();
    pressDigit('1');
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    expect(tab('Structure')).toHaveFocus();
  });

  test('the shortcut for the already-active item un-collapses and never collapses', async () => {
    const user = userEvent.setup();
    renderLayout();
    pressDigit('1');
    expect(isCollapsed()).toBe(false);
    await user.click(tab('Structure'));
    expect(isCollapsed()).toBe(true);
    pressDigit('1');
    expect(isCollapsed()).toBe(false);
    expect(tab('Structure')).toHaveFocus();
  });

  test('prevents the default action on a match', () => {
    renderLayout();
    const ev = new KeyboardEvent('keydown', { key: '2', metaKey: true, ctrlKey: true, bubbles: true, cancelable: true });
    act(() => {
      window.dispatchEvent(ev);
    });
    expect(ev.defaultPrevented).toBe(true);
  });

  test.each([
    ['Shift', { shiftKey: true }],
    ['Alt', { altKey: true }],
  ])('does nothing with %s also held', (_name, extra) => {
    renderLayout();
    pressDigit('2', extra);
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
  });

  test('a digit past the registry length does nothing', () => {
    renderLayout();
    pressDigit('4');
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
  });

  test('a bare digit without the modifier does nothing', () => {
    renderLayout();
    fireEvent.keyDown(window, { key: '2' });
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
  });

  test('fires from a text field', () => {
    renderLayout();
    openTab();
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    fireEvent.keyDown(input, { key: '2', metaKey: true, ctrlKey: true });
    expect(tab('Pages')).toHaveAttribute('aria-selected', 'true');
    input.remove();
  });

  test('with no document open it stays on Structure and still un-collapses', async () => {
    const user = userEvent.setup();
    renderLayout();
    await user.click(tab('Structure'));
    expect(isCollapsed()).toBe(true);
    pressDigit('2');
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    expect(isCollapsed()).toBe(false);
  });
});

describe('the rail view is per tab', () => {
  test('the highlighted item and the shown panel follow the active tab', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    await user.click(tab('Pages'));
    openTab('tab-2');
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    expect(panelFor(tab('Structure'))).not.toHaveClass('invisible');
    expect(panelFor(tab('Pages'))).toHaveClass('invisible');

    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } }));
    expect(tab('Pages')).toHaveAttribute('aria-selected', 'true');
    expect(panelFor(tab('Pages'))).not.toHaveClass('invisible');
    expect(panelFor(tab('Structure'))).toHaveClass('invisible');

    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } }));
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    expect(panelFor(tab('Structure'))).not.toHaveClass('invisible');
  });
});

describe('reveals from outside the tree', () => {
  test('NAVIGATE_TO_REF while Pages is active switches back to Structure and runs the reveal', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Pages'));
    act(() => dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: 'obj:0:2' } }));
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(mockGetAncestorPath).toHaveBeenCalledWith('tab-1', 'obj:0:2'));
    await waitFor(() => expect(state.tabs[0].selectedNodeId).toBe('obj:0:2'));
  });

  test('NAVIGATE_TO_REF while collapsed restores the panel', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Structure'));
    expect(isCollapsed()).toBe(true);
    act(() => dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: 'obj:0:2' } }));
    expect(isCollapsed()).toBe(false);
    await waitFor(() => expect(mockGetAncestorPath).toHaveBeenCalledWith('tab-1', 'obj:0:2'));
  });

  test('a reveal requested while the tree has no size runs once it gets one', async () => {
    // A collapsed pane is zero-sized until allotment restores it; model that
    // with observers that report 0x0 until resized by hand.
    const observers: { cb: ResizeObserverCallback; target: Element; self: ResizeObserver }[] = [];
    class ZeroThenSizedObserver {
      cb: ResizeObserverCallback;
      constructor(cb: ResizeObserverCallback) {
        this.cb = cb;
      }
      observe(target: Element) {
        const self = this as unknown as ResizeObserver;
        observers.push({ cb: this.cb, target, self });
        this.cb([{ target, contentRect: { width: 0, height: 0 } as DOMRectReadOnly } as ResizeObserverEntry], self);
      }
      unobserve() {}
      disconnect() {}
    }
    (globalThis as Record<string, unknown>).ResizeObserver = ZeroThenSizedObserver;

    renderLayout();
    openTab();
    act(() => dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: 'obj:0:2' } }));
    await new Promise((r) => setTimeout(r, 20));
    expect(mockGetAncestorPath).not.toHaveBeenCalled();
    expect(state.tabs[0].pendingNavTarget).toBe('obj:0:2');

    act(() => {
      for (const o of observers) {
        o.cb([{ target: o.target, contentRect: { width: 300, height: 600 } as DOMRectReadOnly } as ResizeObserverEntry], o.self);
      }
    });
    await waitFor(() => expect(mockGetAncestorPath).toHaveBeenCalledWith('tab-1', 'obj:0:2'));
    await waitFor(() => expect(state.tabs[0].selectedNodeId).toBe('obj:0:2'));
    expect(state.tabs[0].pendingNavTarget).toBeNull();
  });

  test('a pane shrinking to zero on collapse keeps both trees mounted', async () => {
    const observers: { cb: ResizeObserverCallback; target: Element; self: ResizeObserver }[] = [];
    class ResizableObserver {
      cb: ResizeObserverCallback;
      constructor(cb: ResizeObserverCallback) {
        this.cb = cb;
      }
      observe(target: Element) {
        const self = this as unknown as ResizeObserver;
        observers.push({ cb: this.cb, target, self });
        this.cb([{ target, contentRect: { width: 300, height: 600 } as DOMRectReadOnly } as ResizeObserverEntry], self);
      }
      unobserve() {}
      disconnect() {}
    }
    (globalThis as Record<string, unknown>).ResizeObserver = ResizableObserver;
    mockGetPageIndex.mockResolvedValue([
      { pageNum: 1, objNum: 3, gen: 0, nodeId: 'obj:0:3', contentNodeId: '', mediaBox: [0, 0, 612, 792], rotate: 0, inherited: 0, annotCount: 0, contentLen: -1, error: '' },
    ]);

    renderLayout();
    openTab();
    act(() => dispatch({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } }));
    const structureRow = await within(screen.getByTestId('tree-panel')).findByText('Pages');
    const pagesPanel = panelFor(tab('Pages'));
    const pageRow = await within(pagesPanel).findByText(/^1:/);

    act(() => {
      for (const o of observers) {
        o.cb([{ target: o.target, contentRect: { width: 0, height: 0 } as DOMRectReadOnly } as ResizeObserverEntry], o.self);
      }
    });

    expect(structureRow.isConnected).toBe(true);
    expect(pageRow.isConnected).toBe(true);
  });
});

describe('panel sizes with the rail present', () => {
  function stored() {
    return JSON.parse(window.localStorage.getItem(WINDOW_KEY) ?? '{}').panelSizes;
  }

  test('the stored tree width is the left pane preferred size', () => {
    window.localStorage.setItem(WINDOW_KEY, JSON.stringify({ panelSizes: { treeWidth: 420, subPanelHeight: 180, treePaneHeight: 500 } }));
    renderLayout();
    expect(leftPane()).toHaveAttribute('data-preferred-size', '420');
  });

  test('treeWidth, subPanelHeight and treePaneHeight round-trip unchanged', () => {
    window.localStorage.setItem(WINDOW_KEY, JSON.stringify({ panelSizes: { treeWidth: 420, subPanelHeight: 180, treePaneHeight: 500 } }));
    const view = renderLayout();
    act(() => allotment.vertical?.([510, 190]));
    act(() => allotment.horizontal?.([430, 900]));
    view.unmount();
    expect(stored()).toEqual({ treeWidth: 430, subPanelHeight: 190, treePaneHeight: 510 });
  });

  test('a size change while collapsed is not saved', async () => {
    window.localStorage.setItem(WINDOW_KEY, JSON.stringify({ panelSizes: { treeWidth: 420, subPanelHeight: 180 } }));
    const user = userEvent.setup();
    const view = renderLayout();
    await user.click(tab('Structure'));
    expect(isCollapsed()).toBe(true);
    act(() => allotment.horizontal?.([0, 1320]));
    act(() => allotment.horizontal?.([150, 1170]));
    view.unmount();
    expect(stored().treeWidth).toBe(420);
  });

  test('a zero width is never saved, even when not collapsed', () => {
    window.localStorage.setItem(WINDOW_KEY, JSON.stringify({ panelSizes: { treeWidth: 420, subPanelHeight: 180 } }));
    const view = renderLayout();
    act(() => allotment.horizontal?.([0, 1320]));
    view.unmount();
    expect(stored().treeWidth).toBe(420);
  });

  test('after collapse and restore, the next drag saves again', async () => {
    window.localStorage.setItem(WINDOW_KEY, JSON.stringify({ panelSizes: { treeWidth: 420, subPanelHeight: 180 } }));
    const user = userEvent.setup();
    const view = renderLayout();
    await user.click(tab('Structure'));
    await user.click(tab('Structure'));
    act(() => allotment.horizontal?.([460, 860]));
    view.unmount();
    expect(stored().treeWidth).toBe(460);
  });
});
