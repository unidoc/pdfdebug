/**
 * Story 4.4: OS File Association, Single Instance, and Window Persistence
 *
 * TDD RED PHASE: Tests MUST fail until story 4-4 is implemented.
 *
 * Unit tests for the useWindowPersistence hook:
 *   4.4-UNIT-001 [P1]: Panel sizes saved to window.localStorage on resize
 *   4.4-UNIT-002 [P1]: Panel sizes restored from window.localStorage on mount
 *   4.4-UNIT-003 [P1]: Panel sizes saved and restored round-trip
 *   4.4-UNIT-004 [P1]: Graceful fallback to null when window.localStorage is empty or corrupt
 *
 * Run: cd frontend && npx vitest run src/hooks/useWindowPersistence.test.ts
 */
import { renderHook, act } from '@testing-library/react';
import { describe, test, expect, beforeEach, vi, afterEach } from 'vitest';
import { useWindowPersistence, isValidGeometry, type WindowGeometry } from './useWindowPersistence';

const STORAGE_KEY = 'unidoc-pdf-debugger:window-state';

// Node 25+ has a built-in localStorage without .clear(). Provide a shim
// so tests run consistently across Node versions and jsdom environments.
function clearStorage() {
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Fallback: iterate and remove
    const keys: string[] = [];
    for (let i = 0; i < window.localStorage.length; i++) {
      const k = window.localStorage.key(i);
      if (k) keys.push(k);
    }
    keys.forEach((k) => window.localStorage.removeItem(k));
  }
}

