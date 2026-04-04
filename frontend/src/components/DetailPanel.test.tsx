/**
 * Story 2.7: Detail Panel -- Context-Sensitive Content Display
 *
 * TDD RED PHASE: Tests MUST fail until DetailPanel.tsx is implemented.
 *
 * Test IDs: 2.7-UNIT-001 through 2.7-UNIT-005 (Vitest)
 * Run: cd frontend && npx vitest run src/components/DetailPanel.test.tsx
 */
import { render, screen, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  type AppAction,
} from '../hooks/useDocumentState';
// RED PHASE: This import will fail until DetailPanel.tsx is created.
import { DetailPanel } from './DetailPanel';

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
const mockGetObjectDetail = vi.fn();
vi.mock(
  '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: (...args: unknown[]) => mockGetObjectDetail(...args),
  })
);

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

const rootChildren = [
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
];

// ObjectDetail fixtures matching model.go types
const dictDetail = {
  nodeId: 'root',
  objectRef: '',
  type: 'dict',
  properties: [
    {
      key: '/Pages',
      value: {
        type: 'reference',
        display: '2 0 R',
        raw: '2 0 R',
        refTarget: 'obj:0:2',
      },
    },
    {
      key: '/Type',
      value: {
        type: 'name',
        display: '/Catalog',
        raw: '/Catalog',
        refTarget: '',
      },
    },
  ],
  elements: [],
  scalarValue: null,
  streamInfo: null,
};

const arrayDetail = {
  nodeId: 'obj:0:5',
  objectRef: '5 0 R',
  type: 'array',
  properties: [],
  elements: [
    {
      type: 'reference',
      display: '3 0 R',
      raw: '3 0 R',
      refTarget: 'obj:0:3',
    },
    {
      type: 'reference',
      display: '4 0 R',
      raw: '4 0 R',
      refTarget: 'obj:0:4',
    },
  ],
  scalarValue: null,
  streamInfo: null,
};

const scalarDetail = {
  nodeId: 'dict:root:Type',
  objectRef: '',
  type: 'scalar',
  properties: [],
  elements: [],
  scalarValue: {
    type: 'name',
    display: '/Catalog',
    raw: '/Catalog',
    refTarget: '',
  },
  streamInfo: null,
};

const streamDetail = {
  nodeId: 'obj:0:10',
  objectRef: '10 0 R',
  type: 'stream',
  properties: [
    {
      key: '/Filter',
      value: {
        type: 'name',
        display: '/FlateDecode',
        raw: '/FlateDecode',
        refTarget: '',
      },
    },
    {
      key: '/Length',
      value: {
        type: 'number',
        display: '1234',
        raw: '1234',
        refTarget: '',
      },
    },
  ],
  elements: [],
  scalarValue: null,
  streamInfo: {
    length: 1234,
    filters: ['FlateDecode'],
  },
};

// Helper to dispatch state so DetailPanel has a selected node
function DispatchHelper({
  action,
}: {
  action: AppAction;
}) {
  const dispatch = useAppDispatch();
  dispatch(action);
  return null;
}

function renderWithState(
  selectedNodeId: string | null,
  selectPayload?: { label?: string; rawKey?: string },
) {
  const openAction: AppAction = {
    type: 'OPEN_DOCUMENT',
    payload: {
      tabId: 'tab-1',
      fileName: 'test.pdf',
      rootNode: catalogNode,
      rootChildren: rootChildren,
    },
  };

  const content = (
    <AppProvider>
      <DispatchHelper action={openAction} />
      {selectedNodeId && (
        <DispatchHelper
          action={{
            type: 'SELECT_NODE',
            payload: { nodeId: selectedNodeId, ...selectPayload },
          }}
        />
      )}
      <DetailPanel />
    </AppProvider>
  );

  return render(content);
}

