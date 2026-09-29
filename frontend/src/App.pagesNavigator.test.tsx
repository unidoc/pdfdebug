/**
 * Cmd/Ctrl+G and the native Navigate > Go to Page menu item open the Pages
 * navigator and focus its jump field. There is no Go to Page modal.
 *
 * Run: cd frontend && npx vitest run src/App.pagesNavigator.test.tsx
 */
import { render, screen, act, fireEvent, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { ReactNode } from 'react';

const RAIL_KEY = 'unidoc-pdf-debugger:left-rail';

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

vi.mock('allotment', () => {
  function Pane({ children, visible }: { children: ReactNode; visible?: boolean }) {
    return <div data-testid="allotment-pane" data-visible={visible === false ? 'false' : 'true'}>{children}</div>;
  }
  function Allotment({ children, vertical }: { children: ReactNode; vertical?: boolean }) {
    return <div data-testid={vertical ? 'allotment-vertical' : 'allotment-horizontal'}>{children}</div>;
  }
  Allotment.Pane = Pane;
  return { Allotment };
});

vi.mock('allotment/dist/style.css', () => ({}));

const mockGetPageIndex = vi.fn();

vi.mock('../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: vi.fn(),
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
  SaveBytesToFile: vi.fn().mockResolvedValue(''),
  DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  GetPageIndex: (...args: unknown[]) => mockGetPageIndex(...args),
}));

vi.mock('./components/UpdateNotifier', () => ({
  UpdateNotifier: () => null,
}));

class MockResizeObserver {
  callback: ResizeObserverCallback;
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }
  observe(target: Element) {
    this.callback(
      [{ target, contentRect: { width: 300, height: 600 } as DOMRectReadOnly } as ResizeObserverEntry],
      this as unknown as ResizeObserver,
    );
  }
  unobserve() {}
  disconnect() {}
}

function emitEvent(name: string, data: Record<string, unknown> = {}) {
  for (const handler of eventHandlers[name] ?? []) handler({ data });
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

function page(pageNum: number, objNum: number) {
  return {
    pageNum,
    objNum,
    gen: 0,
    nodeId: `obj:0:${objNum}`,
    contentNodeId: '',
    mediaBox: [0, 0, 612, 792],
    rotate: 0,
    inherited: 0,
    annotCount: 0,
    contentLen: 0,
    error: '',
  };
}

async function renderAppWithDocument(pageCount = 2) {
  const { default: App } = await import('./App');
  render(<App />);
  act(() => {
    emitEvent('document:opened', {
      tabId: 'tab-1',
      fileName: 'a.pdf',
      filePath: '/tmp/a.pdf',
      pageCount,
      rootNode: catalogNode,
      rootChildren: [],
    });
  });
  await waitFor(() => expect(screen.getByTestId('main-layout')).toBeInTheDocument());
}

function rail() {
  const lists = screen.getAllByRole('tablist').filter((l) => l.getAttribute('aria-orientation') === 'vertical');
  expect(lists).toHaveLength(1);
  return lists[0];
}

function tab(name: string) {
  return within(rail()).getByRole('tab', { name });
}

function jumpField() {
  const panel = document.getElementById(tab('Pages').getAttribute('aria-controls') ?? '');
  expect(panel).not.toBeNull();
  return within(panel!).getByRole('textbox', { name: /go to page/i });
}

function leftPaneCollapsed() {
  const horizontal = screen.getByTestId('allotment-horizontal');
  return within(horizontal).getAllByTestId('allotment-pane')[0].getAttribute('data-visible') === 'false';
}

function pressCmdG(target: EventTarget = document.body) {
  fireEvent.keyDown(target, { key: 'g', metaKey: true, ctrlKey: true });
}

beforeEach(() => {
  for (const key of Object.keys(eventHandlers)) delete eventHandlers[key];
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
  window.localStorage.removeItem(RAIL_KEY);
  mockGetPageIndex.mockReset().mockResolvedValue([page(1, 3), page(2, 5)]);
});

afterEach(() => {
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

describe('Cmd/Ctrl+G opens the Pages navigator', () => {
  test('selects Pages and focuses the jump field', async () => {
    await renderAppWithDocument();
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    pressCmdG();
    expect(tab('Pages')).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(jumpField()).toHaveFocus());
  });

  test('opens no dialog', async () => {
    await renderAppWithDocument();
    pressCmdG();
    await waitFor(() => expect(jumpField()).toHaveFocus());
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  test('restores a collapsed left panel', async () => {
    const user = userEvent.setup();
    await renderAppWithDocument();
    await user.click(tab('Structure'));
    expect(leftPaneCollapsed()).toBe(true);
    pressCmdG();
    expect(leftPaneCollapsed()).toBe(false);
    await waitFor(() => expect(jumpField()).toHaveFocus());
  });

  test('works on a document whose page count is 0', async () => {
    await renderAppWithDocument(0);
    pressCmdG();
    expect(tab('Pages')).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(jumpField()).toHaveFocus());
  });

  test('is ignored while typing in a text field', async () => {
    await renderAppWithDocument();
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    pressCmdG(input);
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    input.remove();
  });

  test('is ignored with no document open', async () => {
    const { default: App } = await import('./App');
    render(<App />);
    pressCmdG();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.queryAllByRole('tablist').filter((l) => l.getAttribute('aria-orientation') === 'vertical')).toHaveLength(0);
  });
});

describe('the native Go to Page menu item opens the Pages navigator', () => {
  test('navigate:goToPage selects Pages and focuses the jump field', async () => {
    await renderAppWithDocument();
    act(() => emitEvent('navigate:goToPage'));
    expect(tab('Pages')).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(jumpField()).toHaveFocus());
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  test('a second request refocuses the field after focus moved away', async () => {
    await renderAppWithDocument();
    act(() => emitEvent('navigate:goToPage'));
    await waitFor(() => expect(jumpField()).toHaveFocus());
    tab('Structure').focus();
    act(() => emitEvent('navigate:goToPage'));
    await waitFor(() => expect(jumpField()).toHaveFocus());
  });

  test('navigate:goToPage with no document open does nothing', async () => {
    const { default: App } = await import('./App');
    render(<App />);
    act(() => emitEvent('navigate:goToPage'));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(mockGetPageIndex).not.toHaveBeenCalled();
  });
});
