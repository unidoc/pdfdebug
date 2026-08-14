/**
 * Tree Panel with Lazy-Loading Navigation
 *
 * Test IDs: through
 * Run: cd frontend && npx vitest run src/components/TreePanel.test.tsx
 */
import { render, screen, act, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  useAppState,
  type AppAction,
} from '../hooks/useDocumentState';
import { TreePanel } from './TreePanel';

// Helper for tests that use dynamic import pattern (returns the already-imported component)
async function importTreePanel() {
  return TreePanel;
}

// Mock allotment -- jsdom has no layout APIs
vi.mock('allotment', () => {
  function Pane({ children }: { children: React.ReactNode }) {
    return <div>{children}</div>;
  }
  function Allotment({ children }: { children: React.ReactNode }) {
    return <div>{children}</div>;
  }
  Allotment.Pane = Pane;
  return { Allotment };
});

vi.mock('allotment/dist/style.css', () => ({}));

// Mock Wails bindings
const mockGetChildren = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: (...args: unknown[]) => mockGetChildren(...args),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: vi.fn(),
  })
);

// Mock ResizeObserver -- jsdom does not provide it
class MockResizeObserver {
  callback: ResizeObserverCallback;
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }
  observe(target: Element) {
    // Fire immediately with a reasonable size so react-arborist has dimensions
    this.callback(
      [
        {
          target,
          contentRect: { width: 300, height: 600 } as DOMRectReadOnly,
          borderBoxSize: [],
          contentBoxSize: [],
          devicePixelContentBoxSize: [],
        } as ResizeObserverEntry,
      ],
      this
    );
  }
  unobserve() {}
  disconnect() {}
}

// --- Test data fixtures ---

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 3,
  iconHint: 'catalog',
  error: '',
};

const childNodes = [
  {
    id: 'dict:root:Type',
    label: 'Type',
    rawKey: '/Type',
    nodeType: 'scalar',
    valueType: 'name',
    hasChildren: false,
    childCount: 0,
    iconHint: 'default',
    error: '',
  },
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
  {
    id: 'obj:0:3',
    label: 'Metadata',
    rawKey: '/Metadata',
    nodeType: 'stream',
    valueType: 'reference',
    hasChildren: true,
    childCount: 1,
    iconHint: 'stream',
    error: '',
  },
];

const errorChildNode = {
  id: 'obj:0:99',
  label: 'Error: malformed object',
  rawKey: '/Broken',
  nodeType: 'dict',
  valueType: '',
  hasChildren: false,
  childCount: 0,
  iconHint: 'default',
  error: 'malformed object at offset 1234',
};

const pagesChildren = [
  {
    id: 'obj:0:4',
    label: 'Page 1',
    rawKey: '/Page',
    nodeType: 'dict',
    valueType: 'reference',
    hasChildren: true,
    childCount: 5,
    iconHint: 'page',
    error: '',
  },
  {
    id: 'obj:0:5',
    label: 'Page 2',
    rawKey: '/Page',
    nodeType: 'dict',
    valueType: 'reference',
    hasChildren: false,
    childCount: 0,
    iconHint: 'page',
    error: '',
  },
];

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: {
    tabId: 'tab-1',
    fileName: 'test.pdf',
    filePath: '/path/to/test.pdf',
    rootNode: catalogNode,
    rootChildren: childNodes,
  },
};

const openActionWithError: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: {
    tabId: 'tab-err',
    fileName: 'broken.pdf',
    filePath: '/path/to/broken.pdf',
    rootNode: catalogNode,
    rootChildren: [...childNodes, errorChildNode],
  },
};

// --- Helper to dispatch an action then render a component ---

function DispatchAndRender({
  action,
  children,
}: {
  action: AppAction;
  children: React.ReactNode;
}) {
  const dispatch = useAppDispatch();
  return (
    <div>
      <button data-testid="dispatch" onClick={() => dispatch(action)} />
      {children}
    </div>
  );
}

// --- Helper to read selectedNodeId from state ---

function SelectedNodeIdReader() {
  const state = useAppState();
  const activeTab = state.tabs.find((t) => t.tabId === state.activeTabId);
  return (
    <span data-testid="selected-node-id">
      {activeTab && 'selectedNodeId' in activeTab
        ? String((activeTab as Record<string, unknown>).selectedNodeId ?? '')
        : ''}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.clearAllMocks();
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
});

