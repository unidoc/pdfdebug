/**
 * Frontend Hook and Render-Path Correctness (finding #9) --
 * useWindowPersistence flushes pending writes on the last unmount instead of
 * discarding them.
 *
 * The cleanup at useWindowPersistence.ts:184-192 calls flush() synchronously
 * when activeHookCount hits 0 and a pending write exists (pendingPanelSizes !==
 * null || pendingGeometry !== null), rather than nulling the buffers.
 *
 * The test proves the flush ran SYNCHRONOUSLY on unmount (not via the 500ms
 * debounce): fake timers are installed and never advanced.
 *
 * Module-scoped shared state (activeHookCount / pending buffers / sharedTimer)
 * lives at module scope in the hook. This file balances every mount with an
 * unmount so the count returns to 0 between tests; each test starts from a
 * clean count because the prior test unmounted its last consumer.
 *
 * Test IDs follow the convention.
 *
 * Run: cd frontend && npx vitest run src/hooks/useWindowPersistence.flush-on-last-unmount.test.ts
 */
import { renderHook, act } from '@testing-library/react';
import { describe, test, expect, beforeEach, afterEach, vi } from 'vitest';
import { useWindowPersistence } from './useWindowPersistence';

const STORAGE_KEY = 'unidoc-pdf-debugger:window-state';

function clearStorage() {
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* ignore */
  }
}

describe('useWindowPersistence flushes on last unmount', () => {
  beforeEach(() => {
    clearStorage();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    clearStorage();
  });

  test('a pending geometry save is persisted on the last unmount WITHOUT advancing timers', () => {
    const { result, unmount } = renderHook(() => useWindowPersistence());

    act(() => {
      result.current.saveWindowGeometry({ x: 12, y: 34, width: 1280, height: 800 });
    });
    // Pending write is buffered; debounce timer has NOT fired.
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();

    // Last consumer unmounts. this must flush() synchronously.
    unmount();

    // No fake-timer advance. If the write is present, the flush was synchronous.
    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).not.toBeNull();
    const parsed = JSON.parse(stored!);
    expect(parsed.windowGeometry).toEqual({ x: 12, y: 34, width: 1280, height: 800 });
  });

  test('a pending panel-size save is persisted on the last unmount WITHOUT advancing timers', () => {
    const { result, unmount } = renderHook(() => useWindowPersistence());

    act(() => {
      result.current.savePanelSizes({ treeWidth: 333, subPanelHeight: 222 });
    });
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();

    unmount();

    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).not.toBeNull();
    const parsed = JSON.parse(stored!);
    expect(parsed.panelSizes.treeWidth).toBe(333);
    expect(parsed.panelSizes.subPanelHeight).toBe(222);
  });

  test('only the LAST unmount flushes -- an earlier unmount of one of two consumers does not', () => {
    const a = renderHook(() => useWindowPersistence());
    const b = renderHook(() => useWindowPersistence());

    act(() => {
      a.result.current.saveWindowGeometry({ x: 1, y: 2, width: 900, height: 600 });
    });
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();

    // First consumer unmounts: count is still > 0, no flush, write stays pending.
    a.unmount();
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();

    // Last consumer unmounts: flush fires synchronously.
    b.unmount();
    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).not.toBeNull();
    const parsed = JSON.parse(stored!);
    expect(parsed.windowGeometry).toEqual({ x: 1, y: 2, width: 900, height: 600 });
  });

  test('no pending writes on last unmount leaves localStorage untouched (no spurious write)', () => {
    const { unmount } = renderHook(() => useWindowPersistence());
    // No save call -> no pending buffers.
    unmount();
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  // Gap (automate): flush() merges BOTH pending buffers in one write. The
  // activated suite covers geometry-only and panel-only flushes separately but
  // never both pending at the same last unmount -- the read-merge path the
  // shared flush() exists for.
  test('both pending panel-size and geometry writes are flushed together on last unmount', () => {
    const { result, unmount } = renderHook(() => useWindowPersistence());

    act(() => {
      result.current.savePanelSizes({ treeWidth: 300, subPanelHeight: 150 });
      result.current.saveWindowGeometry({ x: 5, y: 6, width: 1024, height: 768 });
    });
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();

    unmount();

    const stored = window.localStorage.getItem(STORAGE_KEY);
    expect(stored).not.toBeNull();
    const parsed = JSON.parse(stored!);
    expect(parsed.panelSizes.treeWidth).toBe(300);
    expect(parsed.panelSizes.subPanelHeight).toBe(150);
    expect(parsed.windowGeometry).toEqual({ x: 5, y: 6, width: 1024, height: 768 });
  });

  // Gap (automate): flush() reads existing localStorage and preserves the
  // untouched field. The debounce-path read-merge is tested in the 8.4 suite;
  // this asserts the same merge holds on the synchronous unmount-flush path.
  test('flush on last unmount preserves a pre-existing field it is not updating', () => {
    // Seed storage with panel sizes only.
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ panelSizes: { treeWidth: 250, subPanelHeight: 120 } }),
    );

    const { result, unmount } = renderHook(() => useWindowPersistence());
    act(() => {
      result.current.saveWindowGeometry({ x: 9, y: 9, width: 800, height: 600 });
    });

    unmount();

    const parsed = JSON.parse(window.localStorage.getItem(STORAGE_KEY)!);
    // Newly-flushed geometry plus the pre-existing panel sizes both survive.
    expect(parsed.windowGeometry).toEqual({ x: 9, y: 9, width: 800, height: 600 });
    expect(parsed.panelSizes).toEqual({ treeWidth: 250, subPanelHeight: 120 });
  });
});
