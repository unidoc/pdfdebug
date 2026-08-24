/**
 * Detail Panel -- Context-Sensitive Content Display
 *
 * Test IDs: through (Vitest)
 * Run: cd frontend && npx vitest run src/components/DetailPanel.test.tsx
 */
import { render, screen, waitFor, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  type AppAction,
} from '../hooks/useDocumentState';
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
const mockGetContentStream = vi.fn();
const mockGetImageData = vi.fn();
const mockGetReverseRefs = vi.fn().mockResolvedValue([]);
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: (...args: unknown[]) => mockGetObjectDetail(...args),
    GetContentStream: (...args: unknown[]) => mockGetContentStream(...args),
    GetImageData: (...args: unknown[]) => mockGetImageData(...args),
    GetReverseRefs: (...args: unknown[]) => mockGetReverseRefs(...args),
    GetXRefTable: vi.fn().mockResolvedValue({ tabId: '', entries: [] }),
    // The Embedded + Metadata tab panes forceMount, so DetailPanel
    // calls these on render; stub them so the mock does not throw on the new
    // exports.
    GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
    GetSignatures: vi.fn().mockResolvedValue([]),
    GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
    GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
    SaveBytesToFile: vi.fn().mockResolvedValue(''),
    // The Diff tab imports DiffDocuments; stub so the factory never
    // throws on the new export.
    DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
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
  selectPayload?: { label?: string; rawKey?: string; iconHint?: string },
) {
  const openAction: AppAction = {
    type: 'OPEN_DOCUMENT',
    payload: {
      tabId: 'tab-1',
      fileName: 'test.pdf',
      filePath: '/path/to/test.pdf',
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
// DetailPanel renders PropertyTable for dictionary node: Given a
// dictionary node is selected, When the DetailPanel updates,
//       Then it displays a full PropertyTable with all key-value pairs and
//       type-colored values, And the panel header shows a context label.
// ---------------------------------------------------------------------------

describe('DetailPanel dictionary rendering', () => {
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
// DetailPanel renders ArrayViewer for array node: Given an array node
// is selected, When the DetailPanel updates,
//       Then it displays an ArrayViewer with ordered elements and indices.
// ---------------------------------------------------------------------------

describe('DetailPanel array rendering', () => {
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
// DetailPanel shows placeholder when no node selected: Given no tree node is
// selected, When the user views the DetailPanel,
//       Then it shows "Select a node in the tree to view details" in muted
//       text, centered vertically and horizontally.
// ---------------------------------------------------------------------------

describe('DetailPanel empty state', () => {
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
// DetailPanel error display: Given an error node is selected OR
// GetObjectDetail returns an error,
//       When the DetailPanel updates, Then it displays the error message in
//       text-error styling.
// ---------------------------------------------------------------------------

describe('DetailPanel error display', () => {
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
// DetailPanel aria-live for screen reader announcements: Given the detail
// panel content is updated, When a screen reader is
//       active, Then the content change is announced via aria-live="polite"
//       on the detail panel container.
// ---------------------------------------------------------------------------

describe('DetailPanel accessibility', () => {
  test('content container has aria-live="polite"', () => {
    renderWithState(null);

    const contentEl = screen.getByTestId('detail-panel-content');
    expect(contentEl).toHaveAttribute('aria-live', 'polite');
  });
});

// ---------------------------------------------------------------------------
// DetailPanel scalar rendering: Given a scalar/leaf node is selected, When
// the DetailPanel updates,
//       Then it displays a ScalarViewer showing the value with its type
//       indication.
// ---------------------------------------------------------------------------

describe('DetailPanel scalar rendering', () => {
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
// DetailPanel stream rendering: Given a stream node is selected, When
// the DetailPanel updates,
//       Then it displays the stream dictionary properties as a PropertyTable,
//       And stream metadata (length, filters) below the properties.
// ---------------------------------------------------------------------------

describe('DetailPanel stream rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(streamDetail);
    mockGetContentStream.mockResolvedValue({ nodeId: 'obj:0:10', raw: '', tokenized: null, formatted: null, error: '' });
  });

  test('does not render stream dictionary properties (shown in ObjectInfoPanel)', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Content Stream');
    });

    // Properties and metadata are in ObjectInfoPanel, not DetailPanel
    expect(screen.queryByText('/Filter')).not.toBeInTheDocument();
    expect(screen.queryByTestId('stream-metadata')).not.toBeInTheDocument();
  });

  test('header shows context label "Content Stream"', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Content Stream');
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
// DetailPanel is exported via React.memo: the component is wrapped in
// React.memo to prevent re-renders when switching tabs.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// DetailPanel cancels stale fetch on rapid node change: when selectedNodeId
// changes, previous in-flight fetch results must not overwrite the new result.
// ---------------------------------------------------------------------------

describe('DetailPanel stale fetch cancellation', () => {
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
          payload: { tabId: 'tab-1', fileName: 'test.pdf', filePath: '/path/to/test.pdf', rootNode: catalogNode, rootChildren: rootChildren },
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
// DetailPanel keeps previous detail visible during load Previous detail
// stays visible until the new fetch resolves, avoiding a flash of
// empty/error state during tab switches.
// ---------------------------------------------------------------------------

describe('DetailPanel keeps previous detail during load', () => {
  test('previous detail remains visible while new node loads', async () => {
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
          payload: { tabId: 'tab-1', fileName: 'test.pdf', filePath: '/path/to/test.pdf', rootNode: catalogNode, rootChildren: rootChildren },
        }} />
        <DispatchHelper action={{
          type: 'SELECT_NODE',
          payload: { nodeId: 'obj:0:5' },
        }} />
        <DetailPanel />
      </AppProvider>
    );

    // Old detail stays visible while new fetch is pending (no flash of empty state)
    expect(screen.getByText('/Type')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// DetailPanel edge cases for empty data -#5: Edge cases for
// empty properties, elements, filters.
// ---------------------------------------------------------------------------

describe('DetailPanel edge cases', () => {
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

  test('stream metadata not rendered in DetailPanel (shown in ObjectInfoPanel)', async () => {
    const noFilterStream = {
      ...streamDetail,
      streamInfo: { length: 500, filters: [] },
    };
    mockGetObjectDetail.mockResolvedValue(noFilterStream);
    mockGetContentStream.mockResolvedValue({ nodeId: 'obj:0:10', raw: '', tokenized: null, formatted: null, error: '' });
    renderWithState('obj:0:10');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Content Stream');
    });

    expect(screen.queryByTestId('stream-metadata')).not.toBeInTheDocument();
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

describe('DetailPanel React.memo export', () => {
  test('DetailPanel is a memoized component', () => {
    // React.memo wraps the component and sets $$typeof and type properties.
    // The display name should contain "DetailPanel".
    // React.memo components have a .type property pointing to the inner component.
    expect(DetailPanel).toBeDefined();
    // React.memo components are objects with a $$typeof Symbol and a .type property
    expect((DetailPanel as unknown as { type: unknown }).type).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// Content Stream Viewer integration tests
// ---------------------------------------------------------------------------

const contentStreamData = {
  nodeId: 'obj:0:10',
  raw: 'BT\n/F1 12 Tf\n100 700 Td\n(Hello World) Tj\nET',
  tokenized: null,
  formatted: null,
  error: '',
};

const contentStreamErrorData = {
  nodeId: 'obj:0:10',
  raw: '',
  tokenized: null,
  formatted: null,
  error: 'failed to decode: unsupported filter JBIG2Decode',
};

// ---------------------------------------------------------------------------
// DetailPanel renders ContentStreamViewer for stream nodes: When a stream
// node is selected AND GetContentStream returns raw text,
//       ContentStreamViewer renders with line numbers and content.
// ---------------------------------------------------------------------------

describe('DetailPanel content stream integration', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(streamDetail);
    mockGetContentStream.mockResolvedValue(contentStreamData);
  });

  test('renders ContentStreamViewer with raw text when stream node selected', async () => {
    renderWithState('obj:0:10');

    // Content stream viewer renders with content
    await waitFor(() => {
      expect(screen.getByTestId('content-stream-viewer')).toBeInTheDocument();
    });

    await waitFor(() => {
      const content = screen.getByTestId('content-stream-content');
      expect(content).toHaveTextContent('BT');
      expect(content).toHaveTextContent('/F1 12 Tf');
      expect(content).toHaveTextContent('ET');
    });
  });

  test('renders line numbers in gutter for content stream', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const gutter = screen.getByTestId('content-stream-gutter');
      expect(gutter).toHaveTextContent('1');
      expect(gutter).toHaveTextContent('5');
    });
  });

  test('calls GetContentStream with correct tab and node arguments', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      expect(mockGetContentStream).toHaveBeenCalledWith('tab-1', 'obj:0:10');
    });
  });

  test('header shows "Content Stream" label for stream nodes', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Content Stream');
    });
  });
});

// ---------------------------------------------------------------------------
// DetailPanel shows content stream error: When GetContentStream returns
// an error, the error is displayed.
// ---------------------------------------------------------------------------

describe('DetailPanel content stream error', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(streamDetail);
    mockGetContentStream.mockResolvedValue(contentStreamErrorData);
  });

  test('renders content stream error message when decode fails', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const errorEl = screen.getByTestId('content-stream-error');
      expect(errorEl).toHaveTextContent('failed to decode: unsupported filter JBIG2Decode');
    });
  });

  test('does not render content stream content when error is present', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      expect(screen.getByTestId('content-stream-error')).toBeInTheDocument();
    });

    expect(screen.queryByTestId('content-stream-content')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// GetContentStream NOT called for non-stream nodes: Content stream
// fetch is only triggered for stream-type nodes.
// ---------------------------------------------------------------------------

describe('DetailPanel does not fetch content stream for non-stream nodes', () => {
  test('GetContentStream is not called when a dict node is selected', async () => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(dictDetail);
    renderWithState('root');

    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });

    expect(mockGetContentStream).not.toHaveBeenCalled();
  });

  test('GetContentStream is not called when an array node is selected', async () => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(arrayDetail);
    renderWithState('obj:0:5');

    await waitFor(() => {
      expect(screen.getByText('[0]')).toBeInTheDocument();
    });

    expect(mockGetContentStream).not.toHaveBeenCalled();
  });

  test('GetContentStream is not called when a scalar node is selected', async () => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(scalarDetail);
    renderWithState('dict:root:Type');

    await waitFor(() => {
      expect(screen.getByText('/Catalog')).toBeInTheDocument();
    });

    expect(mockGetContentStream).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Stale content stream fetch cancelled on node change: the content stream
// fetch uses the stale-fetch guard, so changing node discards the previous
// in-flight content stream response.
// ---------------------------------------------------------------------------

describe('DetailPanel stale content stream cancellation', () => {
  test('stale content stream result is discarded when node changes', async () => {
    vi.clearAllMocks();

    // First node: stream that returns slowly
    let resolveFirstStream: (v: unknown) => void;
    const firstStreamPromise = new Promise((resolve) => {
      resolveFirstStream = resolve;
    });
    mockGetObjectDetail
      .mockResolvedValueOnce(streamDetail)
      .mockResolvedValueOnce(dictDetail);
    mockGetContentStream.mockReturnValueOnce(firstStreamPromise);

    const { rerender } = renderWithState('obj:0:10');

    // Wait for stream detail to render (header shows Content Stream)
    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Content Stream');
    });

    // Switch to a dict node before content stream resolves
    rerender(
      <AppProvider>
        <DispatchHelper
          action={{
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: 'tab-1',
              fileName: 'test.pdf',
              filePath: '/path/to/test.pdf',
              rootNode: catalogNode,
              rootChildren: rootChildren,
            },
          }}
        />
        <DispatchHelper
          action={{
            type: 'SELECT_NODE',
            payload: { nodeId: 'root' },
          }}
        />
        <DetailPanel />
      </AppProvider>
    );

    // Now resolve the stale content stream
    resolveFirstStream!(contentStreamData);

    // The dict detail should render, not the content stream
    await waitFor(() => {
      expect(screen.getByText('/Pages')).toBeInTheDocument();
    });

    // Content stream viewer should NOT be present
    expect(
      screen.queryByTestId('content-stream-viewer')
    ).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Loading indicator appears after 200ms delay: When loading takes more than
// 200ms, a subtle loading indicator appears.
// ---------------------------------------------------------------------------

describe('DetailPanel content stream loading indicator', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('loading indicator does NOT appear before 200ms', async () => {
    mockGetObjectDetail.mockResolvedValue(streamDetail);
    // Content stream hangs indefinitely
    mockGetContentStream.mockReturnValue(new Promise(() => {}));

    renderWithState('obj:0:10');

    // Flush pending microtasks (resolved promises) so detail state settles
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    // Advance 199ms -- loading indicator should NOT be visible yet
    await act(async () => {
      await vi.advanceTimersByTimeAsync(199);
    });

    expect(
      screen.queryByTestId('content-stream-loading')
    ).not.toBeInTheDocument();
  });

  test('loading indicator appears after 200ms when stream fetch is pending', async () => {
    mockGetObjectDetail.mockResolvedValue(streamDetail);
    mockGetContentStream.mockReturnValue(new Promise(() => {}));

    renderWithState('obj:0:10');

    // Flush microtasks so detail state settles
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    // Advance past 200ms debounce
    await act(async () => {
      await vi.advanceTimersByTimeAsync(201);
    });

    expect(
      screen.getByTestId('content-stream-loading')
    ).toBeInTheDocument();
    expect(
      screen.getByTestId('content-stream-loading')
    ).toHaveTextContent('Decoding stream...');
  });

  test('loading indicator disappears when content stream resolves', async () => {
    let resolveStream: (v: unknown) => void;
    const streamPromise = new Promise((resolve) => {
      resolveStream = resolve;
    });
    mockGetObjectDetail.mockResolvedValue(streamDetail);
    mockGetContentStream.mockReturnValue(streamPromise);

    renderWithState('obj:0:10');

    // Flush microtasks so detail state settles
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    // Show loading indicator
    await act(async () => {
      await vi.advanceTimersByTimeAsync(201);
    });

    expect(
      screen.getByTestId('content-stream-loading')
    ).toBeInTheDocument();

    // Resolve the stream and flush
    await act(async () => {
      resolveStream!(contentStreamData);
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(
      screen.queryByTestId('content-stream-loading')
    ).not.toBeInTheDocument();
    expect(
      screen.getByTestId('content-stream-viewer')
    ).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// GetContentStream IPC rejection renders error: When the IPC call
// itself rejects (not a struct-level error),
//       the error is wrapped and displayed via ContentStreamViewer.
// ---------------------------------------------------------------------------

describe('DetailPanel content stream IPC rejection', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(streamDetail);
    mockGetContentStream.mockRejectedValue(new Error('IPC call failed: service unavailable'));
  });

  test('renders content stream error when GetContentStream promise rejects', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const errorEl = screen.getByTestId('content-stream-error');
      expect(errorEl).toHaveTextContent('IPC call failed: service unavailable');
    });
  });

  test('does not render content stream viewer when IPC rejects', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      expect(screen.getByTestId('content-stream-error')).toBeInTheDocument();
    });

    expect(screen.queryByTestId('content-stream-content')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Syntax highlighting integration tests
// ---------------------------------------------------------------------------

const sampleTokens = [
  { type: 'operator', value: 'BT', line: 1, col: 1 },
  { type: 'name', value: '/F1', line: 2, col: 1 },
  { type: 'number', value: '12', line: 2, col: 5 },
  { type: 'operator', value: 'Tf', line: 2, col: 8 },
  { type: 'number', value: '100', line: 3, col: 1 },
  { type: 'number', value: '700', line: 3, col: 5 },
  { type: 'operator', value: 'Td', line: 3, col: 9 },
  { type: 'string', value: '(Hello World)', line: 4, col: 1 },
  { type: 'operator', value: 'Tj', line: 4, col: 15 },
  { type: 'operator', value: 'ET', line: 5, col: 1 },
];

const contentStreamDataWithTokens = {
  nodeId: 'obj:0:10',
  raw: 'BT\n/F1 12 Tf\n100 700 Td\n(Hello World) Tj\nET',
  tokenized: sampleTokens,
  // The Go formatter pre-groups tokens into FormattedLine[]; here
  // we wrap all sample tokens into a single row for the integration test
  // since the per-token highlight assertions don't depend on row structure.
  formatted: [{
    tokens: sampleTokens,
    indent: 0,
    operator: 'ET',
    srcLineStart: 1,
    srcLineEnd: 5,
  }],
  error: '',
};

// ---------------------------------------------------------------------------
// DetailPanel passes tokenized data to ContentStreamViewer and renders
// syntax-highlighted tokens.
// When stream node selected with tokenized data, operator tokens have
//       text-token-operator class.
// ---------------------------------------------------------------------------

describe('DetailPanel syntax highlighting integration', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(streamDetail);
    mockGetContentStream.mockResolvedValue(contentStreamDataWithTokens);
  });

  test('renders syntax-highlighted tokens with type-based CSS classes', async () => {
    renderWithState('obj:0:10');

    // Wait for content stream viewer to render
    await waitFor(() => {
      expect(screen.getByTestId('content-stream-viewer')).toBeInTheDocument();
    });

    // Operator tokens should have the operator CSS class
    await waitFor(() => {
      const btEl = screen.getByText('BT');
      expect(btEl.className).toMatch(/text-token-operator/);
    });
  });

  test('name tokens have text-token-name class in integration', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const nameEl = screen.getByText('/F1');
      expect(nameEl.className).toMatch(/text-token-name/);
    });
  });

  test('number tokens have text-token-number class in integration', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const numEl = screen.getByText('12');
      expect(numEl.className).toMatch(/text-token-number/);
    });
  });

  test('string tokens have text-token-string class in integration', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const strEl = screen.getByText('(Hello World)');
      expect(strEl.className).toMatch(/text-token-string/);
    });
  });

  test('operator tokens have font-semibold for non-color differentiation', async () => {
    renderWithState('obj:0:10');

    await waitFor(() => {
      const btEl = screen.getByText('BT');
      expect(btEl.className).toMatch(/font-semibold/);
    });
  });
});