afterEach(() => {
  vi.useRealTimers();
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

// ---------------------------------------------------------------------------
// TreePanel renders root node and expands on click: Given a PDF is
// opened, the tree root is displayed. When the user
//       clicks the expand arrow, child nodes load from the Go backend.
// ---------------------------------------------------------------------------

describe('TreePanel renders root and expands on click', () => {
  test('renders tree-panel container with data-testid', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    expect(screen.getByTestId('tree-panel')).toBeInTheDocument();
  });

  test('renders root Catalog node after OPEN_DOCUMENT', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    expect(screen.getByText('Catalog')).toBeInTheDocument();
  });

  test('renders immediate children of the root (pre-loaded from story 2-4)', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Root children should be visible (root is pre-expanded with children)
    expect(screen.getByText('Type')).toBeInTheDocument();
    expect(screen.getByText('Pages')).toBeInTheDocument();
    expect(screen.getByText('Metadata')).toBeInTheDocument();
  });

  test('expanding a child node calls GetChildren and renders fetched children', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    mockGetChildren.mockResolvedValueOnce(pagesChildren);

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Find the "Pages" node and click to expand it
    const pagesNode = screen.getByText('Pages');
    const pagesRow = pagesNode.closest('[data-testid="tree-node"]');
    expect(pagesRow).toBeTruthy();

    // Click the row to select, then Right arrow to expand
    await user.click(pagesRow!);
    await user.keyboard('{ArrowRight}');

    await waitFor(() => {
      expect(mockGetChildren).toHaveBeenCalledWith('tab-1', 'obj:0:2');
    });

    // After expansion, children should appear
    await waitFor(() => {
      expect(screen.getByText('Page 1')).toBeInTheDocument();
      expect(screen.getByText('Page 2')).toBeInTheDocument();
    });
  });

  test('tree nodes have data-testid="tree-node" and data-node-id attributes', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    const treeNodes = screen.getAllByTestId('tree-node');
    expect(treeNodes.length).toBeGreaterThanOrEqual(1);

    // At least one node should have data-node-id
    const hasNodeId = treeNodes.some(
      (node) => node.getAttribute('data-node-id') !== null
    );
    expect(hasNodeId).toBe(true);
  });

  test('has "Document Structure" header', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    expect(screen.getByText('Document Structure')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// TreePanel keyboard navigation: Arrow keys move selection, Right
// expands, Left collapses.
// ---------------------------------------------------------------------------

describe('TreePanel keyboard navigation', () => {
  test('Down arrow moves focus/selection to next node', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
          <SelectedNodeIdReader />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Click Catalog to focus the tree
    const catalogRow = screen.getByText('Catalog').closest('[data-testid="tree-node"]');
    await user.click(catalogRow!);

    // Press Down arrow to move to next sibling (Type)
    await user.keyboard('{ArrowDown}');

    await waitFor(() => {
      // The selection should have moved -- check via aria-selected or state
      const selectedNodeId = screen.getByTestId('selected-node-id').textContent;
      expect(selectedNodeId).toBeTruthy();
      expect(selectedNodeId).not.toBe('root');
    });
  });

  test('Right arrow expands a collapsed node', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    mockGetChildren.mockResolvedValueOnce(pagesChildren);

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Click Pages node to select it
    const pagesRow = screen.getByText('Pages').closest('[data-testid="tree-node"]');
    await user.click(pagesRow!);

    // Press Right to expand
    await user.keyboard('{ArrowRight}');

    await waitFor(() => {
      expect(mockGetChildren).toHaveBeenCalledWith('tab-1', 'obj:0:2');
    });

    await waitFor(() => {
      expect(screen.getByText('Page 1')).toBeInTheDocument();
    });
  });

  test('Left arrow collapses an expanded node', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    mockGetChildren.mockResolvedValueOnce(pagesChildren);

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Expand Pages first
    const pagesRow = screen.getByText('Pages').closest('[data-testid="tree-node"]');
    await user.click(pagesRow!);
    await user.keyboard('{ArrowRight}');

    await waitFor(() => {
      expect(screen.getByText('Page 1')).toBeInTheDocument();
    });

    // Now press Left to collapse
    await user.keyboard('{ArrowLeft}');

    await waitFor(() => {
      expect(screen.queryByText('Page 1')).not.toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// ARIA roles and attributes: container has role="tree", nodes
// have role="treeitem",
//       aria-expanded, aria-level.
// ---------------------------------------------------------------------------

describe('TreePanel ARIA accessibility', () => {
  test('tree container has role="tree"', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    const treeContainer = screen.getByRole('tree');
    expect(treeContainer).toBeInTheDocument();
  });

  test('tree nodes have role="treeitem"', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    const treeItems = screen.getAllByRole('treeitem');
    expect(treeItems.length).toBeGreaterThanOrEqual(1);
  });

  test('expandable nodes have aria-expanded attribute', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    const treeItems = screen.getAllByRole('treeitem');
    // At least one node (Catalog, Pages, Metadata) should have aria-expanded
    const hasExpanded = treeItems.some(
      (item) => item.getAttribute('aria-expanded') !== null
    );
    expect(hasExpanded).toBe(true);
  });

  test('nodes have aria-level attribute', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    const treeItems = screen.getAllByRole('treeitem');
    const hasLevel = treeItems.some(
      (item) => item.getAttribute('aria-level') !== null
    );
    expect(hasLevel).toBe(true);
  });

  test('selected node has aria-selected="true"', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Click a node to select it
    const catalogRow = screen.getByText('Catalog').closest('[role="treeitem"]');
    if (catalogRow) {
      await user.click(catalogRow);

      await waitFor(() => {
        expect(catalogRow.getAttribute('aria-selected')).toBe('true');
      });
    } else {
      // If treeitem is not directly the clickable element, find by testid
      const treeNode = screen.getByText('Catalog').closest('[data-testid="tree-node"]');
      await user.click(treeNode!);

      await waitFor(() => {
        const selected = screen
          .getAllByRole('treeitem')
          .find((item) => item.getAttribute('aria-selected') === 'true');
        expect(selected).toBeTruthy();
      });
    }
  });
});

