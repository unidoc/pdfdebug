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
import { useWindowPersistence } from './useWindowPersistence';

const STORAGE_KEY = 'unipdf-debugger:window-state';

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
