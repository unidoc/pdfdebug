/**
 * Reload from disk at the app level: the document:reload-active menu event and
 * F5 reload the active tab under a new tab id in the same strip position,
 * failures keep the old tab, a busy file only shows the pill, and on-disk
 * status events mark tabs without disturbing the active view.
 *
 * The real TabBar and ReloadToast render; MainLayout is replaced by a
 * useAppState consumer that counts its renders, standing in for TreePanel and
 * DetailPanel.
 *
 * Run: cd frontend && npx vitest run src/App.reload.test.tsx
 */
import { render, screen, act, waitFor, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import type { AppState } from './hooks/useDocumentState';
import { resetDiskStatusStore } from './lib/diskStatusStore';
import App from './App';
import { getPlatformModifier } from './lib/platform';
import { SetWatchedTabs } from '../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';

type EventHandler = (event: { data: Record<string, unknown> }) => void;
const eventHandlers: Record<string, EventHandler[]> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, handler: EventHandler) => {
      if (!eventHandlers[name]) eventHandlers[name] = [];
      eventHandlers[name].push(handler);
      return () => {
        const idx = eventHandlers[name]?.indexOf(handler) ?? -1;
        if (idx >= 0) eventHandlers[name].splice(idx, 1);
      };
    },
    Emit: vi.fn(),
  },
}));

const bindings = vi.hoisted(() => ({
  OpenFile: vi.fn(),
  GetTreeRoot: vi.fn(),
  GetChildren: vi.fn(),
  CloseDocument: vi.fn(),
  GetDiskStatus: vi.fn(),
}));

vi.mock('../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: (...a: unknown[]) => bindings.OpenFile(...a),
  GetTreeRoot: (...a: unknown[]) => bindings.GetTreeRoot(...a),
  GetChildren: (...a: unknown[]) => bindings.GetChildren(...a),
  CloseDocument: (...a: unknown[]) => bindings.CloseDocument(...a),
  GetDiskStatus: (...a: unknown[]) => bindings.GetDiskStatus(...a),
  SetWatchedTabs: vi.fn().mockResolvedValue(undefined),
  OpenFileDialog: vi.fn(),
  ConsumePendingOpenFiles: vi.fn().mockResolvedValue([]),
  GetObjectDetail: vi.fn(),
  GetContentStream: vi.fn(),
  GetAncestorPath: vi.fn(),
}));

const probe = vi.hoisted(() => ({ renders: 0, state: null as AppState | null }));

vi.mock('./components/MainLayout', async () => {
  const { useAppState } = await import('./hooks/useDocumentState');
  return {
    MainLayout: () => {
      const s = useAppState();
      probe.renders += 1;
      probe.state = s;
      return <div data-testid="main-layout">{s.activeTabId}</div>;
    },
  };
});
vi.mock('./components/EmptyState', () => ({
  EmptyState: () => <div data-testid="empty-state">Empty</div>,
}));
vi.mock('./components/ErrorBanner', () => ({
  ErrorBanner: ({ message }: { message: string }) => <div data-testid="error-banner">{message}</div>,
}));
vi.mock('./components/UpdateNotifier', () => ({
  UpdateNotifier: () => null,
}));

function emitEvent(name: string, data: Record<string, unknown> = {}) {
  for (const handler of [...(eventHandlers[name] ?? [])]) handler({ data });
}

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

// openedDocuments.ts keeps module-level maps of released ids and paths, so
// every test uses its own ids and paths.
let n = 0;
function ids() {
  n += 1;
  return {
    a: `a${n}`,
    b: `b${n}`,
    fresh: `fresh${n}`,
    pathA: `/docs/${n}/a.pdf`,
    pathB: `/docs/${n}/b.pdf`,
  };
}

function deliverOpen(tabId: string, filePath: string, pageCount = 1) {
  act(() => {
    emitEvent('document:opened', {
      tabId,
      fileName: filePath.split('/').pop(),
      filePath,
      pageCount,
      rootNode: catalogNode,
      rootChildren: [],
    });
  });
}

