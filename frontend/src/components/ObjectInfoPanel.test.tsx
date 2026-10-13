/**
 * Object Source View + Reverse References
 *
 * Covers Object Source view contract and the mapping that is the
 * highest-leverage place to catch a regression: `5 0 R` -> nodeID `obj:0:5`
 * (capture-1 = num, capture-2 = gen). Swapping them dispatches silently
 * wrong navigation.
 *
 * The original test file is replaced. The file path stays
 * the same to minimise import churn elsewhere.
 *
 * Run: cd frontend && npx vitest run src/components/ObjectInfoPanel.test.tsx
 */
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  useAppState,
  type AppAction,
} from '../hooks/useDocumentState';
import { ObjectSourcePanel } from './ObjectInfoPanel';

// Mock allotment -- jsdom has no layout APIs.
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

// Mock Wails bindings -- the new fetcher is GetObjectSource, not
// GetObjectDetail. (The old binding remains for other components.)
const mockGetObjectSource = vi.fn();
const mockGetObjectDetail = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: (...args: unknown[]) => mockGetObjectDetail(...args),
    GetObjectSource: (...args: unknown[]) => mockGetObjectSource(...args),
  })
);

// --- Test data ---

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

// Example source strings for the array, dict and stream cases below.
const shortArraySource = `38109 0 obj
[ 38110 0 R 38111 0 R 38112 0 R ]
endobj`;

const dictSource = `4 0 obj
<<
    /Type /Pages
    /Kids [5 0 R 6 0 R 7 0 R]
    /Count 3
>>
endobj`;

const streamSource = `12 0 obj
<< /Length 12345 /Filter /FlateDecode >>
stream
[12,345 bytes -- see Content Stream tab for decoded view]
endstream
endobj`;

// --- Helpers ---

function DispatchHelper({ action }: { action: AppAction }) {
  const dispatch = useAppDispatch();
  dispatch(action);
  return null;
}

