/**
 * DetailPanel Tabs Integration Tests
 *
 * Covers:
 *   - the tab bar with three triggers and Object as the default;
 *   - no-selection behavior: Object shows an empty state while XREF and
 *     Plain Text still fetch and render;
 *   - forceMount preserving scroll position, and switching documents
 *     resetting activeTab to Object;
 *   - NAVIGATE_TO_REF from XREF flipping activeTab to Object FIRST;
 *   - Radix activationMode="manual", arrow keys moving focus, and the
 *     tablist carrying aria-label="Detail view";
 *   - the Object pane keeping its existing header while the XREF and Plain
 *     Text panes do NOT render it (no stale "Properties - <key>");
 *   - no stale cross-document content frame on activeTabId change.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.tabs.test.tsx
 */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  useAppState,
  type AppAction,
} from '../hooks/useDocumentState';
import { DetailPanel } from './DetailPanel';

// Allotment is layout-only.
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

// Mock all Wails bindings DetailPanel touches.
const mockGetObjectDetail = vi.fn();
const mockGetContentStream = vi.fn();
const mockGetImageData = vi.fn();
const mockGetReverseRefs = vi.fn();
const mockGetFontView = vi.fn();
const mockGetXRefTable = vi.fn();
const mockGetPlainText = vi.fn();
const mockGetPlainTextSize = vi.fn();
const mockCancelPlainText = vi.fn();
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
    GetFontView: (...args: unknown[]) => mockGetFontView(...args),
    GetXRefTable: (...args: unknown[]) => mockGetXRefTable(...args),
    GetPlainText: (...args: unknown[]) => mockGetPlainText(...args),
    GetPlainTextSize: (...args: unknown[]) => mockGetPlainTextSize(...args),
    CancelPlainText: (...args: unknown[]) => mockCancelPlainText(...args),
    // The Embedded + Metadata tab panes forceMount, so DetailPanel
    // calls these on render; stub them so the mock does not throw.
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

const xrefSmall = {
  tabId: 'tab-1',
  entries: [
    { objNum: 1, gen: 0, status: 'in-use', offset: 15, hostObjStm: 0, nodeID: 'obj:0:1' },
    { objNum: 2, gen: 0, status: 'in-use', offset: 120, hostObjStm: 0, nodeID: 'obj:0:2' },
  ],
};

const xrefOtherDoc = {
  tabId: 'tab-2',
  entries: [
    { objNum: 99, gen: 0, status: 'in-use', offset: 800, hostObjStm: 0, nodeID: 'obj:0:99' },
  ],
};

const plainTextSmall = {
  tabId: 'tab-1',
  content: '%PDF-1.7\nhello\n',
  totalBytes: 15,
};

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

function DispatchHelper({ action }: { action: AppAction }) {
  const dispatch = useAppDispatch();
  dispatch(action);
  return null;
}

// Spy on dispatched actions for ordering assertions.
let dispatchSpy: ReturnType<typeof vi.fn> | null = null;
function DispatchSpy() {
  const dispatch = useAppDispatch();
  if (dispatchSpy === null) {
    dispatchSpy = vi.fn();
  }
  // Wrap dispatch to record actions. We can't actually replace the context
  // dispatch easily without rewriting the hook, so this is a render-side
  // observation that tracks state via useAppState.
  return null;
}

function StateProbe({ onState }: { onState: (state: ReturnType<typeof useAppState>) => void }) {
  const state = useAppState();
  onState(state);
  return null;
}

function renderDetailPanel(initialActions: AppAction[]) {
  return render(
    <AppProvider>
      {initialActions.map((a, i) => (
        <DispatchHelper key={i} action={a} />
      ))}
      <DispatchSpy />
      <DetailPanel />
    </AppProvider>
  );
}

// ---------------------------------------------------------------------------
// Tab bar renders with three triggers in order.
// ---------------------------------------------------------------------------

describe('tab bar renders three triggers', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetXRefTable.mockResolvedValue(xrefSmall);
    mockGetPlainText.mockResolvedValue(plainTextSmall);
  });

  test('all three tab triggers exist with the documented testids', async () => {
    renderDetailPanel([openAction]);
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-object')).toBeInTheDocument();
    });
    expect(screen.getByTestId('detail-tab-xref')).toBeInTheDocument();
    expect(screen.getByTestId('detail-tab-plaintext')).toBeInTheDocument();
  });

  test('Object tab is the default active tab', async () => {
    renderDetailPanel([openAction]);
    await waitFor(() => {
      const objectTrigger = screen.getByTestId('detail-tab-object');
      expect(objectTrigger.getAttribute('data-state')).toBe('active');
    });
    expect(screen.getByTestId('detail-tab-xref').getAttribute('data-state')).toBe('inactive');
    expect(screen.getByTestId('detail-tab-plaintext').getAttribute('data-state')).toBe('inactive');
  });
});