// ---------------------------------------------------------------------------
// 2.7-UNIT-001 [P1]: DetailPanel renders PropertyTable for dictionary node
// AC#2: Given a dictionary node is selected, When the DetailPanel updates,
//       Then it displays a full PropertyTable with all key-value pairs and
//       type-colored values, And the panel header shows a context label.
// ---------------------------------------------------------------------------

describe('2.7-UNIT-001: DetailPanel dictionary rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(dictDetail);
  });

  test('renders dictionary properties as key-value pairs', async () => {
    renderWithState('root');

    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
      expect(screen.getByText('/Catalog')).toBeInTheDocument();
    });

    await waitFor(() => {
      expect(screen.getByText('/Pages')).toBeInTheDocument();
      expect(screen.getByText('2 0 R')).toBeInTheDocument();
    });
  });

  test('applies type-colored classes for values', async () => {
    renderWithState('root');

    await waitFor(() => {
      const nameValue = screen.getByText('/Catalog');
      expect(nameValue.className).toMatch(/text-type-name/);
    });

    await waitFor(() => {
      const refValue = screen.getByText('2 0 R');
      expect(refValue.className).toMatch(/text-type-reference/);
    });
  });

  test('header shows context label "Properties"', async () => {
    renderWithState('root');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Properties');
    });
  });

  test('calls GetObjectDetail with correct arguments', async () => {
    renderWithState('root');

    await waitFor(() => {
      expect(mockGetObjectDetail).toHaveBeenCalledWith('tab-1', 'root');
    });
  });

  test('dict keys use font-mono text-text-muted text-xs', async () => {
    renderWithState('root');

    await waitFor(() => {
      const keyEl = screen.getByText('/Type');
      expect(keyEl.className).toMatch(/font-mono/);
      expect(keyEl.className).toMatch(/text-text-muted/);
      expect(keyEl.className).toMatch(/text-xs/);
    });
  });

  test('header shows context suffix from rawKey', async () => {
    renderWithState('root', { label: 'Font', rawKey: '/Font' });

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Properties - /Font');
    });
  });

  test('header shows context suffix from label when rawKey is absent', async () => {
    renderWithState('root', { label: 'Catalog' });

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Properties - Catalog');
    });
  });
});

// ---------------------------------------------------------------------------
// 2.7-UNIT-002 [P1]: DetailPanel renders ArrayViewer for array node
// AC#3: Given an array node is selected, When the DetailPanel updates,
//       Then it displays an ArrayViewer with ordered elements and indices.
// ---------------------------------------------------------------------------

describe('2.7-UNIT-002: DetailPanel array rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(arrayDetail);
  });

  test('renders array elements with index labels', async () => {
    renderWithState('obj:0:5');

    await waitFor(() => {
      expect(screen.getByText('[0]')).toBeInTheDocument();
      expect(screen.getByText('[1]')).toBeInTheDocument();
    });
  });

  test('renders array element values', async () => {
    renderWithState('obj:0:5');

    await waitFor(() => {
      expect(screen.getByText('3 0 R')).toBeInTheDocument();
      expect(screen.getByText('4 0 R')).toBeInTheDocument();
    });
  });

  test('array element values have correct type coloring', async () => {
    renderWithState('obj:0:5');

    await waitFor(() => {
      const refValue = screen.getByText('3 0 R');
      expect(refValue.className).toMatch(/text-type-reference/);
    });
  });

  test('header shows context label "Array"', async () => {
    renderWithState('obj:0:5');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Array');
    });
  });

  test('header shows object reference for indirect objects', async () => {
    renderWithState('obj:0:5');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('5 0 R');
    });
  });
});

// ---------------------------------------------------------------------------
// 2.7-UNIT-003 [P1]: DetailPanel shows placeholder when no node selected
// AC#1: Given no tree node is selected, When the user views the DetailPanel,
//       Then it shows "Select a node in the tree to view details" in muted
//       text, centered vertically and horizontally.
// ---------------------------------------------------------------------------

