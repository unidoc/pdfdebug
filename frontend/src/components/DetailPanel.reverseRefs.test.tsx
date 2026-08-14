/**
 * Object Source View + Reverse References
 *
 * Covers frontend banner trigger, section mount-after-parsed-view, orphan
 * empty state propagation, catalog copy, per-document isolation, and
 * redundancy fix (DetailPanel keeps DictView/ArrayView/ ScalarView intact --
 * the section appears in addition).
 *
 * The behavior surface of ReverseRefsSection itself is asserted in
 * ReverseRefsSection.test.tsx. This file asserts the integration: DetailPanel
 * mounts the section for indirect-object selections, skips it for inline-node
 * selections, propagates indexUnavailable from the sentinel error, and keeps
 * the parsed view visible alongside the section.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.reverseRefs.test.tsx
 */
import { render, screen, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  type AppAction,
} from '../hooks/useDocumentState';
import { DetailPanel } from './DetailPanel';

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

// Mock Wails bindings.
const mockGetObjectDetail = vi.fn();
const mockGetContentStream = vi.fn();
const mockGetImageData = vi.fn();
const mockGetReverseRefs = vi.fn();
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
    // calls these on render; stub them so the mock does not throw.
    GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
    GetSignatures: vi.fn().mockResolvedValue([]),
    GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
    GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
    SaveBytesToFile: vi.fn().mockResolvedValue(''),
  })
);

// --- Fixtures ---

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

const pageDetail = {
  nodeId: 'obj:0:3',
  objectRef: '3 0 R',
  type: 'dict',
  properties: [
    {
      key: '/Type',
      value: { type: 'name', display: '/Page', raw: '/Page', refTarget: '' },
    },
  ],
  elements: [],
  scalarValue: null,
  streamInfo: null,
};

const inlineScalarDetail = {
  nodeId: 'dict:obj:0:3:Type',
  objectRef: '',
  type: 'scalar',
  properties: [],
  elements: [],
  scalarValue: { type: 'name', display: '/Page', raw: '/Page', refTarget: '' },
  streamInfo: null,
};

const catalogDetail = {
  nodeId: 'root',
  objectRef: '',
  type: 'dict',
  properties: [
    {
      key: '/Pages',
      value: { type: 'reference', display: '2 0 R', raw: '2 0 R', refTarget: 'obj:0:2' },
    },
  ],
  elements: [],
  scalarValue: null,
  streamInfo: null,
};

const reverseRefsForPage = [
  { parentNodeId: 'obj:0:2', parentRef: '2 0 R', parentType: 'Pages', path: '/Kids[0]' },
];

function DispatchHelper({ action }: { action: AppAction }) {
  const dispatch = useAppDispatch();
  dispatch(action);
  return null;
}

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: {
    tabId: 'tab-1',
    fileName: 'test.pdf',
    filePath: '/path/to/test.pdf',
    rootNode: catalogNode,
    rootChildren: [],
  },
};

function renderDetailPanelFor(
  nodeId: string,
  iconHint: string | null = null
) {
  const selectAction: AppAction = {
    type: 'SELECT_NODE',
    payload: { nodeId, iconHint: iconHint ?? undefined },
  };
  return render(
    <AppProvider>
      <DispatchHelper action={openAction} />
      <DispatchHelper action={selectAction} />
      <DetailPanel />
    </AppProvider>
  );
}

// ---------------------------------------------------------------------------
// Section mounts AFTER the parsed view for indirect-object selections;
// parsed view stays intact.
// ---------------------------------------------------------------------------

describe('section mounts after parsed view for indirect objects', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue(reverseRefsForPage);
  });

  test('parsed view (DictView /Type row) and Referenced by section both render', async () => {
    renderDetailPanelFor('obj:0:3');
    // Parsed view still here
    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
      expect(screen.getByText('/Page')).toBeInTheDocument();
    });
    // Section appears with the reverse-ref entry
    await waitFor(() => {
      expect(screen.getByText('2 0 R')).toBeInTheDocument();
      expect(screen.getByText('/Kids[0]')).toBeInTheDocument();
    });
  });

  test('GetReverseRefs is called with (activeTabId, selectedNodeId)', async () => {
    renderDetailPanelFor('obj:0:3');
    await waitFor(() => {
      expect(mockGetReverseRefs).toHaveBeenCalledWith('tab-1', 'obj:0:3');
    });
  });
});

// ---------------------------------------------------------------------------
// Task 7.1: section is NOT mounted for inline-value nodes. nodeID
// `dict:obj:0:3:Type` is inline; the section MUST NOT appear.
// ---------------------------------------------------------------------------