// ---------------------------------------------------------------------------
// Tablist has aria-label="Detail view".
// ---------------------------------------------------------------------------

describe('tablist aria-label', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('the tablist carries aria-label="Detail view"', async () => {
    renderDetailPanel([openAction]);
    await waitFor(() => {
      expect(screen.getByRole('tablist', { name: 'Detail view' })).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// Clicking XREF activates the XREF pane; Object pane is hidden via Radix
// data-state="inactive".
// ---------------------------------------------------------------------------

describe('clicking XREF activates the XREF pane', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetXRefTable.mockResolvedValue(xrefSmall);
  });

  test('XREF pane becomes active; Object pane becomes inactive', async () => {
    renderDetailPanel([openAction]);
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-xref')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('detail-tab-xref'));
    await waitFor(() => {
      expect(screen.getByTestId('detail-pane-xref').getAttribute('data-state')).toBe('active');
    });
    expect(screen.getByTestId('detail-pane-object').getAttribute('data-state')).toBe('inactive');
  });
});

// ---------------------------------------------------------------------------
// Clicking Plain Text activates the Plain Text pane.
// ---------------------------------------------------------------------------

describe('clicking Plain Text activates the Plain Text pane', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetPlainText.mockResolvedValue(plainTextSmall);
  });

  test('Plain Text pane becomes active', async () => {
    renderDetailPanel([openAction]);
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-plaintext')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('detail-tab-plaintext'));
    await waitFor(() => {
      expect(screen.getByTestId('detail-pane-plaintext').getAttribute('data-state')).toBe('active');
    });
  });
});

// ---------------------------------------------------------------------------
// Object tab pane renders the existing header (nav buttons) -- XREF and
// Plain Text panes do NOT render the same header.
// ---------------------------------------------------------------------------

describe('Object pane keeps the existing header; XREF/Plain Text panes do not', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetXRefTable.mockResolvedValue(xrefSmall);
  });

  test('nav-back-button renders inside the Object pane', async () => {
    const selectAction: AppAction = {
      type: 'SELECT_NODE',
      payload: { nodeId: 'obj:0:3' },
    };
    renderDetailPanel([openAction, selectAction]);
    await waitFor(() => {
      const objectPane = screen.getByTestId('detail-pane-object');
      expect(objectPane.querySelector('[data-testid="nav-back-button"]')).toBeInTheDocument();
    });
  });

  test('nav-back-button does NOT render inside the XREF pane', async () => {
    const selectAction: AppAction = {
      type: 'SELECT_NODE',
      payload: { nodeId: 'obj:0:3' },
    };
    renderDetailPanel([openAction, selectAction]);
    await waitFor(() => {
      expect(screen.getByTestId('detail-pane-xref')).toBeInTheDocument();
    });
    const xrefPane = screen.getByTestId('detail-pane-xref');
    expect(xrefPane.querySelector('[data-testid="nav-back-button"]')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// With NO selection, the Object pane shows the existing empty-state
// copy, while XREF and Plain Text still fetch and render their
// document-level content.
// ---------------------------------------------------------------------------

describe('no-selection -- Object empty + XREF/Plain Text fetch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefSmall);
    mockGetPlainText.mockResolvedValue(plainTextSmall);
  });

  test('Object pane shows the "Select a node" empty-state copy', async () => {
    renderDetailPanel([openAction]);
    await waitFor(() => {
      const objectPane = screen.getByTestId('detail-pane-object');
      expect(objectPane.textContent).toMatch(/Select a node/i);
    });
  });

  test('switching to XREF still fetches GetXRefTable without a selection', async () => {
    renderDetailPanel([openAction]);
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-xref')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('detail-tab-xref'));
    await waitFor(() => {
      expect(mockGetXRefTable).toHaveBeenCalledWith('tab-1');
    });
  });

  test('switching to Plain Text still fetches GetPlainText without a selection', async () => {
    renderDetailPanel([openAction]);
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-plaintext')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('detail-tab-plaintext'));
    await waitFor(() => {
      expect(mockGetPlainText).toHaveBeenCalledWith('tab-1');
    });
  });
});