describe('2.7-UNIT-003: DetailPanel empty state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('shows empty state text when no node is selected', () => {
    renderWithState(null);

    expect(
      screen.getByText('Select a node in the tree to view details')
    ).toBeInTheDocument();
  });

  test('empty state has correct data-testid', () => {
    renderWithState(null);

    expect(screen.getByTestId('detail-panel-empty')).toBeInTheDocument();
  });

  test('empty state text uses muted styling', () => {
    renderWithState(null);

    const emptyEl = screen.getByTestId('detail-panel-empty');
    expect(emptyEl.className).toMatch(/text-text-muted/);
    expect(emptyEl.className).toMatch(/text-sm/);
  });

  test('panel container has data-testid', () => {
    renderWithState(null);

    expect(screen.getByTestId('detail-panel')).toBeInTheDocument();
  });

  test('does not call GetObjectDetail when no node is selected', () => {
    renderWithState(null);

    expect(mockGetObjectDetail).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 2.7-UNIT-004 [P2]: DetailPanel error display
// AC#6: Given an error node is selected OR GetObjectDetail returns an error,
//       When the DetailPanel updates, Then it displays the error message in
//       text-error styling.
// ---------------------------------------------------------------------------

describe('2.7-UNIT-004: DetailPanel error display', () => {
  test('shows error message when GetObjectDetail rejects', async () => {
    mockGetObjectDetail.mockRejectedValue(
      new Error('document not found: tab "tab-1"')
    );
    renderWithState('root');

    await waitFor(() => {
      expect(screen.getByTestId('detail-panel-error')).toBeInTheDocument();
    });
  });

  test('error display uses text-error styling', async () => {
    mockGetObjectDetail.mockRejectedValue(new Error('some error'));
    renderWithState('root');

    await waitFor(() => {
      const errorEl = screen.getByTestId('detail-panel-error');
      expect(errorEl.className).toMatch(/text-error/);
    });
  });

  test('error message text is visible', async () => {
    mockGetObjectDetail.mockRejectedValue(new Error('node not found'));
    renderWithState('root');

    await waitFor(() => {
      const errorEl = screen.getByTestId('detail-panel-error');
      expect(errorEl).toHaveTextContent('node not found');
    });
  });
});

// ---------------------------------------------------------------------------
// 2.7-UNIT-005 [P2]: DetailPanel aria-live for screen reader announcements
// AC#7: Given the detail panel content is updated, When a screen reader is
//       active, Then the content change is announced via aria-live="polite"
//       on the detail panel container.
// ---------------------------------------------------------------------------

describe('2.7-UNIT-005: DetailPanel accessibility', () => {
  test('content container has aria-live="polite"', () => {
    renderWithState(null);

    const contentEl = screen.getByTestId('detail-panel-content');
    expect(contentEl).toHaveAttribute('aria-live', 'polite');
  });
});

// ---------------------------------------------------------------------------
// 2.7-UNIT-006 [P1]: DetailPanel scalar rendering
// AC#4: Given a scalar/leaf node is selected, When the DetailPanel updates,
//       Then it displays a ScalarViewer showing the value with its type
//       indication.
// ---------------------------------------------------------------------------

describe('2.7-UNIT-006: DetailPanel scalar rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(scalarDetail);
  });

  test('renders scalar value display text', async () => {
    renderWithState('dict:root:Type');

    await waitFor(() => {
      expect(screen.getByText('/Catalog')).toBeInTheDocument();
    });
  });

  test('scalar value has correct type color class', async () => {
    renderWithState('dict:root:Type');

    await waitFor(() => {
      const nameValue = screen.getByText('/Catalog');
      expect(nameValue.className).toMatch(/text-type-name/);
    });
  });

  test('scalar view shows type label', async () => {
    renderWithState('dict:root:Type');

    await waitFor(() => {
      expect(screen.getByText('Type: name')).toBeInTheDocument();
    });
  });

  test('header shows context label "Value"', async () => {
    renderWithState('dict:root:Type');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Value');
    });
  });
});

