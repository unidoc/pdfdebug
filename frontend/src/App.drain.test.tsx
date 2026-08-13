/**
 * Story 12.1: Cold-Start File Association Open -- frontend drain.
 *
 * On cold start the backend queues file-association paths instead of emitting
 * document:opened into a not-yet-listening WebView. App.jsx must, inside the
 * SAME useEffect that registers Events.On('document:opened', ...) and STRICTLY
 * AFTER that registration, call ConsumePendingOpenFiles() and open each
 * returned path through the existing openPDFFile() + OPEN_DOCUMENT pipeline.
 *
 * Run: cd frontend && npx vitest run src/App.drain.test.tsx
 */
import { render, screen, act, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { useAppState } from './hooks/useDocumentState';

// Ordered log of significant runtime/binding calls so we can assert that the
// document:opened subscription is registered BEFORE the drain call.
const callOrder: string[] = [];

type EventHandler = (event: { data: Record<string, unknown> }) => void;
const eventHandlers: Record<string, EventHandler[]> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, handler: EventHandler) => {
      if (!eventHandlers[name]) eventHandlers[name] = [];
      eventHandlers[name].push(handler);
      callOrder.push(`On:${name}`);
      return () => {
        const idx = eventHandlers[name]?.indexOf(handler) ?? -1;
        if (idx >= 0) eventHandlers[name].splice(idx, 1);
      };
    },
    Emit: vi.fn(),
  },
  // App.jsx imports Screens/Window for geometry restore; stub them so the
  // mount does not throw.
  Screens: { GetAll: vi.fn().mockResolvedValue(null) },
  Window: {
    SetSize: vi.fn().mockResolvedValue(undefined),
    SetPosition: vi.fn().mockResolvedValue(undefined),
    Position: vi.fn().mockResolvedValue({ x: 0, y: 0 }),
    Size: vi.fn().mockResolvedValue({ width: 800, height: 600 }),
  },
}));

// Binding mocks. ConsumePendingOpenFiles is the new method; the rest back the
// openPDFFile() pipeline (usePDFService.ts).
const mockConsume = vi.fn();
const mockOpenFile = vi.fn();
const mockGetTreeRoot = vi.fn().mockResolvedValue({ id: 'root', label: 'Catalog' });
const mockGetChildren = vi.fn().mockResolvedValue([]);
const mockCloseDocument = vi.fn().mockResolvedValue(undefined);

vi.mock('../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: (...a: unknown[]) => mockOpenFile(...a),
  GetTreeRoot: (...a: unknown[]) => mockGetTreeRoot(...a),
  GetChildren: (...a: unknown[]) => mockGetChildren(...a),
  CloseDocument: (...a: unknown[]) => mockCloseDocument(...a),
  OpenFileDialog: vi.fn(),
  GoToPage: vi.fn(),
  ConsumePendingOpenFiles: (...a: unknown[]) => {
    callOrder.push('ConsumePendingOpenFiles');
    return mockConsume(...a);
  },
}));

// EmptyState surfaces the real isOpening/openingFileName from app state so the
// OPENING_START dispatch is observable. When not opening it stays the plain
// empty marker the other cases assert on.
vi.mock('./components/EmptyState', () => ({
  EmptyState: () => {
    const { isOpening, openingFileName } = useAppState();
    if (isOpening) {
      return <div data-testid="opening-indicator">Opening {openingFileName ?? 'document'}...</div>;
    }
    return <div data-testid="empty-state">Empty</div>;
  },
}));
vi.mock('./components/MainLayout', () => ({
  MainLayout: () => <div data-testid="main-layout">Layout</div>,
}));
vi.mock('./components/ErrorBanner', () => ({
  ErrorBanner: ({ message }: { message: string }) => (
    <div data-testid="error-banner">{message}</div>
  ),
}));
vi.mock('./components/TabBar', () => ({
  TabBar: () => <div data-testid="tab-bar">Tabs</div>,
}));

function docInfo(tabId: string, filePath: string) {
  return {
    tabId,
    fileName: filePath.replace(/^.*[/\\]/, ''),
    filePath,
    pageCount: 1,
    fileSize: 10,
    error: '',
  };
}

beforeEach(() => {
  for (const key of Object.keys(eventHandlers)) delete eventHandlers[key];
  callOrder.length = 0;
  mockConsume.mockReset();
  mockOpenFile.mockReset();
  mockGetTreeRoot.mockClear();
  mockGetChildren.mockClear();
  mockCloseDocument.mockClear();
  mockGetTreeRoot.mockResolvedValue({ id: 'root', label: 'Catalog' });
  mockGetChildren.mockResolvedValue([]);
});

