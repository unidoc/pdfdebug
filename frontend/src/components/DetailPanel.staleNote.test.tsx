/**
 * Stale note in the detail panel's inner-tab row, and signatures refused
 * because the file changed on disk.
 *
 * The note shows only for the active tab while it is changed or missing. It
 * sits next to the inner tab list, never inside it, so the tab triggers do not
 * move. Changed: "Document updated" with Reload and Compare buttons. Missing:
 * "File not found" with no buttons.
 *
 * Test ids: stale-note (the note), detail-tab-list (existing tab list).
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.staleNote.test.tsx
 */
import { render, screen, act, waitFor, within, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { Dispatch } from 'react';
import { AppProvider, useAppState, useAppDispatch, type AppAction, type AppState } from '../hooks/useDocumentState';
import { applyDiskStatusEvent, readDiskStatus, resetDiskStatusStore } from '../lib/diskStatusStore';
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

vi.mock('@wailsio/runtime', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@wailsio/runtime')>()),
  Events: {
    On: vi.fn(() => vi.fn()),
    Emit: vi.fn(),
  },
}));

const b = vi.hoisted(() => ({
  OpenFile: vi.fn(),
  GetTreeRoot: vi.fn(),
  GetChildren: vi.fn(),
  CloseDocument: vi.fn(),
  GetSignatures: vi.fn(),
  GetDiskStatus: vi.fn(),
  DiffDocuments: vi.fn(),
  OpenFileDialog: vi.fn(),
}));

vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: (...a: unknown[]) => b.OpenFile(...a),
  GetTreeRoot: (...a: unknown[]) => b.GetTreeRoot(...a),
  GetChildren: (...a: unknown[]) => b.GetChildren(...a),
  CloseDocument: (...a: unknown[]) => b.CloseDocument(...a),
  GetSignatures: (...a: unknown[]) => b.GetSignatures(...a),
  GetDiskStatus: (...a: unknown[]) => b.GetDiskStatus(...a),
  DiffDocuments: (...a: unknown[]) => b.DiffDocuments(...a),
  OpenFileDialog: (...a: unknown[]) => b.OpenFileDialog(...a),
  GetObjectDetail: vi.fn().mockResolvedValue(null),
  GetContentStream: vi.fn(),
  GetImageData: vi.fn(),
  DescribeImage: vi.fn().mockResolvedValue({ width: 0, height: 0, colorSpace: '', estimatedBytes: 0 }),
  SaveImageToFile: vi.fn().mockResolvedValue(''),
  GetReverseRefs: vi.fn().mockResolvedValue([]),
  GetFontView: vi.fn(),
  GetXRefTable: vi.fn().mockResolvedValue({ tabId: 'x', entries: [] }),
  GetPlainText: vi.fn().mockResolvedValue({ tabId: 'x', content: '', totalBytes: 0 }),
  GetPlainTextSize: vi.fn().mockResolvedValue(0),
  GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
  GetEmbeddedFileBytes: vi.fn(),
  GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '' }),
  SaveBytesToFile: vi.fn(),
  Validate: vi.fn().mockResolvedValue({ problems: [], rules: [], scope: '' }),
}));

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

let state: AppState;
let dispatch: Dispatch<AppAction>;
function Probe() {
  state = useAppState();
  dispatch = useAppDispatch();
  return null;
}

function mount() {
  render(
    <AppProvider>
      <Probe />
      <DetailPanel />
    </AppProvider>,
  );
}

let n = 0;
function paths() {
  n += 1;
  return { a: `a${n}`, b: `b${n}`, fresh: `fresh${n}`, pathA: `/docs/${n}/a.pdf`, pathB: `/docs/${n}/b.pdf` };
}

function open(tabId: string, filePath: string) {
  act(() =>
    dispatch({
      type: 'OPEN_DOCUMENT',
      payload: { tabId, fileName: filePath.split('/').pop() ?? '', filePath, pageCount: 1, rootNode: catalogNode, rootChildren: [] },
    }),
  );
}

const at = (h: number, m: number) => new Date(2026, 0, 12, h, m).getTime();