// ---------------------------------------------------------------------------
// Loading indicator appears only after 200ms delay: A subtle loading
// indicator (pulse animation) appears only if
//       loading takes more than 200ms.
// ---------------------------------------------------------------------------

describe('Loading indicator with 200ms threshold', () => {
  test('no loading indicator for fast responses (< 200ms)', async () => {
    vi.useFakeTimers();

    // GetChildren resolves immediately
    mockGetChildren.mockResolvedValueOnce(pagesChildren);

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    await act(async () => screen.getByTestId('dispatch').click());

    // Select Pages, then expand via ArrowRight
    const pagesRow = screen.getByText('Pages').closest('[data-testid="tree-node"]');
    await act(async () => {
      pagesRow?.click();
    });
    // Trigger expand via keyboard (Right arrow on the focused tree container)
    const treeContainer = screen.getByRole('tree');
    await act(async () => {
      treeContainer.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
    });

    // Advance less than 200ms
    await act(async () => {
      vi.advanceTimersByTime(100);
    });

    // No loading indicator should be visible
    const loadingIndicator = screen.queryByTestId('tree-loading-indicator');
    // If there is a loading indicator element, it should not be visible
    // If there is none, that is also correct (no indicator for fast loads)
    if (loadingIndicator) {
      expect(loadingIndicator).not.toBeVisible();
    }

    // Let the promise resolve
    await act(async () => {
      vi.advanceTimersByTime(200);
    });

    vi.useRealTimers();
  });

  test('loading indicator appears after 200ms for slow responses', async () => {
    vi.useFakeTimers();

    // GetChildren that never resolves (simulates slow backend)
    let resolveGetChildren!: (value: unknown) => void;
    mockGetChildren.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveGetChildren = resolve;
      })
    );

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    await act(async () => screen.getByTestId('dispatch').click());

    // Select Pages, then expand via ArrowRight
    const pagesRow = screen.getByText('Pages').closest('[data-testid="tree-node"]');
    await act(async () => {
      pagesRow?.click();
    });
    const treeContainer = screen.getByRole('tree');
    await act(async () => {
      treeContainer.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
    });

    // Before 200ms: no indicator
    await act(async () => {
      vi.advanceTimersByTime(150);
    });

    // After 200ms: indicator should appear
    await act(async () => {
      vi.advanceTimersByTime(100);
    });

    // Look for animate-pulse class or a loading indicator element
    const treePanel = screen.getByTestId('tree-panel');
    const hasPulse =
      treePanel.querySelector('.animate-pulse') !== null ||
      screen.queryByTestId('tree-loading-indicator') !== null;
    expect(hasPulse).toBe(true);

    // Resolve to clean up
    await act(async () => {
      resolveGetChildren(pagesChildren);
      vi.advanceTimersByTime(100);
    });

    vi.useRealTimers();
  });
});