// ---------------------------------------------------------------------------
// 2.7-UNIT-007 [P1]: DetailPanel stream rendering
// AC#5: Given a stream node is selected, When the DetailPanel updates,
//       Then it displays the stream dictionary properties as a PropertyTable,
//       And stream metadata (length, filters) below the properties.
// ---------------------------------------------------------------------------

describe('2.7-UNIT-007: DetailPanel stream rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(streamDetail);
  });

  test('renders stream dictionary properties', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      expect(screen.getByText('/Filter')).toBeInTheDocument();
      expect(screen.getByText('/FlateDecode')).toBeInTheDocument();
    });
  });

  test('renders stream metadata length', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const metadata = screen.getByTestId('stream-metadata');
      expect(metadata).toHaveTextContent('Length: 1234 bytes');
    });
  });

  test('renders stream filter names', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const metadata = screen.getByTestId('stream-metadata');
      expect(metadata).toHaveTextContent('Filters: FlateDecode');
    });
  });

  test('header shows context label "Stream"', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Stream');
    });
  });

  test('header shows object reference for stream indirect objects', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('10 0 R');
    });
  });
});

// ---------------------------------------------------------------------------
// 2.7-UNIT-008 [P1]: DetailPanel is exported via React.memo
// AC: architecture requirement -- wrapped in React.memo to prevent re-renders
//     when switching tabs.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.7-UNIT-009 [P1]: DetailPanel cancels stale fetch on rapid node change
// AC#2-#5: When selectedNodeId changes, previous in-flight fetch results
//          must not overwrite the new result.
// ---------------------------------------------------------------------------

