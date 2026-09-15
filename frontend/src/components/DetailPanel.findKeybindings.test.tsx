/**
 * Find keybinding coexistence.
 *
 * Cmd+F opens find in the active tab only, opening exactly one bar (no
 * double-open from a second bound listener), and the find machinery leaves
 * Cmd+G (Go-to-Page) and Cmd+K (command palette) alone -- it consumes only
 * Cmd+F / F3.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.findKeybindings.test.tsx
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
const dictDetail = {
  nodeId: 'obj:0:3', objectRef: '3 0 R', type: 'dict',
  properties: [{ key: '/Marker', value: { type: 'string', display: 'objectonly-needle', raw: '', refTarget: '' } }],
  elements: [], scalarValue: null, streamInfo: null,
};
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

function dispatchKey(opts: { key: string; metaKey?: boolean }): KeyboardEvent {
  const ev = new KeyboardEvent('keydown', {
    key: opts.key, metaKey: opts.metaKey ?? false, bubbles: true, cancelable: true,
  });
  act(() => { window.dispatchEvent(ev); });
  return ev;
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
  mockGetXRefTable.mockResolvedValue({ tabId: 'tab-1', entries: [] });
  mockGetPlainText.mockResolvedValue({ tabId: 'tab-1', content: 'x\n', totalBytes: 2 });
  mockGetPlainTextSize.mockResolvedValue(2);
  restore = forceMacPlatform();
});
afterEach(() => restore());

describe('Cmd+F opens exactly one bar in the active tab', () => {
  test('Cmd+F on the Object tab opens a single object-find-bar', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    dispatchKey({ key: 'f', metaKey: true });
    expect(screen.queryAllByTestId('object-find-bar')).toHaveLength(1);
  });

  test('Cmd+F on the Object tab does not open the Plain Text find bar', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    dispatchKey({ key: 'f', metaKey: true });
    expect(screen.queryByTestId('plain-text-find-bar')).toBeNull();
  });
});

describe('find leaves Cmd+G and Cmd+K to their owners', () => {
  test('with the Object find bar open, Cmd+G is not consumed by find', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    dispatchKey({ key: 'f', metaKey: true });
    expect(screen.getByTestId('object-find-bar')).toBeInTheDocument();
    const ev = dispatchKey({ key: 'g', metaKey: true });
    expect(ev.defaultPrevented).toBe(false);
    // The find bar stays open; Cmd+G does not close or hijack it.
    expect(screen.getByTestId('object-find-bar')).toBeInTheDocument();
  });

  test('with the Object find bar open, Cmd+K is not consumed by find', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    dispatchKey({ key: 'f', metaKey: true });
    expect(screen.getByTestId('object-find-bar')).toBeInTheDocument();
    const ev = dispatchKey({ key: 'k', metaKey: true });
    expect(ev.defaultPrevented).toBe(false);
  });
});