function mockOpenResult(tabId: string, filePath: string, pageCount = 7, warning = '') {
  bindings.OpenFile.mockResolvedValue({ tabId, fileName: filePath.split('/').pop(), filePath, pageCount, fileSize: 2048, error: warning });
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function backendError(message: string) {
  return new Error(JSON.stringify({ message, cause: {}, kind: 'RuntimeError' }));
}

function tabOrder(): string[] {
  return Array.from(screen.getByTestId('tab-list').querySelectorAll('[data-testid^="tab-"][role="tab"]')).map(
    (el) => (el.getAttribute('data-testid') ?? '').replace(/^tab-/, ''),
  );
}

function diskStatus(tabId: string, seq: number, state: string) {
  act(() => {
    emitEvent('document:disk-status', {
      tabId,
      seq,
      state,
      loadedModTime: new Date(2026, 0, 12, 14, 0).getTime(),
      diskModTime: state === 'missing' ? 0 : new Date(2026, 0, 12, 14, 5).getTime(),
      diskSize: state === 'missing' ? 0 : 4096,
    });
  });
}

beforeEach(() => {
  for (const key of Object.keys(eventHandlers)) delete eventHandlers[key];
  for (const fn of Object.values(bindings)) fn.mockReset();
  bindings.CloseDocument.mockResolvedValue(undefined);
  bindings.GetTreeRoot.mockResolvedValue(catalogNode);
  bindings.GetChildren.mockResolvedValue([]);
  resetDiskStatusStore();
  probe.renders = 0;
  probe.state = null;
});

describe('reload triggers', () => {
  test('the reload menu event replaces the active tab in place under a new tab id', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    deliverOpen(t.b, t.pathB);
    deliverOpen(t.a, t.pathA); // repeated delivery activates the first tab
    expect(probe.state?.activeTabId).toBe(t.a);
    mockOpenResult(t.fresh, t.pathA, 7);
    const freshChildren = [{ ...catalogNode, id: 'root/Pages', label: 'Pages', rawKey: 'Pages', iconHint: 'pages' }];
    bindings.GetChildren.mockResolvedValue(freshChildren);

    act(() => emitEvent('document:reload-active'));

    await waitFor(() => expect(tabOrder()).toEqual([t.fresh, t.b]));
    expect(bindings.OpenFile).toHaveBeenCalledTimes(1);
    expect(bindings.OpenFile).toHaveBeenCalledWith(t.pathA);
    expect(bindings.GetChildren).toHaveBeenCalledWith(t.fresh, 'root');
    expect(bindings.CloseDocument).toHaveBeenCalledWith(t.a);
    expect(bindings.CloseDocument).not.toHaveBeenCalledWith(t.fresh);
    expect(probe.state?.activeTabId).toBe(t.fresh);
    const fresh = probe.state?.tabs.find((x) => x.tabId === t.fresh);
    expect(fresh?.pageCount).toBe(7);
    expect(fresh?.rootChildren).toEqual(freshChildren);
  });

  test('F5 reloads the active tab and prevents the default action', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    mockOpenResult(t.fresh, t.pathA);

    const notPrevented = fireEvent.keyDown(window, { key: 'F5' });

    expect(notPrevented).toBe(false);
    await waitFor(() => expect(tabOrder()).toEqual([t.fresh]));
    expect(bindings.OpenFile).toHaveBeenCalledWith(t.pathA);
  });

  test('F5 typed in a text field does nothing', () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();

    fireEvent.keyDown(input, { key: 'F5' });

    expect(bindings.OpenFile).not.toHaveBeenCalled();
    input.remove();
  });

  test('a held F5 (repeat) does nothing', () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);

    fireEvent.keyDown(window, { key: 'F5', repeat: true });

    expect(bindings.OpenFile).not.toHaveBeenCalled();
  });

  test('a second trigger while the reload is parsing starts no second open', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    const pending = deferred<unknown>();
    bindings.OpenFile.mockReturnValue(pending.promise);

    act(() => emitEvent('document:reload-active'));
    act(() => emitEvent('document:reload-active'));
    fireEvent.keyDown(window, { key: 'F5' });

    expect(bindings.OpenFile).toHaveBeenCalledTimes(1);
    await act(async () => {
      pending.resolve({ tabId: t.fresh, fileName: 'a.pdf', filePath: t.pathA, pageCount: 1, fileSize: 1, error: '' });
    });
    await waitFor(() => expect(tabOrder()).toEqual([t.fresh]));
  });

  test('with no document open every trigger is a no-op', () => {
    render(<App />);
    act(() => emitEvent('document:reload-active'));
    fireEvent.keyDown(window, { key: 'F5' });
    expect(bindings.OpenFile).not.toHaveBeenCalled();
  });

  test('a reload with no detected change still reloads', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    mockOpenResult(t.fresh, t.pathA);
    act(() => emitEvent('document:reload-active'));
    await waitFor(() => expect(bindings.OpenFile).toHaveBeenCalledWith(t.pathA));
    expect(bindings.GetDiskStatus).not.toHaveBeenCalledWith(t.a);
  });

  test('Cmd/Ctrl+R reloads the active tab and prevents the webview reload', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    mockOpenResult(t.fresh, t.pathA);
    const mod = getPlatformModifier() === 'Cmd' ? { metaKey: true } : { ctrlKey: true };

    const notPrevented = fireEvent.keyDown(window, { key: 'r', ...mod });

    expect(notPrevented).toBe(false);
    await waitFor(() => expect(tabOrder()).toEqual([t.fresh]));
  });

  test('Cmd/Ctrl+R in a text field is swallowed without reloading', () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    const mod = getPlatformModifier() === 'Cmd' ? { metaKey: true } : { ctrlKey: true };

    const notPrevented = fireEvent.keyDown(input, { key: 'r', ...mod });

    expect(notPrevented).toBe(false);
    expect(bindings.OpenFile).not.toHaveBeenCalled();
    input.remove();
  });

  test('the menu event and the keydown for one Cmd/Ctrl+R press open the file once', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    mockOpenResult(t.fresh, t.pathA);
    const mod = getPlatformModifier() === 'Cmd' ? { metaKey: true } : { ctrlKey: true };

    fireEvent.keyDown(window, { key: 'r', ...mod });
    act(() => emitEvent('document:reload-active'));

    await waitFor(() => expect(tabOrder()).toEqual([t.fresh]));
    expect(bindings.OpenFile).toHaveBeenCalledTimes(1);
  });
});