// ---------------------------------------------------------------------------
// Image Preview in Detail Panel -- Integration Tests
// ---------------------------------------------------------------------------

// Minimal 1x1 PNG for test rendering
const TINY_PNG_BASE64 =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==';

const imageStreamDetail = {
  nodeId: 'obj:0:20',
  objectRef: '20 0 R',
  type: 'stream',
  properties: [
    {
      key: '/Subtype',
      value: {
        type: 'name',
        display: '/Image',
        raw: '/Image',
        refTarget: '',
      },
    },
  ],
  elements: [],
  scalarValue: null,
  streamInfo: {
    length: 5000,
    filters: ['DCTDecode'],
  },
};

const mockImageDataResponse = {
  nodeId: 'obj:0:20',
  objectRef: '20 0 R',
  mimeType: 'image/png',
  base64: TINY_PNG_BASE64,
  width: 320,
  height: 240,
  colorSpace: 'DeviceRGB',
  bitsPerComponent: 8,
  filter: 'DCTDecode',
  warning: '',
  error: '',
};

// ---------------------------------------------------------------------------
// DetailPanel renders ImagePreview when selected node has iconHint
// "image".
// When an XObject image node is selected, the DetailPanel switches to
//       image preview mode showing the rendered image.
// ---------------------------------------------------------------------------