function mark(tabId: string, seq: number, state: 'changed' | 'missing' | 'current') {
  act(() =>
    applyDiskStatusEvent({
      tabId,
      seq,
      state,
      loadedModTime: at(14, 0),
      diskModTime: state === 'missing' ? 0 : at(14, 5),
      diskSize: state === 'missing' ? 0 : 4096,
    }),
  );
}

function envelope(message: string) {
  return new Error(JSON.stringify({ message, cause: {}, kind: 'RuntimeError' }));
}

function triggerNames(): string[] {
  return within(screen.getByTestId('detail-tab-list'))
    .getAllByRole('tab')
    .map((el) => el.textContent ?? '');
}

beforeEach(() => {
  for (const fn of Object.values(b)) fn.mockReset();
  b.CloseDocument.mockResolvedValue(undefined);
  b.GetSignatures.mockResolvedValue([]);
  b.GetTreeRoot.mockResolvedValue(catalogNode);
  b.GetChildren.mockResolvedValue([]);
  resetDiskStatusStore();
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date(2026, 0, 12, 18, 0));
});

afterEach(() => {
  vi.useRealTimers();
});

describe('stale note', () => {
  test('changed: shows Document updated with Reload and Compare buttons and the version tooltip', () => {
    const p = paths();
    mount();
    open(p.a, p.pathA);
    mark(p.a, 1, 'changed');

    const note = screen.getByTestId('stale-note');
    expect(note).toHaveTextContent('Document updated');
    const buttons = within(note).getAllByRole('button');
    expect(buttons.map((x) => x.textContent)).toEqual(['Reload', 'Compare']);
    for (const btn of buttons) expect(btn.tagName).toBe('BUTTON');
    const label = within(note).getByText('Document updated');
    expect(label.closest('[title]')).toHaveAttribute('title', 'Showing version from 14:00. Changed on disk 14:05.');
  });

  test('missing: shows File not found with no buttons', () => {
    const p = paths();
    mount();
    open(p.a, p.pathA);
    mark(p.a, 1, 'missing');

    const note = screen.getByTestId('stale-note');
    expect(note).toHaveTextContent('File not found');
    expect(within(note).queryAllByRole('button')).toHaveLength(0);
    expect(within(note).getByText('File not found').closest('[title]')).toHaveAttribute('title', 'File no longer found at this path');
  });

  test('absent for a current active tab, and for a stale background tab', () => {
    const p = paths();
    mount();
    open(p.a, p.pathA);
    open(p.b, p.pathB);
    expect(screen.queryByTestId('stale-note')).not.toBeInTheDocument();
    mark(p.a, 1, 'changed');
    expect(screen.queryByTestId('stale-note')).not.toBeInTheDocument();
  });

  test('sits beside the tab list, outside it, and leaves the tab triggers unchanged', () => {
    const p = paths();
    mount();
    open(p.a, p.pathA);
    const before = triggerNames();

    mark(p.a, 1, 'changed');

    const note = screen.getByTestId('stale-note');
    const list = screen.getByTestId('detail-tab-list');
    expect(list.contains(note)).toBe(false);
    expect(note.closest('[role="tablist"]')).toBeNull();
    expect(note.parentElement).toBe(list.parentElement);
    expect(triggerNames()).toEqual(before);
  });

  test('buttons keep a 24px minimum hit area and the row keeps its classes', () => {
    const p = paths();
    mount();
    open(p.a, p.pathA);
    const list = screen.getByTestId('detail-tab-list');
    const row = list.parentElement as HTMLElement;
    const rowBefore = row.className;
    const listBefore = list.className;

    mark(p.a, 1, 'changed');

    const note = screen.getByTestId('stale-note');
    expect(row.className).toBe(rowBefore);
    expect(list.className).toBe(listBefore);
    // jsdom has no layout: the note adds no vertical size of its own and the
    // buttons carry the 24px minimums.
    for (const el of [note, ...Array.from(note.querySelectorAll('*'))]) {
      expect(el.className).not.toMatch(/(^|\s)(py-|pt-|pb-|my-|mt-|mb-|h-|min-h-(?!\[24px\]))/);
    }
    for (const btn of within(note).getAllByRole('button')) {
      expect(btn.className).toMatch(/(^|\s)min-h-\[24px\](\s|$)/);
      expect(btn.className).toMatch(/(^|\s)min-w-\[24px\](\s|$)/);
    }
  });

  test('UI strings are ASCII and use no character separators', () => {
    const p = paths();
    mount();
    open(p.a, p.pathA);
    mark(p.a, 1, 'changed');
    const text = screen.getByTestId('stale-note').textContent ?? '';
    expect(text).toMatch(/^[\x20-\x7e]*$/);
    expect(text).not.toMatch(/[|/]/);
  });

  test('Reload reloads the active tab and the note goes away', async () => {
    const p = paths();
    mount();
    open(p.a, p.pathA);
    mark(p.a, 1, 'changed');
    b.OpenFile.mockResolvedValue({ tabId: p.fresh, fileName: 'a.pdf', filePath: p.pathA, pageCount: 3, fileSize: 1, error: '' });

    fireEvent.click(within(screen.getByTestId('stale-note')).getByRole('button', { name: 'Reload' }));

    await waitFor(() => expect(state.activeTabId).toBe(p.fresh));
    expect(b.OpenFile).toHaveBeenCalledWith(p.pathA);
    expect(b.CloseDocument).toHaveBeenCalledWith(p.a);
    expect(state.tabs.map((t) => t.tabId)).toEqual([p.fresh]);
    expect(screen.queryByTestId('stale-note')).not.toBeInTheDocument();
  });
});

