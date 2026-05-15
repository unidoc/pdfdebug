/**
 * Story 9.9: Font Inspection View -- DetailPanel integration tests
 *
 * TDD RED PHASE: Tests MUST fail until Task 5 wires FontPreview into
 * DetailPanel: the iconHint='font' branch with GetFontDetail fetch, the
 * fontLoading/showFontLoading state pair with 200ms debounce (AC#9), the
 * ErrNotAFont silent DictView fallback (AC#1), and the "Font - <BaseFont>"
 * header label (AC#11).
 *
 * Standalone file (mirrors DetailPanel.reverseRefs.test.tsx pattern from
 * Story 9-10) to avoid splicing into the 1678-line DetailPanel.test.tsx.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.fontPreview.test.tsx
 */
import { render, screen, waitFor, act } from '@testing-library/react';
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
// RED PHASE: GetFontDetail does not exist in the bindings yet. The mock
// surface anticipates Task 3.2's regenerated bindings.
const mockGetFontDetail = vi.fn();
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
    GetFontDetail: (...args: unknown[]) => mockGetFontDetail(...args),
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
// 9.9-UNIT-201 [P0] AC#1 -- iconHint='font' + dict-type triggers FontPreview
// in place of DictView. GetFontDetail is fetched.
// ---------------------------------------------------------------------------