describe('reload outcome', () => {
  test('success shows the reload pill', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    mockOpenResult(t.fresh, t.pathA);

    act(() => emitEvent('document:reload-active'));

    const pill = await screen.findByText('a.pdf reloaded - view reset');
    expect(pill.closest('[role="status"]')).not.toBeNull();
  });

  test('a warning carried by the reloaded document is shown', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    mockOpenResult(t.fresh, t.pathA, 1, 'This PDF has structural errors. Some objects may display incorrectly.');

    act(() => emitEvent('document:reload-active'));

    await waitFor(() =>
      expect(screen.getByTestId('error-banner')).toHaveTextContent('This PDF has structural errors. Some objects may display incorrectly.'),
    );
  });

  test.each([
    ['document not found: stat /docs/a.pdf: no such file or directory', 'file not found'],
    ['malformed PDF: xref table not found', 'file is damaged or not a valid PDF'],
    ['encrypted PDF: password required', 'file is encrypted'],
    ['something else went wrong', 'unexpected error'],
  ])('failure "%s" shows "Could not reload a.pdf: %s" and keeps the old tab and its marker', async (raw, reason) => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    diskStatus(t.a, 1, 'changed');
    bindings.OpenFile.mockRejectedValue(backendError(raw));

    act(() => emitEvent('document:reload-active'));

    await waitFor(() => expect(screen.getByTestId('error-banner')).toHaveTextContent(`Could not reload a.pdf: ${reason}`));
    expect(tabOrder()).toEqual([t.a]);
    expect(probe.state?.activeTabId).toBe(t.a);
    expect(screen.getByTestId(`tab-disk-dot-${t.a}`)).toBeInTheDocument();
    expect(bindings.CloseDocument).not.toHaveBeenCalledWith(t.a);
  });

  test('a busy file shows only the pill and keeps the tab and its marker', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    diskStatus(t.a, 1, 'changed');
    bindings.OpenFile.mockRejectedValue(backendError('file is busy: open /docs/a.pdf: The process cannot access the file'));

    act(() => emitEvent('document:reload-active'));

    const pill = await screen.findByText('a.pdf is in use by another app - try again');
    expect(pill.closest('[role="status"]')).not.toBeNull();
    expect(screen.queryByTestId('error-banner')).not.toBeInTheDocument();
    expect(tabOrder()).toEqual([t.a]);
    expect(screen.getByTestId(`tab-disk-dot-${t.a}`)).toBeInTheDocument();
  });

  test('closing the tab while its reload parses closes the new document and adds no tab', async () => {
    const t = ids();
    const user = userEvent.setup();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    deliverOpen(t.b, t.pathB);
    deliverOpen(t.a, t.pathA);
    const pending = deferred<unknown>();
    bindings.OpenFile.mockReturnValue(pending.promise);

    act(() => emitEvent('document:reload-active'));
    await user.click(screen.getByTestId(`tab-close-${t.a}`));
    await act(async () => {
      pending.resolve({ tabId: t.fresh, fileName: 'a.pdf', filePath: t.pathA, pageCount: 1, fileSize: 1, error: '' });
    });

    await waitFor(() => expect(bindings.CloseDocument).toHaveBeenCalledWith(t.fresh));
    expect(tabOrder()).toEqual([t.b]);
    expect(screen.queryByText(/reloaded - view reset/)).not.toBeInTheDocument();
  });

  test('the marker clears once the reload lands', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    diskStatus(t.a, 1, 'changed');
    mockOpenResult(t.fresh, t.pathA);

    act(() => emitEvent('document:reload-active'));

    await waitFor(() => expect(tabOrder()).toEqual([t.fresh]));
    expect(screen.queryByTestId(`tab-disk-dot-${t.fresh}`)).not.toBeInTheDocument();
  });

  test('a change that landed while the reload parsed marks the new tab at once', async () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    mockOpenResult(t.fresh, t.pathA);
    bindings.GetDiskStatus.mockImplementation((id: string) => Promise.resolve(
      id === t.fresh
        ? { state: 'changed', loadedModTime: 1, diskModTime: 2, diskSize: 10 }
        : { state: 'current', loadedModTime: 1, diskModTime: 1, diskSize: 10 },
    ));

    act(() => emitEvent('document:reload-active'));

    await waitFor(() => expect(screen.getByTestId(`tab-disk-dot-${t.fresh}`)).toBeInTheDocument());
    expect(bindings.GetDiskStatus).toHaveBeenCalledWith(t.fresh);
  });
});

