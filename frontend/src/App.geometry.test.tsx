/**
 * Window Geometry Persistence
 *
 * Integration tests for the App.jsx wiring of window-geometry events and
 * startup restore. These cover Tasks 4 and 5 of the story:
 *
 *   - Subscribe to common:WindowDidMove and common:WindowDidResize
 *   - Handlers read Window.Position()/Window.Size() and call saveWindowGeometry
 *   - On mount, persisted geometry is applied via Window.SetSize then
 *     Window.SetPosition (size-first ordering per Task 5.1)
 *   - Off-screen guard via Screens.GetAll() skips position restore but still
 *     applies size restore
 *   - Restore-feedback loop suppression: events fired during the restore
 *     window do NOT cause a re-save (Task 4.4 / R4)
 *   - Listeners unsubscribe on unmount (Task 4.3)
 *
 * Run: cd frontend && npx vitest run src/App.geometry.test.tsx
 */
import { render, act, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';

// ---------------------------------------------------------------------------
// Mock @wailsio/runtime: capture event handlers, expose Window + Screens stubs
// ---------------------------------------------------------------------------

type EventHandler = (event: { data?: unknown }) => void;
const eventHandlers: Record<string, EventHandler[]> = {};

const mockSetSize = vi.fn().mockResolvedValue(undefined);
const mockSetPosition = vi.fn().mockResolvedValue(undefined);
const mockPosition = vi.fn().mockResolvedValue({ x: 0, y: 0 });
const mockSize = vi.fn().mockResolvedValue({ width: 1024, height: 768 });

const mockScreensGetAll = vi.fn().mockResolvedValue([
  {
    ID: 'primary',
    Name: 'Primary',
    ScaleFactor: 1,
    X: 0,
    Y: 0,
    Size: { Width: 1920, Height: 1080 },
    Bounds: { X: 0, Y: 0, Width: 1920, Height: 1080 },
    PhysicalBounds: { X: 0, Y: 0, Width: 1920, Height: 1080 },
    WorkArea: { X: 0, Y: 0, Width: 1920, Height: 1040 },
    PhysicalWorkArea: { X: 0, Y: 0, Width: 1920, Height: 1040 },
    IsPrimary: true,
    Rotation: 0,
  },
]);

vi.mock('@wailsio/runtime', () => ({
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
    SetSize: (...args: unknown[]) => mockSetSize(...args),
    SetPosition: (...args: unknown[]) => mockSetPosition(...args),
    Position: () => mockPosition(),
    Size: () => mockSize(),
  },
  Screens: {
    GetAll: () => mockScreensGetAll(),
  },
}));

// Stub PDF service bindings so App imports cleanly.
vi.mock(
  '../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn().mockResolvedValue(undefined),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: vi.fn(),
    GetContentStream: vi.fn(),
    GetAncestorPath: vi.fn(),
    // Close the pre-existing 12-1 harness gap: without this stub the cold-start
    // drain rejects, emitting unhandled errors (Story 13.2 "ideally close the
    // pre-existing" clause).
    ConsumePendingOpenFiles: vi.fn().mockResolvedValue([]),
    // The Diff tab imports DiffDocuments; stub so the factory never
    // throws on the new export.
    DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  }),
);

// Stub child components to keep render light.
vi.mock('./components/EmptyState', () => ({
  EmptyState: () => <div data-testid="empty-state">Empty</div>,
}));
vi.mock('./components/MainLayout', () => ({
  MainLayout: () => <div data-testid="main-layout">Layout</div>,
}));
vi.mock('./components/ErrorBanner', () => ({
  ErrorBanner: ({ message }: { message: string }) => (
    <div data-testid="error-banner">{message}</div>
  ),
}));
vi.mock('./components/TabBar', () => ({
  TabBar: () => <div data-testid="tab-bar">Tabs</div>,
}));

const STORAGE_KEY = 'unidoc-pdf-debugger:window-state';

function clearStorage() {
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}

function emit(name: string, data: unknown = {}) {
  const handlers = eventHandlers[name] ?? [];
  for (const handler of [...handlers]) {
    handler({ data });
  }
}

beforeEach(() => {
  for (const key of Object.keys(eventHandlers)) delete eventHandlers[key];
  clearStorage();
  mockSetSize.mockClear();
  mockSetPosition.mockClear();
  mockPosition.mockReset();
  mockPosition.mockResolvedValue({ x: 0, y: 0 });
  mockSize.mockReset();
  mockSize.mockResolvedValue({ width: 1024, height: 768 });
  mockScreensGetAll.mockReset();
  mockScreensGetAll.mockResolvedValue([
    {
      ID: 'primary',
      Name: 'Primary',
      ScaleFactor: 1,
      X: 0,
      Y: 0,
      Size: { Width: 1920, Height: 1080 },
      Bounds: { X: 0, Y: 0, Width: 1920, Height: 1080 },
      PhysicalBounds: { X: 0, Y: 0, Width: 1920, Height: 1080 },
      WorkArea: { X: 0, Y: 0, Width: 1920, Height: 1040 },
      PhysicalWorkArea: { X: 0, Y: 0, Width: 1920, Height: 1040 },
      IsPrimary: true,
      Rotation: 0,
    },
  ]);
});