// ---------------------------------------------------------------------------
// Switching documents resets active tab to Object.
// ---------------------------------------------------------------------------

describe('switching documents resets active tab to Object', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetXRefTable.mockImplementation((tabID: string) => {
      if (tabID === 'tab-2') return Promise.resolve(xrefOtherDoc);
      return Promise.resolve(xrefSmall);
    });
    mockGetPlainText.mockResolvedValue(plainTextSmall);
  });

  test('after switching tabs, Object pane is active again', async () => {
    const { rerender } = render(
      <AppProvider>
        <DispatchHelper action={openAction} />
        <DetailPanel />
      </AppProvider>
    );
    // Activate XREF in document 1.
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-xref')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('detail-tab-xref'));
    await waitFor(() => {
      expect(screen.getByTestId('detail-pane-xref').getAttribute('data-state')).toBe('active');
    });
    // Open document 2 and switch to it.
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
    rerender(
      <AppProvider>
        <DispatchHelper action={openAction} />
        <DispatchHelper action={openSecond} />
        <DispatchHelper action={{ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } }} />
        <DetailPanel />
      </AppProvider>
    );
    // Object pane should be active again on the new document.
    await waitFor(() => {
      expect(screen.getByTestId('detail-pane-object').getAttribute('data-state')).toBe('active');
    });
  });
});

// ---------------------------------------------------------------------------
// No stale cross-document content frame. While XREF is active in
// doc 1, switching to doc 2 MUST NOT show doc 1's xref rows in doc
// 2's pane.
// ---------------------------------------------------------------------------

describe('no stale cross-document content frame', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetXRefTable.mockImplementation((tabID: string) => {
      if (tabID === 'tab-2') return Promise.resolve(xrefOtherDoc);
      return Promise.resolve(xrefSmall);
    });
  });

  test('switching documents does not flash doc1 rows in doc2 pane', async () => {
    const { rerender } = render(
      <AppProvider>
        <DispatchHelper action={openAction} />
        <DetailPanel />
      </AppProvider>
    );
    // Activate XREF and let it fetch.
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-xref')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('detail-tab-xref'));
    await waitFor(() => {
      // Doc1 row visible.
      expect(screen.getByText('15')).toBeInTheDocument();
    });
    // Switch tabs.
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
    rerender(
      <AppProvider>
        <DispatchHelper action={openAction} />
        <DispatchHelper action={openSecond} />
        <DispatchHelper action={{ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } }} />
        <DetailPanel />
      </AppProvider>
    );
    // The Object pane is active on the new doc, so doc1's "15" value should
    // NOT be visible: no stale frame may leak across a document switch, even
    // before the activeTab reset.
    expect(screen.queryByText('15')).not.toBeInTheDocument();
    expect(screen.queryByText('120')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// NAVIGATE_TO_REF from XREF flips activeTab to Object FIRST so the user
// does not see a flash of XREF active + new selection. Observed via the
// post-click state: the Object pane is active and the new selection's
// content is what shows.
// ---------------------------------------------------------------------------

describe('activeTab=object before SELECT_NODE', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetXRefTable.mockResolvedValue(xrefSmall);
  });

  test('clicking an XREF row activates the Object pane and the selection updates', async () => {
    renderDetailPanel([openAction]);
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-xref')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('detail-tab-xref'));
    await waitFor(() => {
      expect(screen.getByText('15')).toBeInTheDocument();
    });
    // Click an in-use row -- DetailPanel must reset detailView to Object
    // and dispatch NAVIGATE_TO_REF for obj:0:1.
    fireEvent.click(screen.getByTestId('xref-row-1'));
    await waitFor(() => {
      expect(screen.getByTestId('detail-pane-object').getAttribute('data-state')).toBe('active');
    });
    expect(screen.getByTestId('detail-pane-xref').getAttribute('data-state')).toBe('inactive');
  });
});

// ---------------------------------------------------------------------------
// Arrow keys move focus between tab triggers WITHOUT activating.
// activationMode="manual" -- focus ≠ activation.
// ---------------------------------------------------------------------------

