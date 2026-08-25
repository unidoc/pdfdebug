/**
 * Font Inspection View -- DetailPanel integration tests.
 *
 * Updated for the unified GetFontView endpoint (replaces the prior
 * GetFontDetail + GetFontResourceMap two-call cascade). The backend now
 * disambiguates server-side via FontView.Kind so the frontend issues exactly
 * one call per iconHint='font' click and the binding layer never logs ERR on
 * the /Resources /Font false-positive path.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.fontPreview.test.tsx
 */
import { render, screen, waitFor, act, within } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
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
const mockGetFontView = vi.fn();
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

const fontTreeNode = {
  id: 'obj:0:5',
  label: 'Font',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 1,
  iconHint: 'font',
  error: '',
};

const fontDictDetail = {
  nodeId: 'obj:0:5',
  objectRef: '5 0 R',
  type: 'dict',
  properties: [
    {
      key: '/Type',
      value: { type: 'name', display: '/Font', raw: '/Font', refTarget: '' },
    },
    {
      key: '/BaseFont',
      value: {
        type: 'name',
        display: '/Helvetica-Bold',
        raw: '/Helvetica-Bold',
        refTarget: '',
      },
    },
  ],
  elements: [],
  scalarValue: null,
  streamInfo: null,
};

const populatedFontDetail = {
  nodeId: 'obj:0:5',
  objectRef: '5 0 R',
  subtype: 'TrueType',
  baseFont: '/Helvetica-Bold',
  firstChar: 32,
  lastChar: 126,
  encodingName: '/WinAnsiEncoding',
  baseEncoding: '',
  differences: [],
  toUnicodeMappings: [],
  toUnicodeError: '',
  embedded: true,
  fontDescriptor: {
    nodeId: 'obj:0:10',
    objectRef: '10 0 R',
    fontName: 'Helvetica-Bold',
    flags: 32,
    flagNames: ['Nonsymbolic'],
    italicAngle: 0,
    ascent: 718,
    descent: -207,
    capHeight: 718,
    stemV: 140,
    fontBBox: [-170, -228, 1003, 962],
    fontFileFormat: 'TrueType',
    fontFileSize: 14592,
  },
  descendant: null,
  cidSystemInfo: null,
  cidToGIDMap: '',
  defaultWidth: 0,
};

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
    rootNode: fontTreeNode,
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
// iconHint='font' + dict-type triggers FontPreview in place of DictView.
// GetFontView is fetched and returns Kind:'detail'.
// ---------------------------------------------------------------------------

