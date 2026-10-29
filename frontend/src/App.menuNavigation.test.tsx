/**
 * The native Navigate > Back and Forward menu items step the selection
 * history from App, outside the detail panel, so they also render a detail
 * panel that failed on the current node again.
 *
 * Run: cd frontend && npx vitest run src/App.menuNavigation.test.tsx
 */
import { render, screen, act, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import type { AppAction } from './hooks/useDocumentState';
import { catalogNode, setupBoundaryTestEnv } from './components/mainLayoutTestUtils';

type EventHandler = (event: { data: Record<string, unknown> }) => void;
const eventHandlers: Record<string, EventHandler[]> = {};

vi.mock('@wailsio/runtime', async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
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
  Window: {
    SetSize: vi.fn().mockResolvedValue(undefined),
    SetPosition: vi.fn().mockResolvedValue(undefined),
    Position: vi.fn().mockResolvedValue({ x: 0, y: 0 }),
    Size: vi.fn().mockResolvedValue({ width: 1200, height: 800 }),
  },
  Screens: {
    GetAll: vi.fn().mockResolvedValue([]),
  },
}));

vi.mock('allotment', async () => (await import('./components/mainLayoutTestUtils')).allotmentMock());
vi.mock('allotment/dist/style.css', () => ({}));

vi.mock('../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: vi.fn(),
  SetWatchedTabs: vi.fn().mockResolvedValue(undefined),
  GetTreeRoot: vi.fn(),
  GetChildren: vi.fn().mockResolvedValue([]),
  CloseDocument: vi.fn().mockResolvedValue(undefined),
  OpenFileDialog: vi.fn(),
  GetObjectDetail: vi.fn().mockResolvedValue(null),
  GetObjectSource: vi.fn().mockResolvedValue(''),
  GetReverseRefs: vi.fn().mockResolvedValue([]),
  GetAncestorPath: vi.fn().mockResolvedValue([]),
  GetXRefTable: vi.fn().mockResolvedValue({ tabId: '', entries: [] }),
  ConsumePendingOpenFiles: vi.fn().mockResolvedValue([]),
  GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
  GetSignatures: vi.fn().mockResolvedValue([]),
  GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
  GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
  SaveEmbeddedFileToFile: vi.fn().mockResolvedValue(''),
  DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  GetPageIndex: vi.fn().mockResolvedValue([]),
  GetImageIndex: vi.fn(),
  GetImagePages: vi.fn(),
  GetImagePageGroups: vi.fn(),
}));

vi.mock('./components/UpdateNotifier', () => ({
  UpdateNotifier: () => null,
}));

// failOn is the nodes the detail stub throws on. The object source stub
// stays up and hands its dispatch to the test, which selects nodes through it.
const ctl = vi.hoisted(() => ({
  failOn: [] as string[],
  dispatch: null as ((action: AppAction) => void) | null,
}));

vi.mock('./components/DetailPanel', async () => ({
  DetailPanel: (await import('./components/mainLayoutTestUtils')).failingSelectionPanel('detail-panel-ok', () => ctl.failOn),
}));

vi.mock('./components/ObjectInfoPanel', async () => {
  const { useAppDispatch } = await import('./hooks/useDocumentState');
  return {
    ObjectSourcePanel: () => {
      ctl.dispatch = useAppDispatch();
      return null;
    },
  };
});

function emitEvent(name: string, data: Record<string, unknown> = {}) {
  for (const handler of eventHandlers[name] ?? []) handler({ data });
}

setupBoundaryTestEnv();

beforeEach(() => {
  for (const key of Object.keys(eventHandlers)) delete eventHandlers[key];
  ctl.failOn = [];
  ctl.dispatch = null;
});

async function renderAppWithDocument() {
  const { default: App } = await import('./App');
  render(<App />);
  act(() => {
    emitEvent('document:opened', {
      tabId: 'tab-1',
      fileName: 'a.pdf',
      filePath: '/tmp/a.pdf',
      pageCount: 1,
      rootNode: catalogNode,
      rootChildren: [],
    });
  });
  await waitFor(() => expect(screen.getByTestId('main-layout')).toBeInTheDocument());
}

function selectNode(nodeId: string) {
  act(() => {
    ctl.dispatch!({ type: 'SELECT_NODE', payload: { nodeId } });
  });
}

function detailFailed(): boolean {
  return screen.queryByTestId('detail-panel-ok') === null
    && screen.queryAllByText('Something went wrong rendering this panel.').length === 1;
}

describe('Navigate menu Back and Forward with a failed detail panel', () => {
  test('Back renders the detail panel again', async () => {
    ctl.failOn = ['obj:0:4'];
    await renderAppWithDocument();
    selectNode('obj:0:3');
    selectNode('obj:0:4');
    expect(detailFailed()).toBe(true);

    act(() => emitEvent('navigate:back'));

    expect(screen.getByTestId('detail-panel-ok')).toBeInTheDocument();
  });

  test('Forward renders the detail panel again', async () => {
    ctl.failOn = ['obj:0:3'];
    await renderAppWithDocument();
    selectNode('obj:0:3');
    selectNode('obj:0:4');
    act(() => emitEvent('navigate:back'));
    expect(detailFailed()).toBe(true);

    act(() => emitEvent('navigate:forward'));

    expect(screen.getByTestId('detail-panel-ok')).toBeInTheDocument();
  });
});