describe('4.4 useWindowPersistence', () => {
  beforeEach(() => {
    clearStorage();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  /**
   * 4.4-UNIT-001 [P1]: Panel sizes saved to window.localStorage on resize.
   *
   * AC#3: When the user resizes panels, the new sizes are persisted to
   *       window.localStorage so they survive app restart.
   */
  test('4.4-UNIT-001 [P1]: panel sizes saved to window.localStorage on resize', () => {
    const { result } = renderHook(() => useWindowPersistence());

    act(() => {
      result.current.savePanelSizes({ treeWidth: 400, subPanelHeight: 200 });
    });

    // Flush the debounce timer (500ms per story spec)
    act(() => {
      vi.advanceTimersByTime(500);
    });

    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).not.toBeNull();

    const parsed = JSON.parse(stored!);
    expect(parsed.panelSizes.treeWidth).toBe(400);
    expect(parsed.panelSizes.subPanelHeight).toBe(200);
  });

  /**
   * 4.4-UNIT-002 [P1]: Panel sizes restored from window.localStorage on mount.
   *
   * AC#3: When the app starts with valid persisted state in window.localStorage,
   *       the hook returns the stored panel sizes.
   */
  test('4.4-UNIT-002 [P1]: panel sizes restored from window.localStorage on mount', () => {
    // Pre-populate window.localStorage with valid state
    const persistedState = {
      panelSizes: {
        treeWidth: 450,
        subPanelHeight: 180,
      },
    };
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(persistedState));

    const { result } = renderHook(() => useWindowPersistence());

    expect(result.current.panelSizes).not.toBeNull();
    expect(result.current.panelSizes!.treeWidth).toBe(450);
    expect(result.current.panelSizes!.subPanelHeight).toBe(180);
  });

  /**
   * 4.4-UNIT-003 [P1]: Panel sizes saved and restored round-trip.
   *
   * AC#3: Save sizes, then create a new hook instance, verify loaded
   *       sizes match saved sizes.
   */
  test('4.4-UNIT-003 [P1]: panel sizes saved and restored round-trip', () => {
    // First instance: save sizes
    const { result: first, unmount } = renderHook(() => useWindowPersistence());

    act(() => {
      first.current.savePanelSizes({ treeWidth: 350, subPanelHeight: 250 });
    });
    act(() => {
      vi.advanceTimersByTime(500);
    });

    unmount();

    // Second instance: restore sizes
    const { result: second } = renderHook(() => useWindowPersistence());

    expect(second.current.panelSizes).not.toBeNull();
    expect(second.current.panelSizes!.treeWidth).toBe(350);
    expect(second.current.panelSizes!.subPanelHeight).toBe(250);
  });

  /**
   * 4.4-UNIT-004 [P1]: Graceful fallback to null when window.localStorage is empty
   * or corrupt.
   *
   * AC#3: If window.localStorage is empty or corrupt, the application falls back to
   *       default panel sizes with no error.
   */
  test('4.4-UNIT-004 [P1]: fallback to null when window.localStorage is empty', () => {
    // window.localStorage is cleared in beforeEach -- nothing stored
    const { result } = renderHook(() => useWindowPersistence());
    expect(result.current.panelSizes).toBeNull();
  });

  test('4.4-UNIT-004 [P1]: fallback to null when window.localStorage has invalid JSON', () => {
    window.localStorage.setItem(STORAGE_KEY, '{invalid json!!!');

    const { result } = renderHook(() => useWindowPersistence());
    expect(result.current.panelSizes).toBeNull();
  });

  test('4.4-UNIT-004 [P1]: fallback to null when window.localStorage has invalid structure', () => {
    // Valid JSON but wrong shape -- treeWidth is negative
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ panelSizes: { treeWidth: -1, subPanelHeight: 200 } })
    );

    const { result } = renderHook(() => useWindowPersistence());
    expect(result.current.panelSizes).toBeNull();
  });

  test('4.4-UNIT-004 [P1]: fallback to null when window.localStorage has non-finite values', () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ panelSizes: { treeWidth: Infinity, subPanelHeight: 200 } })
    );

    const { result } = renderHook(() => useWindowPersistence());
    expect(result.current.panelSizes).toBeNull();
  });

  test('4.4-UNIT-004 [P1]: fallback to null when panelSizes is missing', () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({}));

    const { result } = renderHook(() => useWindowPersistence());
    expect(result.current.panelSizes).toBeNull();
  });

  /**
   * Supplemental: treePaneHeight is persisted and restored when present.
   */
  test('treePaneHeight is saved and restored when provided', () => {
    const { result: first, unmount } = renderHook(() => useWindowPersistence());

    act(() => {
      first.current.savePanelSizes({ treeWidth: 300, subPanelHeight: 150, treePaneHeight: 420 });
    });
    act(() => {
      vi.advanceTimersByTime(500);
    });

    unmount();

    const { result: second } = renderHook(() => useWindowPersistence());
    expect(second.current.panelSizes).not.toBeNull();
    expect(second.current.panelSizes!.treePaneHeight).toBe(420);
  });

  /**
   * Supplemental: treePaneHeight is omitted from result when not stored.
   */
  test('treePaneHeight is undefined when not stored', () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ panelSizes: { treeWidth: 300, subPanelHeight: 200 } })
    );

    const { result } = renderHook(() => useWindowPersistence());
    expect(result.current.panelSizes).not.toBeNull();
    expect(result.current.panelSizes!.treePaneHeight).toBeUndefined();
  });

  /**
   * Supplemental: invalid treePaneHeight is ignored (does not invalidate other fields).
   */
  test('invalid treePaneHeight is dropped but treeWidth and subPanelHeight are kept', () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ panelSizes: { treeWidth: 300, subPanelHeight: 200, treePaneHeight: -5 } })
    );

    const { result } = renderHook(() => useWindowPersistence());
    expect(result.current.panelSizes).not.toBeNull();
    expect(result.current.panelSizes!.treeWidth).toBe(300);
    expect(result.current.panelSizes!.subPanelHeight).toBe(200);
    expect(result.current.panelSizes!.treePaneHeight).toBeUndefined();
  });

  /**
   * Supplemental: debounce timer is cleaned up on unmount (no leaked timers).
   */
  test('debounce timer is cleaned up on unmount', () => {
    const { result, unmount } = renderHook(() => useWindowPersistence());

    act(() => {
      result.current.savePanelSizes({ treeWidth: 400, subPanelHeight: 200 });
    });

    // Unmount before the debounce fires
    unmount();

    // Advance past debounce -- should not throw or write to localStorage
    act(() => {
      vi.advanceTimersByTime(600);
    });

    // localStorage should not have been written to (the timer was cleared)
    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).toBeNull();
  });

  /**
   * Supplemental: savePanelSizes is debounced -- rapid calls only write once.
   */
  test('savePanelSizes debounces writes to window.localStorage', () => {
    const setItemSpy = vi.spyOn(window.localStorage, 'setItem');
    const { result } = renderHook(() => useWindowPersistence());

    // Rapid-fire multiple saves
    act(() => {
      result.current.savePanelSizes({ treeWidth: 300, subPanelHeight: 100 });
      result.current.savePanelSizes({ treeWidth: 310, subPanelHeight: 110 });
      result.current.savePanelSizes({ treeWidth: 320, subPanelHeight: 120 });
    });

    // Before debounce fires, no write should have happened
    const writesBefore = setItemSpy.mock.calls.filter(
      ([key]) => key === STORAGE_KEY
    ).length;
    expect(writesBefore).toBe(0);

    // Flush debounce
    act(() => {
      vi.advanceTimersByTime(500);
    });

    // Only one write with the last values
    const writesAfter = setItemSpy.mock.calls.filter(
      ([key]) => key === STORAGE_KEY
    );
    expect(writesAfter.length).toBe(1);

    const parsed = JSON.parse(writesAfter[0][1] as string);
    expect(parsed.panelSizes.treeWidth).toBe(320);
    expect(parsed.panelSizes.subPanelHeight).toBe(120);

    setItemSpy.mockRestore();
  });

  /**
   * Supplemental: hook returns savePanelSizes function.
   */
  test('hook returns panelSizes and savePanelSizes', () => {
    const { result } = renderHook(() => useWindowPersistence());

    expect(result.current).toHaveProperty('panelSizes');
    expect(result.current).toHaveProperty('savePanelSizes');
    expect(typeof result.current.savePanelSizes).toBe('function');
  });
});

