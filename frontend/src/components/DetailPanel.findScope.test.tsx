/**
 * Find scoping across DetailPanel tabs.
 *
 * Model B: find always searches ONLY the active tab. These cases cover the
 * anti-merge guarantee (a query in one tab never surfaces or counts matches
 * from another), re-scoping on tab switch, per-tab state persistence across an
 * inner-tab switch, and the force-mount multi-instance DOM correctness (no
 * duplicate element id, per-instance non-Latin-1 hint).
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.findScope.test.tsx
 */
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { AppProvider, useAppDispatch, type AppAction } from '../hooks/useDocumentState';
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

const mockGetObjectDetail = vi.fn();
const mockGetXRefTable = vi.fn();
const mockGetPlainText = vi.fn();
const mockGetPlainTextSize = vi.fn();
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
    GetContentStream: vi.fn(),
    GetImageData: vi.fn(),
    DescribeImage: vi.fn().mockResolvedValue({ width: 0, height: 0, colorSpace: '', estimatedBytes: 0 }),
    SaveImageToFile: vi.fn().mockResolvedValue(''),
    GetReverseRefs: (...args: unknown[]) => mockGetReverseRefs(...args),
    GetFontView: vi.fn(),
    GetXRefTable: (...args: unknown[]) => mockGetXRefTable(...args),
    GetPlainText: (...args: unknown[]) => mockGetPlainText(...args),
    GetPlainTextSize: (...args: unknown[]) => mockGetPlainTextSize(...args),
    GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
    GetSignatures: vi.fn().mockResolvedValue([]),
    GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
    GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
    SaveBytesToFile: vi.fn().mockResolvedValue(''),
    DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  }),
);

const catalogNode = {
  id: 'root', label: 'Catalog', rawKey: '', nodeType: 'dict', valueType: '',
  hasChildren: true, childCount: 1, iconHint: 'catalog', error: '',
};

// Each tab owns a token that appears in NO other tab, so cross-contamination is
// detectable.
const dictDetail = {
  nodeId: 'obj:0:3', objectRef: '3 0 R', type: 'dict',
  properties: [
    { key: '/Type', value: { type: 'name', display: '/Page', raw: '/Page', refTarget: '' } },
    { key: '/Marker', value: { type: 'string', display: 'objectonly-needle', raw: '', refTarget: '' } },
    // A non-Latin-1 value so the Object tab's hint has real subject matter.
    { key: '/Title', value: { type: 'string', display: 'π-title', raw: '', refTarget: '' } },
  ],
  elements: [], scalarValue: null, streamInfo: null,
};
const xrefData = {
  tabId: 'tab-1',
  entries: [{ objNum: 1, gen: 0, status: 'in-use', offset: 4242, hostObjStm: 0, nodeID: 'obj:0:1' }],
};
const plainText = { tabId: 'tab-1', content: 'plainonly-needle here\nsecond line\n', totalBytes: 34 };

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: { tabId: 'tab-1', fileName: 'test.pdf', filePath: '/test.pdf', rootNode: catalogNode, rootChildren: [] },
};
const selectAction: AppAction = { type: 'SELECT_NODE', payload: { nodeId: 'obj:0:3' } };

function Dispatch({ action }: { action: AppAction }) {
  useAppDispatch()(action);
  return null;
}

function forceMacPlatform() {
  const original = Object.getOwnPropertyDescriptor(window.navigator, 'platform');
  Object.defineProperty(window.navigator, 'platform', { configurable: true, get: () => 'MacIntel' });
  return () => { if (original) Object.defineProperty(window.navigator, 'platform', original); };
}

function cmdF() {
  act(() => {
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', metaKey: true, bubbles: true, cancelable: true }));
  });
}

function renderPanel() {
  return render(
    <AppProvider>
      <Dispatch action={openAction} />
      <Dispatch action={selectAction} />
      <DetailPanel />
    </AppProvider>,
  );
}