describe('DetailPanel image preview mode', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetImageData.mockResolvedValue(mockImageDataResponse);
  });

  test('renders ImagePreview when node has iconHint "image"', async () => {
    renderWithState('obj:0:20', { iconHint: 'image' });

    await waitFor(() => {
      expect(screen.getByTestId('image-preview-img')).toBeInTheDocument();
    });

    // Should NOT render content stream viewer
    expect(screen.queryByTestId('content-stream-viewer')).not.toBeInTheDocument();
  });

  test('calls GetImageData with correct tab and node arguments', async () => {
    renderWithState('obj:0:20', { iconHint: 'image' });

    await waitFor(() => {
      expect(mockGetImageData).toHaveBeenCalledWith('tab-1', 'obj:0:20');
    });
  });

  test('does NOT call GetContentStream for image nodes', async () => {
    renderWithState('obj:0:20', { iconHint: 'image' });

    await waitFor(() => {
      expect(screen.getByTestId('image-preview-img')).toBeInTheDocument();
    });

    expect(mockGetContentStream).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// DetailPanel header shows "Image Preview" with object reference
// for image nodes.
// The panel header shows "Image Preview" with the object reference.
// ---------------------------------------------------------------------------

describe('DetailPanel image preview header', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetImageData.mockResolvedValue(mockImageDataResponse);
  });

  test('header shows "Image Preview" instead of "Content Stream" for image nodes', async () => {
    renderWithState('obj:0:20', { iconHint: 'image' });

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Image Preview');
    });
  });

  test('header shows object reference for image nodes', async () => {
    renderWithState('obj:0:20', { iconHint: 'image' });

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('20 0 R');
    });
  });

  test('header does NOT show "Content Stream" for image nodes', async () => {
    renderWithState('obj:0:20', { iconHint: 'image' });

    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).not.toHaveTextContent('Content Stream');
    });
  });
});

