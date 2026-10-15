/**
 * Compare with File on Disk: the active tab's file is parsed again backend-only
 * and shown as the right side of the Diff inner tab, labelled with its disk
 * time. Over 500 MB asks first; a missing file parses nothing. Reload in the
 * Diff header promotes the compared copy to the tab without parsing again.
 *
 * Entry points: the document:compare-active menu event and the stale note's
 * Compare button. Test ids: stale-note, diff-reload (Diff header Reload, only
 * for a disk copy), existing detail-tab-diff, diff-pick-error, diff-close.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.compareDisk.test.tsx
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

type EventHandler = (event: { data: Record<string, unknown> }) => void;
const handlers = vi.hoisted(() => ({} as Record<string, ((e: { data: Record<string, unknown> }) => void)[]>));

vi.mock('@wailsio/runtime', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@wailsio/runtime')>()),
  Events: {
    On: (name: string, handler: EventHandler) => {
      (handlers[name] ??= []).push(handler);
      return () => {
        const i = handlers[name]?.indexOf(handler) ?? -1;
        if (i >= 0) handlers[name].splice(i, 1);
      };
    },
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

const identicalDiff = {
  summary: {
    added: 0,
    removed: 0,
    changed: 0,
    pageCountLeft: 1,
    pageCountRight: 1,
    versionChanged: false,
    encryptionChanged: false,
    infoChanged: false,
    xmpChanged: false,
  },
  root: { path: '/Root', status: 'unchanged', kind: 'dict', changedKeys: [], leftSummary: '', rightSummary: '', children: [] },
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
function ids() {
  n += 1;
  return { a: `a${n}`, b: `b${n}`, disk: `disk${n}`, pathA: `/docs/${n}/a.pdf`, pathB: `/docs/${n}/b.pdf` };
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
const MB = 1024 * 1024;

function diskStatusOf(state: string, diskSize = 4096) {
  return { state, loadedModTime: at(14, 0), diskModTime: state === 'missing' ? 0 : at(14, 5), diskSize: state === 'missing' ? 0 : diskSize };
}

function markChanged(tabId: string) {
  act(() => applyDiskStatusEvent({ tabId, seq: 1, ...diskStatusOf('changed') }));
}

function emit(name: string) {
  act(() => {
    for (const h of [...(handlers[name] ?? [])]) h({ data: {} });
  });
}

function clickCompare() {
  fireEvent.click(within(screen.getByTestId('stale-note')).getByRole('button', { name: 'Compare' }));
}

function diskDoc(tabId: string, filePath: string) {
  return { tabId, fileName: filePath.split('/').pop(), filePath, pageCount: 2, fileSize: 4096, error: '' };
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

beforeEach(() => {
  for (const key of Object.keys(handlers)) delete handlers[key];
  for (const fn of Object.values(b)) fn.mockReset();
  b.CloseDocument.mockResolvedValue(undefined);
  b.GetSignatures.mockResolvedValue([]);
  b.GetTreeRoot.mockResolvedValue(catalogNode);
  b.GetChildren.mockResolvedValue([]);
  b.DiffDocuments.mockResolvedValue(identicalDiff);
  resetDiskStatusStore();
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date(2026, 0, 12, 18, 0));
});

afterEach(() => {
  vi.useRealTimers();
});

describe('compare with the file on disk', () => {
  test('the stale note Compare opens Diff with the disk copy on the right, labelled with its disk time', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    markChanged(t.a);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed'));
    b.OpenFile.mockResolvedValue(diskDoc(t.disk, t.pathA));

    clickCompare();

    await waitFor(() => expect(b.DiffDocuments).toHaveBeenCalledWith(t.a, t.disk));
    expect(b.GetDiskStatus).toHaveBeenCalledWith(t.a);
    expect(b.OpenFile).toHaveBeenCalledWith(t.pathA);
    expect(screen.getByTestId('detail-tab-diff')).toHaveAttribute('aria-selected', 'true');
    expect(await screen.findByText(/On disk \(14:05\)/)).toBeInTheDocument();
    expect(state.tabs.map((x) => x.tabId)).toEqual([t.a]);
  });

  test('the compare menu event does the same for a tab that is not stale', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('current'));
    b.OpenFile.mockResolvedValue(diskDoc(t.disk, t.pathA));

    emit('document:compare-active');

    await waitFor(() => expect(b.DiffDocuments).toHaveBeenCalledWith(t.a, t.disk));
    expect(screen.getByTestId('detail-tab-diff')).toHaveAttribute('aria-selected', 'true');
  });

  test('the compare menu event with no document open does nothing', () => {
    mount();
    emit('document:compare-active');
    expect(b.GetDiskStatus).not.toHaveBeenCalled();
    expect(b.OpenFile).not.toHaveBeenCalled();
  });

  test('over 500 MB asks first; Cancel parses nothing', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    markChanged(t.a);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed', 600 * MB));

    clickCompare();

    const dialog = await screen.findByRole('dialog');
    expect(dialog).toHaveTextContent(/a\.pdf is .+ on disk\. Comparing loads a second copy into memory\. Continue\?/);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(b.OpenFile).not.toHaveBeenCalled();
  });

  test('over 500 MB: Continue parses the disk copy', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    markChanged(t.a);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed', 600 * MB));
    b.OpenFile.mockResolvedValue(diskDoc(t.disk, t.pathA));

    clickCompare();
    fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Continue' }));

    await waitFor(() => expect(b.OpenFile).toHaveBeenCalledWith(t.pathA));
  });

  test('exactly 500 MB does not ask', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    markChanged(t.a);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed', 500 * MB));
    b.OpenFile.mockResolvedValue(diskDoc(t.disk, t.pathA));

    clickCompare();

    await waitFor(() => expect(b.OpenFile).toHaveBeenCalledWith(t.pathA));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  test('a missing file shows the missing message in Diff and parses nothing', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('missing'));

    emit('document:compare-active');

    expect(await screen.findByTestId('diff-pick-error')).toHaveTextContent('File no longer found at this path');
    expect(screen.getByTestId('detail-tab-diff')).toHaveAttribute('aria-selected', 'true');
    expect(b.OpenFile).not.toHaveBeenCalled();
  });

  test('a parse error, busy included, goes to the Diff pick error slot', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed'));
    b.OpenFile.mockRejectedValue(new Error(JSON.stringify({ message: 'file is busy: sharing violation', cause: {}, kind: 'RuntimeError' })));

    emit('document:compare-active');

    expect(await screen.findByTestId('diff-pick-error')).toHaveTextContent('file is busy');
    expect(b.DiffDocuments).not.toHaveBeenCalled();
  });

  test('switching tabs while the disk copy parses closes it', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    open(t.b, t.pathB);
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: t.a } }));
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed'));
    const pending = deferred<unknown>();
    b.OpenFile.mockReturnValue(pending.promise);

    emit('document:compare-active');
    await waitFor(() => expect(b.OpenFile).toHaveBeenCalled());
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: t.b } }));
    await act(async () => {
      pending.resolve(diskDoc(t.disk, t.pathA));
    });

    await waitFor(() => expect(b.CloseDocument).toHaveBeenCalledWith(t.disk));
    expect(b.DiffDocuments).not.toHaveBeenCalled();
  });

  test('a disk check that lands after a tab switch still marks the tab and parses nothing', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    open(t.b, t.pathB);
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: t.a } }));
    const check = deferred<unknown>();
    b.GetDiskStatus.mockReturnValue(check.promise);

    emit('document:compare-active');
    await waitFor(() => expect(b.GetDiskStatus).toHaveBeenCalledWith(t.a));
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: t.b } }));
    await act(async () => {
      check.resolve(diskStatusOf('changed'));
    });

    expect(readDiskStatus(t.a)?.state).toBe('changed');
    expect(b.OpenFile).not.toHaveBeenCalled();
  });

  test('a second Compare while the disk copy parses starts no second parse', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed'));
    const pending = deferred<unknown>();
    b.OpenFile.mockReturnValue(pending.promise);

    emit('document:compare-active');
    await waitFor(() => expect(b.OpenFile).toHaveBeenCalledTimes(1));
    emit('document:compare-active');
    await waitFor(() => expect(b.GetDiskStatus).toHaveBeenCalledTimes(2));
    await act(async () => {
      await Promise.resolve();
    });
    expect(b.OpenFile).toHaveBeenCalledTimes(1);

    await act(async () => {
      pending.resolve(diskDoc(t.disk, t.pathA));
    });
    await waitFor(() => expect(b.DiffDocuments).toHaveBeenCalledWith(t.a, t.disk));
  });

  test('Close diff frees the disk copy', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed'));
    b.OpenFile.mockResolvedValue(diskDoc(t.disk, t.pathA));
    emit('document:compare-active');
    await waitFor(() => expect(b.DiffDocuments).toHaveBeenCalled());

    fireEvent.click(await screen.findByTestId('diff-close'));

    await waitFor(() => expect(b.CloseDocument).toHaveBeenCalledWith(t.disk));
  });
});

describe('reload from the Diff header', () => {
  test('promotes the compared copy into the tab without parsing again', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    open(t.b, t.pathB);
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: t.a } }));
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed'));
    b.OpenFile.mockResolvedValue(diskDoc(t.disk, t.pathA));
    emit('document:compare-active');
    await waitFor(() => expect(b.DiffDocuments).toHaveBeenCalledWith(t.a, t.disk));
    b.GetDiskStatus.mockClear();
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('current'));

    fireEvent.click(await screen.findByTestId('diff-reload'));

    await waitFor(() => expect(state.activeTabId).toBe(t.disk));
    expect(state.tabs.map((x) => x.tabId)).toEqual([t.disk, t.b]);
    expect(state.tabs[0].pageCount).toBe(2);
    expect(b.OpenFile).toHaveBeenCalledTimes(1);
    expect(b.GetTreeRoot).toHaveBeenCalledWith(t.disk);
    expect(b.GetChildren).toHaveBeenCalledWith(t.disk, 'root');
    expect(b.CloseDocument).toHaveBeenCalledWith(t.a);
    await waitFor(() => expect(b.GetDiskStatus).toHaveBeenCalledWith(t.disk));
    expect(b.GetDiskStatus).toHaveBeenCalledTimes(1);
    // Let any cleanup effects run before asserting the promoted id stays open.
    await act(async () => {
      await Promise.resolve();
    });
    expect(b.CloseDocument).not.toHaveBeenCalledWith(t.disk);
  });

  test('Close diff while the promotion fetches the tree leaves the tab alone', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed'));
    b.OpenFile.mockResolvedValue(diskDoc(t.disk, t.pathA));
    emit('document:compare-active');
    await waitFor(() => expect(b.DiffDocuments).toHaveBeenCalledWith(t.a, t.disk));
    const tree = deferred<unknown>();
    b.GetTreeRoot.mockReturnValue(tree.promise);

    fireEvent.click(await screen.findByTestId('diff-reload'));
    fireEvent.click(await screen.findByTestId('diff-close'));
    await waitFor(() => expect(b.CloseDocument).toHaveBeenCalledWith(t.disk));
    await act(async () => {
      tree.resolve(catalogNode);
    });

    expect(state.tabs.map((x) => x.tabId)).toEqual([t.a]);
    expect(state.activeTabId).toBe(t.a);
    expect(state.documentError).toBeNull();
    expect(b.CloseDocument).not.toHaveBeenCalledWith(t.a);
  });

  test('a failed promotion sets only the error banner', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed'));
    b.OpenFile.mockResolvedValue(diskDoc(t.disk, t.pathA));
    emit('document:compare-active');
    await waitFor(() => expect(b.DiffDocuments).toHaveBeenCalledWith(t.a, t.disk));
    act(() => dispatch({ type: 'SET_DOCUMENT_WARNING', payload: { message: 'earlier warning' } }));
    act(() => dispatch({ type: 'OPENING_START', payload: { fileName: 'x.pdf' } }));
    b.GetTreeRoot.mockRejectedValue(new Error('malformed PDF: bad xref'));

    fireEvent.click(await screen.findByTestId('diff-reload'));

    await waitFor(() => expect(state.documentError).toBe('Could not reload a.pdf: file is damaged or not a valid PDF'));
    expect(state.documentWarning).toBe('earlier warning');
    expect(state.isOpening).toBe(true);
    expect(state.tabs.map((x) => x.tabId)).toEqual([t.a]);
  });

  test('the promoted copy\'s parse warning replaces the banner in one dispatch', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    b.GetDiskStatus.mockResolvedValue(diskStatusOf('changed'));
    b.OpenFile.mockResolvedValue({ ...diskDoc(t.disk, t.pathA), error: 'This PDF has structural errors. Some objects may display incorrectly.' });
    emit('document:compare-active');
    await waitFor(() => expect(b.DiffDocuments).toHaveBeenCalledWith(t.a, t.disk));

    fireEvent.click(await screen.findByTestId('diff-reload'));

    await waitFor(() => expect(state.activeTabId).toBe(t.disk));
    expect(state.documentWarning).toBe('This PDF has structural errors. Some objects may display incorrectly.');
  });

  test('a file picked with Compare with another PDF gets no header Reload', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    b.OpenFileDialog.mockResolvedValue(['/elsewhere/other.pdf']);
    b.OpenFile.mockResolvedValue(diskDoc(t.disk, '/elsewhere/other.pdf'));

    fireEvent.click(screen.getByTestId('detail-tab-diff'));
    fireEvent.click(await screen.findByTestId('diff-pick-file'));

    await waitFor(() => expect(b.DiffDocuments).toHaveBeenCalledWith(t.a, t.disk));
    expect(screen.queryByTestId('diff-reload')).not.toBeInTheDocument();
    expect(screen.queryByText(/On disk \(/)).not.toBeInTheDocument();
  });
});
