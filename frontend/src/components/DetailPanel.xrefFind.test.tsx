/**
 * XREF-tab find.
 *
 * Cmd+F find on the XREF tab matches the DISPLAYED cell text of XRefTableView
 * (obj number, gen, status, offset/host with their `-` placeholders) as a
 * literal substring, highlights the matched substring inside the cell (not a
 * whole-row selection), and navigates between matches via the `xref-find-*`
 * testids.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.xrefFind.test.tsx
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

// Displayed cells:
//   row 1: obj 1  gen 0  offset 4242  status in-use     host -
//   row 2: obj 2  gen 0  offset -     status free       host -
//   row 3: obj 3  gen 0  offset -     status in-objstm  host 7
const xrefData = {
  tabId: 'tab-1',
  entries: [
    { objNum: 1, gen: 0, status: 'in-use', offset: 4242, hostObjStm: 0, nodeID: 'obj:0:1' },
    { objNum: 2, gen: 0, status: 'free', offset: 0, hostObjStm: 0, nodeID: '' },
    { objNum: 3, gen: 0, status: 'in-objstm', offset: 0, hostObjStm: 7, nodeID: 'obj:0:3' },
  ],
};

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: { tabId: 'tab-1', fileName: 'test.pdf', filePath: '/test.pdf', rootNode: catalogNode, rootChildren: [] },
};

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

// Render, then activate the XREF tab so it fetches, and wait for a row.
async function renderAndOpenXref() {
  render(
    <AppProvider>
      <Dispatch action={openAction} />
      <DetailPanel />
    </AppProvider>,
  );
  fireEvent.click(await screen.findByTestId('detail-tab-xref'));
  await waitFor(() => {
    expect(screen.getByTestId('detail-pane-xref').getAttribute('data-state')).toBe('active');
    expect(screen.getByTestId('xref-row-1')).toBeInTheDocument();
  });
}

let restore: () => void;
beforeEach(() => {
  vi.clearAllMocks();
  mockGetObjectDetail.mockResolvedValue(null);
  mockGetReverseRefs.mockResolvedValue([]);
  mockGetXRefTable.mockResolvedValue(xrefData);
  mockGetPlainText.mockResolvedValue({ tabId: 'tab-1', content: 'x\n', totalBytes: 2 });
  mockGetPlainTextSize.mockResolvedValue(2);
  restore = forceMacPlatform();
});
afterEach(() => restore());

describe('Cmd+F opens find in the XREF tab', () => {
  test('with XREF active, Cmd+F mounts xref-find-bar', async () => {
    await renderAndOpenXref();
    expect(screen.queryByTestId('xref-find-bar')).toBeNull();
    cmdF();
    expect(screen.getByTestId('xref-find-bar')).toBeInTheDocument();
  });
});

describe('XREF find matches displayed cell text', () => {
  test('an offset value present only on the in-use row matches once', async () => {
    await renderAndOpenXref();
    cmdF();
    fireEvent.change(screen.getByTestId('xref-find-input'), { target: { value: '4242' } });
    await waitFor(() => expect(screen.getByTestId('xref-find-count').textContent).toBe('1 of 1'));
  });

  test('a status string matches the row that displays it', async () => {
    await renderAndOpenXref();
    cmdF();
    fireEvent.change(screen.getByTestId('xref-find-input'), { target: { value: 'in-objstm' } });
    await waitFor(() => {
      const total =
        screen.queryAllByTestId('xref-find-match').length +
        screen.queryAllByTestId('xref-find-active-match').length;
      expect(total).toBeGreaterThanOrEqual(1);
    });
  });

  test('a "-" query matches displayed placeholder and hyphenated status cells, not raw numeric fields', async () => {
    await renderAndOpenXref();
    cmdF();
    // Displayed "-" appears in the four placeholder cells (row1 host, row2
    // offset, row2 host, row3 offset) plus the hyphen inside the "in-use" and
    // "in-objstm" status cells -> six literal-substring matches. The raw numeric
    // 0s behind the placeholders are not displayed and must not match.
    fireEvent.change(screen.getByTestId('xref-find-input'), { target: { value: '-' } });
    await waitFor(() => {
      const total =
        screen.queryAllByTestId('xref-find-match').length +
        screen.queryAllByTestId('xref-find-active-match').length;
      expect(total).toBe(6);
    });
  });

  test('the highlight is a substring mark inside a cell, not a whole-row selection', async () => {
    await renderAndOpenXref();
    cmdF();
    fireEvent.change(screen.getByTestId('xref-find-input'), { target: { value: '4242' } });
    await waitFor(() => expect(screen.getByTestId('xref-find-count').textContent).toBe('1 of 1'));
    const mark = screen.getByTestId('xref-find-active-match');
    expect(mark.textContent).toBe('4242');
    // The mark lives inside the object-1 row, scoped to the cell.
    expect(mark.closest('[data-testid="xref-row-1"]')).not.toBeNull();
  });
});

describe('closing XREF find restores focus', () => {
  test('closing the bar moves focus to the xref scroll container', async () => {
    await renderAndOpenXref();
    cmdF();
    expect(screen.getByTestId('xref-find-bar')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('xref-find-close'));
    await waitFor(() => expect(screen.queryByTestId('xref-find-bar')).toBeNull());
    expect(document.activeElement).toBe(screen.getByTestId('xref-table-container'));
  });
});

describe('XREF find navigation', () => {
  test('Next advances the active match across the "-" matches', async () => {
    await renderAndOpenXref();
    cmdF();
    fireEvent.change(screen.getByTestId('xref-find-input'), { target: { value: '-' } });
    await waitFor(() => expect(screen.getByTestId('xref-find-count').textContent).toBe('1 of 6'));
    fireEvent.click(screen.getByTestId('xref-find-next'));
    expect(screen.getByTestId('xref-find-count').textContent).toBe('2 of 6');
  });
});

describe('XREF find issues no backend call', () => {
  test('opening, typing and navigating make no further pdfservice call', async () => {
    await renderAndOpenXref();
    // Discard the load-phase calls (xref table already fetched); find adds none.
    mockGetXRefTable.mockClear();
    mockGetObjectDetail.mockClear();
    mockGetPlainText.mockClear();
    mockGetPlainTextSize.mockClear();
    mockGetReverseRefs.mockClear();

    cmdF();
    fireEvent.change(screen.getByTestId('xref-find-input'), { target: { value: '-' } });
    await waitFor(() => expect(screen.getByTestId('xref-find-count').textContent).toBe('1 of 6'));
    fireEvent.click(screen.getByTestId('xref-find-next'));
    fireEvent.click(screen.getByTestId('xref-find-prev'));

    expect(mockGetXRefTable).not.toHaveBeenCalled();
    expect(mockGetObjectDetail).not.toHaveBeenCalled();
    expect(mockGetPlainText).not.toHaveBeenCalled();
    expect(mockGetPlainTextSize).not.toHaveBeenCalled();
    expect(mockGetReverseRefs).not.toHaveBeenCalled();
  });
});