// ---------------------------------------------------------------------------
// Loading state shown while image data is being fetched: When fetch takes
// longer than 200ms, a "Loading image..." indicator
//       appears (same debounce pattern as content stream loading).
// ---------------------------------------------------------------------------

describe('DetailPanel image loading state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('loading indicator does NOT appear before 200ms', async () => {
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    // Image data hangs indefinitely
    mockGetImageData.mockReturnValue(new Promise(() => {}));

    renderWithState('obj:0:20', { iconHint: 'image' });

    // Flush pending microtasks so detail state settles
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    // Advance 199ms -- loading indicator should NOT be visible yet
    await act(async () => {
      await vi.advanceTimersByTimeAsync(199);
    });

    expect(screen.queryByTestId('image-loading')).not.toBeInTheDocument();
  });

  test('loading indicator appears after 200ms when image fetch is pending', async () => {
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetImageData.mockReturnValue(new Promise(() => {}));

    renderWithState('obj:0:20', { iconHint: 'image' });

    // Flush microtasks so detail state settles
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    // Advance past 200ms debounce
    await act(async () => {
      await vi.advanceTimersByTimeAsync(201);
    });

    expect(screen.getByTestId('image-loading')).toBeInTheDocument();
    expect(screen.getByTestId('image-loading')).toHaveTextContent('Loading image...');
  });

  test('loading indicator disappears when image data resolves', async () => {
    let resolveImage: (v: unknown) => void;
    const imagePromise = new Promise((resolve) => {
      resolveImage = resolve;
    });
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetImageData.mockReturnValue(imagePromise);

    renderWithState('obj:0:20', { iconHint: 'image' });

    // Flush microtasks so detail state settles
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    // Show loading indicator
    await act(async () => {
      await vi.advanceTimersByTimeAsync(201);
    });

    expect(screen.getByTestId('image-loading')).toBeInTheDocument();

    // Resolve image data and flush
    await act(async () => {
      resolveImage!(mockImageDataResponse);
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(screen.queryByTestId('image-loading')).not.toBeInTheDocument();
    expect(screen.getByTestId('image-preview-img')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Switching from image node to dict node clears image preview and shows
// the appropriate view.
// When a non-image node is selected after an image node, the image
//       preview is cleared and the dict/array/scalar/stream view is shown.
// ---------------------------------------------------------------------------

describe('DetailPanel clears image on node switch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('switching from image node to dict node clears image preview', async () => {
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetImageData.mockResolvedValue(mockImageDataResponse);

    const { rerender } = renderWithState('obj:0:20', { iconHint: 'image' });

    // Image preview should appear
    await waitFor(() => {
      expect(screen.getByTestId('image-preview-img')).toBeInTheDocument();
    });

    // Switch to a dict node
    mockGetObjectDetail.mockResolvedValue(dictDetail);

    rerender(
      <AppProvider>
        <DispatchHelper
          action={{
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: 'tab-1',
              fileName: 'test.pdf',
              filePath: '/path/to/test.pdf',
              rootNode: catalogNode,
              rootChildren: rootChildren,
            },
          }}
        />
        <DispatchHelper
          action={{
            type: 'SELECT_NODE',
            payload: { nodeId: 'root' },
          }}
        />
        <DetailPanel />
      </AppProvider>
    );

    // Dict properties should appear, image preview should be gone
    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });

    expect(screen.queryByTestId('image-preview-img')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Navigating back to an image node restores the image preview:
// NavHistoryEntry.iconHint is preserved and restored on NAVIGATE_BACK.
// ---------------------------------------------------------------------------

describe('DetailPanel navigate back restores image', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('navigating back to image node restores image preview', async () => {
    // First: select image node
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetImageData.mockResolvedValue(mockImageDataResponse);

    const { rerender } = renderWithState('obj:0:20', { iconHint: 'image' });

    await waitFor(() => {
      expect(screen.getByTestId('image-preview-img')).toBeInTheDocument();
    });

    // Then: navigate to a dict node (non-image)
    mockGetObjectDetail.mockResolvedValue(dictDetail);

    rerender(
      <AppProvider>
        <DispatchHelper
          action={{
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: 'tab-1',
              fileName: 'test.pdf',
              filePath: '/path/to/test.pdf',
              rootNode: catalogNode,
              rootChildren: rootChildren,
            },
          }}
        />
        <DispatchHelper
          action={{
            type: 'SELECT_NODE',
            payload: { nodeId: 'obj:0:20', iconHint: 'image' },
          }}
        />
        <DispatchHelper
          action={{
            type: 'SELECT_NODE',
            payload: { nodeId: 'root' },
          }}
        />
        <DetailPanel />
      </AppProvider>
    );

    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });

    expect(screen.queryByTestId('image-preview-img')).not.toBeInTheDocument();

    // Then: navigate back
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetImageData.mockResolvedValue(mockImageDataResponse);

    rerender(
      <AppProvider>
        <DispatchHelper
          action={{
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: 'tab-1',
              fileName: 'test.pdf',
              filePath: '/path/to/test.pdf',
              rootNode: catalogNode,
              rootChildren: rootChildren,
            },
          }}
        />
        <DispatchHelper
          action={{
            type: 'SELECT_NODE',
            payload: { nodeId: 'obj:0:20', iconHint: 'image' },
          }}
        />
        <DispatchHelper
          action={{
            type: 'SELECT_NODE',
            payload: { nodeId: 'root' },
          }}
        />
        <DispatchHelper action={{ type: 'NAVIGATE_BACK' }} />
        <DetailPanel />
      </AppProvider>
    );

    // Image preview should reappear after navigating back
    await waitFor(() => {
      expect(screen.getByTestId('image-preview-img')).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// GetImageData IPC rejection renders error in ImagePreview: When
// GetImageData promise rejects (IPC-level failure), the error is
//       wrapped and displayed via ImagePreview's error state.
// ---------------------------------------------------------------------------

describe('DetailPanel image data IPC rejection', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetImageData.mockRejectedValue(new Error('IPC call failed: service unavailable'));
  });

  test('renders image preview error when GetImageData promise rejects', async () => {
    renderWithState('obj:0:20', { iconHint: 'image' });

    await waitFor(() => {
      const errorEl = screen.getByTestId('image-preview-error');
      expect(errorEl).toHaveTextContent('IPC call failed: service unavailable');
    });
  });

  test('does not render img element when GetImageData IPC rejects', async () => {
    renderWithState('obj:0:20', { iconHint: 'image' });

    await waitFor(() => {
      expect(screen.getByTestId('image-preview-error')).toBeInTheDocument();
    });

    expect(screen.queryByTestId('image-preview-img')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Stale image fetch cancelled on node change: the image data fetch uses the
// stale-fetch guard, so changing node discards the previous in-flight image
// response.
// ---------------------------------------------------------------------------

describe('DetailPanel stale image fetch cancellation', () => {
  test('stale image data result is discarded when node changes', async () => {
    vi.clearAllMocks();

    // First node: image stream that returns slowly
    let resolveFirstImage: (v: unknown) => void;
    const firstImagePromise = new Promise((resolve) => {
      resolveFirstImage = resolve;
    });
    mockGetObjectDetail
      .mockResolvedValueOnce(imageStreamDetail)
      .mockResolvedValueOnce(dictDetail);
    mockGetImageData.mockReturnValueOnce(firstImagePromise);

    const { rerender } = renderWithState('obj:0:20', { iconHint: 'image' });

    // Wait for detail to load (header shows Image Preview)
    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header).toHaveTextContent('Image Preview');
    });

    // Switch to a dict node before image data resolves
    rerender(
      <AppProvider>
        <DispatchHelper
          action={{
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: 'tab-1',
              fileName: 'test.pdf',
              filePath: '/path/to/test.pdf',
              rootNode: catalogNode,
              rootChildren: rootChildren,
            },
          }}
        />
        <DispatchHelper
          action={{
            type: 'SELECT_NODE',
            payload: { nodeId: 'root' },
          }}
        />
        <DetailPanel />
      </AppProvider>
    );

    // Now resolve the stale image data
    resolveFirstImage!(mockImageDataResponse);

    // The dict detail should render, not the image preview
    await waitFor(() => {
      expect(screen.getByText('/Pages')).toBeInTheDocument();
    });

    // Image preview should NOT be present
    expect(screen.queryByTestId('image-preview-img')).not.toBeInTheDocument();
  });
});