describe('section suppressed for inline-value nodes', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(inlineScalarDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('section is not rendered for an inline scalar selection', async () => {
    renderDetailPanelFor('dict:obj:0:3:Type');
    await waitFor(() => {
      expect(screen.getByText('/Page')).toBeInTheDocument();
    });
    // No Referenced by header, no orphan copy, no unavailable banner.
    expect(screen.queryByText(/Referenced by/)).not.toBeInTheDocument();
    expect(screen.queryByText(/possible orphan/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Reverse-ref index unavailable/)).not.toBeInTheDocument();
  });

  test('GetReverseRefs is NOT called for inline-value nodes', async () => {
    renderDetailPanelFor('dict:obj:0:3:Type');
    await waitFor(() => {
      expect(screen.getByText('/Page')).toBeInTheDocument();
    });
    expect(mockGetReverseRefs).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Orphan empty state propagates from the backend. Empty list + non-catalog
// selection -> orphan copy with "dict-graph" qualifier.
// ---------------------------------------------------------------------------

describe('orphan empty-state path', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]); // empty list signals orphan
  });

  test('non-catalog selection with empty list renders orphan copy', async () => {
    renderDetailPanelFor('obj:0:3', 'page');
    await waitFor(() => {
      expect(
        screen.getByText('No incoming dict-graph references (possible orphan).')
      ).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// Catalog selection renders "Document root..."
// ---------------------------------------------------------------------------

describe('catalog selection -- Document root copy', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(catalogDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('selecting the catalog (iconHint=catalog) renders the catalog empty state', async () => {
    renderDetailPanelFor('root', 'catalog');
    await waitFor(() => {
      expect(
        screen.getByText('Document root (no incoming references).')
      ).toBeInTheDocument();
    });
    expect(screen.queryByText(/possible orphan/i)).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Failure mode: backend rejection with the index- unavailable sentinel
// surfaces the unavailable banner. Task 7.3 case (a).
// ---------------------------------------------------------------------------

describe('index-unavailable sentinel surfaces the banner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
  });

  test('rejection containing the sentinel name sets indexUnavailable=true', async () => {
    // The Go sentinel is ErrReverseRefIndexUnavailable. Wails serializes
    // sentinel errors with the wrapped message, so we match by the
    // canonical substring "reverse-ref index unavailable" (case-insensitive).
    mockGetReverseRefs.mockRejectedValue(new Error('reverse-ref index unavailable'));
    renderDetailPanelFor('obj:0:3', 'page');
    await waitFor(() => {
      expect(
        screen.getByText('Reverse-ref index unavailable for this document.')
      ).toBeInTheDocument();
    });
    // Crucially: the orphan copy MUST NOT show (forbids silent
    // mislabelling-as-orphan when the index is unavailable).
    expect(screen.queryByText(/possible orphan/i)).not.toBeInTheDocument();
  });

  test('non-sentinel rejection hides the section silently (Task 7.3 case b)', async () => {
    mockGetReverseRefs.mockRejectedValue(new Error('some other error'));
    renderDetailPanelFor('obj:0:3', 'page');
    // Wait for the parsed view first so the fetch has had a chance to run.
    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });
    // No banner, no orphan copy, no Referenced by header.
    expect(screen.queryByText(/Reverse-ref index unavailable/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Referenced by/)).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Tab switch re-fetches with the active tabId.
// ---------------------------------------------------------------------------

describe('tab switch refetches reverse refs', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('changing the active tab triggers a fresh GetReverseRefs call', async () => {
    // Open a second tab and switch to it.
    const openSecond: AppAction = {
      type: 'OPEN_DOCUMENT',
      payload: {
        tabId: 'tab-2',
        fileName: 'second.pdf',
        filePath: '/second.pdf',
        rootNode: catalogNode,
        rootChildren: [],
      },
    };
    const selectFirst: AppAction = {
      type: 'SELECT_NODE',
      payload: { nodeId: 'obj:0:3', iconHint: 'page' },
    };

    const { rerender } = render(
      <AppProvider>
        <DispatchHelper action={openAction} />
        <DispatchHelper action={selectFirst} />
        <DetailPanel />
      </AppProvider>
    );

    await waitFor(() => {
      expect(mockGetReverseRefs).toHaveBeenCalledWith('tab-1', 'obj:0:3');
    });

    mockGetReverseRefs.mockClear();

    // Switch to a fresh tab + selection.
    rerender(
      <AppProvider>
        <DispatchHelper action={openAction} />
        <DispatchHelper action={openSecond} />
        <DispatchHelper action={{ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } }} />
        <DispatchHelper
          action={{ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:7', iconHint: 'page' } }}
        />
        <DetailPanel />
      </AppProvider>
    );

    await waitFor(() => {
      expect(mockGetReverseRefs).toHaveBeenCalledWith('tab-2', 'obj:0:7');
    });
  });
});

// ---------------------------------------------------------------------------
// Regression guard: the section MUST NOT flash the orphan empty state while
// the GetReverseRefs fetch is still in flight. Before the fix, an in-flight
// selection rendered "No incoming dict-graph references (possible orphan)"
// momentarily because reverseRefs=[] + visible=true.
// ---------------------------------------------------------------------------

describe('no orphan flash before fetch resolves', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
  });

  test('orphan copy does not appear before GetReverseRefs resolves', async () => {
    // Make GetReverseRefs hang forever so the in-flight window is observable.
    let resolveFn: ((val: ReverseRefEntry[]) => void) | null = null;
    mockGetReverseRefs.mockReturnValueOnce(
      new Promise<ReverseRefEntry[]>((resolve) => { resolveFn = resolve; })
    );

    renderDetailPanelFor('obj:0:3', 'page');

    // Wait for the parsed view so we know the panel rendered.
    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });

    // Orphan copy MUST NOT be present while fetch is in flight.
    expect(
      screen.queryByText(/possible orphan/i)
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/Referenced by/)).not.toBeInTheDocument();

    // Now resolve with reverse-refs; the section appears.
    resolveFn!(reverseRefsForPage);
    await waitFor(() => {
      expect(screen.getByText('2 0 R')).toBeInTheDocument();
    });
  });
});