afterEach(() => {
  vi.useRealTimers();
});

// ---------------------------------------------------------------------------
// App subscribes to common:WindowDidMove + WindowDidResize
// ---------------------------------------------------------------------------

describe('8.4 App.jsx geometry wiring', () => {
  test('subscribes to common:WindowDidMove and common:WindowDidResize on mount', async () => {
    const { default: App } = await import('./App');

    render(<App />);

    await waitFor(() => {
      expect(eventHandlers['common:WindowDidMove']?.length ?? 0).toBeGreaterThan(0);
      expect(eventHandlers['common:WindowDidResize']?.length ?? 0).toBeGreaterThan(0);
    });
  });

  /**
   * WindowDidMove handler reads Window.Position() / Window.Size() and persists the
   * result via the geometry save path.
   */
  test('WindowDidMove handler persists current geometry to localStorage (after debounce)', async () => {
    vi.useFakeTimers();

    mockPosition.mockResolvedValue({ x: 250, y: 175 });
    mockSize.mockResolvedValue({ width: 1300, height: 850 });

    const { default: App } = await import('./App');
    render(<App />);

    // Wait for the listener to be registered.
    await vi.waitFor(() => {
      expect(eventHandlers['common:WindowDidMove']?.length ?? 0).toBeGreaterThan(0);
    });

    // Allow any startup-restore microtasks to settle (so the suppression
    // window has expired).
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    // Fire the move event.
    await act(async () => {
      emit('common:WindowDidMove', {});
      // Flush the Position()/Size() promises.
      await vi.advanceTimersByTimeAsync(0);
    });

    // Flush the 500ms debounce.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });

    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).not.toBeNull();
    const parsed = JSON.parse(stored!);
    expect(parsed.windowGeometry).toEqual({ x: 250, y: 175, width: 1300, height: 850 });
  });

  /**
   * WindowDidResize handler persists current geometry too.
   */
  test('WindowDidResize handler persists current geometry to localStorage', async () => {
    vi.useFakeTimers();

    mockPosition.mockResolvedValue({ x: 100, y: 50 });
    mockSize.mockResolvedValue({ width: 1600, height: 900 });

    const { default: App } = await import('./App');
    render(<App />);

    await vi.waitFor(() => {
      expect(eventHandlers['common:WindowDidResize']?.length ?? 0).toBeGreaterThan(0);
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    await act(async () => {
      emit('common:WindowDidResize', {});
      await vi.advanceTimersByTimeAsync(0);
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });

    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).not.toBeNull();
    const parsed = JSON.parse(stored!);
    expect(parsed.windowGeometry).toEqual({ x: 100, y: 50, width: 1600, height: 900 });
  });

  /**
   * Startup restore calls Window.SetSize THEN Window.SetPosition with the
   * persisted values, in that order (Task 5.1 ordering).
   */
  test('on mount, restore calls SetSize before SetPosition with persisted values', async () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        windowGeometry: { x: 300, y: 250, width: 1280, height: 800 },
      }),
    );

    const setSizeOrder: number[] = [];
    const setPositionOrder: number[] = [];
    let counter = 0;
    mockSetSize.mockImplementation(() => {
      setSizeOrder.push(++counter);
      return Promise.resolve();
    });
    mockSetPosition.mockImplementation(() => {
      setPositionOrder.push(++counter);
      return Promise.resolve();
    });

    const { default: App } = await import('./App');
    render(<App />);

    await waitFor(() => {
      expect(mockSetSize).toHaveBeenCalledWith(1280, 800);
      expect(mockSetPosition).toHaveBeenCalledWith(300, 250);
    });

    expect(setSizeOrder.length).toBe(1);
    expect(setPositionOrder.length).toBe(1);
    expect(setSizeOrder[0]).toBeLessThan(setPositionOrder[0]);
  });

  /**
   * When localStorage is empty, neither SetSize nor SetPosition is
   * called.
   */
  test('empty localStorage skips restore entirely', async () => {
    const { default: App } = await import('./App');
    render(<App />);

    // Allow startup effect to run.
    await new Promise((r) => setTimeout(r, 0));
    await waitFor(() => {
      expect(eventHandlers['common:WindowDidMove']?.length ?? 0).toBeGreaterThan(0);
    });

    expect(mockSetSize).not.toHaveBeenCalled();
    expect(mockSetPosition).not.toHaveBeenCalled();
  });

  /**
   * Off-screen guard: when persisted geometry's rectangle does not
   * intersect any screen's WorkArea, skip the position restore but STILL
   * apply the size restore.
   */
  test('off-screen position is skipped, size restore still applies', async () => {
    // Persisted geometry is far off-screen (e.g. external monitor at -3000,-2000)
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        windowGeometry: { x: -3000, y: -2000, width: 1280, height: 800 },
      }),
    );

    // Only the primary screen at 0,0..1920x1080 is connected.
    mockScreensGetAll.mockResolvedValue([
      {
        ID: 'primary',
        Name: 'Primary',
        ScaleFactor: 1,
        X: 0,
        Y: 0,
        Size: { Width: 1920, Height: 1080 },
        Bounds: { X: 0, Y: 0, Width: 1920, Height: 1080 },
        PhysicalBounds: { X: 0, Y: 0, Width: 1920, Height: 1080 },
        WorkArea: { X: 0, Y: 0, Width: 1920, Height: 1040 },
        PhysicalWorkArea: { X: 0, Y: 0, Width: 1920, Height: 1040 },
        IsPrimary: true,
        Rotation: 0,
      },
    ]);

    const { default: App } = await import('./App');
    render(<App />);

    await waitFor(() => {
      expect(mockSetSize).toHaveBeenCalledWith(1280, 800);
    });

    // Position must NOT be restored because the rect doesn't intersect any
    // connected screen's WorkArea.
    expect(mockSetPosition).not.toHaveBeenCalled();
  });

  /**
   * Restore-feedback loop suppression (Task 4.4 / R4).
   *
   * After mount, the OS will fire WindowDidMove/Resize as a side effect of
   * SetSize/SetPosition. Those echo events must NOT trigger a re-save of the
   * just-restored values.
   */
  test('events fired immediately after restore do not re-save', async () => {
    vi.useFakeTimers();

    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        windowGeometry: { x: 400, y: 300, width: 1100, height: 750 },
      }),
    );

    // After restore, Position()/Size() return the just-restored values.
    mockPosition.mockResolvedValue({ x: 400, y: 300 });
    mockSize.mockResolvedValue({ width: 1100, height: 750 });

    const { default: App } = await import('./App');
    render(<App />);

    await vi.waitFor(() => {
      expect(eventHandlers['common:WindowDidMove']?.length ?? 0).toBeGreaterThan(0);
      expect(mockSetSize).toHaveBeenCalled();
    });

    // Track writes after restore completes.
    const setItemSpy = vi.spyOn(window.localStorage, 'setItem');

    // Echo events fire immediately after restore.
    await act(async () => {
      emit('common:WindowDidMove', {});
      emit('common:WindowDidResize', {});
      await vi.advanceTimersByTimeAsync(0);
    });

    // Advance past the debounce window.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });

    const writes = setItemSpy.mock.calls.filter(([key]) => key === STORAGE_KEY);
    expect(writes.length).toBe(0);

    setItemSpy.mockRestore();
  });

  /**
   * Listeners are removed on unmount (Task 4.3).
   *
   * Important for HMR + unit tests; the production root never unmounts but
   * cleanup must exist.
   */
  test('WindowDidMove and WindowDidResize listeners are removed on unmount', async () => {
    const { default: App } = await import('./App');
    const { unmount } = render(<App />);

    await waitFor(() => {
      expect(eventHandlers['common:WindowDidMove']?.length ?? 0).toBeGreaterThan(0);
      expect(eventHandlers['common:WindowDidResize']?.length ?? 0).toBeGreaterThan(0);
    });

    unmount();

    expect(eventHandlers['common:WindowDidMove']?.length ?? 0).toBe(0);
    expect(eventHandlers['common:WindowDidResize']?.length ?? 0).toBe(0);
  });

  /**
   * Corrupt localStorage JSON does not crash startup and skips restore
   * entirely.
   */
  test('corrupt localStorage does not crash and skips restore', async () => {
    window.localStorage.setItem(STORAGE_KEY, '{not valid json');

    const { default: App } = await import('./App');

    expect(() => render(<App />)).not.toThrow();

    await new Promise((r) => setTimeout(r, 0));
    await waitFor(() => {
      expect(eventHandlers['common:WindowDidMove']?.length ?? 0).toBeGreaterThan(0);
    });

    expect(mockSetSize).not.toHaveBeenCalled();
    expect(mockSetPosition).not.toHaveBeenCalled();
  });
});
