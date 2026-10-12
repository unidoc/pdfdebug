/**
 * A render error inside one navigator panel is contained to that panel: the
 * other navigators, the object source pane and the detail panel stay up, and
 * a tab change gives the failed panel another render.
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

const pages = vi.hoisted(() => ({ fail: true }));

vi.mock('./PagesPanel', () => ({
  PagesPanel: () => {
    if (pages.fail) throw new Error('pages panel render failure');
    return <div data-testid="pages-panel-ok" />;
  },
}));

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
  pages.fail = true;
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

function openDocument(tabId: string) {
  act(() => {
    dispatch({
      type: 'OPEN_DOCUMENT',
      payload: {
        tabId,
        fileName: `${tabId}.pdf`,
        filePath: `/path/to/${tabId}.pdf`,
        rootNode: catalogNode,
        rootChildren: [],
      },
    });
  });
}

describe('MainLayout navigator error containment', () => {
  test('a throwing navigator panel leaves the detail panel and the other navigators rendered', () => {
    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    expect(screen.getByTestId('right-panel')).toBeInTheDocument();
    expect(screen.getByTestId('object-source-panel')).toBeInTheDocument();
    expect(screen.getByTestId('tree-panel')).toBeInTheDocument();
    expect(screen.getByTestId('images-panel')).toBeInTheDocument();
    expect(screen.getAllByText('Something went wrong rendering this panel.')).toHaveLength(1);
  });

  test('opening another document renders the failed navigator again', () => {
    render(
      <AppProvider>
        <DispatchCapture />
        <MainLayout />
      </AppProvider>
    );
    openDocument('tab-1');
    expect(screen.getAllByText('Something went wrong rendering this panel.')).toHaveLength(1);

    pages.fail = false;
    openDocument('tab-2');

    expect(screen.getByTestId('pages-panel-ok')).toBeInTheDocument();
    expect(screen.queryByText('Something went wrong rendering this panel.')).not.toBeInTheDocument();
  });

  test('switching to a document whose navigator fails catches the error once', () => {
    pages.fail = false;
    render(
      <AppProvider>
        <DispatchCapture />
        <MainLayout />
      </AppProvider>
    );
    openDocument('tab-1');
    expect(screen.getByTestId('pages-panel-ok')).toBeInTheDocument();

    pages.fail = true;
    consoleError.mockClear();
    openDocument('tab-2');

    const caught = consoleError.mock.calls.filter((args) => args[0] === 'ErrorBoundary caught:');
    expect(caught).toHaveLength(1);
    expect(screen.getAllByText('Something went wrong rendering this panel.')).toHaveLength(1);
  });
});
