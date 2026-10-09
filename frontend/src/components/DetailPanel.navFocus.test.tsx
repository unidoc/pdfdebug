/**
 * Detail Panel -- the Back and Forward buttons keep keyboard focus when a
 * navigation loads a new node.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.navFocus.test.tsx
 */
import { render, screen, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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

function focusedTestId(): string {
  const el = document.activeElement as HTMLElement | null;
  if (!el || el === document.body) return 'body';
  return el.getAttribute('data-testid') ?? el.tagName.toLowerCase();
}

function shownKey(): string {
  if (screen.queryByText('/FirstKey')) return '/FirstKey';
  if (screen.queryByText('/SecondKey')) return '/SecondKey';
  return 'none';
}

describe('DetailPanel Back/Forward keep keyboard focus across a navigation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockImplementation((_tab: string, id: string) =>
      Promise.resolve(dictDetail(id, id === 'obj:0:3' ? '/FirstKey' : '/SecondKey')));
  });

  async function openTwoObjects() {
    renderPanel();
    select('obj:0:3');
    await waitFor(() => expect(screen.getByText('/FirstKey')).toBeInTheDocument());
    select('obj:0:4');
    await waitFor(() => expect(screen.getByText('/SecondKey')).toBeInTheDocument());
  }

  test('Enter on a focused Back button leaves focus on Back', async () => {
    const user = userEvent.setup();
    await openTwoObjects();

    screen.getByTestId('nav-back-button').focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(screen.getByText('/FirstKey')).toBeInTheDocument());

    expect(`focused=${focusedTestId()} | shown=${shownKey()}`)
      .toBe('focused=nav-back-button | shown=/FirstKey');
  });

  test('Enter on a focused Forward button leaves focus on Forward', async () => {
    const user = userEvent.setup();
    await openTwoObjects();

    screen.getByTestId('nav-back-button').focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(screen.getByText('/FirstKey')).toBeInTheDocument());
    screen.getByTestId('nav-forward-button').focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(screen.getByText('/SecondKey')).toBeInTheDocument());

    expect(`focused=${focusedTestId()} | shown=${shownKey()}`)
      .toBe('focused=nav-forward-button | shown=/SecondKey');
  });
});