describe('on-disk status events', () => {
  test('the backend watches exactly the tab ids in the strip', async () => {
    const t = ids();
    const watched = vi.mocked(SetWatchedTabs);
    watched.mockClear();
    render(<App />);
    deliverOpen(t.a, t.pathA);

    await waitFor(() => expect(watched).toHaveBeenLastCalledWith([t.a]));
  });

  test('a changed event marks the tab and announces it once', () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);

    diskStatus(t.a, 1, 'changed');

    expect(screen.getByTestId(`tab-disk-dot-${t.a}`)).toBeInTheDocument();
    const announcer = screen.getByTestId('disk-change-announcer');
    expect(announcer).toHaveAttribute('aria-live', 'polite');
    expect(announcer).toHaveTextContent('a.pdf changed on disk');
  });

  test('an event does not disturb the active view', () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    const focusTarget = document.createElement('button');
    document.body.appendChild(focusTarget);
    focusTarget.focus();
    const rendersBefore = probe.renders;
    const stateBefore = probe.state;

    diskStatus(t.a, 1, 'changed');
    diskStatus(t.a, 2, 'missing');

    expect(document.activeElement).toBe(focusTarget);
    expect(probe.renders).toBe(rendersBefore);
    expect(probe.state).toBe(stateBefore);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByTestId('error-banner')).not.toBeInTheDocument();
    for (const fn of Object.values(bindings)) expect(fn).not.toHaveBeenCalled();
    focusTarget.remove();
  });

  test('an event whose seq is not above the last applied one changes nothing', () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    diskStatus(t.a, 5, 'changed');
    diskStatus(t.a, 4, 'current');
    expect(screen.getByTestId(`tab-disk-dot-${t.a}`)).toBeInTheDocument();
  });

  test('a current event clears the marker', () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    diskStatus(t.a, 1, 'changed');
    diskStatus(t.a, 2, 'current');
    expect(screen.queryByTestId(`tab-disk-dot-${t.a}`)).not.toBeInTheDocument();
  });

  test('events for tab ids not in the tab list are ignored', () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    diskStatus('backend-only-diff-copy', 1, 'changed');
    expect(screen.queryByTestId(/^tab-disk-dot-/)).not.toBeInTheDocument();
    expect(screen.getByTestId('disk-change-announcer')).not.toHaveTextContent('changed on disk');
  });

  test('a background tab is marked too', () => {
    const t = ids();
    render(<App />);
    deliverOpen(t.a, t.pathA);
    deliverOpen(t.b, t.pathB);
    diskStatus(t.a, 1, 'changed');
    expect(screen.getByTestId(`tab-disk-dot-${t.a}`)).toBeInTheDocument();
    expect(screen.queryByTestId(`tab-disk-dot-${t.b}`)).not.toBeInTheDocument();
  });
});
