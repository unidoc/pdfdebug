/**
 * @file Shared setup for the MainLayout error boundary tests. vi.mock calls
 * stay in each test file (Vitest hoists them per file); their factories import
 * the mock modules from here.
 */
import type { ReactNode } from 'react';
import { act } from '@testing-library/react';
import { vi, beforeEach, afterEach, type MockInstance } from 'vitest';
import {
  selectActiveTab,
  useAppDispatch,
  useAppState,
  type AppAction,
  type TreeNode,
} from '../hooks/useDocumentState';

/** Mock for the allotment module: panes render their children unsized. */
export function allotmentMock() {
  function Pane({ children }: { children: ReactNode }) {
    return <div>{children}</div>;
  }
  function Allotment({ children }: { children: ReactNode }) {
    return <div>{children}</div>;
  }
  Allotment.Pane = Pane;
  return { Allotment };
}

/**
 * A panel stub that throws while the active tab's selected node is in
 * failOn(), and otherwise renders an element with testId.
 */
export function failingSelectionPanel(testId: string, failOn: () => readonly string[]) {
  return function FailingSelectionPanel() {
    const node = selectActiveTab(useAppState())?.selectedNodeId ?? null;
    if (node !== null && failOn().includes(node)) throw new Error(`${testId} fails on ${node}`);
    return <div data-testid={testId} />;
  };
}

/** Mock for useWindowPersistence: no stored sizes, saves ignored. */
export function windowPersistenceMock() {
  return {
    useWindowPersistence: () => ({
      panelSizes: null,
      savePanelSizes: vi.fn(),
    }),
  };
}

/** Mock for the pdfservice bindings MainLayout's panels import. */
export function pdfServiceMock() {
  return {
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
  };
}

class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

// React's dev build reports a caught render error as a window error event,
// which jsdom prints unless the event is cancelled.
function swallowWindowError(event: ErrorEvent) {
  event.preventDefault();
}

/**
 * Registers per-test setup: a ResizeObserver stub, silenced caught render
 * errors, and a console.error spy. Returns a getter for the current spy.
 */
export function setupBoundaryTestEnv(): () => MockInstance {
  let consoleError: MockInstance;
  beforeEach(() => {
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
  return () => consoleError;
}

/** Number of errors the boundaries logged through the spy. */
export function caughtCount(consoleError: MockInstance): number {
  return consoleError.mock.calls.filter((args) => args[0] === 'ErrorBoundary caught:').length;
}

let dispatch: (action: AppAction) => void = () => {};

/** Captures the app dispatch for send(); render it inside AppProvider. */
export function DispatchCapture() {
  dispatch = useAppDispatch();
  return null;
}

/** Dispatches an action inside act(). */
export function send(action: AppAction) {
  act(() => {
    dispatch(action);
  });
}

/** Root node for a document opened in a test. */
export const catalogNode: TreeNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 0,
  iconHint: 'catalog',
  error: '',
  objectRef: '',
  typeName: 'Catalog',
  value: '',
  valueRaw: '',
};

/** Opens a document in a new tab, which becomes the active tab. */
export function openDocument(tabId: string) {
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

/** Selects a node in the active tab. */
export function selectNode(nodeId: string) {
  send({ type: 'SELECT_NODE', payload: { nodeId } });
}
