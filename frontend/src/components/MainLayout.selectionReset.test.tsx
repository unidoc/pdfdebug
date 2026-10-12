/**
 * The object source and detail panels render the selected node, so after a
 * render error a new selection, a history step or a tab switch gives them
 * another render. The navigator panels reset on a tab change only.
 */
import { render, screen, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { AppProvider, useAppDispatch, type AppAction } from '../hooks/useDocumentState';
import { MainLayout } from './MainLayout';

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

vi.mock('../hooks/useWindowPersistence', () => ({
  useWindowPersistence: () => ({
    panelSizes: null,
    savePanelSizes: vi.fn(),
  }),
}));

vi.mock(
  '../../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: vi.fn(),
    GetPageIndex: vi.fn(),
    GetImageIndex: vi.fn(),
    GetImagePages: vi.fn(),
    GetImagePageGroups: vi.fn(),
  })
);

// Each panel throws while its fail condition matches; null never matches.
const failOn = vi.hoisted(() => ({
  detail: null as string | null,
  source: null as string | null,
  pagesTab: null as string | null,
}));

const selectedNodeId = vi.hoisted(
  () => (state: { activeTabId: string | null; tabs: { tabId: string; selectedNodeId: string | null }[] }) =>
    state.tabs.find((t) => t.tabId === state.activeTabId)?.selectedNodeId ?? null
);

vi.mock('./DetailPanel', async () => {
  const { useAppState } = await import('../hooks/useDocumentState');
  return {
    DetailPanel: () => {
      const node = selectedNodeId(useAppState());
      if (node !== null && node === failOn.detail) throw new Error(`detail fails on ${node}`);
      return <div data-testid="detail-panel-ok" />;
    },
  };
});

vi.mock('./ObjectInfoPanel', async () => {
  const { useAppState } = await import('../hooks/useDocumentState');
  return {
    ObjectSourcePanel: () => {
      const node = selectedNodeId(useAppState());
      if (node !== null && node === failOn.source) throw new Error(`source fails on ${node}`);
      return <div data-testid="object-source-ok" />;
    },
  };
});

vi.mock('./PagesPanel', async () => {
  const { useAppState } = await import('../hooks/useDocumentState');
  return {
    PagesPanel: () => {
      const { activeTabId } = useAppState();
      if (activeTabId !== null && activeTabId === failOn.pagesTab) throw new Error('pages fails');
      return <div data-testid="pages-panel-ok" />;
    },
  };
});

class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

let consoleError: ReturnType<typeof vi.spyOn>;

// React's dev build reports a caught render error as a window error event,
// which jsdom prints unless the event is cancelled.
function swallowWindowError(event: ErrorEvent) {
  event.preventDefault();
}

beforeEach(() => {
  failOn.detail = null;
  failOn.source = null;
  failOn.pagesTab = null;
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
  window.addEventListener('error', swallowWindowError);
  // React and the boundary both log the caught render error.
  consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  consoleError.mockRestore();
  window.removeEventListener('error', swallowWindowError);
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

let dispatch: (action: AppAction) => void = () => {};

function DispatchCapture() {
  dispatch = useAppDispatch();
  return null;
}

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

function send(action: AppAction) {
  act(() => {
    dispatch(action);
  });
}

function selectNode(nodeId: string) {
  send({ type: 'SELECT_NODE', payload: { nodeId } });
}

function openDocument(tabId: string) {
  send({
    type: 'OPEN_DOCUMENT',
    payload: {
      tabId,
      fileName: `${tabId}.pdf`,
      filePath: `/path/to/${tabId}.pdf`,
      rootNode: catalogNode,
      rootChildren: [],
    },
  });
}

function renderOpen() {
  render(
    <AppProvider>
      <DispatchCapture />
      <MainLayout />
    </AppProvider>
  );
  openDocument('tab-1');
}

function fallbackCount(): number {
  return screen.queryAllByText('Something went wrong rendering this panel.').length;
}

const panels = [
  { name: 'detail', key: 'detail', okTestId: 'detail-panel-ok' },
  { name: 'object source', key: 'source', okTestId: 'object-source-ok' },
] as const;

describe('MainLayout selection-driven panel recovery', () => {
  test.each(panels)('selecting another node renders a failed $name panel again', ({ key, okTestId }) => {
    failOn[key] = 'obj:0:3';
    renderOpen();
    selectNode('obj:0:3');
    expect(screen.queryByTestId(okTestId)).not.toBeInTheDocument();
    expect(fallbackCount()).toBe(1);

    selectNode('obj:0:4');

    expect(screen.getByTestId(okTestId)).toBeInTheDocument();
    expect(fallbackCount()).toBe(0);
  });

  test.each(panels)('a history step renders a failed $name panel again', ({ key, okTestId }) => {
    failOn[key] = 'obj:0:4';
    renderOpen();
    selectNode('obj:0:3');
    selectNode('obj:0:4');
    expect(fallbackCount()).toBe(1);

    send({ type: 'NAVIGATE_BACK' });

    expect(screen.getByTestId(okTestId)).toBeInTheDocument();
    expect(fallbackCount()).toBe(0);
  });

  test.each(panels)('opening another document renders a failed $name panel again', ({ key, okTestId }) => {
    failOn[key] = 'obj:0:3';
    renderOpen();
    selectNode('obj:0:3');
    expect(fallbackCount()).toBe(1);

    openDocument('tab-2');

    expect(screen.getByTestId(okTestId)).toBeInTheDocument();
    expect(fallbackCount()).toBe(0);
  });

  test.each(panels)('selecting a node whose $name panel fails catches the error once', ({ key }) => {
    failOn[key] = 'obj:0:3';
    renderOpen();
    selectNode('obj:0:4');
    consoleError.mockClear();

    selectNode('obj:0:3');

    const caught = consoleError.mock.calls.filter((args) => args[0] === 'ErrorBoundary caught:');
    expect(caught).toHaveLength(1);
    expect(fallbackCount()).toBe(1);
  });

  test('a node selection does not reset a failed navigator', () => {
    failOn.pagesTab = 'tab-1';
    renderOpen();
    expect(fallbackCount()).toBe(1);

    failOn.pagesTab = null;
    selectNode('obj:0:4');

    expect(screen.queryByTestId('pages-panel-ok')).not.toBeInTheDocument();
    expect(fallbackCount()).toBe(1);
  });
});
