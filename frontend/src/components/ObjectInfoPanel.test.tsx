/**
 * Story 2.6: Object Info Panel -- Property Display for Selected Nodes
 *
 * TDD RED PHASE: Tests MUST fail until ObjectInfoPanel.tsx is implemented.
 *
 * Test IDs: 2.6-UNIT-002 through 2.6-UNIT-005 (Vitest)
 * Run: cd frontend && npx vitest run src/components/ObjectInfoPanel.test.tsx
 */
import { render, screen, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  type AppAction,
} from '../hooks/useDocumentState';
// RED PHASE: This import will fail until ObjectInfoPanel.tsx is created.
import { ObjectInfoPanel } from './ObjectInfoPanel';

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

const emptyDictDetail = {
  nodeId: 'obj:0:20',
  objectRef: '20 0 R',
  type: 'dict',
  properties: [],
  elements: [],
  scalarValue: null,
  streamInfo: null,
};

const emptyArrayDetail = {
  nodeId: 'obj:0:21',
  objectRef: '21 0 R',
  type: 'array',
  properties: [],
  elements: [],
  scalarValue: null,
  streamInfo: null,
};

// Helper to dispatch state so ObjectInfoPanel has a selected node
function DispatchHelper({
  action,
}: {
  action: AppAction;
}) {
  const dispatch = useAppDispatch();
  // Dispatch on mount
  dispatch(action);
  return null;
}

