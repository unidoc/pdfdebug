/**
 * A render error inside one navigator panel is contained to that panel: the
 * other navigators, the object source pane and the detail panel stay up, and
 * a tab change gives the failed panel another render.
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { AppProvider } from '../hooks/useDocumentState';
import { MainLayout } from './MainLayout';
import {
  DispatchCapture,
  caughtCount,
  openDocument,
  setupBoundaryTestEnv,
} from './mainLayoutTestUtils';

vi.mock('allotment', async () => (await import('./mainLayoutTestUtils')).allotmentMock());
vi.mock('allotment/dist/style.css', () => ({}));
vi.mock('../hooks/useWindowPersistence', async () =>
  (await import('./mainLayoutTestUtils')).windowPersistenceMock());
vi.mock('../../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', async () =>
  (await import('./mainLayoutTestUtils')).pdfServiceMock());

const pages = vi.hoisted(() => ({ fail: true }));

vi.mock('./PagesPanel', () => ({
  PagesPanel: () => {
    if (pages.fail) throw new Error('pages panel render failure');
    return <div data-testid="pages-panel-ok" />;
  },
}));

const consoleError = setupBoundaryTestEnv();

beforeEach(() => {
  pages.fail = true;
});

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
    consoleError().mockClear();
    openDocument('tab-2');

    expect(caughtCount(consoleError())).toBe(1);
    expect(screen.getAllByText('Something went wrong rendering this panel.')).toHaveLength(1);
  });
});
