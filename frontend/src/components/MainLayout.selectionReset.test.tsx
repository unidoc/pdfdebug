/**
 * The object source and detail panels render the selected node, so after a
 * render error a new selection, a history step or a tab change gives them
 * another render. The navigator panels reset on a tab change only. The
 * fallback's Try again button renders a failed panel again in place.
 */
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { AppProvider } from '../hooks/useDocumentState';
import { MainLayout } from './MainLayout';
import {
  DispatchCapture,
  caughtCount,
  openDocument,
  selectNode,
  send,
  setupBoundaryTestEnv,
} from './mainLayoutTestUtils';

vi.mock('allotment', async () => (await import('./mainLayoutTestUtils')).allotmentMock());
vi.mock('allotment/dist/style.css', () => ({}));
vi.mock('../hooks/useWindowPersistence', async () =>
  (await import('./mainLayoutTestUtils')).windowPersistenceMock());
vi.mock('../../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', async () =>
  (await import('./mainLayoutTestUtils')).pdfServiceMock());

// The nodes each panel throws on, and the tab the Pages navigator throws on.
const failOn = vi.hoisted(() => ({
  detail: [] as string[],
  source: [] as string[],
  pagesTab: null as string | null,
}));

vi.mock('./DetailPanel', async () => ({
  DetailPanel: (await import('./mainLayoutTestUtils')).failingSelectionPanel('detail-panel-ok', () => failOn.detail),
}));

vi.mock('./ObjectInfoPanel', async () => ({
  ObjectSourcePanel: (await import('./mainLayoutTestUtils')).failingSelectionPanel('object-source-ok', () => failOn.source),
}));

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

const consoleError = setupBoundaryTestEnv();

beforeEach(() => {
  failOn.detail = [];
  failOn.source = [];
  failOn.pagesTab = null;
});

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
    failOn[key] = ['obj:0:3'];
    renderOpen();
    selectNode('obj:0:3');
    expect(screen.queryByTestId(okTestId)).not.toBeInTheDocument();
    expect(fallbackCount()).toBe(1);

    selectNode('obj:0:4');

    expect(screen.getByTestId(okTestId)).toBeInTheDocument();
    expect(fallbackCount()).toBe(0);
  });

  test.each(panels)('a Back step renders a failed $name panel again', ({ key, okTestId }) => {
    failOn[key] = ['obj:0:4'];
    renderOpen();
    selectNode('obj:0:3');
    selectNode('obj:0:4');
    expect(fallbackCount()).toBe(1);

    send({ type: 'NAVIGATE_BACK' });

    expect(screen.getByTestId(okTestId)).toBeInTheDocument();
    expect(fallbackCount()).toBe(0);
  });

  test.each(panels)('a Forward step renders a failed $name panel again', ({ key, okTestId }) => {
    failOn[key] = ['obj:0:3'];
    renderOpen();
    selectNode('obj:0:3');
    selectNode('obj:0:4');
    send({ type: 'NAVIGATE_BACK' });
    expect(fallbackCount()).toBe(1);

    send({ type: 'NAVIGATE_FORWARD' });

    expect(screen.getByTestId(okTestId)).toBeInTheDocument();
    expect(fallbackCount()).toBe(0);
  });

  test.each(panels)('opening another document renders a failed $name panel again', ({ key, okTestId }) => {
    failOn[key] = ['obj:0:3'];
    renderOpen();
    selectNode('obj:0:3');
    expect(fallbackCount()).toBe(1);

    openDocument('tab-2');

    expect(screen.getByTestId(okTestId)).toBeInTheDocument();
    expect(fallbackCount()).toBe(0);
  });

  test.each(panels)('switching to an open tab renders a failed $name panel again', ({ key, okTestId }) => {
    failOn[key] = ['obj:0:3'];
    renderOpen();
    selectNode('obj:0:4');
    openDocument('tab-2');
    selectNode('obj:0:3');
    expect(fallbackCount()).toBe(1);

    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } });

    expect(screen.getByTestId(okTestId)).toBeInTheDocument();
    expect(fallbackCount()).toBe(0);
  });

  test.each(panels)('closing the active tab renders a failed $name panel again', ({ key, okTestId }) => {
    failOn[key] = ['obj:0:3'];
    renderOpen();
    selectNode('obj:0:4');
    openDocument('tab-2');
    selectNode('obj:0:3');
    expect(fallbackCount()).toBe(1);

    send({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-2' } });

    expect(screen.getByTestId(okTestId)).toBeInTheDocument();
    expect(fallbackCount()).toBe(0);
  });

  test.each(panels)('selecting a node whose $name panel fails catches the error once', ({ key }) => {
    failOn[key] = ['obj:0:3'];
    renderOpen();
    selectNode('obj:0:4');
    consoleError().mockClear();

    selectNode('obj:0:3');

    expect(caughtCount(consoleError())).toBe(1);
    expect(fallbackCount()).toBe(1);
  });

  test.each(panels)('Try again renders a failed $name panel on the same node', async ({ key, okTestId }) => {
    const user = userEvent.setup();
    failOn[key] = ['obj:0:3'];
    renderOpen();
    selectNode('obj:0:3');
    expect(fallbackCount()).toBe(1);

    failOn[key] = [];
    await user.click(screen.getByRole('button', { name: 'Try again' }));

    expect(screen.getByTestId(okTestId)).toBeInTheDocument();
    expect(fallbackCount()).toBe(0);
  });

  test.each(panels)('a $name panel that fails on the next node too catches the error once', ({ key }) => {
    failOn[key] = ['obj:0:3', 'obj:0:4'];
    renderOpen();
    selectNode('obj:0:3');
    expect(fallbackCount()).toBe(1);
    consoleError().mockClear();

    selectNode('obj:0:4');

    expect(caughtCount(consoleError())).toBe(1);
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