function signedEntry(holeCheckError: string) {
  return {
    fieldName: 'Signature1',
    fieldNodeId: '4 0 R',
    signed: true,
    type: 'signature',
    subFilter: 'adbe.pkcs7.detached',
    notes: ['trust not verified - structural decomposition only'],
    certificates: [],
    byteRange: [0, 100, 200, 50],
    coversWholeFile: true,
    trailingGap: 0,
    holeMatchesContents: false,
    coverageError: '',
    holeCheckError,
  };
}

describe('signatures on a file that changed on disk', () => {
  test('keeps every signature, notes the hole as not checked and marks the tab', async () => {
    const p = paths();
    b.GetSignatures.mockResolvedValue([signedEntry('file changed on disk since it was opened')]);
    b.GetDiskStatus.mockResolvedValue({ state: 'changed', loadedModTime: at(14, 0), diskModTime: at(14, 5), diskSize: 4096 });
    mount();
    open(p.a, p.pathA);

    const sigTab = await screen.findByRole('tab', { name: /signatures/i });
    await waitFor(() => expect(b.GetDiskStatus).toHaveBeenCalledWith(p.a));
    await waitFor(() => expect(readDiskStatus(p.a)?.state).toBe('changed'));
    expect(screen.getByTestId('stale-note')).toBeInTheDocument();

    fireEvent.click(sigTab);
    expect(await screen.findByText('not checked: File changed on disk. Reload to view.')).toBeInTheDocument();
    expect(screen.getByText('adbe.pkcs7.detached')).toBeInTheDocument();
  });

  test('a file locked by another app notes the hole as busy and checks nothing', async () => {
    const p = paths();
    b.GetSignatures.mockResolvedValue([signedEntry('file is busy: another process has it open')]);
    mount();
    open(p.a, p.pathA);

    const sigTab = await screen.findByRole('tab', { name: /signatures/i });
    fireEvent.click(sigTab);
    expect(await screen.findByText('not checked: File is in use by another app. Reload to try again.')).toBeInTheDocument();
    expect(b.GetDiskStatus).not.toHaveBeenCalled();
  });

  test('another GetSignatures error shows the tab with the error and checks nothing', async () => {
    const p = paths();
    b.GetSignatures.mockRejectedValue(envelope('malformed PDF: bad acroform'));
    mount();
    open(p.a, p.pathA);

    const sigTab = await screen.findByRole('tab', { name: /signatures/i });
    expect(screen.getByTestId('tab-issue-signatures').getAttribute('data-tone')).toBe('error');
    fireEvent.click(sigTab);
    expect(screen.getByTestId('signatures-error').textContent).toContain('malformed PDF: bad acroform');
    expect(b.GetDiskStatus).not.toHaveBeenCalled();
  });
});
