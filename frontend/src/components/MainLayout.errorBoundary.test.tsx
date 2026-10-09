/**
 * A render error inside one navigator panel is contained to that panel: the
 * other navigators, the object source pane and the detail panel stay up.
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { AppProvider } from '../hooks/useDocumentState';
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

vi.mock('./PagesPanel', () => ({
  PagesPanel: () => {
    throw new Error('pages panel render failure');
  },
}));

class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

let consoleError: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
  // React and the boundary both log the caught render error.
  consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  consoleError.mockRestore();
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

describe('MainLayout navigator error containment', () => {
  test('a throwing navigator panel leaves the detail panel and the other navigators rendered', () => {
    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    expect(screen.getByTestId('right-panel')).toBeInTheDocument();
    expect(screen.getByTestId('tree-panel')).toBeInTheDocument();
    expect(screen.getByTestId('images-panel')).toBeInTheDocument();
    expect(screen.getAllByText('Something went wrong rendering this panel.')).toHaveLength(1);
  });
});