describe('iconHint=font swaps DictView -> FontPreview', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetFontView.mockResolvedValue({ kind: 'detail', detail: populatedFontDetail, roster: null });
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('GetFontView is called with (tabID, nodeID)', async () => {
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      expect(mockGetFontView).toHaveBeenCalledWith('tab-1', 'obj:0:5');
    });
  });

  test('FontPreview renders BaseFont; DictView /Type row does NOT render', async () => {
    renderDetailPanelFor('obj:0:5', 'font');
    // FontPreview output -- the BaseFont string surfaces.
    await waitFor(() => {
      expect(
        screen.getAllByText('/Helvetica-Bold').length
      ).toBeGreaterThan(0);
    });
    // The generic DictView's `/Type` row MUST NOT render -- it would
    // indicate the swap did not happen (DictView is rendered instead of /
    // alongside FontPreview, which contradicts the swap contract).
    expect(screen.queryByText('/Type')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Fallback -- Kind:'neither' silently renders the generic DictView. No
// error is involved in this path.
// ---------------------------------------------------------------------------

describe('Kind=neither silent DictView fallback', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('Kind:neither renders the generic DictView (not FontPreview)', async () => {
    mockGetFontView.mockResolvedValue({ kind: 'neither', detail: null, roster: null });
    renderDetailPanelFor('obj:0:5', 'font');
    // DictView's /Type row renders because we silently fall back.
    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });
    // No font-specific UI: no font-embedding badge, no font sections. Scope the
    // query to the Object pane so it does NOT match the "Embedded"
    // tab trigger (a document-level tab, not font UI).
    const objectPane = screen.getByTestId('detail-pane-object');
    expect(within(objectPane).queryByText(/Embedded/)).not.toBeInTheDocument();
    expect(within(objectPane).queryByText(/Not embedded/)).not.toBeInTheDocument();
  });

  test('Kind:neither path does NOT surface a generic error banner', async () => {
    mockGetFontView.mockResolvedValue({ kind: 'neither', detail: null, roster: null });
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });
    // Contract: silent fallback. No error banner.
    expect(
      screen.queryByText(/Failed to load font detail/i)
    ).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// /Resources /Font roster view replaces silent DictView when GetFontView
// returns Kind:'roster'.
// ---------------------------------------------------------------------------

describe('Kind=roster renders FontRosterPreview', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('roster renders when GetFontView returns Kind:roster', async () => {
    mockGetFontView.mockResolvedValue({
      kind: 'roster',
      detail: null,
      roster: {
        nodeId: 'dict:dict:obj:0:3:Resources:Font',
        entries: [
          {
            name: 'F1',
            nodeId: 'obj:0:4',
            objectRef: '4 0 R',
            baseFont: '/Helvetica',
            subtype: 'Type1',
            encodingSummary: '/WinAnsiEncoding',
            embedded: false,
            unresolved: false,
          },
        ],
      },
    });
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      expect(screen.getByTestId('font-roster-preview')).toBeInTheDocument();
    });
  });

  test('Kind:neither falls through to DictView silently (no roster mount)', async () => {
    mockGetFontView.mockResolvedValue({ kind: 'neither', detail: null, roster: null });
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('font-roster-preview')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Real backend error surfaces an inline error message inside the dict-view
// slot (does NOT blank the panel). With the unified endpoint, .catch only
// fires for genuine failures (malformed PDF, unknown tab, pdfcpu panics).
// ---------------------------------------------------------------------------

describe('real error renders inline (does not crash)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('GetFontView rejection renders an inline error message', async () => {
    mockGetFontView.mockRejectedValue(new Error('boom from pdfcpu'));
    renderDetailPanelFor('obj:0:5', 'font');
    // The error string surfaces somewhere visible (we accept any container
    // that includes the message). The panel MUST NOT be blank.
    await waitFor(() => {
      expect(screen.getByText(/boom from pdfcpu/)).toBeInTheDocument();
    });
  });

  test('Wails envelope unwraps real-error message and renders inline', async () => {
    const envelope = JSON.stringify({
      message: 'boom from pdfcpu',
      cause: {},
      kind: 'RuntimeError',
    });
    mockGetFontView.mockRejectedValue(new Error(envelope));
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      expect(screen.getByText(/boom from pdfcpu/)).toBeInTheDocument();
    });
    // The JSON wrapper itself must not leak into the UI.
    expect(screen.queryByText(/RuntimeError/)).not.toBeInTheDocument();
  });

  test('iconHint=font with a real error does NOT crash the panel', async () => {
    mockGetFontView.mockRejectedValue(new Error('boom from pdfcpu'));
    renderDetailPanelFor('obj:0:5', 'font');
    // detail-panel root must still exist (the React error boundary did not
    // unmount the panel).
    await waitFor(() => {
      expect(screen.getByTestId('detail-panel')).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// 200ms-debounced loading indicator with data-testid "font-loading"
// ---------------------------------------------------------------------------

describe('200ms-debounced loading indicator', () => {
  // Manually-resolvable promise so the never-resolved state doesn't leak past
  // test teardown (which can trigger unhandled-rejection worker crashes).
  let resolveFont: ((v: unknown) => void) | null = null;

  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetFontView.mockImplementation(
      () => new Promise((res) => { resolveFont = res; })
    );
  });

  afterEach(() => {
    // Resolve any in-flight promise so React effect cleanup completes without
    // leaving pending microtasks at teardown.
    if (resolveFont) {
      resolveFont({ kind: 'detail', detail: populatedFontDetail, roster: null });
      resolveFont = null;
    }
    vi.useRealTimers();
  });

  test('loading indicator does NOT appear before 200ms have elapsed', async () => {
    renderDetailPanelFor('obj:0:5', 'font');
    await act(async () => {
      vi.advanceTimersByTime(150);
    });
    expect(screen.queryByTestId('font-loading')).not.toBeInTheDocument();
  });

  test('loading indicator appears at >= 200ms when fetch is still pending', async () => {
    renderDetailPanelFor('obj:0:5', 'font');
    await act(async () => {
      vi.advanceTimersByTime(250);
    });
    expect(screen.getByTestId('font-loading')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Negative -- iconHint='font' but detail.type !== 'dict' (impossible in
// practice, but defensive). The fetch MUST be skipped because the dict-type
// guard fails.
// ---------------------------------------------------------------------------

describe('iconHint=font + non-dict detail does NOT fetch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetReverseRefs.mockResolvedValue([]);
    // Stream-typed details trigger DetailPanel's GetContentStream effect; mock
    // a benign response so the effect resolves without crashing the test
    // worker (binding mock would otherwise return undefined and .then() blows
    // up).
    mockGetContentStream.mockResolvedValue({
      nodeId: 'obj:0:5',
      raw: '',
      tokenized: [],
      formatted: [],
      error: '',
    });
  });

  test('GetFontView is NOT called when detail.type is "stream"', async () => {
    mockGetObjectDetail.mockResolvedValue({
      ...fontDictDetail,
      type: 'stream',
      streamInfo: { filter: 'FlateDecode', length: 100 },
    });
    renderDetailPanelFor('obj:0:5', 'font');
    // Wait long enough for the detail fetch to settle.
    await waitFor(() => {
      expect(mockGetObjectDetail).toHaveBeenCalled();
    });
    // The font branch is dict-typed only; a stream-typed detail must not
    // call GetFontView.
    expect(mockGetFontView).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Negative -- iconHint is null / not 'font' -> no fetch. Regression guard: a
// missing branch guard would fire GetFontView on every dict selection,
// ballooning IPC traffic.
// ---------------------------------------------------------------------------

describe('iconHint != "font" does NOT trigger GetFontView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('iconHint=null does not fetch font view', async () => {
    renderDetailPanelFor('obj:0:5', null);
    await waitFor(() => {
      expect(mockGetObjectDetail).toHaveBeenCalled();
    });
    expect(mockGetFontView).not.toHaveBeenCalled();
  });

  test('iconHint="image" does not fetch font view', async () => {
    renderDetailPanelFor('obj:0:5', 'image');
    await waitFor(() => {
      expect(mockGetObjectDetail).toHaveBeenCalled();
    });
    expect(mockGetFontView).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Header label reads "Font - <BaseFont>" when FontPreview is active.
// Falls back to "Font" when BaseFont missing.
// ---------------------------------------------------------------------------

describe('detail-panel header label', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('header label includes "Font - /Helvetica-Bold"', async () => {
    mockGetFontView.mockResolvedValue({ kind: 'detail', detail: populatedFontDetail, roster: null });
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header.textContent).toContain('Font');
      expect(header.textContent).toContain('/Helvetica-Bold');
    });
  });

  test('header label is "Font" when BaseFont is empty', async () => {
    mockGetFontView.mockResolvedValue({
      kind: 'detail',
      detail: { ...populatedFontDetail, baseFont: '' },
      roster: null,
    });
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header.textContent).toMatch(/Font/);
    });
  });
});

// ---------------------------------------------------------------------------
// A font dict reached through an IndirectRef chain or packaged in ObjStm uses
// the same code path (resolveNodeObject already handles both backend-side).
// The frontend just calls GetFontDetail; this test pins that no special
// handling exists for the indirect case.
// ---------------------------------------------------------------------------

describe('indirect-ref-chain / ObjStm transparent', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetFontView.mockResolvedValue({ kind: 'detail', detail: populatedFontDetail, roster: null });
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('caller passes nodeId once; FontPreview renders with one fetch', async () => {
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      expect(mockGetFontView).toHaveBeenCalledTimes(1);
    });
    // No retry, no second fetch. Indirect-ref resolution is satisfied entirely
    // at the backend through resolveNodeObject -- the frontend MUST stay dumb.
    expect(mockGetFontView).toHaveBeenCalledWith('tab-1', 'obj:0:5');
  });
});