describe('2.7-UNIT-009: DetailPanel stale fetch cancellation', () => {
  test('does not render stale data when node changes rapidly', async () => {
    // First call resolves slowly with dict, second resolves with array
    let resolveFirst: (v: unknown) => void;
    const firstPromise = new Promise((resolve) => { resolveFirst = resolve; });
    const secondPromise = Promise.resolve(arrayDetail);

    mockGetObjectDetail
      .mockReturnValueOnce(firstPromise)
      .mockReturnValueOnce(secondPromise);

    const { rerender } = renderWithState('root');

    // Re-render with a new node before the first resolves
    rerender(
      <AppProvider>
        <DispatchHelper action={{
          type: 'OPEN_DOCUMENT',
          payload: { tabId: 'tab-1', fileName: 'test.pdf', rootNode: catalogNode, rootChildren: rootChildren },
        }} />
        <DispatchHelper action={{
          type: 'SELECT_NODE',
          payload: { nodeId: 'obj:0:5' },
        }} />
        <DetailPanel />
      </AppProvider>
    );

    // Now resolve the first (stale) promise
    resolveFirst!(dictDetail);

    // The array detail should render, not the dict
    await waitFor(() => {
      expect(screen.getByText('[0]')).toBeInTheDocument();
      expect(screen.getByText('3 0 R')).toBeInTheDocument();
    });

    // Dict keys from the stale response should NOT be present
    expect(screen.queryByText('/Pages')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 2.7-UNIT-010 [P1]: DetailPanel clears previous detail on node change
// AC#2-#5: Selecting a new node clears previous detail before new data loads.
// ---------------------------------------------------------------------------

describe('2.7-UNIT-010: DetailPanel clears on node change', () => {
  test('previous detail is cleared while new node loads', async () => {
    // First call resolves immediately
    mockGetObjectDetail.mockResolvedValueOnce(dictDetail);
    const { rerender } = renderWithState('root');

    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });

    // Second call hangs
    mockGetObjectDetail.mockReturnValueOnce(new Promise(() => {}));

    rerender(
      <AppProvider>
        <DispatchHelper action={{
          type: 'OPEN_DOCUMENT',
          payload: { tabId: 'tab-1', fileName: 'test.pdf', rootNode: catalogNode, rootChildren: rootChildren },
        }} />
        <DispatchHelper action={{
          type: 'SELECT_NODE',
          payload: { nodeId: 'obj:0:5' },
        }} />
        <DetailPanel />
      </AppProvider>
    );

    // Old detail should be cleared -- no dict keys visible, no error, no empty state
    await waitFor(() => {
      expect(screen.queryByText('/Type')).not.toBeInTheDocument();
      expect(screen.queryByText('/Pages')).not.toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// 2.7-UNIT-011 [P2]: DetailPanel edge cases for empty data
// AC#2-#5: Edge cases for empty properties, elements, filters.
// ---------------------------------------------------------------------------

describe('2.7-UNIT-011: DetailPanel edge cases', () => {
  test('renders "Empty dictionary" for dict with no properties', async () => {
    const emptyDict = { ...dictDetail, properties: [] };
    mockGetObjectDetail.mockResolvedValue(emptyDict);
    renderWithState('root');

    await waitFor(() => {
      expect(screen.getByText('Empty dictionary')).toBeInTheDocument();
    });
  });

  test('renders "Empty array" for array with no elements', async () => {
    const emptyArray = { ...arrayDetail, elements: [] };
    mockGetObjectDetail.mockResolvedValue(emptyArray);
    renderWithState('obj:0:5');

    await waitFor(() => {
      expect(screen.getByText('Empty array')).toBeInTheDocument();
    });
  });

  test('stream metadata shows "None" when filters list is empty', async () => {
    const noFilterStream = {
      ...streamDetail,
      streamInfo: { length: 500, filters: [] },
    };
    mockGetObjectDetail.mockResolvedValue(noFilterStream);
    renderWithState('obj:0:10');

    await waitFor(() => {
      const metadata = screen.getByTestId('stream-metadata');
      expect(metadata).toHaveTextContent('Filters: None');
      expect(metadata).toHaveTextContent('Length: 500 bytes');
    });
  });

  test('stream metadata shows comma-separated filter names for multiple filters', async () => {
    const multiFilterStream = {
      ...streamDetail,
      streamInfo: { length: 2000, filters: ['FlateDecode', 'ASCII85Decode'] },
    };
    mockGetObjectDetail.mockResolvedValue(multiFilterStream);
    renderWithState('obj:0:10');

    await waitFor(() => {
      const metadata = screen.getByTestId('stream-metadata');
      expect(metadata).toHaveTextContent('Filters: FlateDecode, ASCII85Decode');
    });
  });

  test('scalar with null scalarValue shows "No value" fallback', async () => {
    const nullScalar = { ...scalarDetail, scalarValue: null };
    mockGetObjectDetail.mockResolvedValue(nullScalar);
    renderWithState('dict:root:Type');

    await waitFor(() => {
      expect(screen.getByText('No value')).toBeInTheDocument();
    });
  });

  test('unknown detail type falls back to "Details" header label', async () => {
    const unknownType = { ...dictDetail, type: 'xobject', properties: [] };
    mockGetObjectDetail.mockResolvedValue(unknownType);
    renderWithState('root');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Details');
    });
  });

  test('header shows only type label when no label or rawKey provided', async () => {
    mockGetObjectDetail.mockResolvedValue(arrayDetail);
    renderWithState('obj:0:5');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Array');
      // Should NOT contain " - " separator since no context suffix
      expect(header.textContent).not.toMatch(/ - /);
    });
  });
});

describe('2.7-UNIT-008: DetailPanel React.memo export', () => {
  test('DetailPanel is a memoized component', () => {
    // React.memo wraps the component and sets $$typeof and type properties.
    // The display name should contain "DetailPanel".
    // React.memo components have a .type property pointing to the inner component.
    expect(DetailPanel).toBeDefined();
    // React.memo components are objects with a $$typeof Symbol and a .type property
    expect((DetailPanel as unknown as { type: unknown }).type).toBeDefined();
  });
});