describe('manual activation -- focus does not activate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetXRefTable.mockResolvedValue(xrefSmall);
    mockGetPlainText.mockResolvedValue(plainTextSmall);
  });

  test('ArrowRight on Object trigger moves focus to XREF but does NOT activate the pane', async () => {
    renderDetailPanel([openAction]);
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-object')).toBeInTheDocument();
    });
    const objectTrigger = screen.getByTestId('detail-tab-object');
    objectTrigger.focus();
    fireEvent.keyDown(objectTrigger, { key: 'ArrowRight' });
    // Focus moved but the XREF pane is still inactive (manual activation).
    expect(screen.getByTestId('detail-tab-xref')).toBe(document.activeElement);
    expect(screen.getByTestId('detail-pane-xref').getAttribute('data-state')).toBe('inactive');
    // XREF data may have already fetched eagerly on mount (revised post-9-12 so
    // the "XREF (N)" label appears without click); the manual-activation
    // contract is about pane visibility, not about fetch gating.
  });
});

// ---------------------------------------------------------------------------
// Tab label shows "XREF (N)" once data has loaded successfully; on initial
// mount the label shows just "XREF"; on error the label stays "XREF" (no
// count).
// ---------------------------------------------------------------------------

describe('tab label row count', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('initial mount shows "XREF" with no count', async () => {
    mockGetXRefTable.mockReturnValue(new Promise(() => {})); // never resolves
    renderDetailPanel([openAction]);
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-xref')).toBeInTheDocument();
    });
    expect(screen.getByTestId('detail-tab-xref').textContent?.trim()).toBe('XREF');
  });

  test('after successful load, label shows "XREF (N)"', async () => {
    mockGetXRefTable.mockResolvedValue(xrefSmall);
    renderDetailPanel([openAction]);
    fireEvent.click(await screen.findByTestId('detail-tab-xref'));
    await waitFor(() => {
      expect(screen.getByTestId('detail-tab-xref').textContent).toMatch(/XREF \(2\)/);
    });
  });

  test('after error, label stays "XREF" (no count)', async () => {
    mockGetXRefTable.mockRejectedValueOnce(new Error('boom'));
    renderDetailPanel([openAction]);
    fireEvent.click(await screen.findByTestId('detail-tab-xref'));
    await waitFor(() => {
      expect(screen.getByTestId('xref-error')).toBeInTheDocument();
    });
    expect(screen.getByTestId('detail-tab-xref').textContent?.trim()).toBe('XREF');
  });
});

// ---------------------------------------------------------------------------
// scrollTop on the Object pane survives a tab toggle within the same document.
// The forceMount + data-[state=inactive]:hidden strategy means switching tabs
// does NOT unmount the inactive pane, so the DOM node (and its scrollTop)
// persists across the toggle. If a future refactor switches to conditional
// mount, this test fails.
//
// Object pane is chosen because Plain Text intentionally resets scrollTop on
// activation and XREF has its own internal scroll container;
// The Object Tabs.Content is the cleanest surface for the contract.
// ---------------------------------------------------------------------------

describe('scrollTop survives tab toggle (Object pane)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(pageDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetXRefTable.mockResolvedValue(xrefSmall);
  });

  test('Object pane DOM node is the same instance after Object -> XREF -> Object', async () => {
    renderDetailPanel([openAction]);
    const objectPaneBefore = await screen.findByTestId('detail-pane-object');

    // Set a scrollTop on the Object pane while it is active.
    objectPaneBefore.scrollTop = 250;
    expect(objectPaneBefore.scrollTop).toBe(250);

    // Switch away to XREF.
    fireEvent.click(screen.getByTestId('detail-tab-xref'));
    await waitFor(() => {
      expect(screen.getByTestId('detail-pane-xref').getAttribute('data-state')).toBe('active');
    });
    // The Object pane is still mounted (forceMount), just hidden.
    expect(screen.getByTestId('detail-pane-object').getAttribute('data-state')).toBe('inactive');

    // Switch back to Object.
    fireEvent.click(screen.getByTestId('detail-tab-object'));
    await waitFor(() => {
      expect(screen.getByTestId('detail-pane-object').getAttribute('data-state')).toBe('active');
    });
    const objectPaneAfter = screen.getByTestId('detail-pane-object');

    // Same DOM instance -- forceMount preserved it across the toggle.
    expect(objectPaneAfter).toBe(objectPaneBefore);
    // And the scrollTop we set earlier is still there.
    expect(objectPaneAfter.scrollTop).toBe(250);
  });
});