function renderWithState(selectedNodeId: string | null) {
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
            payload: { nodeId: selectedNodeId },
          }}
        />
      )}
      <ObjectInfoPanel />
    </AppProvider>
  );

  return render(content);
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-002 [P1]: ObjectInfoPanel renders key-value table for dictionary
// with type-colored values
// AC#2: Dictionary view with PropertyEntry items and ValueEntry type coloring.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-002: ObjectInfoPanel dictionary rendering', () => {
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

  test('applies type-name color class for Name values', async () => {
    renderWithState('root');

    await waitFor(() => {
      const nameValue = screen.getByText('/Catalog');
      expect(nameValue.className).toMatch(/text-type-name/);
    });
  });

  test('applies type-reference color class for reference values', async () => {
    renderWithState('root');

    await waitFor(() => {
      const refValue = screen.getByText('2 0 R');
      expect(refValue.className).toMatch(/text-type-reference/);
    });
  });

  test('reference values are underlined', async () => {
    renderWithState('root');

    await waitFor(() => {
      const refValue = screen.getByText('2 0 R');
      expect(refValue.className).toMatch(/underline/);
    });
  });

  test('reference values have data-ref-target attribute', async () => {
    renderWithState('root');

    await waitFor(() => {
      const refValue = screen.getByText('2 0 R');
      expect(refValue).toHaveAttribute('data-ref-target', 'obj:0:2');
    });
  });

  test('value text uses font-mono text-xs', async () => {
    renderWithState('root');

    await waitFor(() => {
      const nameValue = screen.getByText('/Catalog');
      expect(nameValue.className).toMatch(/font-mono/);
      expect(nameValue.className).toMatch(/text-xs/);
    });
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-003 [P1]: ObjectInfoPanel shows empty state when no node selected
// AC#1: Given no tree node is selected, When the user views the
//       ObjectInfoPanel, Then it shows "Select a node to view properties"
//       in muted text.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-003: ObjectInfoPanel empty state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('shows empty state text when no node is selected', () => {
    renderWithState(null);

    expect(
      screen.getByText('Select a node to view properties')
    ).toBeInTheDocument();
  });

  test('empty state has correct data-testid', () => {
    renderWithState(null);

    expect(screen.getByTestId('object-info-empty')).toBeInTheDocument();
  });

  test('empty state text uses muted styling', () => {
    renderWithState(null);

    const emptyEl = screen.getByTestId('object-info-empty');
    expect(emptyEl.className).toMatch(/text-text-muted/);
    expect(emptyEl.className).toMatch(/text-sm/);
  });

  test('panel container has data-testid', () => {
    renderWithState(null);

    expect(screen.getByTestId('object-info-panel')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-004 [P2]: ObjectInfoPanel shows "Empty dictionary" / "Empty array"
// AC#5: Given an empty dictionary or array is selected, When the
//       ObjectInfoPanel updates, Then it shows "Empty dictionary" or
//       "Empty array" in muted text.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-004: ObjectInfoPanel empty dict/array', () => {
  test('shows "Empty dictionary" for empty dict', async () => {
    mockGetObjectDetail.mockResolvedValue(emptyDictDetail);
    renderWithState('obj:0:20');

    await waitFor(() => {
      expect(screen.getByText('Empty dictionary')).toBeInTheDocument();
    });
  });

  test('"Empty dictionary" text has muted styling', async () => {
    mockGetObjectDetail.mockResolvedValue(emptyDictDetail);
    renderWithState('obj:0:20');

    await waitFor(() => {
      const el = screen.getByText('Empty dictionary');
      expect(el.className).toMatch(/text-text-muted/);
      expect(el.className).toMatch(/text-sm/);
    });
  });

  test('shows "Empty array" for empty array', async () => {
    mockGetObjectDetail.mockResolvedValue(emptyArrayDetail);
    renderWithState('obj:0:21');

    await waitFor(() => {
      expect(screen.getByText('Empty array')).toBeInTheDocument();
    });
  });

  test('"Empty array" text has muted styling', async () => {
    mockGetObjectDetail.mockResolvedValue(emptyArrayDetail);
    renderWithState('obj:0:21');

    await waitFor(() => {
      const el = screen.getByText('Empty array');
      expect(el.className).toMatch(/text-text-muted/);
      expect(el.className).toMatch(/text-sm/);
    });
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-005 [P2]: ObjectInfoPanel scalar view
// AC#4: Given a scalar node is selected, When the ObjectInfoPanel updates,
//       Then it displays the single value with its type label.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-005: ObjectInfoPanel scalar rendering', () => {
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
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-006 [P1]: ObjectInfoPanel array rendering
// AC#3: Given an array node is selected, When the ObjectInfoPanel updates,
//       Then it displays an indexed list of array elements.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-006: ObjectInfoPanel array rendering', () => {
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
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-007 [P1]: ObjectInfoPanel stream rendering
// AC#6: Given a stream node is selected, When the ObjectInfoPanel updates,
//       Then it displays the stream's dictionary properties as a key-value
//       table, And displays stream metadata (length and filter names).
// ---------------------------------------------------------------------------

describe('2.6-UNIT-007: ObjectInfoPanel stream rendering', () => {
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

  test('renders stream filter names as comma-separated list', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const metadata = screen.getByTestId('stream-metadata');
      expect(metadata).toHaveTextContent('Filters: FlateDecode');
    });
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-008 [P1]: ObjectInfoPanel header shows object reference
// AC#2: The object reference (e.g., "4 0 R") is displayed in the panel header.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-008: ObjectInfoPanel header', () => {
  test('shows "Object Properties" header label', async () => {
    mockGetObjectDetail.mockResolvedValue(dictDetail);
    renderWithState('root');

    await waitFor(() => {
      expect(screen.getByText('Object Properties')).toBeInTheDocument();
    });
  });

  test('shows object reference for indirect objects', async () => {
    mockGetObjectDetail.mockResolvedValue(arrayDetail);
    renderWithState('obj:0:5');

    await waitFor(() => {
      expect(screen.getByText('5 0 R')).toBeInTheDocument();
    });
  });

  test('does not show object reference for non-indirect objects', async () => {
    mockGetObjectDetail.mockResolvedValue(dictDetail);
    renderWithState('root');

    await waitFor(() => {
      expect(screen.getByText('Object Properties')).toBeInTheDocument();
    });
    // dictDetail.objectRef is empty -- no ref text should appear in header
    // (the "2 0 R" in the dict properties is a property value, not a header ref)
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-009 [P1]: ObjectInfoPanel error handling
// AC#2 (error): If GetObjectDetail call rejects, show error inline.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-009: ObjectInfoPanel error handling', () => {
  test('shows error message when GetObjectDetail rejects', async () => {
    mockGetObjectDetail.mockRejectedValue(
      new Error('document not found: tab "tab-1"')
    );
    renderWithState('root');

    await waitFor(() => {
      expect(screen.getByTestId('object-info-error')).toBeInTheDocument();
    });
  });

  test('error display uses error text styling', async () => {
    mockGetObjectDetail.mockRejectedValue(new Error('some error'));
    renderWithState('root');

    await waitFor(() => {
      const errorEl = screen.getByTestId('object-info-error');
      expect(errorEl.className).toMatch(/text-error/);
      expect(errorEl.className).toMatch(/text-sm/);
    });
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-010 [P1]: ObjectInfoPanel calls GetObjectDetail with correct args
// AC#2: When selectedNodeId changes, call GetObjectDetail(activeTabId,
//       selectedNodeId).
// ---------------------------------------------------------------------------

describe('2.6-UNIT-010: ObjectInfoPanel data fetching', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(dictDetail);
  });

  test('calls GetObjectDetail with activeTabId and selectedNodeId', async () => {
    renderWithState('root');

    await waitFor(() => {
      expect(mockGetObjectDetail).toHaveBeenCalledWith('tab-1', 'root');
    });
  });

  test('does not call GetObjectDetail when no node is selected', () => {
    renderWithState(null);

    expect(mockGetObjectDetail).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-011 [P1]: ValueDisplay type color mapping completeness
// AC#2: All PDF types have correct Tailwind color classes.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-011: ValueDisplay type color mapping', () => {
  test('boolean values get text-type-boolean class', async () => {
    const boolDetail = {
      ...scalarDetail,
      scalarValue: {
        type: 'boolean',
        display: 'true',
        raw: 'true',
        refTarget: '',
      },
    };
    mockGetObjectDetail.mockResolvedValue(boolDetail);
    renderWithState('dict:root:Type');

    await waitFor(() => {
      const el = screen.getByText('true');
      expect(el.className).toMatch(/text-type-boolean/);
    });
  });

  test('string values get text-type-string class', async () => {
    const stringDetail = {
      ...scalarDetail,
      scalarValue: {
        type: 'string',
        display: '(Hello)',
        raw: '(Hello)',
        refTarget: '',
      },
    };
    mockGetObjectDetail.mockResolvedValue(stringDetail);
    renderWithState('dict:root:Type');

    await waitFor(() => {
      const el = screen.getByText('(Hello)');
      expect(el.className).toMatch(/text-type-string/);
    });
  });

  test('number values get text-type-number class', async () => {
    const numberDetail = {
      ...scalarDetail,
      scalarValue: {
        type: 'number',
        display: '42',
        raw: '42',
        refTarget: '',
      },
    };
    mockGetObjectDetail.mockResolvedValue(numberDetail);
    renderWithState('dict:root:Type');

    await waitFor(() => {
      const el = screen.getByText('42');
      expect(el.className).toMatch(/text-type-number/);
    });
  });

  test('null values get text-type-null class', async () => {
    const nullDetail = {
      ...scalarDetail,
      scalarValue: {
        type: 'null',
        display: 'null',
        raw: 'null',
        refTarget: '',
      },
    };
    mockGetObjectDetail.mockResolvedValue(nullDetail);
    renderWithState('dict:root:Type');

    await waitFor(() => {
      const el = screen.getByText('null');
      expect(el.className).toMatch(/text-type-null/);
    });
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-EXTRA-001 [P1]: Multi-filter comma-separated display
// AC#6: Stream metadata shows filter names as comma-separated list.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-EXTRA-001: Stream with multiple filters', () => {
  test('renders multiple filters as comma-separated list', async () => {
    const multiFilterStream = {
      ...streamDetail,
      streamInfo: {
        length: 2000,
        filters: ['FlateDecode', 'ASCII85Decode'],
      },
    };
    mockGetObjectDetail.mockResolvedValue(multiFilterStream);
    renderWithState('obj:0:10');

    await waitFor(() => {
      const metadata = screen.getByTestId('stream-metadata');
      expect(metadata).toHaveTextContent('Filters: FlateDecode, ASCII85Decode');
    });
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-EXTRA-002 [P1]: Scalar view shows type label
// AC#4: Displays the single value with its type label.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-EXTRA-002: Scalar view type label', () => {
  test('renders type label above scalar value', async () => {
    mockGetObjectDetail.mockResolvedValue(scalarDetail);
    renderWithState('dict:root:Type');

    await waitFor(() => {
      expect(screen.getByText('Type: name')).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-EXTRA-003 [P2]: Dict key styling
// AC#2: Keys in font-mono text-text-muted text-xs.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-EXTRA-003: Dict key styling', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(dictDetail);
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
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-EXTRA-004 [P2]: Reference click is no-op (no navigation yet)
// AC#2: References clickable but no-op until Story 2-8.
// ---------------------------------------------------------------------------

describe('2.6-UNIT-EXTRA-004: Reference click no-op', () => {
  test('clicking a reference value does not throw', async () => {
    mockGetObjectDetail.mockResolvedValue(dictDetail);
    renderWithState('root');

    await waitFor(() => {
      const refValue = screen.getByText('2 0 R');
      expect(() => refValue.click()).not.toThrow();
    });
  });
});

// ---------------------------------------------------------------------------
// 2.6-UNIT-012 [P2]: Stream with no filters shows "None"
// AC#6: Filter names from detail.streamInfo. If empty, show "None".
// ---------------------------------------------------------------------------

describe('2.6-UNIT-012: Stream with no filters', () => {
  test('shows "None" when stream has no filters', async () => {
    const noFilterStream = {
      ...streamDetail,
      streamInfo: {
        length: 500,
        filters: [],
      },
    };
    mockGetObjectDetail.mockResolvedValue(noFilterStream);
    renderWithState('obj:0:10');

    await waitFor(() => {
      const metadata = screen.getByTestId('stream-metadata');
      expect(metadata).toHaveTextContent('Filters: None');
    });
  });
});
