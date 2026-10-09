/**
 * Detail Panel -- scroll position resets to the top when the selection
 * moves to a different node or to another document tab.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.scrollReset.test.tsx
 */
import { render, screen, waitFor, act } from '@testing-library/react';
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

const mockGetObjectDetail = vi.fn();
const mockGetContentStream = vi.fn();
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
    GetImageData: vi.fn(),
    DescribeImage: vi.fn(),
    SaveImageToFile: vi.fn().mockResolvedValue(''),
    GetReverseRefs: vi.fn().mockResolvedValue([]),
    GetXRefTable: vi.fn().mockResolvedValue({ tabId: '', entries: [] }),
    GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
    GetSignatures: vi.fn().mockResolvedValue([]),
    GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
    GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
    SaveBytesToFile: vi.fn().mockResolvedValue(''),
    DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  })
);

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 0,
  iconHint: 'catalog',
  error: '',
};

function streamDetail(nodeId: string) {
  return {
    nodeId,
    objectRef: '',
    type: 'stream',
    properties: [],
    elements: [],
    scalarValue: null,
    streamInfo: { length: 10, filters: [] },
  };
}

function streamData(nodeId: string, text: string) {
  return { nodeId, raw: text, tokenized: [], formatted: null, error: '' };
}

function dictDetail(nodeId: string, key: string) {
  return {
    nodeId,
    objectRef: '',
    type: 'dict',
    properties: [
      { key, value: { type: 'name', display: '/Page', raw: '/Page', refTarget: '' } },
    ],
    elements: [],
    scalarValue: null,
    streamInfo: null,
  };
}

let dispatch: (action: AppAction) => void = () => {};

function DispatchCapture() {
  dispatch = useAppDispatch();
  return null;
}

function select(nodeId: string) {
  act(() => {
    dispatch({ type: 'SELECT_NODE', payload: { nodeId } });
  });
}

function renderPanel() {
  render(
    <AppProvider>
      <DispatchCapture />
      <DetailPanel />
    </AppProvider>
  );
  act(() => {
    dispatch({
      type: 'OPEN_DOCUMENT',
      payload: {
        tabId: 'tab-1',
        fileName: 'test.pdf',
        filePath: '/path/to/test.pdf',
        rootNode: catalogNode,
        rootChildren: [],
      },
    });
  });
}

// jsdom does no layout, so scrollTop is stubbed as a plain per-element value:
// a container that is reused keeps it, a fresh one starts at 0.
function scrollDown(el: HTMLElement) {
  Object.defineProperty(el, 'scrollTop', { value: 500, writable: true, configurable: true });
}

function scrollContainerOf(testId: string): HTMLElement {
  const el = screen.getByTestId(testId).closest('.overflow-auto');
  if (!el) throw new Error(`no scroll container around ${testId}`);
  return el as HTMLElement;
}

describe('DetailPanel scroll position on selection change', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('content stream view starts at the top after switching streams', async () => {
    mockGetObjectDetail.mockImplementation((_tab: string, id: string) => Promise.resolve(streamDetail(id)));
    mockGetContentStream.mockImplementation((_tab: string, id: string) =>
      Promise.resolve(streamData(id, id === 'obj:0:10' ? 'first stream' : 'second stream')));

    renderPanel();
    select('obj:0:10');
    await waitFor(() => {
      expect(screen.getByTestId('content-stream-content')).toHaveTextContent('first stream');
    });
    scrollDown(scrollContainerOf('content-stream-content'));

    select('obj:0:11');
    await waitFor(() => {
      expect(screen.getByTestId('content-stream-content')).toHaveTextContent('second stream');
    });
    expect(scrollContainerOf('content-stream-content').scrollTop).toBe(0);
  });

  test('dictionary view starts at the top after switching dictionaries', async () => {
    mockGetObjectDetail.mockImplementation((_tab: string, id: string) =>
      Promise.resolve(dictDetail(id, id === 'obj:0:3' ? '/FirstKey' : '/SecondKey')));

    renderPanel();
    select('obj:0:3');
    await waitFor(() => {
      expect(screen.getByText('/FirstKey')).toBeInTheDocument();
    });
    const first = screen.getByText('/FirstKey').closest('.overflow-auto') as HTMLElement;
    scrollDown(first);

    select('obj:0:4');
    await waitFor(() => {
      expect(screen.getByText('/SecondKey')).toBeInTheDocument();
    });
    const second = screen.getByText('/SecondKey').closest('.overflow-auto') as HTMLElement;
    expect(second.scrollTop).toBe(0);
  });

  test('switching to another document tab with the same node id selected starts at the top', async () => {
    mockGetObjectDetail.mockImplementation((tab: string, id: string) =>
      Promise.resolve(dictDetail(id, tab === 'tab-1' ? '/FirstKey' : '/SecondKey')));

    renderPanel();
    select('obj:0:3');
    await waitFor(() => {
      expect(screen.getByText('/FirstKey')).toBeInTheDocument();
    });

    act(() => {
      dispatch({
        type: 'OPEN_DOCUMENT',
        payload: {
          tabId: 'tab-2',
          fileName: 'other.pdf',
          filePath: '/path/to/other.pdf',
          rootNode: catalogNode,
          rootChildren: [],
        },
      });
    });
    select('obj:0:3');
    await waitFor(() => {
      expect(screen.getByText('/SecondKey')).toBeInTheDocument();
    });
    scrollDown(screen.getByText('/SecondKey').closest('.overflow-auto') as HTMLElement);

    act(() => {
      dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } });
    });
    await waitFor(() => {
      expect(screen.getByText('/FirstKey')).toBeInTheDocument();
    });
    const first = screen.getByText('/FirstKey').closest('.overflow-auto') as HTMLElement;
    expect(first.scrollTop).toBe(0);
  });
});