let restore: () => void;
beforeEach(() => {
  vi.clearAllMocks();
  mockGetObjectDetail.mockResolvedValue(dictDetail);
  mockGetReverseRefs.mockResolvedValue([]);
  mockGetXRefTable.mockResolvedValue(xrefData);
  mockGetPlainText.mockResolvedValue(plainText);
  mockGetPlainTextSize.mockResolvedValue(plainText.totalBytes);
  restore = forceMacPlatform();
});
afterEach(() => restore());

describe('no cross-tab result merging', () => {
  test('a Plain Text token yields zero matches when searched in the XREF tab', async () => {
    renderPanel();
    // Open XREF and search a token that only exists in the Plain Text corpus.
    fireEvent.click(await screen.findByTestId('detail-tab-xref'));
    await waitFor(() => expect(screen.getByTestId('xref-row-1')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('xref-find-input'), { target: { value: 'plainonly-needle' } });
    await waitFor(() => expect(screen.getByTestId('xref-find-count').textContent).toBe('0 of 0'));
    expect(screen.queryAllByTestId('xref-find-match')).toHaveLength(0);
    // And no Plain Text marks were produced as a side effect.
    expect(screen.queryAllByTestId('plain-text-find-match')).toHaveLength(0);
  });

  test('an XREF token yields zero matches when searched in the Object tab', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: '4242' } });
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('0 of 0'));
    expect(screen.queryAllByTestId('object-find-match')).toHaveLength(0);
  });
});

describe('find re-scopes on tab switch', () => {
  test('the same token count changes as the active tab changes', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    // Object tab: "objectonly-needle" matches here.
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 1'));

    // Switch to XREF and search the XREF-only token: it matches there.
    fireEvent.click(screen.getByTestId('detail-tab-xref'));
    await waitFor(() => expect(screen.getByTestId('xref-row-1')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('xref-find-input'), { target: { value: '4242' } });
    await waitFor(() => expect(screen.getByTestId('xref-find-count').textContent).toBe('1 of 1'));
  });
});

describe('per-tab find state persists across an inner-tab switch', () => {
  test('Object find query survives Object -> XREF -> Object with no reset', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 1'));

    fireEvent.click(screen.getByTestId('detail-tab-xref'));
    await waitFor(() => expect(screen.getByTestId('detail-pane-xref').getAttribute('data-state')).toBe('active'));
    fireEvent.click(screen.getByTestId('detail-tab-object'));
    await waitFor(() => expect(screen.getByTestId('detail-pane-object').getAttribute('data-state')).toBe('active'));

    const input = screen.getByTestId('object-find-input') as HTMLInputElement;
    expect(input.value).toBe('objectonly-needle');
    expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 1');
  });
});

describe('force-mount multi-instance DOM correctness', () => {
  test('two find bars can be open at once with unique testids and no duplicate element id', async () => {
    renderPanel();
    // Open Plain Text find with a non-Latin-1 query so its hint renders.
    fireEvent.click(await screen.findByTestId('detail-tab-plaintext'));
    await waitFor(() => expect(screen.getByTestId('detail-pane-plaintext').getAttribute('data-state')).toBe('active'));
    cmdF();
    fireEvent.change(screen.getByTestId('plain-text-find-input'), { target: { value: 'π' } });
    expect(screen.getByTestId('plain-text-find-non-latin1-hint')).toBeInTheDocument();

    // Switch to Object; the Plain Text bar stays mounted (forceMount + open
    // persists). Open the Object bar too, with its own non-Latin-1 query.
    fireEvent.click(screen.getByTestId('detail-tab-object'));
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'π' } });

    // Both bars mounted, each addressable by its own testid.
    expect(screen.getByTestId('plain-text-find-bar')).toBeInTheDocument();
    expect(screen.getByTestId('object-find-bar')).toBeInTheDocument();

    // The non-Latin-1 hint id is per-instance: no element id appears twice.
    const ids = Array.from(document.querySelectorAll('[id]')).map((el) => el.id);
    const dupes = ids.filter((id, i) => id && ids.indexOf(id) !== i);
    expect(dupes).toEqual([]);
  });
});