describe('9.9-UNIT-201: iconHint=font swaps DictView -> FontPreview', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetFontDetail.mockResolvedValue(populatedFontDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('GetFontDetail is called with (tabID, nodeID)', async () => {
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      expect(mockGetFontDetail).toHaveBeenCalledWith('tab-1', 'obj:0:5');
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
    // alongside FontPreview, which contradicts AC#1).
    expect(screen.queryByText('/Type')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 9.9-UNIT-202 [P0] AC#1 fallback -- ErrNotAFont sentinel silently renders
// generic DictView. The frontend matches the sentinel via wails-rejected
// error message; the canonical wording is "not a font" (parallel to the
// "reverse-ref index unavailable" pattern from Story 9-10).
// ---------------------------------------------------------------------------

describe('9.9-UNIT-202: ErrNotAFont silent DictView fallback', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('ErrNotAFont rejection renders the generic DictView (not FontPreview)', async () => {
    mockGetFontDetail.mockRejectedValue(new Error('not a font'));
    renderDetailPanelFor('obj:0:5', 'font');
    // DictView's /Type row renders because we silently fall back.
    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });
    // No font-specific UI: no Embedded badge, no font sections.
    expect(screen.queryByText(/Embedded/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Not embedded/)).not.toBeInTheDocument();
  });

  test('ErrNotAFont path does NOT surface a generic error banner', async () => {
    mockGetFontDetail.mockRejectedValue(new Error('not a font'));
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      expect(screen.getByText('/Type')).toBeInTheDocument();
    });
    // AC#1 contract: silent fallback. No error banner.
    expect(
      screen.queryByText(/Failed to load font detail/i)
    ).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 9.9-UNIT-203 [P0] AC#9 -- non-sentinel error surfaces an inline error
// message inside the dict-view slot (does NOT blank the panel).
// ---------------------------------------------------------------------------

describe('9.9-UNIT-203: non-sentinel error renders inline (does not crash)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('generic GetFontDetail rejection renders an inline error message', async () => {
    mockGetFontDetail.mockRejectedValue(new Error('boom from pdfcpu'));
    renderDetailPanelFor('obj:0:5', 'font');
    // The error string surfaces somewhere visible (we accept any container
    // that includes the message). The panel MUST NOT be blank.
    await waitFor(() => {
      expect(screen.getByText(/boom from pdfcpu/)).toBeInTheDocument();
    });
  });

  test('iconHint=font with non-sentinel error does NOT crash the panel', async () => {
    mockGetFontDetail.mockRejectedValue(new Error('boom from pdfcpu'));
    renderDetailPanelFor('obj:0:5', 'font');
    // detail-panel root must still exist (the React error boundary did not
    // unmount the panel).
    await waitFor(() => {
      expect(screen.getByTestId('detail-panel')).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// 9.9-UNIT-204 [P0] AC#9 -- 200ms-debounced loading indicator with data-testid
// "font-loading"
// ---------------------------------------------------------------------------

describe('9.9-UNIT-204: AC#9 200ms-debounced loading indicator', () => {
  // Manually-resolvable promise so the never-resolved state doesn't leak past
  // test teardown (which can trigger unhandled-rejection worker crashes).
  let resolveFont: ((v: unknown) => void) | null = null;

  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockGetFontDetail.mockImplementation(
      () => new Promise((res) => { resolveFont = res; })
    );
  });

  afterEach(() => {
    // Resolve any in-flight promise so React effect cleanup completes without
    // leaving pending microtasks at teardown.
    if (resolveFont) {
      resolveFont(populatedFontDetail);
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
// 9.9-UNIT-205 [P0] AC#1 negative -- iconHint='font' but detail.type !== 'dict'
// (impossible in practice, but defensive). The fetch MUST be skipped because
// the dict-type guard fails.
// ---------------------------------------------------------------------------

describe('9.9-UNIT-205: iconHint=font + non-dict detail does NOT fetch', () => {
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

  test('GetFontDetail is NOT called when detail.type is "stream"', async () => {
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
    // call GetFontDetail.
    expect(mockGetFontDetail).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 9.9-UNIT-206 [P0] AC#1 negative -- iconHint is null / not 'font' -> no fetch.
// Regression guard: a missing branch guard would fire GetFontDetail on every
// dict selection, ballooning IPC traffic.
// ---------------------------------------------------------------------------

describe('9.9-UNIT-206: iconHint != "font" does NOT trigger GetFontDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('iconHint=null does not fetch font detail', async () => {
    renderDetailPanelFor('obj:0:5', null);
    await waitFor(() => {
      expect(mockGetObjectDetail).toHaveBeenCalled();
    });
    expect(mockGetFontDetail).not.toHaveBeenCalled();
  });

  test('iconHint="image" does not fetch font detail', async () => {
    renderDetailPanelFor('obj:0:5', 'image');
    await waitFor(() => {
      expect(mockGetObjectDetail).toHaveBeenCalled();
    });
    expect(mockGetFontDetail).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 9.9-UNIT-207 [P1] AC#11 -- header label reads "Font - <BaseFont>" when
// FontPreview is active. Falls back to "Font" when BaseFont missing.
// ---------------------------------------------------------------------------

describe('9.9-UNIT-207: AC#11 detail-panel header label', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('header label includes "Font - /Helvetica-Bold"', async () => {
    mockGetFontDetail.mockResolvedValue(populatedFontDetail);
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header.textContent).toContain('Font');
      expect(header.textContent).toContain('/Helvetica-Bold');
    });
  });

  test('header label is "Font" when BaseFont is empty', async () => {
    mockGetFontDetail.mockResolvedValue({
      ...populatedFontDetail,
      baseFont: '',
    });
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      const header = screen.getByTestId('detail-panel-header');
      expect(header.textContent).toMatch(/Font/);
    });
  });
});

// ---------------------------------------------------------------------------
// 9.9-UNIT-208 [P1] AC#10 -- a font dict reached through an IndirectRef chain
// or packaged in ObjStm uses the same code path (resolveNodeObject already
// handles both backend-side). The frontend just calls GetFontDetail; this
// test pins that no special handling exists for the indirect case.
// ---------------------------------------------------------------------------

describe('9.9-UNIT-208: AC#10 indirect-ref-chain / ObjStm transparent', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(fontDictDetail);
    mockGetFontDetail.mockResolvedValue(populatedFontDetail);
    mockGetReverseRefs.mockResolvedValue([]);
  });

  test('caller passes nodeId once; FontPreview renders without extra fetches', async () => {
    renderDetailPanelFor('obj:0:5', 'font');
    await waitFor(() => {
      expect(mockGetFontDetail).toHaveBeenCalledTimes(1);
    });
    // No retry, no second fetch, no fallback fetch on a "looks like an
    // indirect ref" guess. AC#10 is satisfied entirely at the backend
    // through resolveNodeObject -- the frontend MUST stay dumb.
    expect(mockGetFontDetail).toHaveBeenCalledWith('tab-1', 'obj:0:5');
  });
});
