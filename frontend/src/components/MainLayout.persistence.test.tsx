/**
 * OS File Association, Single Instance, and Window Persistence
 *
 * Integration tests for panel size persistence in MainLayout:
 *   - MainLayout reads persisted panel sizes from useWindowPersistence
 *   - MainLayout passes defaultSizes to Allotment when persisted values exist
 *   - MainLayout calls savePanelSizes when Allotment onChange fires
 *   - MainLayout omits defaultSizes when no persisted values (falls back to preferredSize)
 *
 * Run: cd frontend && npx vitest run src/components/MainLayout.persistence.test.tsx
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  AppProvider,
} from '../hooks/useDocumentState';
import { MainLayout } from './MainLayout';

// Track props passed to Allotment to verify wiring. `panePreferredSizes`
// holds, per Allotment instance, the preferredSize prop on each direct Pane
// child read from the React element tree (so the parent->child relationship
// is preserved without depending on Pane render order).
import { Children, isValidElement } from 'react';

const allotmentInstances: Array<{
  onChange?: (sizes: number[]) => void;
  vertical?: boolean;
  panePreferredSizes: Array<number | string | undefined>;
}> = [];

vi.mock('allotment', () => {
  function Pane({ children }: { children: React.ReactNode }) {
    return <div data-testid="allotment-pane">{children}</div>;
  }
  function Allotment({
    children,
    onChange,
    vertical,
  }: {
    children: React.ReactNode;
    onChange?: (sizes: number[]) => void;
    vertical?: boolean;
  }) {
    const panePreferredSizes = Children.toArray(children).map((child) =>
      isValidElement<{ preferredSize?: number | string }>(child)
        ? child.props.preferredSize
        : undefined,
    );
    allotmentInstances.push({ onChange, vertical: !!vertical, panePreferredSizes });
    return (
      <div data-testid={vertical ? 'allotment-vertical' : 'allotment-horizontal'}>
        {children}
      </div>
    );
  }
  Allotment.Pane = Pane;
  return { Allotment };
});

vi.mock('allotment/dist/style.css', () => ({}));

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

// Mock useWindowPersistence -- the hook under integration test
const mockSavePanelSizes = vi.fn();
let mockPanelSizes: { treeWidth: number; subPanelHeight: number } | null = null;

vi.mock('../hooks/useWindowPersistence', () => ({
  useWindowPersistence: () => ({
    panelSizes: mockPanelSizes,
    savePanelSizes: mockSavePanelSizes,
  }),
}));

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
  allotmentInstances.length = 0;
  mockPanelSizes = null;
  mockSavePanelSizes.mockClear();
});

afterEach(() => {
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

describe('MainLayout panel persistence', () => {
  /**
   * MainLayout passes persisted treeWidth as the left pane's preferredSize.
   *
   * Panel sizes restored from localStorage on app start.
   */
  test('passes persisted treeWidth as preferredSize on left horizontal pane', () => {
    mockPanelSizes = { treeWidth: 400, subPanelHeight: 200 };

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    const horizontal = allotmentInstances.find((a) => !a.vertical);
    expect(horizontal).toBeDefined();
    expect(horizontal!.panePreferredSizes[0]).toBe(400);
  });

  /**
   * MainLayout passes persisted subPanelHeight as the bottom vertical pane's preferredSize.
   *
   * Sub-panel height is restored.
   */
  test('passes persisted subPanelHeight as preferredSize on bottom vertical pane', () => {
    mockPanelSizes = { treeWidth: 400, subPanelHeight: 200 };

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    const vertical = allotmentInstances.find((a) => a.vertical);
    expect(vertical).toBeDefined();
    // Second pane is the ObjectInfoPanel (sub-panel)
    expect(vertical!.panePreferredSizes[1]).toBe(200);
  });

  /**
   * MainLayout uses fallback preferredSize values when no persisted state exists.
   *
   * Falls back to default panel sizes (300px tree, 30% sub-panel).
   */
  test('uses fallback preferredSize when panelSizes is null', () => {
    mockPanelSizes = null;

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    const horizontal = allotmentInstances.find((a) => !a.vertical);
    const vertical = allotmentInstances.find((a) => a.vertical);
    expect(horizontal!.panePreferredSizes[0]).toBe(300);
    expect(vertical!.panePreferredSizes[1]).toBe('30%');
  });

  /**
   * MainLayout calls savePanelSizes when horizontal Allotment onChange fires.
   *
   * Panel sizes are saved on resize.
   */
  test('calls savePanelSizes when horizontal Allotment onChange fires', () => {
    mockPanelSizes = { treeWidth: 300, subPanelHeight: 150 };

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    // Simulate horizontal Allotment onChange
    const horizontal = allotmentInstances.find((a) => !a.vertical);
    expect(horizontal).toBeDefined();
    expect(horizontal!.onChange).toBeDefined();

    horizontal!.onChange!([350, 674]);

    // savePanelSizes should be called with the new tree width
    expect(mockSavePanelSizes).toHaveBeenCalled();
    const lastCall = mockSavePanelSizes.mock.calls[mockSavePanelSizes.mock.calls.length - 1][0];
    expect(lastCall.treeWidth).toBe(350);
  });

  /**
   * MainLayout calls savePanelSizes when vertical Allotment onChange fires.
   *
   * Sub-panel height is saved on resize.
   */
  test('calls savePanelSizes when vertical Allotment onChange fires', () => {
    mockPanelSizes = { treeWidth: 300, subPanelHeight: 150 };

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    // Simulate vertical Allotment onChange
    const vertical = allotmentInstances.find((a) => a.vertical);
    expect(vertical).toBeDefined();
    expect(vertical!.onChange).toBeDefined();

    vertical!.onChange!([400, 200]);

    // savePanelSizes should be called with the new sub-panel height
    expect(mockSavePanelSizes).toHaveBeenCalled();
    const lastCall = mockSavePanelSizes.mock.calls[mockSavePanelSizes.mock.calls.length - 1][0];
    expect(lastCall.subPanelHeight).toBe(200);
  });

  /**
   * MainLayout renders without error when useWindowPersistence returns null.
   *
   * No error on empty/corrupt localStorage.
   */
  test('renders successfully with null panelSizes', () => {
    mockPanelSizes = null;

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    expect(screen.getByTestId('main-layout')).toBeInTheDocument();
  });

  /**
   * Vertical onChange saves treePaneHeight (sizes[0]) in addition to subPanelHeight.
   */
  test('vertical onChange saves treePaneHeight from sizes[0]', () => {
    mockPanelSizes = { treeWidth: 300, subPanelHeight: 150 };

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    const vertical = allotmentInstances.find((a) => a.vertical);
    expect(vertical).toBeDefined();

    vertical!.onChange!([380, 220]);

    expect(mockSavePanelSizes).toHaveBeenCalled();
    const lastCall = mockSavePanelSizes.mock.calls[mockSavePanelSizes.mock.calls.length - 1][0];
    expect(lastCall.treePaneHeight).toBe(380);
    expect(lastCall.subPanelHeight).toBe(220);
  });

  /**
   * Horizontal onChange with invalid (empty) sizes array does not call savePanelSizes.
   */
  test('horizontal onChange ignores empty sizes array', () => {
    mockPanelSizes = { treeWidth: 300, subPanelHeight: 150 };

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    const horizontal = allotmentInstances.find((a) => !a.vertical);
    expect(horizontal).toBeDefined();

    mockSavePanelSizes.mockClear();
    horizontal!.onChange!([]);

    expect(mockSavePanelSizes).not.toHaveBeenCalled();
  });

  /**
   * Vertical onChange with fewer than 2 entries does not call savePanelSizes.
   */
  test('vertical onChange ignores sizes with fewer than 2 entries', () => {
    mockPanelSizes = { treeWidth: 300, subPanelHeight: 150 };

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    const vertical = allotmentInstances.find((a) => a.vertical);
    expect(vertical).toBeDefined();

    mockSavePanelSizes.mockClear();
    vertical!.onChange!([400]);

    expect(mockSavePanelSizes).not.toHaveBeenCalled();
  });
});
