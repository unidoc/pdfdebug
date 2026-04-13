/**
 * Story 4.4: OS File Association, Single Instance, and Window Persistence
 *
 * TDD RED PHASE: Tests MUST fail until story 4-4 is implemented.
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

// Track props passed to Allotment to verify defaultSizes and onChange
const allotmentInstances: Array<{
  defaultSizes?: number[];
  onChange?: (sizes: number[]) => void;
  vertical?: boolean;
}> = [];

// Mock allotment -- capture props for verification
vi.mock('allotment', () => {
  function Pane({ children }: { children: React.ReactNode }) {
    return <div data-testid="allotment-pane">{children}</div>;
  }
  function Allotment({
    children,
    defaultSizes,
    onChange,
    vertical,
  }: {
    children: React.ReactNode;
    defaultSizes?: number[];
    onChange?: (sizes: number[]) => void;
    vertical?: boolean;
  }) {
    allotmentInstances.push({ defaultSizes, onChange, vertical });
    return (
      <div
        data-testid={vertical ? 'allotment-vertical' : 'allotment-horizontal'}
        data-default-sizes={defaultSizes ? JSON.stringify(defaultSizes) : undefined}
      >
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

describe('4.4 MainLayout Panel Persistence', () => {
  /**
   * MainLayout passes persisted defaultSizes to horizontal Allotment
   * when useWindowPersistence returns panel sizes.
   *
   * AC#3: Panel sizes restored from localStorage on app start.
   */
  test('passes persisted treeWidth as defaultSizes to horizontal Allotment', () => {
    mockPanelSizes = { treeWidth: 400, subPanelHeight: 200 };

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    // Find the horizontal Allotment instance (vertical=undefined or false)
    const horizontal = allotmentInstances.find((a) => !a.vertical);
    expect(horizontal).toBeDefined();
    expect(horizontal!.defaultSizes).toBeDefined();
    // First entry should be the persisted tree width
    expect(horizontal!.defaultSizes![0]).toBe(400);
  });

  /**
   * MainLayout passes persisted subPanelHeight as defaultSizes to vertical Allotment.
   *
   * AC#3: Sub-panel height is restored.
   */
  test('passes persisted subPanelHeight as defaultSizes to vertical Allotment', () => {
    mockPanelSizes = { treeWidth: 400, subPanelHeight: 200 };

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    // Find the vertical Allotment instance
    const vertical = allotmentInstances.find((a) => a.vertical);
    expect(vertical).toBeDefined();
    expect(vertical!.defaultSizes).toBeDefined();
    // The sub-panel height (second pane) should be in the defaultSizes
    const lastSize = vertical!.defaultSizes![vertical!.defaultSizes!.length - 1];
    expect(lastSize).toBe(200);
  });

  /**
   * MainLayout omits defaultSizes when no persisted values exist.
   *
   * AC#3: Falls back to default panel sizes (preferredSize props).
   */
  test('omits defaultSizes when panelSizes is null (no persisted state)', () => {
    mockPanelSizes = null;

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    // Both Allotment instances should have no defaultSizes
    for (const instance of allotmentInstances) {
      expect(instance.defaultSizes).toBeUndefined();
    }
  });

  /**
   * MainLayout calls savePanelSizes when horizontal Allotment onChange fires.
   *
   * AC#3: Panel sizes are saved on resize.
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
   * AC#3: Sub-panel height is saved on resize.
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
   * AC#3: No error on empty/corrupt localStorage.
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
   * When treePaneHeight is persisted, vertical Allotment gets
   * [treePaneHeight, subPanelHeight] as defaultSizes.
   */
  test('passes treePaneHeight as first vertical defaultSize when persisted', () => {
    mockPanelSizes = { treeWidth: 350, subPanelHeight: 180 } as any;
    // Patch in treePaneHeight (the mock type is simplified)
    (mockPanelSizes as any).treePaneHeight = 420;

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    const vertical = allotmentInstances.find((a) => a.vertical);
    expect(vertical).toBeDefined();
    expect(vertical!.defaultSizes).toEqual([420, 180]);
  });

  /**
   * When treePaneHeight is absent, vertical Allotment uses the
   * [subPanelHeight * 2, subPanelHeight] fallback.
   */
  test('uses subPanelHeight*2 fallback for vertical when treePaneHeight is absent', () => {
    mockPanelSizes = { treeWidth: 350, subPanelHeight: 180 };

    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );

    const vertical = allotmentInstances.find((a) => a.vertical);
    expect(vertical).toBeDefined();
    expect(vertical!.defaultSizes).toEqual([360, 180]);
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