// ---------------------------------------------------------------------------
// SELECT_NODE dispatch on selection change
// ---------------------------------------------------------------------------

describe('SELECT_NODE dispatch on selection', () => {
  test('clicking a node dispatches SELECT_NODE with the node ID', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
          <SelectedNodeIdReader />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Click the Type leaf node
    const typeNode = screen.getByText('Type').closest('[data-testid="tree-node"]');
    await user.click(typeNode!);

    await waitFor(() => {
      const selectedId = screen.getByTestId('selected-node-id').textContent;
      expect(selectedId).toBe('dict:root:Type');
    });
  });

  test('selected node has visible highlight styling', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    const typeNode = screen.getByText('Type').closest('[data-testid="tree-node"]');
    await user.click(typeNode!);

    await waitFor(() => {
      // Re-query after re-render since react-window may replace DOM nodes
      const updatedNode = screen.getByText('Type').closest('[data-testid="tree-node"]');
      // Selected node should have bg-surface-selected class
      expect(updatedNode!.className).toContain('bg-surface-selected');
    });
  });
});

// ---------------------------------------------------------------------------
// Error nodes display with muted text and warning icon
// ---------------------------------------------------------------------------

describe('Error node rendering', () => {
  test('error node displays with text-text-muted label', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openActionWithError}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    const errorLabel = screen.getByText('Error: malformed object');
    expect(errorLabel).toBeInTheDocument();
    // Error node label should have muted text styling
    const errorRow = errorLabel.closest('[data-testid="tree-node"]');
    expect(errorRow).toBeTruthy();
    expect(errorRow!.innerHTML).toMatch(/text-text-muted/);
  });

  test('error node displays with text-error warning icon', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openActionWithError}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    const errorRow = screen
      .getByText('Error: malformed object')
      .closest('[data-testid="tree-node"]');
    expect(errorRow).toBeTruthy();
    // Must have a text-error colored warning icon
    expect(errorRow!.innerHTML).toMatch(/text-error/);
  });

  test('error node is navigable and does not crash the tree', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    render(
      <AppProvider>
        <DispatchAndRender action={openActionWithError}>
          <TreePanel />
          <SelectedNodeIdReader />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Click the error node -- should not throw
    const errorRow = screen
      .getByText('Error: malformed object')
      .closest('[data-testid="tree-node"]');
    await user.click(errorRow!);

    await waitFor(() => {
      const selectedId = screen.getByTestId('selected-node-id').textContent;
      expect(selectedId).toBe('obj:0:99');
    });
  });
});

// ---------------------------------------------------------------------------
// Node labels show rawKey alongside label when different
// ---------------------------------------------------------------------------

