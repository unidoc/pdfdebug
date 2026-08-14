/**
 * Multi-Document State Isolation
 *
 * Dedup cleanup calls CloseDocument for duplicate tabId.
 *
 * Tests the component-layer cleanup: when a second document:opened event
 * arrives with the same filePath as an existing tab, the new tabId's backend
 * state is freed via CloseDocument.
 *
 * Annotation: The OS file association path in main.go calls openFileAndEmit,
 * which emits the same document:opened event tested here. The frontend event
 * listener dispatches OPEN_DOCUMENT regardless of the originating open path
 * (menu, drag-and-drop, or file association). No separate test is needed for
 * the file association event flow.
 *
 * Run: cd frontend && npx vitest run src/App.test.tsx
 */
import { render, screen, act, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';

// Capture event handlers registered via Events.On so we can simulate events
type EventHandler = (event: { data: Record<string, unknown> }) => void;
const eventHandlers: Record<string, EventHandler[]> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, handler: EventHandler) => {
      if (!eventHandlers[name]) eventHandlers[name] = [];
      eventHandlers[name].push(handler);
      // Return unsubscribe function
      return () => {
        const idx = eventHandlers[name]?.indexOf(handler) ?? -1;
        if (idx >= 0) eventHandlers[name].splice(idx, 1);
      };
    },
    Emit: vi.fn(),
  },
}));

// Mock CloseDocument so we can assert it is called for the duplicate tabId
const mockCloseDocument = vi.fn().mockResolvedValue(undefined);

vi.mock(
  '../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: (...args: unknown[]) => mockCloseDocument(...args),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: vi.fn(),
    GetContentStream: vi.fn(),
    GetAncestorPath: vi.fn(),
    // Story 12-1 harness gap: ConsumePendingOpenFiles was missing from this mock,
    // emitting 10 unhandled errors per run. Stubbed here so the cold-start drain
    // resolves cleanly (Story 13.2 closes the pre-existing gap).
    ConsumePendingOpenFiles: vi.fn().mockResolvedValue([]),
    // Story 13.2 new bound methods: mocked so DetailPanel's new tabs never widen
    // the gap.
    GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
    GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
    GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
    SaveBytesToFile: vi.fn().mockResolvedValue(''),
    // The Diff tab imports DiffDocuments; stubbed so the factory
    // never throws on the new export (the picker only calls it after a second
    // file is chosen).
    DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  })
);

// Mock child components to avoid pulling in the full tree
vi.mock('./components/EmptyState', () => ({
  EmptyState: () => <div data-testid="empty-state">Empty</div>,
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

// Helper: emit a simulated Wails event
function emitEvent(name: string, data: Record<string, unknown>) {
  const handlers = eventHandlers[name] ?? [];
  for (const handler of handlers) {
    handler({ data });
  }
}

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 2,
  iconHint: 'catalog',
  error: '',
};

describe('Dedup cleanup calls CloseDocument', () => {
  beforeEach(() => {
    // Clear event handlers between tests
    for (const key of Object.keys(eventHandlers)) {
      delete eventHandlers[key];
    }
    mockCloseDocument.mockClear();
  });

  test('opening same file twice calls CloseDocument for the duplicate tabId', async () => {
    // Dynamic import to ensure mocks are in place
    const { default: App } = await import('./App');

    render(<App />);

    // First open: creates tab-1 for /path/to/test.pdf
    act(() => {
      emitEvent('document:opened', {
        tabId: 'tab-1',
        fileName: 'test.pdf',
        filePath: '/path/to/test.pdf',
        rootNode: catalogNode,
        rootChildren: [],
      });
    });

    // Verify tab was created (main layout should render)
    await waitFor(() => {
      expect(screen.getByTestId('main-layout')).toBeInTheDocument();
    });

    // Second open: same filePath but new tabId
    act(() => {
      emitEvent('document:opened', {
        tabId: 'tab-duplicate',
        fileName: 'test.pdf',
        filePath: '/path/to/test.pdf',
        rootNode: catalogNode,
        rootChildren: [],
      });
    });

    // The reducer dedup logic activates the existing tab-1 instead of
    // creating tab-duplicate. The component layer must call CloseDocument
    // with the duplicate tabId to free backend resources.
    await waitFor(() => {
      expect(mockCloseDocument).toHaveBeenCalledWith('tab-duplicate');
    });
  });
});