/**
 * Story 8.4: Window Geometry Persistence
 *
 * TDD RED PHASE: Tests MUST fail until story 8-4 is implemented.
 *
 *   8.4-UNIT-001..006 [P1]
 *   8.4-UNIT-007..008 [P2]
 *
 * Run: cd frontend && npx vitest run src/hooks/useWindowPersistence.test.ts
 */
describe('8.4 useWindowPersistence (window geometry)', () => {
  beforeEach(() => {
    clearStorage();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  /**
   * 8.4-UNIT-001 [P1]: Geometry saved to window.localStorage on saveWindowGeometry
   * (debounced 500ms).
   *
   * AC#1, AC#4: Geometry change triggers a debounced write at +500ms.
   */
  test('8.4-UNIT-001 [P1]: geometry saved to window.localStorage on saveWindowGeometry (debounced 500ms)', () => {
    const { result } = renderHook(() => useWindowPersistence());

    act(() => {
      result.current.saveWindowGeometry({ x: 100, y: 80, width: 1280, height: 800 });
    });

    // Before debounce, no write
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();

    act(() => {
      vi.advanceTimersByTime(500);
    });

    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).not.toBeNull();
    const parsed = JSON.parse(stored!);
    expect(parsed.windowGeometry).toEqual({ x: 100, y: 80, width: 1280, height: 800 });
  });

  /**
   * 8.4-UNIT-002 [P1]: Geometry restored from window.localStorage on mount.
   *
   * AC#1: Hook returns persisted geometry on mount when valid state exists.
   */
  test('8.4-UNIT-002 [P1]: geometry restored from window.localStorage on mount', () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        windowGeometry: { x: 200, y: 150, width: 1024, height: 768 },
      }),
    );

    const { result } = renderHook(() => useWindowPersistence());

    expect(result.current.windowGeometry).not.toBeNull();
    expect(result.current.windowGeometry).toEqual({ x: 200, y: 150, width: 1024, height: 768 });
  });

  /**
   * 8.4-UNIT-003 [P1]: Returns null geometry when localStorage is empty.
   *
   * AC#3: Empty localStorage produces null geometry (no error).
   */
  test('8.4-UNIT-003 [P1]: returns null geometry when localStorage is empty', () => {
    const { result } = renderHook(() => useWindowPersistence());
    expect(result.current.windowGeometry).toBeNull();
  });

  /**
   * 8.4-UNIT-004 [P1]: Returns null geometry when geometry shape is corrupt;
   * panelSizes still load if present (forward/backward compat).
   *
   * AC#3, AC#5: Independent validation per field.
   */
  test('8.4-UNIT-004 [P1]: corrupt geometry returns null but valid panelSizes still load', () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        panelSizes: { treeWidth: 350, subPanelHeight: 180 },
        windowGeometry: { x: 'not a number', y: 0, width: 100, height: 100 },
      }),
    );

    const { result } = renderHook(() => useWindowPersistence());

    expect(result.current.windowGeometry).toBeNull();
    expect(result.current.panelSizes).not.toBeNull();
    expect(result.current.panelSizes!.treeWidth).toBe(350);
    expect(result.current.panelSizes!.subPanelHeight).toBe(180);
  });

  /**
   * 8.4-UNIT-005 [P1]: saveWindowGeometry followed by savePanelSizes (or vice versa)
   * within 500ms produces exactly ONE localStorage write at +500ms after the
   * second call. The single write must contain BOTH fields. (AC#5)
   *
   * Shared timer is reset on each call.
   */
  test('8.4-UNIT-005 [P1]: shared 500ms debounce coalesces geometry and panel saves into one write with both fields', () => {
    const setItemSpy = vi.spyOn(window.localStorage, 'setItem');
    const { result } = renderHook(() => useWindowPersistence());

    // First call: save geometry
    act(() => {
      result.current.saveWindowGeometry({ x: 50, y: 60, width: 1200, height: 700 });
    });

    // 200ms later: save panel sizes (within debounce window)
    act(() => {
      vi.advanceTimersByTime(200);
      result.current.savePanelSizes({ treeWidth: 320, subPanelHeight: 200 });
    });

    // 499ms after the second call: still no write (timer was reset)
    act(() => {
      vi.advanceTimersByTime(499);
    });
    const writesMid = setItemSpy.mock.calls.filter(([key]) => key === STORAGE_KEY).length;
    expect(writesMid).toBe(0);

    // Cross the 500ms threshold from the SECOND call
    act(() => {
      vi.advanceTimersByTime(1);
    });

    const writes = setItemSpy.mock.calls.filter(([key]) => key === STORAGE_KEY);
    expect(writes.length).toBe(1);

    const parsed = JSON.parse(writes[0][1] as string);
    expect(parsed.windowGeometry).toEqual({ x: 50, y: 60, width: 1200, height: 700 });
    expect(parsed.panelSizes.treeWidth).toBe(320);
    expect(parsed.panelSizes.subPanelHeight).toBe(200);

    setItemSpy.mockRestore();
  });

  /**
   * 8.4-UNIT-006 [P1]: Saving geometry does not erase a previously-stored
   * panelSizes value already in localStorage (read-merge-write).
   *
   * AC#5: A geometry-only save preserves the existing panelSizes field.
   */
  test('8.4-UNIT-006 [P1]: saving geometry preserves existing panelSizes in localStorage (read-merge-write)', () => {
    // Pre-existing panelSizes in localStorage from an earlier session
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        panelSizes: { treeWidth: 400, subPanelHeight: 250, treePaneHeight: 500 },
      }),
    );

    const { result } = renderHook(() => useWindowPersistence());

    act(() => {
      result.current.saveWindowGeometry({ x: 10, y: 20, width: 1000, height: 600 });
    });
    act(() => {
      vi.advanceTimersByTime(500);
    });

    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).not.toBeNull();
    const parsed = JSON.parse(stored!);

    // Geometry was saved
    expect(parsed.windowGeometry).toEqual({ x: 10, y: 20, width: 1000, height: 600 });
    // Pre-existing panel sizes were not erased
    expect(parsed.panelSizes).toEqual({ treeWidth: 400, subPanelHeight: 250, treePaneHeight: 500 });
  });

  /**
   * 8.4-UNIT-007 [P2]: isValidGeometry() validator semantics.
   *
   * AC#5: Validator allows negative x/y for multi-monitor;
   *       rejects non-finite/NaN x/y/width/height;
   *       rejects zero or negative width/height.
   */
  test('8.4-UNIT-007 [P2]: isValidGeometry allows negative x/y, rejects non-finite or non-positive width/height', () => {
    // Allowed: positive width/height, any finite x/y (including negative for multi-monitor)
    expect(isValidGeometry({ x: 100, y: 200, width: 1024, height: 768 })).toBe(true);
    expect(isValidGeometry({ x: -800, y: -500, width: 1024, height: 768 })).toBe(true);
    expect(isValidGeometry({ x: 0, y: 0, width: 800, height: 600 })).toBe(true);

    // Rejected: zero width/height
    expect(isValidGeometry({ x: 0, y: 0, width: 0, height: 600 })).toBe(false);
    expect(isValidGeometry({ x: 0, y: 0, width: 800, height: 0 })).toBe(false);

    // Rejected: negative width/height
    expect(isValidGeometry({ x: 0, y: 0, width: -1, height: 600 })).toBe(false);
    expect(isValidGeometry({ x: 0, y: 0, width: 800, height: -10 })).toBe(false);

    // Rejected: non-finite
    expect(isValidGeometry({ x: Infinity, y: 0, width: 800, height: 600 })).toBe(false);
    expect(isValidGeometry({ x: 0, y: -Infinity, width: 800, height: 600 })).toBe(false);
    expect(isValidGeometry({ x: 0, y: 0, width: NaN, height: 600 })).toBe(false);
    expect(isValidGeometry({ x: 0, y: 0, width: 800, height: NaN })).toBe(false);
    expect(isValidGeometry({ x: NaN, y: 0, width: 800, height: 600 })).toBe(false);

    // Rejected: wrong types
    expect(isValidGeometry({ x: '1', y: 0, width: 800, height: 600 } as unknown as WindowGeometry)).toBe(false);
    expect(isValidGeometry(null as unknown as WindowGeometry)).toBe(false);
    expect(isValidGeometry(undefined as unknown as WindowGeometry)).toBe(false);
    expect(isValidGeometry({} as unknown as WindowGeometry)).toBe(false);
  });

  /**
   * 8.4-UNIT-008 [P2]: Loader returns windowGeometry: null but a valid
   * panelSizes when the persisted geometry is corrupt (forward/backward compat).
   *
   * AC#5: Mirrors UNIT-004 from the inverse direction (corrupt geometry +
   *       valid panelSizes still loads panelSizes); this case validates
   *       that the panelSizes load path is unaffected by the geometry path.
   */
  test('8.4-UNIT-008 [P2]: loader returns null geometry but valid panelSizes when geometry shape mismatches', () => {
    // windowGeometry has only partial fields -- shape mismatch
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        panelSizes: { treeWidth: 300, subPanelHeight: 150, treePaneHeight: 400 },
        windowGeometry: { x: 10, y: 20 }, // missing width and height
      }),
    );

    const { result } = renderHook(() => useWindowPersistence());

    expect(result.current.windowGeometry).toBeNull();
    expect(result.current.panelSizes).not.toBeNull();
    expect(result.current.panelSizes!.treeWidth).toBe(300);
    expect(result.current.panelSizes!.subPanelHeight).toBe(150);
    expect(result.current.panelSizes!.treePaneHeight).toBe(400);
  });

  /**
   * Supplemental (AC#5 reverse direction): saving panel sizes does not erase
   * a previously-stored windowGeometry value.
   */
  test('saving panel sizes preserves existing windowGeometry in localStorage (read-merge-write)', () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        windowGeometry: { x: 100, y: 100, width: 1024, height: 768 },
      }),
    );

    const { result } = renderHook(() => useWindowPersistence());

    act(() => {
      result.current.savePanelSizes({ treeWidth: 320, subPanelHeight: 180 });
    });
    act(() => {
      vi.advanceTimersByTime(500);
    });

    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).not.toBeNull();
    const parsed = JSON.parse(stored!);
    expect(parsed.windowGeometry).toEqual({ x: 100, y: 100, width: 1024, height: 768 });
    expect(parsed.panelSizes.treeWidth).toBe(320);
    expect(parsed.panelSizes.subPanelHeight).toBe(180);
  });

  /**
   * Supplemental: hook return shape includes windowGeometry and saveWindowGeometry.
   */
  test('hook returns windowGeometry and saveWindowGeometry', () => {
    const { result } = renderHook(() => useWindowPersistence());

    expect(result.current).toHaveProperty('windowGeometry');
    expect(result.current).toHaveProperty('saveWindowGeometry');
    expect(typeof result.current.saveWindowGeometry).toBe('function');
  });

  /**
   * AC#4 cross-instance shared-timer regression: when two hook instances are
   * mounted (App.jsx + MainLayout.tsx in production), a panel save from one
   * instance and a geometry save from the other within the debounce window
   * must coalesce into a SINGLE write. If timers were per-instance the
   * panel save from instance A and the geometry save from instance B
   * would each fire their own setTimeout and produce two writes.
   */
  test('AC#4: shared timer coalesces saves across multiple hook instances into one write', () => {
    const setItemSpy = vi.spyOn(window.localStorage, 'setItem');

    const a = renderHook(() => useWindowPersistence());
    const b = renderHook(() => useWindowPersistence());

    // Instance A saves geometry; instance B saves panel sizes.
    act(() => {
      a.result.current.saveWindowGeometry({ x: 50, y: 60, width: 1200, height: 700 });
    });
    act(() => {
      vi.advanceTimersByTime(200);
      b.result.current.savePanelSizes({ treeWidth: 320, subPanelHeight: 200 });
    });

    // 499ms after the second call: still no write.
    act(() => {
      vi.advanceTimersByTime(499);
    });
    expect(
      setItemSpy.mock.calls.filter(([key]) => key === STORAGE_KEY).length,
    ).toBe(0);

    // Cross 500ms threshold from the SECOND call.
    act(() => {
      vi.advanceTimersByTime(1);
    });

    const writes = setItemSpy.mock.calls.filter(([key]) => key === STORAGE_KEY);
    expect(writes.length).toBe(1);
    const parsed = JSON.parse(writes[0][1] as string);
    expect(parsed.windowGeometry).toEqual({ x: 50, y: 60, width: 1200, height: 700 });
    expect(parsed.panelSizes.treeWidth).toBe(320);
    expect(parsed.panelSizes.subPanelHeight).toBe(200);

    setItemSpy.mockRestore();
  });
});