describe('Node label display with rawKey', () => {
  test('shows rawKey in muted text when different from label', async () => {

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Pages node has rawKey="/Pages" which differs from label "Pages"
    // The rawKey should be shown alongside the label
    const pagesRow = screen.getByText('Pages').closest('[data-testid="tree-node"]');
    expect(pagesRow).toBeTruthy();
    expect(within(pagesRow!).getByText('/Pages')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// NodeRenderer renders lucide icons keyed by iconHint
// ---------------------------------------------------------------------------

describe('Tree icons rendered from iconHint', () => {
  test('catalog/pages/page/stream rows each render an SVG icon; default hint omits the icon', async () => {
    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    const rowFor = (label: string) => {
      const row = screen.getByText(label).closest('[data-testid="tree-node"]');
      if (row === null) throw new Error(`tree row for label ${label} not found`);
      return row as HTMLElement;
    };

    // catalog (root), pages (/Pages ref), stream (/Metadata) all carry an icon.
    expect(rowFor('Catalog').querySelector('svg')).not.toBeNull();
    expect(rowFor('Pages').querySelector('svg')).not.toBeNull();
    expect(rowFor('Metadata').querySelector('svg')).not.toBeNull();

    // The /Type row (iconHint=default) intentionally has no icon so the tree
    // stays visually quiet for untyped scalars.
    expect(rowFor('Type').querySelector('svg')).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// useDocumentState: SELECT_NODE action type must exist
// ---------------------------------------------------------------------------

describe('SELECT_NODE action in useDocumentState', () => {
  test('TabState has selectedNodeId field initialized to null on OPEN_DOCUMENT', async () => {
    // This test validates that the state shape is correct after story 2-5 changes.
    // The reducer must initialize selectedNodeId: null on OPEN_DOCUMENT.

    function StateInspector() {
      const dispatch = useAppDispatch();
      const state = useAppState();
      const activeTab = state.tabs.find((t) => t.tabId === state.activeTabId);
      const hasField = activeTab && 'selectedNodeId' in activeTab;
      const value = hasField
        ? String((activeTab as Record<string, unknown>).selectedNodeId)
        : 'MISSING';

      return (
        <div>
          <button
            data-testid="open"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: {
                  tabId: 'tab-state-test',
                  fileName: 'test.pdf',
                  filePath: '/path/to/test.pdf',
                  rootNode: catalogNode,
                  rootChildren: childNodes,
                },
              })
            }
          />
          <span data-testid="field-value">{value}</span>
        </div>
      );
    }

    render(
      <AppProvider>
        <StateInspector />
      </AppProvider>
    );

    act(() => screen.getByTestId('open').click());

    // selectedNodeId must exist and be null (displayed as "null")
    expect(screen.getByTestId('field-value').textContent).toBe('null');
  });

  test('SELECT_NODE action updates selectedNodeId on active tab', async () => {
    function StateInspector() {
      const dispatch = useAppDispatch();
      const state = useAppState();
      const activeTab = state.tabs.find((t) => t.tabId === state.activeTabId);
      const value =
        activeTab && 'selectedNodeId' in activeTab
          ? String((activeTab as Record<string, unknown>).selectedNodeId ?? '')
          : 'MISSING';

      return (
        <div>
          <button
            data-testid="open"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: {
                  tabId: 'tab-sel',
                  fileName: 'test.pdf',
                  filePath: '/path/to/test-sel.pdf',
                  rootNode: catalogNode,
                  rootChildren: childNodes,
                },
              })
            }
          />
          <button
            data-testid="select"
            onClick={() =>
              dispatch({
                type: 'SELECT_NODE',
                payload: { nodeId: 'obj:0:2' },
              } as AppAction)
            }
          />
          <span data-testid="selected">{value}</span>
        </div>
      );
    }

    render(
      <AppProvider>
        <StateInspector />
      </AppProvider>
    );

    act(() => screen.getByTestId('open').click());
    act(() => screen.getByTestId('select').click());

    expect(screen.getByTestId('selected').textContent).toBe('obj:0:2');
  });

  test('SELECT_NODE is no-op when no active tab', async () => {
    function StateInspector() {
      const dispatch = useAppDispatch();
      const state = useAppState();
      return (
        <div>
          <button
            data-testid="select-no-tab"
            onClick={() =>
              dispatch({
                type: 'SELECT_NODE',
                payload: { nodeId: 'obj:0:2' },
              } as AppAction)
            }
          />
          <span data-testid="tabs-count">{state.tabs.length}</span>
          <span data-testid="active-tab">{String(state.activeTabId)}</span>
        </div>
      );
    }

    render(
      <AppProvider>
        <StateInspector />
      </AppProvider>
    );

    // No OPEN_DOCUMENT dispatched -- no tabs, no activeTabId
    expect(screen.getByTestId('tabs-count').textContent).toBe('0');
    expect(screen.getByTestId('active-tab').textContent).toBe('null');

    // SELECT_NODE should be a no-op, not crash
    act(() => screen.getByTestId('select-no-tab').click());

    expect(screen.getByTestId('tabs-count').textContent).toBe('0');
  });
});

// ---------------------------------------------------------------------------
// Extended keyboard navigation (Space, Home, End): Space on leaf selects,
// Space on internal toggles, Home/End jumps.
// ---------------------------------------------------------------------------

describe('Extended keyboard navigation', () => {
  test('Home key jumps to first visible node', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
          <SelectedNodeIdReader />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Click the last visible child to select it
    const metadataRow = screen.getByText('Metadata').closest('[data-testid="tree-node"]');
    await user.click(metadataRow!);

    await waitFor(() => {
      const selectedId = screen.getByTestId('selected-node-id').textContent;
      expect(selectedId).toBe('obj:0:3');
    });

    // Press Home to jump to the first node
    await user.keyboard('{Home}');

    await waitFor(() => {
      const selectedId = screen.getByTestId('selected-node-id').textContent;
      expect(selectedId).toBe('root');
    });
  });

  test('End key jumps to last visible node', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
          <SelectedNodeIdReader />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Click first node
    const catalogRow = screen.getByText('Catalog').closest('[data-testid="tree-node"]');
    await user.click(catalogRow!);

    // Press End to jump to last visible node
    await user.keyboard('{End}');

    await waitFor(() => {
      const selectedId = screen.getByTestId('selected-node-id').textContent;
      // Last visible node is Metadata (obj:0:3)
      expect(selectedId).toBe('obj:0:3');
    });
  });
});

// ---------------------------------------------------------------------------
// GetChildren failure does not crash the tree
// ---------------------------------------------------------------------------

describe('GetChildren error handling', () => {
  test('GetChildren rejection does not crash the tree', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    mockGetChildren.mockRejectedValueOnce(new Error('backend unavailable'));

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Try to expand Pages -- GetChildren will reject
    const pagesRow = screen.getByText('Pages').closest('[data-testid="tree-node"]');
    await user.click(pagesRow!);
    await user.keyboard('{ArrowRight}');

    // Wait for the rejection to settle
    await waitFor(() => {
      expect(mockGetChildren).toHaveBeenCalledWith('tab-1', 'obj:0:2');
    });

    // Tree should still be intact -- Catalog and children still visible
    expect(screen.getByText('Catalog')).toBeInTheDocument();
    expect(screen.getByText('Pages')).toBeInTheDocument();
    expect(screen.getByText('Type')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Collapsing and re-expanding does not re-fetch
// ---------------------------------------------------------------------------

describe('Re-expand does not re-fetch', () => {
  test('collapsing and re-expanding a loaded node does not call GetChildren again', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    mockGetChildren.mockResolvedValueOnce(pagesChildren);

    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Expand Pages (first time -- triggers GetChildren)
    const pagesRow = screen.getByText('Pages').closest('[data-testid="tree-node"]');
    await user.click(pagesRow!);
    await user.keyboard('{ArrowRight}');

    await waitFor(() => {
      expect(screen.getByText('Page 1')).toBeInTheDocument();
    });

    expect(mockGetChildren).toHaveBeenCalledTimes(1);

    // Collapse Pages
    await user.keyboard('{ArrowLeft}');

    await waitFor(() => {
      expect(screen.queryByText('Page 1')).not.toBeInTheDocument();
    });

    // Re-expand Pages (second time -- should NOT re-fetch)
    await user.keyboard('{ArrowRight}');

    await waitFor(() => {
      expect(screen.getByText('Page 1')).toBeInTheDocument();
    });

    // GetChildren should still have been called only once
    expect(mockGetChildren).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// TreePanel handles null rootChildren gracefully
// ---------------------------------------------------------------------------

describe('TreePanel with null rootChildren', () => {
  test('renders root node when rootChildren is null', async () => {
    const openWithNullChildren: AppAction = {
      type: 'OPEN_DOCUMENT',
      payload: {
        tabId: 'tab-null',
        fileName: 'empty.pdf',
        filePath: '/path/to/empty.pdf',
        rootNode: catalogNode,
        rootChildren: null,
      },
    };

    render(
      <AppProvider>
        <DispatchAndRender action={openWithNullChildren}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Root node should be visible
    expect(screen.getByText('Catalog')).toBeInTheDocument();
    // No children should be rendered since rootChildren is null
    expect(screen.queryByText('Type')).not.toBeInTheDocument();
    expect(screen.queryByText('Pages')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Multi-Document State Isolation
//
// Each tab maintains independent tree expansion state after switching
// away and back.
// Each TabState is independent. Tree expansion is preserved per tab.
//
// Given tab-1 is open with the root expanded (children visible),
// And a child node "Pages" is expanded (grandchildren visible),
// When the user switches to tab-2 and then back to tab-1,
// Then tab-1's tree still shows the expanded Pages children.
// ---------------------------------------------------------------------------

describe('Tree expansion state preserved across tab switches', () => {
  test('expanded children survive tab switch round-trip', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    // GetChildren for Pages node when expanded
    mockGetChildren.mockResolvedValueOnce(pagesChildren);

    const catalogNodeB = {
      id: 'root',
      label: 'Catalog B',
      rawKey: '',
      nodeType: 'dict',
      valueType: '',
      hasChildren: true,
      childCount: 1,
      iconHint: 'catalog',
      error: '',
    };

    const childNodesB = [
      {
        id: 'dict:root:Type',
        label: 'TypeB',
        rawKey: '/Type',
        nodeType: 'scalar',
        valueType: 'name',
        hasChildren: false,
        childCount: 0,
        iconHint: 'default',
        error: '',
      },
    ];

    // Open two tabs, switch between them, verify tree state
    function MultiTabTree() {
      const dispatch = useAppDispatch();
      const state = useAppState();
      return (
        <div>
          <button
            data-testid="open-tab1"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: {
                  tabId: 'tab-1',
                  fileName: 'a.pdf',
                  filePath: '/a.pdf',
                  rootNode: catalogNode,
                  rootChildren: childNodes,
                },
              })
            }
          />
          <button
            data-testid="open-tab2"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: {
                  tabId: 'tab-2',
                  fileName: 'b.pdf',
                  filePath: '/b.pdf',
                  rootNode: catalogNodeB,
                  rootChildren: childNodesB,
                },
              })
            }
          />
          <button
            data-testid="activate-tab1"
            onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } })}
          />
          <button
            data-testid="activate-tab2"
            onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } })}
          />
          <span data-testid="active-tab">{state.activeTabId ?? 'null'}</span>
          <TreePanel />
        </div>
      );
    }

    render(
      <AppProvider>
        <MultiTabTree />
      </AppProvider>
    );

    // Step 1: Open tab-1 and expand the Pages node
    act(() => screen.getByTestId('open-tab1').click());

    await waitFor(() => {
      expect(screen.getByText('Catalog')).toBeInTheDocument();
      expect(screen.getByText('Pages')).toBeInTheDocument();
    });

    // Expand Pages node to load its children
    const pagesRow = screen.getByText('Pages').closest('[data-testid="tree-node"]');
    await user.click(pagesRow!);
    await user.keyboard('{ArrowRight}');

    await waitFor(() => {
      expect(screen.getByText('Page 1')).toBeInTheDocument();
      expect(screen.getByText('Page 2')).toBeInTheDocument();
    });

    // Step 2: Open tab-2 (switches away from tab-1)
    act(() => screen.getByTestId('open-tab2').click());

    await waitFor(() => {
      expect(screen.getByTestId('active-tab').textContent).toBe('tab-2');
      expect(screen.getByText('Catalog B')).toBeInTheDocument();
    });

    // Page 1 / Page 2 from tab-1 should NOT be visible
    expect(screen.queryByText('Page 1')).not.toBeInTheDocument();
    expect(screen.queryByText('Page 2')).not.toBeInTheDocument();

    // Step 3: Switch back to tab-1
    act(() => screen.getByTestId('activate-tab1').click());

    await waitFor(() => {
      expect(screen.getByTestId('active-tab').textContent).toBe('tab-1');
    });

    // The expanded Pages children must be preserved (this is the key assertion).
    // Without tree data caching per tab, the tree remounts from rootNode/rootChildren
    // and Pages would appear collapsed again.
    await waitFor(() => {
      expect(screen.getByText('Page 1')).toBeInTheDocument();
      expect(screen.getByText('Page 2')).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// Story 4.3 supplemental: Tree cache cleanup on tab close
//
// Verifies that the treeDataCache cleanup useEffect (TreePanel.tsx lines
// 235-246) evicts entries for closed tabs. Since the cache is a useRef
// (not directly accessible from tests), we verify indirectly: open two tabs,
// expand a node in tab-1, close tab-1, open a NEW tab with a different
// tabId+filePath, and verify the new tab's tree starts fully collapsed
// (only root expanded) rather than inheriting tab-1's expanded state.
// ---------------------------------------------------------------------------

describe('4.3 supplemental: Tree cache cleanup on tab close', () => {
  test('closed tab cache is evicted; new tab starts with fresh tree state', async () => {
    const TreePanel = await importTreePanel();
    const user = userEvent.setup();

    // First GetChildren call for expanding Pages in tab-1
    mockGetChildren.mockResolvedValueOnce(pagesChildren);

    const catalogNodeC = {
      id: 'root',
      label: 'Catalog C',
      rawKey: '',
      nodeType: 'dict',
      valueType: '',
      hasChildren: true,
      childCount: 1,
      iconHint: 'catalog',
      error: '',
    };

    const childNodesC = [
      {
        id: 'dict:root:Type',
        label: 'TypeC',
        rawKey: '/Type',
        nodeType: 'scalar',
        valueType: 'name',
        hasChildren: false,
        childCount: 0,
        iconHint: 'default',
        error: '',
      },
    ];

    function MultiTabTree() {
      const dispatch = useAppDispatch();
      const state = useAppState();
      return (
        <div>
          <button
            data-testid="open-tab1"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: catalogNode, rootChildren: childNodes },
              })
            }
          />
          <button
            data-testid="open-tab2"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: catalogNodeC, rootChildren: childNodesC },
              })
            }
          />
          <button
            data-testid="close-tab1"
            onClick={() =>
              dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } })
            }
          />
          <button
            data-testid="open-tab3"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: { tabId: 'tab-3', fileName: 'c.pdf', filePath: '/c.pdf', rootNode: catalogNode, rootChildren: childNodes },
              })
            }
          />
          <button
            data-testid="activate-tab3"
            onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-3' } })}
          />
          <span data-testid="active-tab">{state.activeTabId ?? 'null'}</span>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <TreePanel />
        </div>
      );
    }

    render(
      <AppProvider>
        <MultiTabTree />
      </AppProvider>
    );

    // Step 1: Open tab-1 and expand Pages
    act(() => screen.getByTestId('open-tab1').click());

    await waitFor(() => {
      expect(screen.getByText('Pages')).toBeInTheDocument();
    });

    const pagesRow = screen.getByText('Pages').closest('[data-testid="tree-node"]');
    await user.click(pagesRow!);
    await user.keyboard('{ArrowRight}');

    await waitFor(() => {
      expect(screen.getByText('Page 1')).toBeInTheDocument();
      expect(screen.getByText('Page 2')).toBeInTheDocument();
    });

    // Step 2: Open tab-2 (switches away from tab-1)
    act(() => screen.getByTestId('open-tab2').click());

    await waitFor(() => {
      expect(screen.getByTestId('active-tab').textContent).toBe('tab-2');
    });

    // Step 3: Close tab-1 (triggers cache eviction)
    act(() => screen.getByTestId('close-tab1').click());

    expect(screen.getByTestId('tab-count').textContent).toBe('1');

    // Step 4: Open a NEW tab with the same rootNode/rootChildren as tab-1
    // but a DIFFERENT tabId and filePath (to avoid dedup)
    act(() => screen.getByTestId('open-tab3').click());

    await waitFor(() => {
      expect(screen.getByTestId('active-tab').textContent).toBe('tab-3');
    });

    // Step 5: The new tab (tab-3) should show only root-level children,
    // NOT the expanded Pages grandchildren. If the cache was not cleaned up,
    // tab-3 might inherit tab-1's expanded state.
    await waitFor(() => {
      expect(screen.getByText('Pages')).toBeInTheDocument();
    });

    // Page 1 and Page 2 should NOT be visible (they were expanded in tab-1,
    // which is now closed and its cache evicted)
    expect(screen.queryByText('Page 1')).not.toBeInTheDocument();
    expect(screen.queryByText('Page 2')).not.toBeInTheDocument();
  });
});