function StateReader() {
  const state = useAppState();
  const activeTab = state.tabs.find((t) => t.tabId === state.activeTabId);
  return (
    <span data-testid="pending-nav-target">
      {String(activeTab?.pendingNavTarget ?? '')}
    </span>
  );
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

function renderPanel(selectedNodeId: string | null, extra?: React.ReactNode) {
  return render(
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
      <ObjectSourcePanel />
      {extra}
    </AppProvider>
  );
}

// ---------------------------------------------------------------------------
// Empty state when no node is selected
// ---------------------------------------------------------------------------

describe('no-selection empty state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('renders "Select an object to view its source." when nothing selected', () => {
    renderPanel(null);
    expect(
      screen.getByText('Select an object to view its source.')
    ).toBeInTheDocument();
  });

  test('panel container uses the new data-testid object-source-panel', () => {
    renderPanel(null);
    expect(screen.getByTestId('object-source-panel')).toBeInTheDocument();
    // The old testid must be retired
    expect(screen.queryByTestId('object-info-panel')).not.toBeInTheDocument();
  });

  test('panel title is "Object Source"', () => {
    renderPanel(null);
    expect(screen.getByText('Object Source')).toBeInTheDocument();
  });

  test('GetObjectSource is NOT called when nothing is selected', () => {
    renderPanel(null);
    expect(mockGetObjectSource).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Empty state for non-indirect (inline) selections
// ---------------------------------------------------------------------------

describe('inline-node empty state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('backend returning empty string renders the inline placeholder', async () => {
    mockGetObjectSource.mockResolvedValue('');
    renderPanel('dict:obj:0:5:Type');
    await waitFor(() => {
      expect(
        screen.getByText(/Inline object/i)
      ).toBeInTheDocument();
    });
  });

  test('inline empty state does NOT show the no-selection copy', async () => {
    mockGetObjectSource.mockResolvedValue('');
    renderPanel('dict:obj:0:5:Type');
    await waitFor(() => {
      expect(
        screen.queryByText('Select an object to view its source.')
      ).not.toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// Reserialized PDF syntax rendered in monospace
// ---------------------------------------------------------------------------

describe('source rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('fetches GetObjectSource with the active tab id and selected node id', async () => {
    mockGetObjectSource.mockResolvedValue(shortArraySource);
    renderPanel('obj:0:38109');
    await waitFor(() => {
      expect(mockGetObjectSource).toHaveBeenCalledWith('tab-1', 'obj:0:38109');
    });
  });

  test('renders the returned source text verbatim', async () => {
    mockGetObjectSource.mockResolvedValue(shortArraySource);
    renderPanel('obj:0:38109');
    await waitFor(() => {
      expect(screen.getByText(/38109 0 obj/)).toBeInTheDocument();
    });
    expect(screen.getByText(/endobj/)).toBeInTheDocument();
  });

  test('source body uses font-mono so indentation is preserved', async () => {
    mockGetObjectSource.mockResolvedValue(dictSource);
    renderPanel('obj:0:4');
    await waitFor(() => {
      const body = screen.getByTestId('object-source-body');
      expect(body.className).toMatch(/font-mono/);
    });
  });
});

// ---------------------------------------------------------------------------
// Indirect ref click dispatches NAVIGATE_TO_REF with the correct obj:gen:num
// mapping. THIS IS THE LOAD-BEARING TEST: capture 1 is num, capture 2 is gen;
// the dispatched nodeID is `obj:${gen}:${num}`. `5 0 R` -> `obj:0:5`, NOT
// `obj:5:0`.
// ---------------------------------------------------------------------------

describe('indirect-ref click mapping', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('clicking `5 0 R` dispatches NAVIGATE_TO_REF with `obj:0:5`', async () => {
    const user = userEvent.setup();
    mockGetObjectSource.mockResolvedValue(dictSource);
    renderPanel('obj:0:4', <StateReader />);

    await waitFor(() => {
      expect(screen.getByText(/5 0 R/)).toBeInTheDocument();
    });
    // The clickable span MUST carry data-ref-target with the correct mapping.
    const span = screen.getByText('5 0 R');
    expect(span).toHaveAttribute('data-ref-target', 'obj:0:5');
    expect(span).toHaveAttribute('role', 'button');
    expect(span).toHaveAttribute('tabindex', '0');

    await user.click(span);

    await waitFor(() => {
      expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:0:5');
    });
  });

  test('clicking `38110 0 R` dispatches `obj:0:38110` (multi-digit num)', async () => {
    const user = userEvent.setup();
    mockGetObjectSource.mockResolvedValue(shortArraySource);
    renderPanel('obj:0:38109', <StateReader />);

    await waitFor(() => {
      expect(screen.getByText('38110 0 R')).toBeInTheDocument();
    });
    const span = screen.getByText('38110 0 R');
    expect(span).toHaveAttribute('data-ref-target', 'obj:0:38110');

    await user.click(span);
    await waitFor(() => {
      expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:0:38110');
    });
  });

  test('clicking `7 2 R` (non-zero generation) dispatches `obj:2:7`', async () => {
    const user = userEvent.setup();
    const src = `100 0 obj\n[ 7 2 R ]\nendobj`;
    mockGetObjectSource.mockResolvedValue(src);
    renderPanel('obj:0:100', <StateReader />);

    await waitFor(() => {
      expect(screen.getByText('7 2 R')).toBeInTheDocument();
    });
    const span = screen.getByText('7 2 R');
    expect(span).toHaveAttribute('data-ref-target', 'obj:2:7');

    await user.click(span);
    await waitFor(() => {
      // Regression guard: must be obj:2:7, NOT obj:7:2.
      expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:2:7');
    });
  });

  test('Enter key on a ref dispatches NAVIGATE_TO_REF', async () => {
    const user = userEvent.setup();
    mockGetObjectSource.mockResolvedValue(dictSource);
    renderPanel('obj:0:4', <StateReader />);

    await waitFor(() => {
      expect(screen.getByText('5 0 R')).toBeInTheDocument();
    });
    const span = screen.getByText('5 0 R');
    (span as HTMLElement).focus();
    await user.keyboard('{Enter}');
    await waitFor(() => {
      expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:0:5');
    });
  });

  test('Space key on a ref dispatches NAVIGATE_TO_REF', async () => {
    const user = userEvent.setup();
    mockGetObjectSource.mockResolvedValue(dictSource);
    renderPanel('obj:0:4', <StateReader />);

    await waitFor(() => {
      expect(screen.getByText('6 0 R')).toBeInTheDocument();
    });
    const span = screen.getByText('6 0 R');
    (span as HTMLElement).focus();
    await user.keyboard(' ');
    await waitFor(() => {
      expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:0:6');
    });
  });

  test('ref span carries hyperlink styling tokens (subtle, not loud)', async () => {
    mockGetObjectSource.mockResolvedValue(dictSource);
    renderPanel('obj:0:4');
    await waitFor(() => {
      const span = screen.getByText('5 0 R');
      // Styling: cursor-pointer, hover underline, type-reference color.
      expect(span.className).toMatch(/cursor-pointer/);
      expect(span.className).toMatch(/hover:underline/);
      expect(span.className).toMatch(/text-type-reference/);
    });
  });
});

// ---------------------------------------------------------------------------
// Stream object renders placeholder, NOT clickable
// ---------------------------------------------------------------------------

describe('stream object placeholder', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('renders the dict + stream/endstream markers + byte-count line', async () => {
    mockGetObjectSource.mockResolvedValue(streamSource);
    renderPanel('obj:0:12');
    await waitFor(() => {
      expect(screen.getByText(/<< \/Length 12345 \/Filter \/FlateDecode >>/)).toBeInTheDocument();
    });
    expect(screen.getByText(/stream/)).toBeInTheDocument();
    expect(screen.getByText(/endstream/)).toBeInTheDocument();
    expect(
      screen.getByText(/\[12,345 bytes -- see Content Stream tab for decoded view\]/)
    ).toBeInTheDocument();
  });

  test('the "12,345 bytes" line is NOT wrapped as a clickable ref', async () => {
    mockGetObjectSource.mockResolvedValue(streamSource);
    renderPanel('obj:0:12');
    await waitFor(() => {
      expect(screen.getByText(/12,345 bytes/)).toBeInTheDocument();
    });
    // The placeholder line MUST NOT have role=button (i.e. the indirect-ref
    // scanner must skip it). If a future regex regression treats "12 345 R..."
    // as a ref the test catches it.
    const placeholderText = screen.getByText(/12,345 bytes -- see Content Stream tab for decoded view/);
    expect(placeholderText.closest('[role="button"]')).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Header label stays "Object Source"
// ---------------------------------------------------------------------------

describe('header label', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('"Object Source" header remains visible with a selection', async () => {
    mockGetObjectSource.mockResolvedValue(shortArraySource);
    renderPanel('obj:0:38109');
    await waitFor(() => {
      expect(screen.getByText('Object Source')).toBeInTheDocument();
    });
  });

  test('"Object Source" header remains visible with no selection', () => {
    renderPanel(null);
    expect(screen.getByText('Object Source')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Error state on fetch failure
// ---------------------------------------------------------------------------

describe('fetch error inline message', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('inline error message is shown when GetObjectSource rejects', async () => {
    mockGetObjectSource.mockRejectedValue(new Error('boom'));
    renderPanel('obj:0:5');
    await waitFor(() => {
      expect(screen.getByTestId('object-source-error')).toBeInTheDocument();
    });
  });
});
