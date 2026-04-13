/**
 * 2.4-UNIT-003 / 2.5-INTG: MainLayout renders TreePanel component.
 *
 * Updated for Story 2-5: MainLayout now uses TreePanel instead of inline static list.
 */
import { render, screen, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  type AppAction,
} from '../hooks/useDocumentState';
import { MainLayout } from './MainLayout';

// Mock allotment -- it requires browser layout APIs not available in jsdom
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

// Mock useWindowPersistence -- returns no persisted sizes for these tests
vi.mock('../hooks/useWindowPersistence', () => ({
  useWindowPersistence: () => ({
    panelSizes: null,
    savePanelSizes: vi.fn(),
  }),
}));

// Mock Wails bindings
vi.mock(
  '../../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: vi.fn(),
  })
);

// Mock ResizeObserver
class MockResizeObserver {
  callback: ResizeObserverCallback;
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }
  observe(target: Element) {
    this.callback(
      [
        {
          target,
          contentRect: { width: 300, height: 600 } as DOMRectReadOnly,
          borderBoxSize: [],
          contentBoxSize: [],
          devicePixelContentBoxSize: [],
        } as ResizeObserverEntry,
      ],
      this
    );
  }
  unobserve() {}
  disconnect() {}
}

beforeEach(() => {
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
});

afterEach(() => {
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 3,
  iconHint: 'catalog',
  error: '',
};

const childNodes = [
  {
    id: 'dict:root:Type',
    label: 'Type',
    rawKey: '/Type',
    nodeType: 'scalar',
    valueType: 'name',
    hasChildren: false,
    childCount: 0,
    iconHint: 'default',
    error: '',
  },
  {
    id: 'obj:0:2',
    label: 'Pages',
    rawKey: '/Pages',
    nodeType: 'dict',
    valueType: 'reference',
    hasChildren: true,
    childCount: 2,
    iconHint: 'page',
    error: '',
  },
];

function DispatchThenLayout({ action }: { action: AppAction }) {
  const dispatch = useAppDispatch();
  return (
    <div>
      <button data-testid="dispatch" onClick={() => dispatch(action)} />
      <MainLayout />
    </div>
  );
}

describe('2.4-UNIT-003: MainLayout tree content', () => {
  test('shows Document Structure header when no document is open', () => {
    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );
    expect(screen.getByText('Document Structure')).toBeInTheDocument();
  });

  test('shows Catalog root and children after OPEN_DOCUMENT', () => {
    const action: AppAction = {
      type: 'OPEN_DOCUMENT',
      payload: {
        tabId: 'tab-1',
        fileName: 'test.pdf',
        filePath: '/path/to/test.pdf',
        rootNode: catalogNode,
        rootChildren: childNodes,
      },
    };

    render(
      <AppProvider>
        <DispatchThenLayout action={action} />
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    expect(screen.getByText('Catalog')).toBeInTheDocument();
    expect(screen.getByText('Type')).toBeInTheDocument();
    expect(screen.getByText('Pages')).toBeInTheDocument();
  });

  test('uses TreePanel component with react-arborist tree', () => {
    const action: AppAction = {
      type: 'OPEN_DOCUMENT',
      payload: {
        tabId: 'tab-1',
        fileName: 'test.pdf',
        filePath: '/path/to/test.pdf',
        rootNode: catalogNode,
        rootChildren: childNodes,
      },
    };

    render(
      <AppProvider>
        <DispatchThenLayout action={action} />
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // TreePanel uses react-arborist which provides role="tree"
    expect(screen.getByRole('tree')).toBeInTheDocument();
    expect(screen.getByTestId('tree-panel')).toBeInTheDocument();
  });
});