describe('cold-start drain', () => {
  // subscribe-before-drain ordering. The document:opened listener MUST be
  // registered before ConsumePendingOpenFiles is called, or a path delivered
  // between drain and subscribe would be lost.
  test('registers document:opened before draining', async () => {
    mockConsume.mockResolvedValue([]);
    const { default: App } = await import('./App');
    render(<App />);

    await waitFor(() => {
      expect(callOrder).toContain('ConsumePendingOpenFiles');
    });

    const subscribeIdx = callOrder.indexOf('On:document:opened');
    const drainIdx = callOrder.indexOf('ConsumePendingOpenFiles');
    expect(subscribeIdx).toBeGreaterThanOrEqual(0);
    expect(drainIdx).toBeGreaterThanOrEqual(0);
    expect(subscribeIdx).toBeLessThan(drainIdx);
  });

  // A drained path opens via the openPDFFile pipeline and results in an
  // OPEN_DOCUMENT (asserted via the rendered main layout) and a backend
  // OpenFile call for that path.
  test('drained path opens through openPDFFile', async () => {
    mockConsume.mockResolvedValue(['/cold/start.pdf']);
    mockOpenFile.mockResolvedValue(docInfo('tab-cold', '/cold/start.pdf'));

    const { default: App } = await import('./App');
    render(<App />);

    await waitFor(() => {
      expect(mockOpenFile).toHaveBeenCalledWith('/cold/start.pdf');
    });
    await waitFor(() => {
      expect(screen.getByTestId('main-layout')).toBeInTheDocument();
    });
  });

  // A null binding result (Go nil slice -> JSON null) must be treated as an
  // empty list -- no throw, empty state stays rendered.
  test('null drain result renders empty state without throwing', async () => {
    mockConsume.mockResolvedValue(null);

    const { default: App } = await import('./App');
    render(<App />);

    await waitFor(() => {
      expect(callOrder).toContain('ConsumePendingOpenFiles');
    });
    expect(mockOpenFile).not.toHaveBeenCalled();
    expect(screen.getByTestId('empty-state')).toBeInTheDocument();
  });

  // StrictMode double-mount must not double-open. The binding mock returns
  // paths on the first call and empty on the second (mirroring
  // drain-on-read), so even with two mounts only ONE OpenFile per path
  // fires.
  test('StrictMode double-mount does not double-open', async () => {
    mockConsume
      .mockResolvedValueOnce(['/once.pdf'])
      .mockResolvedValue([]);
    mockOpenFile.mockResolvedValue(docInfo('tab-once', '/once.pdf'));

    const { StrictMode } = await import('react');
    const { default: App } = await import('./App');
    render(
      <StrictMode>
        <App />
      </StrictMode>,
    );

    await waitFor(() => {
      expect(mockOpenFile).toHaveBeenCalledWith('/once.pdf');
    });
    // Exactly one open for the single drained path despite the double mount.
    const openCalls = mockOpenFile.mock.calls.filter((c) => c[0] === '/once.pdf');
    expect(openCalls).toHaveLength(1);
  });

  // A drained duplicate of an already-open file frees the new backend tab via
  // the drain loop's own pre-dispatch dedup. The lastOpenedTabIdRef
  // orphan-close fallback does NOT cover drain-path opens.
  test('drained duplicate frees the new backend tab', async () => {
    // First, open a file via document:opened so a tab with this filePath exists.
    mockConsume.mockResolvedValue([]);
    const { default: App } = await import('./App');
    render(<App />);

    await waitFor(() => expect(callOrder).toContain('ConsumePendingOpenFiles'));

    act(() => {
      const handlers = eventHandlers['document:opened'] ?? [];
      for (const h of handlers) {
        h({
          data: {
            tabId: 'tab-existing',
            fileName: 'dup.pdf',
            filePath: '/dup.pdf',
            rootNode: { id: 'root', label: 'Catalog' },
            rootChildren: [],
          },
        });
      }
    });
    await waitFor(() => expect(screen.getByTestId('main-layout')).toBeInTheDocument());

    // Now simulate a SECOND drain (e.g. dev reload) returning the SAME path.
    // openPDFFile yields a NEW tabId; the drain loop must CloseDocument it.
    mockConsume.mockResolvedValue(['/dup.pdf']);
    mockOpenFile.mockResolvedValue(docInfo('tab-drained-dup', '/dup.pdf'));

    // Re-trigger the drain by remounting (a reload re-runs the effect).
    const { unmount } = render(<App />);
    void unmount;

    await waitFor(() => {
      expect(mockCloseDocument).toHaveBeenCalledWith('tab-drained-dup');
    });
  });

  // The drain dispatches OPENING_START with the basename of the FIRST drained
  // path before awaiting the opens (mirrors single-file open UX). openPDFFile
  // is held pending so the OPENING_START state persists and EmptyState surfaces
  // the basename; the multi-path drain pins the "first path" rule and that the
  // basename is stripped of its directory.
  test('drain dispatches OPENING_START with first path basename', async () => {
    mockConsume.mockResolvedValue(['/cold/first.pdf', '/cold/second.pdf']);
    // Never resolves: keeps the app in the isOpening state so the indicator
    // (and its basename) stays rendered for the assertion.
    mockOpenFile.mockReturnValue(new Promise(() => {}));

    const { default: App } = await import('./App');
    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId('opening-indicator')).toHaveTextContent('Opening first.pdf...');
    });
  });

  // A per-path open error dispatches SET_DOCUMENT_ERROR (rendered via the
  // error banner) and does NOT block remaining paths.
  test('per-path error does not block remaining paths', async () => {
    mockConsume.mockResolvedValue(['/bad.pdf', '/good.pdf']);
    mockOpenFile.mockImplementation((p: string) => {
      if (p === '/bad.pdf') return Promise.reject(new Error('malformed PDF'));
      return Promise.resolve(docInfo('tab-good', '/good.pdf'));
    });

    const { default: App } = await import('./App');
    render(<App />);

    // The good path still opens despite the bad one erroring first.
    await waitFor(() => {
      expect(mockOpenFile).toHaveBeenCalledWith('/good.pdf');
    });
    await waitFor(() => {
      expect(screen.getByTestId('error-banner')).toBeInTheDocument();
    });
  });
});
